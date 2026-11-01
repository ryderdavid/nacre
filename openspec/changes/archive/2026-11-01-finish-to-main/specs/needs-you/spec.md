## MODIFIED Requirements

### Requirement: One list of what waits on the owner
nacred SHALL report, for every repository on its computer, each item waiting on Ryder: open beads labelled `human` or typed `decision`, proposals awaiting approval, blocking review findings, stale approvals, interrupted workflows, round questions, parked beads, rounds ready to land, changes ready to archive, discovered work awaiting triage, open epics whose beads are all closed, and beads in progress that nothing is working on. Items SHALL be derived from current state, carry a stable id, and disappear when the state no longer waits on him.

#### Scenario: Question answered elsewhere
- **WHEN** Ryder answers a round question from the round view
- **THEN** that item is gone from Needs you on the next refresh

#### Scenario: Discovered work
- **WHEN** an agent in a spec round leaves a `Found:` comment and its bead finishes
- **THEN** Needs you shows the note with the three found-work answers and Dismiss

#### Scenario: Two computers
- **WHEN** two paired computers report items
- **THEN** the app shows each item once, with its repository and computer

#### Scenario: An epic is done
- **WHEN** every bead under an open epic that belongs to no change is closed
- **THEN** Needs you shows the epic as done, with Finish epic and Open epic
