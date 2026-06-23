# Manual testing checklist

## Prerequisites

1. A Discord application + bot user (https://discord.com/developers/applications), invited to the test server with the `applications.commands` and `bot` scopes, and `Manage Roles` + `Send Messages` permissions.
2. A Google Cloud service account with the Sheets API enabled, its JSON key saved as `service-account.json` (or the path set in `config.yaml`'s `google_credentials_file`).
3. A Google Sheet shared with the service account's email (Editor access), with exactly these 13 headers in row 1, in this order:
   `Candidate | Discord | Status | Challenge submitted | Reviewers | Missing criteria | Decision date | Valoper link | GovDAO status | Moniker | Operator address | Introduction | Review message link`
4. On the test Discord server: a `Testnet Validator Candidate` role, a `Testnet Validator` role, an `Onboarding Reviewer` role (assigned to your test reviewer account), a `#general-chat`-equivalent channel, a `#testnet-onboarding`-equivalent channel, and a new private `#validator-review` channel visible only to `Onboarding Reviewer`.
5. `config.yaml` filled in with all the above IDs (copy `config.example.yaml` and fill it in — this file is gitignored).
6. `gno_rpc_endpoint` set to a reachable test13 RPC URL (e.g. `https://rpc.test13.testnets.gno.land`), and `gnoweb_base_url` set (e.g. `https://gnoweb.test-13.gnoland.network`), in `config.yaml`.

## Running the bot

```bash
go run . -config config.yaml
```

Expected log output: `bot is running, press Ctrl+C to exit`, with no errors during command registration.

## Restricting commands by channel/role

The bot does not do this itself (Discord rejects bot tokens on the permissions endpoint). After the first run registers the commands, go configure them manually per the README's "Restricting commands to a channel/role" section before running the checklist below — otherwise every command is usable everywhere by everyone, and the "not visible/usable" checks will fail for the wrong reason.

## Checklist

- [ ] `/candidate-testnet` in the general-chat channel: grants `Testnet Validator Candidate` and posts the exact "Reply to someone asking to become a validator" wording from `Shared.md` in `#testnet-onboarding`, pinging the candidate. No Sheet row is created at intake; the candidate's row is created later by `/submit-request`.
- [ ] `/candidate-testnet` is not visible/usable in any other channel.
- [ ] `/candidate-testnet` run again by the same member: ephemeral notice, no second role grant.
- [ ] `/submit-request` in the onboarding channel (as the candidate): opens a single-field modal asking for the operator address (`g1...`). Pasting a **registered** valoper's address creates a new Sheet row (`Status` = `Challenge in progress`; `Moniker` (J), `Operator address` (K) parsed from the realm; `Valoper link` (H) = the gnoweb profile URL; `Introduction` (L) = the profile description), posts a notification embed in `#validator-review` (Moniker, Operator address, clickable Valoper link, truncated Introduction) pinging `Onboarding Reviewer`, and posts the exact "Acknowledge a submission" wording in `#testnet-onboarding`, pinging the candidate.
- [ ] `/submit-request` with a non-address / junk string: ephemeral "not a valid operator address"; no Sheet row.
- [ ] `/submit-request` with a well-formed but **unregistered** `g1` address: ephemeral "register on r/gnops/valopers first"; no Sheet row.
- [ ] `/submit-request` with `gno_rpc_endpoint` pointed at an unreachable URL: ephemeral "could not reach the chain"; no Sheet row.
- [ ] Confirm the live qrender response shape matches `internal/valoper/client.go` (`result.response.ResponseBase.Data`); if a registered address wrongly yields "could not reach the chain" / "could not read your valoper profile", capture the raw RPC response and adjust the struct tags + `client_test.go`.
- [ ] `/submit-request` is not visible/usable without the `Testnet Validator Candidate` role, or outside the onboarding channel.
- [ ] `Decline` (right-click the notification in `#validator-review`): the modal collects a multi-line criteria list and shows the "no confidential info" placeholder warning; submitting it removes the `Testnet Validator Candidate` role, posts the exact decline wording in the general chat channel (pinging the candidate, criteria as bullets), and updates the Sheet row's `Status` (`Declined`), `Missing criteria`, `Decision date`, `Reviewers`.
- [ ] `Approve`: no modal; posts the exact "Approve a candidate" wording in `#testnet-onboarding` (pinging the candidate), grants `Testnet Validator`, removes `Testnet Validator Candidate`, sets the Sheet row's `Status` to `GovDAO pending`, and posts a message in `#validator-review` tagging the configured GovDAO contact with the candidate's Valoper link.
- [ ] Neither reviewer command (`Approve`, `Decline`) is visible/usable by a member without the `Onboarding Reviewer` role, or outside `#validator-review`.
- [ ] Resubmission: after a `Declined` row, the candidate re-runs `/candidate-testnet` (regaining the role) then `/submit-request`; confirm a brand-new Sheet row is appended (the first row is left untouched) and a new notification references the new row. (A row manually set to `Needs retry` reopens the same way, without needing `/candidate-testnet` since that status keeps the role.)
- [ ] Channel delivery: run `/candidate-testnet` and `/submit-request` and confirm the welcome and acknowledgement appear in `#testnet-onboarding`, pinging the candidate, with the full message text (no DM is sent). Confirm `#testnet-onboarding` is readable by both the Candidate and Validator roles.
- [ ] Decline delivery + approve visibility: run `Decline` against a candidate and confirm the `Testnet Validator Candidate` role is removed, the decline post appears in the general chat channel pinging the now-roleless candidate, and they can re-apply with `/candidate-testnet`. Run `Approve` against another candidate and confirm the now-`Testnet Validator` account still sees the approval post in `#testnet-onboarding`.
- [ ] Deleted/invalid notification: delete a notification message in `#validator-review`, then right-click a different, unrelated message in that channel with either reviewer command. Confirm an ephemeral error rather than a crash or an orphaned post.
- [ ] Empty required field: submit a modal (Decline, or the `/submit-request` modal) leaving a required field blank. Confirm an ephemeral error naming the missing field, and that no post or Sheet write happens.
- [ ] Sheets failure: temporarily revoke the service account's access to the Sheet, then run `Approve` against a pending candidate — confirm an ephemeral "could not update the tracker" error and that neither role changes (no `Testnet Validator` granted, `Testnet Validator Candidate` not removed). Sheet write failure must block the role change, per the design's error-handling rule. (`/candidate-testnet` no longer writes the Sheet, so it cannot exercise this rule.)

## Harvest checklist

Prereq: enable the privileged **Message Content** intent and give the bot **Read Message History** in the three channels. Seed a few candidate rows (with an operator address in column K for some) and post candidate/reviewer messages, including one with a fake seed phrase or `192.168.x.x` for redaction.

- [ ] Startup: a fresh sheet gets the N-Y assessment headers, checkboxes on P-V, and a `{source}-evidence` tab.
- [ ] `/harvest` (reviewer, in `#validator-review`): replies ephemerally with `harvest.json` + a count (incl. "duplicate rows collapsed" / "already-validated"). The `-evidence` tab fills, and Red flags (W) / Engagement (X) fill for active candidates.
- [ ] Redaction: the seeded secret shows as `[REDACTED:...]` in `harvest.json` and the evidence tab; the Red flags cell names the kind.
- [ ] Valoper: a candidate with operator address in column K shows `signals.valoper_state: "found"`; one without shows `not_found`.
- [ ] Duplicate handles: two rows for one `@handle` → only the latest is evaluated; the older reads `Duplicate of row N` with its assessment cells cleared.
- [ ] Validated rows skipped: set a row's Status to `Approved`/`GovDAO pending`/`GovDAO submitted` → absent from `harvest.json`, columns untouched.
- [ ] Run the `competency-digest` skill on `harvest.json` → `digest.json`, then `/harvest-import` it: Readiness (N), Summary (O), criterion checkboxes (P-V), Evidence links (Y) fill; the human columns (A-M) are untouched.
- [ ] Curation: set a reviewed candidate's Status to `Approved`/`GovDAO pending` → it appears in PR #4's `-approved` tab (no separate Selected column).
- [ ] `/harvest-import` with a malformed file → ephemeral error, no writes.
