package handlers

import "github.com/bwmarrin/discordgo"

func restrictCommand(s *discordgo.Session, guildID, commandID, channelID, roleID string) error {
	var perms []*discordgo.ApplicationCommandPermissions
	if channelID != "" {
		allChannels, err := discordgo.GuildAllChannelsID(guildID)
		if err != nil {
			return err
		}
		perms = append(perms,
			&discordgo.ApplicationCommandPermissions{ID: allChannels, Type: discordgo.ApplicationCommandPermissionTypeChannel, Permission: false},
			&discordgo.ApplicationCommandPermissions{ID: channelID, Type: discordgo.ApplicationCommandPermissionTypeChannel, Permission: true},
		)
	}
	if roleID != "" {
		perms = append(perms,
			&discordgo.ApplicationCommandPermissions{ID: guildID, Type: discordgo.ApplicationCommandPermissionTypeRole, Permission: false},
			&discordgo.ApplicationCommandPermissions{ID: roleID, Type: discordgo.ApplicationCommandPermissionTypeRole, Permission: true},
		)
	}
	if len(perms) == 0 {
		return nil
	}
	return s.ApplicationCommandPermissionsEdit(s.State.User.ID, guildID, commandID, &discordgo.ApplicationCommandPermissionsList{Permissions: perms})
}
