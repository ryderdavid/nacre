package server

import (
	"net/http"

	"github.com/ryderdavid/nacre/bridge/internal/sessions"
)

// hookPayload is the part of a Claude Code hook's stdin JSON that POST
// /v1/sessions reads. The body may be the hook payload as Claude Code wrote it
// (unknown fields are ignored); `nacred hook` sends just these fields.
type hookPayload struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`
	HookEventName  string `json:"hook_event_name"`
	ToolName       string `json:"tool_name"`
	ToolInput      struct {
		Command string `json:"command"`
	} `json:"tool_input"`
}

// sessionsResponse says what became of a hint. It is informational: the hook
// ignores it.
type sessionsResponse struct {
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason,omitempty"`
}

// hintSession handles POST /v1/sessions: a Claude Code hook telling nacred
// about a session (NAC-j5z.10). It only nudges the session watcher, so
// adoption itself follows the same rules, and makes the same single run, as
// a poll would.
func (s *Server) hintSession(w http.ResponseWriter, r *http.Request) {
	var p hookPayload
	if err := decode(r, &p, false); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
		return
	}
	if !sessions.ValidSessionID(p.SessionID) {
		writeError(w, http.StatusBadRequest, "session_id is required", "")
		return
	}
	switch {
	case s.Sessions == nil:
		writeJSON(w, http.StatusAccepted, sessionsResponse{Reason: "adopt_sessions is off"})
		return
	case s.Runs != nil && s.Runs.OwnsSession(p.SessionID):
		writeJSON(w, http.StatusAccepted, sessionsResponse{Reason: "nacred runs this session itself"})
		return
	}
	h := sessions.Hint{SessionID: p.SessionID, TranscriptPath: p.TranscriptPath, CWD: p.CWD}
	if p.ToolName == "Bash" {
		h.Command = p.ToolInput.Command
	}
	s.Sessions.Nudge(h)
	writeJSON(w, http.StatusAccepted, sessionsResponse{Accepted: true})
}
