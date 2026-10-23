package claude

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// index returns the position of flag in args, or -1.
func index(args []string, flag string) int { return slices.Index(args, flag) }

func TestHeadlessArgsStructured(t *testing.T) {
	h := Headless{
		Prompt: "Improve this bead",
		System: "You are an editor",
		Schema: json.RawMessage(`{"type":"object"}`),
		Model:  "claude-opus-5-5",
		Effort: "high",
	}
	args := h.Args()
	joined := strings.Join(args, " ")

	for _, want := range []string{"-p", "--safe-mode", "--no-session-persistence", "--json-schema", "--system-prompt"} {
		if index(args, want) < 0 {
			t.Errorf("args missing %s: %v", want, args)
		}
	}
	// No tools at all for a structured call, and nothing that could prompt.
	if i := index(args, "--tools"); i < 0 || args[i+1] != "" {
		t.Errorf("structured call should disable tools: %v", args)
	}
	if i := index(args, "--permission-prompts"); i < 0 || args[i+1] != "none" {
		t.Errorf("permission prompts should be denied, not asked: %v", args)
	}
	if !strings.Contains(joined, "--model claude-opus-5-5") || !strings.Contains(joined, "--effort high") {
		t.Errorf("model/effort not passed through: %v", args)
	}
	// stream-json so the caller can follow progress, even for one-shot calls.
	if i := index(args, "--output-format"); i < 0 || args[i+1] != "stream-json" {
		t.Errorf("want stream-json output: %v", args)
	}
}

func TestHeadlessArgsAssistantToolsAreReadOnly(t *testing.T) {
	h := Headless{Prompt: "What's next?", AllowedTools: BdReadTools, Persist: true, Resume: "sess-1"}
	args := h.Args()

	if i := index(args, "--tools"); i < 0 || args[i+1] != "Bash" {
		t.Fatalf("assistant should get Bash and nothing else: %v", args)
	}
	if i := index(args, "--resume"); i < 0 || args[i+1] != "sess-1" {
		t.Errorf("resume not passed: %v", args)
	}
	// A resumable session must not be told to forget itself.
	if index(args, "--no-session-persistence") >= 0 {
		t.Errorf("a kept session must stay on disk: %v", args)
	}
	// Every allowed pattern is a bd subcommand that cannot change anything.
	for _, pattern := range BdReadTools {
		if !strings.HasPrefix(pattern, "Bash(bd ") {
			t.Errorf("allowed tool %q is not a bd command", pattern)
		}
		// "bd comments" reads; "bd comment" writes — the patterns are prefixes,
		// so only an exact subcommand match would let a write through.
		for _, verb := range []string{"create", "update", "close", "delete", "reopen", "comment", "dep add", "dolt"} {
			if pattern == "Bash(bd "+verb+":*)" {
				t.Errorf("allowed tool %q can write", pattern)
			}
		}
	}
	if index(args, "--disallowed-tools") < 0 {
		t.Errorf("write bd commands should also be denied outright: %v", args)
	}
}

func TestHeadlessArgsLeadingDashPrompt(t *testing.T) {
	args := Headless{Prompt: "--help me"}.Args()
	if args[1] != " --help me" {
		t.Errorf("a prompt starting with - must not look like a flag: %q", args[1])
	}
}

func TestParseLineStructuredOutput(t *testing.T) {
	line := []byte(`{"type":"result","subtype":"success","is_error":false,"result":"{\"answer\":\"7\"}","structured_output":{"answer":"7"},"session_id":"s1","total_cost_usd":0.002}`)
	p, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if p.Result == nil {
		t.Fatal("no result")
	}
	if got := string(p.Result.Structured); got != `{"answer":"7"}` {
		t.Errorf("structured = %s", got)
	}
	if p.Result.SessionID != "s1" {
		t.Errorf("session = %q", p.Result.SessionID)
	}
}
