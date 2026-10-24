// Package api defines the JSON wire types of the nacred HTTP API (see docs/API.md).
// It has no dependencies on the rest of the bridge so every layer can share it.
package api

import "encoding/json"

// Run statuses.
const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// RunEvent kinds.
const (
	KindUser       = "user"
	KindSystem     = "system"
	KindText       = "text"
	KindThinking   = "thinking"
	KindToolUse    = "tool_use"
	KindToolResult = "tool_result"
	KindResult     = "result"
	KindStderr     = "stderr"
	KindStatus     = "status"
)

// Run origins.
const (
	// OriginNacre marks a run nacred dispatched and owns the process of. It
	// is the default, and is omitted from the wire.
	OriginNacre = "nacre"
	// OriginAdopted marks a Claude Code session an agent started on its own,
	// which nacred found in Claude Code's session registry and only observes.
	// nacred owns no process for it, so it cannot be cancelled and cannot
	// take a follow-up.
	OriginAdopted = "adopted"
)

// Agents that can own a run's session. Agent is omitted from the wire for a
// run nacred dispatched, which is always Claude Code; it is carried on an
// adopted run so clients can tell agents apart once nacred learns to adopt
// more than one.
const (
	AgentClaudeCode = "claude-code"
)

// Activities are the finer-grained state of an adopted session, reported
// beside Status rather than inside it: clients key on the five statuses, so a
// session that is alive but parked is still StatusRunning and says so here.
// Omitted for a run nacred dispatched, whose status is exact.
const (
	ActivityWorking = "working" // alive and working
	ActivityIdle    = "idle"    // alive but parked waiting for input
	ActivityEnded   = "ended"   // the owning process is gone
)

// Modes is the set of permission modes accepted for runs (claude --permission-mode).
var Modes = []string{"auto", "acceptEdits", "plan", "bypassPermissions", "dontAsk"}

// ValidMode reports whether m is an accepted permission mode.
func ValidMode(m string) bool {
	for _, x := range Modes {
		if x == m {
			return true
		}
	}
	return false
}

// Repo is a discovered beads repository plus summary counts.
type Repo struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Path        string         `json:"path"`
	Prefix      string         `json:"prefix"`
	Counts      map[string]int `json:"counts"`
	LastUpdated string         `json:"last_updated"`
	ActiveRuns  int            `json:"active_runs"`
	// AdoptedRuns counts the live adopted sessions nacred is watching in this
	// repo — agent-owned work it does not run. ActiveRuns deliberately
	// excludes them so it keeps meaning "runs nacred is doing". Omitted when
	// zero, so a bridge with adoption off answers exactly as it did before.
	AdoptedRuns int    `json:"adopted_runs,omitempty"`
	Error       string `json:"error"`
	// GitRemote is the normalized origin URL ("github.com/owner/repo"), used by
	// clients to recognise the same repository on several computers.
	GitRemote string `json:"git_remote"`
	GitBranch string `json:"git_branch"`
}

// Host identifies the computer a nacred instance runs on.
type Host struct {
	ID   string `json:"host_id"`
	Name string `json:"host_name"`
}

// Issue is a bd issue object passed through verbatim from `bd list --json`.
type Issue = json.RawMessage

// Comment is a bd comment.
type Comment struct {
	ID        string `json:"id"`
	IssueID   string `json:"issue_id"`
	Author    string `json:"author"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

// Run is a Claude Code run dispatched for one bead.
type Run struct {
	ID            string  `json:"id"`
	RepoID        string  `json:"repo_id"`
	RepoName      string  `json:"repo_name"`
	RepoPath      string  `json:"repo_path"`
	IssueID       string  `json:"issue_id"`
	IssueTitle    string  `json:"issue_title"`
	Status        string  `json:"status"`
	Mode          string  `json:"mode"`
	Model         string  `json:"model"`
	Worktree      bool    `json:"worktree"`
	SessionID     string  `json:"session_id"`
	CreatedAt     string  `json:"created_at"`
	StartedAt     string  `json:"started_at"`
	EndedAt       string  `json:"ended_at"`
	CostUSD       float64 `json:"cost_usd"`
	DurationMS    int64   `json:"duration_ms"`
	NumTurns      int     `json:"num_turns"`
	Turns         int     `json:"turns"`
	ToolCalls     int     `json:"tool_calls"`
	LastText      string  `json:"last_text"`
	ResultText    string  `json:"result_text"`
	Error         string  `json:"error"`
	ResumeCommand string  `json:"resume_command"`
	// SessionCWD is the working directory Claude reported at session init
	// (differs from RepoPath for worktree runs). Extension to API.md.
	SessionCWD string `json:"session_cwd,omitempty"`
	// Origin is OriginAdopted for an agent-owned session nacred only watches.
	// Empty (and absent from the wire) means OriginNacre.
	Origin string `json:"origin,omitempty"`
	// Agent names the tool that owns the session (AgentClaudeCode). Absent
	// means Claude Code, the only agent nacred dispatches.
	Agent string `json:"agent,omitempty"`
	// SessionName is the name the owning agent gave the session, as Claude
	// Code recorded it. It is carried on adopted runs, where it is the only
	// human label for a session nacred did not name itself.
	SessionName string `json:"session_name,omitempty"`
	// OwnerPID is the process that owns the session. It is set only on an
	// adopted run, where that process is not nacred's child — it is exactly
	// the thing no client may be invited to signal.
	OwnerPID int `json:"owner_pid,omitempty"`
	// Activity is one of the Activity* values on an adopted run, "" on a run
	// nacred dispatched.
	Activity string `json:"activity,omitempty"`
	// Stale marks an adopted run whose session is alive but has not written
	// its transcript within the adoption idle window (adopt_idle_minutes): a
	// session left open rather than one doing work. A stale run is left out
	// of GET /v1/runs?include=adopted unless include also names "stale", and
	// is not counted by Repo.AdoptedRuns. Never set on a dispatched run.
	Stale bool `json:"stale,omitempty"`
	// LastActivityAt is the timestamp of the newest transcript entry nacred
	// has read for an adopted run, as Claude Code wrote it; absent until the
	// session has written one.
	LastActivityAt string `json:"last_activity_at,omitempty"`
	// CanCancel and CanFollowup report whether POST /v1/runs/{run}/cancel and
	// .../followup are available for this run at all. See SetCapabilities for
	// why they are pointers.
	CanCancel   *bool `json:"can_cancel,omitempty"`
	CanFollowup *bool `json:"can_followup,omitempty"`
}

// Active reports whether the run is queued or running.
func (r *Run) Active() bool { return r.Status == StatusQueued || r.Status == StatusRunning }

// Adopted reports whether the run mirrors an agent-owned Claude Code session
// rather than a process nacred started.
func (r *Run) Adopted() bool { return r.Origin == OriginAdopted }

// SetCapabilities derives CanCancel and CanFollowup from Origin.
//
// For a run nacred dispatched both are left nil, which keeps them off the
// wire. Absence means "no restriction beyond the usual status rules": cancel
// on a finished run is still a no-op 200, a follow-up to an active run is
// still a 409. Leaving them out is what makes a Run that nacred dispatched
// byte-for-byte what it was before adopted runs existed, so a client built
// against the older API is not handed fields it would have to understand.
//
// For an adopted run both are false: nacred owns no process to signal, and
// resuming the session would fork the agent's work rather than continue it.
// Both endpoints answer 409 for such a run, and these flags are how a client
// knows that without having to try.
func (r *Run) SetCapabilities() {
	if !r.Adopted() {
		r.CanCancel, r.CanFollowup = nil, nil
		return
	}
	cancel, followup := false, false
	r.CanCancel, r.CanFollowup = &cancel, &followup
}

// RunEvent is one entry in a run's transcript.
type RunEvent struct {
	Seq     int64           `json:"seq"`
	TS      string          `json:"ts"`
	Kind    string          `json:"kind"`
	Text    string          `json:"text,omitempty"`
	Tool    string          `json:"tool,omitempty"`
	ToolID  string          `json:"tool_id,omitempty"`
	Input   json.RawMessage `json:"input,omitempty"`
	IsError *bool           `json:"is_error,omitempty"`
	Status  string          `json:"status,omitempty"`
}

// Bool returns a pointer to b, for RunEvent.IsError.
func Bool(b bool) *bool { return &b }

// Error is the body of every non-2xx response.
type Error struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"`
}
