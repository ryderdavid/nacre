package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/chat"
)

// protectCase is one request: what a chat token gets refused for.
type protectCase struct {
	method, path, body string
	// kinds is the needs_decision refusal; nil means the route is open to chats.
	kinds []string
	// paired: only a paired device may call it (answers).
	paired bool
}

// The protected routes, legacy ones included, and the bodies or states that
// make them protected. Every route not named here must stay open to chats.
func protectedCases() []protectCase {
	return []protectCase{
		{"POST", "/v1/repos/r1/issues/N-1/signoff", `{"approve":true}`, []string{"signoff"}, false},
		{"POST", "/v1/repos/r1/issues/N-1/signoff", `{"approve":false,"note":"no"}`, []string{"signoff"}, false},
		{"POST", "/v1/repos/r1/issues/N-1/change", `{"action":"approve","revision":"r4"}`, []string{"approve"}, false},
		{"POST", "/v1/repos/r1/issues/N-1/change", `{"action":"approve","revision":"r4","run":true}`, []string{"approve"}, false},
		{"POST", "/v1/repos/r1/issues/N-1/change", `{"action":"adopt","change_name":"x"}`, []string{"adopt"}, false},
		{"POST", "/v1/repos/r1/issues/N-1/change", `{"action":"develop"}`, nil, false},
		{"POST", "/v1/repos/r1/issues/N-1/change", `{"action":"revise","feedback":"tighter"}`, nil, false},
		{"POST", "/v1/repos/r1/issues/N-1/change", `{"action":"resume"}`, nil, false},
		{"POST", "/v1/repos/r1/rounds", `{"beads":["N-1"]}`, nil, false},
		{"POST", "/v1/repos/r1/rounds", `{"beads":["N-1"],"budget_usd":10}`, nil, false},
		{"POST", "/v1/repos/r1/rounds", `{"beads":["N-1"],"land_when_ready":true}`, []string{"land"}, false},
		{"POST", "/v1/repos/r1/rounds", `{"epic":"N-9","finish":true}`, []string{"finish"}, false},
		{"POST", "/v1/repos/r1/rounds", `{"beads":["N-1"],"budget_usd":100}`, []string{"limit"}, false},
		{"POST", "/v1/rounds/rd1/limits", `{"budget_usd":60}`, []string{"limit"}, false},
		{"POST", "/v1/rounds/rd1/land", ``, []string{"land"}, false},
		{"POST", "/v1/repos/r1/epics/N-9/finish", ``, []string{"finish"}, false},
		{"POST", "/v1/rounds/rd1/choose", `{"choice":"raise-limits","budget_usd":60}`, []string{"limit"}, false},
		{"POST", "/v1/rounds/rd1/choose", `{"choice":"retry-with-budget","budget_usd":5}`, []string{"limit"}, false},
		{"POST", "/v1/rounds/rdland/choose", `{"choice":"continue"}`, []string{"land"}, false},
		{"POST", "/v1/rounds/rd1/choose", `{"choice":"continue"}`, nil, false},
		{"POST", "/v1/rounds/rd1/choose", `{"choice":"skip-bead"}`, nil, false},
		{"POST", "/v1/rounds/rd1/choose", `{"choice":"cancel"}`, nil, false},
		{"POST", "/v1/rounds/rdland/resume", ``, []string{"land"}, false},
		{"POST", "/v1/rounds/rd1/resume", ``, nil, false},
		{"POST", "/v1/chats/CHAT/permissions/req1", `{"allow":true}`, nil, true},
		{"POST", "/v1/decisions/dc_1234567/answer", `{"option":"run_hold"}`, nil, true},
	}
}

// concrete fills a route pattern with sample values.
func concrete(pattern, chatID string) string {
	parts := strings.Split(pattern, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, "{") {
			switch p {
			case "{chat}":
				parts[i] = chatID
			case "{round}":
				parts[i] = "rd1"
			default:
				parts[i] = "x1"
			}
		}
	}
	return strings.Join(parts, "/")
}

// Every route, legacy ones included: a chat token is refused exactly the
// protected actions with 403 needs_decision, and answers with 403 "Answer
// from a paired device"; a device token reaches every route.
func TestProtectedRoutesRefuseTheChatTokenOnly(t *testing.T) {
	bin, _ := chatClaude(t, "")
	s, _ := chatServer(t, bin)
	rec, err := s.Chats.Create(chat.CreateRequest{Agent: chat.AgentClaude})
	if err != nil {
		t.Fatal(err)
	}
	chatTok := ChatTokenFor("tok")(rec.ID)
	facts := newFacts()
	facts.setRound(api.Round{ID: "rd1", State: api.RoundPaused, Stop: &api.Stop{Kind: api.StopGatesFailed, Phase: "final"}})
	facts.setRound(api.Round{ID: "rdland", State: api.RoundPaused, Stop: &api.Stop{Kind: api.StopGitError, Phase: "land"}})
	s.Facts = facts
	// Stub handlers for every real route: protection runs before them.
	mux := http.NewServeMux()
	routes := s.routes()
	for _, rt := range routes {
		mux.HandleFunc(rt.method+" "+rt.pattern, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"reached": rt.pattern})
		})
	}
	h := s.auth(s.protect(mux))
	send := func(method, path, body, token string) (int, map[string]any) {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}

	cases := protectedCases()
	special := map[string]bool{}
	for _, c := range cases {
		path := strings.ReplaceAll(c.path, "CHAT", rec.ID)
		special[c.method+" "+path] = true
		code, body := send(c.method, path, c.body, chatTok)
		switch {
		case c.paired:
			if code != http.StatusForbidden || body["error"] != chat.PairedDeviceMsg {
				t.Errorf("chat token %s %s %s: %d %v, want 403 paired device", c.method, path, c.body, code, body)
			}
		case c.kinds != nil:
			var kinds []string
			for _, k := range body["kinds"].([]any) {
				kinds = append(kinds, k.(string))
			}
			if code != http.StatusForbidden || body["error"] != "needs_decision" || !reflect.DeepEqual(kinds, c.kinds) ||
				!strings.Contains(body["message"].(string), "/v1/chats/"+rec.ID+"/decisions") {
				t.Errorf("chat token %s %s %s: %d %v, want needs_decision %v", c.method, path, c.body, code, body, c.kinds)
			}
		default:
			if code != http.StatusOK {
				t.Errorf("chat token %s %s %s: %d %v, want it open", c.method, path, c.body, code, body)
			}
		}
		// Device tokens keep working everywhere.
		if code, body := send(c.method, path, c.body, "tok"); code != http.StatusOK {
			t.Errorf("device token %s %s %s: %d %v", c.method, path, c.body, code, body)
		}
	}
	// Every other route is open to the chat token as well as the device token.
	for _, rt := range routes {
		path := concrete(rt.pattern, rec.ID)
		if special[rt.method+" "+path] || rt.method == "POST" && protectedPattern(rt.pattern) {
			continue
		}
		for _, tok := range []string{chatTok, "tok"} {
			if code, body := send(rt.method, path, `{}`, tok); code != http.StatusOK || body["reached"] != rt.pattern {
				t.Errorf("%s %s with %s: %d %v, want it open", rt.method, path, tok[:3], code, body)
			}
		}
	}
	// The table names every protected pattern that exists.
	for _, p := range []string{"/v1/rounds/{round}/land", "/v1/repos/{repo}/epics/{id}/finish", "/v1/repos/{repo}/issues/{id}/signoff",
		"/v1/rounds/{round}/limits", "/v1/repos/{repo}/issues/{id}/change", "/v1/repos/{repo}/rounds",
		"/v1/rounds/{round}/choose", "/v1/rounds/{round}/resume", "/v1/chats/{chat}/permissions/{req}"} {
		found := false
		for _, rt := range routes {
			found = found || rt.pattern == p
		}
		if !found {
			t.Errorf("no route %s", p)
		}
	}
}

// protectedPattern names the patterns whose cases protectedCases covers.
func protectedPattern(p string) bool {
	switch p {
	case "/v1/rounds/{round}/land", "/v1/repos/{repo}/epics/{id}/finish", "/v1/repos/{repo}/issues/{id}/signoff",
		"/v1/rounds/{round}/limits", "/v1/repos/{repo}/issues/{id}/change", "/v1/repos/{repo}/rounds",
		"/v1/rounds/{round}/choose", "/v1/rounds/{round}/resume", "/v1/chats/{chat}/permissions/{req}",
		"/v1/decisions/{decision}/answer":
		return true
	}
	return false
}

// Through the real handlers: the chat token is refused landing, the device
// token reaches the route (here an unknown round), and cancelling stays open.
func TestChatTokenAgainstTheRealRoutes(t *testing.T) {
	bin, _ := chatClaude(t, "")
	s, _ := chatServer(t, bin)
	rec, _ := s.Chats.Create(chat.CreateRequest{Agent: chat.AgentClaude})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	chatTok := ChatTokenFor("tok")(rec.ID)
	post := func(path, token string) (int, map[string]any) {
		req, _ := http.NewRequest("POST", ts.URL+path, strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	if code, body := post("/v1/rounds/rdnope/land", chatTok); code != http.StatusForbidden || body["error"] != "needs_decision" {
		t.Fatalf("chat token land: %d %v", code, body)
	}
	if code, _ := post("/v1/rounds/rdnope/land", "tok"); code != http.StatusServiceUnavailable {
		t.Fatalf("device token land reaches the route: %d", code)
	}
	if code, body := post("/v1/rounds/rdnope/cancel", chatTok); code == http.StatusForbidden || code == http.StatusUnauthorized {
		t.Fatalf("chat token cancel: %d %v", code, body)
	}
}
