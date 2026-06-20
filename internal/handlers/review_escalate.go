package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"

	"onboardingbot/internal/config"
	"onboardingbot/internal/forms"
	"onboardingbot/internal/notify"
	"onboardingbot/internal/rowref"
	"onboardingbot/internal/sheet"
	"onboardingbot/internal/templates"
)

const actionEscalate = "escalate_to_call"

func RegisterEscalateToCall(s *discordgo.Session, cfg *config.Config, api sheet.API, tpl *templates.Templates) error {
	cmd := &discordgo.ApplicationCommand{
		Name: "Escalate to call",
		Type: discordgo.MessageApplicationCommand,
	}
	if _, err := s.ApplicationCommandCreate(s.State.User.ID, cfg.GuildID, cmd); err != nil {
		return fmt.Errorf("create Escalate to call command: %w", err)
	}

	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			if i.ApplicationCommandData().Name != "Escalate to call" {
				return
			}
			showEscalateModal(s, i)
		case discordgo.InteractionModalSubmit:
			action, row, candidateID, err := rowref.DecodeCustomID(i.ModalSubmitData().CustomID)
			if err != nil || action != actionEscalate {
				return
			}
			finalizeEscalate(s, i, cfg, api, tpl, row, candidateID)
		}
	})
	return nil
}

func showEscalateModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	msg, ok := data.Resolved.Messages[data.TargetID]
	if !ok {
		respondError(s, i.Interaction, "Could not find the submission message. It may have been deleted.")
		return
	}
	row, candidateID, _, err := notify.ParseSubmissionEmbed(msg)
	if err != nil {
		respondError(s, i.Interaction, "This message is not a valid submission notification.")
		return
	}
	err = showModal(s, i.Interaction, rowref.CustomID(actionEscalate, row, candidateID), "Escalate to call", []*discordgo.TextInput{
		{CustomID: "topic", Label: "Topic to clarify", Style: discordgo.TextInputShort, Required: true},
		{CustomID: "slots", Label: "Proposed time slots, one per line", Style: discordgo.TextInputParagraph, Required: true},
		{CustomID: "scope", Label: "Call scope/focus", Style: discordgo.TextInputShort, Required: true},
	})
	if err != nil {
		respondError(s, i.Interaction, "Could not open the form. Please try again.")
	}
}

func finalizeEscalate(s *discordgo.Session, i *discordgo.InteractionCreate, cfg *config.Config, api sheet.API, tpl *templates.Templates, row int, candidateID string) {
	if err := deferEphemeral(s, i.Interaction); err != nil {
		return
	}

	data := i.ModalSubmitData()
	topic := modalValue(data, "topic")
	slotsRaw := modalValue(data, "slots")
	scope := modalValue(data, "scope")

	missing := forms.MissingRequired([]forms.Field{
		{Label: "Topic to clarify", Value: topic},
		{Label: "Proposed time slots", Value: slotsRaw},
		{Label: "Call scope/focus", Value: scope},
	})
	if len(missing) > 0 {
		editEphemeral(s, i.Interaction, fmt.Sprintf("Missing required field(s): %s", strings.Join(missing, ", ")))
		return
	}

	slots := forms.SplitLines(slotsRaw)
	message, err := tpl.EscalateToCall(topic, strings.Join(slots, ", "), scope)
	if err != nil {
		log.Printf("escalate-to-call: render template: %v", err)
		editEphemeral(s, i.Interaction, "Could not render the message template. Please contact a team member.")
		return
	}
	if err := sheet.UpdateFields(context.Background(), api, cfg.SheetID, cfg.SheetName, row, map[sheet.Column]string{
		sheet.ColumnReviewers: i.Member.User.Username,
	}); err != nil {
		log.Printf("escalate-to-call: update tracker for row %d: %v", row, err)
		editEphemeral(s, i.Interaction, "Could not update the tracker. Please try again.")
		return
	}

	if err := sendDM(s, candidateID, message); err != nil {
		editEphemeral(s, i.Interaction, fmt.Sprintf("Saved, but could not DM the candidate (DMs may be closed). Please relay this manually:\n\n%s", message))
		return
	}
	editEphemeral(s, i.Interaction, "Sent.")
}
