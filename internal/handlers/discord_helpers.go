package handlers

import (
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
)

func today() string {
	return time.Now().Format("2006-01-02")
}

func hasRole(member *discordgo.Member, roleID string) bool {
	for _, r := range member.Roles {
		if r == roleID {
			return true
		}
	}
	return false
}

func modalValue(data discordgo.ModalSubmitInteractionData, customID string) string {
	for _, comp := range data.Components {
		row, ok := comp.(*discordgo.ActionsRow)
		if !ok {
			continue
		}
		for _, inner := range row.Components {
			if ti, ok := inner.(*discordgo.TextInput); ok && ti.CustomID == customID {
				return ti.Value
			}
		}
	}
	return ""
}

func showModal(s *discordgo.Session, i *discordgo.Interaction, customID, title string, inputs []*discordgo.TextInput) error {
	rows := make([]discordgo.MessageComponent, len(inputs))
	for idx, input := range inputs {
		rows[idx] = &discordgo.ActionsRow{Components: []discordgo.MessageComponent{input}}
	}
	return s.InteractionRespond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{CustomID: customID, Title: title, Components: rows},
	})
}

func deferEphemeral(s *discordgo.Session, i *discordgo.Interaction) error {
	return s.InteractionRespond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})
}

func editEphemeral(s *discordgo.Session, i *discordgo.Interaction, content string) {
	if _, err := s.InteractionResponseEdit(i, &discordgo.WebhookEdit{Content: &content}); err != nil {
		log.Printf("editEphemeral: failed to edit interaction response for interaction %s (this is the candidate/reviewer-facing fallback message; it was NOT delivered): %v", i.ID, err)
	}
}

// editEphemeralNoMentions is editEphemeral for content that interpolates
// untrusted text (e.g. member-controlled display names). A zero-value
// MessageAllowedMentions disables all mention parsing, so a nickname like
// "@everyone" or "<@&role>" cannot ping anyone.
func editEphemeralNoMentions(s *discordgo.Session, i *discordgo.Interaction, content string) {
	if _, err := s.InteractionResponseEdit(i, &discordgo.WebhookEdit{
		Content:         &content,
		AllowedMentions: &discordgo.MessageAllowedMentions{},
	}); err != nil {
		log.Printf("editEphemeralNoMentions: failed to edit interaction response for interaction %s (it was NOT delivered): %v", i.ID, err)
	}
}

func respondError(s *discordgo.Session, i *discordgo.Interaction, content string) {
	if err := s.InteractionRespond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content, Flags: discordgo.MessageFlagsEphemeral},
	}); err != nil {
		log.Printf("respondError: failed to respond to interaction %s with error message (message was NOT delivered): %v", i.ID, err)
	}
}

func sendDM(s *discordgo.Session, userID, content string) error {
	ch, err := s.UserChannelCreate(userID)
	if err != nil {
		return err
	}
	_, err = s.ChannelMessageSend(ch.ID, content)
	return err
}
