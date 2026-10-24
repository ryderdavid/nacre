## ADDED Requirements

### Requirement: Start a round from a board
The app SHALL let Ryder select several beads on a board and start a round on the repository's home computer
(or another computer in the group that has the repository) with an optional budget. The action SHALL be
unavailable, with a reason, when the repository has no `nacre.yaml` on that computer.

#### Scenario: Multi-select
- **WHEN** Ryder long-presses NAC-1, selects NAC-2 and NAC-3, and chooses "Work these until done"
- **THEN** the app starts a round with those three beads and opens the round's screen

#### Scenario: No gates
- **WHEN** the repository has no `nacre.yaml`
- **THEN** the action is disabled and says the repository needs `nacre.yaml`

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
A round that is `ready_to_land` SHALL show its final commit, the build artifact, the beads that will close
and those that remain open (with why), and a Land button. Landing SHALL ask for confirmation naming the
target branch and whether it will be pushed.

#### Scenario: Land a round
- **WHEN** Ryder taps Land and confirms "Fast-forward main and push"
- **THEN** the app asks nacred to land and shows the result

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
