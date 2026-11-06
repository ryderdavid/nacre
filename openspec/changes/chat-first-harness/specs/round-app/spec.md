# Spec Delta

## MODIFIED Requirements

### Requirement: Round screen
The app SHALL show each running round or run as a work card, in the status strip and in the thread where
it started. The card shows determinate progress (beads landed out of the total), its gates and its spend.
The card's Stop SHALL say its scope: "Stop this reply" stops only the chat turn, and "Cancel round and its
workers" cancels the round after a confirmation that says what keeps running.

Tapping the card SHALL open the work inspector. It shows:
- the round's state, spend against budget and elapsed time
- each bead's state, branch and verification result, with a link to its run
- the gates, failures, artifacts and changes
- the live integration log

It offers direct controls that work without the agent: per-bead Retry, the server's recovery choices,
Cancel round and the full log.

#### Scenario: Live progress
- **WHEN** a bead is merged while the work inspector is open
- **THEN** the bead's state changes to integrated and the log gains the merge without a manual refresh

#### Scenario: Retry without the agent
- **WHEN** a bead failed its gates and the chat's agent is not running
- **THEN** Retry in the work inspector retries it

### Requirement: Answer decisions
When a round is `needs_decision`, the app SHALL show each question as a question card with the beads it
concerns, the suggested answers with their consequences, and an "In my own words" answer. It SHALL accept
a chosen or typed answer. The plan's ordering SHALL be shown in the work inspector, and reordering or
unlinking overlapping beads before implementation starts SHALL be asked for in chat.

#### Scenario: Answer a question
- **WHEN** Ryder picks an answer on the only open question card and taps Send answer
- **THEN** the app sends it, and the round moves on to implementing

### Requirement: Land
A round that is `ready_to_land` SHALL raise a reviewed-action decision. The decision shows:
- the final commit
- the build artifact's path as copyable text
- the beads that will close and those that remain open, with why
- the target branch, and whether landing will push (which nacred resolves from the round's `nacre.yaml`)
- a link to review the round's commits and changed files

Its options SHALL name these effects, for example "Merge into main and push" or "Merge, don't push".

#### Scenario: Land a round
- **WHEN** Ryder taps "Merge into main and push" on a round's land card
- **THEN** nacred lands the round and the card shows the result

#### Scenario: A round that doesn't push
- **WHEN** the repository's `nacre.yaml` sets `land.push: false`
- **THEN** the card says it moves main on the computer only and doesn't push, and its option reads "Merge, don't push"

#### Scenario: Push not reported
- **WHEN** nacred's round doesn't say whether landing pushes (an older nacred)
- **THEN** the card describes pushing as depending on `nacre.yaml`

#### Scenario: Review before landing
- **WHEN** Ryder opens the card's changes link
- **THEN** the app lists the round's commits and each changed file with its added and deleted lines since the round's base
