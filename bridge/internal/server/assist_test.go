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
	"strconv"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/assist"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/codex"
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
		"invalid turn id":          {Prompt: "x", TurnID: "t_server-made"},
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
	// Reading the repository is its own opt-in, and still never writes.
	if h, _ := (assistRequest{Prompt: "x"}).headless(); h.ReadRepo {
		t.Error("repo reads granted without read_repo")
	}
	h, _ := (assistRequest{Prompt: "x", BdReads: true, ReadRepo: true}).headless()
	args := strings.Join(h.Args(), " ")
	if !h.ReadRepo || !strings.Contains(args, "Read,Grep,Glob") || strings.Contains(args, "Edit") || strings.Contains(args, "Write") {
		t.Errorf("read_repo call got %q", args)
	}
}

// assistServer serves one repo (a temp dir) with a fake claude binary that
// prints transcript.
func assistServer(t *testing.T, transcript string) (ts *httptest.Server, repoID string) {
	t.Helper()
	return assistServerScript(t, "#!/bin/sh\ncat <<'EOF'\n"+transcript+"\nEOF\n")
}

// assistServerScript serves one repo with script as the claude binary.
func assistServerScript(t *testing.T, script string) (ts *httptest.Server, repoID string) {
	t.Helper()
	ts, repoID, _ = assistServerIn(t, script)
	return ts, repoID
}

// assistServerIn is assistServerScript that also returns the repo's directory.
func assistServerIn(t *testing.T, script string) (ts *httptest.Server, repoID, repoDir string) {
	t.Helper()
	repoDir = t.TempDir()
	bin := filepath.Join(t.TempDir(), "claude")
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
	return ts, discovery.RepoID(repoDir), repoDir
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
	if len(names) < 2 || names[0] != "turn" || names[len(names)-1] != "done" {
		t.Fatalf("events = %v", names)
	}
	for _, n := range names[1 : len(names)-1] {
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

// sseMsg is one parsed server-sent event.
type sseMsg struct{ id, name, data string }

// readSSE parses messages from rd until it ends or stop returns true.
func readSSE(rd io.Reader, stop func(sseMsg) bool) []sseMsg {
	var out []sseMsg
	var cur sseMsg
	br := bufio.NewReader(rd)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return out
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case line == "":
			if cur.name != "" {
				out = append(out, cur)
				if stop(cur) {
					return out
				}
			}
			cur = sseMsg{}
		case strings.HasPrefix(line, "id: "):
			cur.id = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			cur.name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			cur.data = strings.TrimPrefix(line, "data: ")
		}
	}
}

// slowAssist prints one tool call, waits for the go file to appear (or two
// seconds), then a second tool call and the answer.
func slowAssist(t *testing.T) (ts *httptest.Server, repoID, gate string) {
	gate = filepath.Join(t.TempDir(), "go")
	script := `#!/bin/sh
echo '{"type":"system","subtype":"init","session_id":"s-9"}'
echo '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"bd list"}}]}}'
i=0; while [ ! -f ` + gate + ` ] && [ $i -lt 40 ]; do sleep 0.05; i=$((i+1)); done
echo '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"bd ready"}}]}}'
echo '{"type":"result","subtype":"success","is_error":false,"session_id":"s-9","result":"{}","structured_output":{"reply":"survived"}}'
`
	ts, repoID = assistServerScript(t, script)
	return ts, repoID, gate
}

func TestAssistTurnSurvivesDroppedConnection(t *testing.T) {
	ts, repoID, gate := slowAssist(t)

	req, _ := http.NewRequest("POST", ts.URL+"/v1/repos/"+repoID+"/claude/stream", strings.NewReader(`{"prompt":"x"}`))
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	// Read the turn id and the first event, then drop the connection.
	first := readSSE(resp.Body, func(m sseMsg) bool { return m.name == "event" })
	resp.Body.Close()
	if len(first) < 2 || first[0].name != "turn" {
		t.Fatalf("first messages = %+v", first)
	}
	var turn struct {
		TurnID string `json:"turn_id"`
	}
	_ = json.Unmarshal([]byte(first[0].data), &turn)
	lastSeq := first[len(first)-1].id
	if turn.TurnID == "" || lastSeq == "" {
		t.Fatalf("turn %+v, seq %q", turn, lastSeq)
	}
	// claude carries on without anyone listening.
	if err := os.WriteFile(gate, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	req, _ = http.NewRequest("GET", ts.URL+"/v1/assist/"+turn.TurnID+"/stream?after="+lastSeq, nil)
	req.Header.Set("Authorization", "Bearer tok")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	rest := readSSE(resp.Body, func(sseMsg) bool { return false })
	if len(rest) == 0 || rest[len(rest)-1].name != "done" || !strings.Contains(rest[len(rest)-1].data, "survived") {
		t.Fatalf("after reattach = %+v", rest)
	}
	seen, _ := strconv.Atoi(lastSeq)
	for _, m := range rest[:len(rest)-1] {
		if n, _ := strconv.Atoi(m.id); n <= seen {
			t.Errorf("replayed seq %s already seen (after %s)", m.id, lastSeq)
		}
	}

	// An unknown turn is a 404.
	if resp, _ := do(t, "GET", ts.URL+"/v1/assist/t_nope/stream", "tok"); resp.StatusCode != 404 {
		t.Errorf("unknown turn: %d", resp.StatusCode)
	}
}

// A phone whose connection dropped before the "turn" message arrived can't
// tell whether its request got here, so it sends it again with the same turn
// id. That must attach to the turn already running, never start a second
// claude for the same question.
func TestAssistResendWithTheSameTurnIDAttaches(t *testing.T) {
	gate := filepath.Join(t.TempDir(), "go")
	runs := filepath.Join(t.TempDir(), "runs")
	script := `#!/bin/sh
echo run >> ` + runs + `
echo '{"type":"system","subtype":"init","session_id":"s-9"}'
echo '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"bd list"}}]}}'
i=0; while [ ! -f ` + gate + ` ] && [ $i -lt 40 ]; do sleep 0.05; i=$((i+1)); done
echo '{"type":"result","subtype":"success","is_error":false,"session_id":"s-9","result":"{}","structured_output":{"reply":"once"}}'
`
	ts, repoID := assistServerScript(t, script)
	send := func() *http.Response {
		req, _ := http.NewRequest("POST", ts.URL+"/v1/repos/"+repoID+"/claude/stream", strings.NewReader(`{"prompt":"x","turn_id":"c_0123456789ab"}`))
		req.Header.Set("Authorization", "Bearer tok")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	resp := send()
	first := readSSE(resp.Body, func(m sseMsg) bool { return m.name == "turn" })
	resp.Body.Close()
	if len(first) != 1 || !strings.Contains(first[0].data, `"c_0123456789ab"`) {
		t.Fatalf("first = %+v", first)
	}

	resp = send()
	defer resp.Body.Close()
	if err := os.WriteFile(gate, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	again := readSSE(resp.Body, func(sseMsg) bool { return false })
	if len(again) < 3 || again[0].name != "turn" || !strings.Contains(again[0].data, `"c_0123456789ab"`) {
		t.Fatalf("resend = %+v", again)
	}
	// It replays the turn from its first event, so nothing is missing either.
	if again[1].name != "event" || again[1].id != "1" {
		t.Errorf("resend starts at %+v, want seq 1", again[1])
	}
	if last := again[len(again)-1]; last.name != "done" || !strings.Contains(last.data, "once") {
		t.Fatalf("resend ended with %+v", last)
	}
	b, _ := os.ReadFile(runs)
	if n := strings.Count(string(b), "run"); n != 1 {
		t.Fatalf("claude ran %d times for one question", n)
	}
}

func TestAssistTurnCancel(t *testing.T) {
	ts, repoID, _ := slowAssist(t)
	req, _ := http.NewRequest("POST", ts.URL+"/v1/repos/"+repoID+"/claude/stream", strings.NewReader(`{"prompt":"x"}`))
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	cancelled := false
	msgs := readSSE(resp.Body, func(m sseMsg) bool {
		if m.name == "turn" && !cancelled {
			var turn struct {
				TurnID string `json:"turn_id"`
			}
			_ = json.Unmarshal([]byte(m.data), &turn)
			if r, _ := post(t, ts.URL+"/v1/assist/"+turn.TurnID+"/cancel", ""); r.StatusCode != 204 {
				t.Errorf("cancel: %d", r.StatusCode)
			}
			cancelled = true
		}
		return false
	})
	if last := msgs[len(msgs)-1]; last.name != "error" {
		t.Fatalf("cancelled turn ended with %+v", last)
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
	if len(feats) != 3 || feats[0] != "claude" || feats[1] != "chat_images" || feats[2] != "assist_turn_ids" {
		t.Fatalf("features = %v", body["features"])
	}
	// A computer without a usable claude binary offers nothing.
	s := &Server{Config: &config.Config{Token: "tok"}, Assist: &assist.Runner{ClaudePath: "/nonexistent/claude"}}
	if f := s.features(codex.Status{Reason: "not installed"}); len(f) != 0 {
		t.Fatalf("features = %v", f)
	}
}
