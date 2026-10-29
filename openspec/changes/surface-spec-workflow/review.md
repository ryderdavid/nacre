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
