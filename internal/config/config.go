package config

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DiscordToken             string `yaml:"discord_token"`
	GuildID                  string `yaml:"guild_id"`
	GeneralChatChannelID     string `yaml:"general_chat_channel_id"`
	OnboardingChannelID      string `yaml:"onboarding_channel_id"`
	ValidatorReviewChannelID string `yaml:"validator_review_channel_id"`
	CandidateRoleID          string `yaml:"candidate_role_id"`
	ValidatorRoleID          string `yaml:"validator_role_id"`
	ReviewerRoleID           string `yaml:"reviewer_role_id"`
	GovDAOContactUserID      string `yaml:"govdao_contact_user_id"`
	GoogleCredentialsFile    string `yaml:"google_credentials_file"`
	SheetID                  string `yaml:"sheet_id"`
	SheetName                string `yaml:"sheet_name"`
	ReviewSLA                string `yaml:"review_sla"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c Config) validate() error {
	required := map[string]string{
		"discord_token":               c.DiscordToken,
		"guild_id":                    c.GuildID,
		"general_chat_channel_id":     c.GeneralChatChannelID,
		"onboarding_channel_id":       c.OnboardingChannelID,
		"validator_review_channel_id": c.ValidatorReviewChannelID,
		"candidate_role_id":           c.CandidateRoleID,
		"validator_role_id":           c.ValidatorRoleID,
		"reviewer_role_id":            c.ReviewerRoleID,
		"govdao_contact_user_id":      c.GovDAOContactUserID,
		"google_credentials_file":     c.GoogleCredentialsFile,
		"sheet_id":                    c.SheetID,
		"sheet_name":                  c.SheetName,
		"review_sla":                  c.ReviewSLA,
	}
	var missing []string
	for key, value := range required {
		if value == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("config missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}
