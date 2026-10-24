package runs

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

// quietClaude prints an init line, goes quiet for a second, then finishes.
func quietClaude(t *testing.T) string {
	t.Helper()
	script := `#!/bin/sh
echo '{"type":"system","subtype":"init","session_id":"sess-q","model":"fake","cwd":"'"$PWD"'"}'
sleep 1
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Back."}]}}'
echo '{"type":"result","subtype":"success","is_error":false,"result":"Finished.","session_id":"sess-q","total_cost_usd":0.01,"duration_ms":1000,"num_turns":1}'
`
	p := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func waitRun(t *testing.T, m *Manager, id string, ok func(api.Run) bool) api.Run {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r, _ := m.Run(id)
		if ok(r) {
			return r
		}
		time.Sleep(10 * time.Millisecond)
	}
	r, _ := m.Run(id)
	t.Fatalf("run never reached the expected state: %+v", r)
	return r
}

func TestQuietRunIsFlaggedStalledAndClearedWhenClaudePrintsAgain(t *testing.T) {
	var (
		mu       sync.Mutex
		notified []api.Run
	)
	m, err := New(Options{
		Dir: t.TempDir(), ClaudePath: quietClaude(t), StallAfter: 200 * time.Millisecond,
		Notify: func(_ string, data any) {
			if r, ok := data.(api.Run); ok {
				mu.Lock()
				notified = append(notified, r)
				mu.Unlock()
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown(time.Second)
	run, err := m.Start(StartRequest{RepoID: "r", RepoPath: t.TempDir(), IssueID: "X-1", Mode: "plan", Prompt: "go"})
	if err != nil {
		t.Fatal(err)
	}

	stalled := waitRun(t, m, run.ID, func(r api.Run) bool { return r.Stalled })
	if stalled.Status != api.StatusRunning || stalled.StalledSince == "" {
		t.Fatalf("stalled run = %+v", stalled)
	}
	done := waitStatus(t, m, run.ID, api.StatusSucceeded)
	if done.Stalled || done.StalledSince != "" {
		t.Fatalf("finished run still stalled: %+v", done)
	}
	_, evs, _ := m.Get(run.ID)
	var notes []string
	for _, e := range evs {
		if e.Kind == api.KindSystem {
			notes = append(notes, e.Text)
		}
	}
	joined := strings.Join(notes, " | ")
	if !strings.Contains(joined, "may be stuck") || !strings.Contains(joined, "printing again") {
		t.Fatalf("system notes = %q", joined)
	}
	// The app hears about the stall without polling.
	mu.Lock()
	defer mu.Unlock()
	sawStall := false
	for _, r := range notified {
		sawStall = sawStall || r.Stalled
	}
	if !sawStall {
		t.Error("no notification carried the stall")
	}
}

func TestStallCheckOffByDefault(t *testing.T) {
	m, err := New(Options{Dir: t.TempDir(), ClaudePath: quietClaude(t)})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown(time.Second)
	run, _ := m.Start(StartRequest{RepoID: "r", RepoPath: t.TempDir(), IssueID: "X-1", Mode: "plan", Prompt: "go"})
	done := waitStatus(t, m, run.ID, api.StatusSucceeded)
	_, evs, _ := m.Get(run.ID)
	for _, e := range evs {
		if strings.Contains(e.Text, "may be stuck") {
			t.Fatalf("stall flagged with the check off: %+v / %s", done, e.Text)
		}
	}
}
