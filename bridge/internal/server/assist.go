package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/assist"
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
	// Keep saves the session so the next turn can resume it.
	Keep bool `json:"keep"`
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

// assistStream answers one prompt and streams the transcript as it arrives:
// "event" messages carrying RunEvents, then one "done" with the structured
// answer, or one "error". This is how the board assistant runs on the Mac.
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
	if bad != "" {
		writeError(w, http.StatusBadRequest, bad, "")
		return
	}

	sw, err := sse.NewWriter(w)
	if err != nil {
		return
	}
	// The process runs on this goroutine; events are queued so a slow phone
	// cannot stall the reader that drains claude's stdout.
	events := make(chan api.RunEvent, 256)
	var (
		out    assist.Outcome
		runErr error
	)
	// Closing events signals completion, so out and runErr are set by the time
	// the loop below leaves.
	go func() {
		defer close(events)
		var seq int64
		o, e := s.Assist.Run(r.Context(), path, h, func(ev api.RunEvent) {
			seq++
			ev.Seq, ev.TS = seq, time.Now().UTC().Format(time.RFC3339Nano)
			select {
			case events <- ev:
			default: // the client is behind; drop progress rather than block
			}
		})
		out, runErr = o, e
	}()

	ping := time.NewTicker(sse.KeepAlive)
	defer ping.Stop()
	for events != nil {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if sw.Ping() != nil {
				return
			}
		case ev, open := <-events:
			if !open {
				events = nil
				break
			}
			if sw.Send("", "event", ev) != nil {
				return
			}
		}
	}
	if runErr != nil {
		s.Log.Printf("assist: %v", runErr)
		_ = sw.Send("", "error", api.Error{Error: runErr.Error()})
		return
	}
	_ = sw.Send("", "done", assistResult{SessionID: out.SessionID, Output: out.Output, Text: out.Text, CostUSD: out.CostUSD})
}
