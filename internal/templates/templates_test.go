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
		want := "Welcome! The `Testnet Validator Candidate` role has been assigned.\n\n" +
			"You now have access to `#testnet-onboarding`. Read the pinned instructions and complete the challenge. Once the node and application are ready, run `/submit-request` in `#testnet-onboarding` and provide only the operator address."
		got, err := tpl.Welcome()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("Welcome() = %q, want %q", got, want)
		}
	})

	t.Run("Acknowledge", func(t *testing.T) {
		want := "Thanks, we received the operator address submitted with `/submit-request`. The Gno team will review it against the published criteria and reply."
		got, err := tpl.Acknowledge()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("Acknowledge() = %q, want %q", got, want)
		}
	})

	t.Run("RequestMissingInfo", func(t *testing.T) {
		want := "Thanks. Before we can finish the review, please correct:\n\n" +
			"- `Sync evidence`\n- `Valoper link`\n\n" +
			"After fixing it, run `/submit-request` again and provide only the operator address."
		got, err := tpl.RequestMissingInfo([]string{"Sync evidence", "Valoper link"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("RequestMissingInfo() = %q, want %q", got, want)
		}
	})

	t.Run("Approve", func(t *testing.T) {
		want := "Congratulations, you passed the reviewers' onboarding check. Your application has been forwarded to the GovDAO.\n\n" +
			"Next: the GovDAO must admit your validator to the active set. Once your validator appears in the active set, the bot automatically grants you the `Testnet Validator` role and notifies you. New external validators start with voting power `1` and may earn more later through a separate, documented process."
		got, err := tpl.Approve()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("Approve() = %q, want %q", got, want)
		}
	})

	t.Run("Activated", func(t *testing.T) {
		want := "Your validator is now in the active set — the GovDAO has admitted it, and the bot has granted you the `Testnet Validator` role. Welcome aboard!"
		got, err := tpl.Activated()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("Activated() = %q, want %q", got, want)
		}
	})

	t.Run("Decline", func(t *testing.T) {
		want := "Thanks for completing the challenge. We cannot approve this application because the following published criteria are not met:\n\n" +
			"- `Sync: not synced`\n- `Profile: missing intro`\n\n" +
			"The `Testnet Validator Candidate` role will now be removed.\n\n" +
			"To apply again, restart the process from the beginning: run `/candidate-testnet` in `┋💬ㆍgeneral-chat`, complete every pinned onboarding step, then run `/submit-request` with only the operator address. The new application will be reviewed independently."
		got, err := tpl.Decline([]string{"Sync: not synced", "Profile: missing intro"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("Decline() = %q, want %q", got, want)
		}
	})

	t.Run("EscalateToCall", func(t *testing.T) {
		want := "Thanks for the submission. Before we decide, we need to clarify `sync status`. Can you join a short technical call at one of these times: `Tue 10:00 UTC, Wed 15:00 UTC`? It will focus on `sync status`."
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
