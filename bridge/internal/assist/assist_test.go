package assist

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// fakeClaude writes a script that prints body (a stream-json transcript) and
// exits with code. It also records its argv and cwd for assertions.
func fakeClaude(t *testing.T, body string, code int) (path, argvFile string) {
	t.Helper()
	dir := t.TempDir()
	path = filepath.Join(dir, "claude")
	argvFile = filepath.Join(dir, "argv")
	script := "#!/bin/sh\n" +
		"{ pwd; for a in \"$@\"; do echo \"$a\"; done; } > " + argvFile + "\n" +
		"cat <<'EOF'\n" + body + "\nEOF\n" +
		"exit " + itoa(code) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path, argvFile
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	return string(rune('0' + n))
}

const successTranscript = `{"type":"system","subtype":"init","session_id":"s-1","model":"claude-opus-5-5","cwd":"/repo"}
{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"checking the board"}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"bd list --status=open"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"NAC-1 open"}]}}
{"type":"result","subtype":"success","is_error":false,"session_id":"s-2","result":"{\"reply\":\"one bead\",\"proposals\":[]}","structured_output":{"reply":"one bead","proposals":[]},"total_cost_usd":0.004,"duration_ms":1200}`

func TestRunStreamsEventsAndReturnsStructuredOutput(t *testing.T) {
	path, argvFile := fakeClaude(t, successTranscript, 0)
	r := &Runner{ClaudePath: path, Timeout: 20 * time.Second}
	cwd := t.TempDir()

	var kinds []string
	out, err := r.Run(context.Background(), cwd, claude.Headless{
		Prompt: "what's next?", Schema: json.RawMessage(`{"type":"object"}`), AllowedTools: claude.BdReadTools, Persist: true,
	}, func(ev api.RunEvent) { kinds = append(kinds, ev.Kind) })
	if err != nil {
		t.Fatal(err)
	}

	// The session id from the result wins, so the next turn resumes the latest one.
	if out.SessionID != "s-2" {
		t.Errorf("session = %q, want s-2", out.SessionID)
	}
	var got struct{ Reply string }
	if err := json.Unmarshal(out.Output, &got); err != nil || got.Reply != "one bead" {
		t.Errorf("output = %s (%v)", out.Output, err)
	}
	if out.CostUSD != 0.004 || out.DurationMS != 1200 {
		t.Errorf("cost/duration = %v %v", out.CostUSD, out.DurationMS)
	}
	want := []string{api.KindSystem, api.KindThinking, api.KindToolUse, api.KindToolResult, api.KindResult}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Errorf("events = %v, want %v", kinds, want)
	}

	// The call ran in the repo, with the flags we asked for.
	argv, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(argv)), "\n")
	if evaled, err := filepath.EvalSymlinks(cwd); err == nil && lines[0] != evaled && lines[0] != cwd {
		t.Errorf("cwd = %q, want %q", lines[0], cwd)
	}
	if !strings.Contains(string(argv), "--safe-mode") {
		t.Errorf("argv = %s", argv)
	}
}

func TestRunReportsClaudeError(t *testing.T) {
	const transcript = `{"type":"result","subtype":"error_during_execution","is_error":true,"result":"rate limited, try later"}`
	path, _ := fakeClaude(t, transcript, 0)
	r := &Runner{ClaudePath: path}
	_, err := r.Run(context.Background(), t.TempDir(), claude.Headless{Prompt: "x"}, nil)
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("err = %v, want the claude message", err)
	}
}

func TestRunReportsMissingResult(t *testing.T) {
	path, _ := fakeClaude(t, `{"type":"system","subtype":"init","session_id":"s"}`, 1)
	r := &Runner{ClaudePath: path}
	_, err := r.Run(context.Background(), t.TempDir(), claude.Headless{Prompt: "x"}, nil)
	if err == nil || !strings.Contains(err.Error(), "status 1") {
		t.Fatalf("err = %v, want the exit status", err)
	}
}

func TestRunRequiresStructuredOutputWhenSchemaGiven(t *testing.T) {
	const transcript = `{"type":"result","subtype":"success","is_error":false,"result":"just prose"}`
	path, _ := fakeClaude(t, transcript, 0)
	r := &Runner{ClaudePath: path}
	_, err := r.Run(context.Background(), t.TempDir(), claude.Headless{Prompt: "x", Schema: json.RawMessage(`{"type":"object"}`)}, nil)
	if err == nil || !strings.Contains(err.Error(), "structured output") {
		t.Fatalf("err = %v", err)
	}
}

func TestAvailable(t *testing.T) {
	path, _ := fakeClaude(t, "", 0)
	for p, want := range map[string]bool{path: true, "/nonexistent/claude": false, "": false} {
		if got := (&Runner{ClaudePath: p}).Available(); got != want {
			t.Errorf("Available(%q) = %v", p, got)
		}
	}
}
