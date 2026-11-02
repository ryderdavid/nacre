## REMOVED Requirements

### Requirement: Start a round from a board
**Reason**: Rounds start from a bead or an epic; the multi-select sheet is gone.
**Migration**: See "Start work from beads and epics".

## ADDED Requirements

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
