package runs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

func TestActiveCodexGoalResumesSameThreadUntilComplete(t *testing.T) {
	dir := t.TempDir()
	bin, argv := filepath.Join(dir, "codex"), filepath.Join(dir, "argv")
	script := `#!/bin/sh
if [ "$1" = app-server ]; then
 read init
 echo '{"id":1,"result":{}}'
 read initialized
 read request
 if [ -f '` + dir + `/continued' ]; then status=complete; else status=active; fi
 echo '{"id":2,"result":{"goal":{"status":"'"$status"'"}}}'
 cat >/dev/null
else
 echo "$@" >> '` + argv + `'
 if [ "$2" = resume ]; then touch '` + dir + `/continued'; fi
 echo '{"type":"thread.started","thread_id":"thread"}'
 echo '{"type":"turn.completed"}'
fi
`
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	m, err := New(Options{Dir: t.TempDir(), CodexPath: bin})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown(time.Second)
	run, err := m.Start(StartRequest{RepoPath: t.TempDir(), Agent: api.AgentCodex, Goal: "every criterion", Prompt: "work"})
	if err != nil {
		t.Fatal(err)
	}
	done := waitStatus(t, m, run.ID, api.StatusSucceeded, api.StatusFailed)
	if done.Status != api.StatusSucceeded || done.Turns != 2 || done.SessionID != "thread" {
		t.Fatalf("%+v", done)
	}
	data, _ := os.ReadFile(argv)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 || !strings.Contains(string(data), "exec resume --json --enable goals") || !strings.Contains(string(data), "thread Continue working") {
		t.Fatalf("did not resume same goal: %s", data)
	}
}

func TestCodexGoalRequiresNativeCompletion(t *testing.T) {
	for _, state := range []string{"complete", "blocked", "missing"} {
		t.Run(state, func(t *testing.T) {
			bin := filepath.Join(t.TempDir(), "codex")
			goal := `{"status":"` + state + `"}`
			if state == "missing" {
				goal = "null"
			}
			script := `#!/bin/sh
if [ "$1" = app-server ]; then
 read init
 echo '{"id":1,"result":{}}'
 read initialized
 read request
 echo '{"id":2,"result":{"goal":` + goal + `}}'
 cat >/dev/null
else
 echo '{"type":"thread.started","thread_id":"thread"}'
 echo '{"type":"turn.completed"}'
fi
`
			if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			m, err := New(Options{Dir: t.TempDir(), CodexPath: bin})
			if err != nil {
				t.Fatal(err)
			}
			defer m.Shutdown(0)
			run, err := m.Start(StartRequest{RepoPath: t.TempDir(), Agent: api.AgentCodex, Goal: "Every criterion", Prompt: "do the work"})
			if err != nil {
				t.Fatal(err)
			}
			done := waitStatus(t, m, run.ID, api.StatusSucceeded, api.StatusFailed)
			if (done.Status == api.StatusSucceeded) != (state == "complete") {
				t.Fatalf("%s: %+v", state, done)
			}
			if done.GoalText != "Every criterion" {
				t.Fatal("goal lost")
			}
		})
	}
}

func TestClaudeGoalPauseIsNotSuccess(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho '{\"type\":\"result\",\"is_error\":false,\"terminal_reason\":\"goal_paused\",\"result\":\"waiting\"}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	m, err := New(Options{Dir: t.TempDir(), ClaudePath: bin})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown(0)
	run, err := m.Start(StartRequest{RepoPath: t.TempDir(), Goal: "Every criterion", Prompt: "work"})
	if err != nil {
		t.Fatal(err)
	}
	done := waitStatus(t, m, run.ID, api.StatusSucceeded, api.StatusFailed)
	if done.Status != api.StatusFailed || !strings.Contains(done.Error, "goal_paused") {
		t.Fatalf("%+v", done)
	}
}
