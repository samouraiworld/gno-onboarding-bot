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

const actionMissingInfo = "missing_info"

func RegisterRequestMissingInfo(s *discordgo.Session, cfg *config.Config, api sheet.API, tpl *templates.Templates) error {
	cmd := &discordgo.ApplicationCommand{
		Name: "Request missing info",
		Type: discordgo.MessageApplicationCommand,
	}
	if _, err := s.ApplicationCommandCreate(s.State.User.ID, cfg.GuildID, cmd); err != nil {
		return fmt.Errorf("create Request missing info command: %w", err)
	}

	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			if i.ApplicationCommandData().Name != "Request missing info" {
				return
			}
			showRequestMissingInfoModal(s, i)
		case discordgo.InteractionModalSubmit:
			action, row, candidateID, err := rowref.DecodeCustomID(i.ModalSubmitData().CustomID)
			if err != nil || action != actionMissingInfo {
				return
			}
			finalizeRequestMissingInfo(s, i, cfg, api, tpl, row, candidateID)
		}
	})
	return nil
}

func showRequestMissingInfoModal(s *discordgo.Session, i *discordgo.InteractionCreate) {
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
	err = showModal(s, i.Interaction, rowref.CustomID(actionMissingInfo, row, candidateID), "Request missing info", []*discordgo.TextInput{
		{CustomID: "items", Label: "Missing/invalid items, one per line", Style: discordgo.TextInputParagraph, Required: true},
	})
	if err != nil {
		respondError(s, i.Interaction, "Could not open the form. Please try again.")
	}
}

func finalizeRequestMissingInfo(s *discordgo.Session, i *discordgo.InteractionCreate, cfg *config.Config, api sheet.API, tpl *templates.Templates, row int, candidateID string) {
	if err := deferEphemeral(s, i.Interaction); err != nil {
		return
	}

	itemsRaw := modalValue(i.ModalSubmitData(), "items")
	items := forms.SplitLines(itemsRaw)
	if len(items) == 0 {
		editEphemeral(s, i.Interaction, "Missing/invalid items cannot be empty.")
		return
	}

	message, err := tpl.RequestMissingInfo(items)
	if err != nil {
		log.Printf("request-missing-info: render template: %v", err)
		editEphemeral(s, i.Interaction, "Could not render the message template. Please contact a team member.")
		return
	}
	if err := sheet.UpdateFields(context.Background(), api, cfg.SheetID, cfg.SheetName, row, map[sheet.Column]string{
		sheet.ColumnStatus:          sheet.StatusNeedsRetry,
		sheet.ColumnMissingCriteria: strings.Join(items, "; "),
		sheet.ColumnDecisionDate:    today(),
		sheet.ColumnReviewers:       i.Member.User.Username,
	}); err != nil {
		log.Printf("request-missing-info: update tracker for row %d: %v", row, err)
		editEphemeral(s, i.Interaction, "Could not update the tracker. Please try again.")
		return
	}

	if err := sendDM(s, candidateID, message); err != nil {
		editEphemeral(s, i.Interaction, fmt.Sprintf("Saved, but could not DM the candidate (DMs may be closed). Please relay this manually:\n\n%s", message))
		return
	}
	editEphemeral(s, i.Interaction, "Sent.")
}
