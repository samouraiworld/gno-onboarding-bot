package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

const validConfig = `
discord_token: "token"
guild_id: "1"
general_chat_channel_id: "2"
onboarding_channel_id: "3"
validator_review_channel_id: "4"
candidate_role_id: "5"
validator_role_id: "6"
reviewer_role_id: "7"
govdao_contact_user_id: "8"
google_credentials_file: "creds.json"
sheet_id: "sheet-id"
sheet_name: "Candidates"
review_sla: "5 business days"
gno_rpc_endpoint: "https://rpc.example:443"
gnoweb_base_url: "https://gnoweb.example"
`

func TestLoad_Valid(t *testing.T) {
	path := writeTempConfig(t, validConfig)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DiscordToken != "token" {
		t.Errorf("DiscordToken = %q, want %q", cfg.DiscordToken, "token")
	}
	if cfg.SheetName != "Candidates" {
		t.Errorf("SheetName = %q, want %q", cfg.SheetName, "Candidates")
	}
	if cfg.GnoRPCEndpoint != "https://rpc.example:443" {
		t.Errorf("GnoRPCEndpoint = %q", cfg.GnoRPCEndpoint)
	}
	if cfg.GnoWebBaseURL != "https://gnoweb.example" {
		t.Errorf("GnoWebBaseURL = %q", cfg.GnoWebBaseURL)
	}
}

func TestLoad_MissingFields(t *testing.T) {
	path := writeTempConfig(t, `discord_token: "token"`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing fields")
	}
	if !strings.Contains(err.Error(), "guild_id") {
		t.Errorf("error %q should mention guild_id", err.Error())
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_HarvestDefault(t *testing.T) {
	cfg, err := Load(writeTempConfig(t, validConfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HarvestMaxMessages != 2000 {
		t.Errorf("HarvestMaxMessages = %d, want 2000", cfg.HarvestMaxMessages)
	}
	if !cfg.HarvestSinceParsed.IsZero() {
		t.Errorf("HarvestSinceParsed = %v, want zero", cfg.HarvestSinceParsed)
	}
}

func TestLoad_InvalidHarvestSince(t *testing.T) {
	path := writeTempConfig(t, validConfig+"\nharvest_since: \"not-a-timestamp\"\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for invalid harvest_since")
	}
}
