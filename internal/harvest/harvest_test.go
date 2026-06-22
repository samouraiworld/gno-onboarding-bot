package harvest

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func ts(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestBuild(t *testing.T) {
	records := []CandidateRecord{
		{Row: 2, Candidate: "alice", Discord: "@alice", ResolvedUserID: "111", Valoper: "https://v/g1alice"},
		{Row: 3, Candidate: "bob", Discord: "@bob", ResolvedUserID: "222"},
	}
	pem := "-----BEGIN PRIVATE KEY-----\nSECRETBODY\n-----END PRIVATE KEY-----"
	messages := []Message{
		{ChannelKey: ChannelOnboarding, AuthorID: "111", AuthorUsername: "alice",
			Content: "node synced https://gnoweb.test-13.gnoland.network/r/gnops/valopers:g1abc", Timestamp: ts("2026-06-05T09:00:00Z"), Permalink: "p1"},
		{ChannelKey: ChannelGeneral, AuthorID: "111", AuthorUsername: "alice",
			Content: "hello team", Timestamp: ts("2026-06-06T10:00:00Z"), Permalink: "p2"},
		{ChannelKey: ChannelReview, AuthorID: "999", AuthorUsername: "carol",
			Content: "<@111> please post your tx hash", Timestamp: ts("2026-06-06T11:00:00Z"), Permalink: "p3"},
		{ChannelKey: ChannelOnboarding, AuthorID: "111", AuthorUsername: "alice",
			Content: "tx A1B2C3D4E5F6071829304152637485960718293041526374859607182930AABB", Timestamp: ts("2026-06-06T12:00:00Z"), Permalink: "p4"},
		{ChannelKey: ChannelOnboarding, AuthorID: "111", AuthorUsername: "alice",
			Content: "oops my key " + pem, Timestamp: ts("2026-06-07T09:00:00Z"), Permalink: "p5"},
		{ChannelKey: ChannelReview, AuthorID: "999", AuthorUsername: "carol",
			Content: "unrelated note about nobody", Timestamp: ts("2026-06-07T10:00:00Z"), Permalink: "p6"},
	}

	hf := Build("guild1", ts("2026-06-19T00:00:00Z"), records, messages)

	if hf.GuildID != "guild1" || hf.GeneratedAt != "2026-06-19T00:00:00Z" {
		t.Errorf("header wrong: %+v", hf)
	}
	if !reflect.DeepEqual(hf.Criteria, Criteria) {
		t.Errorf("criteria = %v", hf.Criteria)
	}
	if len(hf.Candidates) != 2 {
		t.Fatalf("got %d candidates, want 2", len(hf.Candidates))
	}

	alice := hf.Candidates[0]
	if alice.Signals.MessageCount != 4 {
		t.Errorf("alice message_count = %d, want 4", alice.Signals.MessageCount)
	}
	if alice.Signals.ByChannel[ChannelOnboarding] != 3 || alice.Signals.ByChannel[ChannelGeneral] != 1 {
		t.Errorf("alice by_channel = %v", alice.Signals.ByChannel)
	}
	if alice.Signals.ActiveDays != 3 {
		t.Errorf("alice active_days = %d, want 3", alice.Signals.ActiveDays)
	}
	if alice.Signals.FirstActivity != "2026-06-05T09:00:00Z" || alice.Signals.LastActivity != "2026-06-07T09:00:00Z" {
		t.Errorf("alice activity span = %s..%s", alice.Signals.FirstActivity, alice.Signals.LastActivity)
	}
	if alice.Signals.ReviewerPings != 1 || alice.Signals.RepliesAfterPing != 1 {
		t.Errorf("alice pings=%d replies=%d, want 1/1", alice.Signals.ReviewerPings, alice.Signals.RepliesAfterPing)
	}
	if len(alice.Signals.Links) != 1 || !strings.Contains(alice.Signals.Links[0], "valopers:g1abc") {
		t.Errorf("alice links = %v", alice.Signals.Links)
	}
	if len(alice.Signals.TxHashes) != 1 {
		t.Errorf("alice tx_hashes = %v, want 1", alice.Signals.TxHashes)
	}
	if !alice.Signals.SecretLeak || !reflect.DeepEqual(alice.Signals.SecretLeakKinds, []string{"private_key"}) {
		t.Errorf("alice leak = %v / %v", alice.Signals.SecretLeak, alice.Signals.SecretLeakKinds)
	}
	if len(alice.ReviewerCtx) != 1 || alice.ReviewerCtx[0].Author != "carol" {
		t.Errorf("alice reviewer_ctx = %+v", alice.ReviewerCtx)
	}
	// the leaked key body must never be stored
	for _, m := range alice.Messages {
		if strings.Contains(m.Text, "SECRETBODY") {
			t.Errorf("secret survived into stored evidence: %q", m.Text)
		}
	}
	if !redactedSomewhere(alice.Messages, "[REDACTED:private_key]") {
		t.Errorf("expected a redaction marker in alice's evidence")
	}

	bob := hf.Candidates[1]
	if bob.Signals.MessageCount != 0 || bob.Signals.SecretLeak {
		t.Errorf("bob should be neutral: %+v", bob.Signals)
	}
	if bob.Signals.EngagementCell() != "" || bob.Signals.RedFlagsCell() != "" {
		t.Errorf("bob neutral cells should be empty: eng=%q red=%q", bob.Signals.EngagementCell(), bob.Signals.RedFlagsCell())
	}
}

func redactedSomewhere(msgs []EvidenceMessage, marker string) bool {
	for _, m := range msgs {
		if strings.Contains(m.Text, marker) {
			return true
		}
	}
	return false
}

func TestBuild_MatchByUsernameWhenNoID(t *testing.T) {
	records := []CandidateRecord{{Row: 5, Candidate: "dave", Discord: "@Dave"}}
	messages := []Message{
		{ChannelKey: ChannelOnboarding, AuthorID: "777", AuthorUsername: "dave", Content: "hi", Timestamp: ts("2026-06-05T09:00:00Z")},
	}
	hf := Build("g", ts("2026-06-19T00:00:00Z"), records, messages)
	if hf.Candidates[0].Signals.MessageCount != 1 {
		t.Errorf("expected username match, got %d msgs", hf.Candidates[0].Signals.MessageCount)
	}
}

func TestBuild_MentionByUsername(t *testing.T) {
	// alice has no resolved ID, so reviewer mentions must match by @username.
	records := []CandidateRecord{{Row: 2, Candidate: "alice", Discord: "@alice"}}
	messages := []Message{
		// Leading @mention AND a later "#": the old normalizeUsername(content) path
		// dropped both (stripped the leading @, truncated at #).
		{ChannelKey: ChannelReview, AuthorID: "999", AuthorUsername: "carol",
			Content: "@alice please retry, see #testnet-onboarding", Timestamp: ts("2026-06-06T11:00:00Z"), Permalink: "p"},
	}
	hf := Build("g", ts("2026-06-19T00:00:00Z"), records, messages)
	if len(hf.Candidates[0].ReviewerCtx) != 1 {
		t.Errorf("leading @mention with a later # was not attributed: %+v", hf.Candidates[0].ReviewerCtx)
	}
}

func TestBuild_MentionDoesNotPrefixMatch(t *testing.T) {
	// "@alice2"/"@alice_dev" are other people; they must not attribute to "alice".
	// Trailing punctuation ("@alice!") must still match.
	records := []CandidateRecord{{Row: 2, Candidate: "alice", Discord: "@alice"}}
	messages := []Message{
		{ChannelKey: ChannelReview, AuthorID: "999", AuthorUsername: "carol",
			Content: "ping @alice2 and @alice_dev, not @alice.bob either", Timestamp: ts("2026-06-06T11:00:00Z"), Permalink: "p1"},
		{ChannelKey: ChannelReview, AuthorID: "999", AuthorUsername: "carol",
			Content: "thanks @alice!", Timestamp: ts("2026-06-06T12:00:00Z"), Permalink: "p2"},
	}
	hf := Build("g", ts("2026-06-19T00:00:00Z"), records, messages)
	got := hf.Candidates[0].ReviewerCtx
	if len(got) != 1 {
		t.Fatalf("want only the real mention attributed, got %d: %+v", len(got), got)
	}
	if got[0].Permalink != "p2" {
		t.Errorf("matched the wrong message: %+v", got[0])
	}
}

func TestBuild_ValoperStatePropagates(t *testing.T) {
	// The handler computes the valoper verdict from column K and sets it on the
	// record; Build must copy it into Signals (the skill reads it from there).
	records := []CandidateRecord{
		{Row: 2, Candidate: "alice", Discord: "@alice", ValoperState: StateFound, ValoperDetail: "operator address recorded"},
	}
	hf := Build("g", ts("2026-06-19T00:00:00Z"), records, nil)
	got := hf.Candidates[0].Signals
	if got.ValoperState != StateFound || got.ValoperDetail != "operator address recorded" {
		t.Errorf("valoper not propagated into signals: state=%q detail=%q", got.ValoperState, got.ValoperDetail)
	}
}

func TestCriteriaBools(t *testing.T) {
	// Partial map: only setup (index 0) and valoper (index 3) are found; tx is
	// explicitly not_found; the rest are absent. Pins both the Criteria order and
	// the "missing/non-found => false" rule that drives checkboxes P-V.
	c := DigestCandidate{Criteria: map[string]string{
		"setup":   StateFound,
		"valoper": StateFound,
		"tx":      StateNotFound,
		"ops":     StateNeedsCheck,
	}}
	got := c.CriteriaBools()
	want := []bool{true, false, false, true, false, false, false} // setup,sync,tx,valoper,ops,comms,safety
	if len(got) != len(Criteria) {
		t.Fatalf("got %d bools, want %d", len(got), len(Criteria))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("CriteriaBools[%d] (%s) = %v, want %v", i, Criteria[i], got[i], want[i])
		}
	}
}

func TestParseDigest_RejectsDuplicateRows(t *testing.T) {
	dup := `{"candidates":[{"row":2,"readiness":"High"},{"row":2,"readiness":"Low"}]}`
	if _, err := ParseDigest([]byte(dup)); err == nil {
		t.Error("expected error for a digest with two candidates on the same row")
	}
}

func TestCells(t *testing.T) {
	s := Signals{MessageCount: 12, ActiveDays: 5, LastActivity: "2026-06-10T18:00:00Z", SecretLeak: true, SecretLeakKinds: []string{"private_ip", "seed_phrase"}}
	if got := s.EngagementCell(); got != "12 msgs, 5 days, last 2026-06-10" {
		t.Errorf("EngagementCell = %q", got)
	}
	if got := s.RedFlagsCell(); got != "Secret leak: private_ip, seed_phrase" {
		t.Errorf("RedFlagsCell = %q", got)
	}

	d := DigestCandidate{Readiness: "High", ReadinessScore: "6/7"}
	if got := d.ReadinessCell(); got != "High (6/7)" {
		t.Errorf("ReadinessCell = %q", got)
	}
	if got := (DigestCandidate{Readiness: "Neutral"}).ReadinessCell(); got != "Neutral" {
		t.Errorf("ReadinessCell no-score = %q", got)
	}
}

func TestEvidenceLinkUnmarshal(t *testing.T) {
	in := `{"candidates":[{"row":2,"readiness":"High","evidence_links":[
		{"title":"Submission","url":"https://a"},
		"https://b"
	]}]}`
	var got DigestFile
	if err := json.Unmarshal([]byte(in), &got); err != nil {
		t.Fatal(err)
	}
	links := got.Candidates[0].EvidenceLinks
	if len(links) != 2 {
		t.Fatalf("got %d links, want 2", len(links))
	}
	if links[0] != (EvidenceLink{Title: "Submission", URL: "https://a"}) {
		t.Errorf("object form = %#v", links[0])
	}
	// Legacy bare-string form loads with no title; Label falls back to the URL.
	if links[1] != (EvidenceLink{URL: "https://b"}) || links[1].Label() != "https://b" {
		t.Errorf("string form = %#v, label %q", links[1], links[1].Label())
	}
}

func TestParseDigest(t *testing.T) {
	good := DigestFile{Candidates: []DigestCandidate{
		{Row: 2, Candidate: "alice", Readiness: "High", ReadinessScore: "6/7", Summary: "ok",
			Criteria: map[string]string{"setup": StateFound, "tx": StateNotFound}},
	}}
	data, _ := json.Marshal(good)
	if _, err := ParseDigest(data); err != nil {
		t.Fatalf("valid digest rejected: %v", err)
	}

	bad := []string{
		`{"candidates":[]}`,
		`{"candidates":[{"row":0,"readiness":"High"}]}`,
		`{"candidates":[{"row":2,"readiness":"Amazing"}]}`,
		`{"candidates":[{"row":2,"criteria":{"vibes":"found"}}]}`,
		`not json`,
	}
	for _, b := range bad {
		if _, err := ParseDigest([]byte(b)); err == nil {
			t.Errorf("expected error for %q", b)
		}
	}
}
