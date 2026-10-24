## ADDED Requirements

### Requirement: Merge one bead at a time
nacred SHALL integrate verified beads into the round's integration branch one at a time, respecting bd
dependencies and the plan's ordering, using a no-fast-forward merge performed by nacred itself. After
each merge it SHALL run the gates on the integration worktree.

#### Scenario: Clean merge
- **WHEN** NAC-1 is verified and merges without conflict
- **THEN** nacred creates a merge commit on `nacre/r/<round>`, runs the gates there, and on success marks NAC-1 `integrated`

#### Scenario: Serialized
- **WHEN** NAC-1 and NAC-3 are verified at the same moment
- **THEN** nacred merges one, runs the gates, and only then merges the other

### Requirement: Conflicts and post-merge failures go to an integration turn
When a merge conflicts, or the gates fail after a merge, nacred SHALL start a Claude turn in the
integration worktree, given the beads involved, the round's decisions, and the conflicted hunks or the gate
output. The turn SHALL resolve the problem and commit; nacred SHALL then re-run the gates. After
`fix_attempts` failed turns, nacred SHALL reset the integration branch to its pre-merge commit and park the
bead with the reason.

#### Scenario: Conflict resolved
- **WHEN** merging NAC-3 conflicts in `ReposScreen.kt`
- **THEN** an integration turn receives both beads' text and the conflict, commits a resolution, the gates pass, and NAC-3 is `integrated`

#### Scenario: Cannot reconcile
- **WHEN** the gates still fail after the last allowed integration turn for NAC-3
- **THEN** the integration branch is reset to the commit before NAC-3's merge, NAC-3 is parked with the reason, and the remaining beads continue

### Requirement: Agents never move branches
Merging, resetting, deleting branches and pushing SHALL be done by nacred's own git calls. Round agents
SHALL only edit and commit inside the worktree they were given.

#### Scenario: Integration turn
- **WHEN** an integration turn runs
- **THEN** it commits in the integration worktree, and nacred, not the agent, records the result and moves on

### Requirement: Finish the round on one verified commit
When every bead is integrated, parked or skipped, nacred SHALL run `version.bump` once on the integration
branch with the level `minor` if any integrated bead is a feature or an epic and `patch` otherwise, and
commit it, then run all gates and every `build` step there. If they pass, the round SHALL be
`ready_to_land` with its final commit SHA and artifact paths recorded; if they fail, nacred SHALL run an
integration turn under the same attempt limit.

#### Scenario: Ready to land
- **WHEN** all three beads are integrated and the final gates and build pass
- **THEN** the round is `ready_to_land`, with one version bump commit, the final SHA and the APK path recorded

#### Scenario: Feature bumps the minor version
- **WHEN** a round at 0.3.3 integrates one feature and two bugs
- **THEN** `version.bump` runs with `minor` and the version becomes 0.4.0, with the build number one higher

#### Scenario: Only fixes bump the patch
- **WHEN** a round at 0.3.3 integrates only bugs and tasks
- **THEN** `version.bump` runs with `patch` and the version becomes 0.3.4

#### Scenario: Parked bead does not hold the round
- **WHEN** NAC-2 is parked and NAC-1 and NAC-3 are integrated
- **THEN** the round finishes and becomes `ready_to_land` with NAC-1 and NAC-3, and NAC-2 stays open labelled `round-parked` with its reason

### Requirement: Close beads against the final commit
At `ready_to_land`, nacred SHALL close each integrated bead with a reason naming the round and final commit,
except beads that need a human check (labelled `human` or flagged in the plan), which SHALL stay open with a
comment saying what remains to verify. Parked beads SHALL stay open with their reason.

#### Scenario: Human check remains
- **WHEN** NAC-1tm is integrated but its acceptance criteria need a device with an API key
- **THEN** NAC-1tm stays open with a comment listing the checks left for Ryder, and the other beads are closed

### Requirement: Landing
Landing SHALL fast-forward `land.target` to the round's final commit and, when `land.push` is true, push the
target. If the target has moved since the round's base, nacred SHALL first merge the target into the
integration branch and re-run the gates and build, landing only if they pass. Landing SHALL wait for Ryder's
approval unless `land.auto` is true.

#### Scenario: Ryder lands
- **WHEN** Ryder approves landing a ready round whose target has not moved
- **THEN** `main` fast-forwards to the final commit and is pushed

#### Scenario: Target moved
- **WHEN** `main` gained a commit during the round
- **THEN** nacred merges `main` into the integration branch, re-verifies, and fast-forwards only if gates and build pass
