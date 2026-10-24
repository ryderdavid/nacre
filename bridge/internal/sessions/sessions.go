// Package sessions watches Claude Code's on-disk session registry
// (~/.claude/sessions/<pid>.json) so nacred can adopt Claude Code sessions an
// agent started on its own, with no cooperation from that agent.
//
// A record is only adopted when its process is genuinely alive (pid liveness
// plus a process-start-time match, so a reused pid cannot resurrect a dead
// session), when its working directory maps to a discovered beads repository,
// and when it is not a session nacred launched itself. Each adopted session is
// linked to the bead the agent moved, trying three signals in order: the
// session name, the bd commands the agent actually ran (read out of the
// session transcript), then bd's own claim state.
//
// The package only observes: it never signals a session, never writes to a
// transcript and never execs claude. Each poll also tails every adopted
// session's transcript through claude.TranscriptTail, so a Session carries the
// run events and running totals the agent's transcript gained since the last
// poll; turning those into an adopted run is the run manager's job.
package sessions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// Link signals, in increasing order of strength. Session.LinkedBy reports
// which one produced the bead link.
const (
	// LinkName comes from the session's --name carrying a bead id.
	LinkName = "name"
	// LinkTranscript comes from a bd command found in the session transcript:
	// the agent literally moved that bead from this session.
	LinkTranscript = "transcript"
	// LinkBdState comes from bd alone: a bead in the session's repo that is
	// in progress and was started after the session was.
	LinkBdState = "bd_state"
)

// procStartLayout is how Claude Code formats a process start time. It is
// written in UTC, unlike ps(1), which prints local time.
const procStartLayout = "Mon Jan _2 15:04:05 2006"

// Record is one ~/.claude/sessions/<pid>.json entry. Claude Code writes more
// fields than these; the ones below are the ones nacred relies on.
type Record struct {
	PID        int    `json:"pid"`
	SessionID  string `json:"sessionId"`
	CWD        string `json:"cwd"`
	StartedAt  int64  `json:"startedAt"` // epoch milliseconds
	ProcStart  string `json:"procStart"` // process start time, formatted in UTC
	Version    string `json:"version"`   // Claude Code version
	Kind       string `json:"kind"`      // "interactive"
	Entrypoint string `json:"entrypoint"`
	Name       string `json:"name"`
	NameSource string `json:"nameSource"` // "user" when set with --name
	Status     string `json:"status"`     // "idle" when parked, else absent
	UpdatedAt  int64  `json:"updatedAt"`  // epoch milliseconds

	// File is where the record was read from. It is not part of the JSON.
	File string `json:"-"`
}

// Started returns when the session began, or the zero time when unknown.
func (r Record) Started() time.Time { return fromMillis(r.StartedAt) }

// Updated returns when Claude Code last touched the record.
func (r Record) Updated() time.Time { return fromMillis(r.UpdatedAt) }

// ProcStarted returns the process start time the record claims, or the zero
// time when it is missing or unparseable.
func (r Record) ProcStarted() time.Time {
	t, err := time.ParseInLocation(procStartLayout, strings.TrimSpace(r.ProcStart), time.UTC)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Idle reports whether Claude Code has parked the session waiting for input.
func (r Record) Idle() bool { return r.Status == "idle" }

func fromMillis(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

// Repo identifies a discovered beads repository. The fields match repos.Ref so
// callers can convert between the two directly.
type Repo struct {
	ID   string
	Name string
	Path string
}

// Bead is the subset of a bd issue the bead linker needs.
type Bead struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Assignee  string `json:"assignee"`
	StartedAt string `json:"started_at"`
}

// Started returns when the bead was claimed, or the zero time when it has not
// been.
func (b Bead) Started() time.Time {
	t, err := time.Parse(time.RFC3339, b.StartedAt)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// ParseBeads extracts the linker's fields from raw `bd list --json` objects.
// Objects that do not parse, or that carry no id, are dropped.
func ParseBeads(raw []json.RawMessage) []Bead {
	out := make([]Bead, 0, len(raw))
	for _, r := range raw {
		var b Bead
		if json.Unmarshal(r, &b) == nil && b.ID != "" {
			out = append(out, b)
		}
	}
	return out
}

// Session is a live Claude Code session nacred has adopted: a registry record
// that passed the liveness check and resolved to a discovered repo.
type Session struct {
	Record

	// Repo is the discovered repository the session is working in. For a
	// session inside a git worktree this is the main checkout, which is where
	// the beads database lives.
	Repo Repo
	// IssueID is the bead the agent is working on, "" when no signal resolved
	// one. A session with no bead is still reported, as repo activity; a link
	// is never guessed at.
	IssueID string
	// IssueTitle is the bead's title, when bd knew it.
	IssueTitle string
	// LinkedBy names the signal that produced IssueID (LinkName,
	// LinkTranscript or LinkBdState), "" when IssueID is empty.
	LinkedBy string
	// Transcript is the session's on-disk transcript, "" when Claude Code has
	// not written one yet.
	Transcript string
	// LastWrite is when the transcript last changed (its mtime), zero when
	// there is no transcript yet. It is the freshness signal that tells a
	// session doing something from one left open and quiet.
	LastWrite time.Time
	// Events are the run events the transcript gained since the previous
	// poll, oldest first, with the transcript's own timestamps.
	Events []api.RunEvent
	// FromStart reports that Events were read from the start of the
	// transcript rather than from where the previous poll left off: the first
	// poll after a (re)start of nacred, or a transcript that was replaced. A
	// consumer that already holds earlier events must expect to see them again.
	FromStart bool
	// TranscriptState is everything the transcript has said so far: cost,
	// duration, tool calls, turns, model and the last assistant text.
	TranscriptState claude.TranscriptState
}

// LastActive returns the latest sign of life the session has given: a
// transcript write or entry, or failing both, the session's own start. It is
// zero only for a session that has neither.
func (s Session) LastActive() time.Time {
	last := s.LastWrite
	if a := s.TranscriptState.LastActivity; a.After(last) {
		last = a
	}
	if last.IsZero() {
		last = s.Started()
	}
	return last
}

// DefaultDir returns Claude Code's session registry directory.
func DefaultDir(home string) string { return filepath.Join(home, ".claude", "sessions") }

// DefaultProjectsDir returns where Claude Code keeps session transcripts.
func DefaultProjectsDir(home string) string { return filepath.Join(home, ".claude", "projects") }

// Read parses one session record.
func Read(path string) (Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, err
	}
	var rec Record
	if err := json.Unmarshal(data, &rec); err != nil {
		return Record{}, err
	}
	rec.File = path
	return rec, nil
}

// ReadDir returns every parseable record in dir (normally ~/.claude/sessions),
// ordered by pid. A missing directory yields no records and no error.
//
// Unreadable and unparseable files are skipped rather than reported: the
// directory also holds per-session .key files, and Claude Code rewrites a
// record in place, so a read can legitimately catch a partial write.
func ReadDir(dir string) ([]Record, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		rec, err := Read(filepath.Join(dir, e.Name()))
		if err != nil || rec.PID <= 0 || rec.SessionID == "" {
			continue
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	return out, nil
}
