package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/gitx"
)

var ctx = context.Background()

func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, a := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "t@e.com"}, {"config", "user.name", "T"}, {"config", "commit.gpgsign", "false"}} {
		if _, err := gitx.Run(ctx, dir, a...); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644)
	if _, err := gitx.CommitAll(ctx, dir, "first"); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRoundWorkspaces(t *testing.T) {
	r := repo(t)
	m := &Manager{Root: filepath.Join(t.TempDir(), "worktrees")}
	main, _ := gitx.RevParse(ctx, r, "main")

	integ, err := m.CreateIntegration(ctx, r, "repo1", "rd1", "main")
	if err != nil {
		t.Fatal(err)
	}
	if integ.Branch != "nacre/r/rd1/integration" || integ.Base != main {
		t.Fatalf("integration = %+v", integ)
	}
	// Idempotent after a restart.
	again, err := m.CreateIntegration(ctx, r, "repo1", "rd1", "main")
	if err != nil || again.Path != integ.Path {
		t.Fatalf("again = %+v %v", again, err)
	}

	// A bead started after something was integrated starts on top of it.
	os.WriteFile(filepath.Join(integ.Path, "i.txt"), []byte("i\n"), 0o644)
	head, _ := gitx.CommitAll(ctx, integ.Path, "integrated work")
	bead, err := m.CreateBead(ctx, r, "repo1", "rd1", "NAC-1.2", integ.Branch)
	if err != nil {
		t.Fatal(err)
	}
	if bead.Branch != "nacre/r/rd1/NAC-1.2" || bead.Base != head {
		t.Fatalf("bead = %+v, want base %s", bead, head)
	}
	if _, err := os.Stat(filepath.Join(bead.Path, "i.txt")); err != nil {
		t.Error("bead worktree lacks the integrated work")
	}
	if rel, _ := filepath.Rel(r, bead.Path); filepath.IsLocal(rel) {
		t.Errorf("worktree %s is inside the repository", bead.Path)
	}

	if err := m.Remove(ctx, r, bead.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bead.Path); !os.IsNotExist(err) {
		t.Error("bead worktree still on disk")
	}
	if !gitx.BranchExists(ctx, r, bead.Branch) {
		t.Error("bead branch removed with its worktree")
	}
	if err := m.Remove(ctx, r, r); err == nil {
		t.Error("Remove accepted a path outside its root")
	}

	// Recreating a bead whose worktree is gone but whose branch is left.
	if b2, err := m.CreateBead(ctx, r, "repo1", "rd1", "NAC-1.2", integ.Branch); err != nil || b2.Path != bead.Path {
		t.Fatalf("recreate = %+v %v", b2, err)
	}

	parked, _ := m.CreateBead(ctx, r, "repo1", "rd1", "NAC-9", integ.Branch)
	if err := m.RemoveRound(ctx, r, "repo1", "rd1", map[string]bool{parked.Branch: true}); err != nil {
		t.Fatal(err)
	}
	if gitx.BranchExists(ctx, r, integ.Branch) || gitx.BranchExists(ctx, r, bead.Branch) || !gitx.BranchExists(ctx, r, parked.Branch) {
		t.Error("RemoveRound kept or deleted the wrong branches")
	}
	if wts, _ := gitx.Worktrees(ctx, r); len(wts) != 1 {
		t.Errorf("worktrees left: %+v", wts)
	}
}

func TestSweep(t *testing.T) {
	r := repo(t)
	m := &Manager{Root: filepath.Join(t.TempDir(), "worktrees")}
	live, _ := m.CreateIntegration(ctx, r, "repo1", "live", "main")
	dead, _ := m.CreateIntegration(ctx, r, "repo1", "dead", "main")
	removed := m.Sweep(ctx, map[string]string{"repo1": r}, func(round string) bool { return round == "live" })
	if len(removed) != 1 {
		t.Fatalf("removed = %v", removed)
	}
	if _, err := os.Stat(dead.Path); !os.IsNotExist(err) {
		t.Error("dead round's worktree survived")
	}
	if _, err := os.Stat(live.Path); err != nil {
		t.Error("live round's worktree was removed")
	}
}
