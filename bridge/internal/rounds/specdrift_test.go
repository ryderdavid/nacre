package rounds

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/repoconfig"
)

func boardsIndex(string) (*api.RequirementIndex, error) {
	return &api.RequirementIndex{
		Requirements: []api.RequirementRef{{Capability: "boards (change chat)", Title: "Enter inserts a newline", Path: "openspec/changes/chat/specs/boards/spec.md", Line: 12}},
		Omitted:      []string{"huge"},
	}, nil
}

const driftPlan = `{"summary":"","beads":[],"findings":[
 {"kind":"spec-drift","beads":["T-A"],"branch":"","files":[],"text":"T-A makes Enter send.","question":"","options":[],"requirement_path":"openspec/changes/chat/specs/boards/spec.md","requirement":"Enter inserts a newline"},
 {"kind":"spec-drift","beads":["T-B"],"branch":"","files":[],"text":"T-B too.","question":"","options":[],"requirement_path":"openspec/changes/chat/specs/boards/spec.md","requirement":"Enter inserts a newline"},
 {"kind":"spec-drift","beads":["T-D"],"branch":"","files":[],"text":"Found work.","question":"","options":[],"requirement_path":"openspec/changes/chat/specs/boards/spec.md","requirement":"Enter inserts a newline"},
 {"kind":"spec-drift","beads":["T-A"],"branch":"","files":[],"text":"Uncited.","question":"","options":[],"requirement_path":"openspec/specs/nowhere/spec.md","requirement":"Made up"}]}`

func TestSpecDriftExceptionAndLeaveOut(t *testing.T) {
	beads := newFakeBeads(
		&fakeBead{ID: "T-A", Title: "Add a", Type: "task"},
		&fakeBead{ID: "T-B", Title: "Add b", Type: "task"},
		&fakeBead{ID: "T-C", Title: "Add c", Type: "task", After: []string{"T-B"}},
		&fakeBead{ID: "T-D", Title: "Add d", Type: "task", Labels: []string{"found:sf1"}},
	)
	h := newHarnessWith(t, beads, driftPlan, harnessOpts{requirements: boardsIndex})
	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-A", "T-B", "T-C", "T-D"}})
	if err != nil {
		t.Fatal(err)
	}
	asked := h.wait(t, r.ID, api.RoundNeedsDecision)
	if len(asked.Questions) != 2 {
		t.Fatalf("uncited or exempt findings became questions: %+v", asked.Questions)
	}
	for _, q := range asked.Questions {
		if q.Kind != api.FindingSpecDrift || q.Requirement == nil || len(q.Options) != 3 || q.Options[2] != AnswerLeaveOut ||
			!strings.Contains(q.Text, "openspec/changes/chat/specs/boards/spec.md, line 12") {
			t.Fatalf("question=%+v", q)
		}
	}
	if !strings.Contains(h.head.prompt, "Specified behavior") || !strings.Contains(h.head.prompt, "Never report spec drift for T-D") {
		t.Error("the review wasn't given the requirement index and the exempt bead")
	}
	if asked.Plan == nil || len(asked.Plan.SpecUnchecked) != 1 || asked.Plan.SpecUnchecked[0] != "huge" {
		t.Errorf("unchecked capabilities not stated: %+v", asked.Plan)
	}
	for _, q := range asked.Questions {
		answer := AnswerException
		if q.Beads[0] == "T-B" {
			answer = AnswerLeaveOut
		}
		if _, err := h.m.Answer(r.ID, q.ID, answer); err != nil {
			t.Fatal(err)
		}
	}
	done := h.wait(t, r.ID, api.RoundReadyToLand)
	reasons := map[string]string{}
	states := map[string]string{}
	for _, b := range done.Beads {
		states[b.ID], reasons[b.ID] = b.State, b.Reason
	}
	if states["T-A"] != api.BeadIntegrated || states["T-D"] != api.BeadIntegrated {
		t.Fatalf("states=%v", states)
	}
	if states["T-B"] != api.BeadSkipped || !strings.HasPrefix(reasons["T-B"], "Left out: spec drift against") {
		t.Fatalf("T-B=%s %q", states["T-B"], reasons["T-B"])
	}
	if states["T-C"] != api.BeadSkipped || !strings.Contains(reasons["T-C"], "T-B, which was left out") {
		t.Fatalf("T-C=%s %q", states["T-C"], reasons["T-C"])
	}
	time.Sleep(200 * time.Millisecond)
	a := beads.bead("T-A")
	if !contains(a.Labels, "spec-touch:boards") {
		t.Errorf("exception label missing: %v", a.Labels)
	}
	exception := false
	for _, c := range a.Comments {
		exception = exception || strings.Contains(c, "proceeding as an exception")
	}
	if !exception {
		t.Errorf("exception not recorded on the bead: %v", a.Comments)
	}
}

func TestSpecDriftAmendFirstReopensWhenRefused(t *testing.T) {
	beads := newFakeBeads(
		&fakeBead{ID: "T-A", Title: "Add a", Type: "task"},
		&fakeBead{ID: "T-B", Title: "Add b", Type: "task"},
	)
	plan := `{"summary":"","beads":[],"findings":[{"kind":"spec-drift","beads":["T-A"],"branch":"","files":[],"text":"T-A makes Enter send.","question":"","options":[],"requirement_path":"openspec/changes/chat/specs/boards/spec.md","requirement":"Enter inserts a newline"}]}`
	var mu sync.Mutex
	refuse := true
	var feedback string
	develop := func(_ context.Context, _, bead, fb string) error {
		mu.Lock()
		defer mu.Unlock()
		if refuse {
			return errors.New("this repository already has a proposal operation in progress")
		}
		feedback = bead + ": " + fb
		return nil
	}
	h := newHarnessWith(t, beads, plan, harnessOpts{requirements: boardsIndex, develop: develop})
	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-A", "T-B"}})
	if err != nil {
		t.Fatal(err)
	}
	asked := h.wait(t, r.ID, api.RoundNeedsDecision)
	if _, err := h.m.Answer(r.ID, asked.Questions[0].ID, AnswerAmendSpec); err != nil {
		t.Fatal(err)
	}
	var again api.Round
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		again, _, _ = h.m.Get(r.ID)
		if again.State == api.RoundNeedsDecision && again.Questions[0].Problem != "" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if q := again.Questions[0]; q.Answer != "" || !strings.Contains(q.Problem, "already has a proposal operation") {
		t.Fatalf("question not reopened with the reason: %+v", q)
	}
	for _, b := range again.Beads {
		if b.State != api.BeadWaiting {
			t.Fatalf("%s moved although the amendment couldn't start: %s", b.ID, b.State)
		}
	}
	mu.Lock()
	refuse = false
	mu.Unlock()
	if _, err := h.m.Answer(r.ID, asked.Questions[0].ID, AnswerAmendSpec); err != nil {
		t.Fatal(err)
	}
	done := h.wait(t, r.ID, api.RoundReadyToLand)
	for _, b := range done.Beads {
		if b.ID == "T-A" && (b.State != api.BeadSkipped || !strings.HasPrefix(b.Reason, "Left out to amend the spec first")) {
			t.Fatalf("T-A=%s %q", b.State, b.Reason)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.HasPrefix(feedback, "T-A: ") || !strings.Contains(feedback, `"Enter inserts a newline"`) {
		t.Fatalf("development feedback=%q", feedback)
	}
}

func TestNoSpecDriftWithoutOpenSpec(t *testing.T) {
	beads := newFakeBeads(&fakeBead{ID: "T-A", Title: "Add a", Type: "task"})
	h := newHarnessWith(t, beads, driftPlan, harnessOpts{requirements: func(string) (*api.RequirementIndex, error) {
		return nil, errors.New("openspec/config.yaml is missing")
	}})
	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-A"}})
	if err != nil {
		t.Fatal(err)
	}
	done := h.wait(t, r.ID, api.RoundReadyToLand, api.RoundNeedsDecision)
	if done.State != api.RoundReadyToLand || len(done.Questions) != 0 || strings.Contains(h.head.prompt, "Specified behavior") {
		t.Fatalf("spec drift raised without OpenSpec: %s %+v", done.State, done.Questions)
	}
}

func TestPendingAmendmentIsNotRoundWork(t *testing.T) {
	beads := newFakeBeads(&fakeBead{ID: "T-A", Title: "Add a", Type: "task", Labels: []string{"found:sf1", "found-amend"}})
	h := newHarness(t, beads, `{"summary":"","beads":[],"findings":[]}`)
	if _, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-A"}}); err == nil || !strings.Contains(err.Error(), "pending amendment") {
		t.Fatalf("a pending amendment started a round: %v", err)
	}
}

func TestSpecRoundPromptKeepsAgentsInTheirSection(t *testing.T) {
	v := beadPromptVars{Issue: issue{ID: "S-1", Title: "Section"}, Round: api.Round{SpecContracts: map[string]api.SpecContract{"S-1": {ChangeID: "sf1", SectionID: "s1"}}}, Branch: "b", Cfg: &repoconfig.Config{}}
	got := beadPrompt(v)
	if !strings.Contains(got, "Stay inside this specification section") || !strings.Contains(got, `bd comments add S-1 "Found:`) {
		t.Fatalf("spec prompt lacks the found rule:\n%s", got)
	}
	v.Round.SpecContracts = nil
	if strings.Contains(beadPrompt(v), "Stay inside this specification section") {
		t.Fatal("an ordinary round was given the spec rule")
	}
}
