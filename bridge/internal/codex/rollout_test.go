package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// The fixture is modelled on real Codex CLI 0.159 rollouts (trimmed: the real
// session_meta carries the whole system prompt).
const rolloutFixture = "testdata/rollout.jsonl"

func TestRolloutTailTranslatesItems(t *testing.T) {
	var lifecycle []string
	tail := claude.NewTranscriptTail(rolloutFixture, claude.TranscriptOptions{
		Parse: ParseRolloutEntry,
		Observe: func(p claude.TranscriptParsed) {
			switch p.Type {
			case RolloutTurnStarted, RolloutTurnComplete, RolloutTurnAborted:
				lifecycle = append(lifecycle, p.Type)
			}
		},
	})
	if _, err := tail.Read(); err != nil {
		t.Fatal(err)
	}

	type ev struct{ kind, tool, text string }
	var got []ev
	for _, e := range tail.Events() {
		got = append(got, ev{e.Kind, e.Tool, claude.OneLine(e.Text, 60)})
	}
	want := []ev{
		{api.KindUser, "", "Work on DEMO-bgg."},
		{api.KindText, "", "I'll read the bead, then claim it."},
		{api.KindToolUse, "Bash", "bd show DEMO-bgg; bd update DEMO-bgg --claim"},
		{api.KindToolResult, "", "DEMO-bgg Slow check"},
		{api.KindThinking, "", "Checking the tests first."},
		{api.KindToolUse, "Bash", "python3 -m pytest"},
		{api.KindToolResult, "", "1 failed (exit code 1)"},
		{api.KindToolUse, "Edit", "/code/demo/calc.py, /code/demo/slow.txt"},
		{api.KindToolResult, "", "update /code/demo/calc.py add /code/demo/slow.txt"},
		{api.KindToolUse, "mcp__docs__search", ""},
		{api.KindToolResult, "", "3 hits"},
		{api.KindToolUse, "WebSearch", "pytest fixtures ..."},
		{api.KindToolResult, "", "Fixtures https://docs.pytest.org/fixtures"},
		{api.KindSystem, "", "Context compacted"},
		{api.KindText, "", "Done: slow.txt says ok."},
		{api.KindSystem, "", "Turn interrupted"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d:\n%+v", len(got), len(want), got)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.kind != w.kind || g.tool != w.tool || (w.text != "" && !strings.Contains(g.text, w.text)) {
			t.Errorf("event %d = %+v, want %+v", i, g, w)
		}
	}

	evs := tail.Events()
	var cmd struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(evs[2].Input, &cmd); err != nil || cmd.Command != "bd show DEMO-bgg; bd update DEMO-bgg --claim" {
		t.Errorf("Bash input = %s, want the unwrapped command (the bd scan reads it)", evs[2].Input)
	}
	if evs[3].ToolID != evs[2].ToolID || evs[3].ToolID != "exec-1" {
		t.Errorf("tool ids = %q, %q", evs[2].ToolID, evs[3].ToolID)
	}
	if evs[6].IsError == nil || !*evs[6].IsError || evs[3].IsError == nil || *evs[3].IsError {
		t.Errorf("error flags: failed command %v, passing one %v", evs[6].IsError, evs[3].IsError)
	}
	if string(evs[9].Input) != `{"query":"pytest"}` {
		t.Errorf("MCP input = %s, want the call's arguments", evs[9].Input)
	}

	st := tail.State()
	if st.SessionID != "01a17081-82ba-7853-aaa7-9229fb3a78e2" || st.CWD != "/code/demo" || st.Model != "gpt-6.1-sol" || st.Version != "0.159.2" {
		t.Errorf("meta = %+v", st)
	}
	if st.UserPrompts != 1 || st.ToolCalls != 5 || st.LastText != "Done: slow.txt says ok." {
		t.Errorf("counts = prompts %d, tools %d, last %q", st.UserPrompts, st.ToolCalls, st.LastText)
	}
	if st.CostUSD != 0 {
		t.Errorf("cost = %v; Codex writes none", st.CostUSD)
	}
	if st.LastEntryTS != "2026-10-25T00:45:09.000Z" {
		t.Errorf("last entry = %q, want the turn_aborted stamp", st.LastEntryTS)
	}
	if st.Malformed != 0 || st.Ignored == 0 {
		t.Errorf("malformed %d, ignored %d (response_item, token and world_state lines are ignored)", st.Malformed, st.Ignored)
	}
	if strings.Join(lifecycle, ",") != "task_started,task_complete,task_started,turn_aborted" {
		t.Errorf("lifecycle = %v", lifecycle)
	}
}

func TestReadRolloutMeta(t *testing.T) {
	m, err := ReadRolloutMeta(rolloutFixture)
	if err != nil {
		t.Fatal(err)
	}
	want := RolloutMeta{
		ID: "01a17081-82ba-7853-aaa7-9229fb3a78e2", CWD: "/code/demo",
		Started:    time.Date(2026, 10, 25, 0, 41, 38, 291e6, time.UTC),
		Originator: "codex-tui", Version: "0.159.2", Source: "vscode",
	}
	if m != want {
		t.Fatalf("meta = %+v, want %+v", m, want)
	}

	dir := t.TempDir()
	sub := filepath.Join(dir, "sub.jsonl")
	writeFile(t, sub, `{"timestamp":"2026-10-21T18:30:26Z","type":"session_meta","payload":{"id":"s2","cwd":"/x","originator":"Codex Desktop","source":{"subagent":{"other":"guardian"}},"parent_thread_id":"s1"}}`+"\n")
	if m, err := ReadRolloutMeta(sub); err != nil || !m.Subagent || m.Source != "subagent" {
		t.Fatalf("subagent meta = %+v, %v", m, err)
	}
	notMeta := filepath.Join(dir, "x.jsonl")
	writeFile(t, notMeta, `{"type":"event_msg","payload":{}}`+"\n")
	if _, err := ReadRolloutMeta(notMeta); err == nil {
		t.Fatal("a rollout not starting with session_meta was accepted")
	}
	partial := filepath.Join(dir, "partial.jsonl")
	writeFile(t, partial, `{"type":"session_meta","payload":{"id":"s3"`)
	if _, err := ReadRolloutMeta(partial); err == nil {
		t.Fatal("a half-written first line was accepted")
	}
}

func TestReadSessionIndexLastNameWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session_index.jsonl")
	writeFile(t, path, `{"id":"a","thread_name":"first","updated_at":"2026-10-21T17:13:25Z"}
{"id":"b","thread_name":"other","updated_at":"2026-10-21T17:14:25Z"}
not json
{"id":"a","thread_name":"Work on NAC-7","updated_at":"2026-10-21T18:13:25Z"}
`)
	idx, err := ReadSessionIndex(path)
	if err != nil || idx["a"] != "Work on NAC-7" || idx["b"] != "other" || len(idx) != 2 {
		t.Fatalf("index = %v, %v", idx, err)
	}
	if idx, err := ReadSessionIndex(filepath.Join(t.TempDir(), "missing")); err != nil || len(idx) != 0 {
		t.Fatalf("missing index = %v, %v", idx, err)
	}
}

func TestCommandLine(t *testing.T) {
	for raw, want := range map[string]string{
		`["/bin/zsh","-lc","bd show X"]`: "bd show X",
		`["/bin/bash","-c","ls -la"]`:    "ls -la",
		`["git","status"]`:               "git status",
		`"/bin/zsh -lc 'cat a.txt'"`:     "cat a.txt",
	} {
		if got := commandLine(json.RawMessage(raw)); got != want {
			t.Errorf("commandLine(%s) = %q, want %q", raw, got, want)
		}
	}
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}
