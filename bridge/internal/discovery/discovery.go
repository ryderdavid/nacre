// Package discovery finds beads repositories on disk: directories that contain
// a `.beads` directory. The repository root is the parent of `.beads`.
package discovery

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Options controls a scan. Paths may be absolute; callers expand "~" first
// (Exclude entries starting with "~/" are expanded with Home).
type Options struct {
	Roots []string
	Depth int // maximum directory depth below each root (root itself is depth 0)
	Home  string
	// Exclude entries without a path separator match directory basenames;
	// entries containing a separator are absolute (or ~/) paths whose whole
	// subtree is skipped.
	Exclude []string
	Extra   []string // always included when they contain .beads
	Hidden  []string // never included
}

// RepoID is the stable id of a repo: first 12 hex chars of sha256(abs root).
func RepoID(root string) string {
	sum := sha256.Sum256([]byte(root))
	return hex.EncodeToString(sum[:])[:12]
}

// matcher decides which directories are skipped while walking.
type matcher struct {
	names map[string]bool
	paths []string
}

func newMatcher(home string, exclude []string) matcher {
	m := matcher{names: map[string]bool{}}
	for _, e := range exclude {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if strings.HasPrefix(e, "~/") || e == "~" {
			e = filepath.Join(home, strings.TrimPrefix(e, "~"))
		}
		if strings.ContainsRune(e, filepath.Separator) {
			m.paths = append(m.paths, filepath.Clean(e))
		} else {
			m.names[e] = true
		}
	}
	return m
}

// skip reports whether the directory at path (basename name) must not be entered.
func (m matcher) skip(path, name string) bool {
	if m.names[name] {
		return true
	}
	for _, p := range m.paths {
		if path == p || strings.HasPrefix(path, p+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// IsRepo reports whether dir contains a .beads directory.
func IsRepo(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, ".beads"))
	return err == nil && fi.IsDir()
}

// Scan walks the roots and returns sorted, de-duplicated repo roots.
//
// Rules: hidden directories (".foo") are never descended into (this also keeps
// Claude Code worktrees under .claude/worktrees from showing up as duplicate
// repos); symlinks are not followed; the home directory itself (whose .beads
// is the home-level bd state) is never a repo unless listed in Extra; Hidden
// repos are removed last.
func Scan(o Options) []string {
	m := newMatcher(o.Home, o.Exclude)
	found := map[string]bool{}
	for _, root := range o.Roots {
		root = filepath.Clean(root)
		if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
			continue
		}
		walk(root, 0, o.Depth, m, found)
	}
	if o.Home != "" {
		delete(found, filepath.Clean(o.Home))
	}
	for _, e := range o.Extra {
		e = filepath.Clean(e)
		if IsRepo(e) {
			found[e] = true
		}
	}
	return Filter(found, o.Hidden)
}

// Filter returns the sorted keys of found minus the hidden paths.
func Filter(found map[string]bool, hidden []string) []string {
	hide := map[string]bool{}
	for _, h := range hidden {
		hide[filepath.Clean(h)] = true
	}
	out := make([]string, 0, len(found))
	for p := range found {
		if !hide[p] {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func walk(dir string, depth, maxDepth int, m matcher, found map[string]bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // permission denied, vanished, etc.
	}
	for _, e := range entries {
		if e.Name() == ".beads" && e.IsDir() {
			found[dir] = true
			break
		}
	}
	if depth >= maxDepth {
		return
	}
	for _, e := range entries {
		// DirEntry.IsDir is false for symlinks, so links are never followed.
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		child := filepath.Join(dir, name)
		if m.skip(child, name) {
			continue
		}
		walk(child, depth+1, maxDepth, m, found)
	}
}
