package handlers

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestOptionValue(t *testing.T) {
	options := []*discordgo.ApplicationCommandInteractionDataOption{
		{Name: "announcement-link", Type: discordgo.ApplicationCommandOptionString, Value: "https://example.com/reset"},
		{Name: "other", Type: discordgo.ApplicationCommandOptionString, Value: "ignored"},
	}
	if got := optionValue(options, "announcement-link"); got != "https://example.com/reset" {
		t.Errorf("optionValue(announcement-link) = %q, want %q", got, "https://example.com/reset")
	}
	if got := optionValue(options, "missing"); got != "" {
		t.Errorf("optionValue(missing) = %q, want empty", got)
	}
	if got := optionValue(nil, "announcement-link"); got != "" {
		t.Errorf("optionValue(nil) = %q, want empty", got)
	}
}
