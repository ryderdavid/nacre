# repo-gates Specification

## Purpose
Today every bead runs on its own: nacred starts one `claude -p` per bead, the run "succeeds" when the process exits, and nobody owns putting the pieces back together. In one week that produced work built on a stale base (NAC-eky on 0.2.1 because `claude --worktree` branched from an unmerged `main`), finished work left uncommitted (NAC-j5z.6), a release branch that sat unmerged while runs kept fork…
## Requirements
### Requirement: nacre.yaml declares what done means
A repository SHALL declare its gates in a `nacre.yaml` file at its root, and nacred SHALL reject a
malformed file with the line and reason.

#### Scenario: Valid file
- **WHEN** nacred reads a `nacre.yaml` with two gates and one build step
- **THEN** it exposes them in the repository's API entry and uses them for rounds in that repository

#### Scenario: Malformed file
- **WHEN** `nacre.yaml` has a gate without `run`
- **THEN** starting a round fails with a message naming the gate and the missing field

#### Scenario: What nacre.yaml holds
- **WHEN** nacred reads `nacre.yaml`
- **THEN** it accepts `gates` (named commands that must all exit 0, each optionally `final_only` to run only on the round's final commit), `build` (named commands run at the end of a round, each optionally naming an `artifact` path), `env` (environment variables for those commands), `version.bump` (a command run once per round, in which `{level}` is replaced by `patch` or `minor`), `resources` (exclusive resources), `land` (`target`, `auto`, `push`) and `limits` (`gate_timeout_minutes`, `fix_attempts`)

### Requirement: nacred runs the gates itself
nacred SHALL run gates in the relevant worktree with the declared environment, each with the gate timeout,
and SHALL record for each gate: exit status, duration, and the last 200 lines of combined output. A gate
SHALL count as failed on a non-zero exit or a timeout.

#### Scenario: Gate times out
- **WHEN** a gate is still running after `gate_timeout_minutes`
- **THEN** nacred kills its process group and records the gate as failed by timeout

#### Scenario: Output kept
- **WHEN** a gate fails
- **THEN** the run's verification result includes the gate's name, exit status and the tail of its output

### Requirement: A bead's run is done only when verified
A round run's turn that ends successfully SHALL be followed by verification: the worktree has no
uncommitted changes, the branch has at least one commit beyond its base, and every gate passes. Only a
verified run SHALL be offered to integration. A run's `verified` field SHALL report the outcome.

#### Scenario: Uncommitted work
- **WHEN** a run ends with modified files not committed
- **THEN** verification fails with "uncommitted changes" and lists the files

#### Scenario: Nothing committed
- **WHEN** a run ends with no commits beyond its base
- **THEN** verification fails with "no commits"

#### Scenario: All good
- **WHEN** a run ends with a clean tree, two commits, and every gate passing
- **THEN** the run is `verified` and its bead moves to integration

### Requirement: Failed verification is followed up automatically
When verification fails, nacred SHALL resume the same Claude session with a message stating exactly what
failed (the files, or the gate name and output tail), up to `fix_attempts` times, re-verifying after each.
After the last attempt fails, the bead SHALL be parked with the last failure as its reason.

#### Scenario: Fixed on follow-up
- **WHEN** verification fails on `app tests` and the follow-up turn fixes the test
- **THEN** the second verification passes and the bead moves to integration

#### Scenario: Out of attempts
- **WHEN** verification still fails after `fix_attempts` follow-ups
- **THEN** the bead is parked with the last failure and Ryder is notified

### Requirement: Stalls in a round are recovered once
When a run in a round is flagged stalled, nacred SHALL cancel it and resume the session once with a message
asking it to continue from where it was. A second stall of the same bead SHALL park it.

#### Scenario: First stall
- **WHEN** a round run has printed nothing for the stall window
- **THEN** nacred cancels it and starts a follow-up turn in the same session without asking Ryder

### Requirement: The round's runs know the contract
Every run in a round SHALL be told, in its prompt: its bead, the round's decisions that affect it, the
gate commands it will be checked against, that it must commit its work on its branch, and that it must not
merge, push, close the bead, or change shared artifacts named by pre-flight (such as the version).

#### Scenario: Prompt contents
- **WHEN** a round run for NAC-2 starts
- **THEN** its prompt contains NAC-2's id, the gate commands from `nacre.yaml`, the instruction to commit on `nacre/r/<round>/NAC-2`, and the list of things it must not do

