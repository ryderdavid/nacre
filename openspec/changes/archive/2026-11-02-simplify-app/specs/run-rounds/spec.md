## ADDED Requirements

### Requirement: Goal-driven workers fan out
Every bead in a round SHALL be worked by its own goal-driven session in its own worktree and branch,
whose goal is that bead and its acceptance criteria, so the session keeps working until they hold and
nacred's checks pass. A round SHALL NOT run one session for all of its beads. Goal options sent by
older apps SHALL NOT change this, and whether nacred pushes SHALL follow only `nacre.yaml`.

#### Scenario: Three beads, three workers
- **WHEN** a round of three independent beads starts
- **THEN** three goal-driven sessions run, each in its own worktree, and each bead is verified and integrated on its own

#### Scenario: Older app asks to push
- **WHEN** an older app starts a round with the pushed goal option and `nacre.yaml` turns pushing off
- **THEN** the round lands without pushing
