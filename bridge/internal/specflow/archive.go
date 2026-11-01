package specflow

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/gitx"
)

// ArchiveInto archives the change whose epic is being finished, in the
// finishing round's workspace, and commits it there, so the archive lands
// with the epic's work. It returns the change name, or "" when the epic
// belongs to no change. Archiving an already archived change does nothing.
func (m *Manager) ArchiveInto(ctx context.Context, repo, workspace, epic string) (string, error) {
	repo, e := canonicalRepo(repo)
	if e != nil {
		return "", e
	}
	m.mu.Lock()
	var r *record
	for _, rec := range m.records {
		if rec.Repo == repo && rec.Change.EpicID == epic {
			r = clone(rec)
		}
	}
	m.mu.Unlock()
	if r == nil {
		return "", nil
	}
	c := r.Change
	if c.Phase == "archived" {
		return c.ChangeName, nil
	}
	if _, err := os.Stat(filepath.Join(workspace, "openspec", "changes", c.ChangeName)); os.IsNotExist(err) {
		return c.ChangeName, nil
	}
	// What's archived must be what was approved; other specifications moving
	// on since then doesn't call for another review of finished work.
	if !asApproved(c, workspace) {
		return "", fmt.Errorf("%s's files changed since its approved revision, so archiving it needs another review and approval first", c.ChangeName)
	}
	binary, err := exec.LookPath("openspec")
	if err != nil {
		return "", fmt.Errorf("install the OpenSpec CLI on this computer to archive %s", c.ChangeName)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "archive", c.ChangeName, "--yes")
	cmd.Dir = workspace
	if out, err := cmd.CombinedOutput(); err != nil {
		s := strings.TrimSpace(string(out))
		if len(s) > 4000 {
			s = s[:4000]
		}
		return "", fmt.Errorf("openspec archive %s failed: %s", c.ChangeName, s)
	}
	if err := fillPurposes(workspace, c); err != nil {
		return "", err
	}
	if _, err := gitx.CommitAll(ctx, workspace, "Archive OpenSpec change "+c.ChangeName+" (epic "+epic+")"); err != nil {
		return "", fmt.Errorf("committing the archive of %s: %w", c.ChangeName, err)
	}
	return c.ChangeName, nil
}

// fillPurposes replaces the placeholder Purpose openspec archive writes into
// each spec it creates with the change's own "Why", since strict validation,
// one of the gates, rejects the placeholder.
func fillPurposes(workspace string, c Change) error {
	why := ""
	for _, a := range c.Artifacts {
		if strings.HasSuffix(a.Path, "/proposal.md") {
			why = oneParagraph(whyText(a.Content), 400)
		}
	}
	if why == "" {
		why = "Specified by the change " + c.ChangeName + "."
	}
	placeholder := "TBD - created by archiving change " + c.ChangeName + "."
	specs, _ := filepath.Glob(filepath.Join(workspace, "openspec", "specs", "*", "spec.md"))
	for _, p := range specs {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		lines := strings.Split(string(b), "\n")
		changed := false
		for i, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), placeholder) {
				lines[i], changed = why, true
			}
		}
		if changed {
			if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// asApproved reports whether an approved change's own canonical files under
// root are exactly its approved artifacts, ignoring section markers (an
// adopted change keeps them in the workflow only).
func asApproved(c Change, root string) bool {
	if c.ApprovedRevision == "" || c.ApprovedRevision != c.Revision || (c.Phase != "ready" && c.Phase != "stale") || c.ChangeName == "" {
		return false
	}
	inv, err := collect(root)
	if err != nil {
		return false
	}
	files := changeFiles(inv, c.ChangeName)
	if len(files) != len(c.Artifacts) {
		return false
	}
	want := map[string]string{}
	for _, a := range c.Artifacts {
		want[a.Path] = withoutMarkers(a.Content)
	}
	for _, f := range files {
		if w, ok := want[f.Path]; !ok || w != withoutMarkers(f.Content) {
			return false
		}
	}
	return true
}

func withoutMarkers(s string) string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if markerPattern.MatchString(strings.TrimSpace(l)) {
			continue
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// ChangeOf names the change an epic belongs to while it is still to be
// archived, or "".
func (m *Manager) ChangeOf(repo, epic string) string {
	repo, e := canonicalRepo(repo)
	if e != nil {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.records {
		if r.Repo == repo && r.Change.EpicID == epic && r.Change.Phase != "archived" {
			return r.Change.ChangeName
		}
	}
	return ""
}

// MarkArchived records that the change of a finished epic was archived and
// merged, ending its workflow.
func (m *Manager) MarkArchived(repo, epic string) {
	repo, e := canonicalRepo(repo)
	if e != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, rec := range m.records {
		if rec.Repo != repo || rec.Change.EpicID != epic || rec.Change.Phase == "archived" {
			continue
		}
		r := clone(rec)
		r.Change.Phase = "archived"
		r.Stage = "archived"
		r.Change.Error, r.Change.StaleReason = "", ""
		history(r, "archived", "Archived and merged into the main branch with epic "+epic+"; the workflow is complete.")
		if m.saveLocked(r) == nil && m.opts.Notify != nil {
			m.opts.Notify(r.Repo)
		}
	}
}
