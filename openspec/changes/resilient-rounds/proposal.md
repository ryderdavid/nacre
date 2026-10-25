## Why

A round is meant to carry several beads to one landable commit without Ryder in the middle, but when
anything unexpected happens in git it stops and says "do it by hand". Round rdtnfnc4vum did all of its
work and still stopped twice for problems that took no judgment of Ryder's to solve:

| Stop | What was actually wrong | What it took |
|---|---|---|
| "Merging origin/nac-j5z.2 into the round failed (conflicts in transcript.go)"; Resume repeated it | The branch's only commit was already in `main` by rebase, so it shouldn't have been asked about; and the pause kept the answer that caused it | `git cherry` to see the work was upstream; a way to answer again |
| "main moved since the round started and can't be merged in (conflicts in build.gradle.kts)" | `main` shipped 0.5.1 while the round's own bump said 0.6.0; the only conflict was the round's own version line | Redo the bump on top of `main` instead of merging two bumps |

Both were fixed narrowly (NAC-2zc, NAC-gkc), but the gap is general. Nacre already lets Claude resolve
conflicts between two beads during integration, then re-checks the gates; nowhere else does it try. Every
other surprise ends in a pause whose text tells Ryder to open a terminal, and Resume replays the step
that just failed. A person or an agent sitting at the Mac solves these by looking at what is actually
wrong first. A round should do the same, and stop only for decisions that are genuinely Ryder's.

## What Changes

- **A recovery ladder at every stop point.** Before a round stops, it tries, in order: a known
  mechanical fix (no Claude, no cost); a recovery turn in which Claude works in the round's own workspace
  and the gates decide whether it worked; and only then a stop for Ryder. Each step is recorded as a
  round event, so nothing is fixed silently.
- **Mechanical fixes for known cases.** Version bumps (redo the round's bump on top of the target, from
  NAC-gkc), branches already in the target by content (from NAC-2zc), the beads JSONL logs under
  `.beads/` (union of both sides), and files a repository declares as regenerated (`nacre.yaml`
  `merge.regenerate`: a path pattern and the command that rebuilds it, such as a lockfile).
- **Claude resolves every merge, not just bead merges.** The merge of unmerged work before the beads
  start and the merge of a moved target before landing go through the same integration turn bead merges
  use: told what each side meant, bounded by `fix_attempts` and the round's budget, verified by the
  gates. A landing whose target's changes break the round gets a turn too, instead of a pause.
- **Transient failures are retried.** A run that can't start, bd that can't be read, or a workspace that
  can't be made is retried with backoff before a bead is parked, since these are usually a busy disk, a
  restarting service or a locked database.
- **Diagnose before stopping.** When the ladder runs out, a short read-only Claude step looks at the
  failure, the round's events and the repository, and says in plain words what is wrong and what each way
  forward would do. That text is shown on the stop card in the app and in `nacred round status`.
- **Stops are decisions with choices, and Resume never repeats a failure.** A stop carries concrete
  choices (let Claude try again with more budget, skip the branch or bead the stop is about, raise the
  limits, cancel), and "I fixed it, continue" first checks that something actually changed; if the same
  step would fail the same way, the round says so instead of trying. A round is never split by choice:
  it lands whole, or without only the beads that truly couldn't be finished, which it already sets aside
  by itself and lists at Land.
- **Set-aside work carries over.** A bead parked by a round (or left by a cancelled one) keeps its branch;
  today a later round that includes the bead ignores that work and starts again. The next round that
  includes it continues from the kept branch, brought up to date with the new base.
- **App.** The pause card shows the diagnosis and one button per choice; recovery events read as part of
  the round's log; notifications fire only for a real stop.

Out of scope:
- Rounds across several repositories or computers, and any change to single runs outside a round.
- Recovering nacred itself (a crash or restart mid-round is already handled by reloading rounds).
- Learning new mechanical fixes automatically from how Ryder resolved a stop; new ones are added to the
  catalogue in code.
- Overriding Ryder's decisions: a question about intent (a contradiction between beads) is still his, and
  budget and time limits still stop the round.
- Landing part of a round by choice. Beads in a round belong together; an urgent fix that can't wait for
  the rest goes in a round of its own.
- Force-pushing, rewriting the target's history, or deleting branches as a recovery step.

## Capabilities

### New Capabilities
- `round-recovery`: the recovery ladder at every stop point, the catalogue of mechanical fixes, recovery
  turns for merges and landing, transient retries, the diagnosis shown at a stop, stops as choices that
  never replay the same failure, and carrying a set-aside bead's work into the next round.

### Modified Capabilities
<!-- None in openspec/specs/: the round capabilities this builds on (run-rounds, round-preflight,
     round-integration, round-app) are still in the unarchived semi-attended-rounds change. This change
     adds round-recovery alongside them; the scenarios added there for NAC-2zc and NAC-gkc stay. -->

## Impact

- **nacred** `internal/rounds`: a single stop path (`stop` replacing ad hoc `pause` calls) that runs the
  ladder; recovery turns for the pre-flight and landing merges reusing `startIntegrationRun`; transient
  retry for run start, bd reads and workspace creation; a diagnosis call through the headless assist
  path with read-only repository access; choice handling. `internal/gitx`: union merge for JSONL,
  regenerate-and-add for declared files. `internal/repoconfig`: `merge.regenerate`.
- **API** (`docs/API.md`): Round gains `stop` (`kind`, `reason`, `diagnosis`, `choices`, `attempted`);
  `POST /v1/rounds/{round}/choose` with a choice id; new event kind `recover`.
- **App**: the pause card renders the diagnosis and choices; round events show recoveries.
- **Cost**: recovery turns and diagnoses spend the round's budget like any other turn; a diagnosis is
  capped at $0.50 and skipped when the budget is gone.
