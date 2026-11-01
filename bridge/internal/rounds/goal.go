package rounds

import (
	"context"
	"fmt"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/gates"
	"github.com/ryderdavid/nacre/bridge/internal/gitx"
	"github.com/ryderdavid/nacre/bridge/internal/repoconfig"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
)

// driveGoal keeps the round in one session: a goal that includes all beads
// cannot be fulfilled by isolated workers waiting for one another. Preflight,
// decisions, specification guards, resource leases and landing remain owned
// by the round controller.
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
			fmt.Fprintf(&s, "The round always requires one combined implementation commit above base %s, including installed specification files, approved preflight merges and the version bump. Consolidate this round branch's commits since that base before creating the combined commit. Amend it when fixing verification failures before pushing. Do not commit .beads state.\n", meta.Base)
			if meta.Goal.Pushed {
				fmt.Fprintf(&s, "Push only this round branch to origin: git push origin HEAD:refs/heads/%s. Never force push.\n", meta.Branch)
			} else {
				s.WriteString("Do not push; the owner has not selected pushed for this goal.\n")
			}
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
			version := meta.Version
			if cfg.Version.Bump != "" && version == "" {
				minor := false
				for _, b := range meta.Beads {
					if b.Type == "feature" || b.Type == "epic" {
						minor = true
					}
				}
				results := r.m.opts.Gates.RunAll(ctx, meta.Workspace, cfg, []repoconfig.Step{{Name: "version bump", Run: cfg.BumpCommand(minor)}}, true)
				if !gates.AllOK(results) {
					return r.stopFn(api.Stop{Kind: api.StopBumpFailed, Reason: failure(results)})
				}
				version = lastLine(results[0].Tail)
				s.WriteString("The controller has already bumped the version in this worktree. Include those changes in the combined commit; do not bump again.\n")
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
				r.meta.Version = version
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
		v := r.m.check(ctx, meta.Workspace, meta.Base, cfg, finalSteps(cfg))
		if err := r.m.guardSpec(ctx, meta, meta.Workspace); err != nil {
			return r.specStop(err)
		}
		if v.OK && gitx.CurrentBranch(ctx, meta.Workspace) != meta.Branch {
			v.OK, v.Reason = false, "The worktree is no longer on the round's branch"
		}
		if v.OK {
			dirty, err := gitx.Dirty(ctx, meta.Workspace, ignoredPaths...)
			if err != nil || len(dirty) > 0 {
				v.OK, v.Reason = false, "The final gates left uncommitted changes or the worktree could not be checked"
			}
		}
		if v.OK && v.Commits != 1 {
			v.OK, v.Reason = false, "The round needs one combined commit containing all its work"
		}
		head, _ := gitx.RevParse(ctx, meta.Workspace, "HEAD")
		if v.OK && meta.Goal.Pushed {
			remote, err := gitx.Run(ctx, meta.Workspace, "ls-remote", "--exit-code", "origin", "refs/heads/"+meta.Branch)
			if err != nil || !strings.HasPrefix(remote, head+"\t") {
				v.OK, v.Reason = false, "The remote does not contain this round's final commit on "+meta.Branch
			}
		}
		_ = r.m.opts.Runs.SetVerification(run.ID, v)
		if !v.OK {
			if run.SessionID == "" {
				return r.stopFn(api.Stop{Kind: api.StopGatesFailed, Reason: v.Reason + "; no session to resume"})
			}
			_, err := r.m.opts.Runs.FollowupWith(run.ID, runs.Followup{Message: goalContinuation(meta.GoalText, v.Reason), MaxBudgetUSD: meta.BudgetUSD - meta.SpentUSD, Env: env})
			return func() {
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

func gateResults(rs []api.GateResult) []gates.Result {
	out := make([]gates.Result, len(rs))
	for i, r := range rs {
		out[i] = gates.Result{Name: r.Name, OK: r.OK, ExitCode: r.ExitCode, TimedOut: r.TimedOut, DurationMS: r.DurationMS, Tail: r.Tail, Error: r.Error}
	}
	return out
}

func goalContinuation(goal, reason string) string {
	return "/goal " + goal + "\nThe round goal is still incomplete: " + reason + ". Continue working, verify every criterion and selected condition, and amend the round's combined commit if it has not been pushed. Do not force push."
}
