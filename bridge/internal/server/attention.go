package server

import (
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
	writeJSON(w, http.StatusOK, map[string]any{"changes": changes, "total": total, "openspec": specflow.OpenSpecReady(path)})
}
