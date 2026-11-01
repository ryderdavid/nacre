package server

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
	"github.com/ryderdavid/nacre/bridge/internal/sessions"
	"github.com/ryderdavid/nacre/bridge/internal/sse"
)

type fakeHinter struct {
	mu    sync.Mutex
	hints []sessions.Hint
}

func (f *fakeHinter) Nudge(h sessions.Hint) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hints = append(f.hints, h)
	return true
}

func hookServer(t *testing.T, hinter SessionHinter) *httptest.Server {
	t.Helper()
	mgr, err := runs.New(runs.Options{Dir: t.TempDir(), ClaudePath: "/nonexistent"})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		Config:   &config.Config{Token: "tok"},
		Repos:    repos.New(func() []string { return nil }, bd.New("/nonexistent", "")),
		BD:       bd.New("/nonexistent", ""),
		Runs:     mgr,
		Broker:   sse.NewBroker(),
		Log:      log.New(io.Discard, "", 0),
		Sessions: hinter,
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func postSessions(t *testing.T, url, token, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", url+"/v1/sessions", strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}

const sid = "c704d257-8d1e-4bf5-9821-9cc7b84c5131"

func TestPostSessionsNudgesTheWatcher(t *testing.T) {
	h := &fakeHinter{}
	ts := hookServer(t, h)

	if code, _ := postSessions(t, ts.URL, "", `{"session_id":"`+sid+`"}`); code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", code)
	}
	if code, _ := postSessions(t, ts.URL, "tok", `{"session_id":"../x"}`); code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", code)
	}
	if code, _ := postSessions(t, ts.URL, "tok", `not json`); code != http.StatusBadRequest {
		t.Fatalf("bad body: %d", code)
	}

	// The raw Claude Code payload is accepted as is; only Bash commands are
	// passed on.
	body := `{"session_id":"` + sid + `","transcript_path":"/u/.claude/projects/-x/` + sid + `.jsonl","cwd":"/x",` +
		`"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"bd close NAC-1"},"tool_response":{"stdout":"ok"}}`
	code, resp := postSessions(t, ts.URL, "tok", body)
	if code != http.StatusAccepted || !strings.Contains(resp, `"accepted":true`) {
		t.Fatalf("hint: %d %s", code, resp)
	}
	postSessions(t, ts.URL, "tok", `{"session_id":"`+sid+`","tool_name":"Write","tool_input":{"command":"bd close NAC-2"}}`)
	h.mu.Lock()
	defer h.mu.Unlock()
	want := sessions.Hint{SessionID: sid, TranscriptPath: "/u/.claude/projects/-x/" + sid + ".jsonl", CWD: "/x", Command: "bd close NAC-1"}
	if len(h.hints) != 2 || h.hints[0] != want || h.hints[1].Command != "" {
		t.Fatalf("hints = %+v", h.hints)
	}
}

func TestPostSessionsIsANoOpWithAdoptionOff(t *testing.T) {
	ts := hookServer(t, nil)
	code, resp := postSessions(t, ts.URL, "tok", `{"session_id":"`+sid+`"}`)
	if code != http.StatusAccepted || !strings.Contains(resp, `"accepted":false`) || !strings.Contains(resp, "adopt_sessions is off") {
		t.Fatalf("adoption off: %d %s", code, resp)
	}
}
