# gno-onboarding-bot

Go Discord bot that automates the test13 validator onboarding lifecycle: candidate intake, evidence submission, reviewer decisions, and the GovDAO handoff. A Google Sheet is the only persistent store — the bot keeps no local database.

## Commands

The bot registers six Discord slash commands (see `internal/handlers`):

- `/candidate` — candidate intake
- `/submit-request` — evidence submission (one Sheet row per call, including resubmissions)
- request missing info, ask-to-retry, escalate-to-call, approve — reviewer decisions in `#validator-review`

## Setup

1. Copy `config.example.yaml` to `config.yaml` and fill in the Discord token, guild/channel/role IDs, GovDAO contact, Google Sheet ID/name, and review SLA.
2. Place a Google service account key at `service-account.json` (path configurable via `google_credentials_file`).
3. Edit `templates.yaml` to adjust candidate/reviewer-facing wording — it's loaded at startup, no rebuild needed, but the bot must be restarted to pick up changes.

`config.yaml` and `service-account.json` are gitignored and must never be logged.

## Build & run

```bash
go build ./...
go vet ./...
go test ./...
go run . -config config.yaml
```

## Layout

- `main.go` — loads `config.yaml` and `templates.yaml`, connects to Google Sheets, opens the Discord session, registers the six commands.
- `internal/config` — `config.yaml` loader/validator.
- `internal/templates` — loads `templates.yaml` and renders it as Go `text/template`.
- `internal/forms` — modal input validation helpers.
- `internal/rowref` — encodes a Sheet row number + Discord candidate ID into short strings threaded through embed footers and modal `custom_id`s.
- `internal/sheet` — the 12-column Sheet schema and the Google Sheets API client.
- `internal/notify` — builds/parses the `#validator-review` notification embed.
- `internal/handlers` — the six command handlers plus shared Discord glue (defer/edit ephemeral responses, DM-with-fallback, role checks, command permission restriction).

## Testing

Pure-logic packages have unit tests (`go test ./...`). Discord-session-dependent handler code has none — it needs a live Discord session and Google Sheet, so it's verified manually via [MANUAL_TESTING.md](MANUAL_TESTING.md) after any change to `internal/handlers`.

See [CLAUDE.md](CLAUDE.md) for the invariants the codebase relies on (Sheet-before-role-mutation ordering, one row per submission, closed-DM fallback, permissions v2 enforcement).
