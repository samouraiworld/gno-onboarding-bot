// Package harvest holds the pure, deterministic logic for the end-of-window
// pass: attributing Discord messages to candidates, computing signals,
// redacting secrets, and the harvest.json / digest.json contracts shared with
// the competency-digest skill. It performs no Discord or Sheet I/O.
package harvest

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Channel keys are stable identifiers used in JSON and the Engagement column.
const (
	ChannelGeneral    = "general"
	ChannelOnboarding = "onboarding"
	ChannelReview     = "review"
)

// Criteria are the seven Shared.md acceptance criteria, in display order.
var Criteria = []string{"setup", "sync", "tx", "valoper", "ops", "comms", "safety"}

// Criterion states a candidate can hold for each criterion.
const (
	StateFound      = "found"
	StateNotFound   = "not_found"
	StateNeedsCheck = "needs_human_check"
)

// Readiness bands, from most to least evidence.
const (
	ReadinessHigh    = "High"
	ReadinessMedium  = "Medium"
	ReadinessLow     = "Low"
	ReadinessNeutral = "Neutral"
)

const (
	maxLinks = 25
	maxTx    = 25
)

var (
	reURL = regexp.MustCompile(`https?://[^\s<>|]+`)
	reTx  = regexp.MustCompile(`\b[0-9a-fA-F]{64}\b`)
)

// Message is one harvested Discord message, already mapped to a channel key.
type Message struct {
	ChannelKey     string
	AuthorID       string
	AuthorUsername string
	Content        string
	Timestamp      time.Time
	Permalink      string
}

// CandidateRecord is a tracker row the digest attaches to. ResolvedUserID is
// filled by the caller from Discord when known; matching falls back to the
// username stored in Discord otherwise.
type CandidateRecord struct {
	Row            int
	Candidate      string
	Discord        string // "@username" as stored in the Sheet
	ResolvedUserID string
	Moniker        string
	Valoper        string
	Introduction   string
	// ValoperState / ValoperDetail are the bot's deterministic valoper verdict,
	// computed by the handler (it does the fetch) and copied into Signals.
	ValoperState  string
	ValoperDetail string
}

// --- harvest.json (bot -> skill) ---

type HarvestFile struct {
	GeneratedAt string      `json:"generated_at"`
	GuildID     string      `json:"guild_id"`
	Criteria    []string    `json:"criteria"`
	Candidates  []Candidate `json:"candidates"`
}

type Candidate struct {
	Row            int               `json:"row"`
	Candidate      string            `json:"candidate"`
	Discord        string            `json:"discord"`
	ResolvedUserID string            `json:"resolved_user_id,omitempty"`
	Submitted      Submitted         `json:"submitted"`
	Signals        Signals           `json:"signals"`
	Messages       []EvidenceMessage `json:"messages"`
	ReviewerCtx    []ReviewerMessage `json:"reviewer_context"`
}

type Submitted struct {
	MonikerAddress string `json:"moniker_address"`
	ValoperLink    string `json:"valoper_link"`
	Introduction   string `json:"introduction"`
}

type Signals struct {
	MessageCount     int            `json:"message_count"`
	ActiveDays       int            `json:"active_days"`
	FirstActivity    string         `json:"first_activity,omitempty"`
	LastActivity     string         `json:"last_activity,omitempty"`
	ByChannel        map[string]int `json:"by_channel"`
	Links            []string       `json:"links"`
	TxHashes         []string       `json:"tx_hashes"`
	ReviewerPings    int            `json:"reviewer_pings"`
	RepliesAfterPing int            `json:"replies_after_ping"`
	SecretLeak       bool           `json:"secret_leak"`
	SecretLeakKinds  []string       `json:"secret_leak_kinds"`
	// Valoper is the bot's deterministic verdict on the valoper criterion (found
	// / not_found / needs_human_check) from fetching the submitted link; the
	// skill copies ValoperState rather than judging valoper itself.
	ValoperState  string `json:"valoper_state,omitempty"`
	ValoperDetail string `json:"valoper_detail,omitempty"`
}

type EvidenceMessage struct {
	Channel   string `json:"channel"`
	Timestamp string `json:"ts"`
	Permalink string `json:"permalink"`
	Text      string `json:"text"`
}

type ReviewerMessage struct {
	Author    string `json:"author"`
	Timestamp string `json:"ts"`
	Permalink string `json:"permalink"`
	Text      string `json:"text"`
}

// --- digest.json (skill -> bot) ---

type DigestFile struct {
	GeneratedAt string            `json:"generated_at"`
	Candidates  []DigestCandidate `json:"candidates"`
}

type DigestCandidate struct {
	Row            int               `json:"row"`
	Candidate      string            `json:"candidate"`
	Readiness      string            `json:"readiness"`
	ReadinessScore string            `json:"readiness_score"`
	Summary        string            `json:"summary"`
	Criteria       map[string]string `json:"criteria"`
	EvidenceLinks  []string          `json:"evidence_links"`
}

// Build attributes messages to candidates, computes signals, redacts secrets,
// and assembles the harvest file. Candidate order follows records. It makes a
// single pass over messages, grouping each candidate's own messages so signals
// are computed from that group rather than by re-scanning every message.
func Build(guildID string, generatedAt time.Time, records []CandidateRecord, messages []Message) HarvestFile {
	out := make([]Candidate, len(records))
	norm := make([]string, len(records)) // normalized handle, computed once
	grouped := make([][]Message, len(records))
	for i, r := range records {
		out[i] = Candidate{
			Row:            r.Row,
			Candidate:      r.Candidate,
			Discord:        r.Discord,
			ResolvedUserID: r.ResolvedUserID,
			Submitted: Submitted{
				MonikerAddress: r.Moniker,
				ValoperLink:    r.Valoper,
				Introduction:   r.Introduction,
			},
			Signals: Signals{
				ByChannel:     map[string]int{},
				ValoperState:  r.ValoperState,
				ValoperDetail: r.ValoperDetail,
			},
			Messages:    []EvidenceMessage{},
			ReviewerCtx: []ReviewerMessage{},
		}
		norm[i] = normalizeUsername(r.Discord)
	}

	for _, m := range messages {
		if idx := authorIndex(records, norm, m); idx >= 0 {
			addEvidence(&out[idx], m)
			grouped[idx] = append(grouped[idx], m)
			continue
		}
		for idx := range records {
			if mentions(records[idx], norm[idx], m.Content) {
				addReviewerContext(&out[idx], m)
			}
		}
	}

	for i := range out {
		finalizeSignals(&out[i], grouped[i])
	}

	return HarvestFile{
		GeneratedAt: generatedAt.UTC().Format(time.RFC3339),
		GuildID:     guildID,
		Criteria:    Criteria,
		Candidates:  out,
	}
}

func addEvidence(c *Candidate, m Message) {
	clean, kinds := Redact(m.Content)
	c.Messages = append(c.Messages, EvidenceMessage{
		Channel:   m.ChannelKey,
		Timestamp: m.Timestamp.UTC().Format(time.RFC3339),
		Permalink: m.Permalink,
		Text:      clean,
	})
	if len(kinds) > 0 {
		c.Signals.SecretLeak = true
		c.Signals.SecretLeakKinds = mergeSorted(c.Signals.SecretLeakKinds, kinds)
	}
}

func addReviewerContext(c *Candidate, m Message) {
	clean, _ := Redact(m.Content)
	c.ReviewerCtx = append(c.ReviewerCtx, ReviewerMessage{
		Author:    m.AuthorUsername,
		Timestamp: m.Timestamp.UTC().Format(time.RFC3339),
		Permalink: m.Permalink,
		Text:      clean,
	})
}

// finalizeSignals computes a candidate's signals from their own messages.
func finalizeSignals(c *Candidate, msgs []Message) {
	var times []time.Time
	days := map[string]bool{}
	var corpus strings.Builder

	for _, m := range msgs {
		c.Signals.ByChannel[m.ChannelKey]++
		times = append(times, m.Timestamp)
		days[m.Timestamp.UTC().Format("2006-01-02")] = true
		corpus.WriteString(m.Content)
		corpus.WriteByte('\n')
	}

	c.Signals.MessageCount = len(times)
	c.Signals.ActiveDays = len(days)
	if len(times) > 0 {
		sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
		c.Signals.FirstActivity = times[0].UTC().Format(time.RFC3339)
		c.Signals.LastActivity = times[len(times)-1].UTC().Format(time.RFC3339)
	}

	text := corpus.String()
	c.Signals.Links = extractUnique(reURL, text, maxLinks)
	c.Signals.TxHashes = extractUnique(reTx, text, maxTx)

	c.Signals.ReviewerPings = len(c.ReviewerCtx)
	c.Signals.RepliesAfterPing = repliesAfterPing(c.ReviewerCtx, times)
}

// repliesAfterPing counts reviewer pings followed by at least one later
// candidate message, a rough responsiveness proxy.
func repliesAfterPing(pings []ReviewerMessage, candidateTimes []time.Time) int {
	if len(candidateTimes) == 0 {
		return 0
	}
	last := candidateTimes[len(candidateTimes)-1]
	answered := 0
	for _, p := range pings {
		t, err := time.Parse(time.RFC3339, p.Timestamp)
		if err != nil {
			continue
		}
		if last.After(t) {
			answered++
		}
	}
	return answered
}

// authorIndex returns the index of the candidate who authored m, or -1. norm[i]
// is the precomputed normalized handle for records[i].
func authorIndex(records []CandidateRecord, norm []string, m Message) int {
	for i, r := range records {
		if authorMatches(r, norm[i], m) {
			return i
		}
	}
	return -1
}

func authorMatches(r CandidateRecord, norm string, m Message) bool {
	if r.ResolvedUserID != "" && r.ResolvedUserID == m.AuthorID {
		return true
	}
	return norm != "" && norm == normalizeUsername(m.AuthorUsername)
}

func mentions(r CandidateRecord, norm, content string) bool {
	if r.ResolvedUserID != "" {
		if strings.Contains(content, "<@"+r.ResolvedUserID+">") || strings.Contains(content, "<@!"+r.ResolvedUserID+">") {
			return true
		}
	}
	// Match "@handle" as a whole token anywhere in the message. Scanning the raw
	// content (not normalizeUsername(content), which strips a leading "@" and
	// truncates at the first "#") keeps a leading mention or one before a channel
	// ref / hashtag.
	return norm != "" && containsHandle(strings.ToLower(content), norm)
}

// containsHandle reports whether content mentions "@handle" as a complete
// username token, so "@alice" does not match "@alice2" or "@alice_dev". content
// and handle must already be lowercased. The run of username characters after an
// "@" must equal handle once trailing dots (illegal at the end of a Discord
// name, so really sentence punctuation) are dropped.
func containsHandle(content, handle string) bool {
	if handle == "" {
		return false
	}
	for i := 0; i < len(content); i++ {
		if content[i] != '@' {
			continue
		}
		j := i + 1
		for j < len(content) && isUsernameChar(content[j]) {
			j++
		}
		if strings.TrimRight(content[i+1:j], ".") == handle {
			return true
		}
		i = j - 1 // resume scanning just past this token
	}
	return false
}

// isUsernameChar reports whether b is legal inside a Discord username
// (lowercase letter, digit, underscore, or period).
func isUsernameChar(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '_' || b == '.'
}

// NormalizeHandle normalizes a Discord handle for comparison (lowercased,
// trimmed, leading @ and any legacy #discriminator removed).
func NormalizeHandle(s string) string { return normalizeUsername(s) }

func normalizeUsername(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "@")
	if i := strings.IndexByte(s, '#'); i >= 0 { // legacy discriminator
		s = s[:i]
	}
	return s
}

func extractUnique(re *regexp.Regexp, text string, limit int) []string {
	seen := map[string]bool{}
	out := []string{} // non-nil so empty serializes as [] not null, like ByChannel
	for _, m := range re.FindAllString(text, -1) {
		m = strings.TrimRight(m, ".,);]")
		if seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
		if len(out) >= limit {
			break
		}
	}
	sort.Strings(out)
	return out
}

func mergeSorted(a, b []string) []string {
	seen := map[string]bool{}
	for _, x := range a {
		seen[x] = true
	}
	for _, x := range b {
		seen[x] = true
	}
	out := make([]string, 0, len(seen))
	for x := range seen {
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}

// --- digest parsing and cell formatting ---

// ParseDigest unmarshals and validates a digest.json produced by the skill.
func ParseDigest(data []byte) (DigestFile, error) {
	var d DigestFile
	if err := json.Unmarshal(data, &d); err != nil {
		return DigestFile{}, fmt.Errorf("parse digest: %w", err)
	}
	if len(d.Candidates) == 0 {
		return DigestFile{}, fmt.Errorf("digest has no candidates")
	}
	validReadiness := map[string]bool{
		ReadinessHigh: true, ReadinessMedium: true, ReadinessLow: true, ReadinessNeutral: true,
	}
	validCriterion := map[string]bool{}
	for _, k := range Criteria {
		validCriterion[k] = true
	}
	validState := map[string]bool{StateFound: true, StateNotFound: true, StateNeedsCheck: true}
	seenRow := map[int]bool{}
	for i, c := range d.Candidates {
		if c.Row <= 0 {
			return DigestFile{}, fmt.Errorf("candidate %d (%q): invalid row %d", i, c.Candidate, c.Row)
		}
		if seenRow[c.Row] {
			return DigestFile{}, fmt.Errorf("candidate row %d appears more than once", c.Row)
		}
		seenRow[c.Row] = true
		if c.Readiness != "" && !validReadiness[c.Readiness] {
			return DigestFile{}, fmt.Errorf("candidate row %d: invalid readiness %q", c.Row, c.Readiness)
		}
		for k, state := range c.Criteria {
			if !validCriterion[k] {
				return DigestFile{}, fmt.Errorf("candidate row %d: unknown criterion %q", c.Row, k)
			}
			if !validState[state] {
				return DigestFile{}, fmt.Errorf("candidate row %d: criterion %q has invalid state %q", c.Row, k, state)
			}
		}
	}
	return d, nil
}

// CriteriaBools returns the seven criterion results in Criteria order, true when
// the state is "found" (a ticked checkbox). Missing or non-found states are false.
func (c DigestCandidate) CriteriaBools() []bool {
	out := make([]bool, len(Criteria))
	for i, k := range Criteria {
		out[i] = c.Criteria[k] == StateFound
	}
	return out
}

// ReadinessCell renders the Readiness column value, e.g. "High (6/7)".
func (c DigestCandidate) ReadinessCell() string {
	if c.Readiness == "" {
		return ""
	}
	if c.ReadinessScore == "" {
		return c.Readiness
	}
	return fmt.Sprintf("%s (%s)", c.Readiness, c.ReadinessScore)
}

// EngagementCell renders the deterministic Engagement column, e.g.
// "12 msgs, 5 days, last 2026-06-10". Empty when the candidate never posted.
func (s Signals) EngagementCell() string {
	if s.MessageCount == 0 {
		return ""
	}
	cell := fmt.Sprintf("%d msgs, %d days", s.MessageCount, s.ActiveDays)
	if s.LastActivity != "" {
		if t, err := time.Parse(time.RFC3339, s.LastActivity); err == nil {
			cell += ", last " + t.UTC().Format("2006-01-02")
		}
	}
	return cell
}

// RedFlagsCell renders the deterministic Red flags column from the secret-leak
// scan. Empty when nothing was detected.
func (s Signals) RedFlagsCell() string {
	if !s.SecretLeak {
		return ""
	}
	return "Secret leak: " + strings.Join(s.SecretLeakKinds, ", ")
}
