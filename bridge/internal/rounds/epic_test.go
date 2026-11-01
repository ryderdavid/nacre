package rounds

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
