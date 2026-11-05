# round-app Specification

## Purpose
Today every bead runs on its own: nacred starts one `claude -p` per bead, the run "succeeds" when the process exits, and nobody owns putting the pieces back together. In one week that produced work built on a stale base (NAC-eky on 0.2.1 because `claude --worktree` branched from an unmerged `main`), finished work left uncommitted (NAC-j5z.6), a release branch that sat unmerged while runs kept fork…

## Requirements

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
- **WHEN** Ryder taps Merge into main and confirms "Merge and push"
- **THEN** the app asks nacred to land and shows the result

#### Scenario: A round that doesn't push
- **WHEN** the repository's `nacre.yaml` sets `land.push: false` and Ryder taps Merge into main
- **THEN** the confirmation says it moves main on the computer only and doesn't push, and its button reads "Merge, don't push"

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
Chat SHALL be able to start a round of a list of beads, or of an epic, with an optional budget, through nacred's API or the `nacred round` command, without a separate approval card. Chat SHALL link to the round once it starts.

#### Scenario: Assistant proposes
- **WHEN** Ryder asks Chat to finish everything ready under an epic
- **THEN** Chat starts a round with those beads through nacred, the round runs within its own budget, and Chat links to it

### Requirement: Start work from beads and epics
The app SHALL start rounds from the work itself: Start work on a bead and Run epic on an epic, on the
repository's home computer or another computer in the group that has the repository. The action SHALL be
unavailable, with a reason, when the repository has no `nacre.yaml` on that computer. The board SHALL NOT
offer a multi-select sheet for picking beads across epics.

#### Scenario: Run an epic
- **WHEN** Ryder taps Run epic on NAC-9 with children NAC-9.1 to NAC-9.3
- **THEN** the app starts one round with those children and shows its progress on the epic

#### Scenario: No gates
- **WHEN** the repository has no `nacre.yaml`
- **THEN** the action is disabled and says the repository needs `nacre.yaml`
