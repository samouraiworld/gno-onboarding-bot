# gno-onboarding-bot

Go Discord bot that automates the test13 validator onboarding lifecycle: candidate intake, evidence submission, reviewer decisions, and the GovDAO handoff. The Google Sheet is the only persistent store — the bot keeps no local database.

## Commands

```bash
go build ./...
go vet ./...
go test ./...
go run . -config config.yaml
```

## Layout

- `main.go` — loads `config.yaml` and `templates.yaml`, connects to Google Sheets, opens the Discord session, registers the commands.
- `internal/config` — `config.yaml` loader/validator.
- `internal/templates` — loads `templates.yaml` and renders it as Go `text/template`.
- `internal/forms` — modal input validation helpers (`SplitLines`, `MissingRequired`).
- `internal/rowref` — encodes a Sheet row number + Discord candidate ID into short strings threaded through embed footers and modal `custom_id`s, so a later reviewer action can find the right row with no database lookup.
- `internal/sheet` — the Sheet schema (15 intake columns A-O, plus harvest assessment columns P-AA incl. the seven criterion checkboxes R-X), `Ensure`/`EnsureApprovedView`/`EnsureHarvestLayout` provisioning, `API` interface (fake-tested), and the real `google.golang.org/api/sheets/v4` client adapter.
- `internal/notify` — builds/parses the `#validator-review` notification embed.
- `internal/valoper` — reads validator profiles from the on-chain `r/gnops/valopers` realm (ABCI `vm/qrender`) to auto-fill `/submit-request`.
- `internal/harvest` — pure logic for the end-of-window pass (attribution, signals, secret redaction, harvest/digest JSON contracts). No Discord or Sheet I/O; fully unit-tested.
- `internal/handlers` — the command handlers plus shared Discord glue (defer/edit ephemeral responses, channel-post-with-fallback via `sendCandidateMessage`, role checks).
- `skills/competency-digest` — the Claude skill that judges `harvest.json` into `digest.json`. See `docs/harvest.md`.

## Configuration

- `config.yaml` (gitignored) — Discord token, guild/channel/role IDs, GovDAO contact, Google service account path, Sheet ID/name, review SLA. Copy `config.example.yaml` to start.
- `service-account.json` (gitignored) — Google service account key.
- `templates.yaml` (committed) — the candidate/reviewer-facing message wording, copied verbatim from the team's `Shared.md` source-of-truth doc and parsed as Go `text/template` at startup. **Edit this file and restart the bot to change wording — no rebuild needed**, but also no hot-reload while running.

Never log the contents of `config.yaml` or `service-account.json`.

## Invariants to preserve

- **Sheet write before any Discord role mutation**, in every handler that does both. A Sheets failure must never leave a role changed without a tracker record.
- **One Sheet row per `/submit-request` call**, including resubmissions after `Needs retry` — never overwrite a previous attempt's row.
- **The bot never DMs.** Candidate-run commands reply ephemerally (welcome on `/candidate-testnet`, acknowledgement on `/submit-request`). Bot-initiated notices post to a channel and ping the candidate (`sendCandidateMessage` in handlers, `notifyCandidate` in the poller): approval and the activation notice go to the onboarding channel (readable by both the Candidate and Validator roles, since the candidate holds Candidate at approve and Validator at activation); decline goes to the dedicated decline channel (`decline_channel_id`), because it removes the candidate role and the now-roleless candidate keeps access to that channel but not onboarding. DMs to non-mutual or DM-disabled users are unreliable and can get the bot flagged.
- **Decline removes the candidate role**, then posts the reasons to the dedicated decline channel. The decline modal warns the reviewer to keep the text free of confidential info, since that channel is public. A candidate reapplies from scratch with `/candidate-testnet`.
- **Channel-post fallback**: if a reviewer-triggered post to the candidate fails (approve/decline), tell the reviewer so they can relay it manually, including the real message text. The poller logs its post failures.
- Command channel/role restriction is **not** done in code. Discord's command-permissions v2 endpoint (`PUT .../commands/{id}/permissions`) rejects bot tokens outright (`20001 Bots cannot use this endpoint`) — it requires an OAuth2 Bearer token from a guild admin, which this bot does not implement. Instead, a server admin configures per-command channel/role restrictions manually via Discord's *Server Settings → Integrations → (bot) → Command permissions* UI, once after each deploy where command IDs change. See the README's "Discord application setup" section.
- **The harvest writes only its assessment columns (P-AA), never the human cells (A-O).** `/harvest` refreshes Red flags (Y) and Engagement (Z); `/harvest-import` refreshes Readiness (P), Summary (Q), the seven criterion checkboxes (R-X), and Evidence links (AA). Assessment columns refresh each run. Curating good validators is done via the Status column + PR #4's `-approved` view, not a Selected column.
- **The harvest collapses duplicate handles** (keep latest row per handle via `harvest.NormalizeHandle`, mark older `Duplicate of row N`) and **skips already-validated rows** (`sheet.IsValidated`).
- **The harvest never stores a secret**: all harvested text passes through `harvest.Redact` before the evidence tab or `harvest.json`; the `harvest.json` reply is ephemeral.
- **The valoper criterion is the operator address (col K)**: PR-#4's `/submit-request` verifies it on-chain and writes K only on success, so the harvest treats K-present as `valoper: found` rather than re-fetching.

## Testing

Pure-logic packages (`config`, `templates`, `forms`, `rowref`, `sheet`, `notify`, plus `hasRole`/`modalValue` in `handlers`) have unit tests. Discord-session-dependent handler code has none — it needs a live Discord session and Google Sheet, so it's verified manually via `MANUAL_TESTING.md`. Run through that checklist after any change to `internal/handlers`.

## Background

This bot's design doc and implementation plan live locally (not committed) under `.claude/docs/superpowers/specs/2026-06-18-discord-onboarding-bot-design.md` and `.claude/docs/superpowers/plans/2026-06-18-discord-onboarding-bot.md`. The project originated inside `gno-validators-onboarding` (which also holds the onboarding process docs, including `Shared.md`, the wording source of truth) and was extracted into this standalone repo with full git history.

The switch from candidate DMs to channel posts (onboarding channel for welcome, acknowledgement, and approval; the decline channel for decline), plus removing the `Request missing info` and `Escalate to call` commands, is specced under `.claude/docs/superpowers/specs/2026-06-23-remove-dm-channel-post-design.md`.
