# gno-onboarding-bot

Go Discord bot that automates the test13 validator onboarding lifecycle: candidate intake, evidence submission, reviewer decisions, the GovDAO handoff, and the on-chain role activation once the validator joins the active set. A Google Sheet is the only persistent store — the bot keeps no local database.

## Commands

The bot registers these Discord commands (see `internal/handlers`):

- `/candidate-testnet` — candidate intake
- `/submit-request` — evidence submission (one Sheet row per call, including resubmissions)
- `Approve`, `Decline` — reviewer message-context decisions; right-click the submission notification in `#validator-review`. Approve notifies the candidate in `#testnet-onboarding`; Decline removes the candidate role and notifies in the decline channel
- `/harvest` and `/harvest-import` — the end-of-window competency pass (reviewers only); needs the privileged Message Content intent. See [docs/harvest.md](docs/harvest.md).

In addition, a background **activation poller** checks the chain's active validator set every `validator_poll_interval` and grants the `Testnet Validator` role (removing the candidate role, advancing the row to `GovDAO approved`, and posting an activation notice in `#testnet-onboarding`) once an approved candidate's validator is admitted to the active set by the GovDAO. The candidate's signing address is derived on the fly from their operator address via the `r/gnops/valopers` realm and matched against the node's `validators` RPC — no extra Sheet column is stored.

## Setup

1. Copy `config.example.yaml` to `config.yaml` and fill in the Discord token, guild/channel/role IDs, GovDAO contact, Google Sheet ID/name, and the gno RPC endpoint. `validator_poll_interval` (Go duration, e.g. `"5m"`) is optional and defaults to 5 minutes — it sets how often the activation poller checks the chain's active validator set.
2. Place a Google service account key at `service-account.json` (path configurable via `google_credentials_file`).
3. Edit `templates.yaml` to adjust candidate/reviewer-facing wording — it's loaded at startup, no rebuild needed, but the bot must be restarted to pick up changes.

`config.yaml` and `service-account.json` are gitignored and must never be logged.

The two services below (Google Sheets, Discord) need manual one-time setup beyond filling in `config.yaml` — neither failure mode is obvious from the bot's first error message, so follow these in full on a fresh setup.

### Google Cloud / Sheets setup

1. Create a project at the [Google Cloud Console](https://console.cloud.google.com/), then enable the **Google Sheets API** for it (*APIs & Services → Enable APIs and Services*).
2. *IAM & Admin → Service Accounts → Create Service Account*. No project-level role is needed (the "Permissions"/"Principals with access" step can be skipped) — access is granted directly on the Sheet in step 5.
3. Open the new service account → **Keys** tab → *Add Key → Create new key → JSON*. Rename the downloaded file to `service-account.json` and place it at the repo root (it's gitignored — Google's default filename, e.g. `project-name-xxxxxxx.json`, works too if you instead update `google_credentials_file` in `config.yaml` to match).
4. Create or open the Google Sheet:
   - copy its ID from the URL — `https://docs.google.com/spreadsheets/d/{SHEET_ID}/edit...` — into `sheet_id` (the `gid=` query param is the tab ID, unrelated, ignore it)
   - note the exact tab name into `sheet_name` (default `"Candidates"`)
   - set row 1 to exactly these 13 headers, in order:
     `Candidate | Discord | Status | Challenge submitted | Reviewers | Missing criteria | Decision date | Valoper link | GovDAO status | Moniker | Operator address | Introduction | Review message link`
5. **Share the Sheet** with the service account's `client_email` (the `client_email` field inside `service-account.json`) with **Editor** access. This is the step most likely to be missing — without it, `/candidate-testnet` fails with "Something went wrong recording your application", and the bot's logs (`docker compose logs -f`) will show the exact `googleapi:` error (e.g. `403 PERMISSION_DENIED` if unshared, `404` if `sheet_id` is wrong).

### Discord application setup

1. Create the application at the [Discord Developer Portal](https://discord.com/developers/applications).
2. **Bot** tab (left menu) → note the bot's **Username**. This is what shows up in the server's member list — it can differ from the application's display name, so don't search the member list for the application name.
3. **Installation** tab → under *Default Install Settings*:
   - keep **Installation pour une guilde** (Guild Install) checked
   - Scopes: add both `bot` and `applications.commands` — the latter is required for the slash and message-context commands to register
   - Permissions (integer **268520448**):

     | Permission | Why |
     | --- | --- |
     | View Channels | read the channels it posts to |
     | Send Messages | post in `#validator-review`, `#testnet-onboarding`, and the decline channel (candidate notifications) |
     | Embed Links | the `/submit-request` notification in `#validator-review` is an embed ([internal/notify](internal/notify)) |
     | Read Message History | resolve the submission embed targeted by the reviewer context-menu commands |
     | Manage Roles | grant `candidate_role_id` on intake; grant `validator_role_id` and remove the candidate role when the validator joins the active set (the activation poller) |

4. Copy the **install link** shown at the top of that page, open it in a browser, and explicitly pick the target server from the dropdown — easy to authorize into the wrong server if you manage several.
5. Confirm the bot actually joined: open the server's member list and look for the username from step 2.
6. **Role hierarchy** — *Server Settings → Roles* → drag the bot's role **above** `candidate_role_id` and `validator_role_id`. Discord silently rejects `Manage Roles` actions (`HTTP 403, 50013 Missing Permissions`) on any role positioned above the bot's own role in the list, even though the bot holds the `Manage Roles` permission.
7. **Private channel access** — if `validator_review_channel_id` (or any other command-restricted channel) is private, open that channel's own *Permissions* settings and explicitly add the bot's role with View Channel, Send Messages, Embed Links, and Read Message History. A server-wide permission grant doesn't apply to a channel that excludes the bot's role via its own overwrite.
8. **Candidate-facing channels** — the bot never DMs. Candidate-run commands (`/candidate-testnet`, `/submit-request`) reply ephemerally. Bot-initiated notices ping the candidate via a channel post: approval and the activation notice go to `onboarding_channel_id` (must be readable by **both** the Candidate and Validator roles, since the candidate holds Candidate at approve time and Validator at activation); decline removes the candidate role and posts to `decline_channel_id`, which must stay readable by the now-roleless candidate (e.g. visible to @everyone). The bot needs Send Messages in the onboarding and decline channels.

### Restricting commands to a channel/role

The bot does **not** restrict commands programmatically — Discord's command-permissions endpoint rejects bot tokens (`20001 Bots cannot use this endpoint`), it only accepts an OAuth2 Bearer token from a guild admin. So after each deploy where command IDs change (first deploy, or a command renamed), a server admin configures this manually:

1. *Server Settings → Integrations → (this bot)*.
2. For each command, set **Default Permissions** to disabled, then grant the relevant role:
   - `candidate-testnet` → role: none (anyone), channel: `general_chat_channel_id` only
   - `submit-request` → role: `candidate_role_id`, channel: `onboarding_channel_id` only
   - `Approve`, `Decline` (message context commands) → role: `reviewer_role_id`, channel: `validator_review_channel_id` only

## Build & run

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
- `internal/forms` — modal input validation helpers.
- `internal/rowref` — encodes a Sheet row number + Discord candidate ID into short strings threaded through embed footers and modal `custom_id`s.
- `internal/sheet` — the Sheet schema (15 intake columns A-O plus the harvest assessment columns P-AA) and the Google Sheets API client.
- `internal/notify` — builds/parses the `#validator-review` notification embed.
- `internal/handlers` — the command handlers plus shared Discord glue (defer/edit ephemeral responses, channel-post-with-fallback, role checks).

## Testing

Pure-logic packages have unit tests (`go test ./...`). Discord-session-dependent handler code has none — it needs a live Discord session and Google Sheet, so it's verified manually via [MANUAL_TESTING.md](MANUAL_TESTING.md) after any change to `internal/handlers`.

See [CLAUDE.md](CLAUDE.md) for the invariants the codebase relies on (Sheet-before-role-mutation ordering, one row per submission, channel-post fallback).
