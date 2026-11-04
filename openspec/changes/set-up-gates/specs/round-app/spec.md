## MODIFIED Requirements

### Requirement: Start work from beads and epics
The app SHALL start rounds from the work itself: Start work on a bead and Run epic on an epic, on the
repository's home computer or another computer in the group that has the repository. When the repository
has no `nacre.yaml` on that computer, Start work, Run epic and Approve and run SHALL open Set up gates
instead, and SHALL go on with the action once the file is saved. The board SHALL NOT offer a multi-select
sheet for picking beads across epics.

#### Scenario: Run an epic
- **WHEN** Ryder taps Run epic on NAC-9 with children NAC-9.1 to NAC-9.3
- **THEN** the app starts one round with those children and shows its progress on the epic

#### Scenario: No gates
- **WHEN** Ryder taps Start work in a repository without `nacre.yaml`
- **THEN** the app opens Set up gates, and after Ryder saves the drafted file the round starts
