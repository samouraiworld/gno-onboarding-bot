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

// channelPoster is the subset of *discordgo.Session that sendCandidateMessage
// needs. The activation poller's Discord interface satisfies it too, so both the
// command handlers and the poller share this one candidate-post implementation
// (and its mention scoping) instead of each rolling their own.
type channelPoster interface {
	ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, options ...discordgo.RequestOption) (*discordgo.Message, error)
}

// sendCandidateMessage posts a candidate-facing message to channelID and pings
// only the candidate: AllowedMentions is scoped to candidateID, so any stray
// mention token in the content cannot fan out to @everyone or a role. It
// replaces the old DM delivery, which was unreliable for non-mutual or
// DM-disabled users and could get the bot flagged.
//
// Only bot-initiated notices use this. Approval (review_approve) and the
// activation notice (the poller) go to the onboarding channel; decline
// (review_decline) goes to general chat, because it removes the candidate role
// and a roleless candidate keeps access to general but not onboarding.
// Candidate-run commands (welcome on /candidate-testnet, acknowledgement on
// /submit-request) reply ephemerally instead and do not use this helper.
func sendCandidateMessage(poster channelPoster, channelID, candidateID, content string) error {
	_, err := poster.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content:         "<@" + candidateID + ">\n" + content,
		AllowedMentions: &discordgo.MessageAllowedMentions{Users: []string{candidateID}},
	})
	return err
}
