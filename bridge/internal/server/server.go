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
	"net"
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
	"github.com/ryderdavid/nacre/bridge/internal/chat"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
	"github.com/ryderdavid/nacre/bridge/internal/codex"
	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/daemonupdate"
	"github.com/ryderdavid/nacre/bridge/internal/decisions"
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
	// following counts decisions being followed to their outcome in the
	// background; tests wait on it before their temporary files go.
	following sync.WaitGroup
	// announced are the Needs you decisions already published as new; nil
	// until the first sweep, which records what already waits without
	// announcing it.
	announceMu    sync.Mutex
	announced     map[string]bool
	stopFollow    chan struct{}
	stopOnce      sync.Once
	followInit    sync.Once
	Updates       *daemonupdate.Manager
	maintenanceMu sync.Mutex
	mutations     int
	Config        *config.Config
	Repos         *repos.Registry
	BD            *bd.Client
	Runs          *runs.Manager
	Assist        *assist.Runner
	// Chats are the app's Chat conversations; nil when Chat isn't set up.
	Chats *chat.Manager
	// Rounds is nil until nacred has started rounds.
	Rounds  *rounds.Manager
	Changes *specflow.Manager
	// Attention builds Needs you; nil answers an empty list.
	Attention *attention.Builder
	Broker    *sse.Broker
	Log       *log.Logger
	// boardsMu guards the links in Config.AssistBoardsDir.
	boardsMu sync.Mutex
	// CodexCheck reports whether beads can run with Codex here; nil checks
	// the Codex CLI at Config.CodexPath (tests replace it).
	CodexCheck func(ctx context.Context) codex.Status
	// ChatCheck reports whether the installed Claude Code can serve Chat;
	// nil checks the CLI at Config.ClaudePath (tests replace it).
	ChatCheck func(ctx context.Context) claude.ChatSupport
	// CodexAppServerCheck reports whether `codex app-server` starts, for
	// Codex chats; nil starts it and completes its handshake.
	CodexAppServerCheck func(ctx context.Context) error
	// LocalAddrs are this computer's own addresses, whose answers to chat
	// permission requests are refused; nil reads the network interfaces.
	LocalAddrs func() []net.IP
	// Sessions takes hook hints for the session watcher (POST /v1/sessions);
	// nil when adopt_sessions is off, which makes the route a no-op.
	Sessions SessionHinter
	// Facts replaces the state decisions and protection read, in tests;
	// nil reads nacred's own managers.
	Facts Facts
	// Decisions holds owner decisions; nil answers the decision routes 503.
	Decisions *decisions.Store
	// DecisionExec replaces executing an approved decision's action, in
	// tests; nil runs it through nacred's own routes.
	DecisionExec func(ctx context.Context, act decisions.Action) (status int, body []byte)
	innerOnce    sync.Once
	innerHandler http.Handler
	// checked is when each open decision was last revalidated by a list or
	// sweep.
	checkedMu sync.Mutex
	checked   map[string]time.Time

	// signIn is the Claude sign-in the app is part way through.
	signInMu sync.Mutex
	signIn   *claude.SignIn

	verMu     sync.Mutex
	verAt     time.Time
	bdVer     string
	claudeVer string
	codexSt   codex.Status
	chatSt    claude.ChatSupport
	// codexChat says why Codex chats can't run here; "" when they can.
	codexChat string
}

// SessionHinter is the part of the session watcher POST /v1/sessions uses.
type SessionHinter interface {
	Nudge(sessions.Hint) bool
}

// route is one API route: its method, its pattern and its handler.
type route struct {
	method, pattern string
	handler         http.HandlerFunc
}

// routes lists every API route. The protected ones are refused to chat
// tokens by protect (see protectedKinds).
func (s *Server) routes() []route {
	return []route{
		{"GET", "/v1/ping", s.ping},
		{"GET", "/v1/health", s.health},
		{"GET", "/v1/daemon/update", s.updateStatus},
		{"POST", "/v1/daemon/update", s.startUpdate},
		{"GET", "/v1/config", s.getConfig},
		{"GET", "/v1/repos", s.listRepos},
		{"GET", "/v1/repos/{repo}/issues", s.listIssues},
		{"POST", "/v1/repos/{repo}/issues", s.createIssue},
		{"GET", "/v1/repos/{repo}/issues/{id}", s.getIssue},
		{"PATCH", "/v1/repos/{repo}/issues/{id}", s.updateIssue},
		{"POST", "/v1/repos/{repo}/issues/{id}/close", s.closeIssue},
		{"POST", "/v1/repos/{repo}/issues/{id}/reopen", s.reopenIssue},
		{"POST", "/v1/repos/{repo}/issues/{id}/signoff", s.signOff},
		{"POST", "/v1/repos/{repo}/issues/{id}/comments", s.addComment},
		{"POST", "/v1/repos/{repo}/deps", s.addDep},
		{"DELETE", "/v1/repos/{repo}/deps", s.removeDep},
		{"POST", "/v1/repos/{repo}/issues/{id}/runs", s.startRun},
		{"GET", "/v1/repos/{repo}/issues/{id}/change", s.getChange},
		{"POST", "/v1/repos/{repo}/issues/{id}/change", s.actOnChange},
		{"GET", "/v1/repos/{repo}/changes", s.listChanges},
		{"GET", "/v1/repos/{repo}/changes/{change}", s.getUntrackedChange},
		{"POST", "/v1/repos/{repo}/intake", s.intake},
		{"POST", "/v1/repos/{repo}/changes/{change}/found", s.found},
		{"POST", "/v1/repos/{repo}/changes/{change}/draft/dismiss", s.dismissDraft},
		{"POST", "/v1/repos/{repo}/config/draft", s.draftRepoConfig},
		{"POST", "/v1/repos/{repo}/config", s.saveRepoConfig},
		{"GET", "/v1/attention", s.listAttention},
		{"POST", "/v1/repos/{repo}/claude/json", s.assistJSON},
		{"POST", "/v1/repos/{repo}/claude/stream", s.assistStream},
		{"POST", "/v1/claude/stream", s.assistBoardsStream},
		{"GET", "/v1/claude/signin", s.claudeSignInState},
		{"POST", "/v1/claude/signin", s.startClaudeSignIn},
		{"POST", "/v1/claude/signin/code", s.finishClaudeSignIn},
		{"GET", "/v1/assist/{turn}/stream", s.resumeAssist},
		{"POST", "/v1/assist/{turn}/cancel", s.cancelAssist},
		{"GET", "/v1/chats", s.listChats},
		{"POST", "/v1/chats", s.createChat},
		{"GET", "/v1/chats/{chat}", s.getChat},
		{"DELETE", "/v1/chats/{chat}", s.deleteChat},
		{"GET", "/v1/chats/{chat}/stream", s.streamChat},
		{"POST", "/v1/chats/{chat}/messages", s.sendChat},
		{"GET", "/v1/chats/{chat}/images/{image}", s.chatImage},
		{"GET", "/v1/chats/{chat}/mockups/{name}", s.chatMockup},
		{"POST", "/v1/chats/{chat}/cancel", s.cancelChat},
		{"POST", "/v1/chats/{chat}/permissions/{req}", s.answerChatPermission},
		{"GET", "/v1/runs", s.listRuns},
		{"GET", "/v1/runs/{run}", s.getRun},
		{"GET", "/v1/runs/{run}/stream", s.streamRun},
		{"POST", "/v1/runs/{run}/cancel", s.cancelRun},
		{"POST", "/v1/runs/{run}/followup", s.followupRun},
		{"POST", "/v1/runs/{run}/live", s.sendLive},
		{"GET", "/v1/events", s.events},
		{"POST", "/v1/sessions", s.hintSession},
		{"POST", "/v1/repos/{repo}/rounds", s.createRound},
		{"GET", "/v1/rounds", s.listRounds},
		{"GET", "/v1/rounds/{round}", s.getRound},
		{"GET", "/v1/rounds/{round}/stream", s.streamRound},
		{"POST", "/v1/rounds/{round}/answers", s.answerRound},
		{"POST", "/v1/rounds/{round}/answers/recommended", s.acceptRecommended},
		{"POST", "/v1/rounds/{round}/plan", s.editRoundPlan},
		{"POST", "/v1/rounds/{round}/limits", s.setRoundLimits},
		{"GET", "/v1/rounds/{round}/changes", s.roundChanges},
		{"POST", "/v1/rounds/{round}/land", s.landRound},
		{"POST", "/v1/repos/{repo}/epics/{id}/finish", s.finishEpic},
		{"POST", "/v1/rounds/{round}/cancel", s.cancelRound},
		{"POST", "/v1/rounds/{round}/resume", s.resumeRound},
		{"POST", "/v1/rounds/{round}/choose", s.chooseRound},
		{"POST", "/v1/rounds/{round}/beads/{id}/retry", s.retryRoundBead},
		{"GET", "/v1/decisions", s.listDecisions},
		{"GET", "/v1/decisions/{decision}", s.getDecision},
		{"POST", "/v1/decisions/{decision}/answer", s.answerDecision},
		{"POST", "/v1/decisions/{decision}/renew", s.renewDecision},
		{"POST", "/v1/chats/{chat}/decisions", s.raiseDecision},
		{"GET", "/v1/projects", s.listProjects},
		{"GET", "/v1/repos/{repo}/work", s.repoWork},
		{"GET", "/v1/repos/{repo}/specs", s.listSpecs},
	}
}

// Handler returns the routed, authenticated and logged HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, rt := range s.routes() {
		mux.HandleFunc(rt.method+" "+rt.pattern, rt.handler)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "no such route: "+r.Method+" "+r.URL.Path, "")
	})
	return s.logRequests(s.auth(s.protect(s.maintenance(mux))))
}

// auth requires the bearer token on every /v1/ route except /v1/ping.
func (s *Server) auth(next http.Handler) http.Handler {
	want := sha256.Sum256([]byte(s.Config.Token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/ping" {
			next.ServeHTTP(w, r)
			return
		}
		header := r.Header.Get("Authorization")
		if CheckToken(header, want) {
			next.ServeHTTP(w, r.WithContext(withScope(r.Context(), Scope{})))
			return
		}
		if id := chatTokenChat(s.Config.Token, bearer(header)); id != "" && s.Chats != nil && s.Chats.Exists(id) {
			next.ServeHTTP(w, r.WithContext(withScope(r.Context(), Scope{ChatID: id})))
			return
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="nacred"`)
		writeError(w, http.StatusUnauthorized, "missing or invalid bearer token", "")
	})
}

// CheckToken reports whether an Authorization header carries the token whose
// sha256 is want. Hashing first makes the comparison constant-time regardless
// of the presented token's length.
func CheckToken(header string, want [32]byte) bool {
	tok := bearer(header)
	if tok == "" {
		return false
	}
	got := sha256.Sum256([]byte(tok))
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

// decode reads a JSON body of at most maxBody into v. An empty body is
// allowed when optional.
func decode(r *http.Request, v any, optional bool) error {
	return decodeLimit(r, v, optional, maxBody)
}

// decodeLimit is decode with its own size limit. Over the limit the error is
// an *http.MaxBytesError.
func decodeLimit(r *http.Request, v any, optional bool, limit int64) error {
	data, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, limit))
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
		s.chatSt = s.checkChat(ctx)
		s.codexChat = s.checkCodexChat(ctx, s.codexSt)
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

// checkCodexChat says why Codex chats can't run here, given the Codex
// status: "" only when codex.Check passes and `codex app-server` starts.
func (s *Server) checkCodexChat(ctx context.Context, cx codex.Status) string {
	switch {
	case s.Chats == nil:
		return "Chat isn't available on this computer."
	case !cx.Available():
		return cx.Reason
	}
	var err error
	if s.CodexAppServerCheck != nil {
		err = s.CodexAppServerCheck(ctx)
	} else {
		err = codex.ProbeAppServer(ctx, s.Config.CodexPath, chat.CodexEnv(""))
	}
	if err != nil {
		return "Codex app-server doesn't start on this computer (" + claude.OneLine(err.Error(), 200) + ")"
	}
	return ""
}

// codexChatStatus says why Codex chats can't run here ("" when they can). A
// cached refusal is rechecked at once, so signing in to Codex takes effect
// on the next try.
func (s *Server) codexChatStatus(ctx context.Context) string {
	s.versions(ctx)
	s.verMu.Lock()
	defer s.verMu.Unlock()
	if s.codexChat != "" {
		if !s.codexSt.Available() {
			s.codexSt = s.checkCodex(ctx)
		}
		s.codexChat = s.checkCodexChat(ctx, s.codexSt)
	}
	return s.codexChat
}

func (s *Server) checkChat(ctx context.Context) claude.ChatSupport {
	if s.ChatCheck != nil {
		return s.ChatCheck(ctx)
	}
	return claude.CheckChat(ctx, s.Config.ClaudePath)
}

// chatSupport returns whether Chat can run here. A cached "not supported" is
// rechecked at once, so updating Claude Code takes effect on the next try.
func (s *Server) chatSupport(ctx context.Context) claude.ChatSupport {
	s.versions(ctx)
	s.verMu.Lock()
	defer s.verMu.Unlock()
	if !s.chatSt.OK {
		s.chatSt = s.checkChat(ctx)
	}
	return s.chatSt
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
	var cs claude.ChatSupport
	if s.Chats != nil {
		cs = s.chatSupport(r.Context())
	}
	s.verMu.Lock()
	codexChat := s.codexChat
	s.verMu.Unlock()
	body := map[string]any{
		"name": version.Name, "version": version.Version, "commit": version.Commit, "hostname": ShortHostname(host),
		"host_id": h.ID, "host_name": h.Name,
		"bd_version": bdv, "claude_version": cv, "codex_version": cx.Version,
		"repos": s.Repos.Count(), "active_runs": s.Runs.ActiveCount(""),
		"features": s.features(cx, cs, codexChat),
	}
	if !cx.Available() {
		body["codex_unavailable"] = cx.Reason
	}
	if s.Chats != nil && !cs.OK {
		body["agent_chat_unavailable"] = cs.Reason
	}
	if s.Chats != nil && codexChat != "" {
		body["agent_chat_codex_unavailable"] = codexChat
	}
	writeJSON(w, http.StatusOK, body)
}

// features lists optional capabilities, so the app can offer only what this
// computer can actually do (and an older nacred simply reports none).
func (s *Server) features(cx codex.Status, cs claude.ChatSupport, codexChat string) []string {
	f := []string{}
	if s.Updates != nil {
		f = append(f, "daemon-update")
	}
	if s.Assist != nil && s.Assist.Available() {
		// Refine, Draft and the board assistant can run here on this
		// computer's Claude subscription.
		f = append(f, api.FeatureClaude)
		// Bead chat and single-board assistant turns can carry images.
		f = append(f, api.FeatureChatImages)
		// Streamed turns take the app's turn id, so a resend is never a second turn.
		f = append(f, api.FeatureAssistTurnIDs)
	}
	if s.Chats != nil && cs.OK {
		// Chat runs here, and steps the classifier won't allow alone are
		// asked on the phone (host permission prompts).
		f = append(f, api.FeatureAgentChat)
	}
	if s.Chats != nil && codexChat == "" {
		// Chat can run on Codex here: it is signed in and its app-server
		// starts.
		f = append(f, api.FeatureAgentChatCodex)
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
