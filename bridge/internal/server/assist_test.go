package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/assist"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/discovery"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
	"github.com/ryderdavid/nacre/bridge/internal/sse"
)

func TestAssistRequestValidation(t *testing.T) {
	ok := assistRequest{Prompt: "hello", Schema: json.RawMessage(`{"type":"object"}`), Effort: "high", Model: "claude-opus-5-5"}
	if _, bad := ok.headless(); bad != "" {
		t.Fatalf("valid request rejected: %s", bad)
	}
	cases := map[string]assistRequest{
		"prompt required":          {Prompt: "   "},
		"schema is not valid JSON": {Prompt: "x", Schema: json.RawMessage(`{nope`)},
		"unknown effort level":     {Prompt: "x", Effort: "turbo"},
		"invalid model":            {Prompt: "x", Model: "--dangerously-skip-permissions"},
		"invalid session id":       {Prompt: "x", SessionID: "-rf /"},
		"prompt too long":          {Prompt: strings.Repeat("a", maxAssistPrompt+1)},
	}
	for want, req := range cases {
		if _, bad := req.headless(); bad != want {
			t.Errorf("%+v rejected as %q, want %q", req, bad, want)
		}
	}
	// Read-only bd commands only arrive when the caller asks for them.
	if h, _ := (assistRequest{Prompt: "x"}).headless(); len(h.AllowedTools) != 0 {
		t.Errorf("tools granted without bd_reads: %v", h.AllowedTools)
	}
	if h, _ := (assistRequest{Prompt: "x", BdReads: true}).headless(); len(h.AllowedTools) == 0 {
		t.Error("bd_reads granted no tools")
	}
}

// assistServer serves one repo (a temp dir) with a fake claude binary.
func assistServer(t *testing.T, transcript string) (ts *httptest.Server, repoID string) {
	t.Helper()
	repoDir := t.TempDir()
	bin := filepath.Join(t.TempDir(), "claude")
	script := "#!/bin/sh\ncat <<'EOF'\n" + transcript + "\nEOF\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	mgr, err := runs.New(runs.Options{Dir: t.TempDir(), ClaudePath: bin})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		Config: &config.Config{Token: "tok"},
		Repos:  repos.New(func() []string { return []string{repoDir} }, bd.New("/nonexistent", "")),
		BD:     bd.New("/nonexistent", ""),
		Runs:   mgr,
		Assist: &assist.Runner{ClaudePath: bin},
		Broker: sse.NewBroker(),
		Log:    log.New(io.Discard, "", 0),
	}
	ts = httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts, discovery.RepoID(repoDir)
}

func post(t *testing.T, url, body string) (*http.Response, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest("POST", url, bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

const refineTranscript = `{"type":"system","subtype":"init","session_id":"s-1"}
{"type":"result","subtype":"success","is_error":false,"session_id":"s-1","result":"{\"summary\":\"tightened it\"}","structured_output":{"summary":"tightened it"}}`

func TestAssistJSONReturnsStructuredOutput(t *testing.T) {
	ts, repoID := assistServer(t, refineTranscript)

	resp, body := post(t, ts.URL+"/v1/repos/"+repoID+"/claude/json",
		`{"prompt":"improve this bead","system":"you are an editor","schema":{"type":"object"}}`)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %v", resp.StatusCode, body)
	}
	out, _ := body["output"].(map[string]any)
	if out["summary"] != "tightened it" {
		t.Fatalf("output = %v", body["output"])
	}
	if body["session_id"] != "s-1" {
		t.Errorf("session_id = %v", body["session_id"])
	}

	// A bad body is a 400, and an unknown repo is a 404.
	if resp, _ := post(t, ts.URL+"/v1/repos/"+repoID+"/claude/json", `{"prompt":""}`); resp.StatusCode != 400 {
		t.Errorf("empty prompt: %d", resp.StatusCode)
	}
	if resp, _ := post(t, ts.URL+"/v1/repos/nope/claude/json", `{"prompt":"x"}`); resp.StatusCode != 404 {
		t.Errorf("unknown repo: %d", resp.StatusCode)
	}
}

func TestAssistStreamSendsEventsThenDone(t *testing.T) {
	const transcript = `{"type":"system","subtype":"init","session_id":"s-1"}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"bd list"}}]}}
{"type":"result","subtype":"success","is_error":false,"session_id":"s-1","result":"{\"reply\":\"all clear\",\"proposals\":[]}","structured_output":{"reply":"all clear","proposals":[]}}`
	ts, repoID := assistServer(t, transcript)

	req, _ := http.NewRequest("POST", ts.URL+"/v1/repos/"+repoID+"/claude/stream",
		bytes.NewReader([]byte(`{"prompt":"what is next?","bd_reads":true,"keep":true}`)))
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type %q", ct)
	}

	var names []string
	var last string
	br := bufio.NewReader(resp.Body)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "event: "):
			names = append(names, strings.TrimPrefix(line, "event: "))
		case strings.HasPrefix(line, "data: "):
			last = strings.TrimPrefix(line, "data: ")
		}
	}
	if len(names) == 0 || names[len(names)-1] != "done" {
		t.Fatalf("events = %v", names)
	}
	for _, n := range names[:len(names)-1] {
		if n != "event" {
			t.Errorf("unexpected event %q in %v", n, names)
		}
	}
	var done struct {
		SessionID string `json:"session_id"`
		Output    struct {
			Reply string `json:"reply"`
		} `json:"output"`
	}
	if err := json.Unmarshal([]byte(last), &done); err != nil {
		t.Fatalf("done payload %q: %v", last, err)
	}
	if done.SessionID != "s-1" || done.Output.Reply != "all clear" {
		t.Errorf("done = %+v", done)
	}
}

func TestAssistStreamSendsErrorEvent(t *testing.T) {
	ts, repoID := assistServer(t, `{"type":"result","subtype":"error","is_error":true,"result":"usage limit reached"}`)
	req, _ := http.NewRequest("POST", ts.URL+"/v1/repos/"+repoID+"/claude/stream", bytes.NewReader([]byte(`{"prompt":"x"}`)))
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "event: error") || !strings.Contains(string(body), "usage limit reached") {
		t.Fatalf("stream = %s", body)
	}
}

func TestHealthReportsClaudeFeature(t *testing.T) {
	ts, _ := assistServer(t, refineTranscript)
	resp, body := do(t, "GET", ts.URL+"/v1/health", "tok")
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	feats, _ := body["features"].([]any)
	if len(feats) != 1 || feats[0] != "claude" {
		t.Fatalf("features = %v", body["features"])
	}
	// A computer without a usable claude binary offers nothing.
	s := &Server{Config: &config.Config{Token: "tok"}, Assist: &assist.Runner{ClaudePath: "/nonexistent/claude"}}
	if f := s.features(); len(f) != 0 {
		t.Fatalf("features = %v", f)
	}
}
