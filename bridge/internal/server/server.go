// Package server implements the nacred HTTP API (docs/API.md) on top of the
// repo registry, the bd client and the run manager.
package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/assist"
	"github.com/ryderdavid/nacre/bridge/internal/attention"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
	"github.com/ryderdavid/nacre/bridge/internal/codex"
	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/daemonupdate"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
	"github.com/ryderdavid/nacre/bridge/internal/rounds"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
	"github.com/ryderdavid/nacre/bridge/internal/sessions"
	"github.com/ryderdavid/nacre/bridge/internal/specflow"
	"github.com/ryderdavid/nacre/bridge/internal/sse"
	"github.com/ryderdavid/nacre/bridge/internal/version"
)

// maxBody bounds JSON request bodies.
const maxBody = 1 << 20

// Server holds the API's dependencies.
type Server struct {
	Updates       *daemonupdate.Manager
	maintenanceMu sync.Mutex
	mutations     int
	Config        *config.Config
	Repos         *repos.Registry
	BD            *bd.Client
	Runs          *runs.Manager
	Assist        *assist.Runner
	// Rounds is nil until nacred has started rounds.
	Rounds  *rounds.Manager
	Changes *specflow.Manager
	// Attention builds Needs you; nil answers an empty list.
	Attention *attention.Builder
	Broker    *sse.Broker
	Log       *log.Logger
	// CodexCheck reports whether beads can run with Codex here; nil checks
	// the Codex CLI at Config.CodexPath (tests replace it).
	CodexCheck func(ctx context.Context) codex.Status
	// Sessions takes hook hints for the session watcher (POST /v1/sessions);
	// nil when adopt_sessions is off, which makes the route a no-op.
	Sessions SessionHinter

	verMu     sync.Mutex
	verAt     time.Time
	bdVer     string
	claudeVer string
	codexSt   codex.Status
}

// SessionHinter is the part of the session watcher POST /v1/sessions uses.
type SessionHinter interface {
	Nudge(sessions.Hint) bool
}

// Handler returns the routed, authenticated and logged HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/ping", s.ping)
	mux.HandleFunc("GET /v1/health", s.health)
	mux.HandleFunc("GET /v1/daemon/update", s.updateStatus)
	mux.HandleFunc("POST /v1/daemon/update", s.startUpdate)
	mux.HandleFunc("GET /v1/config", s.getConfig)
	mux.HandleFunc("GET /v1/repos", s.listRepos)
	mux.HandleFunc("GET /v1/repos/{repo}/issues", s.listIssues)
	mux.HandleFunc("POST /v1/repos/{repo}/issues", s.createIssue)
	mux.HandleFunc("GET /v1/repos/{repo}/issues/{id}", s.getIssue)
	mux.HandleFunc("PATCH /v1/repos/{repo}/issues/{id}", s.updateIssue)
	mux.HandleFunc("POST /v1/repos/{repo}/issues/{id}/close", s.closeIssue)
	mux.HandleFunc("POST /v1/repos/{repo}/issues/{id}/reopen", s.reopenIssue)
	mux.HandleFunc("POST /v1/repos/{repo}/issues/{id}/comments", s.addComment)
	mux.HandleFunc("POST /v1/repos/{repo}/deps", s.addDep)
	mux.HandleFunc("DELETE /v1/repos/{repo}/deps", s.removeDep)
	mux.HandleFunc("POST /v1/repos/{repo}/issues/{id}/runs", s.startRun)
	mux.HandleFunc("GET /v1/repos/{repo}/issues/{id}/change", s.getChange)
	mux.HandleFunc("POST /v1/repos/{repo}/issues/{id}/change", s.actOnChange)
	mux.HandleFunc("GET /v1/repos/{repo}/changes", s.listChanges)
	mux.HandleFunc("POST /v1/repos/{repo}/intake", s.intake)
	mux.HandleFunc("POST /v1/repos/{repo}/changes/{change}/found", s.found)
	mux.HandleFunc("GET /v1/attention", s.listAttention)
	mux.HandleFunc("POST /v1/repos/{repo}/claude/json", s.assistJSON)
	mux.HandleFunc("POST /v1/repos/{repo}/claude/stream", s.assistStream)
	mux.HandleFunc("GET /v1/assist/{turn}/stream", s.resumeAssist)
	mux.HandleFunc("POST /v1/assist/{turn}/cancel", s.cancelAssist)
	mux.HandleFunc("GET /v1/runs", s.listRuns)
	mux.HandleFunc("GET /v1/runs/{run}", s.getRun)
	mux.HandleFunc("GET /v1/runs/{run}/stream", s.streamRun)
	mux.HandleFunc("POST /v1/runs/{run}/cancel", s.cancelRun)
	mux.HandleFunc("POST /v1/runs/{run}/followup", s.followupRun)
	mux.HandleFunc("POST /v1/runs/{run}/live", s.sendLive)
	mux.HandleFunc("GET /v1/events", s.events)
	mux.HandleFunc("POST /v1/sessions", s.hintSession)
	mux.HandleFunc("POST /v1/repos/{repo}/rounds", s.createRound)
	mux.HandleFunc("GET /v1/rounds", s.listRounds)
	mux.HandleFunc("GET /v1/rounds/{round}", s.getRound)
	mux.HandleFunc("GET /v1/rounds/{round}/stream", s.streamRound)
	mux.HandleFunc("POST /v1/rounds/{round}/answers", s.answerRound)
	mux.HandleFunc("POST /v1/rounds/{round}/plan", s.editRoundPlan)
	mux.HandleFunc("POST /v1/rounds/{round}/limits", s.setRoundLimits)
	mux.HandleFunc("POST /v1/rounds/{round}/land", s.landRound)
	mux.HandleFunc("POST /v1/repos/{repo}/epics/{id}/finish", s.finishEpic)
	mux.HandleFunc("POST /v1/rounds/{round}/cancel", s.cancelRound)
	mux.HandleFunc("POST /v1/rounds/{round}/resume", s.resumeRound)
	mux.HandleFunc("POST /v1/rounds/{round}/choose", s.chooseRound)
	mux.HandleFunc("POST /v1/rounds/{round}/beads/{id}/retry", s.retryRoundBead)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "no such route: "+r.Method+" "+r.URL.Path, "")
	})
	return s.logRequests(s.auth(s.maintenance(mux)))
}

// auth requires the bearer token on every /v1/ route except /v1/ping.
func (s *Server) auth(next http.Handler) http.Handler {
	want := sha256.Sum256([]byte(s.Config.Token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/ping" {
			next.ServeHTTP(w, r)
			return
		}
		if !CheckToken(r.Header.Get("Authorization"), want) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nacred"`)
			writeError(w, http.StatusUnauthorized, "missing or invalid bearer token", "")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CheckToken reports whether an Authorization header carries the token whose
// sha256 is want. Hashing first makes the comparison constant-time regardless
// of the presented token's length.
func CheckToken(header string, want [32]byte) bool {
	const prefix = "bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return false
	}
	got := sha256.Sum256([]byte(strings.TrimSpace(header[len(prefix):])))
	return subtle.ConstantTimeCompare(got[:], want[:]) == 1
}

// statusWriter records the response status for logging. Unwrap lets
// http.ResponseController reach the underlying writer (Flush for SSE).
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// logRequests logs "METHOD /path status duration" (never headers or query, so
// the token cannot leak into logs).
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		s.Log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Millisecond))
	})
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg, detail string) {
	writeJSON(w, status, api.Error{Error: msg, Detail: detail})
}

// fail maps an error from a lower layer to an HTTP error response.
func (s *Server) fail(w http.ResponseWriter, err error) {
	var be *bd.Error
	switch {
	case errors.Is(err, repos.ErrNotFound):
		writeError(w, http.StatusNotFound, "unknown repo", "")
	case errors.Is(err, runs.ErrNotFound):
		writeError(w, http.StatusNotFound, "unknown run", "")
	case errors.Is(err, runs.ErrConflict):
		writeError(w, http.StatusConflict, err.Error(), "")
	case errors.Is(err, rounds.ErrNotFound):
		writeError(w, http.StatusNotFound, "unknown round", "")
	case errors.Is(err, rounds.ErrConflict):
		writeError(w, http.StatusConflict, err.Error(), "")
	case errors.Is(err, specflow.ErrConflict):
		writeError(w, http.StatusConflict, err.Error(), "")
	case errors.Is(err, specflow.ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, err.Error(), "")
	case errors.As(err, &be) && be.NotFound:
		writeError(w, http.StatusNotFound, be.Message, be.Detail())
	case errors.As(err, &be) && be.Invalid:
		writeError(w, http.StatusBadRequest, be.Message, be.Detail())
	case errors.As(err, &be):
		writeError(w, http.StatusBadGateway, be.Error(), be.Detail())
	default:
		s.Log.Printf("internal error: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error(), "")
	}
}

// decode reads a JSON body into v. An empty body is allowed when optional.
func decode(r *http.Request, v any, optional bool) error {
	data, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, maxBody))
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		if optional {
			return nil
		}
		return errors.New("request body required")
	}
	return json.Unmarshal(data, v)
}

var issueIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,199}$`)

// validID reports whether id looks like a bead id (and cannot be mistaken for a flag).
func validID(id string) bool { return issueIDRe.MatchString(id) }

// repoPath resolves the {repo} path value, writing a 404 on failure.
func (s *Server) repoPath(w http.ResponseWriter, r *http.Request) (string, bool) {
	p, err := s.Repos.Path(r.PathValue("repo"))
	if err != nil {
		s.fail(w, err)
		return "", false
	}
	return p, true
}

// issueID validates the {id} path value, writing a 400 on failure.
func issueID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !validID(id) {
		writeError(w, http.StatusBadRequest, "invalid issue id", "")
		return "", false
	}
	return id, true
}

// changed invalidates a repo's cache and notifies /v1/events subscribers.
func (s *Server) changed(repoID string) {
	s.Repos.Invalidate(repoID)
	s.Broker.Publish("repo", map[string]string{"repo_id": repoID})
}

// ---- meta routes ----

func (s *Server) ping(w http.ResponseWriter, r *http.Request) {
	h := s.Config.Host()
	writeJSON(w, http.StatusOK, map[string]string{"name": version.Name, "version": version.Version, "commit": version.Commit, "host_id": h.ID, "host_name": h.Name})
}

func (s *Server) versions(ctx context.Context) (string, string) {
	s.verMu.Lock()
	defer s.verMu.Unlock()
	if s.verAt.IsZero() || time.Since(s.verAt) > 5*time.Minute {
		if v, err := s.BD.Version(ctx); err == nil {
			s.bdVer = v
		}
		if v, err := claude.Version(ctx, s.Config.ClaudePath); err == nil {
			s.claudeVer = v
		}
		s.codexSt = s.checkCodex(ctx)
		s.verAt = time.Now()
	}
	return s.bdVer, s.claudeVer
}

func (s *Server) checkCodex(ctx context.Context) codex.Status {
	if s.CodexCheck != nil {
		return s.CodexCheck(ctx)
	}
	return codex.Check(ctx, s.Config.CodexPath, claude.ChildEnv())
}

// codexStatus returns whether Codex runs can start here. A cached "not
// available" is rechecked at once, so signing in to Codex on the computer
// takes effect on the next try rather than minutes later.
func (s *Server) codexStatus(ctx context.Context) codex.Status {
	s.versions(ctx)
	s.verMu.Lock()
	defer s.verMu.Unlock()
	if !s.codexSt.Available() {
		s.codexSt = s.checkCodex(ctx)
	}
	return s.codexSt
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	host, _ := os.Hostname()
	h := s.Config.Host()
	bdv, cv := s.versions(r.Context())
	cx := s.codexStatus(r.Context())
	body := map[string]any{
		"name": version.Name, "version": version.Version, "commit": version.Commit, "hostname": ShortHostname(host),
		"host_id": h.ID, "host_name": h.Name,
		"bd_version": bdv, "claude_version": cv, "codex_version": cx.Version,
		"repos": s.Repos.Count(), "active_runs": s.Runs.ActiveCount(""),
		"features": s.features(cx),
	}
	if !cx.Available() {
		body["codex_unavailable"] = cx.Reason
	}
	writeJSON(w, http.StatusOK, body)
}

// features lists optional capabilities, so the app can offer only what this
// computer can actually do (and an older nacred simply reports none).
func (s *Server) features(cx codex.Status) []string {
	f := []string{}
	if s.Updates != nil {
		f = append(f, "daemon-update")
	}
	if s.Assist != nil && s.Assist.Available() {
		// Refine, Draft and the board assistant can run here on this
		// computer's Claude subscription.
		f = append(f, api.FeatureClaude)
	}
	if cx.Available() {
		// Beads can be run with the Codex CLI here.
		f = append(f, api.FeatureCodex)
	}
	return f
}

// ShortHostname strips a trailing ".local".
func ShortHostname(h string) string { return strings.TrimSuffix(h, ".local") }

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"default_mode":    s.Config.DefaultMode,
		"default_model":   s.Config.DefaultModel,
		"prompt_template": s.Config.PromptTemplate,
		"modes":           api.Modes,
	})
}

// ---- global events ----

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	ch, unsubscribe := s.Broker.Subscribe()
	defer unsubscribe()
	sw, err := sse.NewWriter(w)
	if err != nil {
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
		case msg := <-ch:
			if sw.Send("", msg.Event, msg.Data) != nil {
				return
			}
		}
	}
}
