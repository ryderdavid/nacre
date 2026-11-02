package specflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/assist"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

type fakeBeads struct {
	mu             sync.Mutex
	issues         map[string]issue
	comments       map[string][]api.Comment
	creates        int
	loseCreate     bool
	loseComment    bool
	failDependency bool
	beforeGet      func()
	created        []bd.CreateParams
}

func newBeads() *fakeBeads {
	return &fakeBeads{issues: map[string]issue{"NAC-1": {ID: "NAC-1", Title: "Keep drafts", Description: "Retain drafts when navigating away", Design: "Use the current visual design", Acceptance: "Survives Back", Status: "open", Type: "task"}}, comments: map[string][]api.Comment{}}
}
func (f *fakeBeads) Get(_ context.Context, _, id string) (api.Issue, error) {
	f.mu.Lock()
	hook := f.beforeGet
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	is, ok := f.issues[id]
	if !ok {
		return nil, fmt.Errorf("missing %s", id)
	}
	b, _ := json.Marshal(is)
	return b, nil
}
func (f *fakeBeads) List(context.Context, string) ([]api.Issue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []api.Issue{}
	for _, is := range f.issues {
		b, _ := json.Marshal(is)
		out = append(out, b)
	}
	return out, nil
}
func (f *fakeBeads) Create(_ context.Context, _ string, p bd.CreateParams) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates++
	f.created = append(f.created, p)
	id := fmt.Sprintf("NAC-%d", f.creates+10)
	f.issues[id] = issue{ID: id, Title: p.Title, Description: p.Description, Design: p.Design, Acceptance: p.AcceptanceCriteria, Type: p.IssueType, Parent: p.Parent, Labels: p.Labels, Status: "open"}
	if f.loseCreate {
		f.loseCreate = false
		return "", errors.New("connection lost after create")
	}
	return id, nil
}
func (f *fakeBeads) Update(_ context.Context, _, id string, p bd.UpdateParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	is, ok := f.issues[id]
	if !ok {
		return errors.New("not found")
	}
	for _, pair := range []struct {
		dest  *string
		value *string
	}{{&is.Title, p.Title}, {&is.Description, p.Description}, {&is.Design, p.Design}, {&is.Acceptance, p.AcceptanceCriteria}, {&is.Type, p.IssueType}, {&is.Status, p.Status}, {&is.Parent, p.Parent}} {
		if pair.value != nil {
			*pair.dest = *pair.value
		}
	}
	for _, l := range p.AddLabels {
		if !contains(is.Labels, l) {
			is.Labels = append(is.Labels, l)
		}
	}
	f.issues[id] = is
	return nil
}
func (f *fakeBeads) Reopen(ctx context.Context, repo, id string) error {
	return f.Update(ctx, repo, id, bd.UpdateParams{Status: ptr("open")})
}
func (f *fakeBeads) Close(ctx context.Context, repo, id, _ string) error {
	return f.Update(ctx, repo, id, bd.UpdateParams{Status: ptr("closed")})
}
func (f *fakeBeads) DepAdd(_ context.Context, _, id, target, typ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failDependency {
		f.failDependency = false
		return errors.New("dependency unavailable")
	}
	is := f.issues[id]
	is.Dependencies = append(is.Dependencies, dependency{IssueID: id, DependsOnID: target, Type: typ})
	f.issues[id] = is
	return nil
}
func (f *fakeBeads) DepRemove(_ context.Context, _, id, target string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	is := f.issues[id]
	keep := is.Dependencies[:0]
	for _, d := range is.Dependencies {
		if d.DependsOnID != target {
			keep = append(keep, d)
		}
	}
	is.Dependencies = keep
	f.issues[id] = is
	return nil
}
func (f *fakeBeads) Comments(_ context.Context, _, id string) ([]api.Comment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return clone(f.comments[id]), nil
}
func (f *fakeBeads) AddComment(_ context.Context, _, id, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.comments[id] = append(f.comments[id], api.Comment{Text: text})
	if f.loseComment {
		f.loseComment = false
		return errors.New("connection lost after comment")
	}
	return nil
}
func (f *fakeBeads) edit(id string, fn func(*issue)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	is := f.issues[id]
	fn(&is)
	f.issues[id] = is
}

type fakeModel struct {
	mu    sync.Mutex
	calls []claude.Headless
	fn    func(context.Context, claude.Headless, int) (any, float64, error)
}

func (f *fakeModel) Run(ctx context.Context, _ string, h claude.Headless, _ func(api.RunEvent)) (assist.Outcome, error) {
	f.mu.Lock()
	f.calls = append(f.calls, h)
	n := len(f.calls)
	fn := f.fn
	f.mu.Unlock()
	if fn == nil {
		fn = normalModel
	}
	v, c, e := fn(ctx, h, n)
	b, _ := json.Marshal(v)
	return assist.Outcome{Output: b, CostUSD: c}, e
}
func (f *fakeModel) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.calls) }
func modelContext(h claude.Headless) Change {
	var p struct {
		Current Change `json:"current"`
	}
	_ = json.Unmarshal([]byte(h.Prompt), &p)
	return p.Current
}
func goodAuthor(c Change) authorResult {
	name := "retain-drafts"
	route := "new"
	tasks := "## 1. Retain drafts\n\n- [ ] Save on Back; run `go test ./drafts`\n\n## 2. Restore drafts\n\n- [ ] Restore after restart; run `go test ./drafts`\n"
	refs := []string{"openspec/config.yaml"}
	if c.ChangeName != "" {
		name = c.ChangeName
		route = "amend"
		refs = []string{"openspec/changes/" + name + "/proposal.md"}
		for _, a := range c.Artifacts {
			if strings.HasSuffix(a.Path, "/tasks.md") {
				tasks = a.Content
			}
		}
	}
	prefix := "openspec/changes/" + name + "/"
	return authorResult{Route: route, ChangeName: name, Summary: "Preserve drafts", Rationale: "Retain work and the existing visual design", References: refs, Artifacts: []Artifact{{prefix + "proposal.md", "## Why\nDrafts should survive navigation.\n"}, {prefix + "design.md", "Keep existing screen. Save drafts locally."}, {prefix + "tasks.md", tasks}, {prefix + "specs/drafts/spec.md", "## ADDED Requirements\n### Requirement: Retain drafts\nThe app SHALL retain drafts.\n#### Scenario: Back\n- WHEN Back is pressed\n- THEN draft is retained\n"}}, Dependencies: []sectionDependency{}, Retired: []RetiredSection{}}
}
func goodReview(c Change) reviewResult {
	r := reviewResult{Findings: []Finding{}}
	for _, key := range rubric {
		r.Assessments = append(r.Assessments, assessment{key, "Satisfies project constraints with explicit verification", []string{"openspec/config.yaml"}})
	}
	for _, f := range c.Findings {
		f.Disposition = "resolved"
		r.Findings = append(r.Findings, f)
	}
	return r
}
func normalModel(_ context.Context, h claude.Headless, _ int) (any, float64, error) {
	c := modelContext(h)
	if h.System == authorSystem {
		return goodAuthor(c), 0.1, nil
	}
	return goodReview(c), 0.1, nil
}
func fixture(t *testing.T) (string, *fakeBeads, *fakeModel, Options) {
	t.Helper()
	repo := t.TempDir()
	write(t, repo, "openspec/config.yaml", "schema: spec-driven\ncontext: Preserve the existing UI; retain user work.\n")
	write(t, repo, "AGENTS.md", "Keep the existing design. Test behavior.\n")
	b, model := newBeads(), &fakeModel{}
	return repo, b, model, Options{Dir: t.TempDir(), Beads: b, Headless: model, Validate: func(context.Context, string, string) error { return nil }}
}
func write(t *testing.T, repo, path, body string) {
	t.Helper()
	name := filepath.Join(repo, path)
	if e := os.MkdirAll(filepath.Dir(name), 0o755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(name, []byte(body), 0o600); e != nil {
		t.Fatal(e)
	}
}
func newManager(t *testing.T, opts Options) *Manager {
	t.Helper()
	m, e := New(opts)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { m.Shutdown(time.Second) })
	return m
}
func act(t *testing.T, m *Manager, repo, id string, a Action) Change {
	t.Helper()
	_, e := m.Act(context.Background(), repo, id, a)
	if e != nil {
		t.Fatal(e)
	}
	m.Wait()
	env, e := m.Get(context.Background(), repo, id)
	if e != nil {
		t.Fatal(e)
	}
	if env.Change == nil {
		t.Fatalf("missing change: %+v", env)
	}
	return *env.Change
}
func develop(t *testing.T, m *Manager, repo string) Change {
	t.Helper()
	c := act(t, m, repo, "NAC-1", Action{Action: "develop"})
	if c.Phase != "awaiting_approval" {
		t.Fatalf("development: phase=%s error=%s", c.Phase, c.Error)
	}
	return c
}
func approve(t *testing.T, m *Manager, repo string, c Change) Change {
	t.Helper()
	c = act(t, m, repo, c.SeedID, Action{Action: "approve", Revision: c.Revision})
	if c.Phase != "ready" {
		t.Fatalf("approval: phase=%s error=%s", c.Phase, c.Error)
	}
	return c
}

func TestDevelopPromoteAndPinnedContract(t *testing.T) {
	repo, b, model, opts := fixture(t)
	m := newManager(t, opts)
	c := develop(t, m, repo)
	if b.creates != 0 {
		t.Fatal("development created implementation work")
	}
	c = approve(t, m, repo, c)
	if len(c.Sections) != 2 || b.creates != 2 {
		t.Fatal("wrong mapping")
	}
	contract, e := m.Contract(context.Background(), repo, c.Sections[0].BeadID)
	if e != nil || contract == nil || contract.Revision != c.ApprovedRevision || len(contract.Artifacts) != 4 {
		t.Fatalf("contract=%+v err=%v", contract, e)
	}
	if _, e = m.Contract(context.Background(), repo, c.EpicID); e == nil {
		t.Fatal("epic accepted as implementation")
	}
	c = approve(t, m, repo, c)
	if b.creates != 2 {
		t.Fatal("duplicate promotion")
	}
	if model.count() != 2 {
		t.Fatal("unexpected calls")
	}
	for _, h := range model.calls {
		if h.Resume != "" || h.Persist || h.ReadRepo || !h.ReadSource || h.MaxBudgetUSD <= 0 || h.MaxBudgetUSD > MaxActionCostUSD {
			t.Fatalf("unsafe model request %+v", h)
		}
	}
	b.edit("NAC-1", func(is *issue) {
		if is.Description != "Retain drafts when navigating away" {
			t.Error("seed intent overwritten")
		}
	})
	for _, s := range c.Sections {
		comments, _ := b.Comments(context.Background(), repo, s.BeadID)
		if len(comments) != 1 {
			t.Fatal("missing or duplicated decision comment")
		}
	}
}
func TestApprovalAndExecutionRejectStaleContextAndSeed(t *testing.T) {
	for _, kind := range []string{"artifact", "goal", "seed"} {
		t.Run(kind, func(t *testing.T) {
			repo, b, _, opts := fixture(t)
			m := newManager(t, opts)
			c := develop(t, m, repo)
			if kind == "artifact" {
				write(t, repo, c.Artifacts[0].Path, c.Artifacts[0].Content+"\nManual edit")
			}
			if kind == "goal" {
				write(t, repo, "AGENTS.md", "A new goal")
			}
			if kind == "seed" {
				b.edit("NAC-1", func(is *issue) { is.Description += " and support accounts" })
			}
			if _, e := m.Act(context.Background(), repo, "NAC-1", Action{Action: "approve", Revision: c.Revision}); !errors.Is(e, ErrConflict) {
				t.Fatalf("stale approval accepted: %v", e)
			}
			if b.creates != 0 {
				t.Fatal("stale created children")
			}
		})
	}
	repo, _, _, opts := fixture(t)
	m := newManager(t, opts)
	c := approve(t, m, repo, develop(t, m, repo))
	write(t, repo, "AGENTS.md", "Changed goal")
	if _, e := m.Contract(context.Background(), repo, c.Sections[0].BeadID); !errors.Is(e, ErrConflict) {
		t.Fatal("stale execution accepted")
	}
}
func TestMissingWorkflowLabelsFailClosed(t *testing.T) {
	repo, b, _, opts := fixture(t)
	b.edit("NAC-1", func(is *issue) { is.Labels = []string{"specflow:missing"} })
	m := newManager(t, opts)
	env, e := m.Get(context.Background(), repo, "NAC-1")
	if e != nil || env.Available || !strings.Contains(env.Reason, "missing") {
		t.Fatalf("%+v %v", env, e)
	}
	if _, e = m.Contract(context.Background(), repo, "NAC-1"); !errors.Is(e, ErrConflict) {
		t.Fatal("orphaned label bypass")
	}
	if _, e = m.Act(context.Background(), repo, "NAC-1", Action{Action: "develop"}); !errors.Is(e, ErrConflict) {
		t.Fatal("orphaned label recreated")
	}
}
func TestLostCreatesAndCommentsRecoverWithoutDuplicates(t *testing.T) {
	repo, b, _, opts := fixture(t)
	m := newManager(t, opts)
	c := develop(t, m, repo)
	b.loseCreate = true
	c = act(t, m, repo, "NAC-1", Action{Action: "approve", Revision: c.Revision})
	if c.Phase != "interrupted" || b.creates != 1 {
		t.Fatalf("%+v", c)
	}
	b.loseComment = true
	c = act(t, m, repo, "NAC-1", Action{Action: "resume"})
	if c.Phase != "interrupted" || b.creates != 2 {
		t.Fatalf("%+v", c)
	}
	c = act(t, m, repo, "NAC-1", Action{Action: "resume"})
	if c.Phase != "ready" || b.creates != 2 {
		t.Fatalf("%+v", c)
	}
	comments, _ := b.Comments(context.Background(), repo, "NAC-1")
	if len(comments) != 2 {
		t.Fatalf("decision duplicates: %d", len(comments))
	}
	m2 := newManager(t, opts)
	env, e := m2.Get(context.Background(), repo, c.Sections[0].BeadID)
	if e != nil || env.Change.ID != c.ID {
		t.Fatalf("restart mapping: %+v %v", env, e)
	}
}
func TestManualEditPreservedAndRevisionReopensSections(t *testing.T) {
	repo, b, model, opts := fixture(t)
	m := newManager(t, opts)
	c := approve(t, m, repo, develop(t, m, repo))
	first, second := c.Sections[0], c.Sections[1]
	b.edit(first.BeadID, func(is *issue) { is.Status = "closed" })
	b.edit(c.EpicID, func(is *issue) { is.Status = "closed" })
	model.fn = func(ctx context.Context, h claude.Headless, n int) (any, float64, error) {
		if h.System == authorSystem {
			a := goodAuthor(modelContext(h))
			for i := range a.Artifacts {
				if strings.HasSuffix(a.Artifacts[i].Path, "/tasks.md") {
					a.Artifacts[i].Content = strings.ReplaceAll(a.Artifacts[i].Content, "1. Retain drafts", "1. Keep unfinished work")
				}
			}
			return a, 0.1, nil
		}
		return normalModel(ctx, h, n)
	}
	c = act(t, m, repo, "NAC-1", Action{Action: "revise", Feedback: "Rename the task section"})
	if c.Phase != "awaiting_approval" {
		t.Fatalf("%+v", c)
	}
	if c.Sections[0].ID != first.ID || c.Sections[0].BeadID != first.BeadID {
		t.Fatal("rename changed identity")
	}
	b.edit(second.BeadID, func(is *issue) { is.Description += "\nManual addition" })
	c = act(t, m, repo, "NAC-1", Action{Action: "approve", Revision: c.Revision})
	if c.Phase != "interrupted" || !strings.Contains(c.Error, "manually edited") {
		t.Fatalf("%+v", c)
	}
	b.edit(second.BeadID, func(is *issue) {
		if !strings.HasSuffix(is.Description, "Manual addition") {
			t.Error("manual edit overwritten")
		}
		is.Description = strings.TrimSuffix(is.Description, "\nManual addition")
	})
	c = act(t, m, repo, "NAC-1", Action{Action: "resume"})
	if c.Phase != "ready" {
		t.Fatalf("%+v", c)
	}
	b.edit(first.BeadID, func(is *issue) {
		if is.Status != "open" {
			t.Error("revised closed child not reopened")
		}
	})
	b.edit(c.EpicID, func(is *issue) {
		if is.Status != "open" {
			t.Error("closed epic not reopened")
		}
	})
	if b.creates != 2 {
		t.Fatal("rename duplicated children")
	}
}
func TestCoveredIsReviewedWithoutArtifactsOrPromotion(t *testing.T) {
	repo, b, model, opts := fixture(t)
	model.fn = func(ctx context.Context, h claude.Headless, n int) (any, float64, error) {
		if h.System == authorSystem {
			return authorResult{Route: "covered", Summary: "Already retained", Rationale: "Existing behavior covers the requested outcome", References: []string{"AGENTS.md"}, Artifacts: []Artifact{}, Dependencies: []sectionDependency{}, Retired: []RetiredSection{}}, 0.1, nil
		}
		return normalModel(ctx, h, n)
	}
	m := newManager(t, opts)
	c := act(t, m, repo, "NAC-1", Action{Action: "develop"})
	if c.Phase != "covered" || b.creates != 0 || len(c.Artifacts) != 0 {
		t.Fatalf("%+v", c)
	}
	if _, e := m.Act(context.Background(), repo, "NAC-1", Action{Action: "approve", Revision: c.Revision}); !errors.Is(e, ErrConflict) {
		t.Fatal("covered promoted")
	}
	if _, e := os.Stat(filepath.Join(repo, "openspec/changes")); !os.IsNotExist(e) {
		t.Fatal("covered wrote artifacts")
	}
}
func TestReviewBlocksBoundedLoopAndMalformedAssessment(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		t.Run(fmt.Sprint(malformed), func(t *testing.T) {
			repo, b, model, opts := fixture(t)
			model.fn = func(ctx context.Context, h claude.Headless, n int) (any, float64, error) {
				if h.System == reviewerSystem {
					r := goodReview(modelContext(h))
					if malformed {
						r.Assessments = r.Assessments[:4]
					} else {
						r.Findings = []Finding{{"B1", "blocker", "Conflicts with goal", "Required recovery is absent", "unresolved"}}
					}
					return r, 0.1, nil
				}
				return normalModel(ctx, h, n)
			}
			m := newManager(t, opts)
			c := act(t, m, repo, "NAC-1", Action{Action: "develop"})
			if c.Phase != "needs_attention" || b.creates != 0 {
				t.Fatalf("%+v", c)
			}
			want := 6
			if malformed {
				want = 2
			}
			if model.count() != want {
				t.Fatalf("calls=%d want=%d", model.count(), want)
			}
			if _, e := m.Act(context.Background(), repo, "NAC-1", Action{Action: "approve", Revision: c.Revision}); !errors.Is(e, ErrConflict) {
				t.Fatal("blocked review approved")
			}
		})
	}
}
func TestIndependentReviewRepairsThenRecordsDispositions(t *testing.T) {
	repo, _, model, opts := fixture(t)
	model.fn = func(ctx context.Context, h claude.Headless, n int) (any, float64, error) {
		if n == 2 {
			r := goodReview(modelContext(h))
			r.Findings = []Finding{{"B1", "high", "Missing restoration", "Add recovery verification", "unresolved"}}
			return r, 0.2, nil
		}
		return normalModel(ctx, h, n)
	}
	m := newManager(t, opts)
	c := develop(t, m, repo)
	if model.count() != 4 || len(c.Findings) != 1 || c.Findings[0].Disposition != "resolved" {
		t.Fatalf("%+v", c)
	}
}

// Strict validation problems go back to the author on the next pass; only
// after the last pass do they stop for the owner, and never reach review.
func TestValidationFailureNeverReachesReviewer(t *testing.T) {
	repo, _, model, opts := fixture(t)
	opts.Validate = func(context.Context, string, string) error { return errors.New("strict validation failed") }
	m := newManager(t, opts)
	c := act(t, m, repo, "NAC-1", Action{Action: "develop"})
	if c.Phase != "needs_attention" || model.count() != MaxPasses || !strings.Contains(c.Error, "validation") {
		t.Fatalf("%+v", c)
	}
	if len(c.Findings) != 1 || c.Findings[0].ID != fmt.Sprintf("validation-%d", MaxPasses-1) {
		t.Fatalf("validation findings pile up: %+v", c.Findings)
	}
}

// A draft that fails strict validation once is fixed by the author's next
// pass and goes on to review without the owner.
func TestValidationFailureIsFixedByTheNextPass(t *testing.T) {
	repo, _, model, opts := fixture(t)
	fails := 1
	opts.Validate = func(context.Context, string, string) error {
		if fails > 0 {
			fails--
			return errors.New("requirement text is very long")
		}
		return nil
	}
	m := newManager(t, opts)
	c := act(t, m, repo, "NAC-1", Action{Action: "develop"})
	if c.Phase != "awaiting_approval" || model.count() != 3 {
		t.Fatalf("phase=%s calls=%d error=%s", c.Phase, model.count(), c.Error)
	}
}
func TestRestartInterruptDoesNotRepeatBillableCall(t *testing.T) {
	repo, _, model, opts := fixture(t)
	entered := make(chan struct{})
	model.fn = func(ctx context.Context, _ claude.Headless, _ int) (any, float64, error) {
		close(entered)
		<-ctx.Done()
		return nil, 0, ctx.Err()
	}
	m := newManager(t, opts)
	if _, e := m.Act(context.Background(), repo, "NAC-1", Action{Action: "develop"}); e != nil {
		t.Fatal(e)
	}
	<-entered
	m.Shutdown(time.Second)
	m2 := newManager(t, opts)
	env, e := m2.Get(context.Background(), repo, "NAC-1")
	if e != nil || env.Change.Phase != "interrupted" || model.count() != 1 {
		t.Fatalf("%+v %v", env, e)
	}
	model.fn = normalModel
	c := act(t, m2, repo, "NAC-1", Action{Action: "resume"})
	if c.Phase != "awaiting_approval" || model.count() != 3 {
		t.Fatalf("%+v", c)
	}
}
func TestBudgetCannotBeBypassedByResume(t *testing.T) {
	repo, _, model, opts := fixture(t)
	model.fn = func(context.Context, claude.Headless, int) (any, float64, error) {
		return nil, 0, errors.New("provider timeout")
	}
	m := newManager(t, opts)
	c := act(t, m, repo, "NAC-1", Action{Action: "develop"})
	for i := 0; i < 5; i++ {
		c = act(t, m, repo, "NAC-1", Action{Action: "resume"})
	}
	if model.count() > MaxPasses || c.CostUSD > MaxActionCostUSD {
		t.Fatalf("calls=%d cost=%f", model.count(), c.CostUSD)
	}
}

func TestCanonicalAmendmentReusesEpicAndReviewsBothSeeds(t *testing.T) {
	repo, b, model, opts := fixture(t)
	m := newManager(t, opts)
	first := approve(t, m, repo, develop(t, m, repo))
	b.edit("NAC-2", func(is *issue) {
		*is = issue{ID: "NAC-2", Title: "Restore focus", Description: "Restore keyboard focus along with the draft", Type: "task", Status: "open"}
	})
	reviewedBoth := false
	model.fn = func(ctx context.Context, h claude.Headless, n int) (any, float64, error) {
		if h.System == authorSystem {
			a := goodAuthor(first)
			for i := range a.Artifacts {
				if strings.HasSuffix(a.Artifacts[i].Path, "/design.md") {
					a.Artifacts[i].Content += "\nRestore keyboard focus."
				}
			}
			return a, 0.1, nil
		}
		if strings.Contains(h.Prompt, "Restore keyboard focus along with the draft") && strings.Contains(h.Prompt, "Retain drafts when navigating away") {
			reviewedBoth = true
		}
		return normalModel(ctx, h, n)
	}
	second := act(t, m, repo, "NAC-2", Action{Action: "develop"})
	if second.Phase != "awaiting_approval" || second.ID != first.ID || !reviewedBoth {
		t.Fatalf("canonical review: %+v both=%v", second, reviewedBoth)
	}
	second = approve(t, m, repo, second)
	if second.EpicID != first.EpicID || b.creates != 2 {
		t.Fatal("canonical change duplicated epic/children")
	}
	m2 := newManager(t, opts)
	env, e := m2.Get(context.Background(), repo, "NAC-2")
	if e != nil || env.Change.ID != first.ID {
		t.Fatalf("alias lost after restart: %+v %v", env, e)
	}
	b.edit("NAC-2", func(is *issue) { is.Acceptance = "New constraint" })
	if _, e = m2.Contract(context.Background(), repo, second.Sections[0].BeadID); !errors.Is(e, ErrConflict) {
		t.Fatal("additional intent edit did not invalidate revision")
	}
}

func TestRetirementRecoveryAndRestorationPreserveBeadIdentity(t *testing.T) {
	repo, b, model, opts := fixture(t)
	m := newManager(t, opts)
	original := approve(t, m, repo, develop(t, m, repo))
	retired := original.Sections[1]
	model.fn = func(ctx context.Context, h claude.Headless, n int) (any, float64, error) {
		if h.System == authorSystem {
			c := modelContext(h)
			a := goodAuthor(c)
			for i := range a.Artifacts {
				if strings.HasSuffix(a.Artifacts[i].Path, "/tasks.md") {
					a.Artifacts[i].Content = strings.Split(a.Artifacts[i].Content, "<!-- nacre-section: "+retired.ID)[0]
				}
			}
			a.Retired = []RetiredSection{{ID: retired.ID, Reason: "Restoration now belongs to the retained section"}}
			return a, 0.1, nil
		}
		return normalModel(ctx, h, n)
	}
	c := act(t, m, repo, "NAC-1", Action{Action: "revise"})
	if c.Phase != "awaiting_approval" || len(c.RetiredSections) != 1 || c.RetiredSections[0].BeadID != retired.BeadID {
		t.Fatalf("retirement lost mapping: %+v", c)
	}
	c = approve(t, m, repo, c)
	if _, e := m.Contract(context.Background(), repo, retired.BeadID); !errors.Is(e, ErrConflict) {
		t.Fatal("retired section is runnable")
	}
	b.edit(retired.BeadID, func(is *issue) {
		if is.Status != "open" {
			t.Error("retired bead automatically closed")
		}
	})
	model.fn = func(ctx context.Context, h claude.Headless, n int) (any, float64, error) {
		if h.System == authorSystem {
			a := goodAuthor(modelContext(h))
			for i := range a.Artifacts {
				if strings.HasSuffix(a.Artifacts[i].Path, "/tasks.md") {
					a.Artifacts[i].Content += "\n<!-- nacre-section: " + retired.ID + " -->\n## Restored behavior\n\n- [ ] Verify restart with `go test ./drafts`\n"
				}
			}
			return a, 0.1, nil
		}
		return normalModel(ctx, h, n)
	}
	c = act(t, m, repo, "NAC-1", Action{Action: "revise"})
	if c.Phase != "awaiting_approval" {
		t.Fatalf("%+v", c)
	}
	c = approve(t, m, repo, c)
	if c.Sections[1].ID != retired.ID || c.Sections[1].BeadID != retired.BeadID || len(c.RetiredSections) != 0 || b.creates != 2 {
		t.Fatalf("restoration lost identity %+v", c)
	}
}

func TestTaskSectionsRejectDisappearingWorkDuplicateMarkersAndCycles(t *testing.T) {
	old := []Section{{ID: "sfknown001", Title: "One", BeadID: "NAC-2", Body: "- [ ] one"}}
	cases := []string{"## Renamed without marker\n- [ ] same task", "<!-- nacre-section: sfknown001 -->\n## One\n- [ ] one\n<!-- nacre-section: sfknown001 -->\n## Two\n- [ ] two", "- [ ] outside\n## One\n- [ ] one"}
	for _, body := range cases {
		if _, _, e := prepareSections(body, old, nil, nil); e == nil {
			t.Fatalf("invalid sections accepted: %s", body)
		}
	}
	body := "<!-- nacre-section: sfknown001 -->\n## Renamed\n- [ ] one\n<!-- nacre-section: sfknown002 -->\n## Other\n- [ ] two"
	deps := []sectionDependency{{"sfknown001", []string{"sfknown002"}}, {"sfknown002", []string{"sfknown001"}}}
	if _, _, e := prepareSections(body, old, nil, deps); e == nil {
		t.Fatal("cycle accepted")
	}
}

func TestArtifactSafetyAndCompareAndSwap(t *testing.T) {
	repo, _, _, _ := fixture(t)
	a := goodAuthor(Change{})
	for _, bad := range []string{"/tmp/escape.md", "../escape.md", "openspec/changes/retain-drafts/../../specs/hack/spec.md", "openspec/changes/retain-drafts/review.md", "openspec/changes/retain-drafts/specs/x/../../escape.md"} {
		copy := clone(a.Artifacts)
		copy[0].Path = bad
		if e := validateArtifactPaths(a.ChangeName, copy); e == nil {
			t.Fatalf("unsafe path accepted: %s", bad)
		}
	}
	inv, e := collect(repo)
	if e != nil {
		t.Fatal(e)
	}
	write(t, repo, a.Artifacts[0].Path, "Manual content")
	if e = writeArtifacts(repo, a.Artifacts, inv.Digests); e == nil {
		t.Fatal("concurrent create overwritten")
	}
	actual, _ := os.ReadFile(filepath.Join(repo, a.Artifacts[0].Path))
	if string(actual) != "Manual content" {
		t.Fatal("manual content changed")
	}
	outside := t.TempDir()
	link := filepath.Join(repo, "openspec/changes/linked")
	if e = os.Symlink(outside, link); e != nil {
		t.Fatal(e)
	}
	if _, e = collect(repo); e == nil {
		t.Fatal("symlink inventory accepted")
	}
	if e = writeArtifacts(repo, []Artifact{{"openspec/changes/linked/design.md", "escape"}}, map[string]string{}); e == nil {
		t.Fatal("symlink write accepted")
	}
	if _, e = os.Stat(filepath.Join(outside, "design.md")); !os.IsNotExist(e) {
		t.Fatal("write escaped root")
	}
}

func TestIncompleteInventoryDisablesDevelopment(t *testing.T) {
	for _, kind := range []string{"config", "oversize", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			repo, _, model, opts := fixture(t)
			switch kind {
			case "config":
				write(t, repo, "openspec/config.yaml", "schema: [invalid\n")
			case "oversize":
				write(t, repo, "openspec/specs/huge/spec.md", strings.Repeat("x", maxFileBytes+1))
			case "symlink":
				if e := os.MkdirAll(filepath.Join(repo, "openspec/specs"), 0o755); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(t.TempDir(), filepath.Join(repo, "openspec/specs/linked")); e != nil {
					t.Fatal(e)
				}
			}
			m := newManager(t, opts)
			env, e := m.Get(context.Background(), repo, "NAC-1")
			if e != nil || env.Available {
				t.Fatalf("%+v %v", env, e)
			}
			if _, e = m.Act(context.Background(), repo, "NAC-1", Action{Action: "develop"}); !errors.Is(e, ErrUnavailable) {
				t.Fatalf("%v", e)
			}
			if model.count() != 0 {
				t.Fatal("incomplete inventory reached model")
			}
		})
	}
}

func TestAmendmentContractIncludesRetainedBehaviorSpecs(t *testing.T) {
	repo, _, model, opts := fixture(t)
	write(t, repo, "openspec/changes/retain-drafts/specs/other/spec.md", "## ADDED Requirements\nPreserve unrelated behavior.")
	write(t, repo, "openspec/changes/retain-drafts/.openspec.yaml", "schema: spec-driven\n")
	model.fn = func(ctx context.Context, h claude.Headless, n int) (any, float64, error) {
		if h.System == authorSystem {
			a := goodAuthor(Change{})
			a.Route = "amend"
			a.References = []string{"openspec/changes/retain-drafts/specs/other/spec.md"}
			return a, 0.1, nil
		}
		return normalModel(ctx, h, n)
	}
	m := newManager(t, opts)
	c := approve(t, m, repo, develop(t, m, repo))
	contract, e := m.Contract(context.Background(), repo, c.Sections[0].BeadID)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, a := range contract.Artifacts {
		if strings.HasSuffix(a.Path, "/other/spec.md") {
			found = true
		}
	}
	if !found {
		t.Fatal("retained behavior omitted from snapshot")
	}
	if len(contract.Artifacts) != 5 {
		t.Fatalf("snapshot includes unsupported files: %+v", contract.Artifacts)
	}
}

func TestPendingPreparedWriteRecoveryRetainsMarkersAndRetirements(t *testing.T) {
	repo, _, _, opts := fixture(t)
	m := newManager(t, opts)
	c := develop(t, m, repo)
	m.mu.Lock()
	r := clone(m.records[c.ID])
	m.mu.Unlock()
	r.Stage = "apply"
	r.PendingPrepared = true
	r.PendingAuthor = &authorResult{Route: r.Change.Route, ChangeName: r.Change.ChangeName, Summary: r.Change.Summary, Rationale: r.Change.Rationale, References: r.Change.References, Artifacts: clone(r.Change.Artifacts), Retired: []RetiredSection{{ID: "alreadyremoved", Reason: "Previously handled"}}}
	before := r.Change.Sections[0].ID
	if e := m.applyAuthor(r); e != nil {
		t.Fatal(e)
	}
	if r.Change.Sections[0].ID != before {
		t.Fatal("recovery reassigned markers")
	}
}

func TestActiveImplementationRefusesRevision(t *testing.T) {
	repo, _, _, opts := fixture(t)
	active := ""
	opts.Active = func(string, string) string { return active }
	m := newManager(t, opts)
	c := approve(t, m, repo, develop(t, m, repo))
	active = "round rd123"
	if _, e := m.Act(context.Background(), repo, c.SeedID, Action{Action: "revise"}); !errors.Is(e, ErrConflict) || !strings.Contains(e.Error(), active) {
		t.Fatalf("active amendment allowed: %v", e)
	}
}

func TestStaleGetCannotOverwriteConcurrentRevision(t *testing.T) {
	repo, b, model, opts := fixture(t)
	m := newManager(t, opts)
	_ = approve(t, m, repo, develop(t, m, repo))
	write(t, repo, "AGENTS.md", "Preserve UI and restore keyboard focus.")
	getEntered, getRelease, authorEntered, authorRelease := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var blocked atomic.Bool
	b.mu.Lock()
	b.beforeGet = func() {
		if blocked.CompareAndSwap(false, true) {
			close(getEntered)
			<-getRelease
		}
	}
	b.mu.Unlock()
	got := make(chan Envelope, 1)
	go func() { env, _ := m.Get(context.Background(), repo, "NAC-1"); got <- env }()
	<-getEntered
	model.fn = func(ctx context.Context, h claude.Headless, n int) (any, float64, error) {
		if h.System == authorSystem {
			close(authorEntered)
			<-authorRelease
		}
		return normalModel(ctx, h, n)
	}
	if _, e := m.Act(context.Background(), repo, "NAC-1", Action{Action: "revise"}); e != nil {
		t.Fatal(e)
	}
	<-authorEntered
	close(getRelease)
	env := <-got
	if env.Change.Phase != "drafting" {
		t.Errorf("stale GET overwrote running operation: %+v", env.Change)
	}
	close(authorRelease)
	m.Wait()
	env, e := m.Get(context.Background(), repo, "NAC-1")
	if e != nil || env.Change.Phase != "awaiting_approval" {
		t.Fatalf("revision lost: %+v %v", env, e)
	}
}
func TestRepeatedActiveStartAndPhoneDisconnect(t *testing.T) {
	repo, _, model, opts := fixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	model.fn = func(ctx context.Context, h claude.Headless, n int) (any, float64, error) {
		if n == 1 {
			close(entered)
			<-release
		}
		return normalModel(ctx, h, n)
	}
	m := newManager(t, opts)
	ctx, cancel := context.WithCancel(context.Background())
	first, e := m.Act(ctx, repo, "NAC-1", Action{Action: "develop"})
	if e != nil {
		t.Fatal(e)
	}
	cancel()
	<-entered
	second, e := m.Act(context.Background(), repo, "NAC-1", Action{Action: "develop"})
	if e != nil || second.Change.ID != first.Change.ID || model.count() != 1 {
		t.Fatalf("duplicate operation: %+v %v", second, e)
	}
	close(release)
	m.Wait()
	env, e := m.Get(context.Background(), repo, "NAC-1")
	if e != nil || env.Change.Phase != "awaiting_approval" {
		t.Fatalf("phone disconnect stopped work %+v %v", env, e)
	}
}
func TestReviewerRetryBound(t *testing.T) {
	repo, _, model, opts := fixture(t)
	model.fn = func(ctx context.Context, h claude.Headless, n int) (any, float64, error) {
		if h.System == reviewerSystem {
			return nil, 0.1, errors.New("review transport failed")
		}
		return normalModel(ctx, h, n)
	}
	m := newManager(t, opts)
	c := act(t, m, repo, "NAC-1", Action{Action: "develop"})
	for i := 0; i < 4; i++ {
		c = act(t, m, repo, "NAC-1", Action{Action: "resume"})
	}
	if model.count() != 4 || c.Phase != "needs_attention" {
		t.Fatalf("review attempts exceeded action bound: %d %+v", model.count(), c)
	}
}

func TestReviewRequiresExplicitProjectGoalEvidence(t *testing.T) {
	repo, _, _, _ := fixture(t)
	inv, e := collect(repo)
	if e != nil {
		t.Fatal(e)
	}
	r := record{Inventory: inv, Change: Change{Findings: []Finding{}}}
	result := goodReview(r.Change)
	result.Assessments[0].References = []string{"openspec/changes/test/tasks.md"}
	r.Inventory.Digests[result.Assessments[0].References[0]] = "present"
	if e = validateReview(result, &r); e == nil || !strings.Contains(e.Error(), "explicit project goals") {
		t.Fatalf("goal alignment without goal citation passed: %v", e)
	}
}

func TestCompletedCallCheckpointResumesWithoutRepaying(t *testing.T) {
	repo, _, model, opts := fixture(t)
	repo, _ = canonicalRepo(repo)
	m := newManager(t, opts)
	inv, e := collect(repo)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := opts.Beads.Get(context.Background(), repo, "NAC-1")
	seed, _ := parseIssue(raw)
	result, _ := json.Marshal(goodAuthor(Change{}))
	r := &record{Repo: repo, Seed: seed, Change: Change{ID: newID(), SeedID: seed.ID, Phase: "drafting", History: []History{}, Sections: []Section{}}, Stage: "author", Pass: 3, ActionCost: 0.3, LastCall: &callResult{Kind: "author", Output: result}, Inventory: inv, Baseline: inv.Digests, Generated: map[string]generated{}, Planned: map[string]generated{}}
	if e = m.checkpoint(r); e != nil {
		t.Fatal(e)
	}
	m2 := newManager(t, opts)
	c := act(t, m2, repo, "NAC-1", Action{Action: "resume"})
	if c.Phase != "awaiting_approval" || model.count() != 1 {
		t.Fatalf("completed author call was repeated: %d %+v", model.count(), c)
	}
}

func TestWorkflowFilesArePrivateAndCorruptionFailsClosed(t *testing.T) {
	repo, _, _, opts := fixture(t)
	m := newManager(t, opts)
	c := develop(t, m, repo)
	for _, path := range []string{opts.Dir, filepath.Join(opts.Dir, c.ID+".json")} {
		info, e := os.Stat(path)
		if e != nil {
			t.Fatal(e)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("workflow state not private: %o", info.Mode().Perm())
		}
	}
	if e := os.WriteFile(filepath.Join(opts.Dir, c.ID+".json"), []byte("corrupt"), 0o600); e != nil {
		t.Fatal(e)
	}
	if _, e := New(opts); e == nil {
		t.Fatal("corrupt state silently skipped")
	}
}

func TestBeadsDependencyShapes(t *testing.T) {
	for _, raw := range []string{`{"id":"NAC-2","dependencies":[{"issue_id":"NAC-2","depends_on_id":"NAC-1","type":"blocks"},{"issue_id":"NAC-2","depends_on_id":"NAC-epic","type":"parent-child"}]}`, `{"id":"NAC-2","dependencies":[{"id":"NAC-1","dependency_type":"blocks","title":"Other task"},{"id":"NAC-epic","dependency_type":"parent-child","title":"Epic"}]}`} {
		is, e := parseIssue(json.RawMessage(raw))
		if e != nil || !hasDep(is, "NAC-1") || issueParent(is) != "NAC-epic" {
			t.Fatalf("unsupported Beads shape: %+v %v", is, e)
		}
	}
}

func TestDependencyWritesRecoverAndRemoveObsoleteManagedEdges(t *testing.T) {
	repo, b, model, opts := fixture(t)
	model.fn = func(ctx context.Context, h claude.Headless, n int) (any, float64, error) {
		if h.System == authorSystem {
			a := goodAuthor(modelContext(h))
			for i := range a.Artifacts {
				if strings.HasSuffix(a.Artifacts[i].Path, "/tasks.md") {
					a.Artifacts[i].Content = "<!-- nacre-section: draftfirst -->\n## Save\n- [ ] Save with test\n<!-- nacre-section: draftsecond -->\n## Restore\n- [ ] Restore with test\n"
				}
			}
			a.Dependencies = []sectionDependency{{"draftsecond", []string{"draftfirst"}}}
			return a, 0.1, nil
		}
		return normalModel(ctx, h, n)
	}
	m := newManager(t, opts)
	c := develop(t, m, repo)
	b.failDependency = true
	c = act(t, m, repo, "NAC-1", Action{Action: "approve", Revision: c.Revision})
	if c.Phase != "interrupted" {
		t.Fatalf("%+v", c)
	}
	c = act(t, m, repo, "NAC-1", Action{Action: "resume"})
	if c.Phase != "ready" {
		t.Fatalf("%+v", c)
	}
	second, e := m.getIssue(context.Background(), repo, c.Sections[1].BeadID)
	if e != nil || !hasDep(second, c.Sections[0].BeadID) {
		t.Fatal("dependency missing after recovery")
	}
	model.fn = func(ctx context.Context, h claude.Headless, n int) (any, float64, error) {
		if h.System == authorSystem {
			cur := modelContext(h)
			a := goodAuthor(cur)
			a.Dependencies = []sectionDependency{{cur.Sections[1].ID, []string{}}}
			return a, 0.1, nil
		}
		return normalModel(ctx, h, n)
	}
	c = act(t, m, repo, "NAC-1", Action{Action: "revise"})
	c = approve(t, m, repo, c)
	second, e = m.getIssue(context.Background(), repo, c.Sections[1].BeadID)
	if e != nil || hasDep(second, c.Sections[0].BeadID) {
		t.Fatal("obsolete managed dependency retained")
	}
}

func TestExecutionRejectsPostApprovalSectionDrift(t *testing.T) {
	for _, kind := range []string{"description", "design", "acceptance", "title", "parent", "labels"} {
		t.Run(kind, func(t *testing.T) {
			repo, b, _, opts := fixture(t)
			m := newManager(t, opts)
			c := approve(t, m, repo, develop(t, m, repo))
			id := c.Sections[0].BeadID
			b.edit(id, func(is *issue) {
				switch kind {
				case "description":
					is.Description += "Manual instruction"
				case "design":
					is.Design += "New design"
				case "acceptance":
					is.Acceptance = "Different result"
				case "title":
					is.Title = "Other task"
				case "parent":
					is.Parent = "OTHER"
				case "labels":
					is.Labels = nil
				}
			})
			if _, e := m.Contract(context.Background(), repo, id); !errors.Is(e, ErrConflict) || !strings.Contains(e.Error(), "changed after synchronization") {
				t.Fatalf("drift accepted: %v", e)
			}
		})
	}
	repo, b, _, opts := fixture(t)
	m := newManager(t, opts)
	c := approve(t, m, repo, develop(t, m, repo))
	id := c.Sections[0].BeadID
	b.edit(id, func(is *issue) { is.Notes = "Implementation progress note" })
	if _, e := m.Contract(context.Background(), repo, id); e != nil {
		t.Fatalf("mutable progress note invalidated contract: %v", e)
	}
}

func TestBusyRevisionDoesNotAcknowledgeIgnoredFeedback(t *testing.T) {
	repo, _, model, opts := fixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	model.fn = func(ctx context.Context, h claude.Headless, n int) (any, float64, error) {
		if n == 1 {
			close(entered)
			<-release
		}
		return normalModel(ctx, h, n)
	}
	m := newManager(t, opts)
	if _, e := m.Act(context.Background(), repo, "NAC-1", Action{Action: "develop"}); e != nil {
		t.Fatal(e)
	}
	<-entered
	if _, e := m.Act(context.Background(), repo, "NAC-1", Action{Action: "revise", Feedback: "Do not lose this feedback"}); !errors.Is(e, ErrConflict) {
		t.Errorf("feedback silently consumed: %v", e)
	}
	close(release)
	m.Wait()
}

func TestGetInvalidatesClosedEditedSectionBeforeAssessingEvidence(t *testing.T) {
	repo, b, _, opts := fixture(t)
	observedPhase := ""
	opts.Evidence = func(_ string, c Change) []Evidence {
		observedPhase = c.Phase
		return []Evidence{{Verified: c.Phase == "ready"}}
	}
	m := newManager(t, opts)
	c := approve(t, m, repo, develop(t, m, repo))
	b.edit(c.Sections[0].BeadID, func(is *issue) { is.Status = "closed"; is.Acceptance = "Manually changed after verification" })
	env, e := m.Get(context.Background(), repo, "NAC-1")
	if e != nil || env.Change.Phase != "stale" || observedPhase != "stale" || env.Change.Evidence[0].Verified {
		t.Fatalf("closed edit retained evidence: %+v %v", env, e)
	}
}
func TestDuplicateDevelopCannotConsumeNewFeedback(t *testing.T) {
	repo, _, _, opts := fixture(t)
	m := newManager(t, opts)
	_ = develop(t, m, repo)
	if _, e := m.Act(context.Background(), repo, "NAC-1", Action{Action: "develop", Feedback: "Please preserve this feedback"}); !errors.Is(e, ErrConflict) {
		t.Fatalf("no-op develop consumed feedback: %v", e)
	}
}
