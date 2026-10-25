package gitx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var ctx = context.Background()

// NewRepo makes a git repository with one commit on main, for tests.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgsign", "false"},
	} {
		if _, err := Run(ctx, dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	write(t, dir, "a.txt", "one\n")
	if _, err := CommitAll(ctx, dir, "first"); err != nil {
		t.Fatal(err)
	}
	return dir
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWorktreeMergeAndFastForward(t *testing.T) {
	repo := newRepo(t)
	base, _ := RevParse(ctx, repo, "main")
	integ := filepath.Join(t.TempDir(), "integ")
	if err := AddWorktree(ctx, repo, integ, "nacre/r/1/integration", base); err != nil {
		t.Fatal(err)
	}
	bead := filepath.Join(t.TempDir(), "bead")
	if err := AddWorktree(ctx, repo, bead, "nacre/r/1/X-1", base); err != nil {
		t.Fatal(err)
	}
	write(t, bead, "b.txt", "bee\n")
	if d, _ := Dirty(ctx, bead); len(d) != 1 || d[0] != "b.txt" {
		t.Fatalf("dirty = %v", d)
	}
	if _, err := CommitAll(ctx, bead, "add b"); err != nil {
		t.Fatal(err)
	}
	if n, _ := CommitsBetween(ctx, bead, base, "HEAD"); n != 1 {
		t.Fatalf("commits = %d", n)
	}
	res, err := Merge(ctx, integ, "nacre/r/1/X-1", "Merge X-1")
	if err != nil || res.Commit == "" || len(res.Conflicts) != 0 {
		t.Fatalf("merge = %+v %v", res, err)
	}
	// main is checked out in the repo itself: fast-forward updates its files.
	if err := FastForward(ctx, repo, "main", res.Commit); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, "b.txt")); err != nil {
		t.Errorf("main checkout not updated: %v", err)
	}
	if err := RemoveWorktree(ctx, repo, bead); err != nil {
		t.Fatal(err)
	}
	if err := DeleteBranch(ctx, repo, "nacre/r/1/X-1"); err != nil || BranchExists(ctx, repo, "nacre/r/1/X-1") {
		t.Fatalf("delete: %v", err)
	}
	wts, _ := Worktrees(ctx, repo)
	if len(wts) != 2 || wts[1].Branch != "nacre/r/1/integration" {
		t.Errorf("worktrees = %+v", wts)
	}
}

func TestConflictAbortAndReset(t *testing.T) {
	repo := newRepo(t)
	base, _ := RevParse(ctx, repo, "main")
	integ := filepath.Join(t.TempDir(), "integ")
	AddWorktree(ctx, repo, integ, "nacre/r/1/integration", base)
	for i, name := range []string{"one", "two"} {
		wt := filepath.Join(t.TempDir(), name)
		if err := AddWorktree(ctx, repo, wt, "b/"+name, base); err != nil {
			t.Fatal(err)
		}
		write(t, wt, "a.txt", name+" was here\n")
		CommitAll(ctx, wt, name)
		res, err := Merge(ctx, integ, "b/"+name, "merge "+name)
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 && (len(res.Conflicts) != 1 || res.Conflicts[0] != "a.txt" || !MergeInProgress(ctx, integ)) {
			t.Fatalf("second merge = %+v", res)
		}
	}
	before, _ := RevParse(ctx, integ, "HEAD")
	if err := ResetHard(ctx, integ, before); err != nil {
		t.Fatal(err)
	}
	if MergeInProgress(ctx, integ) {
		t.Error("merge still in progress after reset")
	}
	if d, _ := Dirty(ctx, integ); len(d) != 0 {
		t.Errorf("dirty after reset: %v", d)
	}
}

func TestFastForwardRefusesDivergence(t *testing.T) {
	repo := newRepo(t)
	base, _ := RevParse(ctx, repo, "main")
	wt := filepath.Join(t.TempDir(), "wt")
	AddWorktree(ctx, repo, wt, "side", base)
	write(t, wt, "s.txt", "s\n")
	side, _ := CommitAll(ctx, wt, "side")
	write(t, repo, "m.txt", "m\n")
	CommitAll(ctx, repo, "main moved")
	if err := FastForward(ctx, repo, "main", side); err != ErrNotFastForward {
		t.Fatalf("err = %v", err)
	}
}

func TestUnmergedAndDirtyIgnore(t *testing.T) {
	repo := newRepo(t)
	base, _ := RevParse(ctx, repo, "main")
	for _, b := range []string{"feature", "nacre/r/9/X"} {
		wt := filepath.Join(t.TempDir(), strings.ReplaceAll(b, "/", "-"))
		AddWorktree(ctx, repo, wt, b, base)
		write(t, wt, "f-"+strings.ReplaceAll(b, "/", "-")+".txt", "x\n")
		CommitAll(ctx, wt, b)
	}
	list, err := Unmerged(ctx, repo, "main", "nacre/r/")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "feature" || len(list[0].Files) != 1 || list[0].Files[0] != "f-feature.txt" {
		t.Errorf("unmerged = %+v", list)
	}
	os.MkdirAll(filepath.Join(repo, ".beads"), 0o755)
	write(t, repo, ".beads/interactions.jsonl", "{}\n")
	if d, _ := Dirty(ctx, repo, ".beads/"); len(d) != 0 {
		t.Errorf("dirty = %v", d)
	}
}

func TestUnmergedSkipsWorkAlreadyInTargetByContent(t *testing.T) {
	repo := newRepo(t)
	base, _ := RevParse(ctx, repo, "main")
	wt := filepath.Join(t.TempDir(), "old")
	AddWorktree(ctx, repo, wt, "old", base)
	write(t, wt, "b.txt", "b\n")
	sha, _ := CommitAll(ctx, wt, "add b")
	// main moves on, takes the same change by cherry-pick, then changes it again.
	write(t, repo, "m.txt", "m\n")
	CommitAll(ctx, repo, "main work")
	if _, err := Run(ctx, repo, "cherry-pick", sha); err != nil {
		t.Fatal(err)
	}
	write(t, repo, "b.txt", "b, improved\n")
	CommitAll(ctx, repo, "improve b")
	if IsAncestor(ctx, repo, "old", "main") {
		t.Fatal("setup: old should not be an ancestor of main")
	}
	if !Upstream(ctx, repo, "main", "old") {
		t.Error("old's only commit is in main by content")
	}
	if list, _ := Unmerged(ctx, repo, "main"); len(list) != 0 {
		t.Errorf("unmerged = %+v, want none", list)
	}
	// A further commit of its own makes it outstanding again.
	write(t, wt, "c.txt", "c\n")
	CommitAll(ctx, wt, "add c")
	if list, _ := Unmerged(ctx, repo, "main"); len(list) != 1 || list[0].Name != "old" {
		t.Errorf("unmerged = %+v, want old", list)
	}
}
