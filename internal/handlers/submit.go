package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"

	"onboardingbot/internal/config"
	"onboardingbot/internal/forms"
	"onboardingbot/internal/notify"
	"onboardingbot/internal/sheet"
	"onboardingbot/internal/templates"
	"onboardingbot/internal/valoper"
)

const actionSubmit = "submit_request"

// submitMu serializes the duplicate-check + append critical section so two
// concurrent submissions of the same operator address cannot both pass the
// check and write a row. The bot is a single process writing a single sheet,
// so one process-wide mutex is sufficient and cheap.
var submitMu sync.Mutex

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

	// operatorAddr from the render is ignored: addr is the address we queried
	// qrender with, so it is the canonical operator address and cannot be
	// spoofed by free text in the description.
	moniker, _, description, err := valoper.ParseRender(raw)
	if err != nil {
		if errors.Is(err, valoper.ErrNotRegistered) {
			editEphemeral(s, i.Interaction, "No valoper profile found for that address. Register on r/gnops/valopers first, then resubmit.")
			return
		}
		log.Printf("submit: parse render for %s failed: %v", addr, err)
		editEphemeral(s, i.Interaction, "Could not read your valoper profile from the chain. Please contact a team member.")
		return
	}

	valoperLink := valoper.ProfileURL(cfg.GnoWebBaseURL, addr)

	// Serialize the duplicate-check + append so two concurrent submissions of
	// the same address cannot both pass the check and double-write.
	submitMu.Lock()
	if existingRow, existingStatus, dupErr := sheet.FindByOperatorAddress(context.Background(), api, cfg.SheetID, cfg.SheetName, addr); dupErr != nil {
		log.Printf("submit: duplicate lookup for %s failed: %v", addr, dupErr)
	} else if existingRow > 0 && !sheet.IsReopenable(existingStatus) {
		submitMu.Unlock()
		log.Printf("submit: duplicate addr=%s row=%d status=%q", addr, existingRow, existingStatus)
		editEphemeral(s, i.Interaction, fmt.Sprintf("This operator address is already in the tracker (row %d, status %q). You can submit again once that row is marked %q or %q.", existingRow, existingStatus, sheet.StatusNeedsRetry, sheet.StatusDeclined))
		return
	}

	row := sheet.CandidateRow{
		Candidate:          i.Member.User.Username,
		Discord:            "@" + i.Member.User.Username,
		Status:             sheet.StatusChallengeInProgress,
		ChallengeSubmitted: today(),
		Valoper:            valoperLink,
		Moniker:            moniker,
		OperatorAddress:    addr,
		Introduction:       description,
	}
	rowNumber, err := sheet.AppendCandidateRow(context.Background(), api, cfg.SheetID, cfg.SheetName, row)
	submitMu.Unlock()
	if err != nil {
		log.Printf("submit: append row for %s: %v", candidateID, err)
		editEphemeral(s, i.Interaction, "Something went wrong recording your submission. Please try again or contact a team member.")
		return
	}
	log.Printf("submit: OK user=%s moniker=%q row=%d", i.Member.User.Username, moniker, rowNumber)

	// Post the reviewer notification before the cosmetic writes. If it fails,
	// roll the row back so the candidate is not left with an un-retryable
	// orphan row (the duplicate guard only re-opens on "Needs retry" / "Declined").
	embed := notify.BuildSubmissionEmbed(rowNumber, candidateID, moniker, addr, valoperLink, description)
	notifMsg, err := s.ChannelMessageSendComplex(cfg.ValidatorReviewChannelID, &discordgo.MessageSend{
		Content:         fmt.Sprintf("<@&%s> new submission to review.", cfg.ReviewerRoleID),
		Embeds:          []*discordgo.MessageEmbed{embed},
		AllowedMentions: &discordgo.MessageAllowedMentions{Roles: []string{cfg.ReviewerRoleID}},
	})
	if err != nil {
		log.Printf("submit: post review notification for row %d: %v", rowNumber, err)
		if clearErr := sheet.ClearRow(context.Background(), api, cfg.SheetID, cfg.SheetName, rowNumber); clearErr != nil {
			log.Printf("submit: rollback row %d after notification failure: %v", rowNumber, clearErr)
		}
		editEphemeral(s, i.Interaction, "Could not post the review notification, so your submission was not recorded. Please try again in a moment.")
		return
	}

	// Submission committed and reviewers notified. The rest is best-effort
	// formatting; failures here are logged but do not fail the submission.
	link := fmt.Sprintf("https://discord.com/channels/%s/%s/%s", cfg.GuildID, cfg.ValidatorReviewChannelID, notifMsg.ID)
	if err := sheet.UpdateFields(context.Background(), api, cfg.SheetID, cfg.SheetName, rowNumber, map[sheet.Column]string{
		sheet.ColumnReviewMessageLink: link,
	}); err != nil {
		log.Printf("submit: update review message link for row %d: %v", rowNumber, err)
	}
	if err := api.SetLinkedText(context.Background(), cfg.SheetID, cfg.SheetName, rowNumber, int(sheet.ColumnDiscord), "@"+i.Member.User.Username, "https://discord.com/users/"+candidateID); err != nil {
		log.Printf("submit: set Discord hyperlink for row %d: %v", rowNumber, err)
	}
	if err := sheet.ApplyStatusDropdown(context.Background(), api, cfg.SheetID, cfg.SheetName, rowNumber); err != nil {
		log.Printf("submit: extend status dropdown to row %d: %v", rowNumber, err)
	}

	ack, err := tpl.Acknowledge()
	if err != nil {
		editEphemeral(s, i.Interaction, "Submission received, but the acknowledgement template failed to render. Please contact a team member.")
		return
	}
	if err := sendCandidateMessage(s, cfg.OnboardingChannelID, candidateID, ack); err != nil {
		editEphemeral(s, i.Interaction, ack)
		return
	}
	editEphemeral(s, i.Interaction, "Submission received! Confirmation posted in the onboarding channel.")
}
