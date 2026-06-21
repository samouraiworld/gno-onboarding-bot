package notify

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
)

func TestBuildAndParseSubmissionEmbed(t *testing.T) {
	embed := BuildSubmissionEmbed(58, "123456789012345678", "alice", "g1abc", "https://example.com/valoper/alice", "Hi, I'm alice")
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

func TestBuildSubmissionEmbed_TruncatesIntroduction(t *testing.T) {
	long := strings.Repeat("a", 2000)
	embed := BuildSubmissionEmbed(1, "123", "m", "g1abc", "https://x", long)
	var intro string
	for _, f := range embed.Fields {
		if f.Name == FieldIntroduction {
			intro = f.Value
		}
	}
	if n := utf8.RuneCountInString(intro); n > 1024 {
		t.Errorf("introduction = %d runes, want <= 1024", n)
	}
	if !strings.HasSuffix(intro, "…") {
		t.Error("expected ellipsis suffix on truncated introduction")
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
