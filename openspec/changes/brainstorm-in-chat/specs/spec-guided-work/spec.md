## ADDED Requirements

### Requirement: The author sees the brainstorm
Every author call and every review call SHALL receive the seed bead's notes. When the bead came from a New effort
draft, those notes hold the brainstorm digest.

#### Scenario: Brainstorm digest
- **WHEN** a change is developed from a bead whose notes hold a digest saying "ruled out: a new tab"
- **THEN** the author's and reviewer's input includes that digest

### Requirement: Owner feedback accumulates
Every author call and every review call SHALL receive every piece of feedback Ryder has given the change through
develop and revise requests, oldest first, each marked with when it was given. New feedback SHALL be added, not
replace earlier feedback. The history SHALL keep the first entry and the most recent ones when it passes its bound.

#### Scenario: Third revision
- **WHEN** Ryder sends a change back three times, with a different note each time
- **THEN** the third author call sees the intake feedback and all three notes, oldest first

#### Scenario: Long history
- **WHEN** a change has more feedback entries than the bound
- **THEN** the author sees the first entry and the most recent entries, and is told how many in between were left out
