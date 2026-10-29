## Context

`spec-guided-beads` (0.8.1) gives every bead a Change card after Design/Notes, relabels a linked bead's
main button "View change", opens a full-screen change view, and offers a spec-only round sheet. Nothing
appears on the Boards screen, on bead cards, in the assistant, in preflight or in notifications.

Behavior this design is fitted to, from Ryder's Nacre sessions and board (83 beads: 33 features, 32
tasks, 10 bugs, 4 epics, 3 chores, 1 spike):
- He drives work by asking the board assistant and approving its write proposals; manual multi-step UI
  is rare.
- He asks "What should I work on next?" and "Which beads need my decision?" repeatedly, and once could
  not tell what a waiting run was asking.
- Rounds are usually two beads with a $25 budget, started from an assistant `start_round` proposal.
- Drift happens in behavior-changing features, not in small fixes.

Existing pieces this reuses: `Nacre.colors.needsYou` and `Round.needsYou`; `RunNotifier`'s round channel;
the assistant's approval cards and auto-apply mode; preflight's finding → question machinery; specflow's
bounded inventory of `openspec/config.yaml`, `AGENTS.md`, accepted specs and active changes.

Mockups of every surface in this design: `docs/mockups/surface-spec-workflow/` (PNG and HTML per
screen, with a README mapping each to its requirement).

## Goals / Non-Goals

**Goals:** proposals start from where Ryder already decides (the assistant, preflight); everything that
waits on him is in one self-explanatory list; the board tells specified from verified at a glance; a
landed change can be finished from the phone. Keep the existing visual vocabulary and navigation.

**Non-Goals:** automatic proposal development; a new tab; group-by-change layout; changes to the
proposal/review/promotion contracts; partial archive.

## Decisions

### 1. Proposals start from assistant approvals, and spending tools always ask
`develop_proposal {repo, id, feedback?}` calls the existing `POST …/issues/{id}/change` with
`action: develop`. There is no per-call budget: specflow already caps each action at
`MaxActionCostUSD` ($5), and the approval card states that limit and what the bead will be compared
with (counts of accepted specs and active changes, from a new `inventory` summary on the change GET).

New beads can't be referred to by a later call in the same turn (the API backend runs calls in order
with literal inputs; the Mac backend returns all proposals in one answer). So `create_bead` gains an
optional `then_develop: true`. One approval card shows both; applying it creates the bead, then calls
`develop` with its id. If `develop` is refused, the bead stays created and the card reports why.
Declining the card declines both; to create without developing, Ryder edits the proposal or asks again.

`ToolSpec` gains `spends: Boolean`. `BoardAssistant.execute` asks for approval for any spending tool
even when auto-apply is on. Only `develop_proposal` and `create_bead` with `then_develop` are spending
in this change; `start_round` keeps its current behavior (out of scope here, noted for Ryder).

The system prompt tells the assistant when to offer it: features or epics whose intent changes behavior
an accepted spec or active change covers, or when Ryder asks. Never for chores, small fixes or beads
already linked.
*Alternative:* a label that auto-starts development. Rejected: beads are created quickly from chat and
each proposal makes paid calls nobody asked for.

### 2. Preflight gets a bounded requirement index and a `spec-drift` finding
specflow exports `RequirementIndex(repo)`: capability, requirement title, path and line for accepted
specs and active changes, at most 24 KB, truncated by whole capabilities with the omitted ones listed.
A repository whose OpenSpec setup is missing or invalid gets no index and preflight skips spec drift
silently, as today. New finding kind `spec-drift` must cite a path and requirement from the index;
uncited findings are dropped. Like every preflight question, it holds the round in "needs decision"
until answered; nothing about per-bead question gating changes. Answers:
- **Amend the spec first** — the bead leaves the round as in "leave it out" below, and nacred calls
  `develop` for it with feedback naming the requirement. If `develop` is refused
  (repository busy, record already exists, unavailable), the question reopens with that reason, the
  way a failed merge choice does today, and the bead stays in the round until answered again.
- **Proceed as an exception** — the answer is posted to the bead as a decision comment and the bead is
  labelled `spec-touch:<capability>`.
- **Leave it out of this round** — a new deterministic answer handled by `applyDecisions` (today it
  acts only on unmerged-work answers): the bead becomes `BeadSkipped` with reason "Left out: spec
  drift against <requirement>", a note is posted to it, and beads that build on it are skipped the way
  `maybeStart` already skips beads whose prerequisite was parked (`prerequisites` treats skipped like
  parked); the skip reason is reworded so it says "was left out" rather than "was parked".
These three options are fixed by nacred, not model-written free text.
Spec section beads are skipped: their contract is the pinned snapshot. `spec-touch:` is deliberately
not a `specflow:`/`spec-section:` prefix, so it never makes a bead linked or blocks ordinary runs.

### 3. Needs you: computed by nacred, merged by the app, injected for the Mac assistant
`GET /v1/attention` returns items for every repository on that computer:
`{id, kind, repo, bead_id?, change_id?, round_id?, title, why, source_ref?, options[], created_at}`.
`id` is a digest of kind and the underlying record and its state, so the same waiting state keeps its
id and the app can dedupe across refreshes and computers. Each option is either
`{type: "action", label, method, path, body}` (an exact endpoint) or `{type: "open", label, target}`
(navigate: bead, change, round). `why` is one or two plain sentences that stand alone.

| Kind | Source | Options |
|---|---|---|
| `bead_decision` | open bead with `human` label or `decision` type (`Issue.needsYou`) | open bead |
| `proposal_ready` | change phase `awaiting_approval` | open change at revision; `approve` with revision |
| `review_blocked` | phase `needs_attention` | open change (feedback lives there) |
| `approval_stale` | phase `stale`, or a ready/awaiting record whose live `current()` check fails for any reason | `revise`; open change |
| `workflow_interrupted` | phase `interrupted` | `resume`; open change |
| `round_question` | round `needs_decision`, per open question | the question's answers via `POST /v1/rounds/{round}/answers`; open round |
| `bead_parked` | round bead parked | `…/beads/{id}/retry`; open round |
| `round_ready` | round `ready` | `…/land`; open round |
| `change_archivable` | every section landed with matching evidence (§6) | open change |

Building the list reads every ready or awaiting change record, which runs inventory collection and bd
intent reads. Results are cached per repository by inventory digest and bd's last-modified time, and
refreshed at most every 30 seconds.

Human checks on spec sections are already `human`-labelled beads, so they arrive as `bead_decision`;
this change adds no new resolve endpoint. The board's existing "Needs you · n" chip keeps its meaning
(beads needing Ryder on that board) and is a subset of the inbox.

The Boards screen shows Needs you above the summary tiles when there are items, ordered questions and
decisions first, then approvals, then landing and archive, newest first within each, at most five
before "Show all". An empty list or bridges without the endpoint hide it. The Runs tab badge is
unchanged; Boards gets its own badge with the attention count.

Assistant: the API backend gets a `needs_you` read tool. The Mac backend drops reads, so nacred
includes the current attention list (titles, why, ids) in the Mac assistant's prompt context for each
turn, and the prompt tells both to start from it for "what next?" / "what needs me?".

### 4. Card chips come from data the board already loads, plus one summary
`GET /v1/repos/{repo}/changes` returns per change `{id, change_name, phase, epic_id, seed_ids[],
sections[] {id, bead_id, evidence}}`, `evidence` = `none | in_round | verified | landed`, derived by
the existing evidence rules. Evidence is cached per change revision and round-event sequence and
recomputed only when either moves; the endpoint returns at most 50 changes. `BeadCard` shows the spec
chip in the first label slot and the first two other labels after it (`labels.take(2)`):
- seed or epic: phase label (`changePhaseLabel`), plus a section strip on epics;
- section bead: "§ n of <change>" and its evidence;
- spec-linked closed bead without verified evidence: "closed, not verified";
- `spec-touch:` label: "Touches a spec".
Colors: violet tint for spec state, `needsYou` for anything waiting on Ryder, success for verified and
landed.

### 5. Epic detail leads with the change; ordinary beads get a quiet entry
For a change's epic, `BeadDetail` places a change header (name, revision, phase), the section strip
with per-section evidence, open human checks and the primary next action from `changeNextAction`
above Description. Seeds keep the card where it is. Unlinked beads lose the Change card; "Develop a
proposal" moves into the Refine sheet and the overflow menu.

### 6. Stale approvals and readiness to archive, without new actions
**Stale.** `Contract()` already refuses any approval whose inventory digest moved, for any cause: another
change's draft, an accepted spec, AGENTS.md. Today that surfaces as a conflict when a round starts.
specflow now computes the cause when reading a ready or awaiting record: it compares the stored
per-file digests (`record.Baseline`, captured with the revision) with the current inventory and records
`stale_reason` ("AGENTS.md changed", "Active change resilient-rounds changed"). Causes that aren't files
are named too: "Seed bead NAC-x changed" when the intent revision moves, and "Section bead NAC-y was
edited" when `sectionCurrent` fails. Records with no baseline get "The project's specifications changed
since this was approved". Reading sets the phase to `stale` as the existing rules
already do. The item offers the existing `revise`. No re-baseline: a revision change invalidates
section provenance and the `Revision:` line in generated section bodies, and keeping evidence
across it needs the revision-alias design deferred to the archive follow-up.

**Ready to archive.** A change is reported as `change_archivable` when every current section of the
approved revision has evidence from a landed round with matching provenance, and no section bead is
reopened or carries an open `human` label. The item opens the change, whose existing obligations list
shows the archive and epic-close steps.

**Deferred** (follow-up change under NAC-mp6): archive from the phone. Its known requirements, from this
change's review: a revision-alias set that `Contract`, evidence and `sectionCurrent` accept; an archive
round kind with its own states (no preflight, no beads, straight to gates and landing) and every bead
guard adjusted; change artifacts committed on the target, or the pinned snapshot installed before
`openspec archive`; and no reliance on a per-repository round mutex, which doesn't exist, with landing
relying on `land()` re-verifying a moved target.

### 7. Change events and notifications
nacred publishes `change` events on `/v1/events` on each phase transition. The repository layer
exposes them like round changes. `RunNotifier` raises "Proposal ready for your review" on the change
into `awaiting_approval` and "Change ready to archive" when an item of that kind first appears, once each,
on the round channel. `RunWatcherService` also watches while any change is drafting, reviewing or
promoting. When nothing is being watched, the app shows the notification on its next refresh rather
than claiming background delivery.

## Risks / Trade-offs

- [Preflight over-asks with spec drift] → Citations required; only behavior contradictions, not mere
  mentions; the plan shows the question count, as today.
- [Assistant offers proposals too eagerly] → Prompt rules exclude small work; every offer needs approval
  and shows a budget; declines are remembered for the bead in that conversation.
- [Needs you grows stale or noisy] → Items are derived from current state on each request, not stored;
  answered items disappear on refresh.
- [Stale approvals are frequent, because any OpenSpec edit stales every change] → The reason is now named
  and actionable; reducing how often it happens belongs with the revision-alias work.
- [Archive stays a CLI step for now] → It is listed in Needs you with the change's obligations, so it
  isn't forgotten.
- [Two computers report the same repo] → Items carry the repo key; the app dedupes by `id`.

## Migration Plan

Additive. Older bridges without `/v1/attention` or `/changes` leave the Needs you section hidden and
chips absent; bead detail falls back to today's Change card. Ship nacred first, then the app.

## Open Questions

None blocking. Defaults: specflow's $5 per-action limit, three review passes (both unchanged), 24 KB
requirement index, five inbox items before "Show all". For Ryder: should `start_round` also become a
spending tool that always asks? Left unchanged here.
