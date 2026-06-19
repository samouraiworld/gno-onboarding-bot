package handlers

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"

	"onboardingbot/internal/config"
	"onboardingbot/internal/notify"
	"onboardingbot/internal/sheet"
	"onboardingbot/internal/templates"
)

func RegisterApprove(s *discordgo.Session, cfg *config.Config, api sheet.API, tpl *templates.Templates) error {
	cmd := &discordgo.ApplicationCommand{
		Name: "Approve",
		Type: discordgo.MessageApplicationCommand,
	}
	created, err := s.ApplicationCommandCreate(s.State.User.ID, cfg.GuildID, cmd)
	if err != nil {
		return fmt.Errorf("create Approve command: %w", err)
	}
	if err := restrictCommand(s, cfg.GuildID, created.ID, cfg.ValidatorReviewChannelID, cfg.ReviewerRoleID); err != nil {
		return fmt.Errorf("restrict Approve command: %w", err)
	}

	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "Approve" {
			return
		}
		handleApprove(s, i, cfg, api, tpl)
	})
	return nil
}

func handleApprove(s *discordgo.Session, i *discordgo.InteractionCreate, cfg *config.Config, api sheet.API, tpl *templates.Templates) {
	if err := deferEphemeral(s, i.Interaction); err != nil {
		return
	}

	data := i.ApplicationCommandData()
	msg, ok := data.Resolved.Messages[data.TargetID]
	if !ok {
		editEphemeral(s, i.Interaction, "Could not find the submission message. It may have been deleted.")
		return
	}
	row, candidateID, valoperLink, err := notify.ParseSubmissionEmbed(msg)
	if err != nil {
		editEphemeral(s, i.Interaction, "This message is not a valid submission notification.")
		return
	}

	if err := sheet.UpdateFields(context.Background(), api, cfg.SheetID, cfg.SheetName, row, map[sheet.Column]string{
		sheet.ColumnStatus:       sheet.StatusGovDAOPending,
		sheet.ColumnDecisionDate: today(),
		sheet.ColumnReviewers:    i.Member.User.Username,
	}); err != nil {
		editEphemeral(s, i.Interaction, "Could not update the tracker. Please try again.")
		return
	}

	if err := s.GuildMemberRoleAdd(cfg.GuildID, candidateID, cfg.ValidatorRoleID); err != nil {
		editEphemeral(s, i.Interaction, "Updated the tracker, but could not grant the Testnet Validator role. Please assign it manually.")
		return
	}
	if err := s.GuildMemberRoleRemove(cfg.GuildID, candidateID, cfg.CandidateRoleID); err != nil {
		editEphemeral(s, i.Interaction, "Granted the Testnet Validator role, but could not remove Testnet Validator Candidate. Please remove it manually.")
		return
	}

	message, err := tpl.Approve()
	if err != nil {
		editEphemeral(s, i.Interaction, "Role updated, but the approval message template failed to render. Please contact a team member.")
		return
	}
	dmFailed := sendDM(s, candidateID, message) != nil

	govDAOMessage := fmt.Sprintf("<@%s> please validate this candidate's entry into the active set via GovDAO. Valoper: %s", cfg.GovDAOContactUserID, valoperLink)
	_, govDAOErr := s.ChannelMessageSend(cfg.ValidatorReviewChannelID, govDAOMessage)
	govDAOFailed := govDAOErr != nil

	if !dmFailed && !govDAOFailed {
		editEphemeral(s, i.Interaction, "Approved.")
		return
	}

	if dmFailed && govDAOFailed {
		editEphemeral(s, i.Interaction, fmt.Sprintf("Approved, but could not DM the candidate (DMs may be closed) and could not post the GovDAO notification. Please relay this manually and tag them manually:\n\n%s", message))
		return
	}

	if dmFailed {
		editEphemeral(s, i.Interaction, fmt.Sprintf("Approved, but could not DM the candidate (DMs may be closed). Please relay this manually:\n\n%s", message))
		return
	}

	editEphemeral(s, i.Interaction, "Approved, but could not post the GovDAO notification. Please tag them manually.")
}
