package sessions

import (
	"context"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// gitTimeout bounds the git call that resolves a worktree to its main checkout.
const gitTimeout = 5 * time.Second

// resolveRepo maps a session's working directory to one of list. The repo with
// the longest path containing cwd wins, so a session in a subdirectory — or in
// a worktree under <repo>/.claude/worktrees — resolves to its repo.
//
// A cwd outside every discovered repo is retried against the main checkout of
// its git worktree: an agent working in an external worktree still uses the
// main checkout's beads database, which is the repo nacred knows.
func resolveRepo(cwd string, list []Repo, mainCheckout func(string) string) (Repo, bool) {
	if cwd == "" {
		return Repo{}, false
	}
	// Longest path first, so a repo nested inside another one wins.
	byDepth := append([]Repo(nil), list...)
	sort.Slice(byDepth, func(i, j int) bool { return len(byDepth[i].Path) > len(byDepth[j].Path) })

	cwd = filepath.Clean(cwd)
	if r, ok := match(cwd, byDepth); ok {
		return r, true
	}
	if mainCheckout == nil {
		return Repo{}, false
	}
	main := mainCheckout(cwd)
	if main == "" || main == cwd {
		return Repo{}, false
	}
	return match(filepath.Clean(main), byDepth)
}

func match(path string, byDepth []Repo) (Repo, bool) {
	for _, r := range byDepth {
		if r.Path != "" && under(path, filepath.Clean(r.Path)) {
			return r, true
		}
	}
	return Repo{}, false
}

// under reports whether path is root or lies inside it.
func under(path, root string) bool {
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

// gitMainCheckout returns the main checkout of the git worktree containing
// dir, or "" when dir is not in a git repository. The common git directory
// belongs to the main checkout even for a linked worktree, so its parent is
// the checkout nacred should attribute the session to.
func gitMainCheckout(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return ""
	}
	common := strings.TrimSpace(string(out))
	if common == "" || !filepath.IsAbs(common) {
		return ""
	}
	return filepath.Dir(common)
}
