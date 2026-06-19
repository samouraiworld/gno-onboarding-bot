# Manual testing checklist

## Prerequisites

1. A Discord application + bot user (https://discord.com/developers/applications), invited to the test server with the `applications.commands` and `bot` scopes, and `Manage Roles` + `Send Messages` permissions.
2. A Google Cloud service account with the Sheets API enabled, its JSON key saved as `bot/service-account.json` (or the path set in `config.yaml`'s `google_credentials_file`).
3. A Google Sheet shared with the service account's email (Editor access), with exactly these 12 headers in row 1, in this order:
   `Candidate | Discord | Status | Challenge submitted | Reviewers | Missing criteria | Decision date | Valoper link | GovDAO status | Moniker & validator address | Introduction | Review message link`
4. On the test Discord server: a `Testnet Validator Candidate` role, a `Testnet Validator` role, an `Onboarding Reviewer` role (assigned to your test reviewer account), a `#general-chat`-equivalent channel, a `#testnet-onboarding`-equivalent channel, and a new private `#validator-review` channel visible only to `Onboarding Reviewer`.
5. `bot/config.yaml` filled in with all the above IDs (copy `config.example.yaml` and fill it in — this file is gitignored).

## Running the bot

```bash
cd bot
go run . -config config.yaml
```

Expected log output: `bot is running, press Ctrl+C to exit`, with no errors during command registration.

## Checklist

- [ ] `/candidate-testnet` in the general-chat channel: grants `Testnet Validator Candidate`, creates a new Sheet row (`Status` = `Candidate`), and sends a DM with the exact "Reply to someone asking to become a validator" wording from `Shared.md`.
- [ ] `/candidate-testnet` is not visible/usable in any other channel.
- [ ] `/candidate-testnet` run again by the same member: ephemeral notice, no second role grant or Sheet row.
- [ ] `/submit-request` in the onboarding channel (as the candidate): opens a 3-field modal; submitting it creates a new Sheet row (`Status` = `Challenge in progress`, all fields filled), posts a notification embed in `#validator-review` pinging `Onboarding Reviewer`, and DMs the candidate the exact "Acknowledge a submission" wording with the configured `review_sla`.
- [ ] `/submit-request` is not visible/usable without the `Testnet Validator Candidate` role, or outside the onboarding channel.
- [ ] `Request missing info` (right-click the notification in `#validator-review`): modal collects a multi-line list; submitting it DMs the candidate the exact "Request missing information" wording with the submitted items as bullets, and updates the Sheet row's `Status` (`Needs retry`), `Missing criteria`, `Decision date`, `Reviewers`.
- [ ] `Ask to retry`: same shape, with the exact "Ask a candidate to retry" wording and two modal fields.
- [ ] `Escalate to call`: same shape, with the exact "Escalate an unclear result to a technical call" wording; confirm the Sheet's `Status` is unchanged and only `Reviewers` is updated.
- [ ] `Approve`: no modal; DMs the candidate the exact "Approve a candidate" wording, grants `Testnet Validator`, removes `Testnet Validator Candidate`, sets the Sheet row's `Status` to `GovDAO pending`, and posts a message in `#validator-review` tagging the configured GovDAO contact with the candidate's Valoper link.
- [ ] None of the four reviewer commands are visible/usable by a member without the `Onboarding Reviewer` role, or outside `#validator-review`.
- [ ] Resubmission: after `Needs retry`, running `/submit-request` again appends a brand-new Sheet row (the first row is left untouched) and posts a new notification referencing the new row.
- [ ] Closed DMs: temporarily block DMs from server members on a test account, then run `/candidate-testnet` and `/submit-request` as that account — confirm the ephemeral fallback shows the full message text. Then have a reviewer run `Approve` against that candidate — confirm the reviewer sees the "could not DM the candidate" fallback ephemeral message instead.
- [ ] Deleted/invalid notification: delete a notification message in `#validator-review`, then try right-clicking a different, unrelated message in that channel with one of the four reviewer commands — confirm an ephemeral error rather than a crash or an orphaned DM.
- [ ] Empty required field: submit any modal leaving a required field blank — confirm an ephemeral error naming the missing field, and that no DM or Sheet write happens.
- [ ] Sheets failure: temporarily revoke the service account's access to the Sheet, then run `/candidate-testnet` — confirm an ephemeral error and that no role is granted (Sheet write failure must block the role change, per the design's error-handling rule).
