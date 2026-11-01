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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/discovery"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
	"github.com/ryderdavid/nacre/bridge/internal/sessions"
	"github.com/ryderdavid/nacre/bridge/internal/sse"
	"github.com/ryderdavid/nacre/bridge/internal/version"
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

func TestBuildProvenance(t *testing.T) {
	oldVersion, oldCommit := version.Version, version.Commit
	version.Version, version.Commit = "0.9.0-2-gabc1234-dirty", strings.Repeat("a", 40)
	t.Cleanup(func() { version.Version, version.Commit = oldVersion, oldCommit })
	ts := newTestServer(t)
	for path, token := range map[string]string{"/v1/ping": "", "/v1/health": "tok"} {
		resp, body := do(t, "GET", ts.URL+path, token)
		if resp.StatusCode != 200 || body["version"] != version.Version || body["commit"] != version.Commit {
			t.Errorf("%s: %d %v", path, resp.StatusCode, body)
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

// sleepingClaude is a claude that reports a session and then stays running, so
// a test can hold a dispatched run in the "running" state.
func sleepingClaude(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "claude")
	script := "#!/bin/sh\n" +
		`echo '{"type":"system","subtype":"init","session_id":"sess-own","model":"fake","cwd":"'"$PWD"'"}'` + "\n" +
		"sleep 30\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// adoptedServer builds a server over mgr, with scanRoot discovered as a repo
// when it is non-empty.
func adoptedServer(t *testing.T, mgr *runs.Manager, scanRoot string) *httptest.Server {
	t.Helper()
	scan := func() []string {
		if scanRoot == "" {
			return nil
		}
		return []string{scanRoot}
	}
	registry := repos.New(scan, bd.New("/nonexistent/bd", ""))
	registry.ActiveRuns = mgr.ActiveCount
	registry.AdoptedRuns = mgr.AdoptedCount
	s := &Server{Config: &config.Config{Token: "tok", DefaultMode: "auto", PromptTemplate: "x"},
		Repos: registry, BD: bd.New("/nonexistent/bd", ""), Runs: mgr,
		Broker: sse.NewBroker(), Log: log.New(io.Discard, "", 0)}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// postJSON sends body (may be "") and returns the status and decoded object.
func postJSON(t *testing.T, url, body string) (*http.Response, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest("POST", url, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

// adoptSessions adopts sids as agent-owned sessions in repoID — all of them in
// one reconcile, which is what the watcher does — and returns their run ids in
// the same order.
func adoptSessions(t *testing.T, mgr *runs.Manager, repoID string, sids ...string) []string {
	t.Helper()
	live := make([]sessions.Session, 0, len(sids))
	for _, sid := range sids {
		live = append(live, sessions.Session{
			Record: sessions.Record{PID: os.Getpid(), SessionID: sid, CWD: "/code/Nacre",
				Name: "agent-session", Version: "2.1.304", Entrypoint: "claude-desktop",
				StartedAt: time.Now().Add(-time.Minute).UnixMilli()},
			Repo:     sessions.Repo{ID: repoID, Name: "Nacre", Path: "/code/Nacre"},
			IssueID:  "NAC-7",
			LinkedBy: sessions.LinkTranscript,
		})
	}
	mgr.SyncAdopted(live)
	byID := map[string]string{}
	for _, r := range mgr.List(runs.Filter{IncludeAdopted: true}) {
		byID[r.SessionID] = r.ID
	}
	ids := make([]string, len(sids))
	for i, sid := range sids {
		if ids[i] = byID[sid]; ids[i] == "" {
			t.Fatalf("session %s was not adopted", sid)
		}
	}
	return ids
}

func TestCancelAndFollowupRefuseAdoptedRuns(t *testing.T) {
	mgr, err := runs.New(runs.Options{Dir: t.TempDir(), ClaudePath: "/nonexistent/claude",
		Logger: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	ts := adoptedServer(t, mgr, "")
	id := adoptSessions(t, mgr, "rid", "sess-agent")[0]

	// Both endpoints exist for this run and both refuse it, rather than
	// signalling or forking a session nacred does not own.
	for _, c := range []struct{ path, body, want string }{
		{"/cancel", "", "signalling"},
		{"/followup", `{"message":"carry on"}`, "fork"},
	} {
		resp, body := postJSON(t, ts.URL+"/v1/runs/"+id+c.path, c.body)
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("POST %s = %d, want 409", c.path, resp.StatusCode)
		}
		msg, _ := body["error"].(string)
		if !strings.Contains(msg, "adopted Claude Code session") || !strings.Contains(msg, c.want) {
			t.Fatalf("POST %s error = %q: it must say why, not just no", c.path, msg)
		}
	}
	// The run is untouched: still running, no end, no error.
	after, err := mgr.Run(id)
	if err != nil || after.Status != "running" || after.EndedAt != "" || after.Error != "" {
		t.Fatalf("run after two refusals = %+v (%v)", after, err)
	}
	// A nonexistent run is still a 404, so 409 really is about this run.
	if resp, _ := postJSON(t, ts.URL+"/v1/runs/r_nope/cancel", ""); resp.StatusCode != 404 {
		t.Fatalf("unknown run cancel = %d, want 404", resp.StatusCode)
	}
}

// dispatchedRunKeys is every key a Run that nacred dispatched serializes, as
// of the API before adopted runs existed. The new fields are all omitempty, so
// this set must not grow: a client built against the older API must get back
// exactly what it got before. Add a key here only together with docs/API.md
// and a deliberate decision that every client can cope.
var dispatchedRunKeys = []string{
	"id", "repo_id", "repo_name", "repo_path", "issue_id", "issue_title",
	"status", "mode", "model", "worktree", "session_id", "created_at",
	"started_at", "ended_at", "cost_usd", "duration_ms", "num_turns", "turns",
	"tool_calls", "last_text", "result_text", "error", "resume_command",
}

func TestListRunsIsUnchangedWithoutIncludeAdopted(t *testing.T) {
	mgr, err := runs.New(runs.Options{Dir: t.TempDir(), ClaudePath: "/nonexistent/claude",
		Logger: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	ts := adoptedServer(t, mgr, "")
	own, err := mgr.Start(runs.StartRequest{RepoID: "rid", RepoPath: t.TempDir(),
		IssueID: "X-1", Prompt: "do it"})
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if r, _ := mgr.Run(own.ID); !r.Active() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	adoptSessions(t, mgr, "rid", "sess-agent")

	// Default listing: the dispatched run only, and serialized exactly as a
	// client that predates adopted runs expects it.
	resp, body := do(t, "GET", ts.URL+"/v1/runs", "tok")
	if resp.StatusCode != 200 {
		t.Fatalf("GET /v1/runs: %d", resp.StatusCode)
	}
	list, _ := body["runs"].([]any)
	if len(list) != 1 {
		t.Fatalf("GET /v1/runs = %v, want just the dispatched run", list)
	}
	got := list[0].(map[string]any)
	if got["id"] != own.ID {
		t.Fatalf("run id = %v, want %v", got["id"], own.ID)
	}
	want := map[string]bool{}
	for _, k := range dispatchedRunKeys {
		want[k] = true
		if _, ok := got[k]; !ok {
			t.Errorf("GET /v1/runs dropped %q from a dispatched run", k)
		}
	}
	for k := range got {
		if !want[k] {
			t.Errorf("GET /v1/runs added %q to a dispatched run: an older client did not ask for it", k)
		}
	}

	// Opting in adds the adopted run and leaves the dispatched one alone.
	resp, body = do(t, "GET", ts.URL+"/v1/runs?include=adopted", "tok")
	if resp.StatusCode != 200 {
		t.Fatalf("GET /v1/runs?include=adopted: %d", resp.StatusCode)
	}
	list, _ = body["runs"].([]any)
	if len(list) != 2 {
		t.Fatalf("include=adopted returned %d runs, want 2", len(list))
	}
	var adopted, dispatched map[string]any
	for _, v := range list {
		r := v.(map[string]any)
		if r["origin"] == "adopted" {
			adopted = r
		} else {
			dispatched = r
		}
	}
	if dispatched == nil || len(dispatched) != len(dispatchedRunKeys) {
		t.Fatalf("include=adopted changed the dispatched run: %v", dispatched)
	}
	if adopted == nil {
		t.Fatalf("no adopted run in %v", list)
	}
	for k, want := range map[string]any{
		"origin": "adopted", "agent": "claude-code", "activity": "working",
		"session_name": "agent-session", "owner_pid": float64(os.Getpid()),
		"can_cancel": false, "can_followup": false,
		"issue_id": "NAC-7", "status": "running",
	} {
		if adopted[k] != want {
			t.Errorf("adopted run %q = %v (%T), want %v", k, adopted[k], adopted[k], want)
		}
	}
	// resume_command is the take-over affordance that replaces them.
	if rc, _ := adopted["resume_command"].(string); !strings.Contains(rc, "claude --resume sess-agent") {
		t.Errorf("adopted resume_command = %q", rc)
	}
}

func TestRepoCountsAdoptedRunsApartFromActiveRuns(t *testing.T) {
	root := t.TempDir()
	repoID := discovery.RepoID(root)
	mgr, err := runs.New(runs.Options{Dir: t.TempDir(), ClaudePath: sleepingClaude(t),
		MaxConcurrent: 2, Logger: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	ts := adoptedServer(t, mgr, root)

	repoOf := func() map[string]any {
		t.Helper()
		resp, body := do(t, "GET", ts.URL+"/v1/repos", "tok")
		if resp.StatusCode != 200 {
			t.Fatalf("GET /v1/repos: %d", resp.StatusCode)
		}
		list, _ := body["repos"].([]any)
		if len(list) != 1 {
			t.Fatalf("GET /v1/repos = %v, want one repo", list)
		}
		return list[0].(map[string]any)
	}

	// With nothing adopted, the repo answers exactly as it did before: the
	// count is present and zero, and adopted_runs is not there at all.
	repo := repoOf()
	if repo["active_runs"] != float64(0) {
		t.Fatalf("active_runs = %v, want 0", repo["active_runs"])
	}
	if _, ok := repo["adopted_runs"]; ok {
		t.Fatalf("adopted_runs present on a bridge with no adopted runs: %v", repo)
	}

	own, err := mgr.Start(runs.StartRequest{RepoID: repoID, RepoPath: root, IssueID: "X-1", Prompt: "do it"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = mgr.Cancel(own.ID); mgr.Shutdown(2 * time.Second) })
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if r, _ := mgr.Run(own.ID); r.Status == "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	adoptSessions(t, mgr, repoID, "sess-a", "sess-b")

	// active_runs still means "runs nacred is doing": the two agent-owned
	// sessions are counted, but separately.
	repo = repoOf()
	if repo["active_runs"] != float64(1) {
		t.Fatalf("active_runs = %v, want 1: only the run nacred owns", repo["active_runs"])
	}
	if repo["adopted_runs"] != float64(2) {
		t.Fatalf("adopted_runs = %v, want 2", repo["adopted_runs"])
	}
	// /v1/health counts the same way.
	_, health := do(t, "GET", ts.URL+"/v1/health", "tok")
	if health["active_runs"] != float64(1) {
		t.Fatalf("health active_runs = %v, want 1", health["active_runs"])
	}
}

// A stale adopted session — alive, but quiet past the idle window — is left out
// of the runs list a client shows and of the repo's adopted count, and comes
// back with include=stale so a bead can still show its idle agent.
func TestListRunsLeavesStaleAdoptedSessionsOutUnlessIncluded(t *testing.T) {
	mgr, err := runs.New(runs.Options{Dir: t.TempDir(), ClaudePath: "/nonexistent/claude",
		Logger: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	quiet := time.Now().Add(-time.Hour)
	mgr.SyncAdopted([]sessions.Session{
		{Record: sessions.Record{PID: 98, SessionID: "sess-busy", CWD: "/code/Nacre", StartedAt: quiet.UnixMilli()},
			Repo: sessions.Repo{ID: "rid", Name: "Nacre", Path: "/code/Nacre"}, IssueID: "NAC-1", LastWrite: time.Now()},
		{Record: sessions.Record{PID: 99, SessionID: "sess-quiet", CWD: "/code/Nacre", StartedAt: quiet.UnixMilli()},
			Repo: sessions.Repo{ID: "rid", Name: "Nacre", Path: "/code/Nacre"}, IssueID: "NAC-7", LastWrite: quiet},
	})
	ts := adoptedServer(t, mgr, "")

	runsIn := func(query string) map[string]map[string]any {
		t.Helper()
		resp, body := do(t, "GET", ts.URL+"/v1/runs"+query, "tok")
		if resp.StatusCode != 200 {
			t.Fatalf("GET /v1/runs%s: %d", query, resp.StatusCode)
		}
		out := map[string]map[string]any{}
		list, _ := body["runs"].([]any)
		for _, r := range list {
			run := r.(map[string]any)
			out[run["session_id"].(string)] = run
		}
		return out
	}
	got := runsIn("?include=adopted")
	if len(got) != 1 || got["sess-busy"] == nil {
		t.Fatalf("?include=adopted = %v, want only the working session", got)
	}
	if _, ok := got["sess-busy"]["stale"]; ok {
		t.Fatalf("a working session carries stale: %v", got["sess-busy"])
	}
	for _, q := range []string{"?include=adopted,stale", "?include=stale,adopted&issue=NAC-7"} {
		got = runsIn(q)
		quietRun := got["sess-quiet"]
		if quietRun == nil || quietRun["stale"] != true || quietRun["activity"] != "idle" || quietRun["status"] != "running" {
			t.Fatalf("GET /v1/runs%s = %v, want the quiet session, idle and stale", q, got)
		}
	}
	if got := runsIn("?include=stale"); len(got) != 0 {
		t.Fatalf("?include=stale alone = %v: stale is a refinement of adopted, not a listing of its own", got)
	}
	if n := mgr.AdoptedCount("rid"); n != 1 {
		t.Fatalf("AdoptedCount = %d, want 1: the stale session is not counted", n)
	}
}

func TestSendLiveRefusesAnIneligibleAdoptedRun(t *testing.T) {
	mgr, err := runs.New(runs.Options{Dir: t.TempDir(), ClaudePath: "/nonexistent/claude",
		Logger: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	ts := adoptedServer(t, mgr, "")
	id := adoptSessions(t, mgr, "rid", "sess-agent")[0]

	if resp, _ := postJSON(t, ts.URL+"/v1/runs/"+id+"/live", `{"message":"  "}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty message = %d, want 400", resp.StatusCode)
	}
	resp, body := postJSON(t, ts.URL+"/v1/runs/"+id+"/live", `{"message":"carry on"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("POST /live = %d, want 409", resp.StatusCode)
	}
	if msg, _ := body["error"].(string); !strings.Contains(msg, "live send unavailable") {
		t.Fatalf("error = %q", msg)
	}
	if resp, _ := postJSON(t, ts.URL+"/v1/runs/r_nope/live", `{"message":"hi"}`); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown run = %d, want 404", resp.StatusCode)
	}
}
