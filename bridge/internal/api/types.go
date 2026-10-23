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
	Error       string         `json:"error"`
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
}

// Active reports whether the run is queued or running.
func (r *Run) Active() bool { return r.Status == StatusQueued || r.Status == StatusRunning }

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
