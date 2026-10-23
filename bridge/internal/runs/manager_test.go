package runs

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

// fakeClaude writes a shell script that mimics `claude -p … --output-format
// stream-json`: it prints an init line, an assistant tool_use + text, a tool
// result and a result line. If the prompt contains "SLEEP" it sleeps instead
// (for cancel tests); "FAIL" makes it exit 3 after writing to stderr.
func fakeClaude(t *testing.T) string {
	t.Helper()
	script := `#!/bin/sh
prompt="$2"
case "$prompt" in
  *SLEEP*) echo '{"type":"system","subtype":"init","session_id":"sess-1","model":"fake","cwd":"'"$PWD"'"}'; sleep 30; exit 0;;
  *FAIL*) echo "boom on stderr" >&2; exit 3;;
esac
resume=""
while [ $# -gt 0 ]; do [ "$1" = "--resume" ] && resume="$2"; shift; done
echo '{"type":"system","subtype":"init","session_id":"sess-1","model":"fake","cwd":"'"$PWD"'"}'
echo '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]}}'
printf '%s\n' '{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"a\nb"}]}}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"All done (resume='"$resume"')."}]}}'
echo 'not json'
echo '{"type":"result","subtype":"success","is_error":false,"result":"Finished.","session_id":"sess-1","total_cost_usd":0.25,"duration_ms":1000,"num_turns":2}'
`
	p := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func waitStatus(t *testing.T, m *Manager, id string, want ...string) api.Run {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		r, err := m.Run(id)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range want {
			if r.Status == w {
				return r
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	r, _ := m.Run(id)
	t.Fatalf("run %s status %q, want %v", id, r.Status, want)
	return r
}

func kinds(evs []api.RunEvent) string {
	var k []string
	for _, e := range evs {
		s := e.Kind
		if e.Kind == api.KindStatus {
			s += ":" + e.Status
		}
		k = append(k, s)
	}
	return strings.Join(k, ",")
}

func TestRunLifecycleFollowupAndPersistence(t *testing.T) {
	dir := t.TempDir()
	repo := t.TempDir()
	var mu sync.Mutex
	var notified []string
	m, err := New(Options{Dir: dir, ClaudePath: fakeClaude(t), MaxConcurrent: 1,
		Notify: func(ev string, data any) {
			mu.Lock()
			notified = append(notified, data.(api.Run).Status)
			mu.Unlock()
		}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := m.Start(StartRequest{RepoID: "rid", RepoName: "R", RepoPath: repo, IssueID: "X-1", IssueTitle: "T", Mode: "plan", Prompt: "do it"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(r.ID, "r_") || len(r.ID) != 28 {
		t.Fatalf("id %q", r.ID)
	}
	done := waitStatus(t, m, r.ID, api.StatusSucceeded, api.StatusFailed)
	if done.Status != api.StatusSucceeded || done.SessionID != "sess-1" || done.CostUSD != 0.25 ||
		done.NumTurns != 2 || done.ToolCalls != 1 || done.ResultText != "Finished." || done.Turns != 1 {
		t.Fatalf("run = %+v", done)
	}
	if !strings.Contains(done.LastText, "All done") || !strings.Contains(done.ResumeCommand, "claude --resume sess-1") {
		t.Fatalf("run = %+v", done)
	}
	_, evs, _ := m.Get(r.ID)
	want := "status:queued,user,status:running,system,tool_use,tool_result,text,stderr,result,status:succeeded"
	if got := kinds(evs); got != want {
		t.Fatalf("events\n got  %s\n want %s", got, want)
	}
	for i, e := range evs {
		if e.Seq != int64(i+1) {
			t.Fatalf("seq %d at %d", e.Seq, i)
		}
	}

	// Follow-up resumes the session and continues seq numbering.
	if _, err := m.Followup(r.ID, "more", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Followup(r.ID, "again", ""); err == nil {
		t.Fatal("follow-up while queued/running should conflict")
	}
	done = waitStatus(t, m, r.ID, api.StatusSucceeded, api.StatusFailed)
	if done.Turns != 2 || done.CostUSD != 0.5 || done.NumTurns != 4 || !strings.Contains(done.LastText, "resume=sess-1") {
		t.Fatalf("after follow-up: %+v", done)
	}
	_, evs, _ = m.Get(r.ID)
	if evs[10].Status != api.StatusQueued || evs[11].Kind != api.KindUser || evs[11].Text != "more" || evs[11].Seq != 12 {
		t.Fatalf("follow-up events = %+v", evs[10:12])
	}
	mu.Lock()
	// Global notifications arrive in status order for each turn.
	if len(notified) < 4 || notified[0] != api.StatusQueued || notified[1] != api.StatusRunning ||
		notified[len(notified)-1] != api.StatusSucceeded {
		t.Fatalf("notifications = %v", notified)
	}
	mu.Unlock()

	// History survives a restart.
	m2, err := New(Options{Dir: dir, ClaudePath: "/nonexistent"})
	if err != nil {
		t.Fatal(err)
	}
	r2, evs2, err := m2.Get(r.ID)
	if err != nil || r2.Status != api.StatusSucceeded || len(evs2) != len(evs) {
		t.Fatalf("reload: %v %+v %d/%d", err, r2, len(evs2), len(evs))
	}
}

func TestCancelAndQueue(t *testing.T) {
	m, err := New(Options{Dir: t.TempDir(), ClaudePath: fakeClaude(t), MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	a, _ := m.Start(StartRequest{RepoID: "rid", RepoPath: repo, IssueID: "X-1", Mode: "auto", Prompt: "SLEEP"})
	b, _ := m.Start(StartRequest{RepoID: "rid", RepoPath: repo, IssueID: "X-2", Mode: "auto", Prompt: "SLEEP"})
	waitStatus(t, m, a.ID, api.StatusRunning)
	if got, _ := m.Run(b.ID); got.Status != api.StatusQueued {
		t.Fatalf("second run should be queued, is %s", got.Status)
	}
	if n := m.ActiveCount("rid"); n != 2 {
		t.Fatalf("active = %d", n)
	}
	// Cancel the queued one: immediate.
	if c, _ := m.Cancel(b.ID); c.Status != api.StatusCancelled {
		t.Fatalf("queued cancel: %s", c.Status)
	}
	// Wait for the session id so we know the process is up, then cancel.
	deadline := time.Now().Add(5 * time.Second)
	for {
		r, _ := m.Run(a.ID)
		if r.SessionID != "" || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	start := time.Now()
	if _, err := m.Cancel(a.ID); err != nil {
		t.Fatal(err)
	}
	got := waitStatus(t, m, a.ID, api.StatusCancelled, api.StatusFailed, api.StatusSucceeded)
	if got.Status != api.StatusCancelled || time.Since(start) > 4*time.Second {
		t.Fatalf("cancel: %+v after %s", got, time.Since(start))
	}
	// Follow-up on a cancelled run with a session is allowed.
	if _, err := m.Followup(a.ID, "resume please", "plan"); err != nil {
		t.Fatal(err)
	}
	waitStatus(t, m, a.ID, api.StatusSucceeded)
}

func TestFailedRunCapturesStderr(t *testing.T) {
	m, _ := New(Options{Dir: t.TempDir(), ClaudePath: fakeClaude(t)})
	r, _ := m.Start(StartRequest{RepoID: "rid", RepoPath: t.TempDir(), IssueID: "X-1", Prompt: "FAIL"})
	got := waitStatus(t, m, r.ID, api.StatusFailed, api.StatusSucceeded)
	if got.Status != api.StatusFailed || got.Error != "claude exited with status 3: boom on stderr" {
		t.Fatalf("got %+v", got)
	}
	if _, err := m.Followup(r.ID, "x", ""); err == nil {
		t.Fatal("follow-up without a session should conflict")
	}
}

func TestRestartMarksActiveRunsFailed(t *testing.T) {
	dir := t.TempDir()
	meta := api.Run{ID: newID(time.Now()), Status: api.StatusRunning, IssueID: "X-1"}
	rd := filepath.Join(dir, meta.ID)
	if err := writeMeta(rd, meta); err != nil {
		t.Fatal(err)
	}
	_ = appendEvent(rd, api.RunEvent{Seq: 1, Kind: api.KindStatus, Status: api.StatusRunning})
	m, err := New(Options{Dir: dir, ClaudePath: "/nonexistent"})
	if err != nil {
		t.Fatal(err)
	}
	r, evs, _ := m.Get(meta.ID)
	if r.Status != api.StatusFailed || r.Error != "bridge restarted" || len(evs) != 2 || evs[1].Seq != 2 {
		t.Fatalf("got %+v %+v", r, evs)
	}
}

func TestPrune(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	var ids []string
	for i := 0; i < 5; i++ {
		id := newID(base.Add(time.Duration(i) * time.Second))
		ids = append(ids, id)
		status := api.StatusSucceeded
		if i == 0 {
			status = api.StatusFailed
		}
		_ = writeMeta(filepath.Join(dir, id), api.Run{ID: id, Status: status})
	}
	m, err := New(Options{Dir: dir, ClaudePath: "/x", Keep: 3})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, r := range m.List(Filter{}) {
		got = append(got, r.ID)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(ids[2:], ",") {
		t.Fatalf("kept %v want %v", got, ids[2:])
	}
	if _, err := os.Stat(filepath.Join(dir, ids[0])); !os.IsNotExist(err) {
		t.Fatal("pruned dir still exists")
	}
}

func TestNewIDSortable(t *testing.T) {
	a := newID(time.UnixMilli(1_700_000_000_000))
	b := newID(time.UnixMilli(1_700_000_000_001))
	c := newID(time.UnixMilli(1_800_000_000_000))
	if !(a < b && b < c) {
		t.Fatalf("not sortable: %s %s %s", a, b, c)
	}
	if len(a) != 28 {
		t.Fatalf("unexpected id %s", a)
	}
}
