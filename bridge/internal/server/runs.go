package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/prompt"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
	"github.com/ryderdavid/nacre/bridge/internal/sse"
)

// runRequest is the RunRequest body.
type runRequest struct {
	Mode         string `json:"mode"`
	Model        string `json:"model"`
	Instructions string `json:"instructions"`
	Worktree     bool   `json:"worktree"`
	Prompt       string `json:"prompt"`
	// Agent is "codex" to run the bead with the Codex CLI; "" or
	// "claude-code" runs it with Claude Code.
	Agent string `json:"agent"`
}

func (s *Server) startRun(w http.ResponseWriter, r *http.Request) {
	repoID := r.PathValue("repo")
	path, ok := s.repoPath(w, r)
	if !ok {
		return
	}
	id, ok := issueID(w, r)
	if !ok {
		return
	}
	var req runRequest
	if err := decode(r, &req, true); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
		return
	}
	if req.Mode == "" {
		req.Mode = s.Config.DefaultMode
	}
	if !api.ValidMode(req.Mode) {
		writeError(w, http.StatusBadRequest, "invalid mode", "one of: "+strings.Join(api.Modes, ", "))
		return
	}
	switch req.Agent {
	case "", api.AgentClaudeCode:
		req.Agent = ""
		if req.Model == "" {
			req.Model = s.Config.DefaultModel
		}
	case api.AgentCodex:
		// default_model names a Claude model; Codex keeps its own default.
		if st := s.codexStatus(r.Context()); !st.Available() {
			writeError(w, http.StatusConflict, "codex unavailable", st.Reason)
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "invalid agent", "one of: "+api.AgentClaudeCode+", "+api.AgentCodex)
		return
	}
	if strings.HasPrefix(req.Model, "-") {
		writeError(w, http.StatusBadRequest, "invalid model", "")
		return
	}
	raw, err := s.BD.Get(r.Context(), path, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	var issue struct {
		Title string `json:"title"`
	}
	_ = json.Unmarshal(raw, &issue)
	repo, _ := s.Repos.Summary(repoID)

	tmpl := s.Config.PromptTemplate
	if strings.TrimSpace(req.Prompt) != "" {
		tmpl = req.Prompt
	}
	text := prompt.Render(tmpl, prompt.Vars{ID: id, Title: issue.Title, Repo: repo.Name, Instructions: req.Instructions})

	run, err := s.Runs.Start(runs.StartRequest{
		RepoID: repoID, RepoName: repo.Name, RepoPath: path,
		IssueID: id, IssueTitle: issue.Title,
		Mode: req.Mode, Model: req.Model, Worktree: req.Worktree, Prompt: text, Agent: req.Agent,
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	list := s.Runs.List(runs.Filter{
		RepoID: q.Get("repo"), IssueID: q.Get("issue"), ActiveOnly: q.Get("active") == "1",
		IncludeAdopted: includes(q.Get("include"), "adopted"),
		IncludeStale:   includes(q.Get("include"), "stale"),
	})
	writeJSON(w, http.StatusOK, map[string]any{"runs": list})
}

// includes reports whether a comma-separated ?include= list names want.
// Adopted runs are opt-in: a client that does not ask for them must not be
// handed sessions it would offer to cancel but nacred cannot control. Stale
// adopted runs, sessions left open and quiet, are a second opt-in ("stale").
func includes(list, want string) bool {
	for _, v := range strings.Split(list, ",") {
		if strings.TrimSpace(v) == want {
			return true
		}
	}
	return false
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	run, events, err := s.Runs.Get(r.PathValue("run"))
	if err != nil {
		s.fail(w, err)
		return
	}
	if events == nil {
		events = []api.RunEvent{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run, "events": events})
}

func (s *Server) cancelRun(w http.ResponseWriter, r *http.Request) {
	run, err := s.Runs.Cancel(r.PathValue("run"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) followupRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message string `json:"message"`
		Mode    string `json:"mode"`
	}
	if err := decode(r, &req, false); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeError(w, http.StatusBadRequest, "message is required", "")
		return
	}
	if req.Mode != "" && !api.ValidMode(req.Mode) {
		writeError(w, http.StatusBadRequest, "invalid mode", "one of: "+strings.Join(api.Modes, ", "))
		return
	}
	run, err := s.Runs.Followup(r.PathValue("run"), req.Message, req.Mode)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// streamRun replays stored events after the client's position, then streams
// live events and run metadata changes until the client disconnects.
func (s *Server) streamRun(w http.ResponseWriter, r *http.Request) {
	sub, err := s.Runs.Subscribe(r.PathValue("run"))
	if err != nil {
		s.fail(w, err)
		return
	}
	defer sub.Close()

	after := int64(0)
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		after, _ = strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	} else if v := r.URL.Query().Get("after"); v != "" {
		after, _ = strconv.ParseInt(v, 10, 64)
	}

	sw, err := sse.NewWriter(w)
	if err != nil {
		return
	}
	sendEvents := func() bool {
		for _, ev := range sub.EventsAfter(after) {
			if sw.Send(strconv.FormatInt(ev.Seq, 10), "event", ev) != nil {
				return false
			}
			after = ev.Seq
		}
		return true
	}
	var metaVer uint64
	sendMeta := func(force bool) bool {
		meta, ver := sub.Meta()
		if !force && ver == metaVer {
			return true
		}
		metaVer = ver
		return sw.Send("", "run", meta) == nil
	}
	if !sendEvents() || !sendMeta(true) {
		return
	}
	ping := time.NewTicker(sse.KeepAlive)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if sw.Ping() != nil {
				return
			}
		case <-sub.C:
			if !sendEvents() || !sendMeta(false) {
				return
			}
		}
	}
}
