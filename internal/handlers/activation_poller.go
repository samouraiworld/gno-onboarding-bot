package handlers

import (
	"context"
	"fmt"
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

// discordActions is the subset of *discordgo.Session the poller calls, extracted
// so the promotion logic can be unit-tested without a live Discord session.
type discordActions interface {
	GuildMemberRoleAdd(guildID, userID, roleID string, options ...discordgo.RequestOption) error
	GuildMemberRoleRemove(guildID, userID, roleID string, options ...discordgo.RequestOption) error
	UserChannelCreate(recipientID string, options ...discordgo.RequestOption) (*discordgo.Channel, error)
	ChannelMessageSend(channelID, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
}

type activationPoller struct {
	cfg   *config.Config
	api   sheet.API
	tpl   *templates.Templates
	chain chainClient
	disc  discordActions
	logf  func(format string, v ...any)
	// warned throttles per-row problem logging: a row that fails the same way on
	// consecutive ticks is logged once, until the failure clears or changes, so a
	// deregistered valoper or a broken Discord link does not spam the log every
	// tick forever. Mutated only from the single poller goroutine.
	warned map[int]string
}

func newActivationPoller(s *discordgo.Session, cfg *config.Config, api sheet.API, tpl *templates.Templates, chain chainClient) *activationPoller {
	return &activationPoller{cfg: cfg, api: api, tpl: tpl, chain: chain, disc: s, logf: log.Printf, warned: map[int]string{}}
}

// StartActivationPoller launches a goroutine that, every `every`, promotes
// "GovDAO pending" candidates whose validator has joined the active set:
// it writes "GovDAO approved", grants the Testnet Validator role (removing the
// Candidate role), and DMs the candidate. Runs until ctx is cancelled. The
// returned channel is closed once the goroutine has observed ctx.Done() and
// exited, so callers can wait for any in-flight tick to finish before
// tearing down dependencies (e.g. closing the Discord session).
func StartActivationPoller(ctx context.Context, s *discordgo.Session, cfg *config.Config, api sheet.API, tpl *templates.Templates, chain chainClient, every time.Duration) <-chan struct{} {
	p := newActivationPoller(s, cfg, api, tpl, chain)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.tick(ctx)
			}
		}
	}()
	return done
}

func (p *activationPoller) tick(ctx context.Context) {
	set, err := p.chain.ValidatorSet(ctx)
	if err != nil {
		p.logf("activation: fetch validator set: %v", err)
		return
	}
	rows, err := sheet.ReadCandidates(ctx, p.api, p.cfg.SheetID, p.cfg.SheetName)
	if err != nil {
		p.logf("activation: read candidates: %v", err)
		return
	}
	pending := map[int]bool{}
	for _, r := range rows {
		if strings.TrimSpace(r.Status) != sheet.StatusGovDAOPending || strings.TrimSpace(r.OperatorAddress) == "" {
			continue
		}
		pending[r.Row] = true
		raw, err := p.chain.Render(ctx, valoper.RealmPath+":"+strings.TrimSpace(r.OperatorAddress))
		if err != nil {
			p.warn(r.Row, "render", err)
			continue
		}
		_, _, signingAddr, _, err := valoper.ParseRender(raw)
		if err != nil {
			p.warn(r.Row, "parse", err)
			continue
		}
		if signingAddr == "" {
			p.clearWarn(r.Row)
			continue
		}
		if _, active := set[signingAddr]; !active {
			p.clearWarn(r.Row)
			continue
		}
		p.activate(ctx, r)
	}
	// Drop throttle state for rows no longer pending (activated, declined, or
	// removed) so a later reuse of the same row number starts fresh.
	for row := range p.warned {
		if !pending[row] {
			delete(p.warned, row)
		}
	}
}

func (p *activationPoller) activate(ctx context.Context, r sheet.TrackerRow) {
	link, err := p.api.CellLink(ctx, p.cfg.SheetID, p.cfg.SheetName, r.Row, int(sheet.ColumnDiscord))
	if err != nil {
		p.warn(r.Row, "discord-link-read", err)
		return
	}
	candidateID, ok := sheet.DiscordIDFromUserURL(link)
	if !ok {
		// No fallback ID source exists (the numeric ID is only persisted in this
		// hyperlink). Throttle the warning so an unfixable row does not loop.
		p.warn(r.Row, "discord-id-unresolvable", fmt.Errorf("no valid users/<id> hyperlink in column B; grant the role manually"))
		return
	}
	// Re-check the row's status immediately before mutating: a reviewer Decline
	// that landed since this tick's bulk read must not be clobbered.
	switch status, err := sheet.ReadStatus(ctx, p.api, p.cfg.SheetID, p.cfg.SheetName, r.Row); {
	case err != nil:
		p.logf("activation: re-read status row %d: %v", r.Row, err)
		return
	case strings.TrimSpace(status) != sheet.StatusGovDAOPending:
		p.logf("activation: row %d no longer GovDAO pending (now %q); skipping", r.Row, status)
		return
	}
	// Sheet write before any role mutation (invariant).
	if err := sheet.UpdateFields(ctx, p.api, p.cfg.SheetID, p.cfg.SheetName, r.Row, map[sheet.Column]string{
		sheet.ColumnStatus: sheet.StatusGovDAOApproved,
	}); err != nil {
		p.logf("activation: set GovDAO approved row %d: %v", r.Row, err)
		return
	}
	if err := p.disc.GuildMemberRoleAdd(p.cfg.GuildID, candidateID, p.cfg.ValidatorRoleID); err != nil {
		// Roll the status back so the next tick retries instead of stranding the
		// candidate at GovDAO approved with no role (the pending-only filter would
		// otherwise never reselect it).
		p.logf("activation: add validator role for %s (row %d): %v — rolling back to GovDAO pending", candidateID, r.Row, err)
		if rbErr := sheet.UpdateFields(ctx, p.api, p.cfg.SheetID, p.cfg.SheetName, r.Row, map[sheet.Column]string{
			sheet.ColumnStatus: sheet.StatusGovDAOPending,
		}); rbErr != nil {
			p.logf("activation: rollback status row %d failed: %v — grant the role manually", r.Row, rbErr)
		}
		return
	}
	if err := p.disc.GuildMemberRoleRemove(p.cfg.GuildID, candidateID, p.cfg.CandidateRoleID); err != nil {
		p.logf("activation: remove candidate role for %s (row %d): %v — remove manually", candidateID, r.Row, err)
	}
	p.clearWarn(r.Row)
	msg, err := p.tpl.Activated()
	if err != nil {
		p.logf("activation: render activated template (row %d): %v", r.Row, err)
		return
	}
	if err := dmUser(p.disc, candidateID, msg); err != nil {
		p.logf("activation: DM candidate %s (row %d) failed (DMs may be closed): %v", candidateID, r.Row, err)
	}
	p.logf("activation: OK row %d user=%s moniker=%q", r.Row, candidateID, r.Moniker)
}

// warn logs a per-row problem at most once per distinct reason: a row that keeps
// failing the same way on later ticks is logged only when the reason first
// appears or changes.
func (p *activationPoller) warn(row int, reason string, err error) {
	if p.warned[row] == reason {
		return
	}
	p.warned[row] = reason
	p.logf("activation: row %d %s: %v (suppressing identical repeats until it changes)", row, reason, err)
}

func (p *activationPoller) clearWarn(row int) {
	delete(p.warned, row)
}

func dmUser(d discordActions, userID, content string) error {
	ch, err := d.UserChannelCreate(userID)
	if err != nil {
		return err
	}
	_, err = d.ChannelMessageSend(ch.ID, content)
	return err
}
