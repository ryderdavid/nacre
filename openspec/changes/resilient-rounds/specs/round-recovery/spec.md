## ADDED Requirements

### Requirement: A recovery ladder before every stop
nacred SHALL try a recovery ladder whenever a round would stop (pause the round, or park a bead for a
reason other than its own run failing). It tries, in order and each at most as often as its bound allows: a mechanical fix
from the catalogue when one applies, a recovery turn in the round's workspace verified by the gates when
the failure is a merge or a gate failure, and a transient retry when the failure comes from the
environment. Only when no rung is left SHALL the round stop. Budget and time limits, and questions about
the beads' intent, SHALL stop the round without a ladder. Each rung tried SHALL be recorded as a `recover`
event with what was tried and the outcome.

#### Scenario: Fixed without stopping
- **WHEN** merging the target at landing conflicts only in the version lines both sides bumped
- **THEN** nacred applies the version fix, the gates pass, the round lands, and the log shows one `recover` event for it and no pause

#### Scenario: Ladder exhausted
- **WHEN** a landing merge conflicts in a source file no fix handles, and the recovery turn's resolution still fails the gates after `fix_attempts` follow-ups
- **THEN** the merge is undone, the round is paused with a diagnosis and choices, and the log shows each rung tried

#### Scenario: Ryder's decisions are not laddered
- **WHEN** the round's budget is used up
- **THEN** the round stops at once with its choices, without a recovery turn

### Requirement: Mechanical fixes catalogue
nacred SHALL resolve a merge mechanically only when every conflicted path is handled by a fix in the
catalogue, and SHALL run the gates after any mechanical fix. The catalogue SHALL include: the round's
version bump (undo it before merging the target and run `version.bump` again at the same level from the
target's version); branches whose every commit is already in the target by content (not merged, not
asked about); the beads logs `.beads/*.jsonl` (union of both sides' lines, duplicates dropped, order
kept); and files declared in `nacre.yaml` under `merge.regenerate` as a path pattern with the command
that rebuilds them (take the target's side, run the command, add the result).

#### Scenario: Beads log conflict
- **WHEN** a bead merge conflicts only in `.beads/interactions.jsonl`
- **THEN** the merged file holds every line from both sides once, and the merge completes without a recovery turn

#### Scenario: Declared lockfile
- **WHEN** `nacre.yaml` declares `merge.regenerate: [{path: "android/gradle.lockfile", run: "cd android && ./gradlew -q dependencies --write-locks"}]` and a merge conflicts only in that file
- **THEN** nacred takes the target's side, runs the command, adds the regenerated file and completes the merge

#### Scenario: Mixed conflict
- **WHEN** a merge conflicts in `build.gradle.kts` version lines and in `Rounds.kt`
- **THEN** no mechanical fix is applied, and the merge goes to a recovery turn

### Requirement: Claude resolves every merge in a round
nacred SHALL give a recovery turn, like a bead merge's integration turn, to the merge of unmerged work
Ryder chose to bring in before the beads start, the merge of a moved target before landing, and a
landing whose target's changes make the gates fail. The turn is told what each side meant (the branch's commits and the pre-flight
finding, the target's commits since the round's base, the beads and decisions), told never to revert the
target's commits and to keep the target's version where both sides made an equivalent change, bounded
by `limits.fix_attempts` and the round's remaining budget, and judged only by the gates.

#### Scenario: Pre-flight merge resolved
- **WHEN** Ryder answers "Merge it into the round first" and the branch conflicts with the base in one file
- **THEN** a recovery turn resolves it on the integration branch, the gates pass, and the beads start on top of it

#### Scenario: Landing merge resolved
- **WHEN** the target gained a commit that touches the same function a bead changed, and the merge at landing conflicts there
- **THEN** a recovery turn resolves it, the final gates and build pass, and the round lands

### Requirement: Transient failures are retried
nacred SHALL retry a failure classified as environmental twice, after 5 seconds and after 30 seconds,
before the bead is parked or the round stops. Environmental failures are a process that can't start, `bd`
failing on a lock or I/O error, `git` failing on a locked index or busy worktree, and a dropped connection
to Claude.

#### Scenario: Locked index
- **WHEN** creating a bead's worktree fails with "index.lock exists" and succeeds on the second try
- **THEN** the bead starts normally and the log shows the retry

#### Scenario: Real failure
- **WHEN** creating a bead's worktree fails because its branch name is invalid
- **THEN** it isn't retried, and the bead is parked with that reason

### Requirement: Diagnosis at a stop
When the ladder is exhausted, nacred SHALL make one read-only Claude call (read-only `bd` commands and
read access to the repository, no writes) with the stop's context, the round's recent events and the
relevant git state, and SHALL show its plain-language summary of what is wrong and an explanation for each
choice on the stop. The call SHALL be capped at $0.50 of the round's budget and 3 minutes, and SHALL be
skipped, showing the plain reason, when the budget can't cover it or the call fails.

#### Scenario: Diagnosed stop
- **WHEN** the landing merge stops after the ladder
- **THEN** the round's stop shows a summary such as "main changed `transcript.go` after this branch was written; both edit `ParseLine`", and under each choice what it would do

#### Scenario: No budget for a diagnosis
- **WHEN** the round stops because its budget is used up
- **THEN** no diagnosis call is made, and the stop shows the plain reason with its choices

### Requirement: Stops are choices that never replay a failure
A paused round SHALL expose the choices that apply to its stop, each one carried out by nacred:
`retry-with-budget` (another recovery turn with an amount Ryder sets), `skip-branch` (on a stop about a
pre-flight branch), `skip-bead` (on a stop about one bead), `continue` (Ryder changed something by hand),
`raise-limits` and `cancel`. A round SHALL NOT offer to land only part of its beads; it lands without a
bead only when that bead was parked because it couldn't be finished. A stop for budget or time SHALL show
what each bead has spent and an estimate of what the unfinished beads need, from the average cost of the
finished ones. `continue` SHALL first compare the state
that made the step fail (the commits and files involved) with now, and if nothing changed SHALL refuse
with "Nothing has changed since this failed" and leave the round paused. The existing resume action SHALL
behave as `continue`.

#### Scenario: Resume without a change
- **WHEN** the landing merge stopped on a conflict and Ryder taps Resume without changing anything
- **THEN** the merge isn't attempted again, and the app says nothing has changed since it failed and shows the other choices

#### Scenario: Skip the branch
- **WHEN** a pre-flight merge of `origin/old-work` stops after the ladder and Ryder chooses `skip-branch`
- **THEN** the question is recorded as "Ignore it", the integration branch is as it was, and the beads start

#### Scenario: Budget runs out partway
- **WHEN** two of three beads are integrated at $8 each and the round's $25 budget runs out while the third is running
- **THEN** the stop offers raising the limit with an estimate of about $8 more, or cancelling, and no choice to land the two on their own

#### Scenario: A bead that can't be finished
- **WHEN** the third bead still fails its gates after its follow-ups and recovery turn
- **THEN** the round parks it without asking, finishes with the other two, and the Land card lists the parked bead and why

### Requirement: Set-aside work carries into the next round
nacred SHALL continue a bead from its kept round branch when a new round includes a bead that an earlier
round parked or left behind by being cancelled, and that branch has commits beyond its own base. The new
bead branch SHALL start from the new round's integration branch with the kept branch merged in, through
the same mechanical fixes and recovery turn as any merge; the bead's prompt SHALL say its earlier work is
present and why it was set aside; and the kept branch SHALL be deleted once the new round lands.

#### Scenario: Picking up parked work
- **WHEN** NAC-7 was parked in round A with two commits on `nacre/r/A/NAC-7`, and round B includes NAC-7
- **THEN** NAC-7's branch in round B contains those two commits on top of B's base, its run is told what they are and why A parked it, and `nacre/r/A/NAC-7` is deleted after B lands

#### Scenario: Nothing to pick up
- **WHEN** round B includes a bead whose kept branch in round A has no commits beyond its base
- **THEN** the bead starts fresh from B's base

### Requirement: The app shows stops and recoveries
The round screen SHALL show a stop's diagnosis summary, one button per choice with its explanation, and
the amount field for `retry-with-budget`. Recovery events SHALL appear in the round's log with the rung
and outcome. A notification SHALL be sent only when the round actually stops, not for a recovery that
succeeded.

#### Scenario: Stop card
- **WHEN** a round stops on a landing conflict after the ladder
- **THEN** the card shows the diagnosis, and buttons for trying again with more budget, continuing after a manual fix, and cancelling

#### Scenario: Quiet recovery
- **WHEN** a version conflict at landing is fixed mechanically
- **THEN** the log shows the recovery, and Ryder gets the "landed" notification only
