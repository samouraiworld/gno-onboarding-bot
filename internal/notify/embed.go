package notify

import (
	"fmt"

	"github.com/bwmarrin/discordgo"

	"onboardingbot/internal/rowref"
)

const (
	FieldMonikerAddress = "Moniker & validator address"
	FieldValoperLink    = "Valoper link"
	FieldIntroduction   = "Introduction"
)

func BuildSubmissionEmbed(row int, candidateID, monikerAddress, valoperLink, introduction string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "New validator submission",
		Description: fmt.Sprintf("From <@%s>", candidateID),
		Fields: []*discordgo.MessageEmbedField{
			{Name: FieldMonikerAddress, Value: monikerAddress},
			{Name: FieldValoperLink, Value: valoperLink},
			{Name: FieldIntroduction, Value: introduction},
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
