# spec-surfacing Specification

## Purpose
The specification workflow shows up where Ryder decides: in the assistant, in round pre-flight, in Needs you, and on board cards and epics.

## Requirements

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

### Requirement: Preflight reports spec drift
Round preflight SHALL compare each ordinary round bead with an index of accepted and active requirements and SHALL report a `spec-drift` finding when a bead would change behavior a cited requirement specifies. A finding without a citation present in the index SHALL be discarded. Spec section beads and found beads SHALL be exempt, and repositories without a valid OpenSpec setup SHALL skip the check.

#### Scenario: No OpenSpec in the repository
- **WHEN** a round runs in a repository without a valid OpenSpec setup
- **THEN** preflight raises no spec-drift findings and behaves as it does today

#### Scenario: Oversized index
- **WHEN** the requirement index exceeds the preflight budget
- **THEN** preflight truncates whole capabilities and the plan states which were not checked

### Requirement: Spec drift holds the round for a decision
Each spec-drift finding SHALL become a question that holds the round like any preflight question, with the answers amend the spec first, proceed as an exception, or leave it out of the round.

#### Scenario: Bead contradicts a requirement
- **WHEN** a round bead wants Enter to send and a requirement says Enter inserts a newline
- **THEN** the round waits for Ryder's decision, citing the requirement's path, before any bead starts

#### Scenario: Leave it out
- **WHEN** Ryder answers leave it out of this round
- **THEN** the bead is skipped with a reason posted to it, and beads in the round that build on it are skipped too

#### Scenario: Amend the spec first
- **WHEN** Ryder answers amend the spec first
- **THEN** the bead leaves the round and proposal development starts for it with the cited requirement as feedback

#### Scenario: Amendment can't start
- **WHEN** Ryder answers amend the spec first and development is refused
- **THEN** the question reopens with the reason and the bead stays in the round

#### Scenario: Proceed as an exception
- **WHEN** Ryder answers proceed as an exception
- **THEN** the decision is posted on the bead, the bead gains a `spec-touch:` label, and the bead is still treated as ordinary work by run and round guards

### Requirement: Board cards show spec state and evidence
Bead cards SHALL show at most one spec chip, in the first label position: the change phase on seeds and epics, the section and its evidence (none, in round, verified, landed) on section beads, "closed, not verified" on spec-linked closed beads without verified evidence, and "Touches a spec" on beads with a `spec-touch:` label. Epics SHALL show a section strip. Cards without spec state SHALL be unchanged.

#### Scenario: Closed without evidence
- **WHEN** a section bead is closed but has no verified run for the current revision
- **THEN** its card says closed, not verified

#### Scenario: Older bridge
- **WHEN** the computer does not provide the changes summary
- **THEN** cards render as today without spec chips

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

### Requirement: Stale approvals name their cause
When a ready or awaiting-approval change no longer matches its approved context, the change SHALL be shown as stale with a reason naming the files that changed, and SHALL offer a new review. Approval SHALL NOT be restored without review.

#### Scenario: Another change was edited
- **WHEN** an active change's artifacts change after this change was approved
- **THEN** this change shows Review needs updating with a reason naming that change, before any round is attempted

#### Scenario: Round started on a stale change
- **WHEN** Ryder opens the round sheet for a change that became stale
- **THEN** the sheet explains the stale reason instead of failing with a conflict

### Requirement: Ready to archive is visible
When every current section of the approved revision has landed evidence with matching provenance, no section bead is reopened or awaiting a human check, and no found bead under the epic is open, the app SHALL report the change as ready to archive and SHALL offer Finish, which archives the change and closes its epic through a finishing round. It SHALL NOT archive or close the epic without that action.

#### Scenario: All sections landed
- **WHEN** the last section's round lands
- **THEN** Needs you shows the change as ready to archive, with Archive spec and merge into main and Open change

#### Scenario: A section was reopened
- **WHEN** a landed section bead is reopened
- **THEN** the change is no longer reported as ready to archive

#### Scenario: Other specifications moved on
- **WHEN** another change is added or archived after approval, so the approval is stale, but this change's own files are still exactly the approved ones
- **THEN** the change is still reported as ready to archive, saying what changed elsewhere

### Requirement: Spec notifications
nacred SHALL publish change phase transitions as events, and the app SHALL notify once when a change becomes ready for Ryder's review and once when it becomes ready to archive, on the existing round notification channel. While a proposal is being drafted, reviewed or promoted the app SHALL keep watching for these events as it does for active runs.

#### Scenario: Review finishes while the phone is away
- **WHEN** a proposal reaches awaiting approval while the app is in the background
- **THEN** Ryder receives one Proposal ready for your review notification that opens the change

#### Scenario: Nothing was being watched
- **WHEN** a change becomes ready to archive while the app watches nothing
- **THEN** the notice appears on the app's next refresh
