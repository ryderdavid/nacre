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
)

func readVersion(t *testing.T, repo, ref string) string {
	t.Helper()
	out, err := gitx.Run(context.Background(), repo, "show", ref+":VERSION")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out)
}

// An epic's beads land on its branch; finishing the epic merges the branch
// into main with one version bump, closes the epic and removes the branch.
func TestEpicWorkLandsOnItsBranchUntilTheEpicIsFinished(t *testing.T) {
	ctx := context.Background()
	beads := newFakeBeads(
		&fakeBead{ID: "T-E", Title: "Drafts", Type: "epic", Status: "open"},
		&fakeBead{ID: "T-F", Title: "Add f", Type: "feature", Parent: "T-E"},
	)
	h := newHarness(t, beads, emptyPlan)
	mainBefore, _ := gitx.RevParse(ctx, h.repo, "main")

	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-F"}, LandWhenReady: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.Target != "epic/t-e" || r.Epic != "T-E" {
		t.Fatalf("round target = %q epic = %q, want the epic's branch", r.Target, r.Epic)
	}
	landed := h.wait(t, r.ID, api.RoundLanded, api.RoundPaused)
	if landed.State != api.RoundLanded {
		t.Fatalf("round = %s %q", landed.State, landed.PauseReason)
	}
	if now, _ := gitx.RevParse(ctx, h.repo, "main"); now != mainBefore {
		t.Fatal("epic work landed on main")
	}
	if _, err := gitx.Run(ctx, h.repo, "show", "epic/t-e:f.txt"); err != nil {
		t.Fatal("the epic branch lacks the bead's work")
	}
	if v := readVersion(t, h.repo, "epic/t-e"); v != "0.1.0" || landed.Tag != "" {
		t.Fatalf("an epic round bumped (%s) or tagged (%q)", v, landed.Tag)
	}
	if beads.bead("T-F").Status != "closed" {
		t.Fatal("the bead isn't closed after landing on its epic branch")
	}

	fin, closed, err := h.m.FinishEpic("repo", "T-E")
	if err != nil || closed {
		t.Fatalf("finish: closed=%v err=%v", closed, err)
	}
	if fin.FinishEpic != "T-E" || fin.Target != "main" || len(fin.Beads) != 0 {
		t.Fatalf("finish round = %+v", fin)
	}
	done := h.wait(t, fin.ID, api.RoundLanded, api.RoundPaused)
	if done.State != api.RoundLanded {
		t.Fatalf("finish round = %s %q", done.State, done.PauseReason)
	}
	if _, err := os.Stat(filepath.Join(h.repo, "f.txt")); err != nil {
		t.Fatal("main lacks the epic's work")
	}
	if v := readVersion(t, h.repo, "main"); v != "0.2.0" || done.Tag != "v0.2.0" {
		t.Fatalf("main version %s tag %q, want one bump to 0.2.0", v, done.Tag)
	}
	if beads.bead("T-E").Status != "closed" {
		t.Fatal("the finished epic is still open")
	}
	if gitx.BranchExists(ctx, h.repo, "epic/t-e") {
		t.Fatal("the epic's branch wasn't removed")
	}
}

func TestFinishEpicRefusesOpenWorkAndClosesEpicsWithNothingToMerge(t *testing.T) {
	beads := newFakeBeads(
		&fakeBead{ID: "T-E", Title: "Drafts", Type: "epic", Status: "open"},
		&fakeBead{ID: "T-F", Title: "Add f", Type: "feature", Parent: "T-E"},
		&fakeBead{ID: "T-D", Title: "Done already", Type: "epic", Status: "open"},
		&fakeBead{ID: "T-G", Title: "Old", Type: "task", Parent: "T-D", Status: "closed"},
	)
	h := newHarness(t, beads, emptyPlan)
	if _, _, err := h.m.FinishEpic("repo", "T-E"); err == nil || !strings.Contains(err.Error(), "T-F") {
		t.Fatalf("finishing an epic with open beads: %v", err)
	}
	if _, closed, err := h.m.FinishEpic("repo", "T-D"); err != nil || !closed || beads.bead("T-D").Status != "closed" {
		t.Fatalf("epic with nothing on its branch: closed=%v err=%v status=%s", closed, err, beads.bead("T-D").Status)
	}
}

func TestBeadsFromDifferentEpicsNeedSeparateRounds(t *testing.T) {
	beads := newFakeBeads(
		&fakeBead{ID: "T-E", Title: "One", Type: "epic", Status: "open"},
		&fakeBead{ID: "T-F", Title: "Add f", Type: "feature", Parent: "T-E"},
		&fakeBead{ID: "T-A", Title: "Loose", Type: "task"},
	)
	h := newHarness(t, beads, emptyPlan)
	if _, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-F", "T-A"}}); err == nil || !strings.Contains(err.Error(), "own branch") {
		t.Fatalf("mixed round: %v", err)
	}
	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-A"}})
	if err != nil || r.Target != "main" {
		t.Fatalf("a bead outside any epic lands on main: %+v %v", r.Target, err)
	}
}

// A change's epic whose work is already on main is still finished by a round,
// so archiving the change lands on main too.
func TestFinishingAChangeEpicArchivesEvenWithItsWorkOnMain(t *testing.T) {
	ctx := context.Background()
	beads := newFakeBeads(
		&fakeBead{ID: "T-E", Title: "Drafts", Type: "epic", Status: "open"},
		&fakeBead{ID: "T-G", Title: "Done", Type: "task", Parent: "T-E", Status: "closed"},
	)
	archivedName, marked := "drafts", ""
	h := newHarnessWith(t, beads, emptyPlan, harnessOpts{
		changeOf: func(_, epic string) string {
			if epic == "T-E" && marked == "" {
				return archivedName
			}
			return ""
		},
		archive: func(ctx context.Context, _, ws, epic string) (string, error) {
			if err := os.WriteFile(filepath.Join(ws, "ARCHIVED"), []byte(epic+"\n"), 0o644); err != nil {
				return "", err
			}
			_, err := gitx.CommitAll(ctx, ws, "Archive drafts")
			return archivedName, err
		},
		archived: func(_, epic string) { marked = epic },
	})
	activeChange(t, h.repo, "drafts")
	fin, closed, err := h.m.FinishEpic("repo", "T-E")
	if err != nil || closed {
		t.Fatalf("finish: closed=%v err=%v", closed, err)
	}
	done := h.wait(t, fin.ID, api.RoundLanded, api.RoundPaused)
	if done.State != api.RoundLanded {
		t.Fatalf("finish round = %s %q", done.State, done.PauseReason)
	}
	if _, err := gitx.Run(ctx, h.repo, "show", "main:ARCHIVED"); err != nil {
		t.Fatal("the archive didn't land on main")
	}
	if marked != "T-E" || beads.bead("T-E").Status != "closed" {
		t.Fatalf("archived=%q epic=%s", marked, beads.bead("T-E").Status)
	}
}

func TestMixedRoundNamesEachEpicsBeads(t *testing.T) {
	beads := newFakeBeads(
		&fakeBead{ID: "T-E", Title: "One", Type: "epic", Status: "open"},
		&fakeBead{ID: "T-F", Title: "Add f", Type: "feature", Parent: "T-E"},
		&fakeBead{ID: "T-A", Title: "Loose", Type: "task"},
	)
	h := newHarness(t, beads, emptyPlan)
	_, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-F", "T-A"}})
	if err == nil || !strings.Contains(err.Error(), "T-E: T-F") || !strings.Contains(err.Error(), "no epic: T-A") {
		t.Fatalf("refusal = %v", err)
	}
}

func TestFinishingAnEpicWhoseBranchIsAlreadyInMainClosesIt(t *testing.T) {
	ctx := context.Background()
	beads := newFakeBeads(
		&fakeBead{ID: "T-E", Title: "Done", Type: "epic", Status: "open"},
		&fakeBead{ID: "T-G", Title: "Old", Type: "task", Parent: "T-E", Status: "closed"},
	)
	h := newHarness(t, beads, emptyPlan)
	if _, err := gitx.Run(ctx, h.repo, "branch", "epic/t-e", "main"); err != nil {
		t.Fatal(err)
	}
	if _, closed, err := h.m.FinishEpic("repo", "T-E"); err != nil || !closed {
		t.Fatalf("closed=%v err=%v", closed, err)
	}
	if beads.bead("T-E").Status != "closed" || gitx.BranchExists(ctx, h.repo, "epic/t-e") {
		t.Fatal("the merged epic wasn't closed and its branch removed")
	}
}

func TestFinishingThatFailsBeforeLandingLeavesMainAndTheEpic(t *testing.T) {
	ctx := context.Background()
	beads := newFakeBeads(
		&fakeBead{ID: "T-E", Title: "Drafts", Type: "epic", Status: "open"},
		&fakeBead{ID: "T-G", Title: "Done", Type: "task", Parent: "T-E", Status: "closed"},
	)
	h := newHarnessWith(t, beads, emptyPlan, harnessOpts{
		changeOf: func(string, string) string { return "drafts" },
		archive: func(context.Context, string, string, string) (string, error) {
			return "", errors.New("openspec archive drafts failed: validation")
		},
	})
	activeChange(t, h.repo, "drafts")
	before, _ := gitx.RevParse(ctx, h.repo, "main")
	fin, _, err := h.m.FinishEpic("repo", "T-E")
	if err != nil {
		t.Fatal(err)
	}
	paused := h.wait(t, fin.ID, api.RoundPaused, api.RoundLanded)
	if paused.State != api.RoundPaused || !strings.Contains(paused.PauseReason, "openspec archive") {
		t.Fatalf("round = %s %q", paused.State, paused.PauseReason)
	}
	if now, _ := gitx.RevParse(ctx, h.repo, "main"); now != before || beads.bead("T-E").Status != "open" {
		t.Fatal("a failed finish moved main or closed the epic")
	}
	if _, err := h.m.Cancel(fin.ID); err != nil {
		t.Fatal(err)
	}
	h.wait(t, fin.ID, api.RoundCancelled)
	if _, _, err := h.m.FinishEpic("repo", "T-E"); err != nil {
		t.Fatalf("finishing again after a cancelled attempt: %v", err)
	}
}

// Approved specification sections land on their own only when asked to;
// land.auto alone keeps them waiting for Ryder.
func TestSpecRoundsLandWhenReadyOnlyWhenAsked(t *testing.T) {
	_, contract := specFixture()
	repo := testRepo(t)
	yaml, _ := os.ReadFile(filepath.Join(repo, "nacre.yaml"))
	os.WriteFile(filepath.Join(repo, "nacre.yaml"), []byte(strings.Replace(string(yaml), "  push: false", "  push: false\n  auto: true", 1)), 0o644)
	if _, err := gitx.CommitAll(context.Background(), repo, "auto land"); err != nil {
		t.Fatal(err)
	}
	beads := newFakeBeads(&fakeBead{ID: "T-1", Title: "Backend", Type: "task"}, &fakeBead{ID: "T-2", Title: "Backend too", Type: "task"})
	h := newHarnessWith(t, beads, emptyPlan, harnessOpts{repo: repo, specContract: func(_ context.Context, _, id string) (*api.SpecContract, error) {
		c := contract
		c.BeadID = id
		return &c, nil
	}})
	for _, tc := range []struct {
		bead string
		ask  bool
	}{{"T-1", false}, {"T-2", true}} {
		r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{tc.bead}, SpecChangeID: contract.ChangeID, SpecRevision: contract.Revision, LandWhenReady: tc.ask})
		if err != nil {
			t.Fatal(err)
		}
		if r.AutoLand != tc.ask {
			t.Fatalf("land_when_ready=%v: auto_land=%v", tc.ask, r.AutoLand)
		}
		if _, err := h.m.Cancel(r.ID); err != nil {
			t.Fatal(err)
		}
		h.wait(t, r.ID, api.RoundCancelled)
	}
}

// A bead whose close fails stays open with the reason on the round, instead
// of the failure being dropped.
func TestAFailedCloseIsRecordedOnTheRound(t *testing.T) {
	beads := newFakeBeads(&fakeBead{ID: "T-F", Title: "Add f", Type: "feature"})
	beads.failClose = map[string]string{"T-F": "cannot close T-F: blocked by open issues [T-Z]"}
	h := newHarness(t, beads, emptyPlan)
	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-F"}, LandWhenReady: true})
	if err != nil {
		t.Fatal(err)
	}
	landed := h.wait(t, r.ID, api.RoundLanded, api.RoundPaused)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && landed.Beads[0].CloseError == "" {
		time.Sleep(20 * time.Millisecond)
		landed, _, _ = h.m.Get(r.ID)
	}
	if b := landed.Beads[0]; b.Closed || !strings.Contains(b.CloseError, "blocked by open issues") {
		t.Fatalf("bead = %+v", b)
	}
	if beads.bead("T-F").Status == "closed" {
		t.Fatal("the fake closed a bead it was told to refuse")
	}
}

// A retried Finish after the archive already landed ends the change and closes
// the epic without another round.
func TestRetriedFinishAfterTheArchiveLandedJustCloses(t *testing.T) {
	beads := newFakeBeads(
		&fakeBead{ID: "T-E", Title: "Drafts", Type: "epic", Status: "open"},
		&fakeBead{ID: "T-G", Title: "Done", Type: "task", Parent: "T-E", Status: "closed"},
	)
	marked := ""
	h := newHarnessWith(t, beads, emptyPlan, harnessOpts{
		changeOf: func(string, string) string { return "drafts" },
		archived: func(_, epic string) { marked = epic },
	})
	if _, closed, err := h.m.FinishEpic("repo", "T-E"); err != nil || !closed {
		t.Fatalf("closed=%v err=%v", closed, err)
	}
	if marked != "T-E" || beads.bead("T-E").Status != "closed" {
		t.Fatalf("archived=%q epic=%s", marked, beads.bead("T-E").Status)
	}
}

// activeChange commits an active OpenSpec change named name to main.
func activeChange(t *testing.T, repo, name string) {
	t.Helper()
	dir := filepath.Join(repo, "openspec", "changes", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "proposal.md"), []byte("## Why\nDrafts.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := gitx.CommitAll(context.Background(), repo, "Change "+name); err != nil {
		t.Fatal(err)
	}
}
