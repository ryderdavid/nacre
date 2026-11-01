# Independent proposal review

Reviewer: a fresh adversarial reviewer agent with read access to the repository, given the change and
pointers to the project context, the related active changes and the source, not the author's reasoning.
Three bounded passes on 2026-10-28.

## Disposition

Proceed to Ryder's approval. Pass 1 found 1 blocking and 16 major or minor findings, pass 2 found 1
blocking and 4 major, and pass 3 found none blocking or major. This approves the proposal's design
against the source as it stands. It does not verify an implementation.

## Pass 1 (revise)

| Finding | Resolution |
|---|---|
| F1 blocking: archive eligibility against a live revision never holds, and archiving stales every other change | Eligibility based on provenance; phone archive later deferred (pass 2) |
| F2 "other beads continue" contradicts preflight blocking the round | Spec drift holds the round like every question |
| F3 no budget field on change actions | Uses specflow's fixed $5 per-action limit, shown on the card |
| F4 a later call can't reference a bead created in the same turn | `create_bead.then_develop`, one approval |
| F5 auto-apply skips approval for every write | `spends` flag on `ToolSpec`, always asks |
| F6 the Mac backend drops read tools | nacred injects the attention list into the Mac prompt |
| F7 "Needs you" already means `human`/`decision` beads | `bead_decision` kind; the board chip is a subset |
| F8 human checks have no endpoint | Arrive as `bead_decision`; options are exact actions or navigation |
| F9 the badge belongs to Runs | Runs badge unchanged; Boards badge added |
| F10 no change events or background watching | Change events, watcher while proposals are worked, honest refresh wording |
| F11 changes to spec-guided-work's contract were unstated | Listed under Modified Capabilities |
| F12 round landing isn't reusable | Deferred with archive |
| F13 tasks too large; 6.2 had no gate | Split into small tasks, each with gate commands |
| F14–F17 minor (index cap, amend-first refusals, label name, chip slot and cost) | Fixed |

## Pass 2 (revise)

| Finding | Resolution |
|---|---|
| N1 blocking: rebaseline needs a new revision, which invalidates section provenance and section bodies | Rebaseline removed; revision aliases recorded as a prerequisite of the archive follow-up |
| N2 stale arises from any inventory edit, not only archive | `approval_stale` for any `current()` failure, with a named cause |
| N3 no per-repository round mutex; N4 rounds require beads; N6 artifacts uncommitted on the target | Phone archive deferred to a follow-up change, with these requirements recorded |
| N5 no deterministic "leave it out" answer | Defined: `BeadSkipped`, note, dependents skipped |
| N7 "does not change its contracts" clause | Removed |
| F8 wrong answers route | `POST /v1/rounds/{round}/answers` |

## Pass 3 (proceed)

Minor items, folded in: stale reasons for seed-intent and section-edit causes and a fallback for records
with no baseline; caching the attention list; "was left out" wording for skipped dependents.

## Owner decisions

- Phone archive is deferred out of this change (author's scoping after pass 2; Ryder may override).
- Whether `start_round` should also always ask under auto-apply is left to Ryder; unchanged here.

# Amendment review (2026-11-01, NAC-mp6.7.1)

Reviewer: a fresh adversarial reviewer agent per pass, with read access to the repository, given the
amended change, the project context, `spec-guided-beads` and the source, not the author's reasoning.
Three passes. The amendment adds New effort intake, the Proposals lane, Found something and the frozen
plan (design §8–§11, `effort-intake`).

## Disposition

Pass 1 found 1 blocking and 6 major findings; pass 2, 1 blocking and 3 major; pass 3, 1 major and minor
items. All were resolved in the text below. Pass 3's fixes were not re-reviewed by a fourth pass.
This reviews the design against the source; it does not verify an implementation.

## Pass 1 (revise)

| Finding | Resolution |
|---|---|
| A1 blocking: "changes what done means" revised a promoted change, silently reopening landed sections and refusing section work until re-approval | Never revises automatically; a found bead and Needs you item state how many landed sections would reopen and offer Revise now or a follow-on change |
| A2 amendment seeds couldn't enter Proposals (the record already has an epic) | Membership by `seed_ids` (including aliases) and pre-promotion phase; the epic stays in its lane |
| A3 an amend of a change with active work fails late, after a paid author call | `target_active` from intake; Create and develop disabled with "develop later" |
| A4 nothing persisted suppressed answered `Found:` comments | `[nacre-found:<comment-id>]` reply comment is the persisted marker |
| A5 deferred revise lost found-bead ids; several notes vs the feedback cap | Revise now through the found endpoint with `found_beads`, notes joined under the cap |
| A6 `--no-inherit-labels` only on the found path | `bd.CreateParams.NoInheritLabels` on every nacred create under a spec epic |
| A7 changing `laneOf` affected other callers | New `workflowLane`; each caller's behavior named |
| A8–A15 minor (structured intake field, ordering before a section, `found_in` from a round, label key, docs and assistant tasks, scope wording, summary cap, mockup and tile details) | Fixed |

## Pass 2 (revise)

| Finding | Resolution |
|---|---|
| N1 blocking: found endpoint had no `comment_id` or dismiss, so markers couldn't be posted | `comment_id` on every answer; `dismiss` answer |
| N2 found-amend beads could run; spec drift flagged found beads against their own change | Guards refuse `found-amend`; spec drift skips `found:` beads |
| N3 alias seeds never close and return to Proposals | Closed with a comment after their revision is approved and synchronized |
| N4 no pending state after Revise now | Items suppressed while the pending revise lists the bead; "in revision" shown |
| N5–N9 minor (related/follow-on plumbing, conditional discovered-from, lane availability, missing scenarios, parked beads' comments) | Fixed |

## Pass 3 (revise)

| Finding | Resolution |
|---|---|
| P1 major: Needs you options couldn't open a prefilled sheet, which out-of-scope, follow-on and how/order need | Two `open` sheet targets (`found_something`, `new_effort`) with prefill |
| P2–P8 minor (promotion's own creates excluded from the found rule, untracked hand-written changes and `target_active`, marker timing on cancel, covered card Close, code names, found_work ordering, scenario placement and app tasks) | Fixed |
| Placement: spec-drift skip and leave-out wording belong under `round-preflight` | Moved in Modified Capabilities |

## Owner decisions (2026-11-01)

- Amend this change, not `spec-guided-beads` (implemented) or a new change.
- Intake may answer "just a bead".
- Add the Proposals lane.
- `tasks.md` is the plan beads are made from, frozen at approval; borrow beads-plan, Turma and
  boardthatpowder ideas rather than installing them; the archive-time `tasks.md` write goes to NAC-mp6.8.

# Implementation decisions (2026-11-01, NAC-mp6.7.2–.11)

Made while implementing, without stopping to ask, per Ryder's instruction. Each follows the spec
where it speaks and the design's intent where it doesn't.

- Section beads were created as plain children of NAC-mp6.7 (no `specflow:` labels): this change has
  no specflow record, and hand-adding labels would make the guards refuse them.
- The changes summary also reports `openspec` (valid OpenSpec setup), so the app knows when to offer
  New effort and the Proposals lane without a failed intake call. Not in the design; additive.
- Spec state uses the theme's violet `secondaryContainer`, matching the mockups; anything waiting on
  Ryder uses the existing `needsYou`/`needsYouContainer`.
- Intake's answer also lists every active change's name, so the Amend override can offer changes
  that have no specflow record (hand-written ones). Additive.
- An "Already covered" verdict offers "Create as a bead anyway" (and the other routes) but not "Open
  the covering change": the cited change may be hand-written and have no change view. Its
  citations are shown instead.
- `found_something` is not a spending tool: its amend answer files a pending amendment and never
  revises by itself (design §10), so only Revise now spends. From the assistant, out of scope
  creates the related bead directly once approved, since the assistant can't open New effort.
- Found beads get their own chips ("Changes <change>" for pending amendments, "Added to <change>"),
  which the spec's chip list didn't name; they follow its one-chip, first-slot rule.
- Change notifications come from Needs you items appearing (proposal ready, ready to archive), on the
  round channel, whether or not the app is in front, like round notices. The first list after
  start only records what is already waiting, so opening the app doesn't replay old items.
- "Needs a new round" in the plan view is shown for a section without evidence once the change has
  been promoted more than once (a later revision was approved).
- Release: app 0.9.0 (code 16) and nacred built as 0.9.0. Not installed on the Mac Studio, not
  tagged or published, not merged to main: Ryder's call.
- The emulator walkthrough (docs/SURFACE_SPEC_WORKFLOW_VERIFICATION.md) found and fixed a bug: a
  `discovered-from` link from a found bead to its own epic, which bd refuses. The emulator keeps a
  paired, now offline, "Walkthrough" computer; remove it in Settings.
- Task 10.2 (one real cycle to ready-to-archive) is left open: it needs this nacred deployed and real
  Claude Code rounds.
