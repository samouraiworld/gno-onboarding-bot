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
	GuildMember(guildID, userID string, options ...discordgo.RequestOption) (*discordgo.Member, error)
	ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, options ...discordgo.RequestOption) (*discordgo.Message, error)
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
	// reconciled memoises GovDAO-approved rows whose candidate is confirmed to
	// hold the validator role, so each is role-checked at most once per process.
	reconciled map[int]bool
}

func newActivationPoller(s *discordgo.Session, cfg *config.Config, api sheet.API, tpl *templates.Templates, chain chainClient) *activationPoller {
	return &activationPoller{cfg: cfg, api: api, tpl: tpl, chain: chain, disc: s, logf: log.Printf, warned: map[int]string{}, reconciled: map[int]bool{}}
}

// StartActivationPoller launches a goroutine that, every `every`, promotes
// "GovDAO pending" candidates whose validator has joined the active set:
// it writes "GovDAO approved", grants the Testnet Validator role (removing the
// Candidate role), and posts an activation notice to the onboarding channel.
// Runs until ctx is cancelled. The
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
	live := map[int]bool{}
	for _, r := range rows {
		switch strings.TrimSpace(r.Status) {
		case sheet.StatusGovDAOPending:
			if strings.TrimSpace(r.OperatorAddress) == "" {
				continue
			}
			live[r.Row] = true
			p.promotePending(ctx, r, set)
		case sheet.StatusGovDAOApproved:
			live[r.Row] = true
			p.reconcileApproved(ctx, r)
		}
	}
	// Drop per-row throttle/reconcile state for rows that are no longer pending or
	// approved, so a later reuse of the same row number starts fresh.
	for row := range p.warned {
		if !live[row] {
			delete(p.warned, row)
		}
	}
	for row := range p.reconciled {
		if !live[row] {
			delete(p.reconciled, row)
		}
	}
}

func (p *activationPoller) promotePending(ctx context.Context, r sheet.TrackerRow, set map[string]struct{}) {
	raw, err := p.chain.Render(ctx, valoper.RealmPath+":"+strings.TrimSpace(r.OperatorAddress))
	if err != nil {
		p.warn(r.Row, "render", err)
		return
	}
	_, _, signingAddr, _, err := valoper.ParseRender(raw)
	if err != nil {
		p.warn(r.Row, "parse", err)
		return
	}
	if signingAddr == "" {
		p.clearWarn(r.Row)
		return
	}
	if _, active := set[signingAddr]; !active {
		p.clearWarn(r.Row)
		return
	}
	p.activate(ctx, r)
}

// reconcileApproved repairs a row left at GovDAO approved without the validator
// role — e.g. a crash between the status write and the role grant, which the
// pending-only promotion path would never revisit. It checks the candidate's
// roles at most once per process (memoised in reconciled) and grants the missing
// role (removing the candidate role and posting the activation notice) when it
// finds none.
func (p *activationPoller) reconcileApproved(ctx context.Context, r sheet.TrackerRow) {
	if p.reconciled[r.Row] {
		return
	}
	link, err := p.api.CellLink(ctx, p.cfg.SheetID, p.cfg.SheetName, r.Row, int(sheet.ColumnDiscord))
	if err != nil {
		p.warn(r.Row, "reconcile-link-read", err)
		return
	}
	candidateID, ok := sheet.DiscordIDFromUserURL(link)
	if !ok {
		p.warn(r.Row, "reconcile-id-unresolvable", fmt.Errorf("no valid users/<id> hyperlink in column B; grant the role manually"))
		return
	}
	member, err := p.disc.GuildMember(p.cfg.GuildID, candidateID)
	if err != nil {
		p.warn(r.Row, "reconcile-member-fetch", err)
		return
	}
	if hasRole(member, p.cfg.ValidatorRoleID) {
		p.reconciled[r.Row] = true
		p.clearWarn(r.Row)
		return
	}
	if err := p.disc.GuildMemberRoleAdd(p.cfg.GuildID, candidateID, p.cfg.ValidatorRoleID); err != nil {
		p.warn(r.Row, "reconcile-grant", err)
		return
	}
	if err := p.disc.GuildMemberRoleRemove(p.cfg.GuildID, candidateID, p.cfg.CandidateRoleID); err != nil {
		p.logf("activation: reconcile row %d remove candidate role for %s: %v — remove manually", r.Row, candidateID, err)
	}
	p.reconciled[r.Row] = true
	p.clearWarn(r.Row)
	if msg, terr := p.tpl.Activated(); terr != nil {
		p.logf("activation: reconcile row %d render template: %v", r.Row, terr)
	} else if derr := sendCandidateMessage(p.disc, p.cfg.OnboardingChannelID, candidateID, msg); derr != nil {
		p.logf("activation: reconcile row %d post activated notice for %s failed: %v", r.Row, candidateID, derr)
	}
	p.logf("activation: reconciled stranded row %d user=%s (granted missing validator role)", r.Row, candidateID)
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
	// The role is granted; this row is now in its terminal state with the role, so
	// the reconcile pass can skip it without a GuildMember fetch next tick.
	p.reconciled[r.Row] = true
	p.clearWarn(r.Row)
	msg, err := p.tpl.Activated()
	if err != nil {
		p.logf("activation: render activated template (row %d): %v", r.Row, err)
		return
	}
	if err := sendCandidateMessage(p.disc, p.cfg.OnboardingChannelID, candidateID, msg); err != nil {
		p.logf("activation: post activated notice for %s (row %d) failed: %v", candidateID, r.Row, err)
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
