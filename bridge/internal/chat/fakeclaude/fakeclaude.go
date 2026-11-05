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

func run(dir, fixtures string, args []string, stdin io.Reader, stdout io.Writer) error {
	write := func(name, content string) { _ = os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644) }
	write("argv", strings.Join(args, "\n")+"\n")
	write("pid", strconv.Itoa(os.Getpid()))
	sid := ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--session-id" || args[i] == "--resume" {
			sid = args[i+1]
		}
	}
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
