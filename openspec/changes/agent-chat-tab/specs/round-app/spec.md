## MODIFIED Requirements

### Requirement: Assistant can propose a round
Chat SHALL be able to start a round of a list of beads, or of an epic, with an optional budget, through nacred's API or the `nacred round` command, without a separate approval card. Chat SHALL link to the round once it starts.

#### Scenario: Assistant proposes
- **WHEN** Ryder asks Chat to finish everything ready under an epic
- **THEN** Chat starts a round with those beads through nacred, the round runs within its own budget, and Chat links to it
