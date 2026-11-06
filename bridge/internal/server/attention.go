package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/specflow"
)

// listAttention is Needs you: everything on this computer waiting on Ryder.
func (s *Server) listAttention(w http.ResponseWriter, r *http.Request) {
	items := []api.AttentionItem{}
	if s.Attention != nil {
		items = s.Attention.Items(r.Context())
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// listChanges summarizes a repository's specification changes for the board.
func (s *Server) listChanges(w http.ResponseWriter, r *http.Request) {
	path, ok := s.repoPath(w, r)
	if !ok {
		return
	}
	if s.Changes == nil {
		writeJSON(w, http.StatusOK, map[string]any{"changes": []api.ChangeSummary{}, "total": 0})
		return
	}
	changes, total, err := s.Changes.Summaries(path)
	if err != nil {
		s.fail(w, err)
		return
	}
	closed := s.closedBead(r.Context(), path)
	kept := changes[:0:0]
	for _, c := range changes {
		// Given up: its folder is gone and the beads that owned it closed.
		if c.Withdrawn(closed) {
			total--
			continue
		}
		kept = append(kept, c)
	}
	// The checkout's other active changes: the Spec panel lists every one.
	untracked, err := s.Changes.Untracked(path)
	if err != nil {
		s.Log.Printf("untracked changes of %s: %v", path, err)
	}
	if untracked == nil {
		untracked = []api.ChangeSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"changes": kept, "total": total, "openspec": specflow.OpenSpecReady(path), "untracked": untracked})
}

// closedBead reports whether a bead of the repository at path is closed; a
// bead that can't be read isn't.
func (s *Server) closedBead(ctx context.Context, path string) func(string) bool {
	return func(id string) bool {
		if !validID(id) {
			return false
		}
		raw, err := s.BD.Get(ctx, path, id)
		if err != nil {
			return false
		}
		var b struct {
			Status string `json:"status"`
		}
		return json.Unmarshal(raw, &b) == nil && b.Status == "closed"
	}
}

// getUntrackedChange reads an active change of the checkout that no workflow
// record tracks, for the Spec panel's overview and reader.
func (s *Server) getUntrackedChange(w http.ResponseWriter, r *http.Request) {
	path, ok := s.repoPath(w, r)
	if !ok {
		return
	}
	if s.Changes == nil {
		writeJSON(w, http.StatusOK, specflow.Envelope{Reason: "Specification workflows are unavailable on this bridge."})
		return
	}
	env, err := s.Changes.UntrackedChange(path, r.PathValue("change"))
	if errors.Is(err, specflow.ErrNoChange) {
		writeError(w, http.StatusNotFound, err.Error(), "")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, env)
}
