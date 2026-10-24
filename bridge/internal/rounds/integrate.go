package rounds

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/gates"
	"github.com/ryderdavid/nacre/bridge/internal/gitx"
	"github.com/ryderdavid/nacre/bridge/internal/repoconfig"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
)

// nextToMerge is the first verified bead whose prerequisites are integrated.
func (r *round) nextToMerge() *api.RoundBead {
	for i := range r.meta.Beads {
		b := &r.meta.Beads[i]
		if b.State != api.BeadIntegrating {
			continue
		}
		if ready, _ := r.prerequisites(b); ready {
			return b
		}
	}
	return nil
}

// startMerge merges one verified bead into the integration branch and runs
// the gates there. Only nacred moves the integration branch.
func (r *round) startMerge(b *api.RoundBead) {
	id, branch, title := b.ID, b.Branch, b.Title
	meta, cfg := clone(r.meta), r.cfg
	r.meta.Integration = &api.IntegrationStep{Bead: id, Phase: "merge"}
	r.event("merge", id, "Merging "+branch)
	r.saveLocked()
	r.goOp("merge", func(ctx context.Context) func() {
		ws := meta.Workspace
		pre, err := gitx.RevParse(ctx, ws, "HEAD")
		if err != nil {
			return func() { r.integrationFailed(id, "git couldn't read the integration branch: "+err.Error()) }
		}
		res, err := gitx.Merge(ctx, ws, branch, fmt.Sprintf("Merge %s: %s (round %s)", id, title, meta.ID))
		if err != nil {
			_ = gitx.ResetHard(ctx, ws, pre)
			return func() {
				r.meta.Integration.PreMerge = pre
				r.integrationFailed(id, "git couldn't merge it: "+err.Error())
			}
		}
		if len(res.Conflicts) > 0 {
			problem := "git reported conflicts in: " + list(res.Conflicts, 20)
			return r.startIntegrationRun(ctx, id, "conflict", pre, problem, conflictPrompt(r.integrationVars(id, res.Conflicts, nil)))
		}
		results := r.m.opts.Gates.RunAll(ctx, ws, cfg, beadGates(cfg), true)
		if gates.AllOK(results) {
			return func() {
				r.meta.Integration.PreMerge = pre
				r.eventWith(api.RoundEvent{Kind: "gates", Bead: id, Text: "Gates pass after the merge", Gates: toAPI(results)})
				r.integrated(id, res.Commit)
			}
		}
		problem := failure(results)
		return r.startIntegrationRun(ctx, id, "gates", pre, problem, gatesPrompt(r.integrationVars(id, nil, results)))
	})
}

// startIntegrationRun asks Claude to fix the integration worktree.
func (r *round) startIntegrationRun(ctx context.Context, bead, phase, pre, problem, prompt string) func() {
	meta := r.metaCopy()
	title := "Integrate round " + meta.ID
	if bead != "" {
		title = "Integrate " + bead + " in round " + meta.ID
	}
	run, err := r.m.opts.Runs.Start(runs.StartRequest{
		RepoID: meta.RepoID, RepoName: meta.RepoName, RepoPath: meta.RepoPath,
		IssueID: bead, IssueTitle: title, Mode: meta.Mode, Model: meta.Model, Prompt: prompt,
		RoundID: meta.ID, Kind: "integration", Branch: meta.Branch, Workspace: meta.Workspace, Base: pre,
		MaxBudgetUSD: meta.BudgetUSD - meta.SpentUSD,
	})
	return func() {
		st := r.meta.Integration
		if st == nil {
			st = &api.IntegrationStep{Bead: bead}
			r.meta.Integration = st
		}
		st.Phase, st.Problem = phase, problem
		if pre != "" {
			st.PreMerge = pre
		}
		if err != nil {
			r.integrationFailed(bead, "the integration run couldn't start: "+err.Error())
			return
		}
		st.RunID, st.Attempts = run.ID, 1
		r.meta.RunIDs = append(r.meta.RunIDs, run.ID)
		kind := "conflict"
		if phase != "conflict" {
			kind = "gates"
		}
		r.event(kind, bead, "Sent to Claude to fix: "+firstLine(problem))
	}
}

func (r *round) metaCopy() api.Round {
	r.mu.Lock()
	defer r.mu.Unlock()
	return clone(r.meta)
}

// watchIntegration follows the integration turn in progress.
func (r *round) watchIntegration() {
	st := r.meta.Integration
	switch {
	case r.ops["merge"] || r.ops["finish"] || r.ops["integrate-check"] || r.ops["integrate-followup"]:
		return
	case st.RunID == "" && st.Phase == "merge":
		// nacred stopped mid-merge: undo it and merge again.
		pre, ws := st.PreMerge, r.meta.Workspace
		r.goOp("merge", func(ctx context.Context) func() {
			if gitx.MergeInProgress(ctx, ws) {
				_ = gitx.AbortMerge(ctx, ws)
			}
			if pre != "" {
				_ = gitx.ResetHard(ctx, ws, pre)
			}
			return func() { r.meta.Integration = nil }
		})
		return
	case st.RunID == "" && st.Phase == "final":
		r.meta.Integration = nil
		r.saveLocked()
		return
	}
	run, err := r.m.opts.Runs.Run(st.RunID)
	if err != nil {
		r.integrationFailed(st.Bead, "its integration run is gone")
		return
	}
	if run.Active() {
		if run.Stalled {
			_, _ = r.m.opts.Runs.Cancel(run.ID)
			r.event("gates", st.Bead, "The integration turn stalled; stopping it")
		}
		return
	}
	phase, bead, cfg, ws := st.Phase, st.Bead, r.cfg, r.meta.Workspace
	r.goOp("integrate-check", func(ctx context.Context) func() {
		problem, results := r.checkIntegration(ctx, ws, phase, bead, cfg)
		if problem == "" {
			head, _ := gitx.RevParse(ctx, ws, "HEAD")
			if phase == "final" {
				return r.finishOK(ctx, head, results)
			}
			return func() {
				r.eventWith(api.RoundEvent{Kind: "gates", Bead: bead, Text: "Fixed; gates pass", Gates: toAPI(results)})
				r.integrated(bead, head)
			}
		}
		return func() { r.integrationRetry(problem, results, run.SessionID != "") }
	})
}

// checkIntegration tests the integration worktree after an integration turn.
func (r *round) checkIntegration(ctx context.Context, ws, phase, bead string, cfg *repoconfig.Config) (string, []gates.Result) {
	if conflicts, _ := gitx.Conflicts(ctx, ws); len(conflicts) > 0 {
		return "conflicts remain in: " + list(conflicts, 20), nil
	}
	if gitx.MergeInProgress(ctx, ws) {
		// Resolved but not committed: finish the merge for it.
		if _, err := gitx.Run(ctx, ws, "commit", "-q", "--no-edit"); err != nil {
			return "the merge is resolved but couldn't be committed: " + err.Error(), nil
		}
	}
	if dirty, _ := gitx.Dirty(ctx, ws, ignoredPaths...); len(dirty) > 0 {
		return "uncommitted changes: " + list(dirty, 8), nil
	}
	steps := beadGates(cfg)
	if phase == "final" {
		steps = finalSteps(cfg)
	}
	results := r.m.opts.Gates.RunAll(ctx, ws, cfg, steps, true)
	if !gates.AllOK(results) {
		return failure(results), results
	}
	return "", results
}

// integrationRetry sends the integration turn back, or gives up on the bead.
func (r *round) integrationRetry(problem string, results []gates.Result, canResume bool) {
	st := r.meta.Integration
	if st == nil {
		return
	}
	r.eventWith(api.RoundEvent{Kind: "gates", Bead: st.Bead, Text: "Still failing: " + firstLine(problem), Gates: toAPI(results)})
	if st.Attempts < r.cfg.Limits.FixAttempts && canResume && r.remaining() >= minTurnBudget {
		st.Attempts++
		st.Problem = problem
		runID, budget := st.RunID, r.remaining()
		msg := integrationFollowup(problem, results, st.Phase == "final")
		r.goOp("integrate-followup", func(ctx context.Context) func() {
			_, err := r.m.opts.Runs.FollowupWith(runID, runs.Followup{Message: msg, MaxBudgetUSD: budget})
			return func() {
				if err != nil {
					r.integrationFailed(st.Bead, "its follow-up couldn't start: "+err.Error())
				}
			}
		})
		return
	}
	r.integrationFailed(st.Bead, problem)
}

// integrationFailed gives up on the current integration step: a bead's
// merge is undone and the bead parked; a failing final check pauses the round.
func (r *round) integrationFailed(bead, problem string) {
	st := r.meta.Integration
	if st != nil && st.Phase == "final" {
		r.meta.Integration = nil
		r.pause("The final gates or build fail on the integration branch: " + firstLine(problem))
		return
	}
	pre, ws := "", r.meta.Workspace
	if st != nil {
		pre = st.PreMerge
	}
	r.meta.Integration = nil
	if pre != "" {
		r.goOp("reset", func(ctx context.Context) func() {
			if err := gitx.ResetHard(ctx, ws, pre); err != nil {
				return func() { r.pause("Undoing a failed merge failed: " + err.Error()) }
			}
			return nil
		})
	}
	r.park(r.bead(bead), "It couldn't be reconciled with the rest of the round: "+problem)
}

// integrated marks a bead merged and frees its worktree.
func (r *round) integrated(id, commit string) {
	b := r.bead(id)
	r.meta.Integration = nil
	if b == nil {
		return
	}
	b.State, b.MergeCommit = api.BeadIntegrated, commit
	r.eventWith(api.RoundEvent{Kind: "merge", Bead: id, Text: "Integrated", Commit: commit})
	r.note(id, fmt.Sprintf("Integrated in round %s at %s.", r.meta.ID, short(commit)))
	repo, ws := r.meta.RepoPath, b.Workspace
	r.async(func(ctx context.Context) { _ = r.m.opts.Workspaces.Remove(ctx, repo, ws) })
	r.saveLocked()
}

// startFinish bumps the version once and runs every gate and the build on
// the integration branch.
func (r *round) startFinish() {
	if r.integratedCount() == 0 {
		r.pause("No bead could be reconciled, so there is nothing to land. Retry a parked bead or cancel the round.")
		return
	}
	meta, cfg := clone(r.meta), r.cfg
	minor := false
	for _, b := range meta.Beads {
		if b.State == api.BeadIntegrated && (b.Type == "feature" || b.Type == "epic") {
			minor = true
		}
	}
	r.meta.Integration = &api.IntegrationStep{Phase: "final"}
	r.event("build", "", "Every bead is integrated or parked; running the final gates and build")
	r.saveLocked()
	r.goOp("finish", func(ctx context.Context) func() {
		ws := meta.Workspace
		version := meta.Version
		if version == "" && cfg.Version.Bump != "" {
			level := "patch"
			if minor {
				level = "minor"
			}
			res := r.m.opts.Gates.RunAll(ctx, ws, cfg, []repoconfig.Step{{Name: "version bump", Run: cfg.BumpCommand(minor)}}, true)
			if !gates.AllOK(res) {
				return func() { r.meta.Integration = nil; r.pause("The version bump failed: " + failure(res)) }
			}
			version = lastLine(res[0].Tail)
			if _, err := gitx.CommitAll(ctx, ws, fmt.Sprintf("Bump version (%s) for round %s: %s", level, meta.ID, version)); err != nil {
				return func() { r.meta.Integration = nil; r.pause("Committing the version bump failed: " + err.Error()) }
			}
		}
		results := r.m.opts.Gates.RunAll(ctx, ws, cfg, finalSteps(cfg), true)
		if gates.AllOK(results) {
			head, _ := gitx.RevParse(ctx, ws, "HEAD")
			apply := r.finishOK(ctx, head, results)
			return func() { r.meta.Version = version; apply() }
		}
		problem := failure(results)
		start := r.startIntegrationRun(ctx, "", "final", "", problem, finalPrompt(r.integrationVars("", nil, results)))
		return func() { r.meta.Version = version; start() }
	})
}

// finishOK records the verified result, keeps the build artifacts, closes
// the beads and marks the round ready to land.
func (r *round) finishOK(ctx context.Context, head string, results []gates.Result) func() {
	meta, cfg := r.metaCopy(), r.cfg
	var artifacts []string
	for _, step := range cfg.Build {
		if step.Artifact == "" {
			continue
		}
		if p, err := keepArtifact(filepath.Join(meta.Workspace, step.Artifact), filepath.Join(r.dir, "artifacts")); err == nil {
			artifacts = append(artifacts, p)
		}
	}
	return func() {
		r.meta.Integration = nil
		r.meta.FinalCommit, r.meta.Artifacts = head, artifacts
		r.eventWith(api.RoundEvent{Kind: "build", Text: "Final gates and build pass", Commit: head, Gates: toAPI(results)})
		r.closeBeads(head)
		r.meta.State = api.RoundReadyToLand
		r.event("state", "", fmt.Sprintf("Ready to land %s on %s", short(head), r.meta.Target))
	}
}

// closeBeads closes integrated beads against the final commit, except those
// only a person can check, which stay open with what's left.
func (r *round) closeBeads(head string) {
	repo, round := r.meta.RepoPath, r.meta.ID
	for i := range r.meta.Beads {
		b := &r.meta.Beads[i]
		if b.State != api.BeadIntegrated || b.Closed {
			continue
		}
		id, check := b.ID, b.HumanCheck
		if check != "" {
			r.note(id, fmt.Sprintf("Built and verified in round %s at %s. Left for you to verify: %s", round, short(head), check))
			continue
		}
		b.Closed = true
		r.async(func(ctx context.Context) {
			_ = r.m.opts.Beads.Close(ctx, repo, id, fmt.Sprintf("Done in round %s at %s; gates and build pass", round, short(head)))
		})
	}
}

// startLand fast-forwards the target to the round's commit, first merging
// the target in and re-verifying if it moved, then pushes and cleans up.
func (r *round) startLand() {
	meta, cfg := clone(r.meta), r.cfg
	keep := map[string]bool{}
	for _, b := range meta.Beads {
		if b.State == api.BeadParked && b.Branch != "" {
			keep[b.Branch] = true
		}
	}
	r.event("land", "", "Landing on "+meta.Target)
	r.saveLocked()
	r.goOp("land", func(ctx context.Context) func() {
		ws, final := meta.Workspace, meta.FinalCommit
		head, err := gitx.RevParse(ctx, meta.RepoPath, "refs/heads/"+meta.Target)
		if err != nil {
			return func() { r.pause(meta.Target + " can't be read: " + err.Error()) }
		}
		var artifacts []string
		if !gitx.IsAncestor(ctx, meta.RepoPath, head, final) {
			res, err := gitx.Merge(ctx, ws, meta.Target, fmt.Sprintf("Merge %s into round %s before landing", meta.Target, meta.ID))
			if err != nil || len(res.Conflicts) > 0 {
				_ = gitx.ResetHard(ctx, ws, final)
				why := "conflicts in " + list(res.Conflicts, 10)
				if err != nil {
					why = err.Error()
				}
				return func() {
					r.pause(fmt.Sprintf("%s moved since the round started and can't be merged in (%s). Merge it into %s by hand, then land again.", meta.Target, why, meta.Branch))
				}
			}
			results := r.m.opts.Gates.RunAll(ctx, ws, cfg, finalSteps(cfg), true)
			if !gates.AllOK(results) {
				_ = gitx.ResetHard(ctx, ws, final)
				return func() {
					r.pause(fmt.Sprintf("%s moved since the round started, and with its changes the round fails: %s", meta.Target, failure(results)))
				}
			}
			final = res.Commit
			for _, step := range cfg.Build {
				if step.Artifact != "" {
					if p, err := keepArtifact(filepath.Join(ws, step.Artifact), filepath.Join(r.dir, "artifacts")); err == nil {
						artifacts = append(artifacts, p)
					}
				}
			}
		}
		if err := gitx.FastForward(ctx, meta.RepoPath, meta.Target, final); err != nil {
			return func() { r.pause(fmt.Sprintf("Moving %s to %s failed: %v", meta.Target, short(final), err)) }
		}
		pushErr := ""
		if cfg.Land.Push {
			if err := gitx.Push(ctx, meta.RepoPath, "origin", meta.Target); err != nil {
				pushErr = err.Error()
			}
		}
		cleanErr := r.m.opts.Workspaces.RemoveRound(ctx, meta.RepoPath, meta.RepoID, meta.ID, keep)
		return func() {
			r.meta.State, r.meta.LandedCommit, r.meta.FinalCommit, r.meta.EndedAt = api.RoundLanded, final, final, stamp()
			if len(artifacts) > 0 {
				r.meta.Artifacts = artifacts
			}
			text := fmt.Sprintf("Landed: %s is at %s", meta.Target, short(final))
			switch {
			case !cfg.Land.Push:
			case pushErr == "":
				r.meta.Pushed = true
				text += " and pushed"
			default:
				r.meta.Error = "The push failed: " + pushErr
				text += "; the push failed: " + firstLine(pushErr)
			}
			r.eventWith(api.RoundEvent{Kind: "land", Text: text, Commit: final})
			if cleanErr != nil {
				r.event("note", "", "Cleaning up the round's worktrees and branches left something behind: "+firstLine(cleanErr.Error()))
			}
		}
	})
}

// failure describes the first failing result.
func failure(results []gates.Result) string {
	for _, r := range results {
		if !r.OK {
			return r.Summary()
		}
	}
	return "no gates ran"
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSpace(s)
}

// keepArtifact copies a build artifact out of the worktree, which is removed
// when the round lands.
func keepArtifact(src, dir string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, filepath.Base(src))
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return "", err
	}
	return dst, nil
}
