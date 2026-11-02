package server

import (
	"net/http"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// The app signs this computer's Claude in from the phone: start returns the
// sign-in link to open there, and the code the page shows comes back to
// finish, which keeps Claude's long-lived token for every claude nacred runs.

func (s *Server) claudeSignInState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"token": claude.HasToken(), "host_name": s.Config.Host().Name})
}

func (s *Server) startClaudeSignIn(w http.ResponseWriter, r *http.Request) {
	s.signInMu.Lock()
	defer s.signInMu.Unlock()
	if s.signIn != nil {
		s.signIn.Close()
		s.signIn = nil
	}
	in, link, err := claude.StartSignIn(s.Config.ClaudePath)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Couldn't start signing in to Claude", err.Error())
		return
	}
	s.signIn = in
	writeJSON(w, http.StatusOK, map[string]any{"url": link})
}

func (s *Server) finishClaudeSignIn(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := decode(r, &req, false); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
		return
	}
	if strings.TrimSpace(req.Code) == "" {
		writeError(w, http.StatusBadRequest, "code is required", "")
		return
	}
	s.signInMu.Lock()
	in := s.signIn
	s.signIn = nil
	s.signInMu.Unlock()
	if in == nil {
		writeError(w, http.StatusConflict, "Start signing in again", "the sign-in ended or was never started")
		return
	}
	if err := in.Finish(req.Code); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "")
		return
	}
	s.Log.Printf("claude: signed in with a long-lived token")
	writeJSON(w, http.StatusOK, map[string]any{"token": true})
}
