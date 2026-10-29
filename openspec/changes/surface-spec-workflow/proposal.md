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
is specified and what is actually verified, and a landed change should be finished from the phone.

Tracking: NAC-mp6.7 (under NAC-mp6). Builds on `spec-guided-beads`; the contract differences are listed under Modified Capabilities.

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

Out of scope:
- Automatic proposal development (from labels, bead types or the assistant's auto-apply mode).
- A new top-level tab; Needs you lives on the existing Boards screen.
- Group-by-change board layout and change swimlanes (a later change).
- Changing how proposals are written, reviewed, approved or promoted (`spec-guided-beads` owns that).
- Running `openspec archive` and closing the epic from the phone. It needs a revision model that
  keeps earlier evidence valid when other changes' context moves, a round that has no beads, and
  committed change artifacts on the target. It is tracked as a separate change under NAC-mp6.
- Re-baselining an approval without a new review.
- Multi-repository changes.

## Capabilities

### New Capabilities
- `spec-surfacing`: the assistant entry point, preflight spec drift, card chips, epic-led detail,
  archive from the phone and the related notifications.
- `needs-you`: the cross-board inbox of items waiting on the owner, its item contract, and the
  assistant's use of it.

### Modified Capabilities
None in the accepted spec store (`openspec/specs/` is empty), so there is nothing to write MODIFIED
deltas against. This change deliberately alters behavior defined by two active, unarchived changes, and
says so here rather than silently:
- `spec-guided-work` (spec-guided-beads): adds change phase events, a stale reason that names what
  changed, the changes summary endpoint and an inventory summary on change GET; the Change card on
  unlinked beads moves into Refine and the overflow menu. Phases, actions, approval binding and
  evidence rules are unchanged.
- `round-preflight` (semi-attended-rounds): adds the `spec-drift` finding kind and its answers.
When those changes are archived, their accepted specs must include these differences.

## Impact

- **nacred**: `GET /v1/attention` (per computer; the app merges computers), `GET /v1/repos/{repo}/changes`,
  a `stale_reason` on change records, change events on `/v1/events`, a
  `spec-drift` preflight finding fed by a bounded requirement index, the attention list injected into
  the Mac assistant's prompt, and a `spec-touch:<capability>` label on beads flagged by preflight.
- **API** (`docs/API.md`): the attention item type and its kinds and options, the changes summary, the
  stale reason, change events and the new finding.
- **App**: `develop_proposal`, `create_bead.then_develop`, a `spends` flag on `ToolSpec`, and a
  `needs_you` read tool in `AssistantTools.kt`/`BoardAssistant.kt`; Needs you section and a Boards badge
  on `ReposScreen`/`NacreRoot`; chips in `BeadCard`; epic-led `BeadDetail`; change notifications in `RunNotifier` and
  `RunWatcherService`.
- **Beads**: no new types; labels already used by specflow, plus `spec-touch:<capability>`.
- **Cost**: proposals spend Ryder's subscription only after an explicit approval, within the existing
  $5 per-action limit.
