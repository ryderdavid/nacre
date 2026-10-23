package server

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
	"github.com/ryderdavid/nacre/bridge/internal/sessions"
	"github.com/ryderdavid/nacre/bridge/internal/sse"
)

func TestCheckToken(t *testing.T) {
	want := sha256.Sum256([]byte("s3cret"))
	cases := []struct {
		header string
		ok     bool
	}{
		{"Bearer s3cret", true},
		{"bearer s3cret", true},
		{"BEARER  s3cret ", true},
		{"Bearer s3cre", false},
		{"Bearer s3cret2", false},
		{"Bearer ", false},
		{"Basic s3cret", false},
		{"s3cret", false},
		{"", false},
	}
	for _, c := range cases {
		if got := CheckToken(c.header, want); got != c.ok {
			t.Errorf("CheckToken(%q) = %v, want %v", c.header, got, c.ok)
		}
	}
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := &config.Config{Token: "tok", DefaultMode: "auto", PromptTemplate: "x"}
	mgr, err := runs.New(runs.Options{Dir: t.TempDir(), ClaudePath: "/nonexistent"})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		Config: cfg,
		Repos:  repos.New(func() []string { return nil }, bd.New("/nonexistent", "")),
		BD:     bd.New("/nonexistent", ""),
		Runs:   mgr,
		Broker: sse.NewBroker(),
		Log:    log.New(io.Discard, "", 0),
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func do(t *testing.T, method, url, token string) (*http.Response, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp, body
}

func TestAuthMiddleware(t *testing.T) {
	ts := newTestServer(t)

	// /v1/ping needs no token.
	resp, body := do(t, "GET", ts.URL+"/v1/ping", "")
	if resp.StatusCode != 200 || body["name"] != "nacred" {
		t.Fatalf("ping: %d %v", resp.StatusCode, body)
	}
	// Everything else does.
	for _, tok := range []string{"", "wrong"} {
		resp, body = do(t, "GET", ts.URL+"/v1/config", tok)
		if resp.StatusCode != 401 || body["error"] == nil {
			t.Fatalf("token %q: %d %v", tok, resp.StatusCode, body)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Fatalf("content-type %q", ct)
		}
	}
	resp, body = do(t, "GET", ts.URL+"/v1/config", "tok")
	if resp.StatusCode != 200 || body["default_mode"] != "auto" {
		t.Fatalf("config: %d %v", resp.StatusCode, body)
	}
	// Unknown routes are JSON 404s (after auth), unknown repos/runs too.
	for _, path := range []string{"/v1/nope", "/v1/repos/abc/issues", "/v1/runs/r_x"} {
		resp, body = do(t, "GET", ts.URL+path, "tok")
		if resp.StatusCode != 404 || body["error"] == nil {
			t.Fatalf("%s: %d %v", path, resp.StatusCode, body)
		}
	}
}

func TestValidID(t *testing.T) {
	for id, ok := range map[string]bool{
		"NAC-013.7": true, "GanttGen-eed.4": true, "personal-7jr.2": true,
		"-rf": false, "": false, "a b": false, "x;y": false,
	} {
		if validID(id) != ok {
			t.Errorf("validID(%q) != %v", id, ok)
		}
	}
}

func TestRunStreamReplayAndDisconnect(t *testing.T) {
	cfg := &config.Config{Token: "tok"}
	mgr, err := runs.New(runs.Options{Dir: t.TempDir(), ClaudePath: "/nonexistent/claude"})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Config: cfg, Repos: repos.New(func() []string { return nil }, bd.New("/x", "")),
		BD: bd.New("/x", ""), Runs: mgr, Broker: sse.NewBroker(), Log: log.New(io.Discard, "", 0)}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// A run whose claude binary is missing fails immediately.
	run, err := mgr.Start(runs.StartRequest{RepoID: "r", RepoPath: t.TempDir(), IssueID: "X-1", Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		r, _ := mgr.Run(run.ID)
		if !r.Active() || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/v1/runs/"+run.ID+"/stream?after=1", nil)
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type %q", ct)
	}
	// Read until the trailing "run" message (replay of seq 2.. then run meta).
	br := bufio.NewReader(resp.Body)
	var got []string
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "id: ") || strings.HasPrefix(line, "event: ") {
			got = append(got, line)
		}
		if line == "event: run" {
			break
		}
	}
	if got[0] != "id: 2" || got[len(got)-1] != "event: run" {
		t.Fatalf("stream = %v", got)
	}
	if n := mgr.Subscribers(run.ID); n != 1 {
		t.Fatalf("subscribers = %d", n)
	}
	cancel()
	resp.Body.Close()
	deadline = time.Now().Add(2 * time.Second)
	for mgr.Subscribers(run.ID) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("stream handler did not release its subscription after disconnect")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestListRunsHidesAdoptedUnlessIncluded(t *testing.T) {
	cfg := &config.Config{Token: "tok"}
	mgr, err := runs.New(runs.Options{Dir: t.TempDir(), ClaudePath: "/nonexistent/claude",
		Logger: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	mgr.SyncAdopted([]sessions.Session{{
		Record: sessions.Record{PID: 99, SessionID: "sess-agent", CWD: "/code/Nacre",
			StartedAt: time.Now().UnixMilli()},
		Repo:    sessions.Repo{ID: "rid", Name: "Nacre", Path: "/code/Nacre"},
		IssueID: "NAC-7",
	}})
	s := &Server{Config: cfg, Repos: repos.New(func() []string { return nil }, bd.New("/x", "")),
		BD: bd.New("/x", ""), Runs: mgr, Broker: sse.NewBroker(), Log: log.New(io.Discard, "", 0)}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	runsIn := func(query string) []any {
		t.Helper()
		resp, body := do(t, "GET", ts.URL+"/v1/runs"+query, "tok")
		if resp.StatusCode != 200 {
			t.Fatalf("GET /v1/runs%s: %d", query, resp.StatusCode)
		}
		list, _ := body["runs"].([]any)
		return list
	}
	// An app that does not know about adopted runs would offer cancel and
	// follow-up for a session nacred cannot control, so they are opt-in.
	if got := runsIn(""); len(got) != 0 {
		t.Fatalf("GET /v1/runs = %v, want no adopted runs", got)
	}
	for _, q := range []string{"?include=adopted", "?include=adopted,future", "?include=future,adopted"} {
		got := runsIn(q)
		if len(got) != 1 {
			t.Fatalf("GET /v1/runs%s = %v", q, got)
		}
		run := got[0].(map[string]any)
		if run["origin"] != "adopted" || run["issue_id"] != "NAC-7" || run["status"] != "running" {
			t.Fatalf("run = %v", run)
		}
	}
	if got := runsIn("?include=other"); len(got) != 0 {
		t.Fatalf("?include=other = %v", got)
	}
	// Fetching one by id still works: only the listing is opt-in.
	id := mgr.List(runs.Filter{IncludeAdopted: true})[0].ID
	if resp, _ := do(t, "GET", ts.URL+"/v1/runs/"+id, "tok"); resp.StatusCode != 200 {
		t.Fatalf("GET /v1/runs/%s: %d", id, resp.StatusCode)
	}
}
