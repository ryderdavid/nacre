// Package workspace makes and removes the worktrees a round works in: one
// integration worktree per round and one per bead, all outside the
// repository so repo discovery never sees them. Branches are
// nacre/r/<round>/integration and nacre/r/<round>/<bead>.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/gitx"
)

// BranchPrefix starts every branch a round makes.
const BranchPrefix = "nacre/r/"

// integrationName is the integration worktree's directory and branch leaf.
const integrationName = "integration"

// Workspace is a worktree on its own branch.
type Workspace struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
	// Base is the commit the branch started from.
	Base string `json:"base"`
}

// Manager creates workspaces under Root (<data_dir>/worktrees).
type Manager struct {
	Root string
}

var unsafe = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// clean makes an id safe as one path element and one branch component.
func clean(id string) string {
	s := unsafe.ReplaceAllString(id, "_")
	s = strings.Trim(s, ".")
	if s == "" {
		s = "_"
	}
	return s
}

// IntegrationBranch is the round's integration branch.
func IntegrationBranch(round string) string {
	return BranchPrefix + clean(round) + "/" + integrationName
}

// BeadBranch is a bead's branch in a round.
func BeadBranch(round, bead string) string { return BranchPrefix + clean(round) + "/" + clean(bead) }

func (m *Manager) dir(repoID, round, name string) string {
	return filepath.Join(m.Root, clean(repoID), clean(round), clean(name))
}

// CreateIntegration makes the round's integration branch at base (a commit or
// ref, resolved and recorded as a commit) and its worktree.
func (m *Manager) CreateIntegration(ctx context.Context, repoPath, repoID, round, base string) (Workspace, error) {
	sha, err := gitx.RevParse(ctx, repoPath, base)
	if err != nil {
		return Workspace{}, fmt.Errorf("base %s: %w", base, err)
	}
	return m.create(ctx, repoPath, m.dir(repoID, round, integrationName), IntegrationBranch(round), sha)
}

// CreateBead makes a bead's branch from the current head of from (normally the
// integration branch) and its worktree.
func (m *Manager) CreateBead(ctx context.Context, repoPath, repoID, round, bead, from string) (Workspace, error) {
	sha, err := gitx.RevParse(ctx, repoPath, from)
	if err != nil {
		return Workspace{}, fmt.Errorf("start of %s: %w", bead, err)
	}
	return m.create(ctx, repoPath, m.dir(repoID, round, bead), BeadBranch(round, bead), sha)
}

// create is idempotent: a worktree already on the branch (nacred restarted
// mid-round) is returned as it is, with the base it was created from.
func (m *Manager) create(ctx context.Context, repo, path, branch, base string) (Workspace, error) {
	wts, err := gitx.Worktrees(ctx, repo)
	if err != nil {
		return Workspace{}, err
	}
	if gitx.BranchExists(ctx, repo, branch) {
		// An existing branch (nacred restarted, or the bead is starting
		// again) started where it meets base, not at base's head now.
		if mb, err := gitx.Run(ctx, repo, "merge-base", branch, base); err == nil && mb != "" {
			base = mb
		}
	}
	for _, wt := range wts {
		if wt.Branch == branch {
			if samePath(wt.Path, path) {
				return Workspace{Path: path, Branch: branch, Base: base}, nil
			}
			return Workspace{}, fmt.Errorf("branch %s is already checked out at %s", branch, wt.Path)
		}
	}
	if gitx.BranchExists(ctx, repo, branch) {
		// Left behind without a worktree: attach a new one to it.
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return Workspace{}, err
		}
		if _, err := gitx.Run(ctx, repo, "worktree", "add", path, branch); err != nil {
			return Workspace{}, err
		}
		return Workspace{Path: path, Branch: branch, Base: base}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Workspace{}, err
	}
	if err := gitx.AddWorktree(ctx, repo, path, branch, base); err != nil {
		return Workspace{}, err
	}
	return Workspace{Path: path, Branch: branch, Base: base}, nil
}

func samePath(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 != nil || err2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

// Remove deletes a worktree; its branch stays.
func (m *Manager) Remove(ctx context.Context, repoPath, path string) error {
	if path == "" {
		return nil
	}
	if !m.owns(path) {
		return fmt.Errorf("%s is not a workspace nacred made", path)
	}
	if err := gitx.RemoveWorktree(ctx, repoPath, path); err != nil {
		return err
	}
	_ = os.RemoveAll(path)
	// Drop the round's directory once its last worktree is gone.
	_ = os.Remove(filepath.Dir(path))
	return nil
}

// owns reports whether path is under Root, so Remove can never delete
// anything else.
func (m *Manager) owns(path string) bool {
	under := func(root, p string) bool {
		rel, err := filepath.Rel(root, p)
		return err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
	}
	if under(m.Root, path) {
		return true
	}
	// git reports resolved paths (/private/var/… for /var/… on macOS).
	root, err1 := filepath.EvalSymlinks(m.Root)
	p, err2 := filepath.EvalSymlinks(path)
	return err1 == nil && err2 == nil && under(root, p)
}

// RemoveRound removes every worktree of the round and deletes its branches,
// except the branches in keep (parked beads).
func (m *Manager) RemoveRound(ctx context.Context, repoPath, repoID, round string, keep map[string]bool) error {
	var errs []error
	wts, err := gitx.Worktrees(ctx, repoPath)
	if err != nil {
		return err
	}
	prefix := BranchPrefix + clean(round) + "/"
	for _, wt := range wts {
		if strings.HasPrefix(wt.Branch, prefix) && m.owns(wt.Path) {
			if err := m.Remove(ctx, repoPath, wt.Path); err != nil {
				errs = append(errs, err)
			}
		}
	}
	out, err := gitx.Run(ctx, repoPath, "for-each-ref", "--format=%(refname:short)", "refs/heads/"+strings.TrimSuffix(prefix, "/"))
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	for _, b := range strings.Split(out, "\n") {
		if b = strings.TrimSpace(b); b == "" || keep[b] {
			continue
		}
		if err := gitx.DeleteBranch(ctx, repoPath, b); err != nil {
			errs = append(errs, err)
		}
	}
	_ = os.RemoveAll(filepath.Join(m.Root, clean(repoID), clean(round)))
	return errors.Join(errs...)
}

// Sweep removes worktree directories left by rounds that are no longer alive
// (landed, cancelled with nothing to recover, or unknown), returning what it
// removed. repoPath maps repo ids to paths; alive reports whether a round's
// worktrees are still needed.
func (m *Manager) Sweep(ctx context.Context, repoPath map[string]string, alive func(round string) bool) []string {
	var removed []string
	repos, _ := os.ReadDir(m.Root)
	for _, r := range repos {
		if !r.IsDir() {
			continue
		}
		rounds, _ := os.ReadDir(filepath.Join(m.Root, r.Name()))
		for _, rd := range rounds {
			if !rd.IsDir() || alive(rd.Name()) {
				continue
			}
			dir := filepath.Join(m.Root, r.Name(), rd.Name())
			if repo := repoPath[r.Name()]; repo != "" {
				leaves, _ := os.ReadDir(dir)
				for _, l := range leaves {
					_ = gitx.RemoveWorktree(ctx, repo, filepath.Join(dir, l.Name()))
				}
				_, _ = gitx.Run(ctx, repo, "worktree", "prune")
			}
			if os.RemoveAll(dir) == nil {
				removed = append(removed, dir)
			}
		}
	}
	return removed
}
