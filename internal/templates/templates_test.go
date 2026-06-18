package templates

import (
	"os"
	"testing"
)

func TestLoad_RendersAllMessagesVerbatim(t *testing.T) {
	tpl, err := Load("../../templates.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("Welcome", func(t *testing.T) {
		want := "Welcome! Anyone can apply to become a Testnet validator candidate.\n\n" +
			"I have assigned you the `Testnet Validator Candidate` role. Please go to `#testnet-onboarding`, read the pinned instructions, and complete the onboarding challenge. Once you submit the requested evidence, a member of the Gno team will review it and send you the next steps."
		got, err := tpl.Welcome()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("Welcome() = %q, want %q", got, want)
		}
	})

	t.Run("Acknowledge", func(t *testing.T) {
		want := "Thanks, we received your validator onboarding submission. A member of the Gno team will review it against the published criteria and reply by `5 business days`. Please do not send any private key or seed phrase during the review."
		got, err := tpl.Acknowledge("5 business days")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("Acknowledge() = %q, want %q", got, want)
		}
	})

	t.Run("RequestMissingInfo", func(t *testing.T) {
		want := "Thanks for the submission. Before we can complete the review, please provide or correct the following:\n\n" +
			"- `Sync evidence`\n- `Valoper link`\n\n" +
			"Please reply with the updated public evidence. Do not share private keys, seed phrases, or validator signing keys."
		got, err := tpl.RequestMissingInfo([]string{"Sync evidence", "Valoper link"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("RequestMissingInfo() = %q, want %q", got, want)
		}
	})

	t.Run("Approve", func(t *testing.T) {
		want := "Your onboarding challenge has been approved. We have assigned you the `Testnet Validator` role.\n\n" +
			"Next steps:\n\n" +
			"1. Wait for GovDAO approval and confirmation before considering your validator active.\n\n" +
			"New external validators start with voting power `1` and may receive more voting power later under a separate, documented process."
		got, err := tpl.Approve()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("Approve() = %q, want %q", got, want)
		}
	})

	t.Run("AskToRetry", func(t *testing.T) {
		want := "Thanks for completing the onboarding challenge. We cannot approve it yet because the following published criteria are not complete:\n\n" +
			"- `Sync: not synced`\n- `Profile: missing intro`\n\n" +
			"To retry, please `re-sync and resubmit` and submit the updated evidence here. You are welcome to ask technical questions if any instruction is unclear."
		got, err := tpl.AskToRetry([]string{"Sync: not synced", "Profile: missing intro"}, "re-sync and resubmit")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("AskToRetry() = %q, want %q", got, want)
		}
	})

	t.Run("EscalateToCall", func(t *testing.T) {
		want := "Thanks for the submission. Most of the challenge is complete, but we need to clarify `sync status` before making a decision. Could you join a short technical call at one of these times: `Tue 10:00 UTC, Wed 15:00 UTC`? The call will focus on `sync status`."
		got, err := tpl.EscalateToCall("sync status", "Tue 10:00 UTC, Wed 15:00 UTC", "sync status")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("EscalateToCall() = %q, want %q", got, want)
		}
	})
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/templates.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/templates.yaml"
	if err := os.WriteFile(path, []byte("welcome: [unterminated"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestLoad_MalformedTemplateSyntax(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/templates.yaml"
	if err := os.WriteFile(path, []byte(`welcome: "{{.Unclosed"`), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for malformed template syntax")
	}
}
