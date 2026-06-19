package handlers

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestHasRole(t *testing.T) {
	member := &discordgo.Member{Roles: []string{"1", "2", "3"}}
	if !hasRole(member, "2") {
		t.Error("expected hasRole to find role 2")
	}
	if hasRole(member, "9") {
		t.Error("expected hasRole to not find role 9")
	}
}

func TestModalValue(t *testing.T) {
	data := discordgo.ModalSubmitInteractionData{
		Components: []discordgo.MessageComponent{
			&discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				&discordgo.TextInput{CustomID: "topic", Value: "sync issue"},
			}},
			&discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				&discordgo.TextInput{CustomID: "scope", Value: "logs review"},
			}},
		},
	}
	if got := modalValue(data, "topic"); got != "sync issue" {
		t.Errorf("modalValue(topic) = %q, want %q", got, "sync issue")
	}
	if got := modalValue(data, "scope"); got != "logs review" {
		t.Errorf("modalValue(scope) = %q, want %q", got, "logs review")
	}
	if got := modalValue(data, "missing"); got != "" {
		t.Errorf("modalValue(missing) = %q, want empty", got)
	}
}
