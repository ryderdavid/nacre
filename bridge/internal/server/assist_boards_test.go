package server

import (
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

// boardsServer serves the given repo dirs with a fake claude that records its
// working directory and arguments in record, then answers.
func boardsServer(t *testing.T, dirs []string, record string) (*httptest.Server, *config.Config) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "claude")
	script := "#!/bin/sh\npwd > '" + record + "'\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> '" + record + "'; done\n" +
		"cat <<'EOF'\n" + `{"type":"system","subtype":"init","session_id":"s-9"}
{"type":"result","subtype":"success","is_error":false,"session_id":"s-9","result":"{}","structured_output":{"reply":"two boards","proposals":[]}}` + "\nEOF\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	mgr, err := runs.New(runs.Options{Dir: t.TempDir(), ClaudePath: bin})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Token: "tok", DataDir: t.TempDir()}
	s := &Server{
		Config: cfg,
		Repos:  repos.New(func() []string { return dirs }, bd.New("/nonexistent", "")),
		BD:     bd.New("/nonexistent", ""),
		Runs:   mgr,
		Assist: &assist.Runner{ClaudePath: bin},
		Broker: sse.NewBroker(),
		Log:    log.New(io.Discard, "", 0),
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts, cfg
}

func beadsDir(t *testing.T, name string) string {
	t.Helper()
	d := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Join(d, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestAssistAcrossBoardsReadsEachBoardAndReportsTheRest(t *testing.T) {
	nacre, home, bare := beadsDir(t, "nacre"), beadsDir(t, "home"), t.TempDir() // bare has no .beads
	record := filepath.Join(t.TempDir(), "record")
	ts, cfg := boardsServer(t, []string{nacre, home, bare}, record)

	body, _ := json.Marshal(map[string]any{
		"prompt": "what's next everywhere?", "system": "you are the board assistant", "keep": true,
		"boards": []map[string]string{
			{"repo": discovery.RepoID(nacre), "name": "Nacre"},
			{"repo": discovery.RepoID(home), "name": "Home Lab"},
			{"repo": discovery.RepoID(bare), "name": "Bare"},
			{"repo": "nope", "name": "Elsewhere"},
		},
	})
	req, _ := http.NewRequest("POST", ts.URL+"/v1/claude/stream", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	msgs := readSSE(resp.Body, func(sseMsg) bool { return false })
	if len(msgs) < 2 || msgs[0].name != "turn" || msgs[len(msgs)-1].name != "done" {
		t.Fatalf("messages = %+v", msgs)
	}
	var turn assistTurn
	if err := json.Unmarshal([]byte(msgs[0].data), &turn); err != nil {
		t.Fatal(err)
	}
	if turn.TurnID == "" || len(turn.Unreadable) != 2 || turn.Unreadable[0].Name != "Bare" || turn.Unreadable[1].Name != "Elsewhere" {
		t.Fatalf("turn = %+v", turn)
	}
	if !strings.Contains(turn.Unreadable[1].Reason, "not found") {
		t.Errorf("unknown repo reason = %q", turn.Unreadable[1].Reason)
	}

	// The links are in place and point at the boards.
	dir := cfg.AssistBoardsDir()
	for link, want := range map[string]string{"nacre": nacre, "home-lab": home} {
		if got, err := os.Readlink(filepath.Join(dir, link)); err != nil || got != want {
			t.Errorf("link %s = %q, %v; want %q", link, got, err, want)
		}
	}

	raw, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	if got, _ := filepath.EvalSymlinks(lines[0]); got != mustEval(t, dir) {
		t.Errorf("ran in %q, want %q", lines[0], dir)
	}
	args := string(raw)
	for _, want := range []string{"Bash(bd -C nacre list:*)", "Bash(bd -C home-lab show:*)", "`home-lab`: Home Lab", "Elsewhere: not found", "Tell Ryder they weren't read"} {
		if !strings.Contains(args, want) {
			t.Errorf("claude call lacks %q:\n%s", want, args)
		}
	}
	for _, bad := range []string{"Bash(bd list:*)", "Read,Grep", "\nRead\n"} {
		if strings.Contains(args, bad) {
			t.Errorf("claude call grants %q", bad)
		}
	}
}

func TestAssistAcrossBoardsKeepsLinkNamesStable(t *testing.T) {
	a, b := beadsDir(t, "a"), beadsDir(t, "b")
	ts, cfg := boardsServer(t, []string{a, b}, filepath.Join(t.TempDir(), "record"))
	ask := func(boards ...map[string]string) {
		body, _ := json.Marshal(map[string]any{"prompt": "x", "boards": boards})
		req, _ := http.NewRequest("POST", ts.URL+"/v1/claude/stream", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer tok")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		readSSE(resp.Body, func(sseMsg) bool { return false })
		resp.Body.Close()
	}
	// Two boards with the same name get different links, and keep them when asked in the other order.
	ask(map[string]string{"repo": discovery.RepoID(a), "name": "Work"}, map[string]string{"repo": discovery.RepoID(b), "name": "Work"})
	ask(map[string]string{"repo": discovery.RepoID(b), "name": "Work"}, map[string]string{"repo": discovery.RepoID(a), "name": "Work"})
	dir := cfg.AssistBoardsDir()
	if got, _ := os.Readlink(filepath.Join(dir, "work")); got != a {
		t.Errorf("work -> %q, want %q", got, a)
	}
	if got, _ := os.Readlink(filepath.Join(dir, "work-2")); got != b {
		t.Errorf("work-2 -> %q, want %q", got, b)
	}
}

func TestAssistAcrossBoardsRefusesWhenNothingCanBeRead(t *testing.T) {
	ts, _ := boardsServer(t, nil, filepath.Join(t.TempDir(), "record"))
	if resp, _ := post(t, ts.URL+"/v1/claude/stream", `{"prompt":"x"}`); resp.StatusCode != 400 {
		t.Errorf("no boards: %d", resp.StatusCode)
	}
	resp, body := post(t, ts.URL+"/v1/claude/stream", `{"prompt":"x","boards":[{"repo":"nope","name":"Gone"}]}`)
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body["error"].(string), "Gone") {
		t.Errorf("nothing readable: %d %v", resp.StatusCode, body)
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{"Nacre": "nacre", "Home Lab (old)": "home-lab-old", "..": "board", "../etc": "etc", "": "board"} {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	out, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
