## MODIFIED Requirements

### Requirement: Start a round from a board
The app SHALL let Ryder select several beads on a board and start a round on the repository's home computer
(or another computer in the group that has the repository) with an optional budget. The action SHALL be
unavailable, with a reason, when the repository has no `nacre.yaml` on that computer. When the board has a
search or an epic, type or needs-you filter, the round's bead list SHALL open narrowed to the same search and
filter; Ryder SHALL be able to change the search and turn the filter off, and beads already picked SHALL stay
picked, with a count, when the search hides them.

#### Scenario: Multi-select
- **WHEN** Ryder long-presses NAC-1, selects NAC-2 and NAC-3, and chooses "Work these until done"
- **THEN** the app starts a round with those three beads and opens the round's screen

#### Scenario: No gates
- **WHEN** the repository has no `nacre.yaml`
- **THEN** the action is disabled and says the repository needs `nacre.yaml`

#### Scenario: Board search carries over
- **WHEN** the board is searched for "search" and Ryder chooses "Work several beads until done"
- **THEN** the sheet lists only open beads matching "search", with the search filled in, and clearing it lists every candidate

### Requirement: Land
A round that is `ready_to_land` SHALL show its final commit, the build artifact's path on the computer as
copyable text, the beads that will close and those that remain open (with why), whether landing will push
the target, an action to review the commits and changed files the round adds to its base, and a Land button.
nacred SHALL resolve whether landing pushes from the round's `nacre.yaml` and include it in the round.
Landing SHALL ask for confirmation naming the target branch and stating whether it will be pushed; only when
nacred does not say SHALL the confirmation describe pushing as depending on `nacre.yaml`.

#### Scenario: Land a round
- **WHEN** Ryder taps Land and confirms "Fast-forward main and push"
- **THEN** the app asks nacred to land and shows the result

#### Scenario: A round that doesn't push
- **WHEN** the repository's `nacre.yaml` sets `land.push: false` and Ryder taps Land
- **THEN** the confirmation says it moves main on the computer only and doesn't push, and its button reads "Fast-forward main, don't push"

#### Scenario: Review before landing
- **WHEN** Ryder taps "Review the changes" on a ready-to-land round
- **THEN** the app lists the round's commits and each changed file with its added and deleted lines since the round's base
