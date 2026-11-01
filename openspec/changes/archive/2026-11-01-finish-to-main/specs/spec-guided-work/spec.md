## MODIFIED Requirements

### Requirement: Evidence distinguishes progress from completion
The app SHALL distinguish mapped tasks, implementation outcomes, verified runs, human checks and landing. Checked task boxes or closed beads alone SHALL NOT establish requirement completion, except for sections recorded as finished before Nacre adopted a change written outside it, which SHALL count as landed only while every task in them stays checked and their bead stays closed, and SHALL say so. Missing work or unresolved checks SHALL leave the change epic incomplete. Archiving SHALL happen only through finishing the change's epic.

#### Scenario: Checked task without evidence
- **WHEN** tasks.md marks a section complete but no verified implementation evidence exists
- **THEN** the app reports the checkbox observation without claiming the requirement is verified

#### Scenario: Round parks a required section
- **WHEN** a round finishes with a required child parked
- **THEN** the change remains incomplete and shows the unfinished section

#### Scenario: Old successful run
- **WHEN** a previous spec revision has a verified run for the same bead
- **THEN** that result remains historical and does not verify the current revision

#### Scenario: Finished before adoption
- **WHEN** an adopted change's section had every task checked and its bead closed when it was adopted
- **THEN** it counts as landed, with a note that it was finished before adoption and has no round evidence, until a task is unchecked or its bead reopened
