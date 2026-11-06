package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/chat"
	"github.com/ryderdavid/nacre/bridge/internal/decisions"
)

// defaultRoundBudgetUSD is the round budget a chat may set without asking,
// when nacred's config sets none (config.RoundBudgetUSD's default).
const defaultRoundBudgetUSD = 25.0

// protect refuses protected actions to chat-scoped tokens with 403
// needs_decision, and answers (to permission requests and decisions) with
// 403 "Answer from a paired device". Device tokens pass untouched.
func (s *Server) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sc := scopeOf(r.Context())
		if !sc.Chat() {
			next.ServeHTTP(w, r)
			return
		}
		if pairedOnly(r) {
			writeError(w, http.StatusForbidden, chat.PairedDeviceMsg, "Only Ryder answers, from a paired device.")
			return
		}
		if kinds := s.protectedKinds(r); len(kinds) > 0 {
			writeJSON(w, http.StatusForbidden, map[string]any{
				"error":   "needs_decision",
				"message": "Ask Ryder: raise a decision with POST /v1/chats/" + sc.ChatID + "/decisions",
				"kinds":   kinds,
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// pairedOnly reports routes only a paired device may call: answers.
func pairedOnly(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	_, perm := matchPath("/v1/chats/{chat}/permissions/{req}", r.URL.EscapedPath())
	_, dec := matchPath("/v1/decisions/{decision}/answer", r.URL.EscapedPath())
	return perm || dec
}

// protectedKinds names the decision kinds a request needs when it performs a
// protected action (owner-decisions: Protected actions need the owner), or
// nil when it performs none. Some routes are protected only for some bodies
// or round states, so the body is read and put back.
func (s *Server) protectedKinds(r *http.Request) []string {
	if r.Method != http.MethodPost {
		return nil
	}
	path := r.URL.EscapedPath()
	if _, ok := matchPath("/v1/rounds/{round}/land", path); ok {
		return []string{decisions.KindLand}
	}
	if _, ok := matchPath("/v1/repos/{repo}/epics/{id}/finish", path); ok {
		return []string{decisions.KindFinish}
	}
	if _, ok := matchPath("/v1/repos/{repo}/issues/{id}/signoff", path); ok {
		return []string{decisions.KindSignoff}
	}
	if _, ok := matchPath("/v1/rounds/{round}/limits", path); ok {
		return []string{decisions.KindLimit}
	}
	if _, ok := matchPath("/v1/repos/{repo}/issues/{id}/change", path); ok {
		var body struct {
			Action string `json:"action"`
		}
		peekJSON(r, &body)
		switch body.Action {
		case "approve":
			return []string{decisions.KindApprove}
		case "adopt":
			return []string{decisions.KindAdopt}
		}
		return nil
	}
	if _, ok := matchPath("/v1/repos/{repo}/rounds", path); ok {
		var body struct {
			LandWhenReady bool    `json:"land_when_ready"`
			Finish        bool    `json:"finish"`
			BudgetUSD     float64 `json:"budget_usd"`
		}
		peekJSON(r, &body)
		var kinds []string
		if body.LandWhenReady {
			kinds = append(kinds, decisions.KindLand)
		}
		if body.Finish {
			kinds = append(kinds, decisions.KindFinish)
		}
		if body.BudgetUSD > s.roundBudgetDefault() {
			kinds = append(kinds, decisions.KindLimit)
		}
		return kinds
	}
	if v, ok := matchPath("/v1/rounds/{round}/choose", path); ok {
		var body struct {
			Choice string `json:"choice"`
		}
		peekJSON(r, &body)
		switch body.Choice {
		case api.ChoiceRetryWithBudget, api.ChoiceRaiseLimits:
			return []string{decisions.KindLimit}
		case api.ChoiceContinue:
			if s.stoppedAtLanding(v["round"]) {
				return []string{decisions.KindLand}
			}
		}
		return nil
	}
	if v, ok := matchPath("/v1/rounds/{round}/resume", path); ok && s.stoppedAtLanding(v["round"]) {
		return []string{decisions.KindLand}
	}
	return nil
}

func (s *Server) roundBudgetDefault() float64 {
	if s.Config != nil && s.Config.RoundBudgetUSD > 0 {
		return s.Config.RoundBudgetUSD
	}
	return defaultRoundBudgetUSD
}

// stoppedAtLanding reports whether a round is paused at its landing step,
// where continuing lands it.
func (s *Server) stoppedAtLanding(id string) bool {
	round, err := s.round(id)
	if err != nil {
		return false
	}
	return round.State == api.RoundPaused && round.Stop != nil && round.Stop.Phase == "land"
}

// peekJSON decodes the request body into v (ignoring errors) and puts the
// body back for the handler.
func peekJSON(r *http.Request, v any) {
	if r.Body == nil {
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(data))
	if err == nil {
		_ = json.Unmarshal(data, v)
	}
}

// matchPath matches a path against a route pattern with {name} segments,
// returning the named values.
func matchPath(pattern, path string) (map[string]string, bool) {
	ps := strings.Split(strings.Trim(pattern, "/"), "/")
	xs := strings.Split(strings.Trim(path, "/"), "/")
	if len(ps) != len(xs) {
		return nil, false
	}
	vals := map[string]string{}
	for i, p := range ps {
		if strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}") {
			if xs[i] == "" {
				return nil, false
			}
			v, err := url.PathUnescape(xs[i])
			if err != nil {
				v = xs[i]
			}
			vals[p[1:len(p)-1]] = v
			continue
		}
		if p != xs[i] {
			return nil, false
		}
	}
	return vals, true
}
