# End-of-window harvest and competency digest

A reviewer-triggered pass that runs once the onboarding window closes. It pulls
each candidate's Discord activity, distils it against the published acceptance
criteria, and writes sortable assessment columns + a per-criterion checklist back
into the tracker so a human can rank candidates and pick the good validators.

The tool **collects and maps evidence to criteria**. It does not decide. The
plan (`Shared.md`) requires every decision to rest on the published criteria
alone and two reviewers to agree, so the digest never produces an approve/reject
verdict, only an assessment the reviewers act on.

## Layout (builds on PR #4's schema)

The intake columns A-M are PR #4's (Candidate … Moniker J, Operator address K …
Review message link M). The harvest appends its assessment columns N-Z to the
same source tab:

| col | name | written by | content |
| --- | ---- | ---------- | ------- |
| N | Readiness | `/harvest-import` | band + score, e.g. `High (6/7)`; `Duplicate of row N` on a collapsed row |
| O | Summary | `/harvest-import` | 1-3 sentence digest |
| P-V | Setup … Safety | `/harvest-import` | seven criterion **checkboxes**, ticked when `found` |
| W | Red flags | `/harvest` | deterministic secret-leak kinds, or empty |
| X | Engagement | `/harvest` | e.g. `12 msgs, 5 days, last 2026-06-10` |
| Y | Evidence links | `/harvest-import` | the skill's curated key permalinks |
| Z | Selected | **human only** | checkbox; tick it to add the candidate to the `-selected` tab |

The bot **refreshes its assessment columns** every run and **never writes the
human cells**: the existing human columns (Reviewers, Missing criteria, Decision
date, GovDAO status) and the `Selected` checkbox (Z), which it only creates.

## Tabs

`EnsureHarvestLayout` provisions these on startup (alongside PR #4's `Ensure`,
`EnsureApprovedView`, dropdown/colors/freeze):

- **source tab** (e.g. `Candidates`) — the review interface: A-M intake + N-Z
  assessment, status dropdown/colors, frozen header.
- **`{source}-approved`** — PR #4's live view of GovDAO-progressing rows.
- **`{source}-selected`** — a live `FILTER` of rows whose `Selected` box is
  ticked (built like `EnsureApprovedView`, locale-aware separator via
  `SetFormula`). The final good-validators list; tick `Selected` and rows appear.
- **`{source}-evidence`** — the raw harvested messages, one per row (audit trail).

## Flow

1. **`/harvest`** (reviewer-only, re-runnable). Reads the intake rows, skips
   already-validated candidates (`Approved` / `GovDAO pending` / `GovDAO
   submitted`), collapses duplicate handles (keeps the latest row, marks older
   ones `Duplicate of row N`), reads `#general` + `#testnet-onboarding` +
   `#validator-review`, redacts secrets, writes the evidence tab + Red flags /
   Engagement, and replies with `harvest.json` as an **ephemeral** attachment.
2. **The `competency-digest` skill** reads `harvest.json` and writes
   `digest.json` (summary, per-criterion state, readiness band).
3. **`/harvest-import`** (reviewer-only, takes `digest.json`) refreshes Readiness,
   Summary, Evidence links, and the seven criterion checkboxes. `Selected` is
   never touched.

`harvest.json` is ephemeral so leaked-secret context is never posted to a
channel. Both files move through Discord attachments, so the bot and the skill
need not share a filesystem.

## Acceptance criteria (the rubric)

The seven `Shared.md` criteria, each scored `found` / `not_found` /
`needs_human_check`, defaulting to neutral when a candidate has no messages.

| key | criterion | judged by |
| --- | --------- | --------- |
| `setup` | Followed the published setup successfully | skill |
| `sync` | Evidence the node is connected to test13 and synced | skill |
| `tx` | Required transaction is valid and publicly verifiable | skill |
| `valoper` | Valoper registration complete and accurate | **bot (operator address)** |
| `ops` | Grasp of keys, backups, monitoring, upgrades, incident | skill |
| `comms` | Communicates clearly enough to coordinate operations | skill |
| `safety` | Never exposed a secret or mishandled a key | regex + skill |

`valoper` is the bot's, not the skill's: PR #4 verifies the operator address
on-chain (ABCI `vm/qrender`) at `/submit-request` and writes it to column K only
on success, so the harvest marks `valoper` = `found` when column K is populated.
The verdict rides in `signals.valoper_state`; the skill copies it verbatim.

`safety`: the secret-leak regex is the hard deterministic signal (Red flags
column); the skill additionally judges described unsafe handling.

## `harvest.json` / `digest.json`

`harvest.json` (bot → skill): per candidate, the `submitted` fields, deterministic
`signals` (counts, links, `valoper_state`, `secret_leak*`), redacted `messages`,
and `reviewer_context`. Secrets in any text are replaced with `[REDACTED:<kind>]`.

`digest.json` (skill → bot): per candidate `row`, `readiness`, `readiness_score`,
`summary`, the seven `criteria` states, and `evidence_links`. Only `row` locates
the Sheet row.

## Prerequisites

- The privileged **Message Content** intent (+ Guild Messages) in the Discord
  portal, and the bot's role with **Read Message History** in the three channels.
- Optional config: `harvest_since` (RFC3339, default all history),
  `harvest_max_messages` (per channel, default 2000).

## Operator runbook

1. Window closes. A reviewer runs `/harvest`; save the attached `harvest.json`.
2. Run the `competency-digest` skill: `harvest.json` → `digest.json` (review it).
3. A reviewer runs `/harvest-import` with `digest.json`.
4. On the source tab: sort by Readiness, read the criterion checkboxes (P-V),
   check Red flags, open Evidence links. Two reviewers decide, then tick
   `Selected`. The `-selected` tab is the resulting good-validators list.
