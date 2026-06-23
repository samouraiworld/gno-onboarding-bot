package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"

	"onboardingbot/internal/config"
	"onboardingbot/internal/sheet"
	"onboardingbot/internal/templates"
)

// guildMembersPageSize is the max members Discord returns per List Guild Members
// REST call; we paginate with `after` until a short page is returned.
const guildMembersPageSize = 1000

func RegisterRemoveValidatorRole(s *discordgo.Session, cfg *config.Config, api sheet.API, tpl *templates.Templates) error {
	cmd := &discordgo.ApplicationCommand{
		Name:        "remove-validator-role",
		Description: "Remove the Testnet Validator role from all members and DM them (onboarding reset)",
		Type:        discordgo.ChatApplicationCommand,
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "announcement-link",
				Description: "Link to the onboarding reset announcement, included in the DM",
				Required:    true,
			},
		},
	}
	if _, err := s.ApplicationCommandCreate(s.State.User.ID, cfg.GuildID, cmd); err != nil {
		return fmt.Errorf("create remove-validator-role command: %w", err)
	}

	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand || i.ApplicationCommandData().Name != "remove-validator-role" {
			return
		}
		handleRemoveValidatorRole(s, i, cfg, tpl)
	})
	return nil
}

func handleRemoveValidatorRole(s *discordgo.Session, i *discordgo.InteractionCreate, cfg *config.Config, tpl *templates.Templates) {
	if !hasRole(i.Member, cfg.ReviewerRoleID) {
		respondError(s, i.Interaction, "You need the reviewer role to use this command.")
		return
	}
	if err := deferEphemeral(s, i.Interaction); err != nil {
		return
	}

	announcementLink := strings.TrimSpace(optionValue(i.ApplicationCommandData().Options, "announcement-link"))
	if announcementLink == "" {
		editEphemeral(s, i.Interaction, "Missing required field: announcement-link.")
		return
	}

	members, err := allGuildMembers(s, cfg.GuildID)
	if err != nil {
		log.Printf("remove-validator-role: list guild members: %v", err)
		editEphemeral(s, i.Interaction, "Could not list the server members. Please try again.")
		return
	}

	var (
		removed    int
		roleErrors []string
		dmFailures []string
	)
	for _, m := range members {
		if m.User == nil || m.User.Bot || !hasRole(m, cfg.ValidatorRoleID) {
			continue
		}

		message, rerr := tpl.RoleRemoved(m.DisplayName(), announcementLink)
		if rerr != nil {
			log.Printf("remove-validator-role: render template for %s: %v", m.User.ID, rerr)
			editEphemeral(s, i.Interaction, "Could not render the message template. Please contact a team member.")
			return
		}

		if err := s.GuildMemberRoleRemove(cfg.GuildID, m.User.ID, cfg.ValidatorRoleID); err != nil {
			log.Printf("remove-validator-role: remove role for %s: %v", m.User.ID, err)
			roleErrors = append(roleErrors, fmt.Sprintf("%s (%s)", m.DisplayName(), m.User.ID))
			continue
		}
		removed++

		if err := sendDM(s, m.User.ID, message); err != nil {
			log.Printf("remove-validator-role: DM %s: %v", m.User.ID, err)
			dmFailures = append(dmFailures, fmt.Sprintf("%s (%s)", m.DisplayName(), m.User.ID))
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Removed the Testnet Validator role from %d member(s).", removed)
	if len(roleErrors) > 0 {
		fmt.Fprintf(&b, "\n\nCould not remove the role from %d member(s) — please handle manually:\n- %s", len(roleErrors), strings.Join(roleErrors, "\n- "))
	}
	if len(dmFailures) > 0 {
		message, _ := tpl.RoleRemoved("[their name]", announcementLink)
		fmt.Fprintf(&b, "\n\nRole removed but the DM failed for %d member(s) (DMs may be closed) — please relay this manually:\n- %s\n\nMessage:\n%s", len(dmFailures), strings.Join(dmFailures, "\n- "), message)
	}
	editEphemeral(s, i.Interaction, b.String())
}

// allGuildMembers pages through the whole guild membership.
func allGuildMembers(s *discordgo.Session, guildID string) ([]*discordgo.Member, error) {
	var all []*discordgo.Member
	after := ""
	for {
		page, err := s.GuildMembers(guildID, after, guildMembersPageSize)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < guildMembersPageSize {
			break
		}
		after = page[len(page)-1].User.ID
	}
	return all, nil
}

func optionValue(options []*discordgo.ApplicationCommandInteractionDataOption, name string) string {
	for _, opt := range options {
		if opt.Name == name {
			return opt.StringValue()
		}
	}
	return ""
}
