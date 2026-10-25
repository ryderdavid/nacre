package rounds

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/gitx"
	"github.com/ryderdavid/nacre/bridge/internal/workspace"
)

const noFindings = `{"summary":"","beads":[],"findings":[]}`

// commitOnMain writes files on the test repository's main branch.
func commitOnMain(t *testing.T, repo string, files map[string]string, msg string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(repo, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := gitx.CommitAll(context.Background(), repo, msg); err != nil {
		t.Fatal(err)
	}
}

// waitFor polls the round until ok says so, or fails with the log.
func (h *harness) waitFor(t *testing.T, id string, what string, ok func(api.Round) bool) api.Round {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		r, _, _ := h.m.Get(id)
		if ok(r) {
			return r
		}
		time.Sleep(50 * time.Millisecond)
	}
	h.wait(t, id, "never: "+what)
	return api.Round{}
}

func hasEvent(evs []api.RoundEvent, kind, text string) bool {
	for _, e := range evs {
		if e.Kind == kind && strings.Contains(e.Text, text) {
			return true
		}
	}
	return false
}

func choiceIDs(s *api.Stop) []string {
	var ids []string
	for _, c := range s.Choices {
		ids = append(ids, c.ID)
	}
	return ids
}

func TestTransientRetry(t *testing.T) {
	calls := 0
	n, err := retry(context.Background(), func() error {
		calls++
		if calls == 1 {
			return errors.New("fatal: Unable to create '/r/.git/worktrees/x/index.lock': File exists.")
		}
		return nil
	})
	if err != nil || n != 2 {
		t.Errorf("locked index: n=%d err=%v, want success on try 2", n, err)
	}
	calls = 0
	n, err = retry(context.Background(), func() error { calls++; return errors.New("fatal: invalid reference: nacre/r/x/bad..name") })
	if err == nil || n != 1 || calls != 1 {
		t.Errorf("real failure: n=%d calls=%d err=%v, want one try", n, calls, err)
	}
	n, _ = retry(context.Background(), func() error { return errors.New("read: connection reset by peer") })
	if n != len(retryBackoff)+1 {
		t.Errorf("persistent transient error tried %d times, want %d", n, len(retryBackoff)+1)
	}
	for msg, want := range map[string]bool{
		"database is locked":            true,
		"dial tcp: i/o timeout":         true,
		"exit status 1: merge conflict": false,
		"bd: no issue NAC-9":            false,
	} {
		if transient(errors.New(msg)) != want {
			t.Errorf("transient(%q) = %v", msg, !want)
		}
	}
}

// Two beads both append to a beads log: the second merge conflicts only
// there, and the union settles it without an integration turn.
func TestBeadsLogConflictSettledWithoutClaude(t *testing.T) {
	beads := newFakeBeads(&fakeBead{ID: "T-J1", Title: "j1", Type: "task"}, &fakeBead{ID: "T-J2", Title: "j2", Type: "task"})
	h := newHarness(t, beads, noFindings)
	commitOnMain(t, h.repo, map[string]string{".beads/log.jsonl": "{\"start\":1}\n"}, "beads log")
	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-J1", "T-J2"}})
	if err != nil {
		t.Fatal(err)
	}
	done := h.wait(t, r.ID, api.RoundReadyToLand)
	_, evs, _ := h.m.Get(r.ID)
	if !hasEvent(evs, "recover", ".beads/log.jsonl (kept both sides' lines)") {
		t.Error("no recover event for the beads log")
	}
	if hasEvent(evs, "conflict", "Sent to Claude") {
		t.Error("a beads log conflict went to Claude")
	}
	b, _ := os.ReadFile(filepath.Join(done.Workspace, ".beads/log.jsonl"))
	for _, line := range []string{`{"start":1}`, `{"j":1}`, `{"j":2}`} {
		if strings.Count(string(b), line) != 1 {
			t.Errorf("log has %q %d times:\n%s", line, strings.Count(string(b), line), b)
		}
	}
}

// A file declared in merge.regenerate is rebuilt, not merged.
func TestRegeneratedFileRebuiltOnConflict(t *testing.T) {
	beads := newFakeBeads(&fakeBead{ID: "T-L1", Title: "l1", Type: "task"}, &fakeBead{ID: "T-L2", Title: "l2", Type: "task"})
	h := newHarness(t, beads, noFindings)
	commitOnMain(t, h.repo, map[string]string{
		"gen.lock":   "l0\n",
		"nacre.yaml": testYAML + "merge:\n  regenerate:\n    - path: gen.lock\n      run: echo regenerated > gen.lock\n",
	}, "lockfile")
	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-L1", "T-L2"}})
	if err != nil {
		t.Fatal(err)
	}
	done := h.wait(t, r.ID, api.RoundReadyToLand)
	if b, _ := os.ReadFile(filepath.Join(done.Workspace, "gen.lock")); strings.TrimSpace(string(b)) != "regenerated" {
		t.Errorf("gen.lock = %q, want it regenerated", b)
	}
	_, evs, _ := h.m.Get(r.ID)
	if !hasEvent(evs, "recover", "gen.lock (rebuilt)") {
		t.Error("no recover event for the regenerated file")
	}
}

// The target changes the file a bead changed: Claude settles the landing
// merge and the round lands without stopping.
func TestLandingConflictResolvedByClaude(t *testing.T) {
	beads := newFakeBeads(&fakeBead{ID: "T-F", Title: "Add f", Type: "feature"})
	h := newHarness(t, beads, noFindings)
	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-F"}})
	if err != nil {
		t.Fatal(err)
	}
	h.wait(t, r.ID, api.RoundReadyToLand)
	commitOnMain(t, h.repo, map[string]string{"f.txt": "main's f\n"}, "main changes f")
	if _, err := h.m.Land(r.ID); err != nil {
		t.Fatal(err)
	}
	landed := h.wait(t, r.ID, api.RoundLanded)
	if landed.Stop != nil {
		t.Errorf("round stopped on the way: %+v", landed.Stop)
	}
	_, evs, _ := h.m.Get(r.ID)
	if !hasEvent(evs, "recover", "Sent to Claude to fix: git reported conflicts in: f.txt") {
		t.Error("the landing conflict didn't go to Claude")
	}
	if !hasEvent(evs, "recover", "Claude settled merging main") || hasEvent(evs, "recover", "without Claude: turn") {
		t.Error("the log doesn't say Claude settled the landing merge")
	}
	if b, _ := os.ReadFile(filepath.Join(h.repo, "f.txt")); strings.TrimSpace(string(b)) != "main's f" {
		t.Errorf("f.txt = %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(h.repo, "VERSION")); strings.TrimSpace(string(b)) != "0.2.0" {
		t.Errorf("VERSION = %q, want 0.2.0", b)
	}
}

// When Claude can't settle the landing merge, the round stops with a
// diagnosis and choices, refuses to continue until something changes, and
// lands once Ryder has merged it by hand.
func TestLandingStopDiagnosesAndRefusesToRepeat(t *testing.T) {
	beads := newFakeBeads(&fakeBead{ID: "T-G", Title: "Add g", Type: "task"})
	h := newHarness(t, beads, noFindings)
	ctx := context.Background()
	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-G"}})
	if err != nil {
		t.Fatal(err)
	}
	ready := h.wait(t, r.ID, api.RoundReadyToLand)
	commitOnMain(t, h.repo, map[string]string{"stubborn.txt": "main\n"}, "main changes stubborn")
	if _, err := h.m.Land(r.ID); err != nil {
		t.Fatal(err)
	}
	stopped := h.waitFor(t, r.ID, "a diagnosed stop", func(x api.Round) bool {
		return x.State == api.RoundPaused && x.Stop != nil && x.Stop.Diagnosis != nil && x.Stop.Fingerprint != ""
	})
	s := stopped.Stop
	if s.Kind != api.StopMergeConflict || s.Phase != "land" || !strings.Contains(s.Reason, "stubborn.txt") {
		t.Fatalf("stop = %+v", s)
	}
	if ids := strings.Join(choiceIDs(s), ","); ids != "retry-with-budget,continue,cancel" {
		t.Errorf("choices = %s", ids)
	}
	if s.Choices[0].Explanation != "Claude tries the merge again." {
		t.Errorf("diagnosis didn't explain the choice: %q", s.Choices[0].Explanation)
	}
	if len(s.Attempted) == 0 || !strings.HasPrefix(s.Attempted[0], "turn:land-merge") {
		t.Errorf("attempted = %v", s.Attempted)
	}
	if stopped.OverheadUSD <= 0 {
		t.Error("the diagnosis cost wasn't counted")
	}
	if len(h.head.diagnosed()) == 0 || !strings.Contains(h.head.diagnosed()[0], "stubborn.txt") {
		t.Error("the diagnosis wasn't told about the conflict")
	}
	if head, _ := gitx.RevParse(ctx, ready.Workspace, "HEAD"); head != ready.FinalCommit {
		t.Errorf("the failed merge wasn't undone: HEAD %s, final %s", head, ready.FinalCommit)
	}
	if _, err := h.m.Resume(r.ID); err == nil || !strings.Contains(err.Error(), "Nothing has changed") {
		t.Fatalf("resume without a change: err = %v", err)
	}
	// Ryder merges main in by hand, taking main's side.
	ws := ready.Workspace
	gitx.Run(ctx, ws, "merge", "--no-edit", "main")
	gitx.Run(ctx, ws, "checkout", "--theirs", "--", "stubborn.txt")
	if _, err := gitx.CommitAll(ctx, ws, "merge main by hand"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.m.Choose(r.ID, api.ChoiceContinue, 0, 0); err != nil {
		t.Fatal(err)
	}
	h.wait(t, r.ID, api.RoundLanded)
	if b, _ := os.ReadFile(filepath.Join(h.repo, "stubborn.txt")); strings.TrimSpace(string(b)) != "main" {
		t.Errorf("stubborn.txt = %q", b)
	}
}

// Running out of budget stops with the spend per bead and only the choices
// that fit; raising the limit carries on.
func TestLimitsStopOffersOnlyRaiseOrCancel(t *testing.T) {
	beads := newFakeBeads(&fakeBead{ID: "T-B", Title: "Add b", Type: "task"}, &fakeBead{ID: "T-F", Title: "Add f", Type: "task"})
	plan := `{"summary":"","beads":[{"id":"T-F","areas":[],"files":[],"depends_on":["T-B"],"needs":[],"human_check":""}],"findings":[]}`
	h := newHarness(t, beads, plan)
	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-B", "T-F"}, BudgetUSD: 0.42})
	if err != nil {
		t.Fatal(err)
	}
	stopped := h.wait(t, r.ID, api.RoundPaused)
	s := stopped.Stop
	if s == nil || s.Kind != api.StopLimits {
		t.Fatalf("stop = %+v", s)
	}
	if ids := strings.Join(choiceIDs(s), ","); ids != "raise-limits,cancel" {
		t.Errorf("choices = %s", ids)
	}
	spent := map[string]float64{}
	for _, b := range s.Spend {
		spent[b.Bead] = b.SpentUSD
	}
	if spent["T-B"] < 0.09 {
		t.Errorf("spend = %+v", s.Spend)
	}
	if len(h.head.diagnosed()) != 0 {
		t.Error("a limits stop was diagnosed")
	}
	if _, err := h.m.Resume(r.ID); err == nil {
		t.Error("resume carried on at the limit")
	}
	if _, err := h.m.Choose(r.ID, api.ChoiceRaiseLimits, 5, 0); err != nil {
		t.Fatal(err)
	}
	h.wait(t, r.ID, api.RoundReadyToLand)
}

// A bead parked in one round continues from its kept branch in the next,
// and the kept branch goes once the new round lands.
func TestParkedWorkCarriesIntoTheNextRound(t *testing.T) {
	beads := newFakeBeads(&fakeBead{ID: "T-B", Title: "Add b", Type: "bug"}, &fakeBead{ID: "T-E", Title: "Add e", Type: "task"})
	h := newHarness(t, beads, noFindings)
	ctx := context.Background()
	a, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-B", "T-E"}})
	if err != nil {
		t.Fatal(err)
	}
	readyA := h.wait(t, a.ID, api.RoundReadyToLand)
	kept := workspace.BeadBranch(a.ID, "T-E")
	for _, b := range readyA.Beads {
		if b.ID == "T-E" && b.State != api.BeadParked {
			t.Fatalf("T-E = %s, want parked", b.State)
		}
	}
	if _, err := h.m.Land(a.ID); err != nil {
		t.Fatal(err)
	}
	h.wait(t, a.ID, api.RoundLanded)
	if !gitx.BranchExists(ctx, h.repo, kept) {
		t.Fatal("the parked bead's branch wasn't kept")
	}

	b, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-E"}})
	if err != nil {
		t.Fatal(err)
	}
	readyB := h.wait(t, b.ID, api.RoundReadyToLand)
	if readyB.Beads[0].CarriedFrom != kept {
		t.Errorf("carried from %q, want %q", readyB.Beads[0].CarriedFrom, kept)
	}
	_, evs, _ := h.m.Get(b.ID)
	if !hasEvent(evs, "recover", "Continues round "+a.ID) {
		t.Error("no event for continuing the earlier work")
	}
	h.waitFor(t, a.ID, "the pickup recorded on the earlier round", func(x api.Round) bool {
		for _, bb := range x.Beads {
			if bb.ID == "T-E" {
				return bb.PickedUpBy == b.ID
			}
		}
		return false
	})
	if _, err := h.m.Land(b.ID); err != nil {
		t.Fatal(err)
	}
	h.wait(t, b.ID, api.RoundLanded)
	if gitx.BranchExists(ctx, h.repo, kept) {
		t.Error("the kept branch outlived the round that continued it")
	}
	if got, _ := os.ReadFile(filepath.Join(h.repo, "e.txt")); strings.TrimSpace(string(got)) != "fixed" {
		t.Errorf("e.txt = %q", got)
	}
}

// A round paused before stops existed loads with a legacy stop.
func TestLegacyPauseLoadsAsAStop(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "round.json"), []byte(`{"id":"rdold","state":"paused","paused_from":"implementing","pause_reason":"something old","workspace":"/nonexistent","repo_path":"`+t.TempDir()+`"}`), 0o600)
	m := &Manager{rounds: map[string]*round{}}
	r, err := m.load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.meta.Stop == nil || r.meta.Stop.Kind != api.StopConfig && r.meta.Stop.Kind != api.StopLegacy {
		t.Fatalf("stop = %+v", r.meta.Stop)
	}
	if !hasChoice(*r.meta.Stop, api.ChoiceContinue) || !hasChoice(*r.meta.Stop, api.ChoiceCancel) {
		t.Errorf("choices = %v", choiceIDs(r.meta.Stop))
	}
}

// A pre-flight merge that conflicts goes to Claude, and the beads start on
// top of the settled merge without asking again.
func TestPreflightMergeResolvedByClaude(t *testing.T) {
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
	commitOnMain(t, h.repo, map[string]string{"clash.txt": "main\n"}, "main work")

	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-B"}})
	if err != nil {
		t.Fatal(err)
	}
	asked := h.wait(t, r.ID, api.RoundNeedsDecision)
	if _, err := h.m.Answer(r.ID, asked.Questions[0].ID, AnswerMergeFirst); err != nil {
		t.Fatal(err)
	}
	done := h.wait(t, r.ID, api.RoundReadyToLand)
	if done.Questions[0].Answer != AnswerMergeFirst || done.Questions[0].Problem != "" {
		t.Errorf("question = %+v", done.Questions[0])
	}
	_, evs, _ := h.m.Get(r.ID)
	if !hasEvent(evs, "merge", "Merged side into the round; Claude settled it") {
		t.Error("no event for the settled pre-flight merge")
	}
	if b, _ := os.ReadFile(filepath.Join(done.Workspace, "clash.txt")); strings.TrimSpace(string(b)) != "side" {
		t.Errorf("clash.txt = %q, want side's", b)
	}
}

func TestRecoveryPromptContracts(t *testing.T) {
	cfg, err := repoconfigParse(testYAML)
	if err != nil {
		t.Fatal(err)
	}
	v := integrationVars{Round: api.Round{ID: "rd1", RepoName: "Nacre", Branch: "nacre/r/rd1/integration"}, Conflicts: []string{"a.go"}, Cfg: cfg}
	land := incomingPrompt(v, incoming{Name: "main", Log: "abc123 Ship 0.5.1", Landing: true})
	for _, want := range []string{markIncoming, "main moved since the round started", "abc123 Ship 0.5.1", "- a.go", "Never revert main's commits", "keep main's version", "Don't change the version", "nacred gates --skip-final"} {
		if !strings.Contains(land, want) {
			t.Errorf("landing merge prompt lacks %q:\n%s", want, land)
		}
	}
	v.Conflicts = nil
	fix := incomingPrompt(v, incoming{Name: "main", Landing: true})
	if !strings.HasPrefix(fix, markLandFix) || !strings.Contains(fix, "nacred gates --build") {
		t.Errorf("landing fix prompt:\n%s", fix)
	}
	pre := incomingPrompt(integrationVars{Round: v.Round, Conflicts: []string{"x.kt"}, Cfg: cfg}, incoming{Name: "origin/old", Why: "It edits x.kt too."})
	if !strings.Contains(pre, "Ryder chose to bring origin/old into the round") || !strings.Contains(pre, "It edits x.kt too.") {
		t.Errorf("pre-flight merge prompt:\n%s", pre)
	}

	base := beadPromptVars{Round: api.Round{ID: "rd2", RepoName: "Nacre"}, Issue: issue{ID: "T-E", Title: "e"}, Branch: "nacre/r/rd2/T-E", Cfg: cfg}
	merged := base
	merged.Earlier = &earlierWork{Round: "rd1", Branch: "nacre/r/rd1/T-E", Reason: "it failed its gates", Merged: true}
	if p := beadPrompt(merged); !strings.Contains(p, "Earlier work on this bead") || !strings.Contains(p, "already merged into your branch") || !strings.Contains(p, "Don't merge") {
		t.Errorf("merged earlier work:\n%s", p)
	}
	pending := base
	pending.Earlier = &earlierWork{Round: "rd1", Branch: "nacre/r/rd1/T-E", Reason: "it failed its gates"}
	if p := beadPrompt(pending); !strings.Contains(p, "git merge nacre/r/rd1/T-E") || !strings.Contains(p, "Apart from merging nacre/r/rd1/T-E") {
		t.Errorf("unmerged earlier work:\n%s", p)
	}
	if p := beadPrompt(base); strings.Contains(p, "Earlier work") {
		t.Error("a fresh bead was told about earlier work")
	}
}
