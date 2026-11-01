package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
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
	case "develop", "revise", "approve", "refresh", "resume", "adopt":
	default:
		writeError(w, http.StatusBadRequest, "Choose a supported specification action.", "develop, revise, approve, refresh, resume, adopt")
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
		if label == specflow.FoundAmendLabel {
			return fmt.Errorf("%w: This is a pending amendment to its specification change: revise the change or make it a follow-on.", specflow.ErrConflict)
		}
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

// underSpecEpic makes a child of a spec epic ordinary found work: it doesn't
// inherit the epic's workflow labels (which would make it fail closed as
// linked work with no mapping) and carries found:<change-id> instead.
func (s *Server) underSpecEpic(ctx context.Context, path string, p *bd.CreateParams) error {
	if p.Parent == "" {
		return nil
	}
	raw, err := s.BD.Get(ctx, path, p.Parent)
	if err != nil {
		return err
	}
	var parent struct {
		Labels []string `json:"labels"`
	}
	if err := json.Unmarshal(raw, &parent); err != nil {
		return err
	}
	if change := specflow.SpecEpicChange(parent.Labels); change != "" {
		p.NoInheritLabels = true
		label := specflow.FoundLabelPrefix + change
		if !slices.Contains(p.Labels, label) {
			p.Labels = append(p.Labels, label)
		}
	}
	return nil
}

// afterCreate links a new bead as asked: related beads, the found bead it
// follows on from (closed with a link), the Found: comment it answers.
func (s *Server) afterCreate(ctx context.Context, path, id string, req createIssueRequest) error {
	var errs []error
	for _, rel := range req.Related {
		if err := s.BD.DepAdd(ctx, path, id, rel, "related"); err != nil {
			errs = append(errs, err)
		}
	}
	if req.FollowOnOf != "" {
		if err := s.BD.Close(ctx, path, req.FollowOnOf, "Made a follow-on change: "+id+"."); err != nil {
			errs = append(errs, err)
		}
	}
	if fc := req.FoundComment; fc != nil && s.Changes != nil && validID(fc.Bead) && fc.CommentID != "" {
		if err := s.Changes.MarkFound(ctx, path, fc.Bead, fc.CommentID, "answered: out of scope → "+id); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// found answers discovered work on a promoted change.
func (s *Server) found(w http.ResponseWriter, r *http.Request) {
	path, ok := s.repoPath(w, r)
	if !ok {
		return
	}
	if s.Changes == nil {
		writeError(w, http.StatusServiceUnavailable, "Specification workflows are unavailable on this bridge.", "")
		return
	}
	var req specflow.FoundRequest
	if !decodeOK(w, r, &req) {
		return
	}
	for _, id := range append([]string{req.FoundIn}, req.FoundBeads...) {
		if id != "" && !validID(id) {
			writeError(w, http.StatusBadRequest, "invalid bead id", "")
			return
		}
	}
	result, err := s.Changes.Found(r.Context(), path, r.PathValue("change"), req)
	if result.BeadID != "" || req.Answer == specflow.FoundDismiss || req.Answer == specflow.FoundRevise {
		s.changed(r.PathValue("repo"))
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
