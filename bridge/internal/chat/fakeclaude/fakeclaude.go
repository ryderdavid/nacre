// Package fakeclaude is a claude stand-in for tests that replays the host
// permission prompt recorded from the installed Claude Code
// (internal/chat/testdata/host_prompt_{allow,deny}.jsonl).
//
// A test package calls [RunIfFake] first in its TestMain, then [Install] to
// get a "claude" binary: a script that re-runs the test binary in fake mode.
// The fake prints the recorded init, tool_use and can_use_tool control
// request, waits for the control_response on stdin, then replays the allow
// or deny recording (a deny's tool_result carries the message it was given),
// prints the result and waits for stdin to close.
//
// Under the fake's directory it writes argv (one per line), stdin (the user
// message), pid, answer (the control_response line), ran (only when the step
// was allowed) and exit (once it finishes normally).
//
// Chat routing adds three more kinds of call, told apart by their argv:
//
//   - A routing call (--json-schema) answers with the route named in the file
//     "router" (light, full, error, malformed, slow or rejected; default full),
//     costing "router_cost" (default 0.01).
//   - A light turn (--permission-mode default with --model) replays
//     testdata/<name>.jsonl, the name read from the file "light" (default
//     light_plain). Each stdin line of the recording after the first is a
//     point where the fake reads one line from its stdin, as the CLI waited
//     for the host there. A recording's /tmp/probe/mock is replaced with the
//     call's NACRE_MOCKUPS_DIR.
//   - A drafting turn (--permission-mode default, no --model) replays the
//     recording named in "drafting" (default default_allowlist_drafting).
//
// A full turn replays testdata/<name>.jsonl the same way when the file "full"
// names a recording, and otherwise the host prompt as above.
//
// Every call also writes call<N>.kind (router, light, drafting or full), call<N>.argv
// (a JSON array) and call<N>.stdin (every line it read), N counting from 0.
package fakeclaude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	envDir      = "NACRE_FAKE_CLAUDE_DIR"
	envFixtures = "NACRE_FAKE_CLAUDE_FIXTURES"
)

// Install writes a claude stand-in replaying the recordings in fixtures (a
// testdata directory) and returns its path and its record directory.
func Install(t testing.TB, fixtures string) (bin, dir string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	dir = t.TempDir()
	bin = filepath.Join(dir, "claude")
	script := fmt.Sprintf("#!/bin/sh\n%s=%s %s=%s exec %s \"$@\"\n",
		envDir, shellQuote(dir), envFixtures, shellQuote(abs), shellQuote(exe))
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, dir
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// RunIfFake runs the fake and exits when this process was started as one.
func RunIfFake() {
	dir := os.Getenv(envDir)
	if dir == "" {
		return
	}
	if err := run(dir, os.Getenv(envFixtures), os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "fakeclaude:", err)
		os.Exit(3)
	}
	os.Exit(0)
}

type record struct {
	Dir  string          `json:"dir"`
	Line json.RawMessage `json:"line"`
}

func load(path string) ([]record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []record
	for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var r record
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func lineType(raw json.RawMessage) string {
	var m struct{ Type string }
	_ = json.Unmarshal(raw, &m)
	return m.Type
}

// split returns the recorded stdout before and including the control request,
// that request, and the stdout after the answer.
func split(recs []record) (before []json.RawMessage, req map[string]any, after []json.RawMessage) {
	seen := false
	for _, r := range recs {
		if r.Dir != "stdout" {
			continue
		}
		if !seen {
			before = append(before, r.Line)
			if lineType(r.Line) == "control_request" {
				seen = true
				_ = json.Unmarshal(r.Line, &req)
			}
			continue
		}
		after = append(after, r.Line)
	}
	return before, req, after
}

// withSession replaces session_id fields with sid.
func withSession(raw json.RawMessage, sid string) []byte {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return raw
	}
	if _, ok := m["session_id"]; ok {
		m["session_id"] = sid
	}
	out, _ := json.Marshal(m)
	return out
}

func has(args []string, flag, value string) bool {
	for i, a := range args {
		if a == flag && (value == "" || (i+1 < len(args) && args[i+1] == value)) {
			return true
		}
	}
	return false
}

func setting(dir, name, def string) string {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return def
	}
	if s := strings.TrimSpace(string(data)); s != "" {
		return s
	}
	return def
}

func run(dir, fixtures string, args []string, stdin io.Reader, stdout io.Writer) error {
	write := func(name, content string) { _ = os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644) }
	kinds, _ := filepath.Glob(filepath.Join(dir, "call*.kind"))
	n := len(kinds)
	call := func(name string) string { return fmt.Sprintf("call%d.%s", n, name) }
	kind := "full"
	switch {
	case has(args, "--json-schema", ""):
		kind = "router"
	case has(args, "--permission-mode", "default") && has(args, "--model", ""):
		kind = "light"
	case has(args, "--permission-mode", "default"):
		kind = "drafting"
	}
	argvJSON, _ := json.Marshal(args)
	write(call("argv"), string(argvJSON))
	write(call("kind"), kind)
	write("pid", strconv.Itoa(os.Getpid()))
	sid := ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--session-id" || args[i] == "--resume" {
			sid = args[i+1]
		}
	}
	switch kind {
	case "router":
		return router(dir, stdout)
	case "light":
		return replay(dir, filepath.Join(fixtures, setting(dir, "light", "light_plain")+".jsonl"), sid, call("stdin"), stdin, stdout)
	case "drafting":
		return replay(dir, filepath.Join(fixtures, setting(dir, "drafting", "default_allowlist_drafting")+".jsonl"), sid, call("stdin"), stdin, stdout)
	}
	if name := setting(dir, "full", ""); name != "" {
		return replay(dir, filepath.Join(fixtures, name+".jsonl"), sid, call("stdin"), stdin, stdout)
	}
	write("argv", strings.Join(args, "\n")+"\n")
	deny, err := load(filepath.Join(fixtures, "host_prompt_deny.jsonl"))
	if err != nil {
		return err
	}
	allow, err := load(filepath.Join(fixtures, "host_prompt_allow.jsonl"))
	if err != nil {
		return err
	}
	in := bufio.NewReader(stdin)
	first, err := in.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read user message: %w", err)
	}
	write("stdin", first)

	before, req, denyAfter := split(deny)
	_, _, allowAfter := split(allow)
	out := bufio.NewWriter(stdout)
	emit := func(b []byte) {
		out.Write(b)
		out.WriteByte('\n')
		out.Flush()
	}
	for _, l := range before {
		emit(withSession(l, sid))
	}
	answer, err := in.ReadString('\n')
	if err != nil {
		return fmt.Errorf("no answer to the permission request: %w", err)
	}
	write("answer", answer)
	var resp struct {
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
	if err := json.Unmarshal([]byte(answer), &resp); err != nil {
		return fmt.Errorf("answer is not JSON: %w", err)
	}
	if resp.Type != "control_response" || resp.Response.RequestID != req["request_id"] {
		return fmt.Errorf("answer %q does not answer %v", answer, req["request_id"])
	}
	toolID := ""
	if r, ok := req["request"].(map[string]any); ok {
		toolID, _ = r["tool_use_id"].(string)
	}
	var rest []json.RawMessage
	switch resp.Response.Response.Behavior {
	case "allow":
		if len(resp.Response.Response.UpdatedInput) == 0 {
			return fmt.Errorf("allow without updatedInput")
		}
		write("ran", string(resp.Response.Response.UpdatedInput))
		rest = allowAfter
	case "deny":
		rest = denyAfter
	default:
		return fmt.Errorf("unknown behavior %q", resp.Response.Response.Behavior)
	}
	for _, l := range rest {
		var m map[string]any
		_ = json.Unmarshal(l, &m)
		if m["type"] == "user" {
			// The tool result answers this run's tool call; a denial carries
			// the message the host gave, as the recording shows.
			if msg, ok := m["message"].(map[string]any); ok {
				if content, ok := msg["content"].([]any); ok {
					for _, c := range content {
						if cm, ok := c.(map[string]any); ok && cm["type"] == "tool_result" {
							cm["tool_use_id"] = toolID
							if resp.Response.Response.Behavior == "deny" {
								cm["content"] = resp.Response.Response.Message
							}
						}
					}
				}
			}
			l, _ = json.Marshal(m)
		}
		emit(withSession(l, sid))
	}
	// Like the CLI, wait for the host to close stdin.
	_, _ = io.Copy(io.Discard, in)
	write("exit", "ok")
	return nil
}

// router answers a routing call as the "router" file says.
func router(dir string, stdout io.Writer) error {
	cost := setting(dir, "router_cost", "0.01")
	result := func(isErr bool, text, structured string) {
		fmt.Fprintf(stdout, `{"type":"system","subtype":"init","session_id":"00000000-0000-4000-8000-00000000r0u7","model":"claude-haiku-5-5"}`+"\n")
		line := fmt.Sprintf(`{"type":"result","subtype":"success","is_error":%t,"result":%q,"total_cost_usd":%s`, isErr, text, cost)
		if structured != "" {
			line += `,"structured_output":` + structured
		}
		fmt.Fprintln(stdout, line+"}")
	}
	switch mode := setting(dir, "router", "full"); mode {
	case "light", "full":
		result(false, "", `{"route":"`+mode+`","reason":"fake router chose `+mode+`"}`)
	case "malformed":
		result(false, "", `{"route":"maybe","reason":"unsure"}`)
	case "rejected":
		cost = "0"
		result(true, "There's an issue with the selected model (claude-nope). It may not exist or you may not have access to it. Run --model to pick a different model.", "")
	case "slow":
		time.Sleep(time.Minute)
	case "error":
		fmt.Fprintln(os.Stderr, "router exploded")
		os.Exit(1)
	default:
		return fmt.Errorf("unknown router mode %q", mode)
	}
	return nil
}

// replay plays a recording: its stdout lines in order, and at each recorded
// stdin line after the first, one line read from stdin. A recorded answer to
// a control request takes the id of the request the host actually sent.
func replay(dir, path, sid, stdinFile string, stdin io.Reader, stdout io.Writer) error {
	recs, err := load(path)
	if err != nil {
		return err
	}
	in := bufio.NewReader(stdin)
	var got []string
	save := func() { _ = os.WriteFile(filepath.Join(dir, stdinFile), []byte(strings.Join(got, "")), 0o644) }
	out := bufio.NewWriter(stdout)
	lastHostReq := ""
	firstIn := true
	for _, r := range recs {
		if r.Dir == "stdin" {
			line, err := in.ReadString('\n')
			if err != nil {
				save()
				return fmt.Errorf("stdin closed before the recording's next input: %w", err)
			}
			got = append(got, line)
			save()
			if firstIn {
				firstIn = false
				continue
			}
			var m struct {
				Type      string `json:"type"`
				RequestID string `json:"request_id"`
			}
			if json.Unmarshal([]byte(line), &m) == nil && m.Type == "control_request" {
				lastHostReq = m.RequestID
			}
			continue
		}
		line := []byte(r.Line)
		if mock := os.Getenv("NACRE_MOCKUPS_DIR"); mock != "" {
			// Recordings name their mockups directory /tmp/probe/mock.
			line = []byte(strings.ReplaceAll(string(line), "/tmp/probe/mock", mock))
		}
		if lineType(r.Line) == "control_response" && lastHostReq != "" {
			var m map[string]any
			_ = json.Unmarshal(r.Line, &m)
			if resp, ok := m["response"].(map[string]any); ok {
				resp["request_id"] = lastHostReq
			}
			line, _ = json.Marshal(m)
		}
		out.Write(withSession(line, sid))
		out.WriteByte('\n')
		out.Flush()
	}
	// Like the CLI, wait for the host to close stdin, keeping what it sent.
	for {
		line, err := in.ReadString('\n')
		if line != "" {
			got = append(got, line)
			save()
		}
		if err != nil {
			break
		}
	}
	_ = os.WriteFile(filepath.Join(dir, "exit"), []byte("ok"), 0o644)
	return nil
}
