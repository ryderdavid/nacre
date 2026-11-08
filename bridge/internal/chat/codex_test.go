package chat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/chat/fakecodex"
)

// codexChat is a manager whose Codex chats run on the fake app-server, with
// a Codex chat on repo r-a of a host with three boards. Claude is missing,
// so any claude call fails the turn.
func codexChat(t *testing.T, approval string, opts ...func(*Options)) (m *Manager, rec Record, fdir string, boards []string) {
	t.Helper()
	bin, fdir := fakecodex.Install(t)
	if approval != "" {
		if err := os.WriteFile(filepath.Join(fdir, "approval"), []byte(approval), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, boards, repo := repoSetup(t)
	all := append([]func(*Options){repo, func(o *Options) { o.CodexPath = bin }}, opts...)
	m = newManager(t, t.TempDir(), "/nonexistent/claude", all...)
	rec, err := m.Create(CreateRequest{Agent: AgentCodex, Repo: strp("r-a")})
	if err != nil {
		t.Fatal(err)
	}
	return m, rec, fdir, boards
}

// rpcCalls are the requests one app-server process read, by method.
func rpcCalls(t *testing.T, fdir string, n int) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	for _, line := range readLines(t, filepath.Join(fdir, "call"+itoa(n)+".rpc")) {
		var m struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if json.Unmarshal([]byte(line), &m) == nil && m.Method != "" {
			out[m.Method] = m.Params
		}
	}
	return out
}

func itoa(n int) string { return string(rune('0' + n)) }

func TestCodexStartAndResumeUseTheSameThread(t *testing.T) {
	m, rec, fdir, boards := codexChat(t, "")
	if rec.SessionID != "" {
		t.Fatalf("a new Codex chat has session %q before thread/start", rec.SessionID)
	}
	ctx := &Context{Screen: "bead", Repo: "r-a", BeadID: "NAC-3", Summary: "Fix the build"}
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_codex_one", Text: "build it", Context: ctx}); err != nil {
		t.Fatal(err)
	}
	first := waitIdle(t, m, rec.ID)
	if first.SessionID != fakecodex.ThreadID || !first.SessionStarted {
		t.Fatalf("after the first turn: session %q started %v", first.SessionID, first.SessionStarted)
	}
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_codex_two", Text: "and again", Context: ctx}); err != nil {
		t.Fatal(err)
	}
	second := waitIdle(t, m, rec.ID)
	if second.SessionID != fakecodex.ThreadID {
		t.Fatalf("second turn changed the thread to %q", second.SessionID)
	}

	if argv := readLines(t, filepath.Join(fdir, "call0.argv")); !reflect.DeepEqual(argv, []string{"app-server"}) {
		t.Errorf("argv = %q", argv)
	}
	if cwd := readLines(t, filepath.Join(fdir, "call0.cwd"))[0]; mustEval(t, cwd) != mustEval(t, boards[0]) {
		t.Errorf("app-server ran in %s, want %s", cwd, boards[0])
	}

	one := rpcCalls(t, fdir, 0)
	start := one["thread/start"]
	if start == nil || one["thread/resume"] != nil {
		t.Fatalf("first turn calls: %v", one)
	}
	if start["cwd"] != boards[0] || start["sandbox"] != "workspace-write" || start["approvalPolicy"] != "on-request" {
		t.Errorf("thread/start = %v", start)
	}
	sw := start["config"].(map[string]any)["sandbox_workspace_write"].(map[string]any)
	if sw["network_access"] != true {
		t.Errorf("network access = %v", sw["network_access"])
	}
	var roots []string
	for _, r := range sw["writable_roots"].([]any) {
		roots = append(roots, r.(string))
	}
	if want := append(append([]string{}, boards...), m.MockupsDir(rec.ID)); !reflect.DeepEqual(roots, want) {
		t.Errorf("writable_roots = %v, want every board and the mockups %v", roots, want)
	}
	if di, _ := start["developerInstructions"].(string); di != Briefing(BriefingInput{Host: "Mac-Studio", NacredURL: "http://100.64.0.7:7420"}) {
		t.Errorf("developer instructions are not the briefing: %.200q", di)
	}
	turn := one["turn/start"]
	if turn["threadId"] != fakecodex.ThreadID {
		t.Errorf("turn/start thread = %v", turn["threadId"])
	}
	input := turn["input"].([]any)
	if len(input) != 2 {
		t.Fatalf("turn/start input = %v", input)
	}
	note := input[0].(map[string]any)["text"].(string)
	text := input[1].(map[string]any)["text"].(string)
	if !strings.HasPrefix(note, "On screen:") || !strings.Contains(note, "NAC-3") || text != "build it" {
		t.Errorf("turn/start input: note %q, text %q", note, text)
	}
	policy := turn["sandboxPolicy"].(map[string]any)
	if policy["type"] != "workspaceWrite" || policy["networkAccess"] != true || len(policy["writableRoots"].([]any)) != 4 {
		t.Errorf("turn sandbox policy = %v", policy)
	}

	two := rpcCalls(t, fdir, 1)
	if two["thread/start"] != nil || two["thread/resume"]["threadId"] != fakecodex.ThreadID {
		t.Fatalf("second turn calls: %v", two)
	}
	if two["turn/start"]["threadId"] != fakecodex.ThreadID {
		t.Errorf("second turn/start thread = %v", two["turn/start"]["threadId"])
	}
	note2 := two["turn/start"]["input"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(note2, "same as before") {
		t.Errorf("second note = %q", note2)
	}

	// Codex reports no cost: cost_usd stays absent.
	data, _ := json.Marshal(second)
	if strings.Contains(string(data), "cost_usd") {
		t.Errorf("record has cost_usd: %s", data)
	}
	_, evs, _ := m.Get(rec.ID, 0)
	var texts []string
	for _, ev := range evs {
		if ev.Kind == "text" {
			if ev.Route != RouteFull {
				t.Errorf("text event route = %q", ev.Route)
			}
			texts = append(texts, ev.Text)
		}
		if ev.Kind == KindTurnEnd && (ev.CostUSD != 0 || ev.IsError != nil) {
			t.Errorf("turn end = %+v", ev)
		}
		if ev.Kind == KindError {
			t.Errorf("error event: %s", ev.Text)
		}
	}
	if !reflect.DeepEqual(texts, []string{"working on it", "done here", "working on it", "done here"}) {
		t.Errorf("texts = %q", texts)
	}
}

func TestCodexChildGetsNacredURL(t *testing.T) {
	t.Setenv("NACRED_URL", "http://stale")
	m, rec, fdir, _ := codexChat(t, "", func(o *Options) { o.NacredURL = "http://100.64.0.7:7420" })
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_codex_env", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m, rec.ID)
	var urls []string
	for _, kv := range readLines(t, filepath.Join(fdir, "call0.env")) {
		if strings.HasPrefix(kv, "NACRED_URL=") {
			urls = append(urls, kv)
		}
	}
	if !reflect.DeepEqual(urls, []string{"NACRED_URL=http://100.64.0.7:7420"}) {
		t.Errorf("child NACRED_URL = %v", urls)
	}
}

func answerOf(t *testing.T, m *Manager, id, req string) Event {
	t.Helper()
	_, evs, _ := m.Get(id, 0)
	for _, ev := range evs {
		if ev.Kind == KindPermissionAnswer && ev.RequestID == req {
			return ev
		}
	}
	t.Fatalf("no answer recorded for %s", req)
	return Event{}
}

func decision(t *testing.T, fdir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fdir, "answer"))
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		Decision string `json:"decision"`
	}
	_ = json.Unmarshal(data, &r)
	return r.Decision
}

func endedNormally(t *testing.T, m *Manager, id string) {
	t.Helper()
	rec := waitIdle(t, m, id)
	if rec.State != StateIdle {
		t.Errorf("state after the turn = %s", rec.State)
	}
	_, evs, _ := m.Get(id, 0)
	last := evs[len(evs)-1]
	if last.Kind != KindTurnEnd || last.IsError != nil || last.Text != "Turn finished" {
		t.Errorf("turn ended with %+v", last)
	}
	if m.Busy() {
		t.Error("a turn is still counted as running")
	}
}

func TestCodexApprovalAllowed(t *testing.T) {
	m, rec, fdir, _ := codexChat(t, "command")
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_codex_allow", Text: "send me the apk"}); err != nil {
		t.Fatal(err)
	}
	perm := waitPermission(t, m, rec.ID)
	if perm.Tool != "Bash" || perm.Text != "Bash: tailscale file cp app.apk pixel:" || perm.Reason != "needs network access outside the sandbox" {
		t.Errorf("permission = %+v", perm)
	}
	if _, err := m.Answer(rec.ID, perm.RequestID, true, ""); err != nil {
		t.Fatal(err)
	}
	endedNormally(t, m, rec.ID)
	if d := decision(t, fdir); d != "accept" {
		t.Errorf("decision = %q", d)
	}
	if _, err := os.Stat(filepath.Join(fdir, "ran")); err != nil {
		t.Error("the allowed command didn't run")
	}
	if a := answerOf(t, m, rec.ID, perm.RequestID); *a.Allowed != true || a.By != ByPhone {
		t.Errorf("answer = %+v", a)
	}
	if _, err := m.Answer(rec.ID, perm.RequestID, false, ""); err != ErrDecided {
		t.Errorf("second answer: %v", err)
	}
}

func TestCodexApprovalDeniedWithANote(t *testing.T) {
	m, rec, fdir, _ := codexChat(t, "file")
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_codex_deny", Text: "fix hosts"}); err != nil {
		t.Fatal(err)
	}
	perm := waitPermission(t, m, rec.ID)
	if perm.Tool != "Edit" || perm.Text != "Edit: /etc/hosts" || perm.Reason != "writes outside the workspace" {
		t.Errorf("permission = %+v", perm)
	}
	if _, err := m.Answer(rec.ID, perm.RequestID, false, "use the debug build"); err != nil {
		t.Fatal(err)
	}
	endedNormally(t, m, rec.ID)
	if d := decision(t, fdir); d != "decline" {
		t.Errorf("decision = %q", d)
	}
	if _, err := os.Stat(filepath.Join(fdir, "ran")); err == nil {
		t.Error("the denied change ran")
	}
	steer, _ := os.ReadFile(filepath.Join(fdir, "steer"))
	if !strings.Contains(string(steer), "use the debug build") {
		t.Errorf("the note didn't reach Codex: steer = %q", steer)
	}
	if a := answerOf(t, m, rec.ID, perm.RequestID); *a.Allowed || a.Reason != "use the debug build" {
		t.Errorf("answer = %+v", a)
	}
}

func TestCodexApprovalTimesOut(t *testing.T) {
	m, rec, fdir, _ := codexChat(t, "command", func(o *Options) { o.PermissionTimeout = 300 * time.Millisecond })
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_codex_wait", Text: "send me the apk"}); err != nil {
		t.Fatal(err)
	}
	perm := waitPermission(t, m, rec.ID)
	endedNormally(t, m, rec.ID)
	if d := decision(t, fdir); d != "decline" {
		t.Errorf("decision = %q", d)
	}
	if a := answerOf(t, m, rec.ID, perm.RequestID); *a.Allowed || a.By != ByTimeout || a.Reason != NoAnswerReason {
		t.Errorf("answer = %+v", a)
	}
	if _, err := os.Stat(filepath.Join(fdir, "ran")); err == nil {
		t.Error("the timed-out command ran")
	}
}

func TestCodexStopWhileWaiting(t *testing.T) {
	m, rec, fdir, _ := codexChat(t, "command")
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_codex_stop", Text: "send me the apk"}); err != nil {
		t.Fatal(err)
	}
	perm := waitPermission(t, m, rec.ID)
	if err := m.Cancel(rec.ID); err != nil {
		t.Fatal(err)
	}
	got := waitIdle(t, m, rec.ID)
	if got.State != StateIdle {
		t.Errorf("state = %s", got.State)
	}
	if a := answerOf(t, m, rec.ID, perm.RequestID); *a.Allowed || a.By != ByStop {
		t.Errorf("answer = %+v", a)
	}
	if d := decision(t, fdir); d != "decline" {
		t.Errorf("decision = %q", d)
	}
	if m.Busy() {
		t.Error("a stopped turn is still counted as running")
	}
}

func TestCodexWithoutTheCLI(t *testing.T) {
	m := newManager(t, t.TempDir(), "/nonexistent/claude")
	rec, err := m.Create(CreateRequest{Agent: AgentCodex})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_codex_none", Text: "hi"}); err != ErrCodexUnavailable {
		t.Fatalf("send = %v", err)
	}
}
