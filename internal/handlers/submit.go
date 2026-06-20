package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"

	"onboardingbot/internal/config"
	"onboardingbot/internal/forms"
	"onboardingbot/internal/notify"
	"onboardingbot/internal/sheet"
	"onboardingbot/internal/templates"
	"onboardingbot/internal/valoper"
)

const actionSubmit = "submit_request"

func RegisterSubmit(s *discordgo.Session, cfg *config.Config, api sheet.API, tpl *templates.Templates, renderer valoper.Renderer) error {
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
			finalizeSubmit(s, i, cfg, api, tpl, renderer)
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
		{CustomID: "valoper_address", Label: "Operator address (g1...)", Style: discordgo.TextInputShort, Required: true, MaxLength: 80},
	})
	if err != nil {
		respondError(s, i.Interaction, "Could not open the form. Please try again.")
	}
}

func finalizeSubmit(s *discordgo.Session, i *discordgo.InteractionCreate, cfg *config.Config, api sheet.API, tpl *templates.Templates, renderer valoper.Renderer) {
	if err := deferEphemeral(s, i.Interaction); err != nil {
		return
	}

	data := i.ModalSubmitData()
	input := modalValue(data, "valoper_address")
	candidateID := i.Member.User.ID
	log.Printf("submit: user=%s input=%q", i.Member.User.Username, input)

	missing := forms.MissingRequired([]forms.Field{
		{Label: "Operator address", Value: input},
	})
	if len(missing) > 0 {
		editEphemeral(s, i.Interaction, fmt.Sprintf("Missing required field(s): %s", strings.Join(missing, ", ")))
		return
	}

	addr, err := valoper.AddressFromInput(input)
	if err != nil {
		log.Printf("submit: AddressFromInput(%q) failed: %v", input, err)
		editEphemeral(s, i.Interaction, "That is not a valid operator address. Paste your validator's g1 address (or your r/gnops/valopers profile link).")
		return
	}

	raw, err := renderer.Render(context.Background(), valoper.RealmPath+":"+addr)
	if err != nil {
		log.Printf("submit: qrender for %s failed: %v", addr, err)
		editEphemeral(s, i.Interaction, "Could not reach the chain to read your valoper profile. Please try again in a moment.")
		return
	}

	moniker, operatorAddr, description, err := valoper.ParseRender(raw)
	if err != nil {
		if errors.Is(err, valoper.ErrNotRegistered) {
			editEphemeral(s, i.Interaction, "No valoper profile found for that address. Register on r/gnops/valopers first, then resubmit.")
			return
		}
		log.Printf("submit: parse render for %s failed: %v", addr, err)
		editEphemeral(s, i.Interaction, "Could not read your valoper profile from the chain. Please contact a team member.")
		return
	}

	valoperLink := valoper.ProfileURL(cfg.GnoWebBaseURL, operatorAddr)

	if existingRow, existingStatus, dupErr := sheet.FindByOperatorAddress(context.Background(), api, cfg.SheetID, cfg.SheetName, operatorAddr); dupErr != nil {
		log.Printf("submit: duplicate lookup for %s failed: %v", operatorAddr, dupErr)
	} else if existingRow > 0 && existingStatus != sheet.StatusNeedsRetry {
		log.Printf("submit: duplicate addr=%s row=%d status=%q", operatorAddr, existingRow, existingStatus)
		editEphemeral(s, i.Interaction, fmt.Sprintf("This operator address is already in the tracker (row %d, status %q). If a reviewer asked you to retry, wait for that row to be marked %q before resubmitting.", existingRow, existingStatus, sheet.StatusNeedsRetry))
		return
	}

	row := sheet.CandidateRow{
		Candidate:          i.Member.User.Username,
		Discord:            "@" + i.Member.User.Username,
		Status:             sheet.StatusChallengeInProgress,
		ChallengeSubmitted: today(),
		Valoper:            valoperLink,
		Moniker:            moniker,
		OperatorAddress:    operatorAddr,
		Introduction:       description,
	}
	rowNumber, err := sheet.AppendCandidateRow(context.Background(), api, cfg.SheetID, cfg.SheetName, row)
	if err != nil {
		log.Printf("submit: append row for %s: %v", candidateID, err)
		editEphemeral(s, i.Interaction, "Something went wrong recording your submission. Please try again or contact a team member.")
		return
	}
	log.Printf("submit: OK user=%s moniker=%q row=%d", i.Member.User.Username, moniker, rowNumber)

	if err := api.SetLinkedText(context.Background(), cfg.SheetID, cfg.SheetName, rowNumber, int(sheet.ColumnDiscord), "@"+i.Member.User.Username, "https://discord.com/users/"+candidateID); err != nil {
		log.Printf("submit: set Discord hyperlink for row %d: %v", rowNumber, err)
	}
	if err := sheet.ApplyStatusDropdown(context.Background(), api, cfg.SheetID, cfg.SheetName, rowNumber); err != nil {
		log.Printf("submit: extend status dropdown to row %d: %v", rowNumber, err)
	}

	embed := notify.BuildSubmissionEmbed(rowNumber, candidateID, moniker, operatorAddr, valoperLink, description)
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
