// Package gitx is the git nacred runs itself: worktrees, merges, resets,
// fast-forwards and pushes. Agents only edit and commit inside the worktree
// they are given; everything that moves a branch happens here, in one place,
// and never through an agent's permissions.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Timeout bounds one git command. Merges of large trees still finish well
// inside it; it only stops a wedged git (e.g. waiting on a lock).
const Timeout = 2 * time.Minute

// Error is a failed git command with its stderr.
type Error struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *Error) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = e.Err.Error()
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), lastLines(msg, 3))
}

func (e *Error) Unwrap() error { return e.Err }

// Run runs git in dir and returns trimmed stdout.
func Run(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	// Never prompt: there is nobody to answer.
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true", "GIT_MERGE_AUTOEDIT=no")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(out.String()), &Error{Args: args, Stderr: errb.String(), Err: err}
	}
	return strings.TrimSpace(out.String()), nil
}

// RevParse resolves ref to a full commit id.
func RevParse(ctx context.Context, dir, ref string) (string, error) {
	return Run(ctx, dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
}

// CurrentBranch is the branch checked out in dir, "" when detached.
func CurrentBranch(ctx context.Context, dir string) string {
	b, err := Run(ctx, dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return b
}

// AddWorktree creates a worktree at path on a new branch starting at base.
func AddWorktree(ctx context.Context, repo, path, branch, base string) error {
	_, err := Run(ctx, repo, "worktree", "add", "-b", branch, path, base)
	return err
}

// RemoveWorktree removes the worktree at path, whatever is in it. Build
// output and other leftovers are expected; the work itself was committed.
func RemoveWorktree(ctx context.Context, repo, path string) error {
	if _, err := Run(ctx, repo, "worktree", "remove", "--force", "--force", path); err != nil {
		// Already gone from disk: let git forget it.
		_, perr := Run(ctx, repo, "worktree", "prune")
		if perr != nil {
			return err
		}
	}
	return nil
}

// Worktree is one entry of `git worktree list`.
type Worktree struct {
	Path   string
	Branch string // short name, "" when detached
	Head   string
}

// Worktrees lists the repository's worktrees, the main one first.
func Worktrees(ctx context.Context, repo string) ([]Worktree, error) {
	out, err := Run(ctx, repo, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var list []Worktree
	var cur *Worktree
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			list = append(list, Worktree{Path: strings.TrimPrefix(line, "worktree ")})
			cur = &list[len(list)-1]
		case cur != nil && strings.HasPrefix(line, "HEAD "):
			cur.Head = strings.TrimPrefix(line, "HEAD ")
		case cur != nil && strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		}
	}
	return list, nil
}

// DeleteBranch deletes a local branch, merged or not.
func DeleteBranch(ctx context.Context, repo, branch string) error {
	_, err := Run(ctx, repo, "branch", "-D", "--", branch)
	return err
}

// BranchExists reports whether a local branch exists.
func BranchExists(ctx context.Context, repo, branch string) bool {
	_, err := Run(ctx, repo, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// Dirty lists files that are modified, staged or untracked in dir, skipping
// paths under any of ignore (e.g. ".beads/", which bd itself writes to).
func Dirty(ctx context.Context, dir string, ignore ...string) ([]string, error) {
	out, err := Run(ctx, dir, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		f := line[3:]
		if i := strings.Index(f, " -> "); i >= 0 {
			f = f[i+4:]
		}
		f = strings.Trim(f, `"`)
		skip := false
		for _, p := range ignore {
			if strings.HasPrefix(f, p) {
				skip = true
			}
		}
		if !skip {
			files = append(files, f)
		}
	}
	return files, nil
}

// CommitsBetween counts commits reachable from head but not from base.
func CommitsBetween(ctx context.Context, dir, base, head string) (int, error) {
	out, err := Run(ctx, dir, "rev-list", "--count", base+".."+head)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(out)
}

// Commit is one commit's id and subject line.
type Commit struct {
	SHA     string
	Subject string
}

// Log lists up to max commits reachable from head but not from base, newest first.
func Log(ctx context.Context, dir, base, head string, max int) ([]Commit, error) {
	out, err := Run(ctx, dir, "log", "--format=%H%x1f%s", "-n", strconv.Itoa(max), base+".."+head)
	if err != nil {
		return nil, err
	}
	var list []Commit
	for _, line := range strings.Split(out, "\n") {
		sha, subject, ok := strings.Cut(line, "\x1f")
		if ok {
			list = append(list, Commit{SHA: sha, Subject: subject})
		}
	}
	return list, nil
}

// FileChange is one file's added and deleted line counts between two commits;
// Binary files have no line counts.
type FileChange struct {
	Path           string
	Added, Deleted int
	Binary         bool
}

// DiffStat lists the files that differ between base and head.
func DiffStat(ctx context.Context, dir, base, head string) ([]FileChange, error) {
	out, err := Run(ctx, dir, "diff", "--numstat", "--no-renames", base, head)
	if err != nil {
		return nil, err
	}
	var list []FileChange
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, "\t", 3)
		if len(f) != 3 {
			continue
		}
		c := FileChange{Path: f[2]}
		if f[0] == "-" {
			c.Binary = true
		} else {
			c.Added, _ = strconv.Atoi(f[0])
			c.Deleted, _ = strconv.Atoi(f[1])
		}
		list = append(list, c)
	}
	return list, nil
}

// IsAncestor reports whether a is an ancestor of (or equal to) b.
func IsAncestor(ctx context.Context, dir, a, b string) bool {
	_, err := Run(ctx, dir, "merge-base", "--is-ancestor", a, b)
	return err == nil
}

// MergeResult is the outcome of Merge.
type MergeResult struct {
	// Conflicts lists conflicted files; the merge is left in progress so an
	// agent can resolve it, or AbortMerge can undo it.
	Conflicts []string
	// Commit is the merge commit when the merge succeeded.
	Commit string
}

// Merge merges branch into the branch checked out in dir with a merge commit.
func Merge(ctx context.Context, dir, branch, message string) (MergeResult, error) {
	_, err := Run(ctx, dir, "merge", "--no-ff", "--no-edit", "-m", message, branch)
	if err != nil {
		conflicts, _ := Conflicts(ctx, dir)
		if len(conflicts) > 0 {
			return MergeResult{Conflicts: conflicts}, nil
		}
		return MergeResult{}, err
	}
	sha, err := RevParse(ctx, dir, "HEAD")
	return MergeResult{Commit: sha}, err
}

// Conflicts lists files with unresolved conflicts.
func Conflicts(ctx context.Context, dir string) ([]string, error) {
	out, err := Run(ctx, dir, "diff", "--name-only", "--diff-filter=U")
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

// MergeInProgress reports whether dir has an unfinished merge.
func MergeInProgress(ctx context.Context, dir string) bool {
	_, err := Run(ctx, dir, "rev-parse", "--verify", "--quiet", "MERGE_HEAD")
	return err == nil
}

// AbortMerge undoes an unfinished merge.
func AbortMerge(ctx context.Context, dir string) error {
	_, err := Run(ctx, dir, "merge", "--abort")
	return err
}

// ResetHard moves the branch checked out in dir to sha and discards changes.
func ResetHard(ctx context.Context, dir, sha string) error {
	if MergeInProgress(ctx, dir) {
		_ = AbortMerge(ctx, dir)
	}
	if _, err := Run(ctx, dir, "reset", "--hard", sha); err != nil {
		return err
	}
	_, err := Run(ctx, dir, "clean", "-fd")
	return err
}

// CommitAll commits every change in dir and returns the new commit; nothing
// to commit returns HEAD unchanged.
func CommitAll(ctx context.Context, dir, message string) (string, error) {
	if _, err := Run(ctx, dir, "add", "-A"); err != nil {
		return "", err
	}
	if _, err := Run(ctx, dir, "diff", "--cached", "--quiet"); err == nil {
		return RevParse(ctx, dir, "HEAD")
	}
	if _, err := Run(ctx, dir, "commit", "-q", "-m", message); err != nil {
		return "", err
	}
	return RevParse(ctx, dir, "HEAD")
}

// UnmergedBranch is a branch with work not in the target.
type UnmergedBranch struct {
	Name  string
	Files []string
}

// Unmerged lists local and remote branches not merged into target, with the
// files each changes relative to their merge base with target. Branches
// whose names start with any of skip are left out, and so are branches whose
// every commit is already in target by content: work that reached target
// through a rebase or cherry-pick isn't outstanding, and merging the stale
// copy would only conflict with what target has done since.
func Unmerged(ctx context.Context, repo, target string, skip ...string) ([]UnmergedBranch, error) {
	out, err := Run(ctx, repo, "for-each-ref", "--format=%(refname:short)", "--no-merged="+target, "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	var list []UnmergedBranch
	for _, name := range strings.Split(out, "\n") {
		name = strings.TrimSpace(name)
		if name == "" || strings.HasSuffix(name, "/HEAD") || name == "origin" {
			continue
		}
		skipped := false
		for _, p := range skip {
			if strings.HasPrefix(name, p) || strings.HasPrefix(name, "origin/"+p) {
				skipped = true
			}
		}
		if skipped || Upstream(ctx, repo, target, name) {
			continue
		}
		files, err := Run(ctx, repo, "diff", "--name-only", target+"..."+name)
		if err != nil {
			continue
		}
		b := UnmergedBranch{Name: name}
		if files != "" {
			b.Files = strings.Split(files, "\n")
		}
		list = append(list, b)
	}
	return list, nil
}

// Upstream reports whether every commit on branch that target lacks by
// ancestry is in target anyway as an equivalent patch (git cherry).
func Upstream(ctx context.Context, repo, target, branch string) bool {
	out, err := Run(ctx, repo, "cherry", target, branch)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "+") {
			return false
		}
	}
	return true
}

// ErrNotFastForward means the target has commits the new commit lacks.
var ErrNotFastForward = errors.New("not a fast-forward")

// FastForward moves branch target to sha, which must contain target's current
// commit. If target is checked out in a worktree, that worktree is updated
// with a fast-forward merge; otherwise only the ref moves.
func FastForward(ctx context.Context, repo, target, sha string) error {
	old, err := RevParse(ctx, repo, "refs/heads/"+target)
	if err != nil {
		return fmt.Errorf("branch %s: %w", target, err)
	}
	if !IsAncestor(ctx, repo, old, sha) {
		return ErrNotFastForward
	}
	wts, err := Worktrees(ctx, repo)
	if err != nil {
		return err
	}
	for _, wt := range wts {
		if wt.Branch == target {
			if err := clearIdenticalUntracked(ctx, wt.Path, old, sha); err != nil {
				return err
			}
			_, err := Run(ctx, wt.Path, "merge", "--ff-only", sha)
			return err
		}
	}
	_, err = Run(ctx, repo, "update-ref", "refs/heads/"+target, sha, old)
	return err
}

// clearIdenticalUntracked removes untracked files in the worktree at dir that
// the commits old..sha add with exactly the same content, so the fast-forward
// isn't refused for them: a specification change's artifacts are written to
// the checkout untracked and land again in the round's commit. A file that
// differs in any byte is left alone, and the merge refuses as before.
func clearIdenticalUntracked(ctx context.Context, dir, old, sha string) error {
	added, err := Run(ctx, dir, "diff", "--name-only", "--diff-filter=A", old, sha)
	if err != nil {
		return err
	}
	for _, p := range strings.Split(strings.TrimSpace(added), "\n") {
		if p == "" {
			continue
		}
		local := filepath.Join(dir, filepath.FromSlash(p))
		have, err := os.ReadFile(local)
		if err != nil {
			continue // absent (or unreadable): nothing to clear
		}
		if _, err := Run(ctx, dir, "ls-files", "--error-unmatch", "--", p); err == nil {
			continue // tracked: the merge handles it
		}
		cmd := exec.CommandContext(ctx, "git", "show", sha+":"+p)
		cmd.Dir = dir
		want, err := cmd.Output()
		if err != nil || !bytes.Equal(want, have) {
			continue
		}
		if err := os.Remove(local); err != nil {
			return err
		}
	}
	return nil
}

// Push pushes branch to remote.
func Push(ctx context.Context, repo, remote, branch string) error {
	_, err := Run(ctx, repo, "push", remote, branch+":"+branch)
	return err
}

// Tag puts the annotated tag name on sha. A tag of that name already on sha
// is fine; one on another commit is an error.
func Tag(ctx context.Context, repo, name, sha, message string) error {
	if have, err := RevParse(ctx, repo, "refs/tags/"+name+"^{commit}"); err == nil {
		if have == sha {
			return nil
		}
		return fmt.Errorf("tag %s is already on %s", name, have)
	}
	_, err := Run(ctx, repo, "tag", "-a", "-m", message, name, sha)
	return err
}

// PushTag pushes tag name to remote.
func PushTag(ctx context.Context, repo, remote, name string) error {
	_, err := Run(ctx, repo, "push", remote, "refs/tags/"+name)
	return err
}

func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " / ")
}
