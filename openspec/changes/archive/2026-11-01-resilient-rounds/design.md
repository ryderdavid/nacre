## Context

`internal/rounds` stops a round in two ways: `pause(reason)` (the round waits, Resume puts it back in
the state it paused from) and `park(bead, reason)` (one bead is set aside, the round carries on). Today
there are 15 pause sites and 10 park sites. Only one failure class has an automatic remedy: a bead merge
that conflicts or breaks the gates during integration starts an integration turn (`startIntegrationRun`,
followed up by `integrationRetry` up to `limits.fix_attempts`), and only when that runs out is the bead
parked.

| Stop point (today) | Kind | Ladder after this change |
|---|---|---|
| Pre-flight merge of unmerged work conflicts / breaks gates | reopen question (NAC-2zc) | mechanical → recovery turn → question again with diagnosis |
| Target moved, merge at landing conflicts | pause | mechanical (version, JSONL, regenerate) → recovery turn → stop |
| Target moved, gates fail with its changes | pause | recovery turn (phase `land`) → stop |
| Final gates or build fail after the integration turn gave up | pause | stop with diagnosis (the turn already ran) |
| Undoing a merge / the version bump fails; target unreadable; fast-forward fails | pause | transient retry → stop with diagnosis |
| Version bump command fails | pause | stop with diagnosis |
| Pre-flight review can't run or can't be parsed | pause | transient retry (one more review) → stop |
| Workspace can't be made; bd can't read the bead; run can't start; follow-up can't start | park | transient retry with backoff → park |
| Run failed / gone / cancelled / stalled twice / verification fails after follow-ups | park | unchanged (the bead's own turns already ran) |
| Nothing could be reconciled | pause | stop with choices (retry a parked bead, cancel) |
| Budget or time used up | pause | stop with choices (raise, with an estimate of what's left; cancel); no ladder |

## Goals / Non-Goals

**Goals:**
- A round stops only when a mechanical fix and a verified Claude turn have both failed, or when the
  decision is genuinely Ryder's (intent, budget, time).
- Every stop explains itself in plain words and offers choices that each do something different.
- Recovery is visible: each attempt is an event with what was tried and what happened.

**Non-Goals:**
- Unbounded retrying. Every rung has a bound (attempts, budget, backoff count).
- Letting a recovery turn change history outside the round: it works on the integration branch only;
  landing is still a fast-forward that nacred performs.

## Decisions

### One stop path with a ladder
Replace direct `pause` calls with `stop(Stop)`, where `Stop` carries a kind (`merge-conflict`,
`gates-failed`, `git-error`, `transient`, `limits`, `nothing-integrated`, `review-failed`), the context
the ladder needs (phase, branch or target, pre-merge commit, conflicted files, gate results), and what
has been attempted. `stop` runs the next rung that applies and hasn't been tried; only when none is left
does the round enter `paused` with a diagnosis and choices. Attempts are stored on the stop so a reload
mid-ladder doesn't repeat rungs.

*Alternative considered:* adding a retry at each call site. Rejected: it is how the current code drifted,
with each site inventing its own advice text.

### Mechanical fixes are a catalogue, tried by conflicted path
A fix declares which paths it handles and how: `version` (the round's bump commit is reverted before the
merge and redone after, at the same level, from the target's version; NAC-gkc's code moves here),
`beads-jsonl` (`.beads/*.jsonl`: union of both sides' lines, de-duplicated, order kept), and
`regenerate` (a `nacre.yaml` `merge.regenerate` entry: take the target's side, run the command, add the
result). A merge is resolved mechanically only if every conflicted path is handled; otherwise the merge is
undone and the next rung runs. The gates always run after a mechanical fix.

### Recovery turns reuse the integration turn
The pre-flight and landing merges call the same `startIntegrationRun` with new phases (`preflight-merge`,
`land-merge`, `land`). The prompt states what each side meant: for a branch, its commits and the
pre-flight finding about it; for the target, its commits since the round's base; for the round, the
beads' titles and decisions. Rules in the prompt: keep both sides' intent; where the target already has
an equivalent change, the target's version wins; never revert target commits. Success is decided by the
gates, not by the turn's own report. `fix_attempts` and the budget bound it, exactly as for bead merges.

### Transient retry
Failures that come from the environment (process start, `bd` exit with a lock or I/O error, `git
worktree add` on a busy index, network to Claude) are retried twice with 5 s and 30 s backoff before the
bead is parked or the round stops. Which errors count is a small classifier in one place, with tests
from real error strings.

### Diagnosis
When the ladder is exhausted, nacred makes one headless call (the assist path, read-only: `bd` reads and
`read_repo`) with the stop's context, the last 30 round events and the relevant `git status`/`git log`
output, asking for JSON `{summary, cause, choices: [{id, explanation}]}` where ids come from the
choices the stop allows. It is capped at $0.50 of the round's budget, times out after 3 minutes, and is
skipped (with the plain reason shown) when the budget can't cover it. Ryder's view: the summary as the
card's text, each choice's explanation under its button.

### Choices instead of Resume
A paused round exposes `choices`, each an id the bridge knows how to carry out: `retry-with-budget`
(another recovery turn, with an amount), `skip-branch` (only on a stop about a pre-flight branch),
`skip-bead` (park it; only on a stop about one bead), `continue` (Ryder changed something by hand),
`raise-limits`, `cancel`.

There is deliberately no "land what's integrated". A round already finishes as soon as every bead is
integrated, parked or skipped, so a bead that can't be finished never blocks landing, and the Land card
lists what was set aside. The only other way to end up with some beads integrated and others unfinished
is a budget or time stop, where the useful answer is to raise the limit (the round is usually most of
the way there) or cancel. Splitting by choice would ship half of work that was started as a unit, spend a
version bump on it, and leave the rest on a branch built on an old base. A limits stop instead shows
what was spent per bead and estimates the rest from the average cost of the finished beads, so raising
the limit is an informed choice. `continue` compares the state that made the step fail (the HEADs and
files involved) with now; if nothing changed it refuses with "nothing has changed since this failed" and
leaves the other choices. The existing Resume route maps to `continue`, so older apps keep working.

### Set-aside work carries over
A parked bead's branch (`nacre/r/<round>/<bead>`) is kept, and so are a cancelled round's. The
pre-flight scan skips every `nacre/r/` branch, so that work is never asked about, but today a later round
that includes the bead also ignores it. When a round creates a bead's workspace, nacred looks up the
newest kept branch for that bead (from stored rounds, not by branch name alone) that has commits beyond
its own base; if there is one, the new bead branch starts from the integration branch and merges the
kept branch in, through the same mechanical fixes and recovery turn as any other merge. The bead's
prompt says its earlier work is already there and why it was set aside. The old round's record notes
which round picked the branch up, and the old branch is deleted once the new round lands.

### API shape
`Round.stop = {kind, reason, diagnosis: {summary, cause}, choices: [{id, label, explanation}],
attempted: ["mechanical:version", "turn:1", ...]}`, present only while paused. `POST
/v1/rounds/{round}/choose {"choice": "...", "budget_usd": n}`. New event kind `recover` with the rung
and outcome. `pause_reason` stays, set to `stop.reason`, for older apps.

## Risks / Trade-offs

- [A recovery turn makes a wrong but gate-passing resolution] → Gates are the bar here as for bead
  merges; the turn's prompt forbids reverting target commits, and every resolution is a separate commit
  with an event, so Ryder can see and undo it before landing.
- [Mechanical fixes hide real conflicts] → A fix applies only when every conflicted path is in its
  catalogue, and the gates still run afterwards.
- [Spend creeps up] → Every rung is bounded and budget-checked; the diagnosis has its own small cap.
- [Classifying transient errors wrongly retries a real failure] → Two retries at most, then the normal
  path; the cost is a short delay.

## Migration Plan

Additive in the bridge and app. Rounds already paused when nacred is upgraded keep `pause_reason`, get
`stop.kind = "legacy"` with `continue` and `cancel` as choices. No data migration.

## Open Questions

None. (Settled: no "land what's integrated" choice; see "Choices instead of Resume".)
