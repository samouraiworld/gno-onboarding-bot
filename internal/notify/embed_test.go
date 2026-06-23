package notify

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
)

func TestBuildAndParseSubmissionEmbed(t *testing.T) {
	embed := BuildSubmissionEmbed(58, "123456789012345678", "alice", "g1abc", "https://example.com/valoper/alice", "Hi, I'm alice", "3 sentries, self-hosted", "hot standby region")
	msg := &discordgo.Message{Embeds: []*discordgo.MessageEmbed{embed}}

	row, candidateID, valoperLink, err := ParseSubmissionEmbed(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if row != 58 {
		t.Errorf("row = %d, want 58", row)
	}
	if candidateID != "123456789012345678" {
		t.Errorf("candidateID = %q, want %q", candidateID, "123456789012345678")
	}
	if valoperLink != "https://example.com/valoper/alice" {
		t.Errorf("valoperLink = %q, want %q", valoperLink, "https://example.com/valoper/alice")
	}
}

func TestBuildSubmissionEmbed_TruncatesLongFields(t *testing.T) {
	long := strings.Repeat("a", 2000)
	embed := BuildSubmissionEmbed(1, "123", "m", "g1abc", "https://x", long, long, long)
	got := map[string]string{}
	for _, f := range embed.Fields {
		got[f.Name] = f.Value
	}
	for _, name := range []string{FieldIntroduction, FieldArchitecture, FieldBackupPlan} {
		v := got[name]
		if n := utf8.RuneCountInString(v); n > 1024 {
			t.Errorf("%s = %d runes, want <= 1024", name, n)
		}
		if !strings.HasSuffix(v, "…") {
			t.Errorf("expected ellipsis suffix on truncated %s", name)
		}
	}
}

func TestBuildSubmissionEmbed_IncludesArchitectureAndBackup(t *testing.T) {
	embed := BuildSubmissionEmbed(1, "123", "m", "g1abc", "https://x", "intro", "3 sentries, self-hosted", "hot standby region")
	got := map[string]string{}
	for _, f := range embed.Fields {
		got[f.Name] = f.Value
	}
	if got[FieldArchitecture] != "3 sentries, self-hosted" {
		t.Errorf("architecture field = %q, want %q", got[FieldArchitecture], "3 sentries, self-hosted")
	}
	if got[FieldBackupPlan] != "hot standby region" {
		t.Errorf("backup plan field = %q, want %q", got[FieldBackupPlan], "hot standby region")
	}
}

func TestParseSubmissionEmbed_NoEmbeds(t *testing.T) {
	_, _, _, err := ParseSubmissionEmbed(&discordgo.Message{})
	if err == nil {
		t.Fatal("expected error for message with no embeds")
	}
}

func TestParseSubmissionEmbed_BadFooter(t *testing.T) {
	msg := &discordgo.Message{Embeds: []*discordgo.MessageEmbed{{
		Footer: &discordgo.MessageEmbedFooter{Text: "not-a-valid-ref"},
	}}}
	_, _, _, err := ParseSubmissionEmbed(msg)
	if err == nil {
		t.Fatal("expected error for malformed footer")
	}
}
