## MODIFIED Requirements

### Requirement: Work until done
The app SHALL offer one primary action to get work done: Start work on a bead and Run epic on an epic
with open children. Start work SHALL start a round of that bead which lands when it passes. Run epic
SHALL start a round of every open child of the epic that lands on the epic's branch and then finishes
the epic. A plain agent run SHALL remain available only from the bead's overflow, as Run an agent
manually…, and leaves checking, committing and closing to Ryder. The app SHALL NOT offer goal,
committed or pushed options when starting work.

#### Scenario: Approved specification sections
- **WHEN** Ryder runs an epic whose children are approved specification sections
- **THEN** nacred resolves the approved change and revision itself and the round lands without waiting

#### Scenario: One tap to finished work
- **WHEN** Ryder taps Start work on a bead outside any epic
- **THEN** a round works it in its own worktree, checks every gate and the build, lands it on the landing target, pushes it when `nacre.yaml` says so and closes the bead without waiting for him

#### Scenario: Manual run
- **WHEN** Ryder chooses Run an agent manually… from a bead's overflow
- **THEN** the app offers the agent, computer, permission, model, effort, worktree and instruction choices and starts a plain run

## ADDED Requirements

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
