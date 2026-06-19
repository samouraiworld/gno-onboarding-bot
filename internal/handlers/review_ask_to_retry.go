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

const actionAskToRetry = "ask_to_retry"

func RegisterAskToRetry(s *discordgo.Session, cfg *config.Config, api sheet.API, tpl *templates.Templates) error {
	cmd := &discordgo.ApplicationCommand{
		Name: "Ask to retry",
		Type: discordgo.MessageApplicationCommand,
	}
	if _, err := s.ApplicationCommandCreate(s.State.User.ID, cfg.GuildID, cmd); err != nil {
		return fmt.Errorf("create Ask to retry command: %w", err)
	}

	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			if i.ApplicationCommandData().Name != "Ask to retry" {
				return
			}
			showAskToRetryModal(s, i)
		case discordgo.InteractionModalSubmit:
			action, row, candidateID, err := rowref.DecodeCustomID(i.ModalSubmitData().CustomID)
			if err != nil || action != actionAskToRetry {
				return
			}
			finalizeAskToRetry(s, i, cfg, api, tpl, row, candidateID)
		}
	})
	return nil
}

func showAskToRetryModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
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
	err = showModal(s, i.Interaction, rowref.CustomID(actionAskToRetry, row, candidateID), "Ask to retry", []*discordgo.TextInput{
		{CustomID: "criteria", Label: "Unmet criteria, 1 per line: Criterion: Issue", Style: discordgo.TextInputParagraph, Required: true},
		{CustomID: "actions", Label: "Actions required to retry", Style: discordgo.TextInputParagraph, Required: true},
	})
	if err != nil {
		respondError(s, i.Interaction, "Could not open the form. Please try again.")
	}
}

func finalizeAskToRetry(s *discordgo.Session, i *discordgo.InteractionCreate, cfg *config.Config, api sheet.API, tpl *templates.Templates, row int, candidateID string) {
	if err := deferEphemeral(s, i.Interaction); err != nil {
		return
	}

	data := i.ModalSubmitData()
	criteriaRaw := modalValue(data, "criteria")
	actionsRaw := modalValue(data, "actions")

	missing := forms.MissingRequired([]forms.Field{
		{Label: "Unmet criteria", Value: criteriaRaw},
		{Label: "Actions required to retry", Value: actionsRaw},
	})
	if len(missing) > 0 {
		editEphemeral(s, i.Interaction, fmt.Sprintf("Missing required field(s): %s", strings.Join(missing, ", ")))
		return
	}

	criteria := forms.SplitLines(criteriaRaw)
	message, err := tpl.AskToRetry(criteria, actionsRaw)
	if err != nil {
		log.Printf("ask-to-retry: render template: %v", err)
		editEphemeral(s, i.Interaction, "Could not render the message template. Please contact a team member.")
		return
	}
	if err := sheet.UpdateFields(context.Background(), api, cfg.SheetID, cfg.SheetName, row, map[sheet.Column]string{
		sheet.ColumnStatus:          sheet.StatusNeedsRetry,
		sheet.ColumnMissingCriteria: strings.Join(criteria, "; "),
		sheet.ColumnDecisionDate:    today(),
		sheet.ColumnReviewers:       i.Member.User.Username,
	}); err != nil {
		log.Printf("ask-to-retry: update tracker for row %d: %v", row, err)
		editEphemeral(s, i.Interaction, "Could not update the tracker. Please try again.")
		return
	}

	if err := sendDM(s, candidateID, message); err != nil {
		editEphemeral(s, i.Interaction, fmt.Sprintf("Saved, but could not DM the candidate (DMs may be closed). Please relay this manually:\n\n%s", message))
		return
	}
	editEphemeral(s, i.Interaction, "Sent.")
}
