package server

import (
	"encoding/json"
	"github.com/ryderdavid/nacre/bridge/internal/daemonupdate"
	"io"
	"net/http"
)

// Admission and update reservation share one lock. A long-running mutation is
// reported busy rather than holding an update request open until it finishes.
func (s *Server) maintenance(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" || r.Method == "HEAD" || r.URL.Path == "/v1/daemon/update" {
			next.ServeHTTP(w, r)
			return
		}
		s.maintenanceMu.Lock()
		if s.Updates != nil && s.Updates.Get().Active {
			s.maintenanceMu.Unlock()
			writeError(w, 503, "This computer is updating nacred. Try again when it reconnects.", "")
			return
		}
		s.mutations++
		s.maintenanceMu.Unlock()
		defer func() { s.maintenanceMu.Lock(); s.mutations--; s.maintenanceMu.Unlock() }()
		next.ServeHTTP(w, r)
	})
}
func (s *Server) updateStatus(w http.ResponseWriter, r *http.Request) {
	if s.Updates == nil {
		writeJSON(w, 200, daemonupdate.Status{Phase: "unavailable", Message: "Install a newer nacred on this computer once to enable updates from Settings."})
		return
	}
	writeJSON(w, 200, s.Updates.Get())
}
func (s *Server) busyForUpdate() string {
	if s.mutations > 0 {
		return "A request is still running. Try updating this computer again shortly."
	}
	if s.Runs != nil && s.Runs.ActiveCount("") > 0 {
		return "This computer has active or queued runs. Finish them before updating."
	}
	if s.Rounds != nil && s.Rounds.Busy() {
		return "This computer has an unfinished round or its cleanup is still running. Wait for it to finish before updating."
	}
	if s.Assist != nil && s.Assist.Busy() {
		return "An assistant turn is running. Wait for it to finish before updating."
	}
	if s.Changes != nil && s.Changes.Busy() {
		return "A specification workflow is running. Wait for it to finish before updating."
	}
	return ""
}
func (s *Server) startUpdate(w http.ResponseWriter, r *http.Request) {
	if s.Updates == nil {
		writeError(w, 501, "Update nacred manually once to enable updates from Settings.", "")
		return
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	d.DisallowUnknownFields()
	var body struct{}
	if err := d.Decode(&body); err != nil && err != io.EOF {
		writeError(w, 400, "Update requests take no options.", "")
		return
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		writeError(w, 400, "Expected a single update request.", "")
		return
	}
	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	if current := s.Updates.Get(); current.Active {
		if current.Phase == "interrupted" {
			writeError(w, 409, current.Message, "")
			return
		}
		writeJSON(w, 202, current)
		return
	}
	if reason := s.busyForUpdate(); reason != "" {
		writeError(w, 409, reason, "")
		return
	}
	state, err := s.Updates.Start()
	if err != nil {
		writeError(w, 409, err.Error(), "")
		return
	}
	writeJSON(w, 202, state)
}
