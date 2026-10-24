## ADDED Requirements

### Requirement: Start a round from a set of beads
nacred SHALL accept a request to start a round in one repository from an explicit list of bead ids or
from an epic (whose open children become the set). nacred SHALL refuse the request when the
repository has no `nacre.yaml`, when any bead does not exist or is closed, or when a bead is already in
another active round, and the refusal SHALL say which.

#### Scenario: Round from explicit beads
- **WHEN** Ryder starts a round with NAC-1, NAC-2 and NAC-3 in a repository that has `nacre.yaml`
- **THEN** nacred creates a round in state `planning` containing those three beads and labels each bead `round:<id>`

#### Scenario: Round from an epic
- **WHEN** Ryder starts a round from epic NAC-9 whose open children are NAC-9.1 and NAC-9.2
- **THEN** the round contains NAC-9.1 and NAC-9.2 and records NAC-9 as its epic

#### Scenario: Repository without gates
- **WHEN** Ryder starts a round in a repository without `nacre.yaml`
- **THEN** nacred refuses with a message saying the repository needs `nacre.yaml` before rounds can run

#### Scenario: Bead already in a round
- **WHEN** a requested bead is part of another round that is not landed or cancelled
- **THEN** nacred refuses and names the bead and the other round

### Requirement: Round lifecycle
A round SHALL move through the states `planning`, `needs_decision`, `implementing`, `integrating`,
`ready_to_land`, `landed`, and the terminal states `cancelled` and `paused`, and SHALL record every
transition as a round event with a timestamp. Each bead in a round SHALL have its own state:
`waiting`, `running`, `verifying`, `integrating`, `integrated`, `parked` or `skipped`.

#### Scenario: Happy path
- **WHEN** pre-flight raises no questions and every bead verifies and integrates
- **THEN** the round passes through planning, implementing, integrating and ends in `ready_to_land`

#### Scenario: Pre-flight question
- **WHEN** pre-flight raises a question
- **THEN** the round is `needs_decision` and no bead starts implementing until every question has an answer

### Requirement: Rounds run unattended and survive restarts
A round SHALL progress without any client connected. When nacred restarts, it SHALL reload each round
that is not terminal and continue from its last recorded state; a bead whose run was interrupted by
the restart SHALL be treated as a failed attempt and retried under the normal follow-up rules.

#### Scenario: Phone disconnects
- **WHEN** the app disconnects while a round is implementing
- **THEN** the round keeps running and the app sees the current state when it reconnects

#### Scenario: nacred restarts mid-round
- **WHEN** nacred restarts while two beads are running
- **THEN** after restart the round is still `implementing`, the two beads are resumed as follow-ups in their existing sessions, and the integrated beads stay integrated

### Requirement: What a round stops for
A round SHALL ask Ryder only when: pre-flight raises a question; a bead or an integration exhausts its
fix attempts; a budget or time limit is reached; or landing needs his approval. Every other condition
SHALL be handled without him. A parked bead SHALL NOT stop the other beads.

#### Scenario: One bead keeps failing
- **WHEN** NAC-2 fails its gates after every allowed follow-up
- **THEN** NAC-2 is `parked` with the failure recorded, and NAC-1 and NAC-3 continue to integration

#### Scenario: Budget reached
- **WHEN** the round's spending reaches its budget
- **THEN** the round becomes `paused`, running turns are allowed to finish but no new turn starts, and Ryder is notified with the spend so far

### Requirement: Budgets
Each round SHALL have a spending limit in USD and a wall-clock limit, defaulting from nacred's config
and overridable when the round starts. Every claude turn in the round SHALL be started with the
remaining budget as `--max-budget-usd`. Raising a limit on a paused round SHALL resume it.

#### Scenario: Per-turn budget
- **WHEN** a round with $20 budget has spent $14
- **THEN** the next turn is started with `--max-budget-usd 6`

#### Scenario: Resume after raising the budget
- **WHEN** Ryder raises the budget of a paused round
- **THEN** the round returns to the state it was paused in and continues

### Requirement: The record lives on the beads
nacred SHALL post, as comments on the affected beads: the pre-flight plan entry for that bead, each
decision Ryder makes that affects it, the result of its verification, and the outcome of its
integration (commit SHA or the reason it was parked).

#### Scenario: Integration recorded
- **WHEN** NAC-1 is merged into the integration branch at commit abc1234
- **THEN** NAC-1 has a comment saying it was integrated in round `<id>` at abc1234

### Requirement: Rounds can be started from three places
A round SHALL be startable from the nacred HTTP API, from the `nacred round` command line on the same
computer, and from the app (directly or by approving the board assistant's `start_round` proposal).
All three SHALL create the same kind of round.

#### Scenario: From a terminal
- **WHEN** a Claude Code session on the Mac runs `nacred round start NAC-1 NAC-2`
- **THEN** a round is created on the local nacred and the command prints its id and how to follow it

#### Scenario: From the assistant
- **WHEN** Ryder tells the board assistant to finish three beads and approves its `start_round` proposal
- **THEN** a round with those three beads is created and the assistant links to it

### Requirement: Cancel
Cancelling a round SHALL stop its running turns, leave the integration branch and bead branches in place,
remove the `round:<id>` labels and record the cancellation on each bead.

#### Scenario: Cancel mid-round
- **WHEN** Ryder cancels a round with one bead integrated and one running
- **THEN** the running turn is stopped, the round is `cancelled`, and both branches still exist so the work can be recovered
