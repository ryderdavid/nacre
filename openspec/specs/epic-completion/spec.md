# epic-completion Specification

## Purpose
Work set in the app ends finished: a bead's round lands it, an epic's work collects on the epic's branch, and finishing the epic merges it into the main branch, archives its change and closes it. Pushing the main branch is the hand-off to the repository's CI.

## Requirements

### Requirement: Work until done
The app SHALL offer one primary action to get work done: Start work on a bead, a round of that bead
which lands when it passes, and Run epic on an epic with open children, a round of every open child
that lands on the epic's branch and then finishes the epic. A plain run SHALL be available only as Run
an agent manually… in a bead's overflow, and starting work SHALL NOT offer goal, committed or pushed
options.

#### Scenario: Approved specification sections
- **WHEN** Ryder runs an epic whose children are approved specification sections
- **THEN** nacred resolves the approved change and revision itself and the round lands without waiting

#### Scenario: One tap to finished work
- **WHEN** Ryder taps Start work on a bead outside any epic
- **THEN** a round works it in its own worktree, checks every gate and the build, lands it on the landing target, pushes it when `nacre.yaml` says so and closes the bead without waiting for him

#### Scenario: Manual run
- **WHEN** Ryder chooses Run an agent manually… from a bead's overflow
- **THEN** the app offers the agent, computer, permission, model, effort, worktree and instruction choices and starts a plain run

### Requirement: Epic work lands on the epic's branch
A round of beads under an epic SHALL land on the epic's branch, `epic/<id>`, made from the landing
target the first time, and SHALL neither bump nor tag the version. nacred SHALL refuse a round that
mixes beads of different epics, or epic beads with beads outside any epic.

#### Scenario: Main untouched mid-epic
- **WHEN** a round of an epic's bead lands
- **THEN** the epic's branch has the work, the landing target has not moved, and the bead is closed

#### Scenario: Mixed round
- **WHEN** Ryder starts a round with beads from two epics
- **THEN** nacred refuses it, naming each epic and its beads

### Requirement: Finishing an epic
Once every bead under an open epic is closed, finishing SHALL merge the epic's branch into the landing
target with every gate and the build and one minor version bump, land and push it without waiting,
close the epic and delete its branch. An epic whose work is already on the target and that has no
change to archive SHALL simply be closed.

#### Scenario: Open beads
- **WHEN** Ryder finishes an epic with an open bead
- **THEN** nacred refuses, naming the open beads

#### Scenario: Nothing to merge
- **WHEN** an epic has no branch, or its branch is already in the target, and it has no change to archive
- **THEN** nacred closes the epic without starting a round

#### Scenario: Finishing fails part-way
- **WHEN** archiving, a gate or the build fails before landing
- **THEN** the finishing round pauses with the reason and its choices, the target and the epic are untouched, and Finish can be started again once it is cancelled

#### Scenario: Closing after landing fails
- **WHEN** closing the epic or removing its branch fails after the target was pushed
- **THEN** the landing stands, the round's log says what failed, and Merge epic into main later closes the epic as having nothing left to merge

### Requirement: Finishing a change's epic archives the change
Finishing the epic of an approved specification change SHALL archive the change in the finishing
round's workspace and commit it before the final gates, so the archive lands on the target with the
epic's work, and the change's workflow SHALL end in the phase `archived`. The checkout SHALL NOT be
written by archiving or by adopting a change.

#### Scenario: Work already on the target
- **WHEN** a change's epic is finished and nothing is left on its branch
- **THEN** a round starts from the landing target, archives the change, passes the gates and build, lands and pushes

### Requirement: Needs you asks about unfinished endings
Needs you SHALL list an open epic whose beads are all closed with a Finish action, a change ready to
archive with Finish, and a bead in progress for a day that no run or round is working on, with Work
until done and Back to Ready.

#### Scenario: Forgotten claim
- **WHEN** a bead has been in progress for two days with nothing working on it
- **THEN** Needs you lists it as stuck in progress with Work until done and Back to Ready

### Requirement: Landing stops at pushing
Nacre SHALL NOT build or publish releases after landing; pushing the landing target SHALL be its
hand-off to the repository's own CI.

#### Scenario: A landed round
- **WHEN** a round lands with `land.push` on
- **THEN** nacred pushes the target and its tag and publishes nothing else

### Requirement: Run epic finishes the epic
A round started with `finish` on an epic SHALL, once it has landed on the epic's branch and the epic has
no open children, finish the epic without waiting for Ryder: merge into the landing target with every
gate and the build, archive the epic's change when it has one, push as `nacre.yaml` says, and close the
epic. When children remain open, the epic SHALL NOT be finished and Needs you SHALL say why.

#### Scenario: Whole epic in one action
- **WHEN** Ryder taps Run epic on an epic with three open children and every child passes
- **THEN** the three are merged into `epic/<id>`, the epic is merged into main, its change is archived and the epic is closed

#### Scenario: A child set aside
- **WHEN** one child is parked and the other two land
- **THEN** the epic stays open with its branch and Needs you says one child is unfinished
