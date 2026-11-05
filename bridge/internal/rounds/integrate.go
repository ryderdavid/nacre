package rounds

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

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
		if err := r.m.guardSpec(ctx, meta, ws); err != nil {
			return r.specStop(err)
		}
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
		var fixes []string
		if len(res.Conflicts) > 0 {
			var ok bool
			if fixes, ok = r.mechanical(ctx, ws, cfg, res.Conflicts); !ok {
				problem := "git reported conflicts in: " + list(res.Conflicts, 20)
				return r.startIntegrationRun(ctx, api.IntegrationStep{Bead: id, Phase: "conflict", PreMerge: pre, Problem: problem},
					conflictPrompt(r.integrationVars(id, res.Conflicts, nil)))
			}
			res.Commit, _ = gitx.RevParse(ctx, ws, "HEAD")
		}
		if err := r.m.guardSpec(ctx, meta, ws); err != nil {
			return r.specStop(err)
		}
		results := r.m.opts.Gates.RunAll(ctx, ws, cfg, beadGates(cfg), true)
		if gates.AllOK(results) {
			return func() {
				r.meta.Integration.PreMerge = pre
				r.recordFixes(id, "the merge", fixes)
				r.eventWith(api.RoundEvent{Kind: "gates", Bead: id, Text: "Gates pass after the merge", Gates: toAPI(results)})
				r.integrated(id, res.Commit)
			}
		}
		return r.startIntegrationRun(ctx, api.IntegrationStep{Bead: id, Phase: "gates", PreMerge: pre, Problem: failure(results), Attempted: fixRungs(fixes)},
			gatesPrompt(r.integrationVars(id, nil, results)))
	})
}

// startIntegrationRun asks Claude to fix the integration worktree. step
// describes the turn: its phase, what it is for and what to reset to if it
// gives up.
func (r *round) startIntegrationRun(ctx context.Context, step api.IntegrationStep, prompt string) func() {
	meta := r.metaCopy()
	if err := r.m.guardSpec(ctx, meta, meta.Workspace); err != nil {
		return r.specStop(err)
	}
	bead := step.Bead
	title := "Integrate round " + meta.ID
	switch {
	case bead != "":
		title = "Integrate " + bead + " in round " + meta.ID
	case step.Branch != "":
		title = "Bring " + step.Branch + " into round " + meta.ID
	case landPhase(step.Phase):
		title = "Merge " + meta.Target + " into round " + meta.ID + " to bring it up to date"
	}
	var run api.Run
	n, err := retry(ctx, func() (e error) {
		run, e = r.m.opts.Runs.Start(runs.StartRequest{
			RepoID: meta.RepoID, RepoName: meta.RepoName, RepoPath: meta.RepoPath,
			IssueID: bead, IssueTitle: title, Mode: meta.Mode, Model: meta.Model, Prompt: prompt,
			RoundID: meta.ID, Kind: "integration", Branch: meta.Branch, Workspace: meta.Workspace, Base: step.PreMerge,
			MaxBudgetUSD: meta.BudgetUSD - meta.SpentUSD,
		})
		return e
	})
	return func() {
		st := r.meta.Integration
		if st == nil {
			st = &api.IntegrationStep{}
			r.meta.Integration = st
		}
		st.Bead, st.Phase, st.Problem, st.Branch, st.Rebump = bead, step.Phase, step.Problem, step.Branch, step.Rebump
		if step.PreMerge != "" {
			st.PreMerge = step.PreMerge
		}
		if landPhase(step.Phase) {
			// The land operation has handed over to Claude; the round follows
			// the turn like any other integration.
			r.meta.State = api.RoundIntegrating
		}
		r.retried(bead, "Starting the integration turn", n)
		// What ran before this turn, then the turn itself, saved as it
		// starts: a reload must not run either again.
		r.attempt(append(append([]string(nil), step.Attempted...), tries(n)...)...)
		if err != nil {
			r.integrationFailed(bead, "the integration run couldn't start: "+err.Error())
			return
		}
		st.RunID, st.Attempts = run.ID, 1
		r.attempt(turnRung(step.Phase, 1))
		r.meta.RunIDs = append(r.meta.RunIDs, run.ID)
		kind := "gates"
		switch step.Phase {
		case "conflict":
			kind = "conflict"
		case "preflight-merge", "land-merge", "land":
			kind = "recover"
		}
		r.event(kind, bead, "Sent to Claude to fix: "+firstLine(step.Problem))
	}
}

// landPhase reports whether an integration phase belongs to landing.
func landPhase(phase string) bool { return phase == "land-merge" || phase == "land" }

func (r *round) metaCopy() api.Round {
	r.mu.Lock()
	defer r.mu.Unlock()
	return clone(r.meta)
}

// watchIntegration follows the integration turn in progress.
func (r *round) watchIntegration() {
	st := r.meta.Integration
	switch {
	case r.ops["merge"] || r.ops["finish"] || r.ops["integrate-check"] || r.ops["integrate-followup"] || r.ops["decide"] || r.ops["land"] || r.ops["reset"]:
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
	case st.RunID == "":
		// A recovery turn that never started (nacred restarted): undo the
		// merge and go back to the step that wanted it.
		pre, ws, phase := st.PreMerge, r.meta.Workspace, st.Phase
		r.goOp("reset", func(ctx context.Context) func() {
			if gitx.MergeInProgress(ctx, ws) {
				_ = gitx.AbortMerge(ctx, ws)
			}
			if pre != "" {
				_ = gitx.ResetHard(ctx, ws, pre)
			}
			return func() {
				r.meta.Integration = nil
				if landPhase(phase) {
					r.meta.State = api.RoundReadyToLand
					r.startLand()
				}
			}
		})
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
	step, cfg, ws, meta := *st, r.cfg, r.meta.Workspace, clone(r.meta)
	r.goOp("integrate-check", func(ctx context.Context) func() {
		problem, results := r.checkIntegration(ctx, ws, step.Phase, step.Bead, cfg)
		if problem != "" {
			return func() { r.integrationRetry(problem, results, run.SessionID != "") }
		}
		head, _ := gitx.RevParse(ctx, ws, "HEAD")
		switch step.Phase {
		case "final":
			return r.finishOK(ctx, head, results)
		case "preflight-merge":
			return func() {
				r.meta.Integration = nil
				r.eventWith(api.RoundEvent{Kind: "merge", Text: "Merged " + step.Branch + " into the round; Claude settled it and the gates pass", Commit: head, Gates: toAPI(results)})
			}
		case "land-merge":
			land := r.afterLandMerge(ctx, meta, cfg, step.Rebump, nil)
			return func() {
				r.eventWith(api.RoundEvent{Kind: "recover", Text: "Claude settled merging " + meta.Target + "; the gates pass", Commit: head, Gates: toAPI(results)})
				land()
			}
		case "land":
			return r.completeLand(ctx, meta, cfg, head, meta.Version, true)
		}
		return func() {
			r.eventWith(api.RoundEvent{Kind: "gates", Bead: step.Bead, Text: "Fixed; gates pass", Gates: toAPI(results)})
			r.integrated(step.Bead, head)
		}
	})
}

// checkIntegration tests the integration worktree after an integration turn.
func (r *round) checkIntegration(ctx context.Context, ws, phase, bead string, cfg *repoconfig.Config) (string, []gates.Result) {
	if err := r.m.guardSpec(ctx, r.metaCopy(), ws); err != nil {
		return err.Error(), nil
	}
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
	if phase == "final" || phase == "land" {
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
		r.attempt(turnRung(st.Phase, st.Attempts))
		runID, budget := st.RunID, r.remaining()
		msg := integrationFollowup(problem, results, st.Phase == "final" || st.Phase == "land")
		r.goOp("integrate-followup", func(ctx context.Context) func() {
			meta := r.metaCopy()
			if err := r.m.guardSpec(ctx, meta, meta.Workspace); err != nil {
				return r.specStop(err)
			}
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

// integrationFailed gives up on the current integration step. A bead's merge
// is undone and the bead parked; unmerged work Ryder chose to bring in goes
// back to its question; a failing final check or landing stops the round.
func (r *round) integrationFailed(bead, problem string) {
	st := r.meta.Integration
	r.meta.Integration = nil
	if st == nil {
		st = &api.IntegrationStep{Bead: bead}
	}
	pre, ws, phase := st.PreMerge, r.meta.Workspace, st.Phase
	attempted := append([]string(nil), st.Attempted...)
	if st.Attempts > 0 && !slices.Contains(attempted, turnRung(phase, st.Attempts)) {
		attempted = append(attempted, turnRung(phase, st.Attempts))
	}
	// undo resets the integration branch, then records the outcome.
	undo := func(then func()) {
		r.goOp("reset", func(ctx context.Context) func() {
			if gitx.MergeInProgress(ctx, ws) {
				_ = gitx.AbortMerge(ctx, ws)
			}
			if pre != "" {
				if err := gitx.ResetHard(ctx, ws, pre); err != nil {
					return r.stopFn(api.Stop{Kind: api.StopGitError, Bead: bead, Reason: "Undoing a failed merge failed: " + err.Error(), Attempted: attempted})
				}
			}
			return then
		})
	}
	switch phase {
	case "final":
		r.stop(api.Stop{Kind: api.StopGatesFailed, Phase: "final", Reason: "The final gates or build fail on the integration branch: " + firstLine(problem), Attempted: attempted})
	case "land-merge", "land":
		kind, what := api.StopMergeConflict, r.meta.Target+" moved since the round started and Claude couldn't merge it in"
		if phase == "land" {
			kind, what = api.StopGatesFailed, r.meta.Target+" moved since the round started, and with its changes the round fails"
		}
		undo(r.stopFn(api.Stop{Kind: kind, Phase: "land", Reason: what + ": " + firstLine(problem), Attempted: attempted}))
	case "preflight-merge":
		// Reopen the question at once, so the answer that led here can't
		// start another merge while this one is being undone.
		undo(nil)
		r.reopen(st.Branch, fmt.Sprintf("Claude couldn't bring %s into the round: %s. Choose another answer, or merge it into %s by hand and choose to merge again.", st.Branch, firstLine(problem), r.meta.Branch))
	default:
		if pre != "" {
			undo(nil)
		}
		r.park(r.bead(bead), "It couldn't be reconciled with the rest of the round: "+problem)
	}
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
	if r.integratedCount() == 0 && r.meta.FinishEpic == "" {
		if r.allLeftOut() {
			// Ryder's own answers left nothing to do: end the round instead
			// of asking him to cancel it.
			r.cancelLocked("Every bead was left out by your answers, so the round ended itself")
			return
		}
		r.stop(api.Stop{Kind: api.StopNothingIntegrated, Reason: "No bead could be reconciled, so there is nothing to merge. Retry a parked bead or cancel the round."})
		return
	}
	meta, cfg := clone(r.meta), r.cfg
	r.meta.Integration = &api.IntegrationStep{Phase: "final"}
	r.event("build", "", "Every bead is integrated or parked; running the final gates and build")
	r.saveLocked()
	r.goOp("finish", func(ctx context.Context) func() {
		ws := meta.Workspace
		if err := r.m.guardSpec(ctx, meta, ws); err != nil {
			return r.specStop(err)
		}
		archived := ""
		if meta.FinishEpic != "" && r.m.opts.ArchiveChange != nil {
			name, err := r.m.opts.ArchiveChange(ctx, meta.RepoPath, ws, meta.FinishEpic)
			if err != nil {
				return func() {
					r.meta.Integration = nil
					r.stop(api.Stop{Kind: api.StopGitError, Phase: "final", Reason: err.Error()})
				}
			}
			archived = name
		}
		noteArchive := func() {
			if archived != "" {
				r.event("note", meta.FinishEpic, "Archived OpenSpec change "+archived+" with the epic's work")
			}
		}
		version := meta.Version
		// Versions move only where work lands for good, not on epic branches.
		if version == "" && cfg.Version.Bump != "" && landsOnTarget(meta, cfg) {
			v, err := r.bump(ctx, ws, cfg, meta, "")
			if err != nil {
				return func() {
					r.meta.Integration = nil
					r.stop(api.Stop{Kind: api.StopBumpFailed, Phase: "final", Reason: err.Error()})
				}
			}
			version = v
		}
		results := r.m.opts.Gates.RunAll(ctx, ws, cfg, finalSteps(cfg), true)
		if gates.AllOK(results) {
			head, _ := gitx.RevParse(ctx, ws, "HEAD")
			apply := r.finishOK(ctx, head, results)
			return func() { noteArchive(); r.meta.Version = version; apply() }
		}
		start := r.startIntegrationRun(ctx, api.IntegrationStep{Phase: "final", Problem: failure(results)}, finalPrompt(r.integrationVars("", nil, results)))
		return func() { noteArchive(); r.meta.Version = version; start() }
	})
}

// finishOK records the verified result, keeps the build artifacts, closes
// the beads and marks the round ready to land.
func (r *round) finishOK(ctx context.Context, head string, results []gates.Result) func() {
	meta, cfg := r.metaCopy(), r.cfg
	if err := r.m.guardSpec(ctx, meta, meta.Workspace); err != nil {
		return r.specStop(err)
	}
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
		r.event("state", "", fmt.Sprintf("Ready to merge %s into %s", short(head), r.meta.Target))
	}
}

// closeBeads closes integrated beads against the final commit, except those
// only a person can check, which stay open with what's left.
func (r *round) closeBeads(head string) {
	round := r.meta.ID
	var batch []closing
	for i := range r.meta.Beads {
		b := &r.meta.Beads[i]
		if b.State != api.BeadIntegrated || b.Closed || r.ops["close:"+b.ID] {
			continue
		}
		// Specification sections remain open until the reviewed build lands.
		// An automatic epic finish waits for these closes too.
		if b.SpecChangeID != "" && r.meta.State != api.RoundLanded {
			continue
		}
		id, check := b.ID, b.HumanCheck
		if check != "" {
			r.note(id, fmt.Sprintf("Built and verified in round %s at %s. Left for you to verify: %s", round, short(head), check))
			r.askSignoff(id, check)
			continue
		}
		reason := fmt.Sprintf("Done in round %s at %s; gates and build pass", round, short(head))
		if b.SpecChangeID != "" {
			reason = fmt.Sprintf("Merged in round %s at %s; approved specification %s, revision %s; gates and build pass", round, short(head), b.SpecChangeID, b.SpecRevision)
		}
		batch = append(batch, closing{id, reason})
	}
	r.closeAll(batch)
}

type closing struct{ id, reason string }

// closeAll closes finished beads one after another, retrying transient
// failures, then keeps retrying the ones that failed for as long as a pass
// closes something: bd refuses to close a bead while one it waits on is open,
// so a chain of beads in the batch closes one link per pass, in any order. A
// close that still fails (the bead waits on a bead left open, say) is
// recorded on the round's bead and logged, so Needs you can offer to close
// it, instead of the bead silently staying open.
func (r *round) closeAll(batch []closing) {
	if len(batch) == 0 {
		return
	}
	repo, repoID := r.meta.RepoPath, r.meta.RepoID
	for _, c := range batch {
		r.ops["close:"+c.id] = true
	}
	r.goOp("close-batch:"+batch[0].id, func(ctx context.Context) func() {
		failed := map[string]error{}
		pending := batch
		// Each pass closes at least one bead or stops, so this ends after at
		// most len(batch) passes.
		for len(pending) > 0 && ctx.Err() == nil {
			var still []closing
			for _, c := range pending {
				_, err := retry(ctx, func() error { return r.m.opts.Beads.Close(ctx, repo, c.id, c.reason) })
				if err != nil {
					failed[c.id] = err
					still = append(still, c)
				} else {
					delete(failed, c.id)
				}
			}
			if len(still) == len(pending) {
				break
			}
			pending = still
		}
		r.m.repoChanged(repoID)
		return func() {
			for _, c := range batch {
				delete(r.ops, "close:"+c.id)
				current := r.bead(c.id)
				if current == nil {
					continue
				}
				if err := failed[c.id]; err != nil {
					current.CloseError = firstLine(err.Error())
					r.event("note", c.id, "Its work is done, but closing "+c.id+" failed: "+current.CloseError+". Close it from Needs you.")
					continue
				}
				current.Closed, current.CloseError = true, ""
			}
		}
	})
}

// bumpsMinor says whether the round's version bump is a minor one: it is when
// a feature or epic was integrated, and a patch otherwise.
func bumpsMinor(meta api.Round) bool {
	if meta.FinishEpic != "" {
		// A finished epic is a feature's worth of work arriving at once.
		return true
	}
	for _, b := range meta.Beads {
		if b.State == api.BeadIntegrated && (b.Type == "feature" || b.Type == "epic") {
			return true
		}
	}
	return false
}

// bump runs the repository's version bump in ws and commits it, returning the
// new version. onTop names the branch whose version it starts from, when the
// bump is being redone after merging it.
func (r *round) bump(ctx context.Context, ws string, cfg *repoconfig.Config, meta api.Round, onTop string) (string, error) {
	minor := bumpsMinor(meta)
	level := "patch"
	if minor {
		level = "minor"
	}
	res := r.m.opts.Gates.RunAll(ctx, ws, cfg, []repoconfig.Step{{Name: "version bump", Run: cfg.BumpCommand(minor)}}, true)
	if !gates.AllOK(res) {
		return "", fmt.Errorf("The version bump failed: %s", failure(res))
	}
	version := lastLine(res[0].Tail)
	msg := fmt.Sprintf("Bump version (%s) for round %s: %s", level, meta.ID, version)
	if onTop != "" {
		msg += ", on top of " + onTop
	}
	if _, err := gitx.CommitAll(ctx, ws, msg); err != nil {
		return "", fmt.Errorf("Committing the version bump failed: %v", err)
	}
	return version, nil
}

// bumpCommit finds the round's latest version bump commit in ws, if any.
func bumpCommit(ctx context.Context, ws, round string) string {
	sha, err := gitx.Run(ctx, ws, "log", "-1", "--format=%H", "--fixed-strings", "--grep", "for round "+round+": ", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(sha)
}

// startLand fast-forwards the target to the round's commit, first merging
// the target in and re-verifying if it moved, then pushes and cleans up.
func (r *round) startLand() {
	meta, cfg := clone(r.meta), r.cfg
	r.event("land", "", "Merging into "+meta.Target)
	r.saveLocked()
	r.goOp("land", func(ctx context.Context) func() { return r.land(ctx, meta, cfg) })
}

// land brings in the target if it moved and lands. The round's version bump
// was made from the base it started on, so before merging a moved target it
// is undone, and after the merge it is redone from the target's version. A
// merge the catalogue can't settle, or a check that fails with the target's
// changes, goes to a recovery turn.
func (r *round) land(ctx context.Context, meta api.Round, cfg *repoconfig.Config) func() {
	ws, final := meta.Workspace, meta.FinalCommit
	if err := r.m.guardSpec(ctx, meta, ws); err != nil {
		return r.specStop(err)
	}
	var target string
	n, err := retry(ctx, func() (e error) { target, e = gitx.RevParse(ctx, meta.RepoPath, "refs/heads/"+meta.Target); return e })
	if err != nil {
		return r.stopFn(api.Stop{Kind: api.StopGitError, Phase: "land", Reason: meta.Target + " can't be read: " + err.Error(), Attempted: tries(n)})
	}
	if gitx.IsAncestor(ctx, meta.RepoPath, target, final) {
		return r.completeLand(ctx, meta, cfg, final, meta.Version, false)
	}
	rebump := false
	if cfg.Version.Bump != "" && meta.Version != "" {
		if bumped := bumpCommit(ctx, ws, meta.ID); bumped != "" {
			if _, err := gitx.Run(ctx, ws, "revert", "--no-commit", bumped); err != nil {
				_ = gitx.ResetHard(ctx, ws, final)
				return r.stopFn(api.Stop{Kind: api.StopGitError, Phase: "land", Reason: "Undoing the round's version bump before merging " + meta.Target + " failed: " + firstLine(err.Error())})
			}
			if _, err := gitx.CommitAll(ctx, ws, fmt.Sprintf("Undo round %s's version bump to redo it on top of %s", meta.ID, meta.Target)); err != nil {
				_ = gitx.ResetHard(ctx, ws, final)
				return r.stopFn(api.Stop{Kind: api.StopGitError, Phase: "land", Reason: "Undoing the round's version bump failed: " + err.Error()})
			}
			rebump = true
		} else if meta.GoalText != "" {
			// A goal round started before nacred made its own bump commit
			// there folded the bump into the agent's combined commit: reverse
			// just the recorded bump in a new commit on top.
			if err := r.undoGoalVersion(ctx, ws, meta); err != nil {
				_ = gitx.ResetHard(ctx, ws, final)
				return r.stopFn(api.Stop{Kind: api.StopGitError, Phase: "land", Reason: "The target moved after this goal round's combined commit, and undoing the round's version bump before merging " + meta.Target + " failed: " + err.Error() + ". The round branch is preserved."})
			}
			rebump = true
		}
	}
	res, err := gitx.Merge(ctx, ws, meta.Target, fmt.Sprintf("Merge %s into round %s to bring it up to date", meta.Target, meta.ID))
	if err != nil {
		_ = gitx.ResetHard(ctx, ws, final)
		return r.stopFn(api.Stop{Kind: api.StopGitError, Phase: "land", Reason: fmt.Sprintf("Merging %s into the round failed: %s", meta.Target, firstLine(err.Error()))})
	}
	notes := []string{}
	if rebump {
		notes = append(notes, "fix:"+fixVersion)
	}
	if len(res.Conflicts) > 0 {
		fixes, ok := r.mechanical(ctx, ws, cfg, res.Conflicts)
		if !ok {
			problem := "git reported conflicts in: " + list(res.Conflicts, 20)
			log, _ := gitx.Run(ctx, meta.RepoPath, "log", "--oneline", "-30", final+".."+meta.Target)
			v := r.integrationVars("", res.Conflicts, nil)
			start := r.startIntegrationRun(ctx, api.IntegrationStep{Phase: "land-merge", PreMerge: final, Rebump: rebump, Problem: problem, Attempted: fixRungs(notes)},
				incomingPrompt(v, incoming{Name: meta.Target, Log: log, Landing: true}))
			return func() {
				r.recordFixes("", "the version", notes)
				start()
			}
		}
		notes = append(notes, fixes...)
	}
	return r.afterLandMerge(ctx, meta, cfg, rebump, notes)
}

// afterLandMerge redoes the version bump if it was undone, runs the final
// gates and build with the target merged in, and lands, or hands a failure
// to a recovery turn.
func (r *round) afterLandMerge(ctx context.Context, meta api.Round, cfg *repoconfig.Config, rebump bool, notes []string) func() {
	ws, final := meta.Workspace, meta.FinalCommit
	if err := r.m.guardSpec(ctx, meta, ws); err != nil {
		return r.specStop(err)
	}
	version := meta.Version
	if rebump {
		v, err := r.bump(ctx, ws, cfg, meta, meta.Target)
		if err != nil {
			_ = gitx.ResetHard(ctx, ws, final)
			return r.stopFn(api.Stop{Kind: api.StopBumpFailed, Phase: "land", Reason: err.Error()})
		}
		version = v
	}
	results := r.m.opts.Gates.RunAll(ctx, ws, cfg, finalSteps(cfg), true)
	if !gates.AllOK(results) {
		log, _ := gitx.Run(ctx, meta.RepoPath, "log", "--oneline", "-30", final+".."+meta.Target)
		v := r.integrationVars("", nil, results)
		start := r.startIntegrationRun(ctx, api.IntegrationStep{Phase: "land", PreMerge: final, Problem: failure(results), Attempted: fixRungs(notes)},
			incomingPrompt(v, incoming{Name: meta.Target, Log: log, Landing: true}))
		return func() {
			r.recordFixes("", "merging "+meta.Target, notes)
			r.meta.Version = version
			start()
		}
	}
	head, _ := gitx.RevParse(ctx, ws, "HEAD")
	land := r.completeLand(ctx, meta, cfg, head, version, true)
	return func() {
		r.recordFixes("", "merging "+meta.Target, notes)
		land()
	}
}

// completeLand fast-forwards the target to head, pushes and cleans up.
// rebuilt says the workspace was built again (after merging the target), so
// its artifacts replace the ones kept at the end of the round.
func (r *round) completeLand(ctx context.Context, meta api.Round, cfg *repoconfig.Config, head, version string, rebuilt bool) func() {
	ws := meta.Workspace
	if err := r.m.guardSpec(ctx, meta, ws); err != nil {
		return r.specStop(err)
	}
	tag := ""
	if landsOnTarget(meta, cfg) {
		tag = cfg.TagName(version)
	}
	if cfg.Version.Tag != "" && tag == "" && landsOnTarget(meta, cfg) {
		return r.stopFn(api.Stop{Kind: api.StopGitError, Phase: "land", Reason: "The version bump didn't print a usable release version for tagging: " + version})
	}
	var artifacts []string
	if rebuilt {
		for _, step := range cfg.Build {
			if step.Artifact != "" {
				if p, err := keepArtifact(filepath.Join(ws, step.Artifact), filepath.Join(r.dir, "artifacts")); err == nil {
					artifacts = append(artifacts, p)
				}
			}
		}
	}
	n, err := retry(ctx, func() error { return fastForward(ctx, meta.RepoPath, meta.Target, head) })
	if errors.Is(err, gitx.ErrNotFastForward) && landRace(meta.ID) < maxLandRaces {
		// The target moved after it was merged in: merge it again.
		return r.land(ctx, meta, cfg)
	}
	if err != nil {
		return r.stopFn(api.Stop{Kind: api.StopGitError, Phase: "land", Reason: fmt.Sprintf("Moving %s to %s failed: %v", meta.Target, short(head), err), Attempted: tries(n)})
	}
	if tag != "" {
		if err := gitx.Tag(ctx, meta.RepoPath, tag, head, fmt.Sprintf("Round %s: %s", meta.ID, version)); err != nil {
			return r.stopFn(api.Stop{Kind: api.StopGitError, Phase: "land", Reason: "The target moved, but tagging release " + tag + " failed: " + err.Error()})
		}
	}
	push := pushesOnLand(meta, cfg)
	pushErr := ""
	if push {
		if _, err := retry(ctx, func() error { return gitx.Push(ctx, meta.RepoPath, "origin", meta.Target) }); err != nil {
			pushErr = err.Error()
		} else if tag != "" {
			if _, err := retry(ctx, func() error { return gitx.PushTag(ctx, meta.RepoPath, "origin", tag) }); err != nil {
				pushErr = "tag " + tag + ": " + err.Error()
			}
		}
	}
	var epicNotes []string
	hadBranch := meta.FinishEpic != "" && gitx.BranchExists(ctx, meta.RepoPath, EpicBranch(meta.FinishEpic))
	if meta.FinishEpic != "" {
		epicNotes = r.finishedEpic(ctx, meta, head, push && pushErr == "")
	}
	dropped := r.dropCarried(ctx, meta)
	keep := map[string]bool{}
	for _, b := range meta.Beads {
		if b.State == api.BeadParked && b.Branch != "" {
			keep[b.Branch] = true
		}
	}
	cleanErr := r.m.opts.Workspaces.RemoveRound(ctx, meta.RepoPath, meta.RepoID, meta.ID, keep)
	return func() {
		r.meta.State, r.meta.LandedCommit, r.meta.FinalCommit, r.meta.EndedAt = api.RoundLanded, head, head, stamp()
		if len(r.meta.SpecContracts) > 0 {
			r.closeBeads(head)
		}
		r.meta.Version, r.meta.Integration = version, nil
		if len(artifacts) > 0 {
			r.meta.Artifacts = artifacts
		}
		text := fmt.Sprintf("Merged: %s is at %s", meta.Target, short(head))
		if tag != "" {
			r.meta.Tag = tag
			text += ", tagged " + tag
		}
		switch {
		case !push:
		case pushErr == "":
			r.meta.Pushed = true
			text += " and pushed"
		default:
			r.meta.Error = "The push failed: " + pushErr
			text += "; the push failed: " + firstLine(pushErr)
		}
		r.eventWith(api.RoundEvent{Kind: "land", Text: text, Commit: head})
		if meta.FinishEpic != "" && len(epicNotes) == 0 {
			done := meta.FinishEpic + " is finished and closed"
			if hadBranch {
				done += ", and its branch " + EpicBranch(meta.FinishEpic) + " removed"
			}
			r.event("note", meta.FinishEpic, done)
		}
		for _, n := range epicNotes {
			r.event("note", meta.FinishEpic, n)
		}
		if len(dropped) > 0 {
			r.event("note", "", "Removed the earlier rounds' kept branches this round continued: "+strings.Join(dropped, ", "))
		}
		if cleanErr != nil {
			r.event("note", "", "Cleaning up the round's worktrees and branches left something behind: "+firstLine(cleanErr.Error()))
		}
	}
}

// pushesOnLand says whether landing pushes the target: when the repository's
// landing policy does. Goal options never override repository policy.
func pushesOnLand(meta api.Round, cfg *repoconfig.Config) bool {
	return cfg.Land.Push
}

// fastForward is gitx.FastForward, replaced in tests to move the target first.
var fastForward = gitx.FastForward

// maxLandRaces bounds how often one landing merges a target that moved again
// between its merge and the fast-forward.
const maxLandRaces = 3

var landRaces sync.Map

// landRace counts one more race for the round's landing.
func landRace(round string) int {
	n, _ := landRaces.LoadOrStore(round, new(atomic.Int32))
	return int(n.(*atomic.Int32).Add(1)) - 1
}

// tries lists retries for a stop's Attempted.
func tries(n int) []string {
	if n <= 1 {
		return nil
	}
	return []string{fmt.Sprintf("retry:%d", n-1)}
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
