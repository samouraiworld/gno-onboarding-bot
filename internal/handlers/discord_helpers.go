package handlers

import (
	"time"

	"github.com/bwmarrin/discordgo"
)

func boolPtr(b bool) *bool {
	return &b
}

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
	s.InteractionResponseEdit(i, &discordgo.WebhookEdit{Content: &content})
}

func respondError(s *discordgo.Session, i *discordgo.Interaction, content string) {
	s.InteractionRespond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content, Flags: discordgo.MessageFlagsEphemeral},
	})
}

func sendDM(s *discordgo.Session, userID, content string) error {
	ch, err := s.UserChannelCreate(userID)
	if err != nil {
		return err
	}
	_, err = s.ChannelMessageSend(ch.ID, content)
	return err
}
