package rounds

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
	"github.com/ryderdavid/nacre/bridge/internal/gates"
	"github.com/ryderdavid/nacre/bridge/internal/gitx"
	"github.com/ryderdavid/nacre/bridge/internal/workspace"
)

// Answers offered for unmerged work.
const (
	AnswerMergeFirst = "Merge it into the round first"
	AnswerIgnore     = "Ignore it"
	AnswerStop       = "Stop the round"
)

// Caps on what the review is given about other branches.
const (
	maxUnmergedBranches = 40
	maxUnmergedFiles    = 60
)

// preflightInput is what the review reads.
type preflightInput struct {
	Beads    []preflightBead
	Unmerged []gitx.UnmergedBranch
}

type preflightBead struct {
	Issue    issue
	Comments []api.Comment
	// After lists round beads this one depends on in bd.
	After []string
}

// startPreflight runs the read-only review of the round's beads.
func (r *round) startPreflight() {
	meta, cfg := clone(r.meta), r.cfg
	r.event("plan", "", "Reviewing the beads together before any code is written")
	r.saveLocked()
	r.goOp("preflight", func(ctx context.Context) func() {
		in, err := r.m.gatherPreflight(ctx, meta)
		if err != nil {
			return func() { r.pause("The pre-flight review couldn't read the beads: " + err.Error()) }
		}
		schema, _ := json.Marshal(preflightSchema)
		h := claude.Headless{
			Prompt: preflightPrompt(meta, in, cfg), System: preflightSystem, Schema: schema,
			Model: meta.Model, AllowedTools: claude.BdReadTools, ReadRepo: true,
		}
		out, err := r.m.opts.Headless.Run(ctx, meta.Workspace, h, nil)
		if err != nil {
			return func() { r.pause("The pre-flight review failed: " + err.Error()) }
		}
		plan, drafts, err := parsePlan(out.Output, meta, in)
		if err != nil {
			return func() { r.pause("The pre-flight review's answer couldn't be read: " + err.Error()) }
		}
		plan.CostUSD = out.CostUSD
		return func() { r.applyPlan(plan, drafts, in) }
	})
}

// gatherPreflight reads the beads, their comments and dependencies, and the
// branches not yet merged into the target.
func (m *Manager) gatherPreflight(ctx context.Context, meta api.Round) (preflightInput, error) {
	var in preflightInput
	ids := map[string]bool{}
	for _, b := range meta.Beads {
		ids[b.ID] = true
	}
	for _, b := range meta.Beads {
		raw, err := m.opts.Beads.Get(ctx, meta.RepoPath, b.ID)
		if err != nil {
			return in, fmt.Errorf("%s: %w", b.ID, err)
		}
		is := parseIssue(raw)
		comments, _ := m.opts.Beads.Comments(ctx, meta.RepoPath, b.ID)
		pb := preflightBead{Issue: is, Comments: comments}
		for _, d := range is.Dependencies {
			if d.Type == "blocks" && ids[d.DependsOnID] && d.DependsOnID != b.ID {
				pb.After = append(pb.After, d.DependsOnID)
			}
		}
		in.Beads = append(in.Beads, pb)
	}
	unmerged, err := gitx.Unmerged(ctx, meta.RepoPath, meta.Target, workspace.BranchPrefix)
	if err == nil {
		if len(unmerged) > maxUnmergedBranches {
			unmerged = unmerged[:maxUnmergedBranches]
		}
		for i := range unmerged {
			if len(unmerged[i].Files) > maxUnmergedFiles {
				unmerged[i].Files = unmerged[i].Files[:maxUnmergedFiles]
			}
		}
		in.Unmerged = unmerged
	}
	return in, nil
}

// planAnswer is the review's structured output.
type planAnswer struct {
	Summary string `json:"summary"`
	Beads   []struct {
		ID         string   `json:"id"`
		Areas      []string `json:"areas"`
		Files      []string `json:"files"`
		DependsOn  []string `json:"depends_on"`
		Needs      []string `json:"needs"`
		HumanCheck string   `json:"human_check"`
	} `json:"beads"`
	Findings []struct {
		Kind     string   `json:"kind"`
		Beads    []string `json:"beads"`
		Branch   string   `json:"branch"`
		Files    []string `json:"files"`
		Text     string   `json:"text"`
		Question string   `json:"question"`
		Options  []string `json:"options"`
	} `json:"findings"`
}

var findingKinds = map[string]bool{
	api.FindingContradiction: true, api.FindingAmbiguity: true, api.FindingOverlap: true,
	api.FindingOrdering: true, api.FindingSharedArtifact: true, api.FindingUnmergedWork: true,
}

// questionDraft is a finding's question text and suggested answers.
type questionDraft struct {
	text    string
	options []string
}

// parsePlan reads the review's answer, keeping only what refers to the
// round's beads and the branches it was shown. drafts holds each finding's
// question, by finding id.
func parsePlan(raw json.RawMessage, meta api.Round, in preflightInput) (*api.Plan, map[string]questionDraft, error) {
	if len(raw) == 0 {
		return nil, nil, fmt.Errorf("no structured answer")
	}
	var a planAnswer
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, nil, err
	}
	inRound := map[string]bool{}
	for _, b := range meta.Beads {
		inRound[b.ID] = true
	}
	branches := map[string]bool{}
	for _, u := range in.Unmerged {
		branches[u.Name] = true
	}
	plan := &api.Plan{Summary: strings.TrimSpace(a.Summary)}
	for _, b := range a.Beads {
		if !inRound[b.ID] {
			continue
		}
		plan.Beads = append(plan.Beads, api.PlanBead{
			ID: b.ID, Areas: b.Areas, Files: b.Files, DependsOn: only(b.DependsOn, inRound, b.ID),
			Needs: b.Needs, HumanCheck: strings.TrimSpace(b.HumanCheck),
		})
	}
	drafts := map[string]questionDraft{}
	for i, f := range a.Findings {
		if !findingKinds[f.Kind] {
			continue
		}
		beads := only(f.Beads, inRound, "")
		switch f.Kind {
		case api.FindingUnmergedWork:
			if !branches[f.Branch] || len(beads) == 0 {
				continue
			}
		case api.FindingOverlap, api.FindingOrdering, api.FindingContradiction:
			if len(beads) < 2 {
				continue
			}
		default:
			if len(beads) == 0 {
				continue
			}
		}
		fd := api.Finding{ID: fmt.Sprintf("f%d", i+1), Kind: f.Kind, Beads: beads, Branch: f.Branch, Files: f.Files, Text: strings.TrimSpace(f.Text)}
		plan.Findings = append(plan.Findings, fd)
		drafts[fd.ID] = questionDraft{text: strings.TrimSpace(f.Question), options: f.Options}
	}
	return plan, drafts, nil
}

// applyPlan records the plan, turns findings into sequencing and questions,
// and moves the round on.
func (r *round) applyPlan(plan *api.Plan, drafts map[string]questionDraft, in preflightInput) {
	r.meta.Plan = plan
	byID := map[string]api.PlanBead{}
	for _, pb := range plan.Beads {
		byID[pb.ID] = pb
	}
	issues := map[string]issue{}
	for _, b := range in.Beads {
		issues[b.Issue.ID] = b.Issue
	}
	order := map[string]int{}
	for i, b := range r.meta.Beads {
		order[b.ID] = i
	}
	resources := map[string]bool{}
	for _, res := range r.cfg.Resources {
		resources[res.Name] = true
	}

	// bd dependencies and the review's implied ones.
	for i := range r.meta.Beads {
		b := &r.meta.Beads[i]
		for _, pb := range in.Beads {
			if pb.Issue.ID == b.ID {
				for _, dep := range pb.After {
					r.addAfter(b.ID, dep)
				}
			}
		}
		pb := byID[b.ID]
		for _, dep := range pb.DependsOn {
			r.addAfter(b.ID, dep)
		}
		is := issues[b.ID]
		var needs []string
		for _, n := range pb.Needs {
			if resources[n] {
				needs = append(needs, n)
			}
		}
		for _, l := range is.Labels {
			if res, ok := strings.CutPrefix(l, "needs:"); ok && resources[res] {
				needs = append(needs, res)
			}
		}
		b.Needs = dedupe(needs)
		b.HumanCheck = pb.HumanCheck
		if b.HumanCheck == "" && is.hasLabel("human") {
			b.HumanCheck = "This bead is labelled human: a person has to check its acceptance criteria."
		}
	}

	// Overlaps and orderings become sequence: later beads stack on earlier.
	var questions []api.Question
	for _, f := range plan.Findings {
		switch f.Kind {
		case api.FindingOverlap:
			beads := append([]string{}, f.Beads...)
			sort.SliceStable(beads, func(i, j int) bool { return order[beads[i]] < order[beads[j]] })
			for i := 1; i < len(beads); i++ {
				r.addAfter(beads[i], beads[i-1])
			}
		case api.FindingOrdering:
			// The review lists them in the order they must happen.
			for i := 1; i < len(f.Beads); i++ {
				r.addAfter(f.Beads[i], f.Beads[i-1])
			}
		case api.FindingContradiction, api.FindingAmbiguity, api.FindingUnmergedWork:
			d := drafts[f.ID]
			q := api.Question{ID: "q" + strings.TrimPrefix(f.ID, "f"), FindingID: f.ID, Kind: f.Kind, Beads: f.Beads, Branch: f.Branch, Text: d.text, Options: d.options}
			if f.Kind == api.FindingUnmergedWork {
				q.Text = fmt.Sprintf("Branch %s isn't merged into %s and changes files these beads will touch (%s). %s", f.Branch, r.meta.Target, list(f.Files, 6), f.Text)
				q.Options = []string{AnswerMergeFirst, AnswerIgnore, AnswerStop}
			}
			if q.Text == "" {
				q.Text = f.Text
			}
			questions = append(questions, q)
		}
	}
	r.meta.Questions = questions

	summary := fmt.Sprintf("Pre-flight: %d finding(s), %d question(s)", len(plan.Findings), len(questions))
	if plan.Summary != "" {
		summary += ". " + plan.Summary
	}
	r.event("plan", "", summary)
	for _, b := range r.meta.Beads {
		r.note(b.ID, r.planNote(b, byID[b.ID]))
	}
	if len(questions) > 0 {
		r.meta.State = api.RoundNeedsDecision
		for _, q := range questions {
			r.event("question", strings.Join(q.Beads, ","), q.Text)
		}
		r.event("state", "", "Waiting for your decisions before any bead starts")
		return
	}
	r.startImplementing()
}

// addAfter records that bead waits for dep, unless that would make a cycle.
func (r *round) addAfter(bead, dep string) {
	b := r.bead(bead)
	if b == nil || r.bead(dep) == nil || bead == dep || contains(b.After, dep) || r.waitsFor(dep, bead, map[string]bool{}) {
		return
	}
	b.After = append(b.After, dep)
}

// waitsFor reports whether a (transitively) waits for b.
func (r *round) waitsFor(a, b string, seen map[string]bool) bool {
	if seen[a] {
		return false
	}
	seen[a] = true
	x := r.bead(a)
	if x == nil {
		return false
	}
	for _, d := range x.After {
		if d == b || r.waitsFor(d, b, seen) {
			return true
		}
	}
	return false
}

func (r *round) planNote(b api.RoundBead, pb api.PlanBead) string {
	var s strings.Builder
	fmt.Fprintf(&s, "Round %s plan for this bead.", r.meta.ID)
	if len(pb.Areas) > 0 {
		fmt.Fprintf(&s, " Expected to touch: %s.", strings.Join(pb.Areas, "; "))
	}
	if len(b.After) > 0 {
		fmt.Fprintf(&s, " Starts after: %s.", strings.Join(b.After, ", "))
	}
	if len(b.Needs) > 0 {
		fmt.Fprintf(&s, " Needs: %s.", strings.Join(b.Needs, ", "))
	}
	if b.HumanCheck != "" {
		fmt.Fprintf(&s, " Left for a person to check: %s", b.HumanCheck)
	}
	return s.String()
}

func (r *round) allAnswered() bool {
	for _, q := range r.meta.Questions {
		if q.Answer == "" {
			return false
		}
	}
	return true
}

// applyDecisions acts on the answers: merging unmerged work first when asked,
// or stopping the round.
func (r *round) applyDecisions() {
	var merge []string
	for _, q := range r.meta.Questions {
		if q.Kind != api.FindingUnmergedWork {
			continue
		}
		switch {
		case strings.HasPrefix(q.Answer, "Stop"):
			r.cancelLocked("You chose to stop the round at the pre-flight question about " + q.Branch)
			return
		case strings.HasPrefix(q.Answer, "Merge"):
			merge = append(merge, q.Branch)
		}
	}
	if len(merge) == 0 {
		r.startImplementing()
		r.saveLocked()
		return
	}
	ws, cfg, round := r.meta.Workspace, r.cfg, r.meta.ID
	r.goOp("decide", func(ctx context.Context) func() {
		for _, br := range merge {
			pre, _ := gitx.RevParse(ctx, ws, "HEAD")
			res, err := gitx.Merge(ctx, ws, br, fmt.Sprintf("Merge %s into round %s before the beads start", br, round))
			if err != nil || len(res.Conflicts) > 0 {
				_ = gitx.ResetHard(ctx, ws, pre)
				why := "conflicts in " + list(res.Conflicts, 10)
				if err != nil {
					why = err.Error()
				}
				return func() {
					r.reopen(br, fmt.Sprintf("Merging %s into the round failed (%s). Choose another answer, or merge it into %s by hand and choose to merge again.", br, why, r.meta.Branch))
				}
			}
			results := r.m.opts.Gates.RunAll(ctx, ws, cfg, beadGates(cfg), true)
			if !gates.AllOK(results) {
				_ = gitx.ResetHard(ctx, ws, pre)
				return func() {
					r.reopen(br, fmt.Sprintf("With %s merged in, the gates fail: %s. Choose another answer, or fix the branch and choose to merge again.", br, failure(results)))
				}
			}
		}
		head, _ := gitx.RevParse(ctx, ws, "HEAD")
		return func() {
			r.eventWith(api.RoundEvent{Kind: "merge", Text: "Merged " + strings.Join(merge, ", ") + " into the round before the beads start", Commit: head})
			r.startImplementing()
		}
	})
}

// reopen hands the question about branch back to Ryder when his answer
// couldn't be carried out, saying why. Pausing instead would only repeat the
// same failure on resume, since the answer that caused it would still stand.
func (r *round) reopen(branch, problem string) {
	for i := range r.meta.Questions {
		q := &r.meta.Questions[i]
		if q.Kind == api.FindingUnmergedWork && q.Branch == branch {
			q.Answer, q.AnsweredAt, q.Problem = "", "", problem
		}
	}
	r.meta.State = api.RoundNeedsDecision
	r.event("state", "", problem)
	r.saveLocked()
}

func (r *round) startImplementing() {
	r.meta.State, r.meta.StartedAt = api.RoundImplementing, stamp()
	r.event("state", "", "Implementing")
}

// only keeps ids in set, dropping self.
func only(ids []string, set map[string]bool, self string) []string {
	var out []string
	for _, id := range ids {
		if set[id] && id != self && !contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}
