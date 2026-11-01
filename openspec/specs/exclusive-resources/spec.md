# exclusive-resources Specification

## Purpose
Today every bead runs on its own: nacred starts one `claude -p` per bead, the run "succeeds" when the process exits, and nobody owns putting the pieces back together. In one week that produced work built on a stale base (NAC-eky on 0.2.1 because `claude --worktree` branched from an unmerged `main`), finished work left uncommitted (NAC-j5z.6), a release branch that sat unmerged while runs kept fork…
## Requirements
### Requirement: Declare exclusive resources
A repository SHALL be able to declare exclusive resources in `nacre.yaml` (for example `emulator`), each
optionally with an identity passed to runs (such as an emulator serial). A bead SHALL need a resource when
the plan says so or when it carries the label `needs:<resource>`.

#### Scenario: Bead needs the emulator
- **WHEN** NAC-5 carries `needs:emulator`
- **THEN** the round treats NAC-5 as needing the `emulator` lease

### Requirement: One holder per resource
nacred SHALL grant each resource to at most one run at a time. A run that needs a held resource SHALL wait
in the round's queue without counting against concurrency, and SHALL start when the lease is free.

#### Scenario: Two beads need the emulator
- **WHEN** NAC-5 and NAC-6 both need the emulator and NAC-5 holds it
- **THEN** NAC-6 waits until NAC-5's turn ends, then starts with the lease

### Requirement: Leases end
A bead SHALL hold its leases from the start of its first turn until it is verified or parked, because its
verification may use the resource too, and SHALL release them then. A stalled holder SHALL be stopped and
resumed once, and parked if it stalls again, which releases its leases.

#### Scenario: Released after verification
- **WHEN** the bead holding the emulator passes verification
- **THEN** the lease is released and the next waiting bead may take it

#### Scenario: Stalled holder
- **WHEN** the run holding the emulator stalls a second time
- **THEN** its bead is parked, the lease is released, and the next waiting bead may take it

### Requirement: Runs are told their resource
A run holding a lease SHALL receive the resource's identity in its prompt and environment
(`NACRE_RESOURCE_<NAME>`), and SHALL be told to use only that one.

#### Scenario: Serial passed
- **WHEN** NAC-5 starts with the emulator lease for `emulator-5554`
- **THEN** its environment has `NACRE_RESOURCE_EMULATOR=emulator-5554` and its prompt names that serial

### Requirement: Computer-wide limits
Each computer's nacred config SHALL set how many Claude runs may be active at once
(`max_concurrent_runs`) and how many gate or build commands may run at once (`max_concurrent_gates`).
Both limits SHALL apply across everything on that computer: single runs and every round's runs,
verifications, integrations and final builds. A run or gate over the limit SHALL wait its turn rather than
fail. The defaults SHALL be 3 runs and 2 gates; a smaller computer lowers them in its own config.

#### Scenario: Gates queue across rounds
- **WHEN** `max_concurrent_gates` is 2, two beads in one round are running their gates, and a single run's verification is requested
- **THEN** the third gate run waits until one of the two finishes

#### Scenario: Single runs count
- **WHEN** `max_concurrent_runs` is 3, two round runs are active and Ryder starts a single "Run with Claude Code"
- **THEN** the single run starts, and the round's next bead waits until a slot frees up

#### Scenario: Smaller computer
- **WHEN** the work laptop's config sets `max_concurrent_runs: 2` and `max_concurrent_gates: 1`
- **THEN** that laptop's nacred never runs more than two Claude runs or one gate at a time

