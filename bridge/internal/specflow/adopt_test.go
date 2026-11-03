package specflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

const adoptTasks = `<!-- nacre-section: rd-save -->
## 1. Save drafts

- [x] Save on Back
- [x] Save on rotation

## 2. Name drafts

- [x] Title from the first line

## 3. Restore drafts

- [ ] Restore after restart; run ` + "`go test ./drafts`" + `
`

// handWritten writes an active change as a CLI session would, with a review.md.
func handWritten(t *testing.T, repo string) {
	t.Helper()
	p := "openspec/changes/retain-drafts/"
	write(t, repo, p+"proposal.md", "# Retain drafts\n\n## Why\nDrafts vanish when people navigate away.\n\n## What Changes\nKeep them.\n")
	write(t, repo, p+"design.md", "Keep the existing screen. Save drafts locally.\n")
	write(t, repo, p+"tasks.md", adoptTasks)
	write(t, repo, p+"specs/drafts/spec.md", "## ADDED Requirements\n### Requirement: Retain drafts\nThe app SHALL retain drafts.\n#### Scenario: Back\n- WHEN Back is pressed\n- THEN the draft is retained\n")
	write(t, repo, p+"review.md", "Reviewed three times; no open findings.\n")
}

func adoptFixture(t *testing.T) (string, *fakeBeads, *fakeModel, *Manager) {
	repo, b, model, opts := fixture(t)
	handWritten(t, repo)
	b.issues["NAC-2"] = issue{ID: "NAC-2", Title: "retain-drafts 1. Save drafts", Description: "Notes from the CLI session", Parent: "NAC-1", Status: "closed", Type: "task"}
	b.issues["NAC-3"] = issue{ID: "NAC-3", Title: "Unrelated child", Parent: "NAC-1", Status: "open", Type: "task"}
	return repo, b, model, newManager(t, opts)
}

func readRepo(t *testing.T, repo, p string) string {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(repo, p))
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}

func TestAdoptOffersUntrackedChanges(t *testing.T) {
	repo, _, _, m := adoptFixture(t)
	env, e := m.Get(context.Background(), repo, "NAC-1")
	if e != nil || len(env.Adoptable) != 1 {
		t.Fatalf("adoptable=%+v err=%v", env.Adoptable, e)
	}
	a := env.Adoptable[0]
	if a.ChangeName != "retain-drafts" || !a.Suggested || !a.HasReview || a.TasksDone != 3 || a.TasksTotal != 4 || a.Problem != "" || !strings.Contains(a.Why, "Drafts vanish") {
		t.Fatalf("adoptable = %+v", a)
	}
	act(t, m, repo, "NAC-1", Action{Action: "adopt", ChangeName: "retain-drafts", RecordedReview: true})
	b2 := "NAC-9"
	m.opts.Beads.(*fakeBeads).issues[b2] = issue{ID: b2, Title: "Something else", Status: "open", Type: "task"}
	if env, _ = m.Get(context.Background(), repo, b2); len(env.Adoptable) != 0 {
		t.Fatalf("a tracked change is still offered: %+v", env.Adoptable)
	}
}

func TestAdoptReviewsAsWrittenLinksBeadsAndKeepsFinishedWorkClosed(t *testing.T) {
	repo, b, model, m := adoptFixture(t)
	proposal := readRepo(t, repo, "openspec/changes/retain-drafts/proposal.md")
	c := act(t, m, repo, "NAC-1", Action{Action: "adopt", ChangeName: "retain-drafts"})
	if c.Phase != "awaiting_approval" || c.Route != adoptedRoute || c.ChangeName != "retain-drafts" {
		t.Fatalf("phase=%s route=%s error=%s", c.Phase, c.Route, c.Error)
	}
	if model.count() != 1 || model.calls[0].System != reviewerSystem {
		t.Fatalf("adoption must run exactly one independent review, got %d calls", model.count())
	}
	if b := model.calls[0].MaxBudgetUSD; b != MaxActionCostUSD {
		t.Fatalf("adoption review budget = %v, want %v", b, MaxActionCostUSD)
	}
	if len(c.Sections) != 3 || c.Sections[0].ID != "rd-save" || c.Sections[0].BeadID != "NAC-2" || c.Sections[1].BeadID != "" {
		t.Fatalf("sections = %+v", c.Sections)
	}
	if got := c.Adoption; got == nil || len(got.Completed) != 2 || len(got.Linked) != 1 {
		t.Fatalf("adoption = %+v", got)
	}
	if readRepo(t, repo, "openspec/changes/retain-drafts/tasks.md") != adoptTasks {
		t.Fatal("adoption wrote to the checkout's tasks.md")
	}
	for _, a := range c.Artifacts {
		if strings.HasSuffix(a.Path, "/tasks.md") && (strings.Count(a.Content, "nacre-section:") != 3 || !strings.Contains(a.Content, "nacre-section: rd-save")) {
			t.Fatalf("the workflow's tasks.md lacks durable markers:\n%s", a.Content)
		}
	}
	if readRepo(t, repo, "openspec/changes/retain-drafts/proposal.md") != proposal {
		t.Fatal("adoption rewrote the proposal")
	}
	if b.creates != 0 {
		t.Fatal("adoption created beads before approval")
	}

	c = approve(t, m, repo, c)
	if b.creates != 2 {
		t.Fatalf("created %d beads; only unlinked sections get new ones", b.creates)
	}
	linked := b.issues["NAC-2"]
	if linked.Status != "closed" || !contains(linked.Labels, workflowLabel(&record{Change: c})) || !strings.Contains(linked.Description, "Specification contract") {
		t.Fatalf("linked bead = %+v", linked)
	}
	comments, _ := b.Comments(context.Background(), repo, "NAC-2")
	kept := false
	for _, cm := range comments {
		kept = kept || (strings.Contains(cm.Text, "[nacre-spec-adopted:") && strings.Contains(cm.Text, "Notes from the CLI session"))
	}
	if !kept {
		t.Fatal("the linked bead's own text was not preserved")
	}
	if s := b.issues[c.Sections[1].BeadID]; s.Status != "closed" {
		t.Fatalf("finished section %s is %s", s.ID, s.Status)
	}
	if s := b.issues[c.Sections[2].BeadID]; s.Status != "open" {
		t.Fatalf("unfinished section %s is %s", s.ID, s.Status)
	}
	if b.issues["NAC-3"].Labels != nil || b.issues["NAC-1"].Type != "epic" {
		t.Fatal("unrelated child changed or the seed did not become the epic")
	}
	if !c.CompletedBeforeAdoption(c.Sections[0]) || c.CompletedBeforeAdoption(c.Sections[2]) {
		t.Fatal("completion before adoption misreported")
	}
	if _, e := m.Contract(context.Background(), repo, c.Sections[2].BeadID); e != nil {
		t.Fatalf("unfinished section cannot run: %v", e)
	}

	// Approving again must not reopen finished work.
	c = approve(t, m, repo, c)
	if b.issues["NAC-2"].Status != "closed" || b.issues[c.Sections[1].BeadID].Status != "closed" {
		t.Fatal("a repeated approval reopened finished sections")
	}
}

func TestAdoptWithRecordedReviewSkipsTheReviewer(t *testing.T) {
	repo, b, model, m := adoptFixture(t)
	c := act(t, m, repo, "NAC-1", Action{Action: "adopt", ChangeName: "retain-drafts", RecordedReview: true})
	if c.Phase != "awaiting_approval" || model.count() != 0 {
		t.Fatalf("phase=%s calls=%d error=%s", c.Phase, model.count(), c.Error)
	}
	c = approve(t, m, repo, c)
	comments, _ := b.Comments(context.Background(), repo, "NAC-1")
	recorded := false
	for _, cm := range comments {
		recorded = recorded || strings.Contains(cm.Text, "Recorded review")
	}
	if !recorded {
		t.Fatal("decision comment doesn't say a recorded review was used")
	}

	// A revision is judged by Nacre's own reviewer.
	c = act(t, m, repo, "NAC-1", Action{Action: "revise", Feedback: "Also keep drafts across devices"})
	if c.Adoption == nil || c.Adoption.RecordedReview || model.count() != 2 {
		t.Fatalf("revision: adoption=%+v calls=%d phase=%s error=%s", c.Adoption, model.count(), c.Phase, c.Error)
	}
}

func TestAdoptStopsInsteadOfRedraftingOnBlockingReview(t *testing.T) {
	repo, _, model, m := adoptFixture(t)
	model.fn = func(_ context.Context, h claude.Headless, _ int) (any, float64, error) {
		r := goodReview(modelContext(h))
		r.Findings = append(r.Findings, Finding{ID: "F1", Severity: "blocker", Summary: "No failure path", Detail: "Nothing says what happens when saving fails.", Disposition: "unresolved"})
		return r, 0.1, nil
	}
	c := act(t, m, repo, "NAC-1", Action{Action: "adopt", ChangeName: "retain-drafts"})
	if c.Phase != "needs_attention" || model.count() != 1 || len(c.Findings) != 1 || !strings.Contains(c.Error, "as written") {
		t.Fatalf("phase=%s calls=%d findings=%d error=%s", c.Phase, model.count(), len(c.Findings), c.Error)
	}

	// Ryder fixes the files himself; resuming reviews them again, no author.
	write(t, repo, "openspec/changes/retain-drafts/design.md", "Keep the existing screen. Save drafts locally; when saving fails, keep the draft in memory and say so.\n")
	model.fn = nil
	c = act(t, m, repo, "NAC-1", Action{Action: "resume"})
	if c.Phase != "awaiting_approval" || model.count() != 2 || model.calls[1].System != reviewerSystem {
		t.Fatalf("after fixing by hand: phase=%s calls=%d error=%s", c.Phase, model.count(), c.Error)
	}
	if !strings.Contains(readRepo(t, repo, "openspec/changes/retain-drafts/design.md"), "keep the draft in memory") {
		t.Fatal("the hand-fixed file was rewritten")
	}
}

func TestAdoptRefusals(t *testing.T) {
	repo, _, _, m := adoptFixture(t)
	ctx := context.Background()
	if _, e := m.Act(ctx, repo, "NAC-1", Action{Action: "adopt", ChangeName: "no-such-change"}); !errors.Is(e, ErrConflict) {
		t.Fatalf("missing change: %v", e)
	}
	os.Remove(filepath.Join(repo, "openspec/changes/retain-drafts/review.md"))
	if _, e := m.Act(ctx, repo, "NAC-1", Action{Action: "adopt", ChangeName: "retain-drafts", RecordedReview: true}); !errors.Is(e, ErrConflict) {
		t.Fatalf("recorded review without review.md: %v", e)
	}
	act(t, m, repo, "NAC-1", Action{Action: "adopt", ChangeName: "retain-drafts"})
	if _, e := m.Act(ctx, repo, "NAC-1", Action{Action: "adopt", ChangeName: "retain-drafts"}); !errors.Is(e, ErrConflict) {
		t.Fatalf("adopting twice through one bead: %v", e)
	}
	m.opts.Beads.(*fakeBeads).issues["NAC-9"] = issue{ID: "NAC-9", Title: "Another", Status: "open", Type: "task"}
	if _, e := m.Act(ctx, repo, "NAC-9", Action{Action: "adopt", ChangeName: "retain-drafts"}); !errors.Is(e, ErrConflict) {
		t.Fatalf("adopting a tracked change: %v", e)
	}
	write(t, repo, "openspec/changes/half-done/proposal.md", "## Why\nx\n")
	if _, e := m.Act(ctx, repo, "NAC-9", Action{Action: "adopt", ChangeName: "half-done"}); !errors.Is(e, ErrConflict) {
		t.Fatalf("incomplete change: %v", e)
	}
}

// Resuming an adoption that stopped before its review takes the files again,
// so specifications that moved meanwhile don't block it.
func TestResumedAdoptionRetakesTheFiles(t *testing.T) {
	repo, _, model, m := adoptFixture(t)
	calls := 0
	model.fn = func(ctx context.Context, h claude.Headless, n int) (any, float64, error) {
		calls++
		if calls == 1 {
			return nil, 0.85, errors.New("Reached maximum budget")
		}
		return normalModel(ctx, h, n)
	}
	c := act(t, m, repo, "NAC-1", Action{Action: "adopt", ChangeName: "retain-drafts"})
	if c.Phase != "needs_attention" {
		t.Fatalf("phase = %s", c.Phase)
	}
	write(t, repo, "openspec/changes/other-work/proposal.md", "## Why\nSomething else.\n")
	c = act(t, m, repo, "NAC-1", Action{Action: "resume"})
	if c.Phase != "awaiting_approval" || c.Error != "" {
		t.Fatalf("resumed adoption: phase=%s error=%s", c.Phase, c.Error)
	}
}

// An adopted change awaiting approval whose files were edited is reviewed
// again on Resume, without the author.
func TestEditedAdoptionAwaitingApprovalIsReviewedAgain(t *testing.T) {
	repo, _, model, m := adoptFixture(t)
	c := act(t, m, repo, "NAC-1", Action{Action: "adopt", ChangeName: "retain-drafts"})
	if c.Phase != "awaiting_approval" {
		t.Fatalf("phase = %s", c.Phase)
	}
	write(t, repo, "openspec/changes/retain-drafts/design.md", "Keep the existing screen; when saving fails, say so.\n")
	c = act(t, m, repo, "NAC-1", Action{Action: "resume"})
	if c.Phase != "awaiting_approval" || model.count() != 2 || model.calls[1].System != reviewerSystem {
		t.Fatalf("phase=%s calls=%d error=%s", c.Phase, model.count(), c.Error)
	}
}
