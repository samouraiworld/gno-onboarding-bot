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
	"onboardingbot/internal/sheet"
	"onboardingbot/internal/templates"
)

const actionSubmit = "submit_request"

func RegisterSubmit(s *discordgo.Session, cfg *config.Config, api sheet.API, tpl *templates.Templates) error {
	cmd := &discordgo.ApplicationCommand{
		Name:        "submit-request",
		Description: "Submit your validator onboarding evidence for review",
		Type:        discordgo.ChatApplicationCommand,
	}
	if _, err := s.ApplicationCommandCreate(s.State.User.ID, cfg.GuildID, cmd); err != nil {
		return fmt.Errorf("create submit-request command: %w", err)
	}

	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			if i.ApplicationCommandData().Name != "submit-request" {
				return
			}
			showSubmitModal(s, i, cfg)
		case discordgo.InteractionModalSubmit:
			if i.ModalSubmitData().CustomID != actionSubmit {
				return
			}
			finalizeSubmit(s, i, cfg, api, tpl)
		}
	})
	return nil
}

func showSubmitModal(s *discordgo.Session, i *discordgo.InteractionCreate, cfg *config.Config) {
	if !hasRole(i.Member, cfg.CandidateRoleID) {
		respondError(s, i.Interaction, "You need the Testnet Validator Candidate role to submit an application.")
		return
	}
	err := showModal(s, i.Interaction, actionSubmit, "Submit your validator evidence", []*discordgo.TextInput{
		{CustomID: "moniker_address", Label: "Moniker and validator address", Style: discordgo.TextInputShort, Required: true},
		{CustomID: "valoper_link", Label: "Public Valoper profile link", Style: discordgo.TextInputShort, Required: true},
		{CustomID: "introduction", Label: "Short introduction", Style: discordgo.TextInputParagraph, Required: true},
	})
	if err != nil {
		respondError(s, i.Interaction, "Could not open the form. Please try again.")
	}
}

func finalizeSubmit(s *discordgo.Session, i *discordgo.InteractionCreate, cfg *config.Config, api sheet.API, tpl *templates.Templates) {
	if err := deferEphemeral(s, i.Interaction); err != nil {
		return
	}

	data := i.ModalSubmitData()
	monikerAddress := modalValue(data, "moniker_address")
	valoperLink := modalValue(data, "valoper_link")
	introduction := modalValue(data, "introduction")

	missing := forms.MissingRequired([]forms.Field{
		{Label: "Moniker and validator address", Value: monikerAddress},
		{Label: "Public Valoper profile link", Value: valoperLink},
		{Label: "Short introduction", Value: introduction},
	})
	if len(missing) > 0 {
		editEphemeral(s, i.Interaction, fmt.Sprintf("Missing required field(s): %s", strings.Join(missing, ", ")))
		return
	}

	candidateID := i.Member.User.ID
	row := sheet.CandidateRow{
		Candidate:          i.Member.User.Username,
		Discord:            "@" + i.Member.User.Username,
		Status:             sheet.StatusChallengeInProgress,
		ChallengeSubmitted: today(),
		Valoper:            valoperLink,
		MonikerAddress:     monikerAddress,
		Introduction:       introduction,
	}
	rowNumber, err := sheet.AppendCandidateRow(context.Background(), api, cfg.SheetID, cfg.SheetName, row)
	if err != nil {
		log.Printf("submit-request: append candidate row for %s: %v", candidateID, err)
		editEphemeral(s, i.Interaction, "Something went wrong recording your submission. Please try again or contact a team member.")
		return
	}

	embed := notify.BuildSubmissionEmbed(rowNumber, candidateID, monikerAddress, valoperLink, introduction)
	notifMsg, err := s.ChannelMessageSendComplex(cfg.ValidatorReviewChannelID, &discordgo.MessageSend{
		Content:         fmt.Sprintf("<@&%s> new submission to review.", cfg.ReviewerRoleID),
		Embeds:          []*discordgo.MessageEmbed{embed},
		AllowedMentions: &discordgo.MessageAllowedMentions{Roles: []string{cfg.ReviewerRoleID}},
	})
	if err != nil {
		log.Printf("submit-request: post review notification for row %d: %v", rowNumber, err)
		editEphemeral(s, i.Interaction, "Your submission was saved, but the review notification could not be posted. Please contact a team member.")
		return
	}

	link := fmt.Sprintf("https://discord.com/channels/%s/%s/%s", cfg.GuildID, cfg.ValidatorReviewChannelID, notifMsg.ID)
	if err := sheet.UpdateFields(context.Background(), api, cfg.SheetID, cfg.SheetName, rowNumber, map[sheet.Column]string{
		sheet.ColumnReviewMessageLink: link,
	}); err != nil {
		log.Printf("update review message link for row %d: %v", rowNumber, err)
	}

	ack, err := tpl.Acknowledge(cfg.ReviewSLA)
	if err != nil {
		editEphemeral(s, i.Interaction, "Submission received, but the acknowledgement template failed to render. Please contact a team member.")
		return
	}
	if err := sendDM(s, candidateID, ack); err != nil {
		editEphemeral(s, i.Interaction, ack)
		return
	}
	editEphemeral(s, i.Interaction, "Submission received! Check your DMs for confirmation.")
}
