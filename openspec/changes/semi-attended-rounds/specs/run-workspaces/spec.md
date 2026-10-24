## ADDED Requirements

### Requirement: Integration branch from a recorded base
When a round starts, nacred SHALL record the base commit (the HEAD of the round's target branch unless
another is given), create the integration branch `nacre/r/<round>` at that commit, and create a worktree
for it outside the repository directory.

#### Scenario: Base recorded
- **WHEN** a round targeting `main` starts while `main` is at bd299b7
- **THEN** the round records base bd299b7 and `nacre/r/<round>` points at bd299b7

### Requirement: Each bead gets its own branch and worktree
nacred SHALL create, for each bead when it starts, a branch `nacre/r/<round>/<bead>` from the integration
branch's current head and a worktree for it, sharing the repository's beads database. The run SHALL
execute with that worktree as its working directory, and SHALL NOT be started with `claude --worktree`.

#### Scenario: Bead starts on the latest integration
- **WHEN** NAC-2 starts after NAC-1 has been integrated
- **THEN** NAC-2's branch starts at the integration commit that contains NAC-1

#### Scenario: No Claude-made worktrees
- **WHEN** a round run is started
- **THEN** the claude command line has no `--worktree` flag and its working directory is the bead's worktree

### Requirement: Runs report their workspace
A run created by a round SHALL report its `round_id`, `branch`, `workspace` (path) and `base` (commit) in
the API, and keep them after the run ends.

#### Scenario: API shows the branch
- **WHEN** the app fetches a round run
- **THEN** the run includes its round id, branch name, worktree path and base commit

### Requirement: Clean up after landing
nacred SHALL remove a bead's worktree once the bead is integrated or skipped, and SHALL delete the round's
branches and remaining worktrees once the round is landed. Branches of parked beads and of cancelled
rounds SHALL be kept. On startup, nacred SHALL remove worktrees whose round is landed or no longer exists.

#### Scenario: After integration
- **WHEN** NAC-1 has been merged into the integration branch
- **THEN** NAC-1's worktree directory is removed and its branch is kept until the round lands

#### Scenario: After landing
- **WHEN** the round has landed
- **THEN** `nacre/r/<round>` and every `nacre/r/<round>/*` branch except parked ones are deleted, and no worktree for the round remains

#### Scenario: Leftover after a crash
- **WHEN** nacred starts and finds a worktree for a round that is landed
- **THEN** it removes the worktree and logs that it did
