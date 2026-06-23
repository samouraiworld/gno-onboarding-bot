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

func respondError(s *discordgo.Session, i *discordgo.Interaction, content string) {
	if err := s.InteractionRespond(i, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content, Flags: discordgo.MessageFlagsEphemeral},
	}); err != nil {
		log.Printf("respondError: failed to respond to interaction %s with error message (message was NOT delivered): %v", i.ID, err)
	}
}

// sendCandidateMessage posts a candidate-facing message to channelID and pings
// the candidate. It replaces the old DM delivery: DMs to non-mutual or
// DM-disabled users are unreliable and can get the bot flagged, and a channel
// post gives a persistent, re-readable record instead.
//
// Callers choose the channel by who can still see it. Welcome, acknowledgement,
// and approval go to the onboarding channel (readable by the Candidate and
// Validator roles). Decline goes to general chat: it removes the candidate
// role, and the now-roleless candidate keeps access to general but not to the
// onboarding channel.
func sendCandidateMessage(s *discordgo.Session, channelID, candidateID, content string) error {
	_, err := s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content:         "<@" + candidateID + ">\n" + content,
		AllowedMentions: &discordgo.MessageAllowedMentions{Users: []string{candidateID}},
	})
	return err
}
