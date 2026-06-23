package handlers

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"

	"onboardingbot/internal/config"
	"onboardingbot/internal/sheet"
	"onboardingbot/internal/templates"
)

func RegisterCandidate(s *discordgo.Session, cfg *config.Config, api sheet.API, tpl *templates.Templates) error {
	cmd := &discordgo.ApplicationCommand{
		Name:        "candidate-testnet",
		Description: "Apply to become a test13 validator candidate",
		Type:        discordgo.ChatApplicationCommand,
	}
	if _, err := s.ApplicationCommandCreate(s.State.User.ID, cfg.GuildID, cmd); err != nil {
		return fmt.Errorf("create candidate-testnet command: %w", err)
	}

	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "candidate-testnet" {
			return
		}
		handleCandidateTestnet(s, i, cfg, api, tpl)
	})
	return nil
}

func handleCandidateTestnet(s *discordgo.Session, i *discordgo.InteractionCreate, cfg *config.Config, api sheet.API, tpl *templates.Templates) {
	if err := deferEphemeral(s, i.Interaction); err != nil {
		return
	}

	member := i.Member
	if hasRole(member, cfg.CandidateRoleID) || hasRole(member, cfg.ValidatorRoleID) {
		editEphemeral(s, i.Interaction, "You already have the Testnet Validator Candidate role or higher.")
		return
	}

	// No tracker row is created at intake: the candidate's row is created by
	// /submit-request, the only command that writes a candidate row. A candidate
	// who never submits never appears in the tracker.
	if err := s.GuildMemberRoleAdd(cfg.GuildID, member.User.ID, cfg.CandidateRoleID); err != nil {
		log.Printf("candidate-testnet: add candidate role for %s: %v", member.User.ID, err)
		editEphemeral(s, i.Interaction, "Something went wrong assigning your role. Please contact a team member.")
		return
	}

	welcome, err := tpl.Welcome()
	if err != nil {
		editEphemeral(s, i.Interaction, "Role assigned, but the welcome message template failed to render. Please contact a team member.")
		return
	}
	if err := sendCandidateMessage(s, cfg.OnboardingChannelID, member.User.ID, welcome); err != nil {
		editEphemeral(s, i.Interaction, welcome)
		return
	}
	editEphemeral(s, i.Interaction, "Welcome posted in the onboarding channel.")
}
