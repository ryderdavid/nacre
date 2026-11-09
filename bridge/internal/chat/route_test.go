package chat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/chat/fakeclaude"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// routedChat is a Claude chat on the fake CLI with routing on. files are
// written to the fake's directory first (router, light, full, router_cost).
func routedChat(t *testing.T, files map[string]string, opts ...func(*Options)) (m *Manager, rec Record, fdir string) {
	t.Helper()
	bin, fdir := fakeclaude.Install(t, "testdata")
	for k, v := range files {
		if err := os.WriteFile(filepath.Join(fdir, k), []byte(v), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	all := append([]func(*Options){func(o *Options) {
		o.RouterModel, o.LightModel = "haiku", "sonnet"
		// The fake CLI is a fresh re-run of the test binary, which can take
		// seconds to start while the whole gate runs; a router that answers
		// is never near this.
		o.RouterTimeout = 10 * time.Second
	}}, opts...)
	m = newManager(t, t.TempDir(), bin, all...)
	rec, err := m.Create(CreateRequest{Agent: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	return m, rec, fdir
}

func sendAndWait(t *testing.T, m *Manager, id, turnID, text string) Record {
	t.Helper()
	if _, err := m.Send(id, SendRequest{TurnID: turnID, Text: text, Context: &Context{Screen: "bead", Repo: "r-a", BeadID: "NAC-7"}}); err != nil {
		t.Fatal(err)
	}
	return waitIdle(t, m, id)
}

// fakeCall is one claude invocation the fake recorded.
type fakeCall struct {
	kind  string
	argv  []string
	stdin []string
}

func calls(t *testing.T, fdir string) []fakeCall {
	t.Helper()
	var out []fakeCall
	for n := 0; ; n++ {
		kind, err := os.ReadFile(filepath.Join(fdir, fmt.Sprintf("call%d.kind", n)))
		if err != nil {
			return out
		}
		c := fakeCall{kind: string(kind)}
		argv, err := os.ReadFile(filepath.Join(fdir, fmt.Sprintf("call%d.argv", n)))
		if err != nil || json.Unmarshal(argv, &c.argv) != nil {
			t.Fatalf("call %d argv: %v %s", n, err, argv)
		}
		if data, err := os.ReadFile(filepath.Join(fdir, fmt.Sprintf("call%d.stdin", n))); err == nil {
			c.stdin = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		}
		out = append(out, c)
	}
}

func kinds(cs []fakeCall) string {
	var ks []string
	for _, c := range cs {
		ks = append(ks, c.kind)
	}
	return strings.Join(ks, ",")
}

func flag(argv []string, name string) (string, bool) {
	i := indexOf(argv, name)
	if i < 0 || i+1 >= len(argv) {
		return "", i >= 0
	}
	return argv[i+1], true
}

// userTexts are the text blocks of a stream-json user message.
func userTexts(t *testing.T, line string) []string {
	t.Helper()
	var m struct {
		Message struct {
			Content []struct{ Type, Text string }
		}
	}
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("stdin %q: %v", line, err)
	}
	var out []string
	for _, c := range m.Message.Content {
		out = append(out, c.Text)
	}
	return out
}

func allEvents(t *testing.T, m *Manager, id string) []Event {
	t.Helper()
	_, evs, err := m.Get(id, 0)
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func checkRouterArgv(t *testing.T, argv []string, budget string) {
	t.Helper()
	if v, _ := flag(argv, "--model"); v != "haiku" {
		t.Errorf("router model: %v", argv)
	}
	for _, f := range []string{"--safe-mode", "--no-session-persistence"} {
		if indexOf(argv, f) < 0 {
			t.Errorf("router argv lacks %s: %v", f, argv)
		}
	}
	if v, _ := flag(argv, "--permission-prompts"); v != "none" {
		t.Errorf("router permission prompts: %v", argv)
	}
	if v, ok := flag(argv, "--tools"); !ok || v != "" {
		t.Errorf("router must have no tools: %v", argv)
	}
	for _, f := range []string{"--allowed-tools", "--add-dir", "--resume", "--session-id"} {
		if indexOf(argv, f) >= 0 {
			t.Errorf("router argv has %s: %v", f, argv)
		}
	}
	if v, _ := flag(argv, "--json-schema"); v != string(RouterSchema) {
		t.Errorf("router schema %q", v)
	}
	if v, _ := flag(argv, "--max-budget-usd"); v != budget {
		t.Errorf("router budget %q, want %s", v, budget)
	}
}

func TestRouterLightRunsTheLightArgv(t *testing.T) {
	_, _, opt := repoSetup(t)
	m, rec, fdir := routedChat(t, map[string]string{"router": "light", "light": "light_plain"}, opt)
	// A chat that has spent $12: the router starts with $0.10, the light
	// turn with what is left after the router.
	m.mu.Lock()
	m.chats[rec.ID].rec.CostUSD = 12
	m.mu.Unlock()
	got := sendAndWait(t, m, rec.ID, "c_light001", "What's the difference between a merge and a rebase?")

	cs := calls(t, fdir)
	if kinds(cs) != "router,light" {
		t.Fatalf("calls %s", kinds(cs))
	}
	checkRouterArgv(t, cs[0].argv, "0.10")
	la := cs[1].argv
	if v, _ := flag(la, "--model"); v != "sonnet" {
		t.Errorf("light model: %v", la)
	}
	if v, _ := flag(la, "--permission-mode"); v != "default" {
		t.Errorf("light permission mode: %v", la)
	}
	if !containsAll(la, "--restricted", "--tools", "--allowed-tools", "Read", "Bash(bd show:*)", "Bash(git status:*)") {
		t.Errorf("light allowlist: %v", la)
	}
	for _, leaky := range []string{"Bash(git diff:*)", "Bash(git log:*)", "Bash(rg:*)", "Bash(cat:*)"} {
		if containsAll(la, leaky) {
			t.Errorf("light allowlist has %s, whose flags can write files", leaky)
		}
	}
	if v, _ := flag(la, "--session-id"); v != rec.SessionID {
		t.Errorf("light session: %v", la)
	}
	if v, _ := flag(la, "--max-budget-usd"); v != "37.99" {
		t.Errorf("light budget %q, want 37.99 (50 - 12 - router 0.01)", v)
	}
	for _, want := range []string{"--permission-prompts", "--append-system-prompt", "--input-format"} {
		if indexOf(la, want) < 0 {
			t.Errorf("light argv lacks the claude.Chat flag %s: %v", want, la)
		}
	}
	texts := userTexts(t, cs[1].stdin[0])
	if len(texts) != 2 || !strings.HasPrefix(texts[0], "On screen: bead NAC-7") || !strings.HasSuffix(texts[0], LightAddendum) || !strings.Contains(texts[1], "merge and a rebase") {
		t.Errorf("light message %q", texts)
	}
	// The recorded plain answer stays light, with no handoff.
	var sawText bool
	for _, ev := range allEvents(t, m, rec.ID) {
		switch ev.Kind {
		case "text":
			sawText = true
			if ev.Route != RouteLight || ev.TurnID != "c_light001" {
				t.Errorf("text event %+v", ev)
			}
		case KindHandoff, KindPermission:
			t.Errorf("unexpected %s event: %+v", ev.Kind, ev)
		}
	}
	if !sawText {
		t.Error("no answer text")
	}
	// Router and light costs count toward the $50.
	if want := 12 + 0.01 + 0.0969761; !near(got.CostUSD, want) {
		t.Errorf("cost %v, want %v", got.CostUSD, want)
	}
	if !got.SessionStarted || got.State != StateIdle {
		t.Errorf("record %+v", got)
	}
}

func TestRouterFullRunsTheFullArgv(t *testing.T) {
	m, rec, fdir := routedChat(t, map[string]string{"router": "full", "full": "light_plain"})
	got := sendAndWait(t, m, rec.ID, "c_full0001", "Fix the failing test")
	cs := calls(t, fdir)
	if kinds(cs) != "router,full" {
		t.Fatalf("calls %s", kinds(cs))
	}
	fa := cs[1].argv
	if v, _ := flag(fa, "--permission-mode"); v != "auto" {
		t.Errorf("full permission mode: %v", fa)
	}
	if indexOf(fa, "--model") >= 0 {
		t.Errorf("full argv has --model: %v", fa)
	}
	texts := userTexts(t, cs[1].stdin[0])
	if strings.Contains(texts[0], LightAddendum) {
		t.Error("full turn got the light addendum")
	}
	for _, ev := range allEvents(t, m, rec.ID) {
		if ev.Kind == "text" && ev.Route != RouteFull {
			t.Errorf("full text event %+v", ev)
		}
	}
	if want := 0.01 + 0.0969761; !near(got.CostUSD, want) {
		t.Errorf("cost %v, want %v", got.CostUSD, want)
	}
}

func TestRouterFailuresRunFull(t *testing.T) {
	for _, mode := range []string{"error", "slow", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			m, rec, fdir := routedChat(t, map[string]string{"router": mode, "full": "light_plain"}, func(o *Options) {
				// Long enough for the fake to start and record its call under
				// load; the slow fake sleeps a minute, so it still times out.
				o.RouterTimeout = 5 * time.Second
			})
			start := time.Now()
			got := sendAndWait(t, m, rec.ID, "c_fail0001", "hello")
			cs := calls(t, fdir)
			if kinds(cs) != "router,full" {
				t.Fatalf("calls %s", kinds(cs))
			}
			if v, _ := flag(cs[1].argv, "--permission-mode"); v != "auto" {
				t.Errorf("not a full turn: %v", cs[1].argv)
			}
			if mode == "slow" && time.Since(start) > 30*time.Second {
				t.Errorf("router timeout not applied: %v", time.Since(start))
			}
			if got.State != StateIdle {
				t.Errorf("state %s", got.State)
			}
		})
	}
}

func TestRejectedModelGoesStraightToFullAndLogsOnce(t *testing.T) {
	var logs bytes.Buffer
	m, rec, fdir := routedChat(t, map[string]string{"router": "rejected", "full": "light_plain"}, func(o *Options) {
		o.Logger = log.New(&logs, "", 0)
	})
	sendAndWait(t, m, rec.ID, "c_rej00001", "hello")
	sendAndWait(t, m, rec.ID, "c_rej00002", "again")
	if k := kinds(calls(t, fdir)); k != "router,full,full" {
		t.Fatalf("calls %s: the second message should skip the rejected router", k)
	}
	if n := strings.Count(logs.String(), "rejected the router model"); n != 1 {
		t.Errorf("logged %d times: %q", n, logs.String())
	}

	// A rejected light model: this message carries on in full, the next
	// goes straight to full.
	m2, rec2, fdir2 := routedChat(t, map[string]string{"router": "light", "light": "light_model_rejected", "full": "light_plain"})
	sendAndWait(t, m2, rec2.ID, "c_rej00003", "hello")
	sendAndWait(t, m2, rec2.ID, "c_rej00004", "again")
	if k := kinds(calls(t, fdir2)); k != "router,light,full,full" {
		t.Fatalf("calls %s", k)
	}
	for _, ev := range allEvents(t, m2, rec2.ID) {
		if ev.Kind == KindHandoff || ev.Kind == KindError {
			t.Errorf("a rejected model is not a handoff or an error: %+v", ev)
		}
	}
}

func TestRouterInput(t *testing.T) {
	// History: the last six user and assistant messages, trimmed to 2 KB,
	// without the message being sent.
	var evs []Event
	for i := 0; i < 5; i++ {
		turn := fmt.Sprintf("t%d", i)
		evs = append(evs,
			Event{Kind: KindUser, TurnID: turn, Text: fmt.Sprintf("question %d %s", i, strings.Repeat("q", 500))},
			Event{Kind: "tool_use", TurnID: turn, Tool: "Read", Text: "secret file contents"},
			Event{Kind: "text", TurnID: turn, Text: fmt.Sprintf("answer %d part one", i)},
			Event{Kind: "text", TurnID: turn, Text: strings.Repeat("a", 400)},
		)
	}
	h := routerHistory(evs)
	// Six messages are ~2.8 KB here: the oldest of them is cut to fit.
	if len(h) != 5 || h[0].Role != "Assistant" || len(h[0].Text) >= 400 {
		t.Fatalf("%d messages, oldest %+v", len(h), h[0])
	}
	short := routerHistory([]Event{
		{Kind: KindUser, TurnID: "a", Text: "1"}, {Kind: "text", TurnID: "a", Text: "2"},
		{Kind: KindUser, TurnID: "b", Text: "3"}, {Kind: "text", TurnID: "b", Text: "4"},
		{Kind: KindUser, TurnID: "c", Text: "5"}, {Kind: "text", TurnID: "c", Text: "6"},
		{Kind: KindUser, TurnID: "d", Text: "7"}, {Kind: "text", TurnID: "d", Text: "8"},
	})
	if len(short) != 6 || short[0].Text != "3" || short[5].Text != "8" {
		t.Fatalf("last six: %+v", short)
	}
	total := 0
	for _, m := range h {
		total += len(m.Text)
		if strings.Contains(m.Text, "secret file contents") {
			t.Error("tool output reached the router")
		}
	}
	if total > 2048 {
		t.Errorf("history is %d bytes", total)
	}
	if last := h[len(h)-1]; last.Role != "Assistant" || !strings.HasPrefix(last.Text, "answer 4 part one\n") {
		t.Errorf("newest message %+v", last)
	}

	// Through the fake: the prompt holds the note, the text, the image count,
	// the New effort flag and the trimmed history, and no files.
	_, _, opt := repoSetup(t)
	bin, fdir := fakeclaude.Install(t, "testdata")
	os.WriteFile(filepath.Join(fdir, "router"), []byte("full"), 0o644)
	os.WriteFile(filepath.Join(fdir, "full"), []byte("light_plain"), 0o644)
	m := newManager(t, t.TempDir(), bin, opt, func(o *Options) { o.RouterModel, o.LightModel = "haiku", "sonnet" })
	rec, err := m.Create(CreateRequest{Agent: "claude", Repo: strp("r-a"), NewEffort: true})
	if err != nil {
		t.Fatal(err)
	}
	// Once drafting has ended, a New effort chat is routed like any other.
	m.EndDrafting(rec.ID)
	sendAndWait(t, m, rec.ID, "c_in000001", strings.Repeat("long first message ", 200))
	sendAndWait(t, m, rec.ID, "c_in000002", "and a follow-up")
	cs := calls(t, fdir)
	if kinds(cs) != "router,full,router,full" {
		t.Fatalf("calls %s", kinds(cs))
	}
	prompt := cs[2].argv[indexOf(cs[2].argv, "-p")+1]
	for _, want := range []string{
		"a New effort chat", "Images attached to the new message: 0", "On screen: same as before (NAC-7)",
		"New message:\nand a follow-up", "Ryder: ", "Assistant: A merge combines",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("router prompt lacks %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, strings.Repeat("long first message ", 120)) {
		t.Error("history was not trimmed to 2 KB")
	}
	if strings.Contains(prompt, "/") && strings.Contains(prompt, rec.Cwd) {
		t.Error("router prompt names the checkout")
	}
	checkRouterArgv(t, cs[2].argv, "0.10")
	if p := (RouterInput{Images: 3}).Prompt(); !strings.Contains(p, "Images attached to the new message: 3") || !strings.Contains(p, "an ordinary chat") {
		t.Errorf("prompt %q", p)
	}
}

func TestRecordedReadOnlyStepStaysLight(t *testing.T) {
	// The recording shows plan mode ran `git log` / `git status` without
	// asking the host, so the step stays inside the light turn.
	m, rec, fdir := routedChat(t, map[string]string{"router": "light", "light": "light_readonly_bash"})
	sendAndWait(t, m, rec.ID, "c_ro000001", "What's the latest commit and is the tree clean?")
	cs := calls(t, fdir)
	if kinds(cs) != "router,light" {
		t.Fatalf("calls %s", kinds(cs))
	}
	if len(cs[1].stdin) != 1 {
		t.Errorf("nacred wrote more than the message: %q", cs[1].stdin)
	}
	var tool, result bool
	for _, ev := range allEvents(t, m, rec.ID) {
		switch ev.Kind {
		case "tool_use":
			tool = ev.Tool == "Bash" && ev.Route == RouteLight
		case "tool_result":
			result = ev.Route == RouteLight
		case KindHandoff, KindPermission:
			t.Errorf("unexpected %s: %+v", ev.Kind, ev)
		}
	}
	if !tool || !result {
		t.Error("the read-only Bash step is not in the light turn")
	}
}

// fixturePlan is the plan of the recorded ExitPlanMode call.
func fixturePlan(t *testing.T) (plan, reqID string) {
	t.Helper()
	data, err := os.ReadFile("testdata/light_exit_plan.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(data), "\n") {
		var r struct {
			Line json.RawMessage
		}
		if json.Unmarshal([]byte(l), &r) != nil {
			continue
		}
		if req, ok := claude.ParseControlRequest(r.Line); ok {
			return planText(req.Input), req.RequestID
		}
	}
	t.Fatal("no ExitPlanMode request in the recording")
	return "", ""
}

// checkHandoff checks a light turn handed off to a full turn on the same
// session and turn id, with no card.
func checkHandoff(t *testing.T, m *Manager, rec Record, fdir, turnID string) (handoff Event, cs []fakeCall) {
	t.Helper()
	cs = calls(t, fdir)
	if kinds(cs) != "router,light,full" {
		t.Fatalf("calls %s", kinds(cs))
	}
	fa := cs[2].argv
	if v, _ := flag(fa, "--resume"); v != rec.SessionID {
		t.Errorf("full turn doesn't resume the chat's session %s: %v", rec.SessionID, fa)
	}
	if v, _ := flag(fa, "--permission-mode"); v != "auto" || indexOf(fa, "--model") >= 0 {
		t.Errorf("not the full argv: %v", fa)
	}
	texts := userTexts(t, cs[2].stdin[0])
	if len(texts) != 2 || texts[0] != ContinuationNote || texts[1] != "On screen: same as before (NAC-7)" {
		t.Errorf("full turn input %q", texts)
	}
	var handoffs, ends int
	seenHandoff := false
	for _, ev := range allEvents(t, m, rec.ID) {
		if ev.Kind != KindUser && ev.TurnID != turnID && ev.TurnID != "" {
			t.Errorf("event of another turn: %+v", ev)
		}
		switch ev.Kind {
		case KindPermission, KindPermissionAnswer:
			t.Errorf("a card reached the phone: %+v", ev)
		case KindHandoff:
			handoffs++
			handoff = ev
			seenHandoff = true
		case KindTurnEnd:
			ends++
		case "text":
			if strings.Contains(ev.Text, handoffPrefix) || strings.Contains(ev.Text, HandoffMessage) {
				t.Errorf("handoff text shown: %q", ev.Text)
			}
			want := RouteLight
			if seenHandoff {
				want = RouteFull
			}
			if ev.Route != want {
				t.Errorf("text %q is %s, want %s", ev.Text, ev.Route, want)
			}
		}
	}
	if handoffs != 1 || ends != 1 {
		t.Errorf("%d handoffs, %d turn ends", handoffs, ends)
	}
	return handoff, cs
}

func TestHandoffLineStartsAFullTurn(t *testing.T) {
	m, rec, fdir := routedChat(t, map[string]string{"router": "light", "light": "light_handoff_line", "full": "light_plain"})
	got := sendAndWait(t, m, rec.ID, "c_line0001", "Add a line to the README")
	h, _ := checkHandoff(t, m, rec, fdir, "c_line0001")
	reason := "Appending 'Built by Nacre.' to README.md needs a file edit, which this read-only chat can't do."
	if h.Reason != reason || h.Text != "Switching to the full agent: "+reason {
		t.Errorf("handoff %+v", h)
	}
	if want := 0.01 + 0.0957821 + 0.0969761; !near(got.CostUSD, want) {
		t.Errorf("cost %v, want %v", got.CostUSD, want)
	}
	if got.TurnID != "c_line0001" || got.State != StateIdle {
		t.Errorf("record %+v", got)
	}
}

func TestRecordedExitPlanModeHandsOffWithItsPlan(t *testing.T) {
	m, rec, fdir := routedChat(t, map[string]string{"router": "light", "light": "light_exit_plan", "full": "light_plain"})
	got := sendAndWait(t, m, rec.ID, "c_plan0001", "Add a line to the README")
	h, cs := checkHandoff(t, m, rec, fdir, "c_plan0001")
	plan, reqID := fixturePlan(t)
	if plan == "" || h.Reason != plan {
		t.Errorf("handoff reason %q, want the plan %q", h.Reason, plan)
	}
	// nacred refused the call and stopped the light turn.
	if len(cs[1].stdin) != 3 {
		t.Fatalf("light stdin %q", cs[1].stdin)
	}
	a := struct {
		Type     string
		Response struct {
			RequestID string `json:"request_id"`
			Response  struct{ Behavior, Message string }
		}
	}{}
	if err := json.Unmarshal([]byte(cs[1].stdin[1]), &a); err != nil {
		t.Fatal(err)
	}
	if a.Response.RequestID != reqID || a.Response.Response.Behavior != "deny" || a.Response.Response.Message != HandoffMessage {
		t.Errorf("answer %q", cs[1].stdin[1])
	}
	if !strings.Contains(cs[1].stdin[2], `"subtype":"interrupt"`) {
		t.Errorf("light turn not interrupted: %q", cs[1].stdin[2])
	}
	// The interrupted light turn's result still counts.
	if want := 0.01 + 0.1634221 + 0.0969761; !near(got.CostUSD, want) {
		t.Errorf("cost %v, want %v", got.CostUSD, want)
	}
}

func TestPermissionRequestInALightTurnHandsOffWithoutACard(t *testing.T) {
	m, rec, fdir := routedChat(t, map[string]string{"router": "light", "light": "host_prompt_deny", "full": "light_plain"})
	sendAndWait(t, m, rec.ID, "c_perm0002", "push it")
	h, cs := checkHandoff(t, m, rec, fdir, "c_perm0002")
	if !strings.Contains(h.Reason, "git push") {
		t.Errorf("reason %q", h.Reason)
	}
	if len(cs[1].stdin) < 2 || !strings.Contains(cs[1].stdin[1], `"behavior":"deny"`) || !strings.Contains(cs[1].stdin[1], HandoffMessage) {
		t.Errorf("light stdin %q", cs[1].stdin)
	}
}

func TestStopCancelsTheRouter(t *testing.T) {
	m, rec, fdir := routedChat(t, map[string]string{"router": "slow"}, func(o *Options) { o.RouterTimeout = time.Minute })
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_stop0001", Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(calls(t, fdir)) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	start := time.Now()
	if err := m.Cancel(rec.ID); err != nil {
		t.Fatal(err)
	}
	got := waitIdle(t, m, rec.ID)
	if time.Since(start) > 5*time.Second || got.State != StateIdle {
		t.Errorf("stop took %v, state %s", time.Since(start), got.State)
	}
	if k := kinds(calls(t, fdir)); k != "router" {
		t.Errorf("calls after stop: %s", k)
	}
	ends := eventsOf(t, m, rec.ID, KindTurnEnd)
	if len(ends) != 1 || ends[0].Text != "Stopped" {
		t.Errorf("turn end %+v", ends)
	}
	if m.Busy() {
		t.Error("still busy")
	}
}

func TestRoutingCountsAsAnAssistantCall(t *testing.T) {
	m, rec, fdir := routedChat(t, map[string]string{"router": "slow"}, func(o *Options) { o.RouterTimeout = time.Minute })
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_busy0001", Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(calls(t, fdir)) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !m.Busy() {
		t.Error("a routing call doesn't hold off updates")
	}
	m.Cancel(rec.ID)
	waitIdle(t, m, rec.ID)
}

func TestCodexChatsAreNotRouted(t *testing.T) {
	m, _, fdir := routedChat(t, map[string]string{"router": "light"})
	rec, err := m.Create(CreateRequest{Agent: AgentCodex})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = m.Send(rec.ID, SendRequest{TurnID: "c_codex001", Text: "what is this?"})
	time.Sleep(200 * time.Millisecond)
	if k := kinds(calls(t, fdir)); k != "" {
		t.Errorf("a Codex chat made claude calls: %s", k)
	}
}

func TestSplitHandoff(t *testing.T) {
	for _, tc := range []struct {
		in, rest, reason string
		found            bool
	}{
		{"Sure.\n\nNACRE_HANDOFF: needs to edit files\n", "Sure.", "needs to edit files", true},
		{"NACRE_HANDOFF: build it", "", "build it", true},
		{"Use NACRE_HANDOFF: x only at the end.\nDone.", "Use NACRE_HANDOFF: x only at the end.\nDone.", "", false},
		{"plain answer", "plain answer", "", false},
	} {
		rest, reason, found := splitHandoff(tc.in)
		if rest != tc.rest || reason != tc.reason || found != tc.found {
			t.Errorf("splitHandoff(%q) = %q, %q, %v", tc.in, rest, reason, found)
		}
	}
}

// containsAll reports whether argv holds every one of want.
func containsAll(argv []string, want ...string) bool {
	have := map[string]bool{}
	for _, a := range argv {
		have[a] = true
	}
	for _, w := range want {
		if !have[w] {
			return false
		}
	}
	return true
}
