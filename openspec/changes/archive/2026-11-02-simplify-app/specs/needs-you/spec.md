## REMOVED Requirements

### Requirement: Needs you on the Boards screen
**Reason**: Needs you moves from the top of Boards to its own Inbox destination.
**Migration**: See "Inbox".

## ADDED Requirements

### Requirement: Inbox
The app SHALL show Needs you as its own Inbox destination beside Boards and Activity, ordered questions,
decisions and discovered work first, then approvals, then landing and archive. Answering inline SHALL
call the item's action and refresh. Inbox SHALL carry a badge with the item count, and the Boards
screen SHALL show the boards without Needs you items or totals above them.

#### Scenario: Approve from the list
- **WHEN** Ryder taps Review and approve on a proposal item in Inbox
- **THEN** the change view opens at that revision, ready for approval

#### Scenario: A human check on a section
- **WHEN** a spec section bead carries the `human` label
- **THEN** it appears as a bead decision item that opens the bead

#### Scenario: Boards first
- **WHEN** Ryder opens Boards while items wait in Inbox
- **THEN** the boards are the first thing on the screen and the Inbox badge shows the count

### Requirement: Only actionable items
nacred SHALL NOT list a Needs you item whose only action cannot run.

#### Scenario: Unrunnable stuck bead
- **WHEN** a bead has been in progress for a day in a repository without `nacre.yaml`
- **THEN** it is not listed in Inbox
