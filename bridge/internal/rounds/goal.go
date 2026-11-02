package rounds

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/gates"
	"github.com/ryderdavid/nacre/bridge/internal/gitx"
	"github.com/ryderdavid/nacre/bridge/internal/repoconfig"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
)

// goalVersionPatch names the file in the round's directory holding the diff
// of the version bump that goal rounds started before NAC-ow0 folded into the
// agent's combined commit. Goal rounds now get nacred's own bump commit like
// every other round; the patch is read only to land such an older round.
const goalVersionPatch = "goal-version.patch"

// goalCommitRule tells the goal session what it may do with git: commit in its
// worktree and stop. nacred bumps the version, and pushes when it lands the
// round, as in every round ("Agents never move branches").
const goalCommitRule = "Commit your work on this round branch in this worktree (one or more commits), then stop. Do not bump or edit the version, push, or rewrite commits nacred made: nacred bumps the version in a commit of its own, runs the final gates and build, and pushes when it lands the round. Do not commit .beads state.\n"

// undoGoalVersion commits the reverse of an older goal round's recorded
// version bump on top of ws's HEAD, so a moved target can be merged and the
// bump redone from its version.
func (r *round) undoGoalVersion(ctx context.Context, ws string, meta api.Round) error {
	patch := filepath.Join(r.dir, goalVersionPatch)
	if _, err := os.Stat(patch); err != nil {
		return fmt.Errorf("no record of the version change folded into its combined commit")
	}
	if _, err := gitx.Run(ctx, ws, "apply", "--reverse", "--index", patch); err != nil {
		if _, err3 := gitx.Run(ctx, ws, "apply", "--reverse", "--3way", patch); err3 != nil {
			return fmt.Errorf("its version change no longer reverses cleanly: %s", firstLine(err.Error()))
		}
		if files, _ := gitx.Run(ctx, ws, "diff", "--name-only", "--diff-filter=U"); files != "" {
			return fmt.Errorf("its version change no longer reverses cleanly in: %s", strings.ReplaceAll(files, "\n", ", "))
		}
	}
	if _, err := gitx.CommitAll(ctx, ws, fmt.Sprintf("Undo round %s's version change to redo it on top of %s", meta.ID, meta.Target)); err != nil {
		return fmt.Errorf("committing the undone version change failed: %v", err)
	}
	return nil
}

// driveGoal finishes a legacy round's shared goal session. New rounds use
// per-bead goals and never enter this path. Preflight,
// decisions, specification guards, resource leases, the version bump, the
// final gates and landing (with its push) remain owned by the round
// controller; the session only commits in its worktree.
func (r *round) driveGoal() {
	if r.ops["goal"] {
		return
	}
	var needs []string
	for _, b := range r.meta.Beads {
		needs = append(needs, b.Needs...)
	}
	env, ok := r.m.leases.acquire(r.meta.ID+"/goal", dedupe(needs), r.cfg)
	if !ok {
		return
	}
	meta, cfg := clone(r.meta), r.cfg
	if meta.GoalRunID == "" {
		r.goOp("goal", func(ctx context.Context) func() {
			if err := r.m.guardSpec(ctx, meta, meta.Workspace); err != nil {
				return r.specStop(err)
			}
			var s strings.Builder
			fmt.Fprintf(&s, "Implement every bead in round %s in this integration worktree on %s. Work in dependency order. Do not switch branches, close beads, merge the target, or land the round. nacred owns landing.\n", meta.ID, meta.Branch)
			s.WriteString(goalCommitRule)
			for _, d := range meta.Questions {
				if d.Answer != "" {
					fmt.Fprintf(&s, "Round decision: %s: %s\n", d.Text, d.Answer)
				}
			}
			for _, b := range meta.Beads {
				fmt.Fprintf(&s, "\n%s depends on: %s\n", b.ID, strings.Join(b.After, ", "))
				comments, err := r.m.opts.Beads.Comments(ctx, meta.RepoPath, b.ID)
				if err != nil {
					return r.stopFn(api.Stop{Kind: api.StopGitError, Reason: "Reading round comments failed: " + err.Error()})
				}
				for _, c := range comments {
					fmt.Fprintf(&s, "Comment on %s from %s: %s\n", b.ID, c.Author, c.Text)
				}
				if c, ok := meta.SpecContracts[b.ID]; ok {
					s.WriteString(specContractPrompt(c))
				}
			}
			s.WriteString("Run all final gates and builds before finishing:\n")
			for _, g := range finalSteps(cfg) {
				fmt.Fprintf(&s, "- %s: %s\n", g.Name, g.Run)
			}
			run, err := r.m.opts.Runs.Start(runs.StartRequest{
				RepoID: meta.RepoID, RepoName: meta.RepoName, RepoPath: meta.RepoPath,
				IssueTitle: "Goal for round " + meta.ID, Mode: meta.Mode, Model: meta.Model,
				Goal: meta.GoalText, Prompt: s.String(), RoundID: meta.ID, Kind: "goal",
				Branch: meta.Branch, Workspace: meta.Workspace, Base: meta.Base,
				MaxBudgetUSD: meta.BudgetUSD - meta.SpentUSD, Env: env,
			})
			return func() {
				if err != nil {
					r.m.leases.release(meta.ID + "/goal")
					r.stop(api.Stop{Kind: api.StopGitError, Reason: "The goal session couldn't start: " + err.Error()})
					return
				}
				r.meta.GoalRunID = run.ID
				r.meta.GoalRetry = false
				r.meta.RunIDs = append(r.meta.RunIDs, run.ID)
				if r.meta.State == api.RoundCancelled || r.meta.State == api.RoundPaused {
					_, _ = r.m.opts.Runs.Cancel(run.ID)
					return
				}
				for i := range r.meta.Beads {
					r.meta.Beads[i].RunID, r.meta.Beads[i].State = run.ID, api.BeadRunning
				}
				r.event("goal", "", "Started one goal covering every bead in the round")
			}
		})
		return
	}
	run, err := r.m.opts.Runs.Run(meta.GoalRunID)
	if err != nil {
		r.stop(api.Stop{Kind: api.StopGitError, Reason: "The round's goal run is unavailable: " + err.Error()})
		return
	}
	if run.Active() {
		if run.Stalled && !r.stallCancelled[run.ID] {
			_, _ = r.m.opts.Runs.Cancel(run.ID)
			if r.meta.GoalStalls == 0 {
				r.meta.GoalStalls++
				r.stallCancelled[run.ID] = true
				r.event("goal", "", "Stalled; stopping and resuming the goal session once")
			} else {
				r.stop(api.Stop{Kind: api.StopGatesFailed, Reason: "The goal session stalled a second time"})
			}
		}
		return
	}
	if meta.GoalRetry || r.stallCancelled[run.ID] {
		r.meta.GoalRetry = false
		delete(r.stallCancelled, run.ID)
		if run.SessionID == "" {
			r.meta.GoalRunID = ""
			r.meta.GoalRetry = true
			r.poke()
			return
		}
		r.goOp("goal", func(ctx context.Context) func() {
			if err := r.m.guardSpec(ctx, meta, meta.Workspace); err != nil {
				return r.specStop(err)
			}
			_, err := r.m.opts.Runs.FollowupWith(run.ID, runs.Followup{Message: goalContinuation(meta.GoalText, "Resume the interrupted goal session from the existing work."), MaxBudgetUSD: meta.BudgetUSD - meta.SpentUSD, Env: env})
			return func() {
				if err != nil {
					r.stop(api.Stop{Kind: api.StopGatesFailed, Reason: err.Error()})
				}
			}
		})
		return
	}
	if run.Status != api.StatusSucceeded {
		r.m.leases.release(meta.ID + "/goal")
		r.stop(api.Stop{Kind: api.StopGatesFailed, Reason: "The goal session did not finish: " + run.Error})
		return
	}
	r.goOp("goal", func(ctx context.Context) func() {
		if err := r.m.guardSpec(ctx, meta, meta.Workspace); err != nil {
			return r.specStop(err)
		}
		v, version, err := r.verifyGoal(ctx, meta, cfg)
		if err != nil {
			return r.stopFn(api.Stop{Kind: api.StopBumpFailed, Phase: "final", Reason: err.Error()})
		}
		if err := r.m.guardSpec(ctx, meta, meta.Workspace); err != nil {
			return r.specStop(err)
		}
		head, _ := gitx.RevParse(ctx, meta.Workspace, "HEAD")
		_ = r.m.opts.Runs.SetVerification(run.ID, v)
		if !v.OK {
			if run.SessionID == "" {
				return r.stopFn(api.Stop{Kind: api.StopGatesFailed, Reason: v.Reason + "; no session to resume"})
			}
			_, err := r.m.opts.Runs.FollowupWith(run.ID, runs.Followup{Message: goalContinuation(meta.GoalText, v.Reason), MaxBudgetUSD: meta.BudgetUSD - meta.SpentUSD, Env: env})
			return func() {
				r.meta.Version = version
				if err != nil {
					r.stop(api.Stop{Kind: api.StopGatesFailed, Reason: err.Error()})
				} else {
					r.event("goal", "", "Continuing: "+v.Reason)
				}
			}
		}
		if err := r.m.guardSpec(ctx, meta, meta.Workspace); err != nil {
			return r.specStop(err)
		}
		finish := r.finishOK(ctx, head, gateResults(v.Gates))
		return func() {
			r.meta.Version = version
			if r.meta.State != api.RoundImplementing && r.meta.State != api.RoundIntegrating {
				return
			}
			for i := range r.meta.Beads {
				b := &r.meta.Beads[i]
				b.State, b.MergeCommit, b.Verification = api.BeadIntegrated, head, v
			}
			r.m.leases.release(meta.ID + "/goal")
			finish()
		}
	})
}

// verifyGoal checks a finished goal session's work: the worktree is still on
// the round branch, clean, and holds at least one commit of the agent's own
// above the base. Then, once per round, nacred makes its version bump commit
// (as startFinish does) and runs every final gate and build step. It returns
// the round's version; err means the bump itself failed.
func (r *round) verifyGoal(ctx context.Context, meta api.Round, cfg *repoconfig.Config) (*api.Verification, string, error) {
	ws, version := meta.Workspace, meta.Version
	v := &api.Verification{At: stamp()}
	dirty, err := gitx.Dirty(ctx, ws, ignoredPaths...)
	own, _ := gitx.CommitsBetween(ctx, ws, meta.Base, "HEAD")
	if bumpCommit(ctx, ws, meta.ID) != "" {
		own--
	}
	switch {
	case gitx.CurrentBranch(ctx, ws) != meta.Branch:
		v.Reason = "The worktree is no longer on the round's branch"
	case err != nil:
		v.Reason = "The worktree could not be checked: " + err.Error()
	case len(dirty) > 0:
		v.Uncommitted, v.Reason = dirty, "The round's work is not all committed: "+list(dirty, 8)
	case own < 1:
		v.Reason = "The round's work has not been committed on " + meta.Branch
	}
	if v.Reason != "" {
		return v, version, nil
	}
	if version == "" && cfg.Version.Bump != "" && landsOnTarget(meta, cfg) {
		// The session's beads are integrated by this commit if it passes,
		// so they decide the bump's level as integrated beads do.
		sized := clone(meta)
		for i := range sized.Beads {
			if sized.Beads[i].State == api.BeadRunning {
				sized.Beads[i].State = api.BeadIntegrated
			}
		}
		if version, err = r.bump(ctx, ws, cfg, sized, ""); err != nil {
			_ = gitx.ResetHard(ctx, ws, "HEAD")
			return nil, "", err
		}
	}
	return r.m.check(ctx, ws, meta.Base, cfg, finalSteps(cfg)), version, nil
}

func gateResults(rs []api.GateResult) []gates.Result {
	out := make([]gates.Result, len(rs))
	for i, r := range rs {
		out[i] = gates.Result{Name: r.Name, OK: r.OK, ExitCode: r.ExitCode, TimedOut: r.TimedOut, DurationMS: r.DurationMS, Tail: r.Tail, Error: r.Error}
	}
	return out
}

func goalContinuation(goal, reason string) string {
	return "/goal " + goal + "\nThe round goal is still incomplete: " + reason + ". Continue working, verify every criterion and selected condition, and commit the fixes on the round branch. Do not bump the version, push, or rewrite commits nacred made."
}
