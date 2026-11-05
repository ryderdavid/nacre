package chat

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/chat/fakeclaude"
	"github.com/ryderdavid/nacre/bridge/internal/chat/fakecodex"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

func TestMain(m *testing.M) {
	fakeclaude.RunIfFake()
	fakecodex.RunIfFake()
	os.Exit(m.Run())
}

// promptChat starts a chat turn on the fixture-replaying CLI and waits for
// its permission request.
func promptChat(t *testing.T, opts ...func(*Options)) (m *Manager, id, fdir string, perm Event) {
	t.Helper()
	bin, fdir := fakeclaude.Install(t, "testdata")
	m = newManager(t, t.TempDir(), bin, opts...)
	rec, err := m.Create(CreateRequest{Agent: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_perm0001", Text: "push it"}); err != nil {
		t.Fatal(err)
	}
	return m, rec.ID, fdir, waitPermission(t, m, rec.ID)
}

func waitPermission(t *testing.T, m *Manager, id string) Event {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		snap, err := m.Since(id, 0)
		if err != nil {
			t.Fatal(err)
		}
		if snap.Record.State == StateWaitingPermission {
			for _, ev := range snap.Events {
				if ev.Kind == KindPermission {
					return ev
				}
			}
		}
		select {
		case <-snap.Changed:
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatal("no permission request")
	return Event{}
}

// answerLine is the control_response the fake CLI got.
type answerLine struct {
	Type     string
	Response struct {
		Subtype   string
		RequestID string `json:"request_id"`
		Response  struct {
			Behavior     string
			Message      string
			UpdatedInput json.RawMessage `json:"updatedInput"`
		}
	}
}

func readAnswer(t *testing.T, fdir string) answerLine {
	t.Helper()
	var a answerLine
	line := readLines(t, filepath.Join(fdir, "answer"))[0]
	if err := json.Unmarshal([]byte(line), &a); err != nil {
		t.Fatal(err)
	}
	return a
}

func eventsOf(t *testing.T, m *Manager, id string, kind string) []Event {
	t.Helper()
	_, evs, err := m.Get(id, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []Event
	for _, ev := range evs {
		if ev.Kind == kind {
			out = append(out, ev)
		}
	}
	return out
}

// noTurnLeft checks that nothing of the turn is still running: the manager
// counts no active turn and the CLI process is gone.
func noTurnLeft(t *testing.T, m *Manager, fdir string) {
	t.Helper()
	if m.Busy() {
		t.Error("a chat turn is still counted as running")
	}
	data, err := os.ReadFile(filepath.Join(fdir, "pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(string(data))
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("claude process %d is still running", pid)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestFixtureParsesIntoAPermissionEvent(t *testing.T) {
	data, err := os.ReadFile("testdata/host_prompt_deny.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var req claude.ControlRequest
	found := false
	for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var r struct {
			Dir  string
			Line json.RawMessage
		}
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatal(err)
		}
		if r.Dir == "stdout" {
			if req, found = claude.ParseControlRequest(r.Line); found {
				break
			}
		}
	}
	if !found {
		t.Fatal("the recording has no control_request")
	}
	if req.Subtype != "can_use_tool" || req.RequestID != "dd5c7040-e921-4dcf-9fc9-9838523a2cb9" || req.ToolName != "Bash" ||
		req.ToolID != "toolu_013DCm61FdFYqipEzXettzRr" {
		t.Fatalf("request = %+v", req)
	}
	if got := PermissionSummary(req); got != "Bash: git push --force origin HEAD:main" {
		t.Errorf("summary = %q", got)
	}
	if req.Reason != "A permission rule asks before this step (flagSettings)" {
		t.Errorf("reason = %q", req.Reason)
	}
	mcp := PermissionSummary(claude.ControlRequest{ToolName: "mcp__github__create_issue", Input: json.RawMessage(`{"title":"x"}`)})
	if mcp != "MCP tool create_issue on github" {
		t.Errorf("mcp summary = %q", mcp)
	}
	classifier, _ := claude.ParseControlRequest([]byte(`{"type":"control_request","request_id":"r","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"x"},"decision_reason":"deletes files"}}`))
	if classifier.Reason != "deletes files" {
		t.Errorf("classifier reason = %q", classifier.Reason)
	}
}

func TestPermissionRequestWaitsForThePhone(t *testing.T) {
	m, id, fdir, perm := promptChat(t)
	if perm.RequestID != "dd5c7040-e921-4dcf-9fc9-9838523a2cb9" || perm.Tool != "Bash" ||
		perm.Text != "Bash: git push --force origin HEAD:main" || perm.Reason == "" || perm.Allowed != nil {
		t.Fatalf("permission event = %+v", perm)
	}
	rec, _, _ := m.Get(id, 0)
	if rec.State != StateWaitingPermission {
		t.Fatalf("state = %s", rec.State)
	}
	if _, err := os.Stat(filepath.Join(fdir, "answer")); err == nil {
		t.Fatal("the request was answered without the phone")
	}
	if !m.Busy() {
		t.Fatal("a waiting turn must count as running")
	}
	m.Cancel(id)
	waitIdle(t, m, id)
}

func TestAllowLetsTheStepRun(t *testing.T) {
	m, id, fdir, perm := promptChat(t)
	ev, err := m.Answer(id, perm.RequestID, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if ev.Kind != KindPermissionAnswer || ev.Allowed == nil || !*ev.Allowed || ev.By != ByPhone {
		t.Fatalf("answer event = %+v", ev)
	}
	rec := waitIdle(t, m, id)
	if rec.State != StateIdle {
		t.Fatalf("state = %s", rec.State)
	}
	a := readAnswer(t, fdir)
	if a.Type != "control_response" || a.Response.RequestID != perm.RequestID || a.Response.Response.Behavior != "allow" ||
		!strings.Contains(string(a.Response.Response.UpdatedInput), "git push --force origin HEAD:main") {
		t.Fatalf("answer = %+v", a)
	}
	if _, err := os.Stat(filepath.Join(fdir, "ran")); err != nil {
		t.Fatal("the allowed step did not run")
	}
	results := eventsOf(t, m, id, "tool_result")
	if len(results) != 1 || !strings.Contains(results[0].Text, "forced update") {
		t.Fatalf("tool results = %+v", results)
	}
	ends := eventsOf(t, m, id, KindTurnEnd)
	if len(ends) != 1 || ends[0].Text != "Turn finished" || ends[0].IsError != nil {
		t.Fatalf("turn end = %+v", ends)
	}
	noTurnLeft(t, m, fdir)
}

func TestDenyWithANoteRefusesAndTheTurnContinues(t *testing.T) {
	m, id, fdir, perm := promptChat(t)
	if _, err := m.Answer(id, perm.RequestID, false, "  use the debug build "); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m, id)
	a := readAnswer(t, fdir)
	if a.Response.Response.Behavior != "deny" || a.Response.Response.Message != "use the debug build" {
		t.Fatalf("answer = %+v", a)
	}
	if _, err := os.Stat(filepath.Join(fdir, "ran")); err == nil {
		t.Fatal("a denied step ran")
	}
	answers := eventsOf(t, m, id, KindPermissionAnswer)
	if len(answers) != 1 || *answers[0].Allowed || answers[0].Reason != "use the debug build" || answers[0].By != ByPhone {
		t.Fatalf("answers = %+v", answers)
	}
	// The agent carried on after the refusal: it reported back, and the
	// turn ended normally.
	texts := eventsOf(t, m, id, "text")
	results := eventsOf(t, m, id, "tool_result")
	if len(results) != 1 || results[0].Text != "use the debug build" || len(texts) == 0 || texts[len(texts)-1].Seq < results[0].Seq ||
		!strings.Contains(texts[len(texts)-1].Text, "use the debug build") {
		t.Fatalf("texts after the denial = %+v", texts)
	}
	ends := eventsOf(t, m, id, KindTurnEnd)
	if len(ends) != 1 || ends[0].Text != "Turn finished" {
		t.Fatalf("turn end = %+v", ends)
	}
	noTurnLeft(t, m, fdir)

	// A denial without a note still tells the agent who refused.
	m2, id2, fdir2, perm2 := promptChat(t)
	m2.Answer(id2, perm2.RequestID, false, "")
	waitIdle(t, m2, id2)
	if a := readAnswer(t, fdir2); a.Response.Response.Message != DeniedNoNote {
		t.Fatalf("answer without a note = %+v", a)
	}
}

func TestTimeoutDeniesAndTheTurnEndsNormally(t *testing.T) {
	m, id, fdir, _ := promptChat(t, func(o *Options) { o.PermissionTimeout = 300 * time.Millisecond })
	rec := waitIdle(t, m, id)
	if rec.State != StateIdle {
		t.Fatalf("state = %s", rec.State)
	}
	a := readAnswer(t, fdir)
	if a.Response.Response.Behavior != "deny" || a.Response.Response.Message != "No answer from the phone within 15 minutes" {
		t.Fatalf("answer = %+v", a)
	}
	answers := eventsOf(t, m, id, KindPermissionAnswer)
	if len(answers) != 1 || answers[0].By != ByTimeout || answers[0].Reason != NoAnswerReason {
		t.Fatalf("answers = %+v", answers)
	}
	ends := eventsOf(t, m, id, KindTurnEnd)
	if len(ends) != 1 || ends[0].Text != "Turn finished" || ends[0].IsError != nil {
		t.Fatalf("turn end = %+v", ends)
	}
	noTurnLeft(t, m, fdir)
	if PermissionTimeout != 15*time.Minute {
		t.Fatalf("default timeout = %s", PermissionTimeout)
	}
}

func TestSecondAnswerIsRefusedWithTheOutcome(t *testing.T) {
	m, id, fdir, perm := promptChat(t)
	if _, err := m.Answer(id, perm.RequestID, true, ""); err != nil {
		t.Fatal(err)
	}
	ev, err := m.Answer(id, perm.RequestID, false, "no")
	if !errors.Is(err, ErrDecided) || ev.Allowed == nil || !*ev.Allowed {
		t.Fatalf("second answer = %+v, %v", ev, err)
	}
	waitIdle(t, m, id)
	if _, err := m.Answer(id, perm.RequestID, true, ""); !errors.Is(err, ErrDecided) {
		t.Fatalf("answer after the turn = %v", err)
	}
	if _, err := m.Answer(id, "no-such-request", true, ""); !errors.Is(err, ErrUnknownRequest) {
		t.Fatalf("unknown request = %v", err)
	}
	if n := len(eventsOf(t, m, id, KindPermissionAnswer)); n != 1 {
		t.Fatalf("%d answers recorded", n)
	}
	noTurnLeft(t, m, fdir)
}

func TestCancelWhilePendingDeniesAndEndsTheTurn(t *testing.T) {
	m, id, fdir, perm := promptChat(t)
	if err := m.Cancel(id); err != nil {
		t.Fatal(err)
	}
	rec := waitIdle(t, m, id)
	if rec.State != StateIdle {
		t.Fatalf("state = %s", rec.State)
	}
	answers := eventsOf(t, m, id, KindPermissionAnswer)
	if len(answers) != 1 || answers[0].RequestID != perm.RequestID || *answers[0].Allowed || answers[0].By != ByStop {
		t.Fatalf("answers = %+v", answers)
	}
	// The denial is written before the process is stopped; the CLI may be
	// gone before it reads it, but it never sees anything else.
	if data, _ := os.ReadFile(filepath.Join(fdir, "answer")); strings.HasSuffix(string(data), "\n") {
		if a := readAnswer(t, fdir); a.Response.Response.Behavior != "deny" || a.Response.Response.Message != StoppedReason {
			t.Fatalf("answer = %+v", a)
		}
	}
	ends := eventsOf(t, m, id, KindTurnEnd)
	if len(ends) != 1 || ends[0].Text != "Stopped" {
		t.Fatalf("turn end = %+v", ends)
	}
	if _, err := m.Answer(id, perm.RequestID, true, ""); !errors.Is(err, ErrDecided) {
		t.Fatalf("answer after stop = %v", err)
	}
	noTurnLeft(t, m, fdir)
}

func TestRestartDeniesPendingRequests(t *testing.T) {
	dir := t.TempDir()
	bin, fdir := fakeclaude.Install(t, "testdata")
	m := newManager(t, dir, bin)
	rec, _ := m.Create(CreateRequest{Agent: "claude"})
	m.Send(rec.ID, SendRequest{TurnID: "c_perm0001", Text: "push it"})
	perm := waitPermission(t, m, rec.ID)

	// A second manager on the same directory is nacred after a restart.
	m2 := newManager(t, dir, bin)
	got, _, _ := m2.Get(rec.ID, 0)
	if got.State != StateInterrupted {
		t.Fatalf("state after restart = %s", got.State)
	}
	answers := eventsOf(t, m2, rec.ID, KindPermissionAnswer)
	if len(answers) != 1 || answers[0].RequestID != perm.RequestID || *answers[0].Allowed || answers[0].By != ByRestart {
		t.Fatalf("answers after restart = %+v", answers)
	}
	if _, err := m2.Answer(rec.ID, perm.RequestID, true, ""); !errors.Is(err, ErrDecided) {
		t.Fatalf("answer after restart = %v", err)
	}
	m.Cancel(rec.ID)
	waitIdle(t, m, rec.ID)
	noTurnLeft(t, m, fdir)
}

func TestDeleteWhilePendingEndsTheTurn(t *testing.T) {
	m, id, fdir, _ := promptChat(t)
	if err := m.Delete(id); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for m.Busy() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	noTurnLeft(t, m, fdir)
}
