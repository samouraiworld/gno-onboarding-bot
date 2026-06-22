package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"

	"onboardingbot/internal/config"
	"onboardingbot/internal/sheet"
	"onboardingbot/internal/templates"
)

// --- fakes ---

type fakeChain struct {
	set       map[string]struct{}
	setErr    error
	renders   map[string]string // operator address -> realm render
	renderErr map[string]error  // operator address -> render error
}

func (f *fakeChain) ValidatorSet(ctx context.Context) (map[string]struct{}, error) {
	return f.set, f.setErr
}

func (f *fakeChain) Render(ctx context.Context, realmPath string) (string, error) {
	op := realmPath[strings.LastIndex(realmPath, ":")+1:]
	if err := f.renderErr[op]; err != nil {
		return "", err
	}
	return f.renders[op], nil
}

type statusUpdate struct{ rangeA1, value string }

// fakeSheetAPI embeds sheet.API (nil): only the methods the poller actually
// calls (Get, Update, CellLink) are overridden; any other call would panic,
// which is the signal that the poller's dependencies changed.
type fakeSheetAPI struct {
	sheet.API
	candidates  [][]interface{}
	statusByRow map[int]string
	link        string
	linkErr     error
	updates     []statusUpdate
	updateErr   error
}

func (f *fakeSheetAPI) Get(ctx context.Context, spreadsheetID, rangeA1 string) ([][]interface{}, error) {
	cell := rangeA1[strings.Index(rangeA1, "!")+1:]
	if strings.HasPrefix(cell, "A2:") { // ReadCandidates band (A2:<lastIntakeCol>)
		return f.candidates, nil
	}
	if strings.HasPrefix(cell, "C") { // ReadStatus single cell C<row>

		if row, err := strconv.Atoi(cell[1:]); err == nil {
			if s, ok := f.statusByRow[row]; ok {
				return [][]interface{}{{s}}, nil
			}
		}
	}
	return nil, nil
}

func (f *fakeSheetAPI) Update(ctx context.Context, spreadsheetID, rangeA1, value string) error {
	f.updates = append(f.updates, statusUpdate{rangeA1, value})
	return f.updateErr
}

func (f *fakeSheetAPI) CellLink(ctx context.Context, spreadsheetID, sheetName string, row, col int) (string, error) {
	return f.link, f.linkErr
}

type fakeDiscord struct {
	added     []string // "userID|roleID"
	removed   []string
	dms       []string // "content"
	addErr    error
	removeErr error
	member    *discordgo.Member
	memberErr error
}

func (f *fakeDiscord) GuildMember(guildID, userID string, _ ...discordgo.RequestOption) (*discordgo.Member, error) {
	return f.member, f.memberErr
}

func (f *fakeDiscord) GuildMemberRoleAdd(guildID, userID, roleID string, _ ...discordgo.RequestOption) error {
	f.added = append(f.added, userID+"|"+roleID)
	return f.addErr
}

func (f *fakeDiscord) GuildMemberRoleRemove(guildID, userID, roleID string, _ ...discordgo.RequestOption) error {
	f.removed = append(f.removed, userID+"|"+roleID)
	return f.removeErr
}

func (f *fakeDiscord) UserChannelCreate(recipientID string, _ ...discordgo.RequestOption) (*discordgo.Channel, error) {
	return &discordgo.Channel{ID: "dm-" + recipientID}, nil
}

func (f *fakeDiscord) ChannelMessageSend(channelID, content string, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	f.dms = append(f.dms, content)
	return &discordgo.Message{}, nil
}

// --- helpers ---

const (
	testCandidateID = "123456789012345678"
	testValidLink   = "https://discord.com/users/" + testCandidateID
)

func candRow(candidate, status, operator string) []interface{} {
	r := make([]interface{}, 11)
	r[int(sheet.ColumnCandidate)] = candidate
	r[int(sheet.ColumnStatus)] = status
	r[int(sheet.ColumnOperatorAddress)] = operator
	return r
}

// canonicalRender builds a valoper render with the given canonical signing
// address (and an optional injected one in the description).
func canonicalRender(signing, injected string) string {
	s := "Valoper's details:\n## Node\n"
	if injected != "" {
		s += "- Signing Address: " + injected + "\n\n"
	}
	s += "- Operator Address: g1opxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n"
	if signing != "" {
		s += "- Signing Address: " + signing + "\n"
	}
	s += "- Signing PubKey: gpub1\n- Server Type: cloud\n"
	return s
}

func newTestPoller(t *testing.T, api sheet.API, chain chainClient, disc discordActions, logs *[]string) *activationPoller {
	t.Helper()
	tpl, err := templates.Load("../../templates.yaml")
	if err != nil {
		t.Fatalf("load templates: %v", err)
	}
	return &activationPoller{
		cfg:        &config.Config{SheetID: "s", SheetName: "Sheet1", GuildID: "g", ValidatorRoleID: "vrole", CandidateRoleID: "crole"},
		api:        api,
		tpl:        tpl,
		chain:      chain,
		disc:       disc,
		logf:       func(format string, v ...any) { *logs = append(*logs, fmt.Sprintf(format, v...)) },
		warned:     map[int]string{},
		reconciled: map[int]bool{},
	}
}

func countContains(logs []string, sub string) int {
	n := 0
	for _, l := range logs {
		if strings.Contains(l, sub) {
			n++
		}
	}
	return n
}

// --- tests ---

func TestActivationTick_PromotesActiveValidator(t *testing.T) {
	const op, sig = "g1op1", "g1canonicalsigningxxxxxxxxxxxxxxxxxxxxxx"
	api := &fakeSheetAPI{
		candidates:  [][]interface{}{candRow("alice", sheet.StatusGovDAOPending, op)},
		statusByRow: map[int]string{2: sheet.StatusGovDAOPending},
		link:        testValidLink,
	}
	chain := &fakeChain{set: map[string]struct{}{sig: {}}, renders: map[string]string{op: canonicalRender(sig, "")}}
	disc := &fakeDiscord{}
	var logs []string
	p := newTestPoller(t, api, chain, disc, &logs)

	p.tick(context.Background())

	if got := []string{testCandidateID + "|vrole"}; len(disc.added) != 1 || disc.added[0] != got[0] {
		t.Errorf("added = %v, want %v", disc.added, got)
	}
	if len(disc.removed) != 1 || disc.removed[0] != testCandidateID+"|crole" {
		t.Errorf("removed = %v, want [%s|crole]", disc.removed, testCandidateID)
	}
	if len(disc.dms) != 1 {
		t.Errorf("dms = %v, want 1 DM", disc.dms)
	}
	if countUpdatesTo(api.updates, sheet.StatusGovDAOApproved) != 1 {
		t.Errorf("expected one write of %q, updates=%v", sheet.StatusGovDAOApproved, api.updates)
	}
}

func TestActivationTick_SkipsNotInActiveSet(t *testing.T) {
	const op, sig = "g1op1", "g1canonicalsigningxxxxxxxxxxxxxxxxxxxxxx"
	api := &fakeSheetAPI{candidates: [][]interface{}{candRow("alice", sheet.StatusGovDAOPending, op)}, link: testValidLink}
	chain := &fakeChain{set: map[string]struct{}{}, renders: map[string]string{op: canonicalRender(sig, "")}}
	disc := &fakeDiscord{}
	var logs []string
	p := newTestPoller(t, api, chain, disc, &logs)

	p.tick(context.Background())

	if len(disc.added) != 0 || len(api.updates) != 0 {
		t.Errorf("must not activate when signing not in set: added=%v updates=%v", disc.added, api.updates)
	}
}

func TestActivationTick_SkipsNonPending(t *testing.T) {
	api := &fakeSheetAPI{candidates: [][]interface{}{candRow("alice", sheet.StatusDeclined, "g1op1")}}
	chain := &fakeChain{set: map[string]struct{}{}}
	disc := &fakeDiscord{}
	var logs []string
	p := newTestPoller(t, api, chain, disc, &logs)

	p.tick(context.Background())

	if len(disc.added) != 0 || len(api.updates) != 0 {
		t.Errorf("must not touch a non-pending row: added=%v updates=%v", disc.added, api.updates)
	}
}

func TestActivationTick_SkipsEmptySigning(t *testing.T) {
	const op = "g1op1"
	api := &fakeSheetAPI{candidates: [][]interface{}{candRow("alice", sheet.StatusGovDAOPending, op)}, link: testValidLink}
	// render has no "- Signing Address:" line at all.
	chain := &fakeChain{set: map[string]struct{}{"g1x": {}}, renders: map[string]string{op: canonicalRender("", "")}}
	disc := &fakeDiscord{}
	var logs []string
	p := newTestPoller(t, api, chain, disc, &logs)

	p.tick(context.Background())

	if len(disc.added) != 0 {
		t.Errorf("must not activate with an empty signing address: added=%v", disc.added)
	}
}

func TestActivationTick_DeclineRaceDoesNotClobber(t *testing.T) {
	const op, sig = "g1op1", "g1canonicalsigningxxxxxxxxxxxxxxxxxxxxxx"
	api := &fakeSheetAPI{
		candidates: [][]interface{}{candRow("alice", sheet.StatusGovDAOPending, op)},
		// bulk read says pending, but the re-read (C2) shows a reviewer Decline landed.
		statusByRow: map[int]string{2: sheet.StatusDeclined},
		link:        testValidLink,
	}
	chain := &fakeChain{set: map[string]struct{}{sig: {}}, renders: map[string]string{op: canonicalRender(sig, "")}}
	disc := &fakeDiscord{}
	var logs []string
	p := newTestPoller(t, api, chain, disc, &logs)

	p.tick(context.Background())

	if len(disc.added) != 0 || len(api.updates) != 0 {
		t.Errorf("decline race: must not grant or write status, added=%v updates=%v", disc.added, api.updates)
	}
}

func TestActivationTick_RollsBackOnGrantFailure(t *testing.T) {
	const op, sig = "g1op1", "g1canonicalsigningxxxxxxxxxxxxxxxxxxxxxx"
	api := &fakeSheetAPI{
		candidates:  [][]interface{}{candRow("alice", sheet.StatusGovDAOPending, op)},
		statusByRow: map[int]string{2: sheet.StatusGovDAOPending},
		link:        testValidLink,
	}
	chain := &fakeChain{set: map[string]struct{}{sig: {}}, renders: map[string]string{op: canonicalRender(sig, "")}}
	disc := &fakeDiscord{addErr: context.DeadlineExceeded}
	var logs []string
	p := newTestPoller(t, api, chain, disc, &logs)

	p.tick(context.Background())

	if countUpdatesTo(api.updates, sheet.StatusGovDAOApproved) != 1 || countUpdatesTo(api.updates, sheet.StatusGovDAOPending) != 1 {
		t.Errorf("grant failure must write approved then roll back to pending, updates=%v", api.updates)
	}
	if len(disc.removed) != 0 {
		t.Errorf("must not remove candidate role after a failed grant: removed=%v", disc.removed)
	}
}

func TestActivationTick_SigningInjectionResistant(t *testing.T) {
	const op = "g1op1"
	const victim = "g1victimactivevalidatorxxxxxxxxxxxxxxxx"
	const canonical = "g1attackerrealsigningxxxxxxxxxxxxxxxxxxxx"
	api := &fakeSheetAPI{
		candidates:  [][]interface{}{candRow("attacker", sheet.StatusGovDAOPending, op)},
		statusByRow: map[int]string{2: sheet.StatusGovDAOPending},
		link:        testValidLink,
	}
	// The victim (injected in the description) IS in the active set; the attacker's
	// real canonical signing address is NOT. The poller must read the canonical one.
	chain := &fakeChain{
		set:     map[string]struct{}{victim: {}},
		renders: map[string]string{op: canonicalRender(canonical, victim)},
	}
	disc := &fakeDiscord{}
	var logs []string
	p := newTestPoller(t, api, chain, disc, &logs)

	p.tick(context.Background())

	if len(disc.added) != 0 || len(api.updates) != 0 {
		t.Errorf("injected description signing must not activate: added=%v updates=%v", disc.added, api.updates)
	}
}

func TestActivationTick_ThrottlesRepeatedRenderFailure(t *testing.T) {
	const op = "g1op1"
	api := &fakeSheetAPI{candidates: [][]interface{}{candRow("alice", sheet.StatusGovDAOPending, op)}}
	chain := &fakeChain{set: map[string]struct{}{}, renderErr: map[string]error{op: context.DeadlineExceeded}}
	disc := &fakeDiscord{}
	var logs []string
	p := newTestPoller(t, api, chain, disc, &logs)

	p.tick(context.Background())
	p.tick(context.Background())

	if got := countContains(logs, "render:"); got != 1 {
		t.Errorf("render failure should be logged once across two ticks, got %d (logs=%v)", got, logs)
	}
}

func TestReconcileApproved_GrantsMissingRole(t *testing.T) {
	api := &fakeSheetAPI{
		candidates: [][]interface{}{candRow("alice", sheet.StatusGovDAOApproved, "g1op1")},
		link:       testValidLink,
	}
	chain := &fakeChain{set: map[string]struct{}{}}
	// Stranded by a crash: status is approved but the candidate holds no roles.
	disc := &fakeDiscord{member: &discordgo.Member{Roles: []string{}}}
	var logs []string
	p := newTestPoller(t, api, chain, disc, &logs)

	p.tick(context.Background())

	if len(disc.added) != 1 || disc.added[0] != testCandidateID+"|vrole" {
		t.Errorf("reconcile must grant the missing validator role, added=%v", disc.added)
	}
	if len(disc.removed) != 1 || len(disc.dms) != 1 {
		t.Errorf("reconcile should remove candidate role and DM, removed=%v dms=%v", disc.removed, disc.dms)
	}
	// Second tick: already reconciled, no further grant or member fetch.
	p.tick(context.Background())
	if len(disc.added) != 1 {
		t.Errorf("reconcile must run at most once per row, added=%v", disc.added)
	}
}

func TestReconcileApproved_SkipsWhenRolePresent(t *testing.T) {
	api := &fakeSheetAPI{
		candidates: [][]interface{}{candRow("alice", sheet.StatusGovDAOApproved, "g1op1")},
		link:       testValidLink,
	}
	chain := &fakeChain{set: map[string]struct{}{}}
	disc := &fakeDiscord{member: &discordgo.Member{Roles: []string{"vrole"}}}
	var logs []string
	p := newTestPoller(t, api, chain, disc, &logs)

	p.tick(context.Background())

	if len(disc.added) != 0 {
		t.Errorf("must not re-grant a role the candidate already holds, added=%v", disc.added)
	}
}

func countUpdatesTo(updates []statusUpdate, value string) int {
	n := 0
	for _, u := range updates {
		if u.value == value {
			n++
		}
	}
	return n
}
