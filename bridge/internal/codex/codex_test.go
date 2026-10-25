package codex

import (
	"bufio"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

func TestArgs(t *testing.T) {
	cases := []struct {
		name string
		turn Turn
		want []string
	}{
		{"first turn", Turn{Prompt: "do it", Mode: "acceptEdits", Worktree: true},
			[]string{"exec", "--json", "-c", `sandbox_mode="workspace-write"`, "--worktree", "do it"}},
		{"plan with model", Turn{Prompt: "look", Mode: "plan", Model: "gpt-5"},
			[]string{"exec", "--json", "-c", `sandbox_mode="read-only"`, "--model", "gpt-5", "look"}},
		{"auto gets network", Turn{Prompt: "x", Mode: "auto"},
			[]string{"exec", "--json", "-c", `sandbox_mode="workspace-write"`, "-c", "sandbox_workspace_write.network_access=true", "x"}},
		{"bypass", Turn{Prompt: "x", Mode: "bypassPermissions"},
			[]string{"exec", "--json", "--dangerously-bypass-approvals-and-sandbox", "x"}},
		{"resume keeps mode, drops worktree", Turn{Prompt: "-more", Mode: "plan", Resume: "th-1", Worktree: true},
			[]string{"exec", "resume", "--json", "-c", `sandbox_mode="read-only"`, "th-1", " -more"}},
	}
	for _, c := range cases {
		if got := c.turn.Args(); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: args = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestParseRecordedStream replays a real `codex exec --json` run (codex-cli
// 0.159.2): two shell commands, a file write and a final message.
func TestParseRecordedStream(t *testing.T) {
	f, err := os.Open("testdata/exec.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var p Parser
	var init *claude.Init
	var res *claude.Result
	var evs []api.RunEvent
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		out, err := p.ParseLine(sc.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		if out.Init != nil {
			init = out.Init
		}
		if out.Result != nil {
			res = out.Result
		}
		evs = append(evs, out.Events...)
	}
	if init == nil || init.SessionID != "01a17070-ea99-7d02-8e1f-3b87e8e18184" {
		t.Fatalf("init = %+v", init)
	}
	if res == nil || res.IsError || !strings.Contains(res.Text, "created `b.txt`") {
		t.Fatalf("result = %+v", res)
	}
	var kinds []string
	for _, e := range evs {
		kinds = append(kinds, e.Kind)
	}
	want := "system,system,text,tool_use,tool_result,tool_use,tool_result,tool_use,tool_result,text,result"
	if got := strings.Join(kinds, ","); got != want {
		t.Fatalf("kinds = %s\nwant    %s", got, want)
	}
	use := evs[5]
	if use.Tool != "Bash" || use.ToolID != "item_3" || use.Text != "cat a.txt" {
		t.Fatalf("tool_use = %+v", use)
	}
	var in map[string]string
	if json.Unmarshal(use.Input, &in) != nil || in["command"] != "cat a.txt" {
		t.Fatalf("input = %s", use.Input)
	}
	if r := evs[6]; r.ToolID != "item_3" || r.Text != "hi\n" || *r.IsError {
		t.Fatalf("tool_result = %+v", r)
	}
	if u := evs[7]; u.Text != "printf 'ok' > b.txt" {
		t.Fatalf("double-quoted command = %q", u.Text)
	}
}

func TestParseItemsAndFailures(t *testing.T) {
	var p Parser
	lines := []string{
		`{"type":"item.completed","item":{"id":"r","type":"reasoning","text":"thinking hard"}}`,
		`{"type":"item.started","item":{"id":"c","type":"command_execution","command":"bash -lc 'false'","status":"in_progress"}}`,
		`{"type":"item.updated","item":{"id":"c","type":"command_execution","command":"bash -lc 'false'","status":"in_progress"}}`,
		`{"type":"item.completed","item":{"id":"c","type":"command_execution","command":"bash -lc 'false'","aggregated_output":"","exit_code":1,"status":"failed"}}`,
		`{"type":"item.completed","item":{"id":"f","type":"file_change","changes":[{"path":"a.go","kind":"update"},{"path":"b.go","kind":"add"}],"status":"completed"}}`,
		`{"type":"item.completed","item":{"id":"m","type":"mcp_tool_call","server":"docs","tool":"search","arguments":{"q":"x"},"error":{"message":"denied"},"status":"failed"}}`,
		`{"type":"item.completed","item":{"id":"w","type":"web_search","query":"go generics"}}`,
		`{"type":"item.completed","item":{"id":"t","type":"todo_list","items":[{"text":"a","completed":true},{"text":"b","completed":false}]}}`,
		`{"type":"error","message":"stream disconnected"}`,
		`{"type":"turn.failed","error":{"message":"usage limit reached"}}`,
		``,
		`{"type":"something.new"}`,
	}
	var evs []api.RunEvent
	var res *claude.Result
	for _, l := range lines {
		out, err := p.ParseLine([]byte(l))
		if err != nil {
			t.Fatalf("%s: %v", l, err)
		}
		evs = append(evs, out.Events...)
		if out.Result != nil {
			res = out.Result
		}
	}
	type ev struct{ kind, tool, text string }
	var got []ev
	for _, e := range evs {
		got = append(got, ev{e.Kind, e.Tool, e.Text})
	}
	want := []ev{
		{"thinking", "", "thinking hard"},
		{"tool_use", "Bash", "false"},
		{"tool_result", "", "(exit code 1)"},
		{"tool_use", "Edit", "a.go, b.go"},
		{"tool_result", "", "update a.go\nadd b.go"},
		{"tool_use", "mcp__docs__search", "x"},
		{"tool_result", "", "denied"},
		{"tool_use", "WebSearch", "go generics"},
		{"tool_result", "", ""},
		{"tool_use", "TodoWrite", "2 todos"},
		{"tool_result", "", "1 of 2 done"},
		{"system", "", "Error: stream disconnected"},
		{"result", "", "usage limit reached"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events:\n%q\nwant\n%q", got, want)
	}
	if !*evs[2].IsError || !*evs[6].IsError || *evs[4].IsError {
		t.Fatal("tool_result error flags wrong")
	}
	if res == nil || !res.IsError || res.Text != "usage limit reached" {
		t.Fatalf("result = %+v", res)
	}
	if _, err := p.ParseLine([]byte("not json")); err == nil {
		t.Fatal("want error for malformed line")
	}
}

func TestCheck(t *testing.T) {
	if st := Check(t.Context(), "", nil); st.Available() || !strings.Contains(st.Reason, "isn't installed") {
		t.Fatalf("empty path: %+v", st)
	}
	if st := Check(t.Context(), "/nonexistent/codex", nil); st.Available() {
		t.Fatalf("missing binary: %+v", st)
	}
	dir := t.TempDir()
	fake := func(login string) string {
		p := dir + "/codex-" + login
		script := "#!/bin/sh\ncase \"$1\" in --version) echo 'codex-cli 1.2.3';; login) " + login + ";; esac\n"
		if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if st := Check(t.Context(), fake("echo 'Logged in using ChatGPT'"), nil); !st.Available() || st.Version != "1.2.3" {
		t.Fatalf("signed in: %+v", st)
	}
	if st := Check(t.Context(), fake("echo 'Not logged in'; exit 1"), nil); st.Available() || !strings.Contains(st.Reason, "codex login") || st.Version != "1.2.3" {
		t.Fatalf("signed out: %+v", st)
	}
}

func TestResumeCommand(t *testing.T) {
	if got := ResumeCommand("/a b/it's", "th-1"); got != `cd '/a b/it'\''s' && codex resume th-1` {
		t.Fatal(got)
	}
}
