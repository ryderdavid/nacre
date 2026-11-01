# round-app Specification

## Purpose
Today every bead runs on its own: nacred starts one `claude -p` per bead, the run "succeeds" when the process exits, and nobody owns putting the pieces back together. In one week that produced work built on a stale base (NAC-eky on 0.2.1 because `claude --worktree` branched from an unmerged `main`), finished work left uncommitted (NAC-j5z.6), a release branch that sat unmerged while runs kept fork…
## Requirements
### Requirement: Start a round from a board
The app SHALL let Ryder select several beads on a board and start a round on the repository's home computer
(or another computer in the group that has the repository) with an optional budget. The action SHALL be
unavailable, with a reason, when the repository has no `nacre.yaml` on that computer. The round's bead list
SHALL open with the board's search and filter, which Ryder can change.

#### Scenario: Multi-select
- **WHEN** Ryder long-presses NAC-1, selects NAC-2 and NAC-3, and chooses "Work these until done"
- **THEN** the app starts a round with those three beads and opens the round's screen

#### Scenario: No gates
- **WHEN** the repository has no `nacre.yaml`
- **THEN** the action is disabled and says the repository needs `nacre.yaml`

#### Scenario: Board search carries over
- **WHEN** the board is searched for "search" and Ryder chooses "Work several beads until done"
- **THEN** the sheet lists only open beads matching "search", with the search filled in, and clearing it lists every candidate

#### Scenario: Filters and hidden picks
- **WHEN** the board has an epic, type or needs-you filter, or Ryder narrows the search after picking beads
- **THEN** the list opens with the same filter, which Ryder can turn off, and picked beads stay picked, with a count, when the search hides them

### Requirement: Round screen
The app SHALL show each round with its state, spend against budget, elapsed time, and for each bead its
state, branch, verification result, and a link to its run. It SHALL show the integration log (merges,
conflicts, gate results) live.

#### Scenario: Live progress
- **WHEN** a bead is merged while the round screen is open
- **THEN** the bead's state changes to integrated and the log gains the merge without a manual refresh

### Requirement: Answer decisions
When a round is `needs_decision`, the app SHALL show each question with the beads it concerns and the
suggested answers, and SHALL accept a chosen or typed answer. It SHALL also show the plan's ordering and let
Ryder reorder or unlink overlapping beads before implementation starts.

#### Scenario: Answer a question
- **WHEN** Ryder picks an answer for the only open question
- **THEN** the app sends it, and the round moves on to implementing

### Requirement: Land
A round that is `ready_to_land` SHALL show its final commit, the build artifact's path as copyable text,
the beads that will close and those that remain open (with why), whether landing will push, a way to review
the round's commits and changed files, and a Land button. Landing SHALL ask for confirmation naming the
target branch and whether it will be pushed, which nacred resolves from the round's `nacre.yaml`.

#### Scenario: Land a round
- **WHEN** Ryder taps Land and confirms "Fast-forward main and push"
- **THEN** the app asks nacred to land and shows the result

#### Scenario: A round that doesn't push
- **WHEN** the repository's `nacre.yaml` sets `land.push: false` and Ryder taps Land
- **THEN** the confirmation says it moves main on the computer only and doesn't push, and its button reads "Fast-forward main, don't push"

#### Scenario: Push not reported
- **WHEN** nacred's round doesn't say whether landing pushes (an older nacred)
- **THEN** the confirmation describes pushing as depending on `nacre.yaml`

#### Scenario: Review before landing
- **WHEN** Ryder taps "Review the changes" on a ready-to-land round
- **THEN** the app lists the round's commits and each changed file with its added and deleted lines since the round's base

### Requirement: Notifications
The app SHALL notify Ryder when a round needs a decision, when a bead is parked, when a round is paused by a
budget, and when a round is ready to land, at most once per event, and only for rounds on computers the app
is paired with.

#### Scenario: Ready to land
- **WHEN** a round becomes `ready_to_land` while the app is in the background
- **THEN** Ryder gets one notification that opens the round's screen

### Requirement: Assistant can propose a round
The board assistant SHALL have a `start_round` write tool taking a list of beads (or an epic) and an
optional budget, approved like other writes, which starts a round and links to it.

#### Scenario: Assistant proposes
- **WHEN** Ryder asks the assistant to finish everything ready under an epic
- **THEN** the assistant proposes `start_round` with those beads, and approving it starts the round

