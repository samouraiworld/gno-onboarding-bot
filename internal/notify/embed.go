package notify

import (
	"fmt"

	"github.com/bwmarrin/discordgo"

	"onboardingbot/internal/rowref"
)

const (
	FieldMoniker         = "Moniker"
	FieldOperatorAddress = "Operator address"
	FieldValoperLink     = "Valoper link"
	FieldIntroduction    = "Introduction"
)

const embedFieldMax = 1024

// MessagePermalink builds the jump link for a message in a guild channel.
func MessagePermalink(guildID, channelID, messageID string) string {
	return fmt.Sprintf("https://discord.com/channels/%s/%s/%s", guildID, channelID, messageID)
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func BuildSubmissionEmbed(row int, candidateID, moniker, operatorAddr, valoperLink, introduction string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "New validator submission",
		Description: fmt.Sprintf("From <@%s>", candidateID),
		Fields: []*discordgo.MessageEmbedField{
			{Name: FieldMoniker, Value: moniker},
			{Name: FieldOperatorAddress, Value: operatorAddr},
			{Name: FieldValoperLink, Value: valoperLink},
			{Name: FieldIntroduction, Value: truncateRunes(introduction, embedFieldMax)},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: rowref.Encode(row, candidateID)},
	}
}

func ParseSubmissionEmbed(msg *discordgo.Message) (row int, candidateID, valoperLink string, err error) {
	if len(msg.Embeds) == 0 || msg.Embeds[0].Footer == nil {
		return 0, "", "", fmt.Errorf("notification message has no usable embed")
	}
	row, candidateID, err = rowref.Decode(msg.Embeds[0].Footer.Text)
	if err != nil {
		return 0, "", "", err
	}
	for _, f := range msg.Embeds[0].Fields {
		if f.Name == FieldValoperLink {
			valoperLink = f.Value
		}
	}
	return row, candidateID, valoperLink, nil
}
