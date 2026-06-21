---
name: competency-digest
description: Use when turning a harvest.json from the gno-onboarding-bot /harvest command into a digest.json. Judges each validator candidate against the seven published onboarding criteria from their Discord evidence, assigns a readiness band, and writes a short summary plus key evidence links. It surfaces evidence for human reviewers; it never decides approval.
---

# Competency digest

Turn one `harvest.json` into one `digest.json`. The bot's `/harvest-import`
command writes the result into the candidate tracker (Readiness, Summary,
Evidence links columns).

## Inputs and outputs

- **Input:** a `harvest.json` path (the schema is in `docs/harvest.md`). Each
  candidate has `submitted` fields, deterministic `signals`, their own
  `messages`, and `reviewer_context` (messages about them). Secrets are already
  redacted as `[REDACTED:<kind>]`; never try to reconstruct them.
- **Output:** write `digest.json` next to the input (schema below). Do not write
  to the Sheet; the bot does that.

## Process

1. Read `harvest.json`.
2. For each candidate, judge the seven criteria below using their `submitted`
   fields, `messages`, and `reviewer_context`. Use only evidence present in the
   file.
3. For each criterion assign exactly one state:
   - `found` — clear public evidence exists. Note the permalink.
   - `not_found` — no evidence either way.
   - `needs_human_check` — evidence exists but is ambiguous, conflicting, or
     needs verification a reader cannot do from text alone.
4. Write a 1-3 sentence `summary`: what is solid, what is missing. Plain, factual,
   no recommendation to approve or reject.
5. Set `evidence_links` to the 1-4 most useful permalinks (the submission, the
   sync proof, the tx, anything a reviewer should open first).
6. Compute `readiness_score` as `<count of found>/7` and `readiness` from the
   band table.
7. Validate against the rules below, then write `digest.json`.

## The seven criteria

| key       | satisfied (`found`) when the evidence shows...                          |
| --------- | ---------------------------------------------------------------------- |
| `setup`   | the candidate followed the published Test13 node setup                  |
| `sync`    | the node is connected to test13 and synced (e.g. block-height output)   |
| `tx`      | a required transaction that is valid and publicly verifiable           |
| `valoper` | the Valoper registration resolves and matches the submitted address    |
| `ops`     | grasp of keys, backups, monitoring, upgrades, incident response        |
| `comms`   | clear enough communication to coordinate operations                    |
| `safety`  | no secret was exposed and no described key mishandling                  |

`valoper`: **the bot already determined this.** Copy `signals.valoper_state`
verbatim into `criteria.valoper` (it is `found` / `not_found` /
`needs_human_check`); do not re-judge it. `signals.valoper_detail` explains why,
and is worth a phrase in the summary.

`safety`: if `signals.secret_leak` is true, set `safety` to `not_found` and say
so in the summary (the deterministic Red flags column already records the kind).
Also downgrade `safety` if a candidate *describes* unsafe key handling, even
without a raw leak.

## Neutral default

If a candidate has `message_count` 0 and no `submitted` evidence, set every
criterion to `not_found`, `readiness` to `Neutral`, `readiness_score` to `0/7`,
and `summary` to `No messages or submitted evidence found.` Absence of evidence
is never a failure; it is neutral.

## Readiness bands

| found criteria | readiness |
| -------------- | --------- |
| 6-7            | `High`    |
| 3-5            | `Medium`  |
| 1-2            | `Low`     |
| 0              | `Neutral` |

## Rules

- Judge on the published criteria only. Never use reputation, prior fame, or
  anything outside this file.
- Never output a verdict, score out of 100, or approve/reject. Two human
  reviewers decide.
- Never include a secret, even if you think you see one; it is already redacted.
- Keep `row` and `candidate` exactly as given for each input candidate. Do not
  rename, translate, or drop the `candidate`; `/harvest-import` matches on both
  and skips any row whose `candidate` no longer matches the tracker.

## Output schema

```json
{
  "generated_at": "<RFC3339 timestamp>",
  "candidates": [
    {
      "row": 2,
      "candidate": "alice",
      "readiness": "High",
      "readiness_score": "6/7",
      "summary": "Node set up and synced, valoper complete, clear ops answers. No public tx evidence posted.",
      "criteria": {
        "setup": "found", "sync": "found", "tx": "not_found",
        "valoper": "found", "ops": "found", "comms": "found", "safety": "found"
      },
      "evidence_links": ["https://discord.com/channels/..."]
    }
  ]
}
```

Include one object per candidate in `harvest.json`, in the same order.
