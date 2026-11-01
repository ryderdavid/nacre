package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/assist"
	"github.com/ryderdavid/nacre/bridge/internal/attention"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
	"github.com/ryderdavid/nacre/bridge/internal/sse"
)

// Limits on an assist request. The prompts the app builds are a few thousand
// characters; these only stop something pathological.
const (
	maxAssistPrompt = 120_000
	maxAssistSystem = 40_000
	maxAssistSchema = 40_000
)

// assistRequest asks this computer to answer one prompt with `claude -p`.
//
// The app owns the prompt, the system prompt and the JSON schema, so the
// answer parses against exactly the same types as its Claude API path and the
// two backends cannot drift apart. The bridge only decides where the call runs
// and what it is allowed to touch.
type assistRequest struct {
	Prompt string          `json:"prompt"`
	System string          `json:"system"`
	Schema json.RawMessage `json:"schema"`
	Model  string          `json:"model"`
	Effort string          `json:"effort"`
	// SessionID continues an earlier assistant conversation.
	SessionID string `json:"session_id"`
	// BdReads lets the call run read-only bd commands in the repo. Without it
	// the call gets no tools at all.
	BdReads bool `json:"bd_reads"`
	// ReadRepo also lets the call read the repository: files, search and
	// read-only git. It can still write nothing.
	ReadRepo bool `json:"read_repo"`
	// Keep saves the session so the next turn can resume it.
	Keep bool `json:"keep"`
	// NeedsYou adds this computer's Needs you list to the prompt, for the
	// board assistant, which can't call a read tool on this backend.
	NeedsYou bool `json:"needs_you"`
}

var effortLevels = map[string]bool{"low": true, "medium": true, "high": true, "xhigh": true, "max": true}

// headless validates the request and turns it into a claude invocation.
func (req assistRequest) headless() (claude.Headless, string) {
	switch {
	case strings.TrimSpace(req.Prompt) == "":
		return claude.Headless{}, "prompt required"
	case len(req.Prompt) > maxAssistPrompt:
		return claude.Headless{}, "prompt too long"
	case len(req.System) > maxAssistSystem:
		return claude.Headless{}, "system prompt too long"
	case len(req.Schema) > maxAssistSchema:
		return claude.Headless{}, "schema too long"
	case len(req.Schema) > 0 && !json.Valid(req.Schema):
		return claude.Headless{}, "schema is not valid JSON"
	case req.Effort != "" && !effortLevels[req.Effort]:
		return claude.Headless{}, "unknown effort level"
	case !safeFlagValue(req.Model):
		return claude.Headless{}, "invalid model"
	case !safeFlagValue(req.SessionID):
		return claude.Headless{}, "invalid session id"
	}
	h := claude.Headless{
		Prompt: req.Prompt, System: req.System, Schema: req.Schema,
		Model: req.Model, Effort: req.Effort, Resume: req.SessionID, Persist: req.Keep,
	}
	if req.BdReads {
		h.AllowedTools = claude.BdReadTools
	}
	h.ReadRepo = req.ReadRepo
	return h, ""
}

// safeFlagValue rejects values that would be read as another flag.
func safeFlagValue(v string) bool {
	return !strings.HasPrefix(v, "-") && !strings.ContainsAny(v, " \t\n")
}

// assistJSON answers one prompt and returns the structured result. This is how
// Refine and Draft run on the Mac.
func (s *Server) assistJSON(w http.ResponseWriter, r *http.Request) {
	path, ok := s.repoPath(w, r)
	if !ok {
		return
	}
	var req assistRequest
	if err := decode(r, &req, false); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
		return
	}
	h, bad := req.headless()
	if bad == "" && req.NeedsYou && s.Attention != nil {
		h.Prompt += attention.Context(s.Attention.Items(r.Context()))
	}
	if bad != "" {
		writeError(w, http.StatusBadRequest, bad, "")
		return
	}
	out, err := s.Assist.Run(r.Context(), path, h, nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error(), "")
		return
	}
	writeJSON(w, http.StatusOK, assistResult{SessionID: out.SessionID, Output: out.Output, Text: out.Text, CostUSD: out.CostUSD})
}

// assistResult is the body of a finished assist call, and the payload of the
// terminal "done" event when it streamed.
type assistResult struct {
	SessionID string          `json:"session_id,omitempty"`
	Output    json.RawMessage `json:"output,omitempty"`
	Text      string          `json:"text,omitempty"`
	CostUSD   float64         `json:"cost_usd,omitempty"`
}

// assistStream answers one prompt and streams the transcript as it arrives.
// This is how the board assistant runs on the Mac.
//
// The turn runs on its own, not on this request: a phone that loses its
// connection mid-answer reattaches with GET /v1/assist/{turn}/stream and
// picks up where it left off instead of killing claude and losing the answer.
// The first message is "turn" with the id to reattach with.
func (s *Server) assistStream(w http.ResponseWriter, r *http.Request) {
	path, ok := s.repoPath(w, r)
	if !ok {
		return
	}
	var req assistRequest
	if err := decode(r, &req, false); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
		return
	}
	h, bad := req.headless()
	if bad == "" && req.NeedsYou && s.Attention != nil {
		h.Prompt += attention.Context(s.Attention.Items(r.Context()))
	}
	if bad != "" {
		writeError(w, http.StatusBadRequest, bad, "")
		return
	}
	turn := s.Assist.Start(path, h)
	s.streamTurn(w, r, turn, 0, true)
}

// resumeAssist reattaches to a turn, replaying the events after ?after=seq
// (or Last-Event-ID) and then following it live.
func (s *Server) resumeAssist(w http.ResponseWriter, r *http.Request) {
	turn := s.Assist.Turn(r.PathValue("turn"))
	if turn == nil {
		writeError(w, http.StatusNotFound, "that answer is no longer available", "")
		return
	}
	after := int64(0)
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		after, _ = strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	} else if v := r.URL.Query().Get("after"); v != "" {
		after, _ = strconv.ParseInt(v, 10, 64)
	}
	s.streamTurn(w, r, turn, after, false)
}

// cancelAssist stops a turn the person no longer wants.
func (s *Server) cancelAssist(w http.ResponseWriter, r *http.Request) {
	turn := s.Assist.Turn(r.PathValue("turn"))
	if turn == nil {
		writeError(w, http.StatusNotFound, "that answer is no longer available", "")
		return
	}
	turn.Cancel()
	w.WriteHeader(http.StatusNoContent)
}

// assistTurn is the payload of the first "turn" message.
type assistTurn struct {
	TurnID string `json:"turn_id"`
}

// streamTurn sends a turn's events after seq "after", then one "done" with
// the structured answer or one "error". Leaving early, when the client goes
// away, leaves the turn running.
func (s *Server) streamTurn(w http.ResponseWriter, r *http.Request, turn *assist.Turn, after int64, announce bool) {
	sw, err := sse.NewWriter(w)
	if err != nil {
		return
	}
	if announce && sw.Send("", "turn", assistTurn{TurnID: turn.ID}) != nil {
		return
	}
	ping := time.NewTicker(sse.KeepAlive)
	defer ping.Stop()
	for {
		st := turn.State(after)
		for _, ev := range st.Events {
			if sw.Send(strconv.FormatInt(ev.Seq, 10), "event", ev) != nil {
				return
			}
			after = ev.Seq
		}
		if st.Done {
			if st.Err != nil {
				if !errors.Is(st.Err, context.Canceled) {
					s.Log.Printf("assist: %v", st.Err)
				}
				_ = sw.Send("", "error", api.Error{Error: st.Err.Error()})
				return
			}
			out := st.Out
			_ = sw.Send("", "done", assistResult{SessionID: out.SessionID, Output: out.Output, Text: out.Text, CostUSD: out.CostUSD})
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if sw.Ping() != nil {
				return
			}
		case <-st.Changed:
		}
	}
}
