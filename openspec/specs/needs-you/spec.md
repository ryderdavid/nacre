# needs-you Specification

## Purpose
One list, on the Boards screen and for the assistant, of everything on Ryder's computers that waits on him, each item saying why and offering its answers.
## Requirements
### Requirement: One list of what waits on the owner
nacred SHALL report, for every repository on its computer, each item waiting on Ryder: beads labelled
`human` or typed `decision`, proposals and stale or blocked reviews, interrupted workflows, round
questions, parked beads, rounds ready to land, changes ready to archive, found work, done epics, and beads
stuck in progress. Items SHALL be derived from current state, carry a stable id, and disappear when the
state no longer waits on him.

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
- **THEN** Needs you shows the epic as done, with Merge epic into main and Open epic

### Requirement: Items explain themselves
Every item SHALL include a title, a plain one- or two-sentence reason that stands on its own, its source reference when one exists, and its options, each either an exact API action, a navigation to the bead, change or round, or a prefilled sheet for answering found work.

#### Scenario: Unfamiliar question
- **WHEN** a round asks a question about a bead Ryder hasn't looked at recently
- **THEN** the item states the bead, what is being asked and why, and offers the answers without opening the transcript

### Requirement: Needs you on the Boards screen
The Boards screen SHALL show Needs you above the board summary whenever there are items, ordered questions, decisions and discovered work first, then approvals, then landing and archive, showing at most five before Show all. Answering inline SHALL call the item's action and refresh. The section SHALL be hidden when empty or when no computer supports it. Boards SHALL carry its own badge with the item count; the Runs badge SHALL keep its current meaning.

#### Scenario: Approve from the list
- **WHEN** Ryder taps Review and approve on a proposal item
- **THEN** the change view opens at that revision, ready for approval

#### Scenario: A human check on a section
- **WHEN** a spec section bead carries the `human` label
- **THEN** it appears as a bead decision item that opens the bead

#### Scenario: Nothing waiting
- **WHEN** no item waits on Ryder
- **THEN** Boards looks as it does today

### Requirement: The assistant starts from Needs you
The board assistant SHALL see the same list on both backends, through a read tool on the API backend and in the prompt context nacred supplies on the Mac backend, and SHALL use it first when Ryder asks what to work on or what needs him.

#### Scenario: What should I work on next
- **WHEN** Ryder asks what to work on next while a proposal awaits approval
- **THEN** the answer mentions the waiting proposal before suggesting new work

#### Scenario: Mac assistant
- **WHEN** the assistant runs on the Mac backend
- **THEN** it answers from the attention list in its prompt context without a read tool

