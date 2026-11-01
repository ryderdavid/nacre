package gitx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

// A specification change's artifacts sit untracked in the checkout and land
// again, identical, in the round's commit: landing must not be refused.
func TestFastForwardClearsIdenticalUntrackedFiles(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644)
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-qm", "base")
	git(t, repo, "checkout", "-q", "-b", "round")
	os.MkdirAll(filepath.Join(repo, "openspec/changes/x"), 0o755)
	os.WriteFile(filepath.Join(repo, "openspec/changes/x/tasks.md"), []byte("## 1. Work\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "notes.md"), []byte("theirs\n"), 0o644)
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-qm", "round")
	sha, _ := RevParse(ctx, repo, "HEAD")
	git(t, repo, "checkout", "-q", "main")
	// Same bytes untracked on main, as specflow leaves them.
	os.MkdirAll(filepath.Join(repo, "openspec/changes/x"), 0o755)
	os.WriteFile(filepath.Join(repo, "openspec/changes/x/tasks.md"), []byte("## 1. Work\n"), 0o644)
	// Different bytes: Ryder's own work, never removed.
	os.WriteFile(filepath.Join(repo, "notes.md"), []byte("mine\n"), 0o644)
	if err := FastForward(ctx, repo, "main", sha); err == nil {
		t.Fatal("a differing untracked file was overwritten")
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "notes.md")); string(b) != "mine\n" {
		t.Fatalf("own work changed: %q", b)
	}
	os.Remove(filepath.Join(repo, "notes.md"))
	if err := FastForward(ctx, repo, "main", sha); err != nil {
		t.Fatalf("identical untracked artifacts blocked landing: %v", err)
	}
	if head, _ := RevParse(ctx, repo, "HEAD"); head != sha {
		t.Fatalf("main at %s, want %s", head, sha)
	}
}
