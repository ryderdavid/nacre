## ADDED Requirements

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
A lease SHALL be released when the holding run's turn ends for any reason, and SHALL be revoked when the
holding run is flagged stalled.

#### Scenario: Stalled holder
- **WHEN** the run holding the emulator is flagged stalled
- **THEN** the lease is revoked and the next waiting run may take it

### Requirement: Runs are told their resource
A run holding a lease SHALL receive the resource's identity in its prompt and environment
(`NACRE_RESOURCE_<NAME>`), and SHALL be told to use only that one.

#### Scenario: Serial passed
- **WHEN** NAC-5 starts with the emulator lease for `emulator-5554`
- **THEN** its environment has `NACRE_RESOURCE_EMULATOR=emulator-5554` and its prompt names that serial
