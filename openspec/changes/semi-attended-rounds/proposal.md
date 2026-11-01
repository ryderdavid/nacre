## Why

Today every bead runs on its own: nacred starts one `claude -p` per bead, the run "succeeds" when the
process exits, and nobody owns putting the pieces back together. In one week that produced work built on
a stale base (NAC-eky on 0.2.1 because `claude --worktree` branched from an unmerged `main`), finished
work left uncommitted (NAC-j5z.6), a release branch that sat unmerged while runs kept forking from `main`
(integration-0.3), a run that hung for four hours (NAC-1tm), runs fighting over one emulator, and
reconciliation done by hand afterwards. Ryder wants to name several beads, say "work these until they're
done", and come back to one commit that reconciles all of them, passes every test and builds — stopping
only for decisions that are genuinely his.

## What Changes

- **Rounds.** A new unit of work above a run: a set of beads in one repository that nacred carries from
  plan to a single integration commit. A round runs on the Mac without the phone attached and has a
  visible state (planning, needs decision, implementing, integrating, ready to land, landed, parked).
- **Pre-flight review before any code.** A round first reads every bead and the repository and reports
  where their intentions collide: contradictory designs, overlapping areas, implied ordering, shared
  artifacts such as the version number. Contradictions and ambiguities become questions for Ryder and
  block implementation until answered; plain overlaps are resolved by ordering, without asking.
- **Workspaces nacred owns.** nacred creates each bead's worktree and branch itself, from a base it
  records (the round's integration branch), instead of `claude --worktree` choosing the default branch.
  The branch and path are part of the run; nacred removes them once their work has landed.
- **Repository gates.** A checked-in `nacre.yaml` declares the commands that define "done" for a
  repository (tests, build, artifact), the environment they need, the version bump, and exclusive
  resources. nacred runs the gates itself; an agent saying it is done is not evidence.
- **Verified bead runs with automatic follow-up.** A bead's run counts as finished only when its tree is
  clean, it has commits on its branch and the gates pass. Otherwise nacred resumes the same session with
  the failure, a bounded number of times, before asking Ryder. A stalled run in a round is cancelled and
  resumed once automatically.
- **Serialized integration.** nacred merges finished bead branches into the round's integration branch
  one at a time, in dependency order, re-running the gates after each. Conflicts and post-merge
  failures go to an integration turn that knows both beads' intentions. The round ends with the gates
  and the build passing on one commit, one version bump, and beads closed against that commit.
- **Landing on Ryder's word by default.** A finished round is "ready to land": one tap fast-forwards the
  target branch and pushes. Repositories can opt in to landing automatically.
- **Exclusive resources.** Runs lease declared resources (an emulator, a device) so two runs never drive
  the same one.
- **Entry points.** Rounds start from the app (multi-select beads on a board), from the board assistant
  (a `start_round` proposal Ryder approves), and from a terminal (`nacred round start …`), so a Claude
  Code session can hand a round to nacred too.
- **App.** A Rounds view with each bead's state, the decisions waiting on Ryder, the integration log,
  and a Land button; notifications when a round needs a decision or is ready to land.

Out of scope:
- Rounds spanning several repositories or several computers. A round runs on one computer, in one
  repository (the group's home computer unless Ryder picks another).
- Opening GitHub pull requests or waiting on CI; landing is a local fast-forward and push.
- Agents other than Claude Code.
- Reworking how single runs work outside a round; "Run with Claude Code" on one bead stays as it is.
- Deciding acceptance criteria that need a human (a device, an API key, judgment). Rounds report them;
  they don't fake them.

## Capabilities

### New Capabilities
- `run-rounds`: the round lifecycle — creating a round from beads, its states, what it stops for, budgets,
  entry points (app, assistant, CLI), and how its progress is recorded on the beads.
- `round-preflight`: the intent review before implementation, the questions it raises, how Ryder's answers
  are recorded, and how overlaps turn into ordering.
- `run-workspaces`: nacred-created worktrees and branches from a recorded base, what a run reports about
  them, and their cleanup.
- `repo-gates`: the `nacre.yaml` contract (gates, build, environment, version bump, resources), how nacred
  runs it, and when a bead's run counts as done.
- `round-integration`: serialized merging, conflict and failure resolution, the final verification, the
  version bump, closing beads, and landing.
- `exclusive-resources`: declaring and leasing resources such as emulators across concurrent runs.
- `round-app`: the app's Rounds view, starting a round from a board, answering decisions, landing, and
  notifications.

### Modified Capabilities
None. round-recovery and epic-completion, accepted since, refine how rounds stop and recover and where
they land; this change's requirements defer to them where they say so.

## Impact

- **nacred**: new `internal/rounds` package (state machine, persistence under the data dir), workspace
  management (git/bd worktree), gate runner, integration (git merge, run by nacred, not by an agent),
  resource leases, `nacred round` CLI, new API routes and SSE events; `internal/runs` gains workspace
  and verification hooks and a prompt contract for round runs.
- **API** (`docs/API.md`): `/v1/repos/{repo}/rounds`, `/v1/rounds/{round}` (+ stream, decisions, land,
  cancel); Run gains `round_id`, `branch`, `workspace`, `base`, `verified`.
- **App**: Rounds screen and detail, multi-select on boards, decisions sheet, assistant `start_round`
  tool, notifications.
- **Repositories**: each repo that uses rounds adds `nacre.yaml`; Nacre's own goes in with this change.
- **Beads**: round label `round:<id>`, plan and decisions as bead comments; closes happen at round end.
- **Permissions**: merging, branch deletion and pushing are done by nacred's own git calls, so round
  agents never need destructive git permissions.
