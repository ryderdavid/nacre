package chat

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// fakeClaude writes a claude stand-in into a new directory. It records its
// argv (one per line), cwd, environment and the first stdin line under that
// directory, prints an init line for the session it was given, runs extra
// (more shell), prints a result costing cost, and waits for stdin to close.
func fakeClaude(t *testing.T, cost string, extra string) (bin, dir string) {
	t.Helper()
	dir = t.TempDir()
	bin = filepath.Join(dir, "claude")
	script := `#!/bin/sh
d="$(dirname "$0")"
n=$(ls "$d" | grep -c '^argv')
: > "$d/argv$n"
for a in "$@"; do printf '%s\n' "$a" >> "$d/argv$n"; done
pwd > "$d/cwd$n"
env > "$d/env$n"
sid=""; prev=""
for a in "$@"; do
  if [ "$prev" = "--session-id" ] || [ "$prev" = "--resume" ]; then sid="$a"; fi
  prev="$a"
done
IFS= read -r first
printf '%s\n' "$first" > "$d/stdin$n"
printf '{"type":"system","subtype":"init","session_id":"%s","model":"claude-opus-5-5"}\n' "$sid"
printf '{"type":"assistant","message":{"content":[{"type":"text","text":"working on it"}]}}\n'
` + extra + `
printf '{"type":"result","subtype":"success","is_error":false,"session_id":"%s","result":"done","total_cost_usd":` + cost + `}\n' "$sid"
cat > /dev/null
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, dir
}

func newManager(t *testing.T, dir, bin string, opts ...func(*Options)) *Manager {
	t.Helper()
	o := Options{
		Dir: dir, ClaudePath: bin, NacredURL: "http://100.64.0.7:7420", HostID: "h_1", HostName: "Mac-Studio",
		Logger: log.New(io.Discard, "", 0),
		Whois:  func(string) (string, error) { return "", fmt.Errorf("no tailscale") },
	}
	for _, f := range opts {
		f(&o)
	}
	m, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// waitIdle waits for the chat's turn to end.
func waitIdle(t *testing.T, m *Manager, id string) Record {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		snap, err := m.Since(id, 0)
		if err != nil {
			t.Fatal(err)
		}
		if st := snap.Record.State; st != StateRunning && st != StateWaitingPermission {
			return snap.Record
		}
		select {
		case <-snap.Changed:
		case <-time.After(200 * time.Millisecond):
		}
	}
	t.Fatal("turn did not finish")
	return Record{}
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

func strp(s string) *string { return &s }

// repoSetup is a host with three boards; repo key "r-a" is checkout a.
func repoSetup(t *testing.T) (repoA string, boards []string, opt func(*Options)) {
	t.Helper()
	root := t.TempDir()
	for _, n := range []string{"a", "b", "c"} {
		p := filepath.Join(root, n)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		boards = append(boards, p)
	}
	repoA = boards[0]
	opt = func(o *Options) {
		o.RepoPath = func(key string) (string, error) {
			if key == "r-a" {
				return repoA, nil
			}
			return "", fmt.Errorf("unknown")
		}
		o.Boards = func() []string { return boards }
	}
	return repoA, boards, opt
}

func TestFirstResumedAndAcrossBoardsArgv(t *testing.T) {
	bin, fdir := fakeClaude(t, "1.5", "")
	repoA, boards, opt := repoSetup(t)
	m := newManager(t, t.TempDir(), bin, opt)

	rec, err := m.Create(CreateRequest{Agent: "claude", Repo: strp("r-a")})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Cwd != repoA {
		t.Fatalf("cwd = %q, want the checkout %q", rec.Cwd, repoA)
	}
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_first01", Text: "hello", Context: &Context{Screen: "board", Repo: "r-a"}}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m, rec.ID)
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_second1", Text: "and again", Context: &Context{Screen: "board", Repo: "r-a"}}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m, rec.ID)

	brief := Briefing(BriefingInput{Host: "Mac-Studio", NacredURL: "http://100.64.0.7:7420"})
	want0 := []string{
		"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose",
		"--permission-mode", "auto", "--permission-prompts", "host",
		"--append-system-prompt", brief,
		"--session-id", rec.SessionID,
		"--max-budget-usd", "50.00",
		"--add-dir", boards[1], "--add-dir", boards[2],
	}
	got0 := strings.Join(readLines(t, filepath.Join(fdir, "argv0")), "\n")
	if got0 != strings.Join(want0, "\n") {
		t.Errorf("first turn argv:\n%s\nwant:\n%s", got0, strings.Join(want0, "\n"))
	}
	want1 := append([]string{}, want0...)
	want1[12], want1[13], want1[15] = "--resume", rec.SessionID, "48.50"
	got1 := strings.Join(readLines(t, filepath.Join(fdir, "argv1")), "\n")
	if got1 != strings.Join(want1, "\n") {
		t.Errorf("resumed turn argv:\n%s\nwant:\n%s", got1, strings.Join(want1, "\n"))
	}
	for i := 0; i < 2; i++ {
		cwd := readLines(t, filepath.Join(fdir, fmt.Sprintf("cwd%d", i)))[0]
		if r1, _ := filepath.EvalSymlinks(cwd); r1 != mustEval(t, repoA) {
			t.Errorf("turn %d cwd = %q, want %q", i, cwd, repoA)
		}
	}

	// An across-boards chat runs in chats/work with every board added.
	bin2, fdir2 := fakeClaude(t, "0", "")
	m2 := newManager(t, t.TempDir(), bin2, opt)
	across, err := m2.Create(CreateRequest{Agent: "claude", Context: &Context{Screen: "boards"}})
	if err != nil {
		t.Fatal(err)
	}
	if across.Repo != nil || across.Cwd != filepath.Join(m2.opts.Dir, "work") {
		t.Fatalf("across-boards chat: repo %v cwd %q", across.Repo, across.Cwd)
	}
	if _, err := m2.Send(across.ID, SendRequest{TurnID: "c_across1", Text: "what's next?", Context: &Context{Screen: "boards"}}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m2, across.ID)
	want2 := append(append([]string{}, want0[:12]...), "--session-id", across.SessionID, "--max-budget-usd", "50.00",
		"--add-dir", boards[0], "--add-dir", boards[1], "--add-dir", boards[2])
	got2 := strings.Join(readLines(t, filepath.Join(fdir2, "argv0")), "\n")
	if got2 != strings.Join(want2, "\n") {
		t.Errorf("across-boards argv:\n%s\nwant:\n%s", got2, strings.Join(want2, "\n"))
	}
	if cwd := readLines(t, filepath.Join(fdir2, "cwd0"))[0]; mustEval(t, cwd) != mustEval(t, across.Cwd) {
		t.Errorf("across-boards cwd = %q", cwd)
	}
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestArgvLeavesTheOwnersSetupAlone(t *testing.T) {
	for _, c := range []claude.Chat{
		{Briefing: "b", SessionID: "s", MaxBudgetUSD: 50},
		{Briefing: "b", Resume: "s", MaxBudgetUSD: 1, AddDirs: []string{"/x"}},
	} {
		args := c.Args()
		for _, banned := range []string{"--safe-mode", "--no-session-persistence", "--tools", "--model", "--system-prompt",
			"--setting-sources", "--strict-mcp-config", "--mcp-config", "--allowed-tools", "--disallowed-tools", "--bare"} {
			for _, a := range args {
				if a == banned {
					t.Errorf("argv has %s: %v", banned, args)
				}
			}
		}
	}
}

func TestChildEnvironment(t *testing.T) {
	os.Unsetenv("CLAUDE_CONFIG_DIR")
	t.Setenv("HOME", "/Users/owner")
	t.Setenv("NACRED_URL", "http://stale")
	bin, fdir := fakeClaude(t, "0", "")
	m := newManager(t, t.TempDir(), bin, func(o *Options) { o.NacredURL = "http://100.64.0.7:7420" })
	rec, _ := m.Create(CreateRequest{Agent: "claude"})
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_envtest", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m, rec.ID)
	env := readLines(t, filepath.Join(fdir, "env0"))
	has := func(kv string) bool {
		for _, e := range env {
			if e == kv {
				return true
			}
		}
		return false
	}
	for _, e := range env {
		if strings.HasPrefix(e, "CLAUDE_CONFIG_DIR=") {
			t.Errorf("child has %s", e)
		}
		if strings.HasPrefix(e, "NACRED_URL=") && e != "NACRED_URL=http://100.64.0.7:7420" {
			t.Errorf("child has %s", e)
		}
	}
	for _, kv := range []string{"HOME=/Users/owner", "CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD=1", "NACRED_URL=http://100.64.0.7:7420"} {
		if !has(kv) {
			t.Errorf("child environment lacks %s", kv)
		}
	}
}

func TestBudgetRemainderAndLimit(t *testing.T) {
	bin, fdir := fakeClaude(t, "26.0", "")
	m := newManager(t, t.TempDir(), bin)
	rec, _ := m.Create(CreateRequest{Agent: "claude"})
	// A chat that has spent $12 starts its next turn with $38 left.
	m.mu.Lock()
	m.chats[rec.ID].rec.CostUSD = 12
	m.mu.Unlock()
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_budget1", Text: "go"}); err != nil {
		t.Fatal(err)
	}
	got := waitIdle(t, m, rec.ID)
	argv := readLines(t, filepath.Join(fdir, "argv0"))
	if i := indexOf(argv, "--max-budget-usd"); i < 0 || argv[i+1] != "38.00" {
		t.Errorf("budget flag: %v", argv)
	}
	if got.CostUSD != 38 || got.State != StateIdle {
		t.Fatalf("after $26: cost %v state %s", got.CostUSD, got.State)
	}
	// The next turn costs past $50: the chat is limit_reached and refuses.
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_budget2", Text: "more"}); err != nil {
		t.Fatal(err)
	}
	got = waitIdle(t, m, rec.ID)
	if got.State != StateLimitReached || got.Reason != LimitReason {
		t.Fatalf("state %s reason %q", got.State, got.Reason)
	}
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_budget3", Text: "and more"}); err != ErrLimit || err.Error() != "This chat reached its runaway limit. Start a new chat to keep going." {
		t.Fatalf("send at limit: %v", err)
	}
}

func indexOf(xs []string, s string) int {
	for i, x := range xs {
		if x == s {
			return i
		}
	}
	return -1
}

func TestResendReturnsTheSameTurn(t *testing.T) {
	bin, fdir := fakeClaude(t, "0.1", "sleep 0.3")
	m := newManager(t, t.TempDir(), bin)
	rec, _ := m.Create(CreateRequest{Agent: "claude"})
	first, err := m.Send(rec.ID, SendRequest{TurnID: "c_resend01", Text: "once"})
	if err != nil || !first.Started {
		t.Fatalf("first: %+v %v", first, err)
	}
	// Resent while it runs, and again after it ended: the same turn, nothing new.
	again, err := m.Send(rec.ID, SendRequest{TurnID: "c_resend01", Text: "once"})
	if err != nil || again.Started || again.TurnID != "c_resend01" {
		t.Fatalf("resend while running: %+v %v", again, err)
	}
	// A different message while the turn runs is refused.
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_other001", Text: "two"}); err != ErrBusy {
		t.Fatalf("second message while running: %v", err)
	}
	waitIdle(t, m, rec.ID)
	again, err = m.Send(rec.ID, SendRequest{TurnID: "c_resend01", Text: "once"})
	if err != nil || again.Started {
		t.Fatalf("resend after: %+v %v", again, err)
	}
	if _, err := os.Stat(filepath.Join(fdir, "argv1")); err == nil {
		t.Fatal("a resend started a second process")
	}
	_, evs, _ := m.Get(rec.ID, 0)
	users := 0
	for _, e := range evs {
		if e.Kind == KindUser {
			users++
		}
	}
	if users != 1 {
		t.Fatalf("%d user events, want 1", users)
	}
}

func TestReattachReplaysExactlyTheMissingEvents(t *testing.T) {
	bin, _ := fakeClaude(t, "0", `printf '{"type":"assistant","message":{"content":[{"type":"text","text":"two"}]}}\n'
printf '{"type":"assistant","message":{"content":[{"type":"text","text":"three"}]}}\n'`)
	m := newManager(t, t.TempDir(), bin)
	rec, _ := m.Create(CreateRequest{Agent: "claude"})
	m.Send(rec.ID, SendRequest{TurnID: "c_replay01", Text: "go"})
	waitIdle(t, m, rec.ID)
	all, _ := m.Since(rec.ID, 0)
	if len(all.Events) < 5 {
		t.Fatalf("events: %+v", all.Events)
	}
	for i, ev := range all.Events {
		if ev.Seq != int64(i+1) {
			t.Fatalf("seq %d at %d", ev.Seq, i)
		}
	}
	cut := all.Events[2].Seq
	rest, _ := m.Since(rec.ID, cut)
	if len(rest.Events) != len(all.Events)-3 || rest.Events[0].Seq != cut+1 {
		t.Fatalf("after %d got %d events starting %d", cut, len(rest.Events), rest.Events[0].Seq)
	}
	// The log on disk holds the same events, so a restart replays them too.
	m2 := newManager(t, m.opts.Dir, bin)
	again, _ := m2.Since(rec.ID, cut)
	if len(again.Events) != len(rest.Events) {
		t.Fatalf("after restart: %d events, want %d", len(again.Events), len(rest.Events))
	}
	// Assistant events record their route.
	for _, ev := range all.Events {
		if ev.Kind == "text" && ev.Route != RouteFull {
			t.Errorf("text event route %q", ev.Route)
		}
	}
}

func TestRestartInterruptsAndNextTurnResumes(t *testing.T) {
	dir := t.TempDir()
	// The first process gets its session, then hangs.
	bin, fdir := fakeClaude(t, "0", "sleep 30")
	m := newManager(t, dir, bin)
	rec, _ := m.Create(CreateRequest{Agent: "claude"})
	m.Send(rec.ID, SendRequest{TurnID: "c_restart1", Text: "long job"})
	deadline := time.Now().Add(5 * time.Second)
	for {
		r, _, _ := m.Get(rec.ID, 0)
		if r.SessionStarted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("session never started")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// nacred restarts: a new manager over the same directory.
	bin2, fdir2 := fakeClaude(t, "0", "")
	m2 := newManager(t, dir, bin2)
	r2, evs, _ := m2.Get(rec.ID, 0)
	if r2.State != StateInterrupted {
		t.Fatalf("state after restart = %s", r2.State)
	}
	if last := evs[len(evs)-1]; !strings.Contains(last.Text, "Interrupted") {
		t.Fatalf("last event = %+v", last)
	}
	m.Cancel(rec.ID) // stop the old process
	waitIdle(t, m, rec.ID)
	if _, err := m2.Send(rec.ID, SendRequest{TurnID: "c_restart2", Text: "carry on"}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m2, rec.ID)
	argv := readLines(t, filepath.Join(fdir2, "argv0"))
	if i := indexOf(argv, "--resume"); i < 0 || argv[i+1] != rec.SessionID {
		t.Fatalf("next turn argv: %v", argv)
	}
	_ = fdir
}

func TestContextNotes(t *testing.T) {
	repoA, _, opt := repoSetup(t)
	var o Options
	opt(&o)
	bead := &Context{Screen: "bead", Repo: "r-a", BeadID: "NAC-3", Summary: "Fix the header · open · P2"}
	full := Note(bead, nil, "Mac-Studio", o.RepoPath)
	for _, want := range []string{"On screen: bead NAC-3", "Repository: r-a (checkout " + repoA + ")", "Bead: NAC-3", "Fix the header"} {
		if !strings.Contains(full, want) {
			t.Errorf("full note lacks %q:\n%s", want, full)
		}
	}
	same := Note(bead, &Context{Screen: "bead", Repo: "r-a", BeadID: "NAC-3", Summary: "Fix the header · open · P2"}, "Mac-Studio", o.RepoPath)
	if same != "On screen: same as before (NAC-3)" {
		t.Errorf("same screen note = %q", same)
	}
	round := &Context{Screen: "round", Repo: "r-a", RoundID: "r-12", Selection: "NAC-3", Summary: "paused · stop: gates failed · choices: retry, cancel"}
	changed := Note(round, bead, "Mac-Studio", o.RepoPath)
	for _, want := range []string{"On screen: round r-12", "Round: r-12", "Selected: NAC-3", "stop: gates failed"} {
		if !strings.Contains(changed, want) {
			t.Errorf("changed-screen note lacks %q:\n%s", want, changed)
		}
	}
	inbox := Note(&Context{Screen: "inbox"}, nil, "Mac-Studio", o.RepoPath)
	if !strings.Contains(inbox, "On screen: Inbox, across all boards on this computer") || !strings.Contains(inbox, "about all boards on this computer") {
		t.Errorf("inbox note = %q", inbox)
	}
	if one := Note(&Context{Screen: "inbox"}, &Context{Screen: "inbox"}, "Mac-Studio", nil); one != "On screen: same as before (all boards)" {
		t.Errorf("inbox reminder = %q", one)
	}
	away := Note(&Context{Screen: "bead", Repo: "MacBook~r-z", Elsewhere: "MacBook", BeadID: "ZZ-1"}, nil, "Mac-Studio", o.RepoPath)
	if !strings.Contains(away, "Repository: MacBook~r-z is on MacBook, not on this computer (Mac-Studio)") {
		t.Errorf("elsewhere note = %q", away)
	}
}

func TestNotesReachStdinBeforeTheText(t *testing.T) {
	bin, fdir := fakeClaude(t, "0", "")
	_, _, opt := repoSetup(t)
	m := newManager(t, t.TempDir(), bin, opt)
	rec, _ := m.Create(CreateRequest{Agent: "claude", Repo: strp("r-a")})
	ctx := &Context{Screen: "bead", Repo: "r-a", BeadID: "NAC-3"}
	m.Send(rec.ID, SendRequest{TurnID: "c_stdin001", Text: "close this one", Context: ctx})
	waitIdle(t, m, rec.ID)
	m.Send(rec.ID, SendRequest{TurnID: "c_stdin002", Text: "and reopen it", Context: ctx})
	waitIdle(t, m, rec.ID)
	for i, want := range []string{"On screen: bead NAC-3", "On screen: same as before (NAC-3)"} {
		var msg struct {
			Type    string
			Message struct {
				Content []struct{ Type, Text string }
			}
		}
		line := readLines(t, filepath.Join(fdir, fmt.Sprintf("stdin%d", i)))[0]
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatal(err)
		}
		if msg.Type != "user" || len(msg.Message.Content) != 2 || !strings.HasPrefix(msg.Message.Content[0].Text, want) {
			t.Errorf("turn %d stdin = %s", i, line)
		}
	}
}

func TestHostPromptIsDeniedAtOnce(t *testing.T) {
	bin, fdir := fakeClaude(t, "0.2", `printf '{"type":"control_request","request_id":"req-1","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"rm -rf build"},"decision_reason":"deletes files"}}\n'
IFS= read -r answer
printf '%s\n' "$answer" > "$d/answer"`)
	m := newManager(t, t.TempDir(), bin)
	rec, _ := m.Create(CreateRequest{Agent: "claude"})
	m.Send(rec.ID, SendRequest{TurnID: "c_prompt01", Text: "clean up"})
	got := waitIdle(t, m, rec.ID)
	if got.State != StateIdle {
		t.Fatalf("state = %s", got.State)
	}
	var answer struct {
		Type     string
		Response struct {
			Subtype   string
			RequestID string `json:"request_id"`
			Response  struct{ Behavior, Message string }
		}
	}
	line := readLines(t, filepath.Join(fdir, "answer"))[0]
	if err := json.Unmarshal([]byte(line), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Type != "control_response" || answer.Response.RequestID != "req-1" ||
		answer.Response.Response.Behavior != "deny" || answer.Response.Response.Message != "Chat can't ask the phone yet" {
		t.Fatalf("answer = %s", line)
	}
	_, evs, _ := m.Get(rec.ID, 0)
	var perm, end *Event
	for i := range evs {
		switch evs[i].Kind {
		case KindPermission:
			perm = &evs[i]
		case KindTurnEnd:
			end = &evs[i]
		}
	}
	if perm == nil || perm.Tool != "Bash" || perm.Reason != "deletes files" || perm.Allowed == nil || *perm.Allowed {
		t.Fatalf("permission event = %+v", perm)
	}
	if end == nil || end.IsError != nil || end.Text != "Turn finished" {
		t.Fatalf("turn end = %+v", end)
	}
}

func TestBriefing(t *testing.T) {
	b := Briefing(BriefingInput{Host: "Mac-Studio", NacredURL: "http://100.64.0.7:7420"})
	for _, want := range []string{
		"`bd prime`", "openspec validate <change> --strict",
		"\"On screen:\" note", "\"This\", \"this one\" and \"here\" mean what the note names",
		"`nacred` command", "$NACRED_URL (http://100.64.0.7:7420)", "`nacred token`", "docs/API.md",
		"GET /v1/attention", "Needs you", "only for work that changes behaviour", "chore, a small fix, or a bead already linked to a change",
		"nacred round start", "ask Ryder which answer applies", "/found", "signoff",
		"lands rounds, commits `nacre.yaml` and finishes epics", "on a branch or in a git worktree (`git worktree add`)",
		"commit the edits, or tell Ryder exactly which files are left uncommitted",
		"`nacre-draft`", "Taildrop",
		"Ryder's phone's Tailscale device is unknown", "Ask Ryder for the device name",
	} {
		if !strings.Contains(b, want) {
			t.Errorf("briefing lacks %q", want)
		}
	}
	known := Briefing(BriefingInput{Host: "Mac-Studio", Phone: "pixel-fold"})
	if !strings.Contains(known, "tailscale file cp <file> pixel-fold:") || strings.Contains(known, "is unknown") {
		t.Errorf("briefing with a phone:\n%s", known)
	}
}

func TestWhoisResolvesThePhoneAtCreation(t *testing.T) {
	m := newManager(t, t.TempDir(), "/nonexistent", func(o *Options) {
		o.Whois = func(addr string) (string, error) {
			if addr == "100.101.102.103:51234" {
				return "pixel-fold", nil
			}
			return "", fmt.Errorf("unknown peer")
		}
	})
	rec, _ := m.Create(CreateRequest{Agent: "claude", PeerAddr: "100.101.102.103:51234"})
	if rec.Phone != "pixel-fold" {
		t.Fatalf("phone = %q", rec.Phone)
	}
	rec2, _ := m.Create(CreateRequest{Agent: "claude", PeerAddr: "127.0.0.1:5000"})
	if rec2.Phone != "" {
		t.Fatalf("phone = %q", rec2.Phone)
	}
	name := WhoisName([]byte(`{"Node":{"Name":"pixel-fold.tail1234.ts.net.","ComputedName":"pixel-fold"}}`))
	if name != "pixel-fold" {
		t.Fatalf("WhoisName = %q", name)
	}
}

func TestRecordFilesAndModes(t *testing.T) {
	dir := t.TempDir()
	bin, _ := fakeClaude(t, "0", "")
	m := newManager(t, dir, bin)
	rec, _ := m.Create(CreateRequest{Agent: "claude"})
	m.Send(rec.ID, SendRequest{TurnID: "c_modes001", Text: "hi"})
	waitIdle(t, m, rec.ID)
	for _, name := range []string{rec.ID + ".json", rec.ID + ".events.jsonl"} {
		st, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %o", name, st.Mode().Perm())
		}
	}
	got, _, _ := m.Get(rec.ID, 0)
	if got.Title != "hi" || got.State != StateIdle || got.CreatedAt == "" {
		t.Errorf("record = %+v", got)
	}
	// Delete removes the record, log and the chat's directory.
	os.MkdirAll(filepath.Join(dir, rec.ID, "images"), 0o700)
	if err := m.Delete(rec.ID); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{rec.ID + ".json", rec.ID + ".events.jsonl", rec.ID} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s still exists", name)
		}
	}
}

func TestRetentionKeepsTheNewestChats(t *testing.T) {
	dir := t.TempDir()
	clock := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	m := newManager(t, dir, "/nonexistent", func(o *Options) {
		o.Now = func() time.Time { clock = clock.Add(time.Second); return clock }
	})
	first, _ := m.Create(CreateRequest{Agent: "claude"})
	os.MkdirAll(filepath.Join(dir, first.ID, "images"), 0o700)
	m.mu.Lock()
	m.appendLocked(m.chats[first.ID], Event{Kind: KindSystem, Text: "x"})
	m.mu.Unlock()
	for i := 0; i < MaxChats; i++ {
		if _, err := m.Create(CreateRequest{Agent: "claude"}); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(m.List()); n != MaxChats {
		t.Fatalf("%d chats kept, want %d", n, MaxChats)
	}
	if _, _, err := m.Get(first.ID, 0); err != ErrNotFound {
		t.Fatalf("oldest chat still there: %v", err)
	}
	for _, name := range []string{first.ID + ".json", first.ID + ".events.jsonl", first.ID} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s survived the trim", name)
		}
	}
}

func TestEventCapTruncates(t *testing.T) {
	dir := t.TempDir()
	m := newManager(t, dir, "/nonexistent")
	rec, _ := m.Create(CreateRequest{Agent: "claude"})
	m.mu.Lock()
	c := m.chats[rec.ID]
	for i := 0; i < MaxEvents+25; i++ {
		m.appendLocked(c, Event{Kind: KindSystem, Text: fmt.Sprint(i)})
	}
	m.mu.Unlock()
	_, evs, _ := m.Get(rec.ID, 0)
	if len(evs) != MaxEvents+1 || evs[0].Kind != KindTruncated || evs[0].Text != "25 earlier events were dropped" {
		t.Fatalf("%d events, first %+v", len(evs), evs[0])
	}
	if evs[1].Seq != 26 || evs[0].Seq != 25 || evs[len(evs)-1].Seq != MaxEvents+25 {
		t.Fatalf("seqs %d %d %d", evs[0].Seq, evs[1].Seq, evs[len(evs)-1].Seq)
	}
	// On disk too, and a reader that missed the dropped ones gets the marker first.
	lines := readLines(t, filepath.Join(dir, rec.ID+".events.jsonl"))
	if len(lines) != MaxEvents+1 {
		t.Fatalf("%d lines on disk", len(lines))
	}
	m2 := newManager(t, dir, "/nonexistent")
	snap, _ := m2.Since(rec.ID, 3)
	if snap.Events[0].Kind != KindTruncated || len(snap.Events) != MaxEvents+1 {
		t.Fatalf("reader after 3: %d events, first %+v", len(snap.Events), snap.Events[0])
	}
}

func TestStopEndsTheTurn(t *testing.T) {
	bin, _ := fakeClaude(t, "0", "sleep 30")
	m := newManager(t, t.TempDir(), bin)
	rec, _ := m.Create(CreateRequest{Agent: "claude"})
	m.Send(rec.ID, SendRequest{TurnID: "c_stop0001", Text: "long"})
	if !m.Busy() {
		t.Fatal("a running turn doesn't count as busy")
	}
	time.Sleep(100 * time.Millisecond)
	m.Cancel(rec.ID)
	got := waitIdle(t, m, rec.ID)
	if got.State != StateIdle || m.Busy() {
		t.Fatalf("state %s busy %v", got.State, m.Busy())
	}
	_, evs, _ := m.Get(rec.ID, 0)
	if last := evs[len(evs)-1]; last.Kind != KindTurnEnd || last.Text != "Stopped" {
		t.Fatalf("last = %+v", last)
	}
}

func TestExitWithoutResultEndsWithAnError(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	os.WriteFile(bin, []byte("#!/bin/sh\necho 'boom' >&2\nexit 3\n"), 0o755)
	m := newManager(t, t.TempDir(), bin)
	rec, _ := m.Create(CreateRequest{Agent: "claude"})
	m.Send(rec.ID, SendRequest{TurnID: "c_crash001", Text: "x"})
	got := waitIdle(t, m, rec.ID)
	if got.State != StateIdle {
		t.Fatalf("state %s", got.State)
	}
	_, evs, _ := m.Get(rec.ID, 0)
	foundErr := false
	for _, e := range evs {
		if e.Kind == KindError && strings.Contains(e.Text, "status 3") && strings.Contains(e.Text, "boom") {
			foundErr = true
		}
	}
	if !foundErr {
		t.Fatalf("events: %+v", evs)
	}
}
