## MODIFIED Requirements

### Requirement: Rounds can be started from three places
A round SHALL be startable from:
- the nacred HTTP API
- the `nacred round` command line on the same computer
- the app, either directly or from Chat

All three SHALL create the same kind of round.

#### Scenario: From a terminal
- **WHEN** a Claude Code session on the Mac runs `nacred round start NAC-1 NAC-2`
- **THEN** a round is created on the local nacred and the command prints its id and how to follow it

#### Scenario: From the assistant
- **WHEN** Ryder tells Chat to finish three beads
- **THEN** a round with those three beads is created and Chat links to it
