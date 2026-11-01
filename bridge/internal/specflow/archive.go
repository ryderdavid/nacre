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
	if c.Phase != "ready" || c.ApprovedRevision == "" || c.ApprovedRevision != c.Revision {
		return "", fmt.Errorf("%s isn't approved at its current revision (it is %s), so it can't be archived; review and approve it first", c.ChangeName, c.Phase)
	}
	if _, err := os.Stat(filepath.Join(workspace, "openspec", "changes", c.ChangeName)); os.IsNotExist(err) {
		return c.ChangeName, nil
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
	if _, err := gitx.CommitAll(ctx, workspace, "Archive OpenSpec change "+c.ChangeName+" (epic "+epic+")"); err != nil {
		return "", fmt.Errorf("committing the archive of %s: %w", c.ChangeName, err)
	}
	return c.ChangeName, nil
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
