## Why

Nacre 0.8.1 can carry a bead from intent to a reviewed OpenSpec change, section beads and a pinned round
(`spec-guided-beads`), but the whole workflow surfaces as one card near the bottom of bead detail. It has
never been used: the daemon's `changes/` directory is empty.

That placement doesn't match how Ryder works. He rarely navigates manual screens; he asks the board
assistant and approves its proposals (`create_bead`, `start_round`, `run_in_claude_code`). His recurring
questions are "What should I work on next?", "Which beads need my decision?", "How many of the
in-progress tickets are actually resolved?" and, once, a run waiting on him that he couldn't make sense
of. Most of his beads are small tasks and bugs that go straight to a run or a two-bead round, so a
proposal on every bead would be friction; the drift he cares about (NAC-013.13, "I'm not seeing them";
the round-app picker that contradicts its spec) happens in features that change specified behavior.

So the workflow should appear at three places he already is: when the assistant files work, when a
round's preflight reads beads, and in one list of everything waiting on him. The board should show what
is specified and what is actually verified, and a landed change should be reported ready to archive.

**Amendment, 2026-11-01 (NAC-mp6.7.1).** Ryder wants every effort on a board to start the same way,
with the system deciding whether it needs a new change, amends an active one, or is just a bead. That
keeps the friction finding above: intake may answer "just a bead", so small work still goes straight to
a run. The same research, on how people combine OpenSpec and Beads, settled two boundaries this change
now adopts. First, OpenSpec owns what "done" means and Beads owns the work: `tasks.md` is the plan that
beads are made from, it is frozen with the approval, and progress is read from beads, never ticked back
into it. Second, work discovered along the way is triaged by one rule: if it changes what "done" means,
amend the spec first; if it only changes how or in what order, it's a bead under the change's epic; if
it's out of scope, it's a new effort. Today that rule exists only before a round (spec drift), not while
or after work runs.

Tracking: NAC-mp6.7 (under NAC-mp6); the amendment is NAC-mp6.7.1. Builds on `spec-guided-beads`; the contract differences are listed under Modified Capabilities.

## What Changes

- **Assistant `develop_proposal` tool.** The board assistant can propose developing a reviewed proposal
  for an existing bead, and `create_bead` gains a `then_develop` option so a new bead and its proposal
  are one approval. Both are marked as spending tools, which always ask, even in auto-apply mode. Each
  proposal action stays within specflow's existing per-action limit ($5), shown on the card.
- **Spec drift in round preflight.** Preflight is given the accepted specs and active changes and gains a
  `spec-drift` finding: a round bead would change behavior that a requirement specifies. Like every
  preflight question it holds the round until answered (amend the spec first / proceed as an exception /
  leave it out), never a silent pass. Repositories without a valid OpenSpec setup skip the check.
- **Needs you.** One inbox at the top of Boards that collects everything waiting on Ryder across boards
  and computers: beads already marked as needing him (`human` label or `decision` type, today's board
  "Needs you" chip), proposals ready for approval, review blockers, stale approvals, interrupted
  workflows, round questions, parked beads, ready-to-land rounds and changes ready to archive. Each item
  explains itself and offers its answers inline. The assistant reads the same list on both backends.
- **Board card chips.** Spec-linked cards show the change phase; section beads show their change and an
  evidence state; closed beads without evidence say so; ordinary beads that preflight flagged show
  "Touches a spec". Ordinary cards are otherwise unchanged.
- **Epic detail led by its change.** For a spec epic, the change summary and a section strip
  (planned, in round, verified, landed) come first, above description and notes. The large Change card
  on ordinary beads becomes a quiet "Develop a proposal" action beside Refine.
- **Ready to archive, made visible.** When every section has landed with evidence for the approved
  revision, Needs you says so and opens the change, which already lists the remaining archive and
  epic-close steps. Running the archive from the phone is a follow-up change (see Out of scope).
- **Stale approvals explained.** Any context edit that breaks a change's approval (another change, an
  accepted spec, AGENTS.md) shows as a stale item naming what changed, instead of a bare conflict when a
  round is started.
- **Notifications** for "Proposal ready for your review" and "Change ready to archive", on the existing
  round notification channel, driven by new change events on `/v1/events`.
- **New effort, with intake.** The board's "New bead" button becomes "New effort" in repositories with
  a valid OpenSpec setup. After the existing draft step, one bounded intake call compares the draft
  with the requirement index and active changes and returns a verdict: just a bead, a new change, an
  amendment to a named active change, or already covered, with a rationale and citations. Ryder
  confirms or overrides it. "Just a bead" creates the bead as today; a change verdict creates the bead
  and starts `develop` with the verdict as feedback, which spends and so says so on the button.
  Without OpenSpec, or on an older bridge, the sheet is today's New bead.
- **Proposals lane.** A first lane on the Workflow board, shown wherever New effort is, holds
  beads whose change is still being planned: drafting, in review, needing attention, awaiting
  approval, interrupted, stale before promotion, or judged already covered. Its cards show the change
  phase. Promotion moves the bead (now the epic) to its ordinary lane. The lane is derived from change
  records, not a new bd status, and is not a drop target.
- **Found something.** On a spec epic, its section beads and their spec rounds, Ryder can report
  discovered work and answer it with the same rule as spec drift: it only changes how or in what order
  (a bead under the epic), it changes what "done" means (a found bead and a Needs you item that offers
  revising, stating how many landed sections that would reopen, or a follow-on change), or it's out of
  scope (a new effort, linked as related). Agents in spec rounds stay inside their section
  and report what they find as `Found:` comments, which become Needs you items with those three answers.
- **The plan is frozen; progress comes from beads.** After approval `tasks.md` is part of the approved
  revision and is not edited to track progress. Change detail and the epic show the plan with each
  section's live state from its bead and evidence, plus the beads added under the epic since. Archive
  readiness also waits for those added beads. Writing a final `tasks.md` from bead state at archive time
  belongs to the archive follow-up (NAC-mp6.8).

Out of scope:
- Automatic proposal development (from labels, bead types or the assistant's auto-apply mode). Intake
  runs only when Ryder continues the New effort sheet, and only a confirmed change verdict develops.
- A new top-level tab; Needs you lives on the existing Boards screen. The Proposals lane is a board
  lane, not navigation.
- Group-by-change board layout and change swimlanes (a later change).
- Changing how proposals are written, reviewed, approved or promoted (`spec-guided-beads` owns that).
  Intake's verdict is advice to the author; the author's full reconciliation still chooses the route.
- Installing third-party OpenSpec-to-Beads tools or skills (beads-plan, Turma, openspec-to-beads).
  specflow's promotion already does their import step with stable identity and recovery; this change
  borrows their ideas, not their code.
- Agents creating beads during spec rounds. They report; Ryder decides.
- Running `openspec archive` and closing the epic from the phone. It needs a revision model that
  keeps earlier evidence valid when other changes' context moves, a round that has no beads, and
  committed change artifacts on the target. It is tracked as a separate change under NAC-mp6.
- Re-baselining an approval without a new review.
- Multi-repository changes.

## Capabilities

### New Capabilities
- `spec-surfacing`: the assistant entry point, preflight spec drift, card chips, epic-led detail,
  ready-to-archive reporting and the related notifications.
- `needs-you`: the cross-board inbox of items waiting on the owner, its item contract, and the
  assistant's use of it.
- `effort-intake`: the New effort intake and its verdicts, the Proposals lane, Found something and its
  three answers, and showing the frozen plan with live progress from beads.

### Modified Capabilities
None in the accepted spec store (`openspec/specs/` is empty), so there is nothing to write MODIFIED
deltas against. This change deliberately alters behavior defined by two active, unarchived changes, and
says so here rather than silently:
- `spec-guided-work` (spec-guided-beads): adds change phase events, a stale reason that names what
  changed, the changes summary endpoint and an inventory summary on change GET; the Change card on
  unlinked beads moves into Refine and the overflow menu; `develop` accepts an intake verdict as
  feedback (with a structured `intake` field); creates under a spec epic don't inherit its workflow
  labels; archive readiness also waits for beads added under the epic; approval of a revision started
  from found work closes those found beads; alias seeds are closed once their revision is approved and
  synchronized; run and round guards refuse `found-amend` beads. Phases, actions, approval binding and evidence rules are
  unchanged. Its design's "no new top-level navigation or replacement board" still holds: the Proposals
  lane adds a lane to the existing board.
- `round-preflight` (semi-attended-rounds): adds the `spec-drift` finding kind and its answers; spec
  round prompts gain the stay-in-section rule and `Found:` comments; spec drift skips `found:` beads;
  the "leave it out" answer rewords the skip reason for dependent beads ("was left out", not "was
  parked").
When those changes are archived, their accepted specs must include these differences.

## Impact

- **nacred**: `GET /v1/attention` (per computer; the app merges computers), `GET /v1/repos/{repo}/changes`,
  a `stale_reason` on change records, change events on `/v1/events`, a
  `spec-drift` preflight finding fed by a bounded requirement index, the attention list injected into
  the Mac assistant's prompt, and a `spec-touch:<capability>` label on beads flagged by preflight;
  `POST /v1/repos/{repo}/intake`, `POST /v1/repos/{repo}/changes/{change}/found`, the `found_work`
  attention kind, and `Found:` comment collection after spec round beads finish.
- **API** (`docs/API.md`): the attention item type and its kinds and options, the changes summary, the
  stale reason, change events and the new finding; intake, found work and the extra archive condition.
- **App**: `develop_proposal`, `create_bead.then_develop`, a `spends` flag on `ToolSpec`, and a
  `needs_you` read tool in `AssistantTools.kt`/`BoardAssistant.kt`; Needs you section and a Boards badge
  on `ReposScreen`/`NacreRoot`; chips in `BeadCard`; epic-led `BeadDetail`; change notifications in `RunNotifier` and
  `RunWatcherService`; New effort in `NewBeadSheet`/`BoardScreen`, the Proposals lane in
  `data/board` and `KanbanBoard`, the Found something sheet, the plan-and-progress view in
  `ChangeDetail`, and a `found_something` assistant write tool.
- **Beads**: no new types or statuses; labels already used by specflow, plus `spec-touch:<capability>`,
  `found:<change-id>` and `found-amend`; `discovered-from` and `related` dependencies; children added under a spec epic
  are created with `--no-inherit-labels` so they don't inherit `specflow:` and fail closed as linked.
- **Cost**: proposals spend Ryder's subscription only after an explicit approval, within the existing
  $5 per-action limit. Intake is one read-only call capped at $0.50, run only when Ryder continues.
