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

const actionDecline = "decline"

func RegisterDecline(s *discordgo.Session, cfg *config.Config, api sheet.API, tpl *templates.Templates) error {
	cmd := &discordgo.ApplicationCommand{
		Name: "Decline",
		Type: discordgo.MessageApplicationCommand,
	}
	if _, err := s.ApplicationCommandCreate(s.State.User.ID, cfg.GuildID, cmd); err != nil {
		return fmt.Errorf("create Decline command: %w", err)
	}

	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			if i.ApplicationCommandData().Name != "Decline" {
				return
			}
			showDeclineModal(s, i, cfg)
		case discordgo.InteractionModalSubmit:
			action, row, candidateID, err := rowref.DecodeCustomID(i.ModalSubmitData().CustomID)
			if err != nil || action != actionDecline {
				return
			}
			finalizeDecline(s, i, cfg, api, tpl, row, candidateID)
		}
	})
	return nil
}

func showDeclineModal(s *discordgo.Session, i *discordgo.InteractionCreate, cfg *config.Config) {
	if !hasRole(i.Member, cfg.ReviewerRoleID) {
		respondError(s, i.Interaction, "You need the reviewer role to use this command.")
		return
	}
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
	err = showModal(s, i.Interaction, rowref.CustomID(actionDecline, row, candidateID), "Decline", []*discordgo.TextInput{
		{CustomID: "criteria", Label: "Unmet criteria, 1 per line: Criterion: Issue", Style: discordgo.TextInputParagraph, Required: true},
	})
	if err != nil {
		respondError(s, i.Interaction, "Could not open the form. Please try again.")
	}
}

func finalizeDecline(s *discordgo.Session, i *discordgo.InteractionCreate, cfg *config.Config, api sheet.API, tpl *templates.Templates, row int, candidateID string) {
	if err := deferEphemeral(s, i.Interaction); err != nil {
		return
	}

	data := i.ModalSubmitData()
	criteriaRaw := modalValue(data, "criteria")

	missing := forms.MissingRequired([]forms.Field{
		{Label: "Unmet criteria", Value: criteriaRaw},
	})
	if len(missing) > 0 {
		editEphemeral(s, i.Interaction, fmt.Sprintf("Missing required field(s): %s", strings.Join(missing, ", ")))
		return
	}

	criteria := forms.SplitLines(criteriaRaw)
	message, err := tpl.Decline(criteria)
	if err != nil {
		log.Printf("decline: render template: %v", err)
		editEphemeral(s, i.Interaction, "Could not render the message template. Please contact a team member.")
		return
	}
	if err := sheet.UpdateFields(context.Background(), api, cfg.SheetID, cfg.SheetName, row, map[sheet.Column]string{
		sheet.ColumnStatus:          sheet.StatusDeclined,
		sheet.ColumnMissingCriteria: strings.Join(criteria, "; "),
		sheet.ColumnDecisionDate:    today(),
		sheet.ColumnReviewers:       i.Member.User.Username,
	}); err != nil {
		log.Printf("decline: update tracker for row %d: %v", row, err)
		editEphemeral(s, i.Interaction, "Could not update the tracker. Please try again.")
		return
	}

	if err := s.GuildMemberRoleRemove(cfg.GuildID, candidateID, cfg.CandidateRoleID); err != nil {
		log.Printf("decline: remove candidate role for %s: %v", candidateID, err)
		editEphemeral(s, i.Interaction, fmt.Sprintf("Updated the tracker, but could not remove the Testnet Validator Candidate role. Please remove it manually and relay this to the candidate:\n\n%s", message))
		return
	}

	if err := sendDM(s, candidateID, message); err != nil {
		editEphemeral(s, i.Interaction, fmt.Sprintf("Saved, but could not DM the candidate (DMs may be closed). Please relay this manually:\n\n%s", message))
		return
	}
	editEphemeral(s, i.Interaction, "Sent.")
}
