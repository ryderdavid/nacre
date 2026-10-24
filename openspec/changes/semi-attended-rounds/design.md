## Context

nacred runs one `claude -p --output-format stream-json` per bead (`internal/runs`), optionally with
`--worktree`, which makes Claude Code create a worktree from the repository's default branch. nacred
does not learn the branch or path, does not look at the bead's dependencies, and treats exit status 0
with a `result` line as success. Up to `max_concurrent_runs` (3) run at once. Nothing merges, verifies,
or cleans up; that has been done by hand in a separate Claude Code session.

What went wrong in practice (all during the 0.3 work):

| Symptom | Cause |
|---|---|
| NAC-eky built on 0.2.1 | `--worktree` branched from `main`; the real work was on an unmerged branch |
| NAC-j5z.6 work uncommitted | Nothing checks the tree after a run; the run still "succeeded" |
| integration-0.3 unmerged for a day | No step owns landing; new runs kept forking from `main` |
| NAC-1tm "running" for 4 h | Dead API stream; now flagged by the stall check, not recovered |
| Two runs on one emulator | No notion of a shared resource |
| Every run bumped versionName | Version is shared state with no owner |
| Cleanup blocked | Destructive git (`branch -D`, `push --delete`) is refused to agents in auto mode |

Beads (bd 1.2.2) already provides dependencies, `bd ready`, `bd swarm` (epic DAGs), `bd merge-slot`,
gates and `bd worktree`. The Mac is the only place that can run the work unattended; the phone comes and
goes.

## Goals / Non-Goals

**Goals:**
- Ryder names N beads in one repository and gets back one commit that contains all of them, passes the
  repository's gates and builds, without supervising the middle.
- Conflicts of intent are found and decided before any code is written.
- Every claim of "done" is checked by nacred, not taken from the agent.
- The round survives the phone disconnecting, nacred restarting, and individual runs hanging or failing.
- Rounds stop only for: decisions, repeated failures, budget, and acceptance criteria only a human can
  check.

**Non-Goals:**
- Multi-repository or multi-computer rounds; PRs and CI; non-Claude agents (see proposal).
- Replacing single-bead runs. Rounds are built on the same run machinery.
- A general workflow engine. Rounds are one fixed pipeline.

## Decisions

### 1. Rounds live in nacred, persisted on the Mac
The round is a state machine in a new `internal/rounds` package, persisted as
`<data_dir>/rounds/<id>/round.json` plus an append-only `events.jsonl`, like runs. On restart, a round
resumes from its last durable state; runs that were active are marked failed by the run manager as
today, and the round treats that as a retryable failure.
*Alternatives:* the phone driving the round (dies with the connection, and is exactly the "attended"
mode we're leaving); a long-lived Claude Code session as orchestrator (it hangs, it is subject to the
permission classifier, and its state is a transcript, not data).

### 2. Beads carry the record, nacred carries the state
Each bead in a round gets the label `round:<id>`. The pre-flight plan, Ryder's decisions, verification
results and the integration outcome are written as bead comments, so the history stays with the work
and is visible in any bd client. Round state (phase, queue, leases, SHAs) is nacred's.
*Alternative:* model the round as a `bd swarm` molecule. Swarms need an epic whose children form the
DAG; Ryder's five beads usually don't share one, and the swarm doesn't hold workspace or SHA state.
Rounds can still be started from an epic (its open children become the set).

### 3. nacred creates the workspaces
For a round `r`, nacred creates the integration branch `nacre/r/<id>` at the recorded base SHA (default:
the target branch's HEAD at round start) and a worktree for it. Each bead gets
`nacre/r/<id>/<bead>` and a worktree under `<data_dir>/worktrees/<repo>/<round>/<bead>` (outside the
repository, so discovery never sees it), created with `bd worktree create` so the beads database is
shared. A bead's branch starts from the integration branch *as it is when the bead starts*, which is
how dependencies and serialized overlaps see their prerequisites. claude runs with that worktree as its
cwd and without `--worktree`.
*Alternative:* keep `--worktree` and discover the path from the `init` line. That still branches from
the default branch, and nacred can't control the base.

### 4. `nacre.yaml` is the definition of done
A checked-in file at the repository root:

```yaml
gates:                       # every one must exit 0
  - name: bridge
    run: cd bridge && go vet ./... && go test ./...
  - name: app tests
    run: cd android && ./gradlew -q testDebugUnitTest
build:                       # run on the integration branch at the end of a round
  - name: release apk
    run: cd android && ./gradlew -q assembleRelease
    artifact: android/app/build/outputs/apk/release/app-release.apk
env:
  JAVA_HOME: /Applications/Android Studio.app/Contents/jbr/Contents/Home
version:
  bump: scripts/bump-version.sh   # once per round, on the integration branch
resources: [emulator]
land:
  target: main
  auto: false
  push: true
limits:
  gate_timeout_minutes: 20
  fix_attempts: 2
```

nacred runs gates with a timeout, captures the tail of their output, and records results on the run and
the bead. The prompt for a round run tells the agent the same gate commands, so it can run them first.
*Alternatives:* gates in nacred's own config (invisible to the agent and to other computers); letting the
agent report success (that is today's failure mode).

### 5. Pre-flight is one structured, read-only Claude call
nacred runs `claude -p` in plan mode on the integration worktree with bd reads allowed, the beads' full
text and dependency graph, and a JSON schema. The answer lists, per bead, the areas and files it
expects to touch and any implied dependency, and a list of findings, each one of:
- `contradiction` — the beads want incompatible things → **question** to Ryder, with options;
- `ambiguity` — a bead is underspecified in a way that changes its effect on another → **question**;
- `overlap` — same files or areas, compatible intent → **ordering** (later bead stacks on the earlier);
- `ordering` — one bead needs another's result → **dependency** (added to bd with Ryder's approval
  shown in the plan, not silently);
- `shared-artifact` — version numbers, migrations, generated files → **owned by the round** (agents
  are told not to touch them; the round does it once).

Questions block the round from implementing until each has an answer. Answers are posted to the beads
as comments and fed to every affected run's prompt. Ryder may also mark the plan's ordering wrong and
reorder.
*Alternative:* detect conflicts only at merge time. That is what happens today, and it turns a design
disagreement into a merge-conflict resolution by an agent that has to guess.

### 6. Scheduling
Beads run in parallel up to the repository's concurrency (existing `max_concurrent_runs`), in an order
that respects bd dependencies plus the plan's orderings: a bead starts when everything it depends on or
stacks on has been integrated. Exclusive resources are leased (decision 9), so a bead that needs the
emulator waits for it rather than colliding.

### 7. A bead run is verified by nacred, then followed up
When a round run's turn ends successfully, nacred checks, in its worktree: no uncommitted changes, at
least one commit beyond its base, and all gates pass. On a miss, nacred resumes the same session with a
short, specific message ("These files are uncommitted: …" / "Gate `app tests` failed: <tail>") up to
`fix_attempts` times. After that the bead is **parked** (needs Ryder) and the round carries on with the
rest. A stalled run (the existing stall flag) is cancelled and resumed once with "You stopped
responding; carry on from where you were"; a second stall parks the bead.

### 8. Integration is nacred's job, done one bead at a time
Integration is serialized per round by an in-process mutex (one nacred owns a repository's rounds;
`bd merge-slot` is unnecessary inside one process and is kept in reserve for multi-computer rounds).
For each verified bead, in order: `git merge --no-ff` its branch into the integration branch.
- **Clean merge** → run gates on the integration worktree.
- **Conflict** → an integration turn: claude in the integration worktree, given both beads' text, the
  pre-flight decisions and the conflicted hunks; it must resolve, commit, and pass the gates.
- **Gates fail after merge** → an integration turn to fix, attributed to the merged bead.
After `fix_attempts` failed integration turns, nacred resets the integration branch to its pre-merge SHA,
parks the bead with the reason, and moves on.

All git that moves branches (merge, reset, branch delete, push) is done by nacred directly. Agents
only edit and commit inside their own worktree, which keeps them clear of the permission classifier's
refusals and keeps history changes in one audited place.

### 9. Resources are leases
`resources` names exclusive resources; a run declares what it needs (from the plan, or a bead label
like `needs:emulator`). nacred holds at most one lease per resource; waiting runs queue. Leases are
released when the run's turn ends, and expire with the stall window so a hung run can't hold one forever.
The resource's identity (e.g. the emulator serial) is passed to the run in its prompt and environment.

### 10. Finishing and landing
When every bead is integrated or parked: run `version.bump` once on the integration branch, run
gates and `build`, record the artifact path and SHA. The round is **ready to land**. Beads whose
acceptance criteria need a human (label `human`, or flagged by pre-flight) stay open with a
"verify: …" note; the rest are closed with a reason naming the round and commit. Landing
fast-forwards `land.target` to the integration commit; if the target moved, nacred merges the target
in, re-runs gates, and only then fast-forwards. `push: true` pushes the target. `auto: true` lands
without the tap. Afterwards nacred removes the round's worktrees and branches.

### 11. Entry points
- **API**: `POST /v1/repos/{repo}/rounds {"beads":[…] | "epic":…, "target", "budget_usd"}`, plus
  `GET /v1/rounds/{id}`, `/stream`, `POST …/decisions`, `…/land`, `…/cancel`, `…/beads/{id}/retry`.
- **CLI**: `nacred round start NAC-1 NAC-2 …`, `nacred round status`, `nacred round land`, which talk to
  the local nacred, so a Claude Code session in a terminal can delegate a round instead of doing it
  inline.
- **Assistant**: a `start_round` write tool, approved like any other write.
- **App**: multi-select on a board → "Work these until done".

### 12. Budgets
A round has a USD budget and a wall-clock limit (defaults from config; overridable per round). Each
turn passes `--max-budget-usd` with the remaining budget. Hitting either limit pauses the round with
state intact; Ryder can raise the limit and resume.

## Risks / Trade-offs

- [Gates are slow (Gradle builds take minutes) and run many times] → Gates run per bead in the bead's
  worktree in parallel, and once per merge; `nacre.yaml` can mark a gate `final_only` for the expensive
  build. Gradle caches are shared across worktrees.
- [Pre-flight misses a real conflict] → Integration still catches it; the integration turn sees the
  decisions made so far, and a parked bead is a visible, recoverable outcome rather than a bad merge.
- [Pre-flight over-asks, making rounds feel attended] → Only `contradiction` and `ambiguity` ask;
  everything else resolves by ordering. The question count is shown so the prompt can be tuned.
- [Stacking overlapping beads serializes work that could have been parallel] → Accepted: correctness
  first. Ryder can override ordering in the plan.
- [Worktrees and branches pile up if nacred crashes mid-round] → Workspaces are recorded in the round;
  a `nacred round gc` and startup sweep remove those whose round is landed, cancelled or missing.
- [A long round on the subscription] → Budgets and per-turn `--max-budget-usd`; cost is shown live.
- [Disk use from many worktrees] → Worktrees are removed as soon as a bead is integrated, not at round end.
- [Ryder pushes to the target branch during a round] → Landing merges the target in and re-verifies.

## Migration Plan

1. Ship nacred with rounds behind the presence of `nacre.yaml`: repositories without it can't start a
   round, and single runs are unchanged.
2. Add `nacre.yaml` to Nacre itself and try a round on two or three small beads.
3. Ship the app's Rounds UI; older apps simply don't show rounds (runs created by a round still appear
   in Runs, tagged with `round_id`).
Rollback: rounds are additive; disabling is removing `nacre.yaml`. Round branches are ordinary branches.

## Open Questions

- Should a parked bead block landing, or should the round land what reconciled and leave the parked bead
  for a later round? (Proposed: land what reconciled; parked beads stay open and labelled.)
- Version bump scheme: patch per round, or derived from bead types (feature → minor)?
- Should pre-flight also look at open, non-round work on other branches (e.g. an unmerged branch that
  touches the same files)? It would have caught the NAC-eky base problem.
- Default concurrency for rounds on a laptop vs the Mac Studio.
