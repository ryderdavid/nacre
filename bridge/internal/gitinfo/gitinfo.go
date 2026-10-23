// Package gitinfo reads the git facts nacred reports per repo: the origin
// remote (normalized so the same repository matches across computers) and the
// current branch.
package gitinfo

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// Info is what a repo reports about its git checkout. Empty fields mean unknown.
type Info struct {
	Remote string // normalized origin URL, e.g. "github.com/ryderdavid/nacre"
	Branch string // current branch, "" when detached or not a git repo
}

// Read returns git facts for the checkout at dir. It never fails: a missing git
// binary, a non-git directory or a missing remote all yield empty fields.
func Read(ctx context.Context, dir string) Info {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	remote := run(ctx, dir, "config", "--get", "remote.origin.url")
	branch := run(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if branch == "HEAD" {
		branch = ""
	}
	return Info{Remote: resolveAlias(ctx, Normalize(remote), remote), Branch: branch}
}

// resolveAlias swaps an SSH host alias ("github.com-work", set up in
// ~/.ssh/config for a second account) for the real host it points at, so the
// same repository matches a computer that clones it under the plain host name.
func resolveAlias(ctx context.Context, normalized, raw string) string {
	if normalized == "" || strings.Contains(raw, "://") && !strings.HasPrefix(raw, "ssh://") {
		return normalized
	}
	host, path, _ := strings.Cut(normalized, "/")
	out, err := exec.CommandContext(ctx, "ssh", "-G", host).Output()
	if err != nil {
		return normalized
	}
	for _, line := range strings.Split(string(out), "\n") {
		if real, ok := strings.CutPrefix(line, "hostname "); ok {
			if real = strings.ToLower(strings.TrimSpace(real)); real != "" {
				return real + "/" + path
			}
		}
	}
	return normalized
}

func run(ctx context.Context, dir string, args ...string) string {
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Normalize turns the many spellings of a remote URL into "host/path" so that
// SSH and HTTPS clones of one repository compare equal:
//
//	git@github.com:Owner/Repo.git        → github.com/owner/repo
//	https://user@github.com/Owner/Repo   → github.com/owner/repo
//	ssh://git@host:2222/team/repo.git    → host/team/repo
//
// Local paths and file:// URLs differ per computer, so they normalize to "".
func Normalize(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || strings.HasPrefix(s, "/") || strings.HasPrefix(s, "file:") || strings.HasPrefix(s, ".") {
		return ""
	}
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	} else if at, colon := strings.Index(s, "@"), strings.Index(s, ":"); colon > 0 && (at < 0 || at < colon) {
		// scp-like syntax: [user@]host:path
		s = s[:colon] + "/" + s[colon+1:]
	}
	if i := strings.Index(s, "@"); i >= 0 && i < strings.Index(s+"/", "/") {
		s = s[i+1:]
	}
	host, path, _ := strings.Cut(s, "/")
	if h, _, ok := strings.Cut(host, ":"); ok { // drop a port
		host = h
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if host == "" || path == "" {
		return ""
	}
	return strings.ToLower(host + "/" + path)
}
