package rounds

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
)

// recheck is how often a round looks at its runs even without being poked,
// as a safety net for a missed notification.
const recheck = 30 * time.Second

// minTurnBudget is the least budget worth starting a turn with.
const minTurnBudget = 0.25

// spawn starts the round's loop. Caller holds m.mu.
func (m *Manager) spawn(r *round) {
	m.wg.Add(1)
	go r.loop()
	r.poke()
}

func (r *round) poke() {
	select {
	case r.kick <- struct{}{}:
	default:
	}
}

func (r *round) loop() {
	defer r.m.wg.Done()
	tick := time.NewTicker(recheck)
	defer tick.Stop()
	for {
		select {
		case <-r.m.ctx.Done():
			return
		case <-r.kick:
		case <-tick.C:
		}
		if !r.advance() {
			return
		}
	}
}

// goOp runs work in the background under key. work returns a function that
// records the result; it runs with r.mu held, after which the round is saved
// and advanced again.
func (r *round) goOp(key string, work func(ctx context.Context) func()) {
	r.ops[key] = true
	r.m.wg.Add(1)
	go func() {
		defer r.m.wg.Done()
		apply := work(r.m.ctx)
		r.mu.Lock()
		delete(r.ops, key)
		if apply != nil && r.m.ctx.Err() == nil {
			apply()
		}
		r.saveLocked()
		r.mu.Unlock()
		r.poke()
	}()
}

// async runs a side effect (bd comments and labels) that nothing waits for.
func (r *round) async(f func(ctx context.Context)) {
	r.m.wg.Add(1)
	go func() {
		defer r.m.wg.Done()
		f(r.m.ctx)
		r.m.repoChanged(r.meta.RepoID)
	}()
}

// note posts a comment on a bead.
func (r *round) note(bead, text string) {
	repo := r.meta.RepoPath
	r.async(func(ctx context.Context) { _ = r.m.opts.Beads.AddComment(ctx, repo, bead, text) })
}

// advance moves the round forward as far as it can without waiting. It
// returns false once the round is finished and its loop can end.
func (r *round) advance() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.meta.Active() {
		return false
	}
	before := r.ver
	r.refreshSpend()
	switch r.meta.State {
	case api.RoundPlanning:
		if r.meta.Plan == nil && !r.ops["preflight"] {
			r.startPreflight()
		}
	case api.RoundNeedsDecision:
		if r.allAnswered() && !r.ops["decide"] {
			r.applyDecisions()
		}
	case api.RoundImplementing, api.RoundIntegrating:
		r.drive()
	case api.RoundReadyToLand:
		if r.meta.AutoLand && !r.ops["land"] {
			r.startLand()
		}
	}
	if r.ver == before {
		// Spend and run states can change without a transition; publish
		// them when they do.
		r.publishIfChanged()
	}
	return true
}

// refreshSpend sums what the round's runs have cost.
func (r *round) refreshSpend() {
	spent := 0.0
	if r.meta.Plan != nil {
		spent += r.meta.Plan.CostUSD
	}
	for _, id := range r.meta.RunIDs {
		if run, err := r.m.opts.Runs.Run(id); err == nil {
			spent += run.CostUSD
		}
	}
	r.meta.SpentUSD = spent
}

// publishIfChanged saves when a run the round follows changed state.
func (r *round) publishIfChanged() {
	changed := false
	for _, id := range r.meta.RunIDs {
		run, err := r.m.opts.Runs.Run(id)
		if err != nil {
			continue
		}
		key := run.Status + fmt.Sprintf("/%.4f/%v", run.CostUSD, run.Stalled)
		if r.seenRun[id] != key {
			r.seenRun[id] = key
			changed = true
		}
	}
	if changed {
		r.saveLocked()
	}
}

// remaining is the budget left for the next turn.
func (r *round) remaining() float64 { return r.meta.BudgetUSD - r.meta.SpentUSD }

// limitsHit pauses the round when its budget or time is used up.
func (r *round) limitsHit() bool {
	reason := ""
	switch {
	case r.remaining() < minTurnBudget:
		reason = fmt.Sprintf("The round has spent $%.2f of its $%.2f budget. Raise the budget to carry on.", r.meta.SpentUSD, r.meta.BudgetUSD)
	case r.meta.StartedAt != "" && r.meta.TimeLimitMinutes > 0:
		if t, err := time.Parse(time.RFC3339, r.meta.StartedAt); err == nil && time.Since(t) > time.Duration(r.meta.TimeLimitMinutes)*time.Minute {
			reason = fmt.Sprintf("The round has run for its %d-minute time limit. Raise the limit to carry on.", r.meta.TimeLimitMinutes)
		}
	}
	if reason == "" {
		return false
	}
	r.pause(reason)
	return true
}

func (r *round) pause(reason string) {
	if r.meta.State == api.RoundPaused {
		return
	}
	r.meta.PausedFrom, r.meta.State, r.meta.PauseReason = r.meta.State, api.RoundPaused, reason
	r.event("state", "", "Paused: "+reason)
	r.saveLocked()
}

func (r *round) setState(state, text string) {
	if r.meta.State == state {
		return
	}
	r.meta.State = state
	r.event("state", "", text)
	r.saveLocked()
}

// drive is the implementing and integrating phases: start beads that can
// start, follow their runs, verify them, and merge them one at a time.
func (r *round) drive() {
	if r.limitsHit() {
		return
	}
	for i := range r.meta.Beads {
		b := &r.meta.Beads[i]
		switch b.State {
		case api.BeadWaiting:
			r.maybeStart(b)
		case api.BeadRunning:
			r.watchRun(b)
		case api.BeadVerifying:
			if !r.ops["verify:"+b.ID] && !r.ops["start:"+b.ID] {
				r.startVerify(b)
			}
		}
	}

	busy := false
	for _, b := range r.meta.Beads {
		switch b.State {
		case api.BeadWaiting, api.BeadRunning, api.BeadVerifying:
			busy = true
		}
	}
	if busy {
		r.setState(api.RoundImplementing, "Implementing")
	} else {
		r.setState(api.RoundIntegrating, "Every bead has finished its own work; integrating")
	}

	switch {
	case r.meta.Integration != nil:
		r.watchIntegration()
	case r.ops["merge"]:
	default:
		if b := r.nextToMerge(); b != nil {
			r.startMerge(b)
		} else if !busy && !r.ops["finish"] && r.allSettled() {
			r.startFinish()
		}
	}
}

// prerequisites reports whether the bead's prerequisites are integrated, and
// the first one that never will be.
func (r *round) prerequisites(b *api.RoundBead) (ready bool, lost string) {
	ready = true
	for _, dep := range b.After {
		other := r.bead(dep)
		if other == nil {
			continue
		}
		switch other.State {
		case api.BeadIntegrated:
		case api.BeadParked, api.BeadSkipped:
			return false, dep
		default:
			ready = false
		}
	}
	return ready, ""
}

func (r *round) bead(id string) *api.RoundBead {
	for i := range r.meta.Beads {
		if r.meta.Beads[i].ID == id {
			return &r.meta.Beads[i]
		}
	}
	return nil
}

// maybeStart starts a waiting bead whose prerequisites are integrated and
// whose resources are free.
func (r *round) maybeStart(b *api.RoundBead) {
	if r.ops["start:"+b.ID] {
		return
	}
	ready, lost := r.prerequisites(b)
	if lost != "" {
		b.State, b.Reason = api.BeadSkipped, fmt.Sprintf("It builds on %s, which was parked", lost)
		r.event("bead", b.ID, "Skipped: "+b.Reason)
		r.note(b.ID, fmt.Sprintf("Round %s skipped this bead: %s. Start a new round for it once %s is done.", r.meta.ID, b.Reason, lost))
		r.saveLocked()
		return
	}
	if !ready {
		return
	}
	// The merge queue holds the integration branch still while a bead is
	// merging, so new beads start from a settled head.
	if r.meta.Integration != nil || r.ops["merge"] {
		return
	}
	env, ok := r.m.leases.acquire(r.meta.ID+"/"+b.ID, b.Needs, r.cfg)
	if !ok {
		return
	}
	r.startBead(b, env)
}

// startBead makes the bead's workspace and starts its run.
func (r *round) startBead(b *api.RoundBead, env []string) {
	id := b.ID
	meta := clone(r.meta)
	decisions := r.decisionsFor(id)
	shared := r.sharedArtifacts()
	integrated := r.integratedSummaries()
	budget := r.remaining()
	cfg := r.cfg
	r.goOp("start:"+id, func(ctx context.Context) func() {
		ws, err := r.m.opts.Workspaces.CreateBead(ctx, meta.RepoPath, meta.RepoID, meta.ID, id, meta.Branch)
		if err != nil {
			return func() { r.park(r.bead(id), "Its workspace couldn't be made: "+err.Error()) }
		}
		raw, err := r.m.opts.Beads.Get(ctx, meta.RepoPath, id)
		if err != nil {
			return func() { r.park(r.bead(id), "bd couldn't read it: "+err.Error()) }
		}
		comments, _ := r.m.opts.Beads.Comments(ctx, meta.RepoPath, id)
		prompt := beadPrompt(beadPromptVars{
			Round: meta, Issue: parseIssue(raw), Comments: comments, Branch: ws.Branch, Base: ws.Base,
			Decisions: decisions, Shared: shared, Integrated: integrated, Cfg: cfg, Env: env,
		})
		b := r.beadCopy(id)
		run, err := r.m.opts.Runs.Start(runs.StartRequest{
			RepoID: meta.RepoID, RepoName: meta.RepoName, RepoPath: meta.RepoPath,
			IssueID: id, IssueTitle: b.Title, Mode: meta.Mode, Model: meta.Model, Prompt: prompt,
			RoundID: meta.ID, Kind: "bead", Branch: ws.Branch, Workspace: ws.Path, Base: ws.Base,
			MaxBudgetUSD: budget, Env: env,
		})
		return func() {
			b := r.bead(id)
			if err != nil {
				r.park(b, "Its run couldn't start: "+err.Error())
				return
			}
			b.Branch, b.Workspace, b.Base, b.RunID = ws.Branch, ws.Path, ws.Base, run.ID
			b.State = api.BeadRunning
			r.meta.RunIDs = append(r.meta.RunIDs, run.ID)
			r.event("bead", id, fmt.Sprintf("Started on %s from %s", ws.Branch, short(ws.Base)))
		}
	})
}

// beadCopy returns a copy of a bead for use outside the lock.
func (r *round) beadCopy(id string) api.RoundBead {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b := r.bead(id); b != nil {
		return *b
	}
	return api.RoundBead{ID: id}
}

// watchRun follows a running bead's run.
func (r *round) watchRun(b *api.RoundBead) {
	if b.RunID == "" || r.ops["start:"+b.ID] || r.ops["followup:"+b.ID] {
		return
	}
	run, err := r.m.opts.Runs.Run(b.RunID)
	if err != nil {
		r.park(b, "Its run is gone: "+err.Error())
		return
	}
	if run.Active() {
		if run.Stalled && !r.stallCancelled[run.ID] {
			if b.Stalls == 0 {
				b.Stalls++
				r.stallCancelled[run.ID] = true
				r.event("bead", b.ID, "Stalled; stopping the turn and resuming it once")
				_, _ = r.m.opts.Runs.Cancel(run.ID)
				r.saveLocked()
			} else {
				r.park(b, "It stalled a second time")
			}
		}
		return
	}
	switch {
	case run.Status == api.StatusCancelled && r.stallCancelled[run.ID]:
		delete(r.stallCancelled, run.ID)
		r.followUp(b, stallPrompt)
	case run.Status == api.StatusSucceeded:
		b.State = api.BeadVerifying
		r.event("bead", b.ID, "Finished its turn; verifying")
		r.saveLocked()
		r.startVerify(b)
	case run.Status == api.StatusCancelled:
		r.park(b, "Its run was cancelled")
	default:
		switch {
		case b.FixAttempts >= r.cfg.Limits.FixAttempts:
			r.park(b, "Its run failed: "+firstLine(run.Error))
		case run.SessionID != "":
			b.FixAttempts++
			r.followUp(b, errorPrompt(run.Error))
		default:
			// Claude never got going (no session to resume): start the bead
			// again on its existing branch.
			b.FixAttempts++
			b.State, b.RunID = api.BeadWaiting, ""
			r.m.leases.release(r.meta.ID + "/" + b.ID)
			r.event("bead", b.ID, "Its run failed before Claude started ("+firstLine(run.Error)+"); starting it again")
			r.saveLocked()
		}
	}
}

// followUp resumes a bead's session.
func (r *round) followUp(b *api.RoundBead, message string) {
	if r.remaining() < minTurnBudget {
		r.limitsHit()
		return
	}
	id, runID, budget := b.ID, b.RunID, r.remaining()
	env := r.m.leases.env(r.meta.ID+"/"+id, r.cfg)
	b.State = api.BeadRunning
	r.goOp("followup:"+id, func(ctx context.Context) func() {
		_, err := r.m.opts.Runs.FollowupWith(runID, runs.Followup{Message: message, MaxBudgetUSD: budget, Env: env})
		return func() {
			if err != nil {
				r.park(r.bead(id), "Its follow-up couldn't start: "+err.Error())
				return
			}
			r.event("bead", id, "Sent back to fix: "+firstLine(message))
		}
	})
}

// park takes a bead out of the round; the others carry on.
func (r *round) park(b *api.RoundBead, reason string) {
	if b == nil || b.State == api.BeadParked {
		return
	}
	if b.RunID != "" {
		if run, err := r.m.opts.Runs.Run(b.RunID); err == nil && run.Active() {
			_, _ = r.m.opts.Runs.Cancel(b.RunID)
		}
	}
	b.State, b.Reason = api.BeadParked, reason
	r.m.leases.release(r.meta.ID + "/" + b.ID)
	r.event("bead", b.ID, "Parked: "+reason)
	repo, id, round, branch := r.meta.RepoPath, b.ID, r.meta.ID, b.Branch
	r.async(func(ctx context.Context) {
		_ = r.m.opts.Beads.Update(ctx, repo, id, bd.UpdateParams{AddLabels: []string{"round-parked"}})
		msg := fmt.Sprintf("Round %s parked this bead: %s.", round, reason)
		if branch != "" {
			msg += fmt.Sprintf(" Its work so far is on branch %s.", branch)
		}
		_ = r.m.opts.Beads.AddComment(ctx, repo, id, msg+" The rest of the round carries on without it.")
	})
	r.saveLocked()
}

// allSettled reports whether every bead is integrated, parked or skipped.
func (r *round) allSettled() bool {
	for _, b := range r.meta.Beads {
		switch b.State {
		case api.BeadIntegrated, api.BeadParked, api.BeadSkipped:
		default:
			return false
		}
	}
	return true
}

func (r *round) integratedCount() int {
	n := 0
	for _, b := range r.meta.Beads {
		if b.State == api.BeadIntegrated {
			n++
		}
	}
	return n
}

// ---- API actions ----

// Answer records Ryder's answer to a pre-flight question.
func (m *Manager) Answer(id, questionID, answer string) (api.Round, error) {
	r := m.get(id)
	if r == nil {
		return api.Round{}, ErrNotFound
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return api.Round{}, conflict("give an answer")
	}
	r.mu.Lock()
	defer r.poke()
	defer r.mu.Unlock()
	if r.meta.State != api.RoundNeedsDecision && r.meta.State != api.RoundPlanning {
		return clone(r.meta), conflict("the round isn't waiting for decisions")
	}
	for i := range r.meta.Questions {
		q := &r.meta.Questions[i]
		if q.ID != questionID {
			continue
		}
		q.Answer, q.AnsweredAt = answer, stamp()
		r.event("answer", "", fmt.Sprintf("%s → %s", q.Text, answer))
		for _, b := range q.Beads {
			r.note(b, fmt.Sprintf("Decision for round %s: %s\nAnswer: %s", r.meta.ID, q.Text, answer))
		}
		r.saveLocked()
		return clone(r.meta), nil
	}
	return clone(r.meta), conflict("no question " + questionID)
}

// PlanEdit changes the plan's sequencing before the beads involved start.
type PlanEdit struct {
	// Unlink removes "a waits for b" for each [a, b].
	Unlink [][2]string `json:"unlink"`
	// AcceptDependencies adds each [a, b] to bd as "a depends on b".
	AcceptDependencies [][2]string `json:"accept_dependencies"`
}

// EditPlan applies a PlanEdit.
func (m *Manager) EditPlan(id string, e PlanEdit) (api.Round, error) {
	r := m.get(id)
	if r == nil {
		return api.Round{}, ErrNotFound
	}
	r.mu.Lock()
	defer r.poke()
	defer r.mu.Unlock()
	for _, pair := range e.Unlink {
		b := r.bead(pair[0])
		if b == nil {
			return clone(r.meta), conflict("no bead " + pair[0] + " in the round")
		}
		if b.State != api.BeadWaiting {
			return clone(r.meta), conflict(b.ID + " has already started")
		}
		b.After = remove(b.After, pair[1])
		r.markFinding(pair, func(f *api.Finding) { f.Unlinked = true })
		r.event("plan", b.ID, fmt.Sprintf("%s no longer waits for %s", b.ID, pair[1]))
	}
	repo := r.meta.RepoPath
	for _, pair := range e.AcceptDependencies {
		a, dep := pair[0], pair[1]
		r.markFinding(pair, func(f *api.Finding) { f.Accepted = true })
		r.event("plan", a, fmt.Sprintf("%s now depends on %s in bd", a, dep))
		r.async(func(ctx context.Context) { _ = r.m.opts.Beads.DepAdd(ctx, repo, a, dep, "blocks") })
	}
	r.saveLocked()
	return clone(r.meta), nil
}

func (r *round) markFinding(pair [2]string, f func(*api.Finding)) {
	if r.meta.Plan == nil {
		return
	}
	for i := range r.meta.Plan.Findings {
		fd := &r.meta.Plan.Findings[i]
		if contains(fd.Beads, pair[0]) && contains(fd.Beads, pair[1]) {
			f(fd)
		}
	}
}

// RetryBead puts a parked or skipped bead back in the round.
func (m *Manager) RetryBead(id, beadID string) (api.Round, error) {
	r := m.get(id)
	if r == nil {
		return api.Round{}, ErrNotFound
	}
	r.mu.Lock()
	defer r.poke()
	defer r.mu.Unlock()
	if !r.meta.Active() {
		return clone(r.meta), conflict("the round has ended")
	}
	b := r.bead(beadID)
	if b == nil || (b.State != api.BeadParked && b.State != api.BeadSkipped) {
		return clone(r.meta), conflict(beadID + " isn't parked or skipped")
	}
	b.State, b.Reason, b.FixAttempts, b.Stalls = api.BeadWaiting, "", 0, 0
	b.RunID = ""
	switch r.meta.State {
	case api.RoundReadyToLand:
		r.meta.State = api.RoundImplementing
	case api.RoundPaused:
		r.meta.State, r.meta.PausedFrom, r.meta.PauseReason = api.RoundImplementing, "", ""
	}
	r.event("bead", beadID, "Back in the round")
	repo := r.meta.RepoPath
	r.async(func(ctx context.Context) {
		_ = r.m.opts.Beads.Update(ctx, repo, beadID, bd.UpdateParams{RemoveLabels: []string{"round-parked"}})
	})
	r.saveLocked()
	return clone(r.meta), nil
}

// SetLimits changes a round's budget and time limit; a round paused by a
// limit resumes once it has room again.
func (m *Manager) SetLimits(id string, budgetUSD float64, minutes int) (api.Round, error) {
	r := m.get(id)
	if r == nil {
		return api.Round{}, ErrNotFound
	}
	r.mu.Lock()
	defer r.poke()
	defer r.mu.Unlock()
	if budgetUSD > 0 {
		r.meta.BudgetUSD = budgetUSD
	}
	if minutes > 0 {
		r.meta.TimeLimitMinutes = minutes
	}
	r.event("note", "", fmt.Sprintf("Limits set to $%.2f and %d minutes", r.meta.BudgetUSD, r.meta.TimeLimitMinutes))
	if r.meta.State == api.RoundPaused && r.meta.PausedFrom != "" {
		r.meta.State, r.meta.PausedFrom, r.meta.PauseReason = r.meta.PausedFrom, "", ""
		r.event("state", "", "Resumed")
	}
	r.saveLocked()
	return clone(r.meta), nil
}

// Resume restarts a paused round once its cause is dealt with.
func (m *Manager) Resume(id string) (api.Round, error) {
	r := m.get(id)
	if r == nil {
		return api.Round{}, ErrNotFound
	}
	r.mu.Lock()
	defer r.poke()
	defer r.mu.Unlock()
	if r.meta.State != api.RoundPaused {
		return clone(r.meta), conflict("the round isn't paused")
	}
	to := r.meta.PausedFrom
	if to == "" || to == api.RoundPaused {
		to = api.RoundImplementing
	}
	r.meta.State, r.meta.PausedFrom, r.meta.PauseReason = to, "", ""
	r.event("state", "", "Resumed")
	r.saveLocked()
	return clone(r.meta), nil
}

// Cancel stops the round. Its branches stay so the work can be recovered.
func (m *Manager) Cancel(id string) (api.Round, error) {
	r := m.get(id)
	if r == nil {
		return api.Round{}, ErrNotFound
	}
	r.mu.Lock()
	defer r.poke()
	defer r.mu.Unlock()
	if !r.meta.Active() {
		return clone(r.meta), conflict("the round has already ended")
	}
	r.cancelLocked("Cancelled")
	return clone(r.meta), nil
}

// cancelLocked stops the round's runs, ends it and records why on its beads.
// Branches stay so the work can be recovered.
func (r *round) cancelLocked(why string) {
	for _, runID := range r.meta.RunIDs {
		if run, err := r.m.opts.Runs.Run(runID); err == nil && run.Active() {
			_, _ = r.m.opts.Runs.Cancel(runID)
		}
	}
	for _, b := range r.meta.Beads {
		r.m.leases.release(r.meta.ID + "/" + b.ID)
	}
	r.meta.State, r.meta.EndedAt = api.RoundCancelled, stamp()
	r.event("state", "", why+"; branches are kept")
	repo, round := r.meta.RepoPath, r.meta.ID
	beads := append([]api.RoundBead{}, r.meta.Beads...)
	r.async(func(ctx context.Context) {
		for _, b := range beads {
			_ = r.m.opts.Beads.Update(ctx, repo, b.ID, bd.UpdateParams{RemoveLabels: []string{"round:" + round}})
			msg := fmt.Sprintf("Round %s was cancelled.", round)
			if b.Branch != "" {
				msg += " Work so far is on branch " + b.Branch + "."
			}
			_ = r.m.opts.Beads.AddComment(ctx, repo, b.ID, msg)
		}
	})
	r.saveLocked()
}

// Land lands a ready round.
func (m *Manager) Land(id string) (api.Round, error) {
	r := m.get(id)
	if r == nil {
		return api.Round{}, ErrNotFound
	}
	r.mu.Lock()
	defer r.poke()
	defer r.mu.Unlock()
	if r.meta.State != api.RoundReadyToLand {
		return clone(r.meta), conflict("the round isn't ready to land")
	}
	if !r.ops["land"] {
		r.startLand()
	}
	return clone(r.meta), nil
}

func remove(list []string, s string) []string {
	out := list[:0:0]
	for _, x := range list {
		if x != s {
			out = append(out, x)
		}
	}
	return out
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}
