package rounds

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/assist"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
	"github.com/ryderdavid/nacre/bridge/internal/gates"
	"github.com/ryderdavid/nacre/bridge/internal/gitx"
	"github.com/ryderdavid/nacre/bridge/internal/repoconfig"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
	"github.com/ryderdavid/nacre/bridge/internal/workspace"
)

// ---- fakes ----

type fakeBead struct {
	ID, Title, Type, Status string
	Labels                  []string
	After                   []string // blocks dependencies
	Comments                []string
	CloseReason             string
}

// fakeBeads is an in-memory bd.
type fakeBeads struct {
	mu    sync.Mutex
	beads map[string]*fakeBead
	deps  [][2]string
}

func newFakeBeads(bs ...*fakeBead) *fakeBeads {
	f := &fakeBeads{beads: map[string]*fakeBead{}}
	for _, b := range bs {
		if b.Status == "" {
			b.Status = "open"
		}
		f.beads[b.ID] = b
	}
	return f
}

func (f *fakeBeads) raw(b *fakeBead) api.Issue {
	deps := []map[string]string{}
	for _, a := range b.After {
		deps = append(deps, map[string]string{"issue_id": b.ID, "depends_on_id": a, "type": "blocks"})
	}
	m := map[string]any{"id": b.ID, "title": b.Title, "issue_type": b.Type, "status": b.Status, "priority": 2,
		"labels": b.Labels, "dependencies": deps, "description": "Do " + b.Title + "."}
	raw, _ := json.Marshal(m)
	return raw
}

func (f *fakeBeads) Get(_ context.Context, _, id string) (api.Issue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b := f.beads[id]
	if b == nil {
		return nil, fmt.Errorf("no issue %s", id)
	}
	return f.raw(b), nil
}

func (f *fakeBeads) List(context.Context, string) ([]api.Issue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []api.Issue
	for _, b := range f.beads {
		out = append(out, f.raw(b))
	}
	return out, nil
}

func (f *fakeBeads) Comments(_ context.Context, _, id string) ([]api.Comment, error) { return nil, nil }

func (f *fakeBeads) AddComment(_ context.Context, _, id, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b := f.beads[id]; b != nil {
		b.Comments = append(b.Comments, text)
	}
	return nil
}

func (f *fakeBeads) Update(_ context.Context, _, id string, p bd.UpdateParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	b := f.beads[id]
	if b == nil {
		return nil
	}
	b.Labels = append(b.Labels, p.AddLabels...)
	for _, l := range p.RemoveLabels {
		b.Labels = remove(b.Labels, l)
	}
	return nil
}

func (f *fakeBeads) Close(_ context.Context, _, id, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b := f.beads[id]; b != nil {
		b.Status, b.CloseReason = "closed", reason
	}
	return nil
}

func (f *fakeBeads) DepAdd(_ context.Context, _, issue, dep, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deps = append(f.deps, [2]string{issue, dep})
	return nil
}

func (f *fakeBeads) bead(id string) fakeBead {
	f.mu.Lock()
	defer f.mu.Unlock()
	return *f.beads[id]
}

// fakeHeadless answers the pre-flight review with a canned plan.
type fakeHeadless struct {
	plan   string
	prompt string
}

func (h *fakeHeadless) Run(_ context.Context, _ string, hl claude.Headless, _ func(api.RunEvent)) (assist.Outcome, error) {
	h.prompt = hl.Prompt
	return assist.Outcome{Output: json.RawMessage(h.plan), CostUSD: 0.05}, nil
}

// fakeClaude does each bead's scripted work in its worktree.
const fakeClaude = `#!/bin/sh
prompt="$2"
sess="sess-$$"
prev=""
for a in "$@"; do [ "$prev" = "--resume" ] && sess="$a"; prev="$a"; done
echo '{"type":"system","subtype":"init","session_id":"'$sess'","model":"fake","cwd":"'"$PWD"'"}'
git config user.email t@e.com; git config user.name T
case "$prompt" in
  *"Resolve the merge conflicts"*)
    printf 'A\nD\n' > conflict.txt; git add -A; git commit -q --no-edit;;
  *"nacred checked your work"*)
    git add -A; git commit -q -m "commit the rest" >/dev/null 2>&1;;
  *"You are working on bead"*)
    bead=$(printf '%s\n' "$prompt" | sed -n '1s/^You are working on bead \([^ ]*\) .*/\1/p')
    case "$bead" in
      T-A) echo a > a.txt; echo A >> shared.txt; echo A > conflict.txt; git add -A; git commit -q -m A;;
      T-B) echo B >> shared.txt; git add -A; git commit -q -m B;;
      T-C) echo c > c.txt;;
      T-D) echo D > conflict.txt; git add -A; git commit -q -m D;;
      T-E) echo BROKEN > e.txt; git add -A; git commit -q -m E;;
      T-F) echo f > f.txt; git add -A; git commit -q -m F;;
      T-S|T-R) sleep 20;;
    esac;;
  *"You stopped responding"*|*"ended with an error"*)
    echo s > s.txt; git add -A; git commit -q -m "carried on";;
esac
echo '{"type":"result","subtype":"success","is_error":false,"result":"done","session_id":"'$sess'","total_cost_usd":0.1,"num_turns":1}'
`

const testYAML = `gates:
  - name: check
    run: ./check.sh
build:
  - name: bundle
    run: cat shared.txt > bundle.out
    artifact: bundle.out
version:
  bump: ./bump.sh {level}
resources: [emulator]
land:
  target: main
  push: false
limits:
  fix_attempts: 1
`

func testRepo(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	for _, a := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "t@e.com"}, {"config", "user.name", "T"}, {"config", "commit.gpgsign", "false"}} {
		if _, err := gitx.Run(ctx, dir, a...); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"nacre.yaml": testYAML,
		"check.sh":   "#!/bin/sh\nif grep -qs BROKEN *.txt; then echo 'found BROKEN'; exit 1; fi\n",
		"bump.sh": `#!/bin/sh
old=$(cat VERSION); maj=${old%%.*}; rest=${old#*.}; min=${rest%%.*}; pat=${rest#*.}
if [ "$1" = minor ]; then min=$((min+1)); pat=0; else pat=$((pat+1)); fi
echo "$maj.$min.$pat" > VERSION; echo "$old -> $maj.$min.$pat"
`,
		"VERSION":    "0.1.0\n",
		"shared.txt": "start\n",
		".gitignore": "bundle.out\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := gitx.CommitAll(ctx, dir, "start"); err != nil {
		t.Fatal(err)
	}
	return dir
}

type harness struct {
	m     *Manager
	runs  *runs.Manager
	beads *fakeBeads
	head  *fakeHeadless
	repo  string
}

func newHarness(t *testing.T, beads *fakeBeads, plan string) *harness {
	return newHarnessWith(t, beads, plan, harnessOpts{})
}

type harnessOpts struct {
	stallAfter time.Duration
	runsDir    string
	roundsDir  string
	wsRoot     string
	repo       string
}

func newHarnessWith(t *testing.T, beads *fakeBeads, plan string, o harnessOpts) *harness {
	t.Helper()
	repo := o.repo
	if repo == "" {
		repo = testRepo(t)
	}
	for _, d := range []*string{&o.runsDir, &o.roundsDir, &o.wsRoot} {
		if *d == "" {
			*d = t.TempDir()
		}
	}
	bin := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(bin, []byte(fakeClaude), 0o755); err != nil {
		t.Fatal(err)
	}
	var m *Manager
	rm, err := runs.New(runs.Options{
		Dir: o.runsDir, ClaudePath: bin, MaxConcurrent: 3, Logger: log.New(io.Discard, "", 0), StallAfter: o.stallAfter,
		Notify: func(ev string, data any) {
			if run, ok := data.(api.Run); ok && m != nil {
				m.OnRun(run)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	head := &fakeHeadless{plan: plan}
	m, err = New(Options{
		Dir: o.roundsDir, Runs: rm, Beads: beads, Headless: head, Gates: gates.NewRunner(2),
		Workspaces: &workspace.Manager{Root: o.wsRoot},
		Repo:       func(string) (string, string, error) { return "test", repo, nil },
		Logger:     log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	m.Start()
	t.Cleanup(func() { m.Shutdown(5 * time.Second); rm.Shutdown(5 * time.Second) })
	return &harness{m: m, runs: rm, beads: beads, head: head, repo: repo}
}

func (h *harness) wait(t *testing.T, id string, states ...string) api.Round {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		r, _, _ := h.m.Get(id)
		for _, s := range states {
			if r.State == s {
				return r
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	r, evs, _ := h.m.Get(id)
	var log []string
	for _, e := range evs {
		log = append(log, fmt.Sprintf("[%s %s] %s", e.Kind, e.Bead, e.Text))
	}
	b, _ := json.MarshalIndent(r.Beads, "", " ")
	t.Fatalf("round %s is %q (%s), want %v\nbeads: %s\nlog:\n%s", id, r.State, r.PauseReason, states, b, strings.Join(log, "\n"))
	return r
}

// ---- the end-to-end round ----

// Four beads: A and B overlap (B stacks on A), C depends on A in bd and
// leaves work uncommitted the first time, D conflicts with A and needs an
// integration turn. The round ends on one verified commit and lands.
func TestRoundEndToEnd(t *testing.T) {
	beads := newFakeBeads(
		&fakeBead{ID: "T-A", Title: "Add a", Type: "feature", Labels: []string{"needs:emulator"}},
		&fakeBead{ID: "T-B", Title: "Add b", Type: "task"},
		&fakeBead{ID: "T-C", Title: "Add c", Type: "task", After: []string{"T-A"}},
		&fakeBead{ID: "T-D", Title: "Add d", Type: "bug", Labels: []string{"human"}},
	)
	plan := `{"summary":"A and B both touch shared.txt.","beads":[
	 {"id":"T-A","areas":["shared"],"files":["shared.txt"],"depends_on":[],"needs":[],"human_check":""},
	 {"id":"T-B","areas":["shared"],"files":["shared.txt"],"depends_on":[],"needs":[],"human_check":""},
	 {"id":"T-C","areas":["c"],"files":["c.txt"],"depends_on":[],"needs":[],"human_check":""},
	 {"id":"T-D","areas":["conflict"],"files":["conflict.txt"],"depends_on":[],"needs":[],"human_check":""}],
	 "findings":[{"kind":"overlap","beads":["T-B","T-A"],"branch":"","files":["shared.txt"],"text":"Both append to shared.txt.","question":"","options":[]},
	 {"kind":"overlap","beads":["T-A","T-NOT-IN-ROUND"],"branch":"","files":[],"text":"ignored","question":"","options":[]}]}`
	h := newHarness(t, beads, plan)

	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-A", "T-B", "T-C", "T-D"}, BudgetUSD: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(beads.bead("T-A").Labels, ","), "round:"+r.ID) {
		t.Error("bead not labelled with the round")
	}
	done := h.wait(t, r.ID, api.RoundReadyToLand)

	if !strings.Contains(h.head.prompt, markPreflight) || !strings.Contains(h.head.prompt, "T-C") {
		t.Errorf("pre-flight prompt: %.300s", h.head.prompt)
	}
	byID := map[string]api.RoundBead{}
	for _, b := range done.Beads {
		byID[b.ID] = b
	}
	for _, id := range []string{"T-A", "T-B", "T-C", "T-D"} {
		if byID[id].State != api.BeadIntegrated {
			t.Errorf("%s is %s (%s)", id, byID[id].State, byID[id].Reason)
		}
	}
	if !contains(byID["T-B"].After, "T-A") || !contains(byID["T-C"].After, "T-A") {
		t.Errorf("sequencing: B after %v, C after %v", byID["T-B"].After, byID["T-C"].After)
	}
	if byID["T-C"].FixAttempts != 1 {
		t.Errorf("C fix attempts = %d, want 1 (it left work uncommitted)", byID["T-C"].FixAttempts)
	}
	if !contains(byID["T-A"].Needs, "emulator") {
		t.Errorf("A needs = %v", byID["T-A"].Needs)
	}
	ctx := context.Background()
	ws := done.Workspace
	if b, _ := os.ReadFile(filepath.Join(ws, "VERSION")); strings.TrimSpace(string(b)) != "0.2.0" {
		t.Errorf("VERSION = %q, want 0.2.0 (a feature was integrated)", b)
	}
	if b, _ := os.ReadFile(filepath.Join(ws, "conflict.txt")); string(b) != "A\nD\n" {
		t.Errorf("conflict.txt = %q", b)
	}
	if len(done.Artifacts) != 1 {
		t.Errorf("artifacts = %v", done.Artifacts)
	}
	if head, _ := gitx.RevParse(ctx, ws, "HEAD"); head != done.FinalCommit {
		t.Errorf("final commit %s, head %s", done.FinalCommit, head)
	}
	// Beads close against the final commit, except the one only a person can check.
	time.Sleep(200 * time.Millisecond)
	if b := beads.bead("T-A"); b.Status != "closed" || !strings.Contains(b.CloseReason, short(done.FinalCommit)) {
		t.Errorf("T-A = %s %q", b.Status, b.CloseReason)
	}
	if b := beads.bead("T-D"); b.Status == "closed" || !strings.Contains(strings.Join(b.Comments, "\n"), "Left for you to verify") {
		t.Errorf("T-D (human) = %s, comments %v", b.Status, b.Comments)
	}
	// Some run was an integration turn for the conflict.
	sawIntegration := false
	for _, id := range done.RunIDs {
		if run, _ := h.runs.Run(id); run.Kind == "integration" {
			sawIntegration = true
		}
	}
	if !sawIntegration {
		t.Error("no integration turn ran for the A/D conflict")
	}
	if done.SpentUSD <= 0.05 {
		t.Errorf("spent = %.2f", done.SpentUSD)
	}
	if holders := h.m.leases.holders(); len(holders) != 0 {
		t.Errorf("leases still held: %v", holders)
	}

	// Land: main fast-forwards to the final commit and the worktrees go.
	if _, err := h.m.Land(r.ID); err != nil {
		t.Fatal(err)
	}
	landed := h.wait(t, r.ID, api.RoundLanded)
	if main, _ := gitx.RevParse(ctx, h.repo, "main"); main != landed.LandedCommit {
		t.Errorf("main %s, landed %s", main, landed.LandedCommit)
	}
	if _, err := os.Stat(filepath.Join(h.repo, "c.txt")); err != nil {
		t.Error("main checkout lacks the round's work")
	}
	if wts, _ := gitx.Worktrees(ctx, h.repo); len(wts) != 1 {
		t.Errorf("worktrees left after landing: %+v", wts)
	}
	if out, _ := gitx.Run(ctx, h.repo, "branch", "--list", "nacre/*"); out != "" {
		t.Errorf("round branches left: %s", out)
	}
}

// E never passes its gate and is parked; F builds on E and is skipped; the
// round still finishes with what reconciled. An unmerged branch touching a
// bead's file is asked about first.
func TestParkSkipAndUnmergedQuestion(t *testing.T) {
	beads := newFakeBeads(
		&fakeBead{ID: "T-B", Title: "Add b", Type: "bug"},
		&fakeBead{ID: "T-E", Title: "Add e", Type: "task"},
		&fakeBead{ID: "T-F", Title: "Add f", Type: "task", After: []string{"T-E"}},
	)
	plan := `{"summary":"","beads":[],"findings":[
	 {"kind":"unmerged-work","beads":["T-B"],"branch":"side","files":["shared.txt"],"text":"side edits shared.txt too.","question":"","options":[]},
	 {"kind":"unmerged-work","beads":["T-B"],"branch":"not-a-branch","files":[],"text":"made up","question":"","options":[]}]}`
	h := newHarness(t, beads, plan)
	ctx := context.Background()
	// An unmerged branch that changes shared.txt.
	wt := filepath.Join(t.TempDir(), "side")
	if err := gitx.AddWorktree(ctx, h.repo, wt, "side", "main"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(wt, "side.txt"), []byte("side\n"), 0o644)
	gitx.CommitAll(ctx, wt, "side work")

	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-B", "T-E", "T-F"}})
	if err != nil {
		t.Fatal(err)
	}
	asked := h.wait(t, r.ID, api.RoundNeedsDecision)
	if len(asked.Questions) != 1 || asked.Questions[0].Branch != "side" || asked.Questions[0].Options[0] != AnswerMergeFirst {
		t.Fatalf("questions = %+v", asked.Questions)
	}
	if !strings.Contains(h.head.prompt, "side") {
		t.Error("the review wasn't told about the unmerged branch")
	}
	for _, b := range asked.Beads {
		if b.State != api.BeadWaiting {
			t.Errorf("%s started before the question was answered", b.ID)
		}
	}
	if _, err := h.m.Answer(r.ID, asked.Questions[0].ID, AnswerMergeFirst); err != nil {
		t.Fatal(err)
	}
	done := h.wait(t, r.ID, api.RoundReadyToLand)
	states := map[string]string{}
	for _, b := range done.Beads {
		states[b.ID] = b.State
	}
	if states["T-B"] != api.BeadIntegrated || states["T-E"] != api.BeadParked || states["T-F"] != api.BeadSkipped {
		t.Fatalf("states = %v", states)
	}
	if _, err := os.Stat(filepath.Join(done.Workspace, "side.txt")); err != nil {
		t.Error("the side branch wasn't merged in first")
	}
	if b, _ := os.ReadFile(filepath.Join(done.Workspace, "VERSION")); strings.TrimSpace(string(b)) != "0.1.1" {
		t.Errorf("VERSION = %q, want 0.1.1 (only a bug was integrated)", b)
	}
	time.Sleep(200 * time.Millisecond)
	if e := beads.bead("T-E"); e.Status == "closed" || !contains(e.Labels, "round-parked") {
		t.Errorf("T-E = %s %v", e.Status, e.Labels)
	}
	// The parked bead's branch survives landing.
	if _, err := h.m.Land(r.ID); err != nil {
		t.Fatal(err)
	}
	h.wait(t, r.ID, api.RoundLanded)
	if !gitx.BranchExists(ctx, h.repo, workspace.BeadBranch(r.ID, "T-E")) {
		t.Error("the parked bead's branch was deleted")
	}
}

// A merge Ryder asked for that can't be done hands the question back, rather
// than pausing into the same failure on every resume.
func TestFailedPreflightMergeAsksAgain(t *testing.T) {
	beads := newFakeBeads(&fakeBead{ID: "T-B", Title: "Add b", Type: "bug"})
	plan := `{"summary":"","beads":[],"findings":[
	 {"kind":"unmerged-work","beads":["T-B"],"branch":"side","files":["clash.txt"],"text":"side edits clash.txt.","question":"","options":[]}]}`
	h := newHarness(t, beads, plan)
	ctx := context.Background()
	wt := filepath.Join(t.TempDir(), "side")
	if err := gitx.AddWorktree(ctx, h.repo, wt, "side", "main"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(wt, "clash.txt"), []byte("side\n"), 0o644)
	gitx.CommitAll(ctx, wt, "side work")
	os.WriteFile(filepath.Join(h.repo, "clash.txt"), []byte("main\n"), 0o644)
	gitx.CommitAll(ctx, h.repo, "main work")

	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-B"}})
	if err != nil {
		t.Fatal(err)
	}
	asked := h.wait(t, r.ID, api.RoundNeedsDecision)
	if _, err := h.m.Answer(r.ID, asked.Questions[0].ID, AnswerMergeFirst); err != nil {
		t.Fatal(err)
	}
	var again api.Round
	deadline := time.Now().Add(10 * time.Second)
	for {
		again, _, _ = h.m.Get(r.ID)
		if again.State == api.RoundNeedsDecision && again.Questions[0].Problem != "" {
			break
		}
		if again.State == api.RoundPaused || time.Now().After(deadline) {
			t.Fatalf("round = %s %q, want the question asked again", again.State, again.PauseReason)
		}
		time.Sleep(20 * time.Millisecond)
	}
	q := again.Questions[0]
	if q.Answer != "" || !strings.Contains(q.Problem, "clash.txt") {
		t.Fatalf("question = %+v", q)
	}
	for _, b := range again.Beads {
		if b.State != api.BeadWaiting {
			t.Errorf("%s started before the question was settled", b.ID)
		}
	}
	if _, err := h.m.Answer(r.ID, q.ID, AnswerIgnore); err != nil {
		t.Fatal(err)
	}
	done := h.wait(t, r.ID, api.RoundReadyToLand)
	if done.Questions[0].Problem != "" {
		t.Error("the problem outlived the new answer")
	}
	if b, _ := os.ReadFile(filepath.Join(done.Workspace, "clash.txt")); strings.TrimSpace(string(b)) != "main" {
		t.Errorf("clash.txt = %q, want main's", b)
	}
}

func TestCreateRefusals(t *testing.T) {
	beads := newFakeBeads(&fakeBead{ID: "T-A", Title: "a", Type: "task"}, &fakeBead{ID: "T-X", Title: "x", Type: "task", Status: "closed"})
	h := newHarness(t, beads, `{"summary":"","beads":[],"findings":[]}`)
	if _, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-X"}}); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Errorf("closed bead: %v", err)
	}
	if _, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-nope"}}); err == nil {
		t.Error("missing bead accepted")
	}
	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-A"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-A"}}); err == nil || !strings.Contains(err.Error(), r.ID) {
		t.Errorf("bead in another round: %v", err)
	}
	os.Remove(filepath.Join(h.repo, "nacre.yaml"))
	if _, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-A"}}); err == nil || !strings.Contains(err.Error(), "nacre.yaml") {
		t.Errorf("no nacre.yaml: %v", err)
	}
	if _, err := h.m.Cancel(r.ID); err != nil {
		t.Fatal(err)
	}
	h.wait(t, r.ID, api.RoundCancelled)
}

const emptyPlan = `{"summary":"","beads":[],"findings":[]}`

// A run that goes quiet is stopped and resumed once without asking anyone.
func TestStalledRunIsResumedOnce(t *testing.T) {
	beads := newFakeBeads(&fakeBead{ID: "T-S", Title: "Slow", Type: "task"})
	h := newHarnessWith(t, beads, emptyPlan, harnessOpts{stallAfter: 300 * time.Millisecond})
	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-S"}})
	if err != nil {
		t.Fatal(err)
	}
	done := h.wait(t, r.ID, api.RoundReadyToLand)
	if b := done.Beads[0]; b.State != api.BeadIntegrated || b.Stalls != 1 {
		t.Errorf("bead = %+v", b)
	}
}

// Hitting the budget pauses the round with its state intact; raising it
// carries on.
func TestBudgetPausesAndResumes(t *testing.T) {
	beads := newFakeBeads(&fakeBead{ID: "T-A", Title: "a", Type: "task"}, &fakeBead{ID: "T-B", Title: "b", Type: "task"})
	plan := `{"summary":"","beads":[],"findings":[{"kind":"overlap","beads":["T-A","T-B"],"branch":"","files":[],"text":"x","question":"","options":[]}]}`
	h := newHarness(t, beads, plan)
	// The plan costs 0.05 and each turn 0.10: one turn fits, two don't.
	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-A", "T-B"}, BudgetUSD: 0.42})
	if err != nil {
		t.Fatal(err)
	}
	paused := h.wait(t, r.ID, api.RoundPaused)
	if !strings.Contains(paused.PauseReason, "budget") {
		t.Errorf("pause reason = %q", paused.PauseReason)
	}
	if _, err := h.m.SetLimits(r.ID, 5, 0); err != nil {
		t.Fatal(err)
	}
	done := h.wait(t, r.ID, api.RoundReadyToLand)
	for _, b := range done.Beads {
		if b.State != api.BeadIntegrated {
			t.Errorf("%s = %s", b.ID, b.State)
		}
	}
}

// nacred restarting mid-round: the round reloads, the interrupted run is
// followed up in its session, and the round finishes.
func TestRoundSurvivesRestart(t *testing.T) {
	beads := newFakeBeads(&fakeBead{ID: "T-R", Title: "Restart", Type: "task"})
	o := harnessOpts{runsDir: t.TempDir(), roundsDir: t.TempDir(), wsRoot: t.TempDir()}
	h := newHarnessWith(t, beads, emptyPlan, o)
	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-R"}})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if got, _, _ := h.m.Get(r.ID); got.Beads[0].State == api.BeadRunning {
			if run, _ := h.runs.Run(got.Beads[0].RunID); run.Status == api.StatusRunning && run.SessionID != "" {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.m.Shutdown(5 * time.Second)
	h.runs.Shutdown(5 * time.Second)

	o.repo = h.repo
	h2 := newHarnessWith(t, beads, emptyPlan, o)
	done := h2.wait(t, r.ID, api.RoundReadyToLand)
	if b := done.Beads[0]; b.State != api.BeadIntegrated || b.FixAttempts != 1 {
		t.Errorf("after restart: %+v", b)
	}
	if run, _ := h2.runs.Run(done.Beads[0].RunID); run.Turns != 2 {
		t.Errorf("the interrupted run wasn't followed up in its session: %d turns", run.Turns)
	}
	_, evs, _ := h2.m.Get(r.ID)
	if len(evs) < 5 || evs[0].Seq != 1 {
		t.Errorf("events not kept across the restart: %d", len(evs))
	}
}

func TestBeadPromptContract(t *testing.T) {
	cfg, err := repoconfigParse(testYAML)
	if err != nil {
		t.Fatal(err)
	}
	p := beadPrompt(beadPromptVars{
		Round:  api.Round{ID: "rd1", RepoName: "Nacre"},
		Issue:  issue{ID: "NAC-2", Title: "Hide the bar", Type: "feature", Priority: 1, Description: "Hide it on scroll.", AcceptanceCriteria: "It hides."},
		Branch: "nacre/r/rd1/NAC-2", Base: "abcdef1234", Cfg: cfg,
		Decisions:  []string{"Hide only when folded? Answer: yes"},
		Shared:     []string{"the version number (the round bumps it once at the end)"},
		Integrated: []string{"NAC-1 (Chat bar)"},
		Env:        []string{"NACRE_RESOURCE_EMULATOR=emulator-5554"},
	})
	for _, want := range []string{
		"You are working on bead NAC-2", "Hide it on scroll.", "It hides.",
		"Hide only when folded? Answer: yes", "nacre/r/rd1/NAC-2", "abcdef1",
		"`./check.sh`", "nacred gates --skip-final", "Don't merge", "Don't close NAC-2",
		"Don't change the version number", "emulator-5554", "NAC-1 (Chat bar)", "don't fake it",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q:\n%s", want, p)
		}
	}
}

func TestLeases(t *testing.T) {
	cfg, _ := repoconfigParse(testYAML + "")
	l := newLeases()
	env, ok := l.acquire("r/A", []string{"emulator"}, cfg)
	if !ok || len(env) != 1 || env[0] != "NACRE_RESOURCE_EMULATOR=emulator" {
		t.Fatalf("A: %v %v", env, ok)
	}
	if _, ok := l.acquire("r/B", []string{"emulator"}, cfg); ok {
		t.Error("B got a resource A holds")
	}
	if _, ok := l.acquire("r/A", []string{"emulator"}, cfg); !ok {
		t.Error("A couldn't re-acquire its own lease")
	}
	if _, ok := l.acquire("r/C", nil, cfg); !ok {
		t.Error("a bead needing nothing had to wait")
	}
	l.release("r/A")
	if _, ok := l.acquire("r/B", []string{"emulator"}, cfg); !ok {
		t.Error("B couldn't take the released resource")
	}
}

func repoconfigParse(s string) (*repoconfig.Config, error) { return repoconfig.Parse([]byte(s)) }

// While a question holds the round, Ryder can unlink an overlap and accept an
// ordering as a bd dependency.
func TestEditPlanBeforeStart(t *testing.T) {
	beads := newFakeBeads(&fakeBead{ID: "T-A", Title: "a", Type: "task"}, &fakeBead{ID: "T-B", Title: "b", Type: "task"})
	plan := `{"summary":"","beads":[],"findings":[
	 {"kind":"overlap","beads":["T-A","T-B"],"branch":"","files":["shared.txt"],"text":"both","question":"","options":[]},
	 {"kind":"ambiguity","beads":["T-A"],"branch":"","files":[],"text":"A is vague","question":"How big is a?","options":["Small","Large"]}]}`
	h := newHarness(t, beads, plan)
	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-A", "T-B"}})
	if err != nil {
		t.Fatal(err)
	}
	asked := h.wait(t, r.ID, api.RoundNeedsDecision)
	if q := asked.Questions[0]; q.Text != "How big is a?" || len(q.Options) != 2 {
		t.Fatalf("question = %+v", q)
	}
	if _, err := h.m.EditPlan(r.ID, PlanEdit{Unlink: [][2]string{{"T-B", "T-A"}}, AcceptDependencies: [][2]string{{"T-B", "T-A"}}}); err != nil {
		t.Fatal(err)
	}
	got, _, _ := h.m.Get(r.ID)
	if len(got.Beads[1].After) != 0 {
		t.Errorf("T-B still waits for %v", got.Beads[1].After)
	}
	if _, err := h.m.Answer(r.ID, "q2", "Small"); err != nil {
		t.Fatal(err)
	}
	h.wait(t, r.ID, api.RoundReadyToLand)
	time.Sleep(100 * time.Millisecond)
	beads.mu.Lock()
	deps := beads.deps
	beads.mu.Unlock()
	if len(deps) != 1 || deps[0] != [2]string{"T-B", "T-A"} {
		t.Errorf("bd deps = %v", deps)
	}
	if a := beads.bead("T-A"); !strings.Contains(strings.Join(a.Comments, "\n"), "Answer: Small") {
		t.Errorf("decision not recorded on the bead: %v", a.Comments)
	}
}
