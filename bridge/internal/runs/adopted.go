package runs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/sessions"
)

// Adopted runs mirror Claude Code sessions an agent started on its own, which
// nacred found in Claude Code's session registry (see internal/sessions). They
// deliberately bypass everything in this package that exists for processes
// nacred owns: they never enter the queue, never take a concurrency slot, are
// never started or signalled, and a bridge restart does not fail them. What
// they share with a dispatched run is the storage layout and the id scheme, so
// the store, prune and the API need no special case.

// OwnsSession reports whether sessionID belongs to a run nacred dispatched
// itself. The session watcher uses it to avoid adopting nacred's own sessions,
// which appear in the registry like any other.
func (m *Manager) OwnsSession(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.runs {
		r.mu.Lock()
		own := r.meta.SessionID == sessionID && !r.meta.Adopted()
		r.mu.Unlock()
		if own {
			return true
		}
	}
	return false
}

// SyncAdopted reconciles the adopted runs with live, the complete set of
// agent-owned sessions the watcher can currently see. A session with no run
// yet gets one; a session already adopted has its run updated; an adopted run
// whose session has gone is ended.
//
// Passing nil ends every adopted run, which is what a start with adoption
// disabled wants: runs left active by an earlier, adoption-enabled start would
// otherwise stay "running" forever.
func (m *Manager) SyncAdopted(live []sessions.Session) {
	seen := make(map[string]bool, len(live))
	var changes []api.Run

	m.mu.Lock()
	for _, s := range live {
		if s.SessionID == "" {
			continue
		}
		seen[s.SessionID] = true
		if id := m.adopted[s.SessionID]; id != "" {
			if r := m.runs[id]; r != nil {
				if snap, ok := m.refreshAdopted(r, s); ok {
					changes = append(changes, snap)
				}
				continue
			}
			delete(m.adopted, s.SessionID)
		}
		r, snap, err := m.newAdopted(s)
		if err != nil {
			m.opts.Logger.Printf("runs: adopt session %s: %v", s.SessionID, err)
			continue
		}
		m.runs[snap.ID] = r
		m.adopted[s.SessionID] = snap.ID
		changes = append(changes, snap)
	}
	for sid, id := range m.adopted {
		if seen[sid] {
			continue
		}
		r := m.runs[id]
		if r == nil {
			delete(m.adopted, sid)
			continue
		}
		if snap, ok := m.endAdopted(r); ok {
			changes = append(changes, snap)
		}
	}
	m.mu.Unlock()

	for _, snap := range changes {
		m.notify(snap)
	}
	m.prune()
}

// newAdopted creates the run that mirrors s. The id is a normal sortable run
// id: listRunDirs only accepts the "r_" prefix and both List and prune rely on
// ids sorting by time, so an adopted run must not be marked out by its id.
// Caller holds m.mu.
func (m *Manager) newAdopted(s sessions.Session) (*run, api.Run, error) {
	id := newID(time.Now())
	started := s.Started()
	if started.IsZero() {
		started = time.Now()
	}
	ts := started.UTC().Format(time.RFC3339)
	meta := api.Run{
		ID: id, Origin: api.OriginAdopted,
		RepoID: s.Repo.ID, RepoName: s.Repo.Name, RepoPath: s.Repo.Path,
		IssueID: s.IssueID, IssueTitle: s.IssueTitle,
		Status: api.StatusRunning, Mode: "", Model: "",
		SessionID: s.SessionID, SessionCWD: s.CWD,
		CreatedAt: ts, StartedAt: ts, Turns: 1,
	}
	r := newRun(meta, filepath.Join(m.opts.Dir, id))
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return nil, api.Run{}, err
	}
	r.loaded = true
	r.mu.Lock()
	defer r.mu.Unlock()
	r.changed() // fills in ResumeCommand ("take over on the Mac")
	r.addEvent(api.RunEvent{Kind: api.KindSystem, Text: adoptedNotice(s)})
	r.addEvent(api.RunEvent{Kind: api.KindStatus, Status: api.StatusRunning})
	if err := r.save(); err != nil {
		return nil, api.Run{}, err
	}
	return r, r.meta, nil
}

// refreshAdopted folds a fresh registry reading into an existing adopted run:
// a bead link that has only now resolved, and a session that came back after
// being reported gone. Caller holds m.mu.
func (m *Manager) refreshAdopted(r *run, s sessions.Session) (api.Run, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	changed := false
	if s.IssueID != "" && s.IssueID != r.meta.IssueID {
		r.meta.IssueID, r.meta.IssueTitle = s.IssueID, s.IssueTitle
		r.addEvent(api.RunEvent{Kind: api.KindSystem, Text: "linked to " + s.IssueID + " (" + s.LinkedBy + ")"})
		changed = true
	}
	if s.CWD != "" && s.CWD != r.meta.SessionCWD {
		r.meta.SessionCWD = s.CWD
		changed = true
	}
	if !r.meta.Active() {
		// The pid was gone for a poll (or the bridge restarted while it was)
		// and is back: the session outlived the report of its ending.
		r.meta.EndedAt, r.meta.Error = "", ""
		r.setStatus(api.StatusRunning)
		changed = true
	}
	if !changed {
		return api.Run{}, false
	}
	r.changed()
	_ = r.save()
	return r.meta, true
}

// endAdopted records that an adopted session's process is gone. The outcome is
// unknowable — nobody reports a result for a session nacred did not run — so
// the run ends without an error rather than being called a failure. Caller
// holds m.mu.
func (m *Manager) endAdopted(r *run) (api.Run, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.meta.Active() {
		return api.Run{}, false
	}
	r.ensureLoaded()
	r.meta.EndedAt = now()
	r.addEvent(api.RunEvent{Kind: api.KindSystem, Text: "session ended: the Claude Code process is no longer running"})
	r.setStatus(api.StatusSucceeded)
	_ = r.save()
	return r.meta, true
}

// adoptedNotice is the first event of an adopted run: what was found, where,
// and which signal linked it to a bead.
func adoptedNotice(s sessions.Session) string {
	var b strings.Builder
	fmt.Fprintf(&b, "adopted Claude Code session %s (pid %d", s.SessionID, s.PID)
	if s.Version != "" {
		fmt.Fprintf(&b, ", claude %s", s.Version)
	}
	if s.Entrypoint != "" {
		fmt.Fprintf(&b, ", %s", s.Entrypoint)
	}
	fmt.Fprintf(&b, ") in %s", s.CWD)
	if s.Name != "" {
		fmt.Fprintf(&b, "\nsession name: %s", s.Name)
	}
	if s.IssueID != "" {
		fmt.Fprintf(&b, "\nlinked to %s (%s)", s.IssueID, s.LinkedBy)
	} else {
		b.WriteString("\nno bead linked: this session has not moved one")
	}
	b.WriteString("\nnacred did not start this session: it is read-only here, and cannot be cancelled or sent a follow-up.")
	return b.String()
}
