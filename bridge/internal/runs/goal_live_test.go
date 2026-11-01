package runs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

// Opt-in: spends a small amount of the signed-in Codex subscription. It proves
// the continuation that codex exec does not perform on its own, with no writes
// outside a temporary repository/profile and no remote or commit operations.
func TestLiveCodexGoalContinuation(t *testing.T) {
	if os.Getenv("NACRE_LIVE_CODEX_TEST") != "1" {
		t.Skip("set NACRE_LIVE_CODEX_TEST=1 for the native CLI smoke test")
	}
	bin, err := exec.LookPath("codex")
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	profile := t.TempDir()
	if err := os.Symlink(filepath.Join(home, ".codex", "auth.json"), filepath.Join(profile, "auth.json")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", profile)
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(repo, "counter.txt"), []byte("0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := New(Options{Dir: t.TempDir(), CodexPath: bin})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown(5 * time.Second)
	r, err := m.Start(StartRequest{RepoPath: repo, Agent: api.AgentCodex, Mode: "acceptEdits",
		Goal:   "The file counter.txt contains exactly the integer 2. This goal is complete only once that file is verified to contain 2.",
		Prompt: "Native continuation smoke test. On EACH turn read counter.txt and increment it by exactly 1, only once per turn, then END THAT TURN with a progress message. Do not increment twice in one turn. Keep the goal active if the counter is only 1 so Nacre resumes you. Do not mark the goal complete until it contains 2. Once it is 2, mark the native goal complete and reply NACRE_LOOP_OK. No network, plugins, subagents, commits, pushes, or changes outside this temporary directory."})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		r, _ = m.Run(r.ID)
		if !r.Active() {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	data, err := os.ReadFile(filepath.Join(repo, "counter.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != api.StatusSucceeded || r.Turns < 2 || strings.TrimSpace(string(data)) != "2" {
		t.Fatalf("status=%s turns=%d counter=%s error=%s", r.Status, r.Turns, data, r.Error)
	}
	t.Logf("native goal completed in %d turns; counter=2; same session=%s", r.Turns, r.SessionID)
}
