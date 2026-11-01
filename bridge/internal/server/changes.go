package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/specflow"
)

func (s *Server) getChange(w http.ResponseWriter, r *http.Request) {
	path, ok := s.repoPath(w, r)
	if !ok {
		return
	}
	id, ok := issueID(w, r)
	if !ok {
		return
	}
	if s.Changes == nil {
		writeJSON(w, http.StatusOK, specflow.Envelope{Reason: "Specification workflows are unavailable on this bridge."})
		return
	}
	result, err := s.Changes.Get(r.Context(), path, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) actOnChange(w http.ResponseWriter, r *http.Request) {
	path, ok := s.repoPath(w, r)
	if !ok {
		return
	}
	id, ok := issueID(w, r)
	if !ok {
		return
	}
	if s.Changes == nil {
		writeError(w, http.StatusServiceUnavailable, "Specification workflows are unavailable on this bridge.", "")
		return
	}
	var action specflow.Action
	if !decodeOK(w, r, &action) {
		return
	}
	switch action.Action {
	case "develop", "revise", "approve", "refresh", "resume":
	default:
		writeError(w, http.StatusBadRequest, "Choose a supported specification action.", "develop, revise, approve, refresh, resume")
		return
	}
	if len(action.Feedback) > 12000 {
		writeError(w, http.StatusBadRequest, "Feedback exceeds the 12000-byte limit.", "")
		return
	}
	result, err := s.Changes.Act(r.Context(), path, id, action)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// Generic runs cannot bypass a reviewed change's revision and workspace guards.
func (s *Server) standaloneChangeGuard(ctx context.Context, path, id string, raw api.Issue) error {
	if s.Changes != nil {
		contract, err := s.Changes.Contract(ctx, path, id)
		if err != nil {
			return err
		}
		if contract != nil {
			return fmt.Errorf("%w: Start a specification round from the Change view.", specflow.ErrConflict)
		}
	}
	var issue struct {
		Labels []string `json:"labels"`
	}
	if err := json.Unmarshal(raw, &issue); err != nil {
		return err
	}
	for _, label := range issue.Labels {
		if strings.HasPrefix(label, "specflow:") || strings.HasPrefix(label, "spec-section:") {
			return fmt.Errorf("%w: Start a specification round from the Change view; its workflow record must be available.", specflow.ErrConflict)
		}
	}
	return nil
}

// intake gives a New effort draft its verdict: just a bead, a new change, an
// amendment, or already covered. It writes nothing.
func (s *Server) intake(w http.ResponseWriter, r *http.Request) {
	path, ok := s.repoPath(w, r)
	if !ok {
		return
	}
	if s.Changes == nil {
		writeError(w, http.StatusServiceUnavailable, "Specification workflows are unavailable on this bridge.", "")
		return
	}
	var draft specflow.IntakeDraft
	if !decodeOK(w, r, &draft) {
		return
	}
	if len(draft.Title)+len(draft.Description)+len(draft.Design)+len(draft.Acceptance) > 24000 {
		writeError(w, http.StatusBadRequest, "The draft is too long for intake; shorten it.", "")
		return
	}
	verdict, err := s.Changes.Intake(r.Context(), path, draft)
	if err != nil {
		if errors.Is(err, specflow.ErrConflict) || errors.Is(err, specflow.ErrUnavailable) {
			s.fail(w, err)
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "cost_usd": verdict.CostUSD})
		return
	}
	writeJSON(w, http.StatusOK, verdict)
}
