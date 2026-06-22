# GovDAO On-Chain Role Activation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the `Testnet Validator` role grant from the reviewer's Approve action to an automatic on-chain check that fires once the candidate's validator joins the active set.

**Architecture:** Approve only sets status `GovDAO pending`. A new in-bot poller periodically reads `GovDAO pending` rows, derives each candidate's signing address from their operator address via the valopers realm, and when that signing address appears in the node's `/validators` set, writes `GovDAO submitted` then grants the role and DMs the candidate. The candidate's Discord ID is read back from the column-B hyperlink already persisted at submit time. No Sheet schema change.

**Tech Stack:** Go, `github.com/bwmarrin/discordgo`, `google.golang.org/api/sheets/v4`, gno JSON-RPC (`abci_query` / `validators`).

## Global Constraints

- **Sheet write before any Discord role mutation** — in activation, write `GovDAO submitted` before adding/removing roles.
- **No Sheet schema change** — signing address is derived, never stored; columns A–Y untouched.
- **Operator address (column K)** stays the dedup key and valopers realm key; it is never replaced.
- **Never log `config.yaml` / `service-account.json` contents.**
- Commit messages must contain **no** co-author / attribution trailer.
- Build/verify with `go build ./... && go vet ./... && go test ./...`.

---

### Task 1: `ParseRender` exposes the signing address

**Files:**
- Modify: `internal/valoper/valoper.go` (ParseRender)
- Modify: `internal/handlers/submit.go:104` (caller, 4→5 return values)
- Test: `internal/valoper/valoper_test.go`

**Interfaces:**
- Produces: `valoper.ParseRender(raw string) (moniker, operatorAddr, signingAddr, description string, err error)` — the new 3rd return `signingAddr` is the `- Signing Address:` value, or `""` when that line is absent (not an error).

- [ ] **Step 1: Update the existing tests to the 5-value signature and assert the signing address**

In `internal/valoper/valoper_test.go`, replace the three affected test functions with:

```go
func TestParseRender(t *testing.T) {
	moniker, gotAddr, signing, desc, err := ParseRender(fullRender)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if moniker != "SamouraiCoop" {
		t.Errorf("moniker = %q", moniker)
	}
	if gotAddr != addr {
		t.Errorf("addr = %q", gotAddr)
	}
	if signing != "g1k7asng8uzf74xs0tsrfwytldl76hs4l3asglym" {
		t.Errorf("signing = %q", signing)
	}
	want := "Multi-line intro.\n\nSecond paragraph with a [link](https://x)."
	if desc != want {
		t.Errorf("desc = %q, want %q", desc, want)
	}
}

func TestParseRender_EmptyDescription(t *testing.T) {
	raw := "Valoper's details:\n## Solo\n- Operator Address: g1abc\n- Server Type: cloud\n"
	moniker, gotAddr, signing, desc, err := ParseRender(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if moniker != "Solo" || gotAddr != "g1abc" || signing != "" || desc != "" {
		t.Errorf("got moniker=%q addr=%q signing=%q desc=%q", moniker, gotAddr, signing, desc)
	}
}

func TestParseRender_Unknown(t *testing.T) {
	for _, raw := range []string{"unknown address " + addr, "invalid address foo"} {
		if _, _, _, _, err := ParseRender(raw); !errors.Is(err, ErrNotRegistered) {
			t.Errorf("ParseRender(%q) err = %v, want ErrNotRegistered", raw, err)
		}
	}
}

func TestParseRender_MissingMarkers(t *testing.T) {
	if _, _, _, _, err := ParseRender("garbage with no markers"); !errors.Is(err, ErrUnparseable) {
		t.Errorf("err = %v, want ErrUnparseable", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail to compile**

Run: `go test ./internal/valoper/`
Expected: FAIL — `ParseRender` returns 4 values, not 5 (compile error / assignment mismatch).

- [ ] **Step 3: Update `ParseRender` to capture the signing address**

In `internal/valoper/valoper.go`, replace the `ParseRender` function body with:

```go
func ParseRender(raw string) (moniker, operatorAddr, signingAddr, description string, err error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if t := strings.TrimSpace(raw); strings.HasPrefix(t, "unknown address") || strings.HasPrefix(t, "invalid address") {
		return "", "", "", "", ErrNotRegistered
	}

	const opMarker = "- Operator Address:"
	const signMarker = "- Signing Address:"
	lines := strings.Split(raw, "\n")
	monikerIdx, opIdx := -1, -1
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if monikerIdx == -1 && strings.HasPrefix(t, "## ") {
			monikerIdx = i
			moniker = strings.TrimSpace(strings.TrimPrefix(t, "## "))
			continue
		}
		if opIdx == -1 && strings.HasPrefix(t, opMarker) {
			opIdx = i
			operatorAddr = strings.TrimSpace(strings.TrimPrefix(t, opMarker))
			continue
		}
		if signingAddr == "" && strings.HasPrefix(t, signMarker) {
			signingAddr = strings.TrimSpace(strings.TrimPrefix(t, signMarker))
		}
	}
	if monikerIdx == -1 || opIdx == -1 || moniker == "" || operatorAddr == "" {
		return "", "", "", "", ErrUnparseable
	}
	description = strings.TrimSpace(strings.Join(lines[monikerIdx+1:opIdx], "\n"))
	return moniker, operatorAddr, signingAddr, description, nil
}
```

(The previous loop `break`-ed at the operator marker; it now `continue`s so the later `- Signing Address:` line is still seen. Description is still the text between the moniker and the operator line, so it is unaffected.)

- [ ] **Step 4: Update the submit caller**

In `internal/handlers/submit.go`, change line 104 from:

```go
	moniker, _, description, err := valoper.ParseRender(raw)
```

to:

```go
	moniker, _, _, description, err := valoper.ParseRender(raw)
```

- [ ] **Step 5: Run tests + build to verify they pass**

Run: `go test ./internal/valoper/ && go build ./...`
Expected: PASS, build succeeds.

- [ ] **Step 6: Commit**

```bash
git add internal/valoper/valoper.go internal/valoper/valoper_test.go internal/handlers/submit.go
git commit -m "feat(valoper): ParseRender returns the signing address"
```

---

### Task 2: `valoper.Client.ValidatorSet`

**Files:**
- Modify: `internal/valoper/client.go`
- Test: `internal/valoper/client_test.go`

**Interfaces:**
- Produces: `(*valoper.Client).ValidatorSet(ctx context.Context) (map[string]struct{}, error)` — the set of active validator **signing addresses** from the node's `validators` JSON-RPC method.

- [ ] **Step 1: Write the failing tests**

Append to `internal/valoper/client_test.go`:

```go
func TestClientValidatorSet_Success(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"result":{"block_height":"1","validators":[` +
		`{"address":"g1aaa"},{"address":"g1bbb"}]}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Method != "validators" {
			t.Errorf("method = %q, want validators", req.Method)
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()

	set, err := NewClient(srv.URL).ValidatorSet(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(set) != 2 {
		t.Fatalf("len = %d, want 2", len(set))
	}
	if _, ok := set["g1aaa"]; !ok {
		t.Errorf("g1aaa missing")
	}
	if _, ok := set["g1bbb"]; !ok {
		t.Errorf("g1bbb missing")
	}
}

func TestClientValidatorSet_RPCError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"message":"boom"}}`))
	}))
	defer srv.Close()
	if _, err := NewClient(srv.URL).ValidatorSet(context.Background()); err == nil {
		t.Fatal("expected error on rpc-level error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/valoper/ -run ValidatorSet`
Expected: FAIL — `ValidatorSet` undefined.

- [ ] **Step 3: Implement `ValidatorSet`**

Append to `internal/valoper/client.go`:

```go
type rpcValidatorsRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

type validatorsResponse struct {
	Result *struct {
		Validators []struct {
			Address string `json:"address"`
		} `json:"validators"`
	} `json:"result"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ValidatorSet returns the set of active validator signing addresses reported by
// the node's `validators` RPC method, for O(1) membership checks. The gno node
// returns the full set in one call (no pagination).
func (c *Client) ValidatorSet(ctx context.Context) (map[string]struct{}, error) {
	reqBody, err := json.Marshal(rpcValidatorsRequest{
		JSONRPC: "2.0", ID: 1, Method: "validators", Params: struct{}{},
	})
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("validators request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("validators status %d", resp.StatusCode)
	}

	var vr validatorsResponse
	if err := json.NewDecoder(resp.Body).Decode(&vr); err != nil {
		return nil, fmt.Errorf("decode validators response: %w", err)
	}
	if vr.Error != nil {
		return nil, fmt.Errorf("rpc error: %s", vr.Error.Message)
	}
	if vr.Result == nil {
		return nil, fmt.Errorf("validators response missing result")
	}
	set := make(map[string]struct{}, len(vr.Result.Validators))
	for _, v := range vr.Result.Validators {
		if v.Address != "" {
			set[v.Address] = struct{}{}
		}
	}
	return set, nil
}
```

(The existing imports `bytes`, `context`, `encoding/json`, `fmt`, `net/http` already cover this.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/valoper/ -run ValidatorSet`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/valoper/client.go internal/valoper/client_test.go
git commit -m "feat(valoper): add ValidatorSet client method"
```

---

### Task 3: `validator_poll_interval` config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `config.example.yaml`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Config.ValidatorPollEvery time.Duration` — parsed poll interval, defaulting to `5 * time.Minute` when `validator_poll_interval` is unset. Invalid or non-positive values are a load error.

- [ ] **Step 1: Write the failing tests**

Append to `internal/config/config_test.go` (and add `"time"` to that file's imports):

```go
func TestLoad_ValidatorPollDefault(t *testing.T) {
	cfg, err := Load(writeTempConfig(t, validConfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ValidatorPollEvery != 5*time.Minute {
		t.Errorf("ValidatorPollEvery = %v, want 5m", cfg.ValidatorPollEvery)
	}
}

func TestLoad_ValidatorPollCustom(t *testing.T) {
	cfg, err := Load(writeTempConfig(t, validConfig+"\nvalidator_poll_interval: \"30s\"\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ValidatorPollEvery != 30*time.Second {
		t.Errorf("ValidatorPollEvery = %v, want 30s", cfg.ValidatorPollEvery)
	}
}

func TestLoad_InvalidValidatorPoll(t *testing.T) {
	path := writeTempConfig(t, validConfig+"\nvalidator_poll_interval: \"not-a-duration\"\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for invalid validator_poll_interval")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/`
Expected: FAIL — `ValidatorPollEvery` undefined.

- [ ] **Step 3: Add the field and parsing**

In `internal/config/config.go`, add the two fields to the `Config` struct (after `GnoWebBaseURL`):

```go
	// ValidatorPollEvery is how often the activation poller checks the active
	// validator set; ValidatorPollInterval is its Go-duration source (default 5m).
	ValidatorPollInterval string        `yaml:"validator_poll_interval"`
	ValidatorPollEvery    time.Duration `yaml:"-"`
```

Then in `Load`, after the `HarvestSince` block and before `cfg.validate()`, add:

```go
	cfg.ValidatorPollEvery = 5 * time.Minute
	if cfg.ValidatorPollInterval != "" {
		d, derr := time.ParseDuration(cfg.ValidatorPollInterval)
		if derr != nil {
			return nil, fmt.Errorf("config validator_poll_interval %q is not a valid Go duration: %w", cfg.ValidatorPollInterval, derr)
		}
		if d <= 0 {
			return nil, fmt.Errorf("config validator_poll_interval must be positive, got %q", cfg.ValidatorPollInterval)
		}
		cfg.ValidatorPollEvery = d
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/`
Expected: PASS.

- [ ] **Step 5: Document the field in `config.example.yaml`**

Append to `config.example.yaml`:

```yaml

# How often the bot checks the chain's active validator set to auto-grant the
# Testnet Validator role once a "GovDAO pending" candidate is admitted. Go
# duration (e.g. "5m", "30s"). Optional; defaults to 5m.
validator_poll_interval: "5m"
```

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go config.example.yaml
git commit -m "feat(config): add validator_poll_interval"
```

---

### Task 4: Templates — reword `approve`, add `activated`

**Files:**
- Modify: `templates.yaml`
- Modify: `internal/templates/templates.go`
- Test: `internal/templates/templates_test.go`

**Interfaces:**
- Produces: `(*templates.Templates).Activated() (string, error)` — the DM sent when the role is auto-granted.

- [ ] **Step 1: Update the Approve test and add an Activated test**

In `internal/templates/templates_test.go`, replace the `Approve` subtest's `want` and add an `Activated` subtest:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/templates/`
Expected: FAIL — `Activated` undefined and `Approve` text mismatch.

- [ ] **Step 3: Reword `approve` and add `activated` in `templates.yaml`**

Replace the `approve:` block in `templates.yaml` with:

```yaml
approve: |-
  Congratulations, you passed the reviewers' onboarding check. Your application has been forwarded to the GovDAO.

  Next: the GovDAO must admit your validator to the active set. Once your validator appears in the active set, the bot automatically grants you the `Testnet Validator` role and notifies you. New external validators start with voting power `1` and may earn more later through a separate, documented process.
```

And add a new `activated` block immediately after the `approve` block:

```yaml
activated: |-
  Your validator is now in the active set — the GovDAO has admitted it, and the bot has granted you the `Testnet Validator` role. Welcome aboard!
```

- [ ] **Step 4: Wire the `activated` template in `internal/templates/templates.go`**

Add the field to `rawTemplates` (after `EscalateToCall`):

```go
	Activated string `yaml:"activated"`
```

Add the field to `Templates` (after `escalateToCall`):

```go
	activated *template.Template
```

Add the entry to the `entries` slice in `Load` (after the `escalate_to_call` entry):

```go
		{"activated", raw.Activated, &t.activated},
```

Add the method (after `EscalateToCall`):

```go
func (t *Templates) Activated() (string, error) {
	return render(t.activated, nil)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/templates/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add templates.yaml internal/templates/templates.go internal/templates/templates_test.go
git commit -m "feat(templates): reword approve, add activated message"
```

---

### Task 5: Sheet `CellLink` + `DiscordIDFromUserURL`

**Files:**
- Modify: `internal/sheet/sheet.go` (API interface + `DiscordIDFromUserURL`)
- Modify: `internal/sheet/client.go` (`CellLink` real impl)
- Test: `internal/sheet/sheet_test.go` (`DiscordIDFromUserURL` test + `fakeAPI.CellLink`)

**Interfaces:**
- Produces: `sheet.DiscordIDFromUserURL(url string) (id string, ok bool)` — extracts `<id>` from `https://discord.com/users/<id>`.
- Produces: `API.CellLink(ctx context.Context, spreadsheetID, sheetName string, row, col int) (string, error)` — the hyperlink URI on a single cell (`row` 1-based, `col` 0-based `Column` index), `""` if none.

- [ ] **Step 1: Write the failing test for `DiscordIDFromUserURL`**

Append to `internal/sheet/sheet_test.go`:

```go
func TestDiscordIDFromUserURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"https://discord.com/users/123456789", "123456789", true},
		{"https://discord.com/users/", "", false},
		{"https://example.com/users/123", "", false},
		{"https://discord.com/users/123/extra", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := DiscordIDFromUserURL(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("DiscordIDFromUserURL(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/sheet/ -run DiscordIDFromUserURL`
Expected: FAIL — `DiscordIDFromUserURL` undefined.

- [ ] **Step 3: Implement `DiscordIDFromUserURL` and add `CellLink` to the interface**

In `internal/sheet/sheet.go`, add to the `API` interface (after `WriteRows`):

```go
	CellLink(ctx context.Context, spreadsheetID, sheetName string, row, col int) (string, error)
```

And add the pure helper (anywhere at top level, e.g. after `FindByOperatorAddress`):

```go
// DiscordIDFromUserURL extracts the numeric user ID from a Discord profile URL
// of the form https://discord.com/users/<id>. Returns ok=false for any other
// shape. The bot persists this URL as the column-B hyperlink at submit time, so
// the activation poller reads the candidate's Discord ID back from it.
func DiscordIDFromUserURL(url string) (string, bool) {
	const prefix = "https://discord.com/users/"
	if !strings.HasPrefix(url, prefix) {
		return "", false
	}
	id := strings.TrimPrefix(url, prefix)
	if id == "" || strings.ContainsAny(id, "/?#") {
		return "", false
	}
	return id, true
}
```

- [ ] **Step 4: Add `CellLink` to the test fake so the package compiles**

In `internal/sheet/sheet_test.go`, add a method on `fakeAPI`:

```go
func (f *fakeAPI) CellLink(ctx context.Context, spreadsheetID, sheetName string, row, col int) (string, error) {
	return "", nil
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/sheet/`
Expected: PASS.

- [ ] **Step 6: Implement the real `CellLink` on `GoogleSheetsClient`**

Append to `internal/sheet/client.go`:

```go
// CellLink returns the hyperlink URI attached to a single cell, or "" if the
// cell has no link. row is 1-based; col is a 0-based Column index. The link is
// read from the cell's textFormatRuns (how SetLinkedText writes it) with the
// cell-level hyperlink field as a fallback.
func (c *GoogleSheetsClient) CellLink(ctx context.Context, spreadsheetID, sheetName string, row, col int) (string, error) {
	cell := fmt.Sprintf("%s!%s%d", sheetName, columnLetter(Column(col)), row)
	resp, err := c.svc.Spreadsheets.Get(spreadsheetID).
		Ranges(cell).
		Fields("sheets.data.rowData.values(hyperlink,textFormatRuns.format.link.uri)").
		IncludeGridData(true).
		Context(ctx).Do()
	if err != nil {
		return "", err
	}
	for _, sh := range resp.Sheets {
		for _, d := range sh.Data {
			for _, rd := range d.RowData {
				for _, v := range rd.Values {
					for _, run := range v.TextFormatRuns {
						if run.Format != nil && run.Format.Link != nil && run.Format.Link.Uri != "" {
							return run.Format.Link.Uri, nil
						}
					}
					if v.Hyperlink != "" {
						return v.Hyperlink, nil
					}
				}
			}
		}
	}
	return "", nil
}
```

(`fmt` is already imported in `client.go`; `columnLetter` and `Column` are in the same package.)

- [ ] **Step 7: Build + test to verify everything compiles and passes**

Run: `go build ./... && go test ./internal/sheet/`
Expected: build succeeds, PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/sheet/sheet.go internal/sheet/client.go internal/sheet/sheet_test.go
git commit -m "feat(sheet): read cell hyperlink + parse Discord ID from it"
```

---

### Task 6: Approve no longer mutates roles

**Files:**
- Modify: `internal/handlers/review_approve.go:65-74`

**Interfaces:**
- Consumes: nothing new. After this task, Approve sets `GovDAO pending` + DM + GovDAO ping only.

- [ ] **Step 1: Remove the role-mutation block**

In `internal/handlers/review_approve.go`, delete these ten lines (currently 65–74), so the function goes straight from the `sheet.UpdateFields` block to `message, err := tpl.Approve()`:

```go
	if err := s.GuildMemberRoleAdd(cfg.GuildID, candidateID, cfg.ValidatorRoleID); err != nil {
		log.Printf("approve: add validator role for %s: %v", candidateID, err)
		editEphemeral(s, i.Interaction, "Updated the tracker, but could not grant the Testnet Validator role. Please assign it manually.")
		return
	}
	if err := s.GuildMemberRoleRemove(cfg.GuildID, candidateID, cfg.CandidateRoleID); err != nil {
		log.Printf("approve: remove candidate role for %s: %v", candidateID, err)
		editEphemeral(s, i.Interaction, "Granted the Testnet Validator role, but could not remove Testnet Validator Candidate. Please remove it manually.")
		return
	}
```

`candidateID` is still used by `sendDM`, and `valoperLink` by the GovDAO message, so no variables become unused.

- [ ] **Step 2: Build + vet to confirm no unused imports/variables**

Run: `go build ./... && go vet ./...`
Expected: both succeed. (If `go vet` flags the `cfg.ValidatorRoleID` / `cfg.CandidateRoleID` fields as unused — it will not, they are still referenced by the poller in Task 7 and config validation — no action needed.)

- [ ] **Step 3: Commit**

```bash
git add internal/handlers/review_approve.go
git commit -m "feat(approve): stop granting the validator role at reviewer approval"
```

---

### Task 7: The activation poller + main wiring

**Files:**
- Create: `internal/handlers/activation_poller.go`
- Modify: `main.go`
- Modify: `MANUAL_TESTING.md`

**Interfaces:**
- Consumes: `valoper.ParseRender` (Task 1), `(*valoper.Client).ValidatorSet` (Task 2), `cfg.ValidatorPollEvery` (Task 3), `tpl.Activated` (Task 4), `api.CellLink` + `sheet.DiscordIDFromUserURL` (Task 5), and existing `sheet.ReadCandidates`, `sheet.UpdateFields`, `sendDM`.
- Produces: `handlers.StartActivationPoller(ctx, s, cfg, api, tpl, chain, every)` — launches the background goroutine.

- [ ] **Step 1: Create the poller**

Create `internal/handlers/activation_poller.go`:

```go
package handlers

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"onboardingbot/internal/config"
	"onboardingbot/internal/sheet"
	"onboardingbot/internal/templates"
	"onboardingbot/internal/valoper"
)

// chainClient is the subset of *valoper.Client the poller needs: render a realm
// (to derive a signing address) and read the active validator set.
type chainClient interface {
	Render(ctx context.Context, realmPath string) (string, error)
	ValidatorSet(ctx context.Context) (map[string]struct{}, error)
}

// StartActivationPoller launches a goroutine that, every `every`, promotes
// "GovDAO pending" candidates whose validator has joined the active set:
// it writes "GovDAO submitted", grants the Testnet Validator role (removing the
// Candidate role), and DMs the candidate. Runs until ctx is cancelled.
func StartActivationPoller(ctx context.Context, s *discordgo.Session, cfg *config.Config, api sheet.API, tpl *templates.Templates, chain chainClient, every time.Duration) {
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runActivationTick(ctx, s, cfg, api, tpl, chain)
			}
		}
	}()
}

func runActivationTick(ctx context.Context, s *discordgo.Session, cfg *config.Config, api sheet.API, tpl *templates.Templates, chain chainClient) {
	set, err := chain.ValidatorSet(ctx)
	if err != nil {
		log.Printf("activation: fetch validator set: %v", err)
		return
	}
	rows, err := sheet.ReadCandidates(ctx, api, cfg.SheetID, cfg.SheetName)
	if err != nil {
		log.Printf("activation: read candidates: %v", err)
		return
	}
	for _, r := range rows {
		if strings.TrimSpace(r.Status) != sheet.StatusGovDAOPending || strings.TrimSpace(r.OperatorAddress) == "" {
			continue
		}
		raw, err := chain.Render(ctx, valoper.RealmPath+":"+strings.TrimSpace(r.OperatorAddress))
		if err != nil {
			log.Printf("activation: render row %d: %v", r.Row, err)
			continue
		}
		_, _, signingAddr, _, err := valoper.ParseRender(raw)
		if err != nil {
			log.Printf("activation: parse row %d: %v", r.Row, err)
			continue
		}
		if signingAddr == "" {
			continue
		}
		if _, active := set[signingAddr]; !active {
			continue
		}
		activateCandidate(ctx, s, cfg, api, tpl, r)
	}
}

func activateCandidate(ctx context.Context, s *discordgo.Session, cfg *config.Config, api sheet.API, tpl *templates.Templates, r sheet.TrackerRow) {
	link, err := api.CellLink(ctx, cfg.SheetID, cfg.SheetName, r.Row, int(sheet.ColumnDiscord))
	if err != nil {
		log.Printf("activation: read Discord link row %d: %v", r.Row, err)
		return
	}
	candidateID, ok := sheet.DiscordIDFromUserURL(link)
	if !ok {
		log.Printf("activation: row %d has no resolvable Discord ID; grant the role manually", r.Row)
		return
	}
	// Sheet write before any role mutation (invariant).
	if err := sheet.UpdateFields(ctx, api, cfg.SheetID, cfg.SheetName, r.Row, map[sheet.Column]string{
		sheet.ColumnStatus: sheet.StatusGovDAOSubmitted,
	}); err != nil {
		log.Printf("activation: set GovDAO submitted row %d: %v", r.Row, err)
		return
	}
	if err := s.GuildMemberRoleAdd(cfg.GuildID, candidateID, cfg.ValidatorRoleID); err != nil {
		log.Printf("activation: add validator role for %s (row %d): %v — grant manually", candidateID, r.Row, err)
		return
	}
	if err := s.GuildMemberRoleRemove(cfg.GuildID, candidateID, cfg.CandidateRoleID); err != nil {
		log.Printf("activation: remove candidate role for %s (row %d): %v — remove manually", candidateID, r.Row, err)
	}
	msg, err := tpl.Activated()
	if err != nil {
		log.Printf("activation: render activated template (row %d): %v", r.Row, err)
		return
	}
	if err := sendDM(s, candidateID, msg); err != nil {
		log.Printf("activation: DM candidate %s (row %d) failed (DMs may be closed): %v", candidateID, r.Row, err)
	}
	log.Printf("activation: OK row %d user=%s moniker=%q", r.Row, candidateID, r.Moniker)
}
```

- [ ] **Step 2: Wire the poller into `main.go`**

In `main.go`, the renderer is already built as `renderer := valoper.NewClient(cfg.GnoRPCEndpoint)`. After the `handlers.RegisterSubmit(...)` block and before the "bot is running" log line, add:

```go
	pollCtx, cancelPoll := context.WithCancel(context.Background())
	defer cancelPoll()
	handlers.StartActivationPoller(pollCtx, s, cfg, sheetsClient, tpl, renderer, cfg.ValidatorPollEvery)
	log.Printf("activation poller running every %s", cfg.ValidatorPollEvery)
```

(`context` and `log` are already imported in `main.go`. `renderer` is a `*valoper.Client`, which satisfies `chainClient` via its `Render` and `ValidatorSet` methods.)

- [ ] **Step 3: Build + vet + full test suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: build succeeds, vet clean, all tests PASS.

- [ ] **Step 4: Add the manual-test steps**

Append to `MANUAL_TESTING.md` a new section:

```markdown
## GovDAO on-chain role activation

1. **Approve grants no role.** Run **Approve** on a submission. Expect: tracker row → `GovDAO pending`, candidate DM received, GovDAO contact pinged, and the candidate's roles **unchanged** (still `Testnet Validator Candidate`, no `Testnet Validator`).
2. **Poller activates on-chain membership.** With a candidate whose valoper's signing address is in `<gno_rpc_endpoint>/validators`, wait one `validator_poll_interval`. Expect: tracker row → `GovDAO submitted`, the `Testnet Validator` role granted and `Testnet Validator Candidate` removed, and the `activated` DM received.
3. **No double-processing.** On the next tick, the now-`GovDAO submitted` row is left untouched (no duplicate DM/role calls in the logs).
4. **Unresolvable Discord ID.** For a `GovDAO pending` row whose column-B cell has no `https://discord.com/users/<id>` hyperlink, expect a single log line asking to grant the role manually, and no status change.
```

- [ ] **Step 5: Commit**

```bash
git add internal/handlers/activation_poller.go main.go MANUAL_TESTING.md
git commit -m "feat(handlers): add on-chain validator activation poller"
```

---

## Self-Review notes

- **Spec coverage:** Approve-no-role (Task 6), ParseRender signing address (Task 1), ValidatorSet (Task 2), Discord-ID resolution = CellLink + DiscordIDFromUserURL (Task 5), poller (Task 7), config interval (Task 3), template reword + activated (Task 4), status `GovDAO submitted` reuse (Task 7 `activateCandidate`), edge cases (empty signing addr / unknown address / unresolvable ID — handled by `continue`/skip+log in Task 7). All spec sections map to a task.
- **Type consistency:** `ParseRender` is 5-value in Tasks 1, 7 and both valoper tests. `ValidatorSet` returns `map[string]struct{}` in Tasks 2, 7. `CellLink(ctx, id, name, row, col int)` identical in interface (Task 5), real impl (Task 5), fake (Task 5), and caller (Task 7). `StartActivationPoller` signature matches its `main.go` call. `ValidatorPollEvery` is `time.Duration` in Tasks 3 and 7.
- **No placeholders:** every code step contains the full code.
