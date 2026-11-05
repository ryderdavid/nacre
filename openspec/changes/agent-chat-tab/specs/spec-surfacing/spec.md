## MODIFIED Requirements

### Requirement: Assistant proposes developing a proposal
Chat SHALL start proposal development for an existing bead, or for a bead it has just created, through nacred's develop endpoint, so intake and specflow's per-action limit apply. Development SHALL NOT wait for a separate approval card. The guards SHALL be the auto classifier, the chat's runaway ceiling and that limit. Chat's briefing SHALL tell it not to develop proposals for chores, small fixes or beads already linked to a change.

#### Scenario: New behavior-changing feature
- **WHEN** Ryder asks Chat for a feature that changes behavior an active change or accepted spec covers
- **THEN** Chat creates the bead and starts development through nacred, and the change appears in Proposals

#### Scenario: Auto-apply is on
- **WHEN** Chat asks nacred to develop a proposal
- **THEN** no separate approval card is shown, and the work is bounded by specflow's per-action limit and the chat's runaway ceiling

#### Scenario: Development refused after creation
- **WHEN** the bead is created but `develop` is refused
- **THEN** the bead remains and Chat reports why development didn't start

#### Scenario: Small fix
- **WHEN** Ryder asks Chat for a typo fix or a chore
- **THEN** Chat does the fix or files the bead without developing a proposal

### Requirement: Spec epics lead with their change
Bead detail for a change's epic SHALL show the following before the description:
- the change name, revision and phase
- the section strip with per-section evidence
- open human checks
- the next action

Unlinked beads SHALL offer "Develop a proposal" from the overflow menu instead of a Change card.

#### Scenario: Opening an epic mid-implementation
- **WHEN** Ryder opens an epic whose sections are partly verified
- **THEN** the first screen shows how many sections are verified and landed and the action to view the active round

#### Scenario: Unlinked bead
- **WHEN** Ryder opens the overflow menu of a bead with no change
- **THEN** it offers Develop a proposal
