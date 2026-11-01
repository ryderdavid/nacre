package rounds

// What a round does before it stops, and what Ryder can do once it has.
//
// Every stop point runs a ladder first: a mechanical fix when the failure is
// one the catalogue knows (mechanical), a recovery turn in the round's
// workspace judged by the gates (startIntegrationRun with the incoming-merge
// phases), and a retry when the failure came from the environment (retry).
// Only when none is left does the round stop: stop() pauses it with the
// choices that fit, then records the state it failed in and, budget allowing,
// asks Claude what is wrong. The design is in
// openspec/changes/resilient-rounds.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
	"github.com/ryderdavid/nacre/bridge/internal/gates"
	"github.com/ryderdavid/nacre/bridge/internal/gitx"
	"github.com/ryderdavid/nacre/bridge/internal/repoconfig"
)

// ---- transient retry ----

// retryBackoff is how long to wait before each retry of an environmental
// failure. Tests shorten it.
var retryBackoff = []time.Duration{5 * time.Second, 30 * time.Second}

// transientMarkers are fragments of error text that come from the
// environment rather than the request: a lock someone else holds, a busy
// file, a dropped connection, a service that is briefly unavailable.
var transientMarkers = []string{
	"index.lock", "unable to create", "file exists",
	"database is locked", "could not acquire lock", "lock is held", "resource temporarily unavailable",
	"text file busy", "too many open files", "device or resource busy",
	"connection reset", "connection refused", "broken pipe", "i/o timeout", "tls handshake timeout",
	"temporary failure", "no such host", "unexpected eof", "service unavailable", "overloaded",
}

// transient reports whether err looks environmental and worth retrying.
func transient(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, m := range transientMarkers {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}

// retry runs f until it succeeds, fails with an error that isn't transient,
// or the backoffs run out. It returns how many times it ran f.
func retry(ctx context.Context, f func() error) (int, error) {
	for i := 0; ; i++ {
		err := f()
		if err == nil || !transient(err) || i >= len(retryBackoff) {
			return i + 1, err
		}
		select {
		case <-ctx.Done():
			return i + 1, err
		case <-time.After(retryBackoff[i]):
		}
	}
}

// retried records that a step needed more than one try. Caller holds r.mu.
func (r *round) retried(bead, what string, attempts int) {
	if attempts > 1 {
		r.event("recover", bead, fmt.Sprintf("%s worked on try %d after a transient error", what, attempts))
	}
}

// ---- attempts ----

// attempt records rungs on the integration step in progress and saves the
// round at once, so a reload mid-ladder knows they ran and doesn't run them
// again. A turn rung replaces the same phase's earlier count. Caller holds
// r.mu.
func (r *round) attempt(rungs ...string) {
	st := r.meta.Integration
	if st == nil || len(rungs) == 0 {
		return
	}
	for _, rung := range rungs {
		if prefix, _, ok := strings.Cut(rung, "×"); ok && strings.HasPrefix(rung, "turn:") {
			st.Attempted = slices.DeleteFunc(st.Attempted, func(a string) bool { return strings.HasPrefix(a, prefix+"×") })
		}
		if !slices.Contains(st.Attempted, rung) {
			st.Attempted = append(st.Attempted, rung)
		}
	}
	r.saveLocked()
}

// turnRung names n recovery turns in phase.
func turnRung(phase string, n int) string { return fmt.Sprintf("turn:%s×%d", phase, max(n, 1)) }

// fixRungs names mechanical fixes as rungs ("fix:<kind>[:<file>]").
func fixRungs(fixes []string) []string {
	out := make([]string, 0, len(fixes))
	for _, f := range fixes {
		if !strings.HasPrefix(f, "fix:") {
			f = "fix:" + f
		}
		out = append(out, f)
	}
	return out
}

// ---- mechanical fixes ----

// fixBeadsLog and fixRegenerate are the conflict fixes the catalogue applies
// by path. The version fix happens before a landing merge instead (the
// round's bump is undone and redone), so it never sees a conflict.
const (
	fixBeadsLog   = "beads-jsonl"
	fixRegenerate = "regenerate"
	fixVersion    = "version"
)

func isBeadsLog(file string) bool {
	return strings.HasPrefix(file, ".beads/") && strings.HasSuffix(file, ".jsonl")
}

// mechanical settles a conflicted merge in ws without Claude when every
// conflicted path is one the catalogue handles, and commits the merge. It
// returns the fixes it applied; when it can't, it returns ok=false and leaves
// the merge in progress for the caller to undo or hand on.
func (r *round) mechanical(ctx context.Context, ws string, cfg *repoconfig.Config, conflicts []string) (fixes []string, ok bool) {
	regen := map[string]repoconfig.Regenerate{}
	for _, f := range conflicts {
		if isBeadsLog(f) {
			continue
		}
		rg, found := cfg.Merge.Regenerates(f)
		if !found {
			return nil, false
		}
		regen[f] = rg
	}
	commands := map[string]repoconfig.Regenerate{}
	for _, f := range conflicts {
		if isBeadsLog(f) {
			if err := unionLines(ctx, ws, f); err != nil {
				return nil, false
			}
			fixes = append(fixes, fixBeadsLog+":"+f)
			continue
		}
		if _, err := gitx.Run(ctx, ws, "checkout", "--theirs", "--", f); err != nil {
			return nil, false
		}
		commands[regen[f].Run] = regen[f]
		fixes = append(fixes, fixRegenerate+":"+f)
	}
	for _, rg := range commands {
		res := r.m.opts.Gates.RunAll(ctx, ws, cfg, []repoconfig.Step{{Name: "regenerate " + rg.Path, Run: rg.Run}}, true)
		if !gates.AllOK(res) {
			return nil, false
		}
	}
	if _, err := gitx.Run(ctx, ws, append([]string{"add", "--"}, conflicts...)...); err != nil {
		return nil, false
	}
	if left, _ := gitx.Conflicts(ctx, ws); len(left) > 0 {
		return nil, false
	}
	if _, err := gitx.Run(ctx, ws, "commit", "-q", "--no-edit"); err != nil {
		return nil, false
	}
	return fixes, true
}

// unionLines resolves a conflicted line-per-record file (a beads JSONL log)
// as every line from both sides once, ours first, in order.
func unionLines(ctx context.Context, ws, file string) error {
	ours, err := gitx.Run(ctx, ws, "show", ":2:"+file)
	if err != nil {
		return err
	}
	theirs, err := gitx.Run(ctx, ws, "show", ":3:"+file)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	var out strings.Builder
	for _, side := range []string{ours, theirs} {
		for _, line := range strings.Split(side, "\n") {
			if strings.TrimSpace(line) == "" || seen[line] {
				continue
			}
			seen[line] = true
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	return os.WriteFile(filepath.Join(ws, file), []byte(out.String()), 0o644)
}

// recordFixes logs the mechanical fixes a merge needed. Caller holds r.mu.
func (r *round) recordFixes(bead, what string, fixes []string) {
	if len(fixes) == 0 {
		return
	}
	words := make([]string, len(fixes))
	for i, f := range fixes {
		words[i] = fixWords(f)
	}
	r.event("recover", bead, fmt.Sprintf("Settled %s without Claude: %s", what, strings.Join(words, ", ")))
}

// fixWords says what a mechanical fix did.
func fixWords(fix string) string {
	kind, file, _ := strings.Cut(strings.TrimPrefix(fix, "fix:"), ":")
	switch kind {
	case fixBeadsLog:
		return file + " (kept both sides' lines)"
	case fixRegenerate:
		return file + " (rebuilt)"
	case fixVersion:
		return "the version (bump undone and redone on top of the target)"
	}
	return fix
}

// ---- stopping ----

// stop pauses the round once recovery has run out. It offers the choices
// that fit the stop, then in the background records the state the step
// failed in (so "continue" can tell whether anything changed) and, when the
// budget allows, asks Claude what is wrong. Caller holds r.mu.
func (r *round) stop(s api.Stop) {
	if r.meta.State == api.RoundPaused {
		return
	}
	if r.meta.GoalText != "" {
		r.m.leases.release(r.meta.ID + "/goal")
		if r.meta.GoalRunID != "" {
			if run, err := r.m.opts.Runs.Run(r.meta.GoalRunID); err == nil && run.Active() {
				_, _ = r.m.opts.Runs.Cancel(run.ID)
			}
		}
	}
	s.Choices = r.choicesFor(s)
	if s.Kind == api.StopLimits {
		s.Spend, s.EstimateUSD = r.spendByBead()
	}
	r.meta.PausedFrom, r.meta.State, r.meta.PauseReason = r.meta.State, api.RoundPaused, s.Reason
	r.meta.Stop = &s
	r.event("state", s.Bead, "Paused: "+s.Reason)
	r.saveLocked()
	r.afterStop()
}

// stopFn is stop as the result of a background operation.
func (r *round) stopFn(s api.Stop) func() { return func() { r.stop(s) } }

// afterStop records the state the step failed in, then diagnoses it. The
// fingerprint is its own quick operation, saved before the diagnosis starts:
// the diagnosis can take minutes, and a "continue" sent meanwhile must still
// find the fingerprint, or it would replay the failure. Caller holds r.mu.
func (r *round) afterStop() {
	if r.meta.Stop == nil {
		return
	}
	r.fingerprintStop()
	r.diagnoseStop()
}

// fingerprintStop records the state the current stop happened in, if it is
// about the repository and has none yet. Caller holds r.mu.
func (r *round) fingerprintStop() {
	cur := r.meta.Stop
	if cur == nil || cur.Fingerprint != "" || !stateful(cur.Kind) || r.ops["fingerprint"] {
		return
	}
	s, meta := *cur, clone(r.meta)
	r.goOp("fingerprint", func(ctx context.Context) func() {
		fp := r.m.fingerprint(ctx, meta, s)
		return func() {
			cur := r.meta.Stop
			if cur == nil || r.meta.State != api.RoundPaused {
				return
			}
			if cur.Reason != s.Reason {
				// The round stopped again while this ran; record that stop.
				r.fingerprintStop()
				return
			}
			cur.Fingerprint = fp
		}
	})
}

// diagnoseStop asks Claude what is wrong with the current stop, when the
// budget allows. Caller holds r.mu.
func (r *round) diagnoseStop() {
	if r.meta.Stop == nil || r.ops["diagnose"] {
		return
	}
	s, meta, events := *r.meta.Stop, clone(r.meta), r.recentEvents(30)
	if s.Kind == api.StopLimits || s.Kind == api.StopLegacy || r.remaining() < diagnosisCapUSD {
		return
	}
	r.goOp("diagnose", func(ctx context.Context) func() {
		d := r.m.diagnose(ctx, meta, s, events)
		return func() {
			if d == nil {
				return
			}
			r.meta.OverheadUSD += d.cost
			cur := r.meta.Stop
			if cur == nil || r.meta.State != api.RoundPaused || cur.Reason != s.Reason || d.Summary == "" {
				return
			}
			cur.Diagnosis = &api.Diagnosis{Summary: d.Summary, Cause: d.Cause, CostUSD: d.cost}
			for i := range cur.Choices {
				if e := d.explain[cur.Choices[i].ID]; e != "" {
					cur.Choices[i].Explanation = e
				}
			}
		}
	})
}

// stateful reports whether a stop is about the state of the repository, so
// retrying it without a change would fail the same way. A review that
// couldn't be read, or nacre.yaml being fixed elsewhere, isn't.
func stateful(kind string) bool {
	switch kind {
	case api.StopMergeConflict, api.StopGatesFailed, api.StopGitError, api.StopBumpFailed:
		return true
	}
	return false
}

// diagnoseQuestion adds Claude's read of a failed merge to the pre-flight
// question it reopened. Caller holds r.mu.
func (r *round) diagnoseQuestion(branch, problem string) {
	if r.ops["diagnose"] || r.remaining() < diagnosisCapUSD {
		return
	}
	s := api.Stop{Kind: api.StopMergeConflict, Phase: "preflight", Branch: branch, Reason: problem,
		Choices: []api.StopChoice{
			{ID: "merge", Label: AnswerMergeFirst + " (after merging it by hand)"},
			{ID: "ignore", Label: AnswerIgnore}, {ID: "stop", Label: AnswerStop},
		}}
	meta, events := clone(r.meta), r.recentEvents(30)
	r.goOp("diagnose", func(ctx context.Context) func() {
		d := r.m.diagnose(ctx, meta, s, events)
		return func() {
			if d == nil {
				return
			}
			r.meta.OverheadUSD += d.cost
			for i := range r.meta.Questions {
				q := &r.meta.Questions[i]
				if q.Branch == branch && q.Answer == "" && q.Problem == problem && d.Summary != "" {
					q.Problem = problem + "\n\n" + d.Summary
				}
			}
		}
	})
}

// choicesFor lists the ways forward from a stop.
func (r *round) choicesFor(s api.Stop) []api.StopChoice {
	var out []api.StopChoice
	add := func(id, label, explanation string) {
		out = append(out, api.StopChoice{ID: id, Label: label, Explanation: explanation})
	}
	switch s.Kind {
	case api.StopLimits:
		add(api.ChoiceRaiseLimits, "Raise the limit", "Give the round more budget or time; it carries on where it stopped.")
	case api.StopMergeConflict, api.StopGatesFailed:
		add(api.ChoiceRetryWithBudget, "Let Claude try again", "Another round of recovery turns, with the budget you give it.")
		add(api.ChoiceContinue, "I fixed it, carry on", "Use this after fixing it by hand in "+r.meta.Workspace+". The round checks that something changed first.")
	case api.StopNothingIntegrated:
	default:
		add(api.ChoiceContinue, "Try again", "Retries the step that stopped, once something has changed.")
	}
	if s.Bead != "" && s.Kind != api.StopLimits {
		add(api.ChoiceSkipBead, "Set "+s.Bead+" aside", "Park "+s.Bead+" with its work kept on its branch; the rest of the round carries on.")
	}
	if s.Branch != "" {
		add(api.ChoiceSkipBranch, "Leave "+s.Branch+" out", "The beads start without "+s.Branch+"'s work.")
	}
	add(api.ChoiceCancel, "Cancel the round", "Stop here. Every branch is kept, and a later round that includes these beads continues their work.")
	return out
}

// spendByBead is what each bead's runs have cost, and an estimate of what
// the unfinished ones need, from the average of the finished ones.
func (r *round) spendByBead() ([]api.BeadSpend, float64) {
	cost := map[string]float64{}
	for _, id := range r.meta.RunIDs {
		if run, err := r.m.opts.Runs.Run(id); err == nil && run.IssueID != "" {
			cost[run.IssueID] += run.CostUSD
		}
	}
	var out []api.BeadSpend
	done, doneCost, open := 0, 0.0, 0
	for _, b := range r.meta.Beads {
		finished := b.State == api.BeadIntegrated
		out = append(out, api.BeadSpend{Bead: b.ID, SpentUSD: cost[b.ID], Done: finished})
		switch {
		case finished:
			done++
			doneCost += cost[b.ID]
		case b.State != api.BeadParked && b.State != api.BeadSkipped:
			open++
		}
	}
	if done == 0 || open == 0 {
		return out, 0
	}
	avg := doneCost / float64(done)
	need := 0.0
	for i, b := range r.meta.Beads {
		if s := out[i]; !s.Done && s.SpentUSD < avg && b.State != api.BeadParked && b.State != api.BeadSkipped {
			need += avg - s.SpentUSD
		}
	}
	return out, need
}

func (r *round) recentEvents(n int) []api.RoundEvent {
	if len(r.events) > n {
		return append([]api.RoundEvent{}, r.events[len(r.events)-n:]...)
	}
	return append([]api.RoundEvent{}, r.events...)
}

// fingerprint summarises the state a stop happened in: the commits and
// working tree involved. "continue" is refused while it is unchanged.
func (m *Manager) fingerprint(ctx context.Context, meta api.Round, s api.Stop) string {
	var parts []string
	ref := func(label, dir, name string) {
		sha, _ := gitx.RevParse(ctx, dir, name)
		parts = append(parts, label+"="+sha)
	}
	if _, err := os.Stat(meta.Workspace); err == nil {
		ref("round", meta.Workspace, "HEAD")
		status, _ := gitx.Run(ctx, meta.Workspace, "status", "--porcelain")
		parts = append(parts, "status="+status)
	}
	ref("target", meta.RepoPath, "refs/heads/"+meta.Target)
	// Landing moves the checkout: clearing what blocked it there is a change.
	if s.Kind == api.StopGitError || s.Phase == "land" {
		status, _ := gitx.Run(ctx, meta.RepoPath, "status", "--porcelain", "--untracked-files=all")
		parts = append(parts, "checkout="+status)
	}
	if s.Branch != "" {
		ref("branch", meta.RepoPath, s.Branch)
	}
	for _, b := range meta.Beads {
		if b.ID == s.Bead && b.Branch != "" {
			ref("bead", meta.RepoPath, b.Branch)
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:8])
}

// ---- diagnosis ----

// diagnosisCapUSD and diagnosisTimeout bound the diagnosis call.
const (
	diagnosisCapUSD  = 0.50
	diagnosisTimeout = 3 * time.Minute
)

const diagnosisSystem = `You explain why an automated round of software work stopped. A round takes several beads (issues) in one repository, implements each on its own branch, merges them into an integration branch, checks them with the repository's gates and build, and lands the result on the target branch. It has already tried to recover by itself before stopping. Look at what actually happened (the round's log, git, the code) and tell the owner, in plain words for a phone screen, what is wrong and what each available choice would do. You can read the repository and run read-only bd and git commands; you cannot change anything. Be specific: name files, commits and beads. Don't recommend anything that isn't one of the choices.`

var diagnosisSchema = map[string]any{
	"type": "object", "additionalProperties": false,
	"required": []string{"summary", "cause", "choices"},
	"properties": map[string]any{
		"summary": map[string]any{"type": "string", "description": "Two or three sentences: what is wrong, in plain words"},
		"cause":   map[string]any{"type": "string", "description": "The underlying cause, naming files, commits or beads; empty if unknown"},
		"choices": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object", "additionalProperties": false, "required": []string{"id", "explanation"},
				"properties": map[string]any{
					"id":          map[string]any{"type": "string"},
					"explanation": map[string]any{"type": "string", "description": "One sentence: what this choice would do here"},
				},
			},
		},
	},
}

type diagnosis struct {
	Summary string `json:"summary"`
	Cause   string `json:"cause"`
	Choices []struct {
		ID          string `json:"id"`
		Explanation string `json:"explanation"`
	} `json:"choices"`
	explain map[string]string
	cost    float64
}

// diagnose asks Claude, read-only, what is wrong. It returns nil when the
// call fails; the stop then shows its plain reason.
func (m *Manager) diagnose(ctx context.Context, meta api.Round, s api.Stop, events []api.RoundEvent) *diagnosis {
	ctx, cancel := context.WithTimeout(ctx, diagnosisTimeout)
	defer cancel()
	cwd := meta.Workspace
	if _, err := os.Stat(cwd); err != nil {
		cwd = meta.RepoPath
	}
	var p strings.Builder
	fmt.Fprintf(&p, "Round %s in %s stopped.\n\nWhy, as nacred saw it: %s\n", meta.ID, meta.RepoName, s.Reason)
	if len(s.Attempted) > 0 {
		fmt.Fprintf(&p, "\nWhat it already tried: %s\n", strings.Join(s.Attempted, ", "))
	}
	fmt.Fprintf(&p, "\nThe integration worktree is %s on branch %s; the target is %s (the round started from %s).\n", meta.Workspace, meta.Branch, meta.Target, short(meta.Base))
	p.WriteString("\nBeads:\n")
	for _, b := range meta.Beads {
		fmt.Fprintf(&p, "- %s %q: %s", b.ID, b.Title, b.State)
		if b.Reason != "" {
			fmt.Fprintf(&p, " (%s)", b.Reason)
		}
		if b.Branch != "" {
			fmt.Fprintf(&p, ", branch %s", b.Branch)
		}
		p.WriteByte('\n')
	}
	if out, err := gitx.Run(ctx, cwd, "status", "--short", "--branch"); err == nil {
		fmt.Fprintf(&p, "\ngit status in the worktree:\n```\n%s\n```\n", lastN(out, 40))
	}
	if out, err := gitx.Run(ctx, meta.RepoPath, "log", "--oneline", "-15", meta.Base+".."+meta.Target); err == nil && out != "" {
		fmt.Fprintf(&p, "\nCommits on %s since the round started:\n```\n%s\n```\n", meta.Target, out)
	}
	p.WriteString("\nThe round's recent log:\n")
	for _, e := range events {
		fmt.Fprintf(&p, "- [%s] %s%s\n", e.Kind, beadPrefix(e.Bead), oneLine(e.Text, 400))
	}
	p.WriteString("\nThe owner's choices (use these ids):\n")
	for _, c := range s.Choices {
		fmt.Fprintf(&p, "- %s: %s\n", c.ID, c.Label)
	}
	schema, _ := json.Marshal(diagnosisSchema)
	out, err := m.opts.Headless.Run(ctx, cwd, claude.Headless{
		Prompt: p.String(), System: diagnosisSystem, Schema: schema, Model: meta.Model,
		AllowedTools: claude.BdReadTools, ReadRepo: true, MaxBudgetUSD: diagnosisCapUSD,
	}, nil)
	if err != nil {
		return nil
	}
	var d diagnosis
	if json.Unmarshal(out.Output, &d) != nil || strings.TrimSpace(d.Summary) == "" {
		return &diagnosis{cost: out.CostUSD}
	}
	d.Summary, d.Cause, d.cost = strings.TrimSpace(d.Summary), strings.TrimSpace(d.Cause), out.CostUSD
	d.explain = map[string]string{}
	for _, c := range d.Choices {
		d.explain[c.ID] = strings.TrimSpace(c.Explanation)
	}
	return &d
}

func beadPrefix(bead string) string {
	if bead == "" {
		return ""
	}
	return bead + ": "
}

// ---- choices ----

// Choose carries out one of a stopped round's choices. budgetUSD is the
// extra budget for retry-with-budget, or the new budget for raise-limits;
// minutes is a new time limit.
func (m *Manager) Choose(id, choice string, budgetUSD float64, minutes int) (api.Round, error) {
	r := m.get(id)
	if r == nil {
		return api.Round{}, ErrNotFound
	}
	snap := r.metaCopy()
	if snap.State != api.RoundPaused || snap.Stop == nil {
		return snap, conflict("the round isn't stopped")
	}
	if choice != api.ChoiceCancel {
		if err := m.guardSpec(m.ctx, snap, snap.Workspace); err != nil {
			return snap, conflict(err.Error())
		}
	}
	s := *snap.Stop
	if !hasChoice(s, choice) {
		return snap, conflict(fmt.Sprintf("%q isn't one of this stop's choices", choice))
	}
	fp := ""
	if choice == api.ChoiceContinue && s.Fingerprint != "" {
		fp = m.fingerprint(m.ctx, snap, s)
	}
	r.mu.Lock()
	defer r.poke()
	defer r.mu.Unlock()
	if r.meta.State != api.RoundPaused || r.meta.Stop == nil || r.meta.Stop.Reason != s.Reason {
		return clone(r.meta), conflict("the round has moved on since; reload it")
	}
	switch choice {
	case api.ChoiceCancel:
		r.cancelLocked("Cancelled")
	case api.ChoiceRaiseLimits:
		if budgetUSD <= 0 && minutes <= 0 {
			return clone(r.meta), conflict("give a new budget or time limit")
		}
		if budgetUSD > 0 {
			r.meta.BudgetUSD = budgetUSD
		}
		if minutes > 0 {
			r.meta.TimeLimitMinutes = minutes
		}
		r.resumeFromStop(fmt.Sprintf("Limits raised to $%.2f and %d minutes; carrying on", r.meta.BudgetUSD, r.meta.TimeLimitMinutes))
	case api.ChoiceRetryWithBudget:
		if budgetUSD > 0 && r.meta.SpentUSD+budgetUSD > r.meta.BudgetUSD {
			r.meta.BudgetUSD = r.meta.SpentUSD + budgetUSD
		}
		r.resumeFromStop(fmt.Sprintf("Trying again with $%.2f left", r.remaining()))
	case api.ChoiceContinue:
		// A landing whose target moved needs no change from Ryder: trying
		// again merges the target as it is now, so its fingerprint doesn't
		// matter.
		moved := s.Phase == "land" && strings.Contains(s.Reason, gitx.ErrNotFastForward.Error())
		if !moved && (r.ops["fingerprint"] || r.meta.Stop.Fingerprint != s.Fingerprint) {
			return clone(r.meta), conflict("The round is still recording the state it stopped in; try again in a moment.")
		}
		if s.Fingerprint != "" && fp == s.Fingerprint && !moved {
			return clone(r.meta), conflict("Nothing has changed since this failed, so it would fail the same way. Fix it first, or pick another choice.")
		}
		r.resumeFromStop("Carrying on after a change")
	case api.ChoiceSkipBead:
		r.park(r.bead(s.Bead), "You set it aside when the round stopped: "+firstLine(s.Reason))
		r.resumeFromStop("Carrying on without " + s.Bead)
	case api.ChoiceSkipBranch:
		for i := range r.meta.Questions {
			q := &r.meta.Questions[i]
			if q.Kind == api.FindingUnmergedWork && q.Branch == s.Branch {
				q.Answer, q.AnsweredAt, q.Problem = AnswerIgnore, stamp(), ""
			}
		}
		r.resumeFromStop("Leaving " + s.Branch + " out")
	}
	r.saveLocked()
	return clone(r.meta), nil
}

func hasChoice(s api.Stop, id string) bool {
	for _, c := range s.Choices {
		if c.ID == id {
			return true
		}
	}
	return false
}

// resumeFromStop clears the stop and goes back to the step it came from.
// Caller holds r.mu.
func (r *round) resumeFromStop(text string) {
	if r.meta.GoalText != "" && r.meta.PausedFrom != api.RoundReadyToLand {
		r.meta.GoalRetry = true
	}
	s := r.meta.Stop
	to := r.meta.PausedFrom
	if to == "" || to == api.RoundPaused {
		to = api.RoundImplementing
	}
	r.meta.State, r.meta.PausedFrom, r.meta.PauseReason, r.meta.Stop = to, "", "", nil
	r.event("state", "", text)
	if s == nil {
		return
	}
	switch s.Phase {
	case "land":
		r.meta.State, r.meta.Integration = api.RoundReadyToLand, nil
		if !r.ops["land"] {
			r.startLand()
		}
	case "preflight":
		r.meta.State, r.meta.Integration = api.RoundNeedsDecision, nil
	case "final":
		r.meta.State, r.meta.Integration = api.RoundIntegrating, nil
	}
}

// legacyStop describes a round that was paused before stops existed.
func legacyStop(reason string) *api.Stop {
	return &api.Stop{
		Kind: api.StopLegacy, Reason: reason,
		Choices: []api.StopChoice{
			{ID: api.ChoiceContinue, Label: "Try again", Explanation: "Retries the step that stopped."},
			{ID: api.ChoiceCancel, Label: "Cancel the round", Explanation: "Stop here; every branch is kept."},
		},
	}
}

// ---- set-aside work carries over ----

// keptWork is a bead's work that an earlier round set aside.
type keptWork struct {
	Round, Branch, Base, Reason string
}

// keptWorkFor finds the newest kept branch of bead from an earlier round in
// the same repository that hasn't been picked up yet.
func (m *Manager) keptWorkFor(repoID, roundID, bead string) *keptWork {
	m.mu.Lock()
	list := make([]*round, 0, len(m.rounds))
	for _, r := range m.rounds {
		list = append(list, r)
	}
	m.mu.Unlock()
	var best *keptWork
	for _, r := range list {
		r.mu.Lock()
		meta := r.meta
		if meta.ID != roundID && meta.RepoID == repoID && !meta.Active() && (best == nil || meta.ID > best.Round) {
			for _, b := range meta.Beads {
				left := b.State == api.BeadParked || b.State == api.BeadSkipped ||
					(meta.State == api.RoundCancelled && b.State != api.BeadIntegrated)
				if b.ID == bead && b.Branch != "" && b.PickedUpBy == "" && left {
					reason := b.Reason
					if reason == "" {
						reason = "the round was cancelled"
					}
					best = &keptWork{Round: meta.ID, Branch: b.Branch, Base: b.Base, Reason: reason}
				}
			}
		}
		r.mu.Unlock()
	}
	return best
}

// hasCommits reports whether branch exists and has commits beyond base.
func hasCommits(ctx context.Context, repo, base, branch string) bool {
	if !gitx.BranchExists(ctx, repo, branch) {
		return false
	}
	rng := branch
	if base != "" {
		rng = base + ".." + branch
	}
	out, err := gitx.Run(ctx, repo, "rev-list", "--count", rng)
	return err == nil && strings.TrimSpace(out) != "0"
}

// markPickedUp records on the earlier round that a later one continued its
// bead.
func (m *Manager) markPickedUp(round, bead, by string) {
	r := m.get(round)
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if b := r.bead(bead); b != nil {
		b.PickedUpBy = by
		r.event("note", bead, "Round "+by+" continues this bead's work from "+b.Branch)
		r.saveLocked()
	}
}

// dropCarried deletes the kept branches this round's integrated beads
// continued from, now that their work has landed.
func (r *round) dropCarried(ctx context.Context, meta api.Round) []string {
	var dropped []string
	wts, _ := gitx.Worktrees(ctx, meta.RepoPath)
	for _, b := range meta.Beads {
		if b.CarriedFrom == "" || b.State != api.BeadIntegrated {
			continue
		}
		for _, wt := range wts {
			if wt.Branch == b.CarriedFrom {
				_ = r.m.opts.Workspaces.Remove(ctx, meta.RepoPath, wt.Path)
			}
		}
		if err := gitx.DeleteBranch(ctx, meta.RepoPath, b.CarriedFrom); err == nil {
			dropped = append(dropped, b.CarriedFrom)
		}
	}
	return dropped
}
