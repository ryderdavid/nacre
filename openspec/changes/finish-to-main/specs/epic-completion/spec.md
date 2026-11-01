## ADDED Requirements

### Requirement: Work until done
The app SHALL offer Work until done first when a bead is implemented: a round of that bead which lands
when it passes unless Ryder switches that off. A plain run SHALL remain available as a secondary
choice that leaves checking, committing and closing to Ryder.

#### Scenario: One tap to finished work
- **WHEN** Ryder taps Work until done on a bead outside any epic
- **THEN** a round works it in its own worktree, checks every gate and the build, lands it on the landing target, pushes it and closes the bead without waiting for him

### Requirement: Epic work lands on the epic's branch
A round of beads under an epic SHALL land on the epic's branch, `epic/<id>`, made from the landing
target the first time, and SHALL neither bump nor tag the version. nacred SHALL refuse a round that
mixes beads of different epics, or epic beads with beads outside any epic.

#### Scenario: Main untouched mid-epic
- **WHEN** a round of an epic's bead lands
- **THEN** the epic's branch has the work, the landing target has not moved, and the bead is closed

#### Scenario: Mixed round
- **WHEN** Ryder starts a round with beads from two epics
- **THEN** nacred refuses it, naming the beads of each epic

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
