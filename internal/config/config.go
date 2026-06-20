package config

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

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
	GnoRPCEndpoint           string `yaml:"gno_rpc_endpoint"`
	GnoWebBaseURL            string `yaml:"gnoweb_base_url"`

	// Harvest pass (optional). HarvestMaxMessages defaults to 2000 per channel
	// when unset or non-positive; HarvestSince is an RFC3339 lower bound (empty =
	// all available history).
	HarvestSince       string    `yaml:"harvest_since"`
	HarvestMaxMessages int       `yaml:"harvest_max_messages"`
	HarvestSinceParsed time.Time `yaml:"-"`
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
	if cfg.HarvestMaxMessages <= 0 {
		cfg.HarvestMaxMessages = 2000
	}
	if cfg.HarvestSince != "" {
		t, perr := time.Parse(time.RFC3339, cfg.HarvestSince)
		if perr != nil {
			return nil, fmt.Errorf("config harvest_since %q is not a valid RFC3339 timestamp: %w", cfg.HarvestSince, perr)
		}
		cfg.HarvestSinceParsed = t
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
		"gno_rpc_endpoint":            c.GnoRPCEndpoint,
		"gnoweb_base_url":             c.GnoWebBaseURL,
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
