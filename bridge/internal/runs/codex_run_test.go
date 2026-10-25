package runs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

// fakeCodex mimics `codex exec [resume] --json … [thread] prompt`: it prints
// a thread id, a command item and a final message, and records its argv. A
// prompt containing "SLEEP" sleeps instead (for cancel) and notes the SIGINT
// that stops it, which is the signal real Codex cleans up its commands on;
// "FAIL" reports a failed turn.
func fakeCodex(t *testing.T) (path, argvLog string) {
	t.Helper()
	dir := t.TempDir()
	argvLog = filepath.Join(dir, "argv")
	script := `#!/bin/sh
echo "$@" >> '` + argvLog + `'
for last in "$@"; do :; done
echo '{"type":"thread.started","thread_id":"th-1"}'
echo '{"type":"turn.started"}'
case "$last" in
  *SLEEP*) trap 'echo INT >> '"'"'` + argvLog + `.sig'"'"'; exit 130' INT; sleep 30 & wait; exit 0;;
  *FAIL*) echo '{"type":"turn.failed","error":{"message":"usage limit"}}'; exit 1;;
esac
echo '{"type":"item.started","item":{"id":"item_1","type":"command_execution","command":"/bin/zsh -lc ls","status":"in_progress"}}'
printf '%s\n' '{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"/bin/zsh -lc ls","aggregated_output":"a\n","exit_code":0,"status":"completed"}}'
echo 'stderr noise {not json' >&2
echo '{"type":"item.completed","item":{"id":"item_2","type":"agent_message","text":"Done in '"$PWD"'."}}'
echo '{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":2}}'
`
	path = filepath.Join(dir, "codex")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path, argvLog
}

func TestCodexRunLifecycle(t *testing.T) {
	codexPath, argvLog := fakeCodex(t)
	repo := t.TempDir()
	m, err := New(Options{Dir: t.TempDir(), ClaudePath: "/nonexistent/claude", CodexPath: codexPath, MaxConcurrent: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown(0)
	r, err := m.Start(StartRequest{RepoID: "rid", RepoName: "R", RepoPath: repo, IssueID: "X-1", Mode: "auto", Prompt: "do it", Agent: api.AgentCodex})
	if err != nil {
		t.Fatal(err)
	}
	if r.Agent != api.AgentCodex {
		t.Fatalf("agent = %q", r.Agent)
	}
	done := waitStatus(t, m, r.ID, api.StatusSucceeded, api.StatusFailed)
	if done.Status != api.StatusSucceeded || done.SessionID != "th-1" || done.ToolCalls != 1 || done.NumTurns != 1 {
		t.Fatalf("run = %+v", done)
	}
	if !strings.HasPrefix(done.ResultText, "Done in ") || done.LastText != done.ResultText {
		t.Fatalf("result %q, last %q", done.ResultText, done.LastText)
	}
	if done.ResumeCommand != "cd '"+repo+"' && codex resume th-1" {
		t.Fatalf("resume = %q", done.ResumeCommand)
	}
	_, evs, _ := m.Get(r.ID)
	var out []api.RunEvent
	stderr := 0
	for _, e := range evs {
		if e.Kind == api.KindStderr {
			stderr++ // stderr interleaves with stdout in any order
			continue
		}
		out = append(out, e)
	}
	if got := kinds(out); stderr != 1 || got != "status:queued,user,status:running,system,tool_use,tool_result,text,result,status:succeeded" {
		t.Fatalf("events = %s", got)
	}

	if _, err := m.Followup(r.ID, "more", ""); err != nil {
		t.Fatal(err)
	}
	if f := waitStatus(t, m, r.ID, api.StatusSucceeded, api.StatusFailed); f.Status != api.StatusSucceeded || f.Turns != 2 || f.NumTurns != 2 {
		t.Fatalf("followup run = %+v", f)
	}
	argv, _ := os.ReadFile(argvLog)
	lines := strings.Split(strings.TrimSpace(string(argv)), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "exec --json ") || !strings.HasPrefix(lines[1], "exec resume --json ") || !strings.HasSuffix(lines[1], "th-1 more") {
		t.Fatalf("argv = %q", lines)
	}
}

func TestCodexRunFailsAndCancels(t *testing.T) {
	codexPath, argvLog := fakeCodex(t)
	m, err := New(Options{Dir: t.TempDir(), CodexPath: codexPath, MaxConcurrent: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown(0)
	f, _ := m.Start(StartRequest{RepoPath: t.TempDir(), IssueID: "X-1", Mode: "auto", Prompt: "FAIL", Agent: api.AgentCodex})
	if got := waitStatus(t, m, f.ID, api.StatusFailed, api.StatusSucceeded); got.Status != api.StatusFailed || got.Error != "usage limit" {
		t.Fatalf("failed run = %+v", got)
	}
	s, _ := m.Start(StartRequest{RepoPath: t.TempDir(), IssueID: "X-2", Mode: "auto", Prompt: "SLEEP", Agent: api.AgentCodex})
	waitStatus(t, m, s.ID, api.StatusRunning)
	// Wait for the thread id so cancel lands on a live process.
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if r, _ := m.Run(s.ID); r.SessionID != "" {
			break
		}
	}
	if _, err := m.Cancel(s.ID); err != nil {
		t.Fatal(err)
	}
	if got := waitStatus(t, m, s.ID, api.StatusCancelled, api.StatusFailed); got.Status != api.StatusCancelled {
		t.Fatalf("cancelled run = %+v", got)
	}
	if sig, _ := os.ReadFile(argvLog + ".sig"); strings.TrimSpace(string(sig)) != "INT" {
		t.Fatalf("codex was not interrupted with SIGINT (got %q)", sig)
	}
}

func TestCodexExitWithoutResult(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "codex")
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho 'Not logged in' >&2\nexit 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := New(Options{Dir: t.TempDir(), CodexPath: p})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown(0)
	r, _ := m.Start(StartRequest{RepoPath: t.TempDir(), IssueID: "X-1", Prompt: "x", Agent: api.AgentCodex})
	if got := waitStatus(t, m, r.ID, api.StatusFailed, api.StatusSucceeded); got.Error != "codex exited with status 2: Not logged in" {
		t.Fatalf("error = %q", got.Error)
	}
}
