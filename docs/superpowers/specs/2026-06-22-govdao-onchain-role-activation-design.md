# GovDAO on-chain role activation

**Date:** 2026-06-22
**Status:** Design approved, pending implementation plan

## Problem

Today the `Testnet Validator` role is granted the moment a reviewer clicks **Approve**
in `#validator-review` ([review_approve.go](../../../internal/handlers/review_approve.go)).
At that point the candidate has only passed *reviewer* approval — the GovDAO has not yet
voted them into the active validator set. The role therefore overstates the candidate's
real status.

We want the role to change **only when the GovDAO has actually admitted the validator to
the active set**, which is observable on-chain: the validator's signing address appears in
`<gno_rpc_endpoint>/validators`.

## The address model (the crux)

There are two distinct gno addresses per validator, and conflating them is the root
confusion this design resolves:

- **Operator address** — the account that registered the valoper in
  `r/gnops/valopers`. This is what the candidate pastes into `/submit-request`, what the
  bot stores in **Sheet column K**, the key into the valopers realm, the basis of the
  profile link (column H), and the deduplication key
  ([FindByOperatorAddress](../../../internal/sheet/sheet.go)).
- **Signing address** — the consensus address derived from the validator's signing key.
  This is what appears in `<rpc>/validators`.

These address spaces are **disjoint**. Verified on test13:

- `qrender gno.land/r/gnops/valopers:<operatorAddr>` → full profile, which contains a
  `- Signing Address: g1...` line.
- `qrender gno.land/r/gnops/valopers:<signingAddr>` → `unknown address`.

So the valopers realm is keyed by operator address, and the **only** way to obtain a
validator's signing address is to render its profile by operator address.

**Consequence:** the operator address (column K) stays exactly as it is. The signing
address is derived on demand from it — never stored. No new Sheet column, no schema
change, no migration.

## Goals

1. Reviewer **Approve** sets status to `GovDAO pending` and does **not** mutate roles.
2. A periodic on-chain check grants the `Testnet Validator` role (and removes
   `Testnet Validator Candidate`) once the candidate's signing address is in the active
   validator set, then advances status to `GovDAO submitted`.
3. No change to the Sheet schema.

## Non-goals

- Reacting in real time to GovDAO events (we poll, we don't subscribe).
- Handling validators that *leave* the active set (role removal on exit) — out of scope.
- Storing the signing address for human display.

## Design

### 1. Approve no longer touches roles

In [review_approve.go](../../../internal/handlers/review_approve.go), remove the
`GuildMemberRoleAdd(ValidatorRoleID)` and `GuildMemberRoleRemove(CandidateRoleID)` calls.
Approve keeps: status → `GovDAO pending`, decision date, reviewer name, the candidate DM,
and the GovDAO contact ping.

The `approve` template in [templates.yaml](../../../templates.yaml) currently says "We
assigned you the `Testnet Validator` role" — reword it to "reviewers approved your
application; you'll get the role once the GovDAO admits your validator to the active set."
Wording only, no rebuild.

### 2. `ParseRender` exposes the signing address

[valoper.ParseRender](../../../internal/valoper/valoper.go) returns
`(moniker, operatorAddr, description, err)` today. Add a fourth value `signingAddr`,
extracted from the `- Signing Address:` line (marker confirmed stable across profiles).
The existing `submit-request` caller ignores the new return — submit is unchanged.

### 3. Validator-set client method

Add `valoper.Client.ValidatorSet(ctx) (map[string]struct{}, error)` that calls the gno RPC
`validators` JSON-RPC method and returns the set of active **signing addresses** for O(1)
lookup. Reuses the existing `gno_rpc_endpoint`.

Edge case: the `validators` endpoint may paginate (default page size). The method must read
all pages (or document the cap) so large sets aren't silently truncated.

### 4. Resolving the candidate's Discord ID

To grant/remove a role and DM the candidate, the poller needs the candidate's **numeric
Discord user ID**. The Sheet does not store it as a plain value: column B holds
`@username` text, and the ID lives only inside that cell's **hyperlink**, written by
[submit.go](../../../internal/handlers/submit.go)'s `SetLinkedText` call as
`https://discord.com/users/<id>`. (The Approve handler doesn't have this problem — it reads
the ID from the review-notification embed, which the poller has no access to.)

So the poller reads the column-B hyperlink back and parses the ID out of it — the data is
already persisted, so this needs no schema change and no change to submit:

- Add `CellLink(ctx, spreadsheetID, sheetName string, row, col int) (string, error)` to the
  `sheet.API` interface, implemented on `GoogleSheetsClient` via `spreadsheets.get` with
  `IncludeGridData(true)` reading `textFormatRuns[0].format.link.uri`. The `fakeAPI` test
  double gains a trivial implementation.
- Add a pure helper `sheet.DiscordIDFromUserURL(url string) (id string, ok bool)` that
  extracts `<id>` from a `https://discord.com/users/<id>` URL (unit-tested).

### 5. The poller

A new goroutine (new file in `internal/handlers`, reusing `sendDM`, config, session),
started from [main.go](../../../main.go) after command registration, driven by a
`time.Ticker`.

Each tick:

1. Fetch the active validator set once via `ValidatorSet`.
2. Read the Sheet rows whose status is `GovDAO pending` (via the existing
   [ReadCandidates](../../../internal/sheet/sheet.go), filtering on `Status`).
3. For each such row:
   a. Take the operator address (column K).
   b. `qrender` the valopers profile and `ParseRender` it to get the **fresh** signing
      address (always re-derived — never cached/stored).
   c. If that signing address is **not** in the active set, leave the row untouched for the
      next tick.
   d. If it **is**, resolve the Discord ID via `CellLink`(column B) +
      `DiscordIDFromUserURL`. If the ID can't be resolved, log and skip (a reviewer can
      grant manually). Otherwise **activate** (below).

**Activation order (preserves "Sheet write before any role mutation"):**

1. Sheet: status → `GovDAO submitted`.
2. `GuildMemberRoleAdd(ValidatorRoleID)` then `GuildMemberRoleRemove(CandidateRoleID)`.
3. DM the candidate a new `activated` template ("your validator is now in the active set;
   the `Testnet Validator` role has been granted").

Idempotency: the poller only scans `GovDAO pending` rows, so an already-activated row
(now `GovDAO submitted`) is never reprocessed. A role-grant failure after the Sheet write
leaves the row at `GovDAO submitted`; it won't be retried automatically — logged for manual
follow-up, consistent with how the Approve handler reports partial failures today.

### 6. Configuration

Add `validator_poll_interval` to [config.go](../../../internal/config/config.go) and
`config.example.yaml`, parsed as a Go duration (e.g. `"5m"`). Default to 5 minutes when
unset or non-positive. Not a required field (the bot runs without it, using the default).

## Status lifecycle (after this change)

```text
Candidate
  → Challenge in progress        (submit-request)
  → GovDAO pending               (reviewer Approve — NO role change)
  → GovDAO submitted             (poller: signing address found in /validators
                                  — role granted here)
```

`Needs retry` / `Declined` branches are unchanged. `GovDAO submitted` is reused as the
terminal "in the active set" status (it already sorts above `GovDAO pending` in the
`-approved` view and `IsValidated` already covers it); no new status value is introduced.

## Error handling

- `ValidatorSet` RPC failure on a tick: log and skip the tick; try again next interval.
- `qrender`/`ParseRender` failure for one row: log and skip that row; other rows still
  process.
- Activation Sheet write failure: do **not** touch roles; log and retry next tick (the row
  is still `GovDAO pending`).
- Role mutation failure after the Sheet write: log for manual follow-up; the row is at
  `GovDAO submitted` and won't be retried.
- DM failure on activation: log; the role is already granted, so this is non-fatal (mirror
  the existing DM-fallback posture).

## Edge cases

- **Profile re-registered after submit:** because the signing address is always re-derived
  from column K at tick time, a candidate who reconfigures their valoper (new signing key)
  is matched against their current signing address, not a stale snapshot. This is the whole
  reason the value is derived, not stored.
- **No `- Signing Address:` line in a profile:** `ParseRender` returns an empty signing
  address; the poller treats the row as "not yet active" and skips it (it can never match
  the set). Logged.
- **Operator address removed from the realm between submit and poll:** `qrender` returns
  `unknown address`; treated as "not active", skipped, logged.

## Testing

Following the repo's split (pure logic unit-tested; Discord/session code manual):

- **Unit:** extend `valoper` tests — `ParseRender` extracts the signing address (and tolerates
  its absence); `ValidatorSet` parses a sample `validators` JSON response into the address
  set (with a fake HTTP transport). `sheet.DiscordIDFromUserURL` extracts the ID (and
  rejects non-matching URLs). Config: `validator_poll_interval` parsing + default.
- **Manual (`MANUAL_TESTING.md`):** Approve grants no role and only sets `GovDAO pending`;
  with a candidate whose signing address is in the set, the poller grants the role, removes
  the candidate role, sets `GovDAO submitted`, and DMs the `activated` message within one
  interval.

## Invariants preserved

- **Sheet write before any Discord role mutation:** activation writes `GovDAO submitted`
  before adding/removing roles.
- **No Sheet schema change:** signing address is derived, never stored; columns A–Y and the
  harvest layout are untouched.
- **Operator address (column K)** remains the dedup key and the valopers realm key.
