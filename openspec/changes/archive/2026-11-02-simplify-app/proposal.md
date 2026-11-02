## Why

Nacre grew one concept at a time and now asks Ryder to learn too many of them before work gets done. A
review of the whole app (Nov 2026) counted five overlapping ways to execute work (run, round, goal,
finish epic, agent session), about 25 execution labels, 13 kinds of Needs you items, three chat surfaces
(a sheet, a dialog and a stacked floating button) and settings that only restate defaults. Turning on a
goal silently changed a round from one worker per bead into one session for every bead, so "goal-driven"
and "fanned out" could not both be true. On the Boards screen, 36 of 39 Needs you items were stuck beads
in repositories that cannot run rounds at all, pushing the boards off screen.

## What Changes

- **One flow per epic.** An epic is the unit of work; in a repository with OpenSpec it is the change, and
  its children are the change's task sections. Ryder approves once with **Approve and run**; nacred then
  runs every open child, merges them into `epic/<id>`, finishes the epic into the target branch
  (archiving the change) and closes it, stopping only for real questions, failed checks it cannot fix,
  limits, or human checks.
- **Goals fan out.** Every round worker is goal-driven: one worker per bead, in its own worktree, kept
  working until its bead's acceptance criteria hold. The goal, committed and pushed options are removed;
  pushing follows `nacre.yaml`.
- **One action per object.** A bead offers **Start work**; an epic offers **Run epic** (and **Finish
  epic** only when everything is already closed). Plain agent runs move to the overflow as **Run an agent
  manually…**.
- **Plain-language phases.** A change reads Planning, Ready for your approval, Working or Done, with a
  single Needs attention banner when something needs fixing.
- **Navigation.** Boards, Inbox (Needs you) and Activity (runs and rounds); Settings moves to the Boards
  top app bar. Needs you no longer sits above the boards and only lists items whose actions can run.
- **Chat.** The docked-chat-pane change is implemented as part of this: one AI action in the top app bar,
  a docked pane on wide layouts and a sheet on phones; the floating chat button and the chat dialog go.
- **Less chrome.** The Boards aggregate tiles, the board's count strip, the Priority and Type groupings,
  internal labels on cards and machine marker comments are removed. Settings drops the Material You,
  default worktree and default permission options and the developer footer.

## Capabilities

### Modified Capabilities

- `run-rounds`: goal-driven workers always fan out; rounds may finish their epic.
- `round-app`: rounds start from beads and epics, not a multi-select sheet; no goal options.
- `epic-completion`: Run epic replaces Work until done and finishes into the target automatically.
- `needs-you`: Inbox destination; only actionable items.
- `spec-guided-work`: Approve and run; plain-language phases.
- `effort-intake`: Proposals lane wording follows the new phases.

## Impact

- nacred: `internal/rounds`, `internal/specflow`, `internal/attention`, `docs/API.md` (new `finish` and
  `run` request fields; goal options ignored).
- App: navigation, Boards, board, bead, epic, change, rounds, settings and chat UI.
- Rounds already running keep working and landing under the old rules.

## Out of scope

- Setting up other repositories (`nacre.yaml` files) and triaging their boards.
- Changing how gates, verification, integration or landing work inside a round.
- Changing the assistant's tools or the Claude API features beyond where chat opens.
- Multi-computer scheduling.
