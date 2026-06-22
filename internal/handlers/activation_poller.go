package handlers

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"onboardingbot/internal/config"
	"onboardingbot/internal/sheet"
	"onboardingbot/internal/templates"
	"onboardingbot/internal/valoper"
)

// chainClient is the subset of *valoper.Client the poller needs: render a realm
// (to derive a signing address) and read the active validator set.
type chainClient interface {
	Render(ctx context.Context, realmPath string) (string, error)
	ValidatorSet(ctx context.Context) (map[string]struct{}, error)
}

// StartActivationPoller launches a goroutine that, every `every`, promotes
// "GovDAO pending" candidates whose validator has joined the active set:
// it writes "GovDAO submitted", grants the Testnet Validator role (removing the
// Candidate role), and DMs the candidate. Runs until ctx is cancelled.
func StartActivationPoller(ctx context.Context, s *discordgo.Session, cfg *config.Config, api sheet.API, tpl *templates.Templates, chain chainClient, every time.Duration) {
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runActivationTick(ctx, s, cfg, api, tpl, chain)
			}
		}
	}()
}

func runActivationTick(ctx context.Context, s *discordgo.Session, cfg *config.Config, api sheet.API, tpl *templates.Templates, chain chainClient) {
	set, err := chain.ValidatorSet(ctx)
	if err != nil {
		log.Printf("activation: fetch validator set: %v", err)
		return
	}
	rows, err := sheet.ReadCandidates(ctx, api, cfg.SheetID, cfg.SheetName)
	if err != nil {
		log.Printf("activation: read candidates: %v", err)
		return
	}
	for _, r := range rows {
		if strings.TrimSpace(r.Status) != sheet.StatusGovDAOPending || strings.TrimSpace(r.OperatorAddress) == "" {
			continue
		}
		raw, err := chain.Render(ctx, valoper.RealmPath+":"+strings.TrimSpace(r.OperatorAddress))
		if err != nil {
			log.Printf("activation: render row %d: %v", r.Row, err)
			continue
		}
		_, _, signingAddr, _, err := valoper.ParseRender(raw)
		if err != nil {
			log.Printf("activation: parse row %d: %v", r.Row, err)
			continue
		}
		if signingAddr == "" {
			continue
		}
		if _, active := set[signingAddr]; !active {
			continue
		}
		activateCandidate(ctx, s, cfg, api, tpl, r)
	}
}

func activateCandidate(ctx context.Context, s *discordgo.Session, cfg *config.Config, api sheet.API, tpl *templates.Templates, r sheet.TrackerRow) {
	link, err := api.CellLink(ctx, cfg.SheetID, cfg.SheetName, r.Row, int(sheet.ColumnDiscord))
	if err != nil {
		log.Printf("activation: read Discord link row %d: %v", r.Row, err)
		return
	}
	candidateID, ok := sheet.DiscordIDFromUserURL(link)
	if !ok {
		log.Printf("activation: row %d has no resolvable Discord ID; grant the role manually", r.Row)
		return
	}
	// Sheet write before any role mutation (invariant).
	if err := sheet.UpdateFields(ctx, api, cfg.SheetID, cfg.SheetName, r.Row, map[sheet.Column]string{
		sheet.ColumnStatus: sheet.StatusGovDAOSubmitted,
	}); err != nil {
		log.Printf("activation: set GovDAO submitted row %d: %v", r.Row, err)
		return
	}
	if err := s.GuildMemberRoleAdd(cfg.GuildID, candidateID, cfg.ValidatorRoleID); err != nil {
		log.Printf("activation: add validator role for %s (row %d): %v — grant manually", candidateID, r.Row, err)
		return
	}
	if err := s.GuildMemberRoleRemove(cfg.GuildID, candidateID, cfg.CandidateRoleID); err != nil {
		log.Printf("activation: remove candidate role for %s (row %d): %v — remove manually", candidateID, r.Row, err)
	}
	msg, err := tpl.Activated()
	if err != nil {
		log.Printf("activation: render activated template (row %d): %v", r.Row, err)
		return
	}
	if err := sendDM(s, candidateID, msg); err != nil {
		log.Printf("activation: DM candidate %s (row %d) failed (DMs may be closed): %v", candidateID, r.Row, err)
	}
	log.Printf("activation: OK row %d user=%s moniker=%q", r.Row, candidateID, r.Moniker)
}
