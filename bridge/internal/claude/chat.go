package claude

import (
	"encoding/json"
	"strings"
)

// ChatRunawayUSD is the most one Chat conversation may spend across all its
// turns. It is a runaway ceiling, not a budget: each turn starts with what is
// left of it as its --max-budget-usd.
const ChatRunawayUSD = 50.0

// Chat describes one full turn of the app's Chat: the owner's own Claude
// Code, with their MCP servers, CLAUDE.md, skills and hooks, run on this
// computer and fed one stream-json user message on stdin.
//
// Unlike [Headless] it sets no sandbox: no --safe-mode, no tool list, no
// model, no system prompt of its own and no setting or MCP overrides. The
// auto classifier decides what runs, and anything it won't allow alone is
// asked of the host (--permission-prompts host), which nacred answers on the
// control channel.
type Chat struct {
	// Briefing is appended to the owner's system prompt.
	Briefing string
	// SessionID starts a new session with this UUID (the chat's first turn).
	SessionID string
	// Resume continues the session with this UUID (every later turn). It
	// wins over SessionID.
	Resume string
	// MaxBudgetUSD is what is left of the chat's runaway ceiling.
	MaxBudgetUSD float64
	// AddDirs are the other boards on this computer.
	AddDirs []string
}

// Args returns the argv (without the binary) for the turn.
func (c Chat) Args() []string {
	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json", "--verbose",
		"--permission-mode", "auto",
		"--permission-prompts", "host",
		"--append-system-prompt", c.Briefing,
	}
	if c.Resume != "" {
		args = append(args, "--resume", c.Resume)
	} else if c.SessionID != "" {
		args = append(args, "--session-id", c.SessionID)
	}
	args = append(args, budgetArgs(c.MaxBudgetUSD, ChatRunawayUSD)...)
	for _, d := range c.AddDirs {
		args = append(args, "--add-dir", d)
	}
	return args
}

// ChatEnv is the environment of a Chat turn: nacred's own environment
// (including the owner's HOME, and without any CLAUDE_CONFIG_DIR nacred would
// set, since it sets none), the sign-in token when the app signed this
// computer in, the added boards' CLAUDE.md files, and NACRED_URL so the agent
// reaches nacred at the address it actually listens on.
func ChatEnv(nacredURL string) []string {
	env := RunEnv()
	out := env[:0:0]
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if k == "NACRED_URL" {
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD=1")
	if nacredURL != "" {
		out = append(out, "NACRED_URL="+nacredURL)
	}
	return out
}

// ChatUserMessage is the stream-json user message for one chat message: the
// "On screen" note, then the text, as separate text blocks. It ends with a
// newline.
func ChatUserMessage(note, text string) ([]byte, error) {
	content := []map[string]any{}
	if strings.TrimSpace(note) != "" {
		content = append(content, map[string]any{"type": "text", "text": note})
	}
	if strings.TrimSpace(text) != "" {
		content = append(content, map[string]any{"type": "text", "text": text})
	}
	msg := map[string]any{
		"type":    "user",
		"message": map[string]any{"role": "user", "content": content},
	}
	out, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// ControlRequest is a request Claude Code sends the host on stdout when it
// runs with --permission-prompts host, such as asking whether a tool may run.
type ControlRequest struct {
	RequestID string
	Subtype   string
	ToolName  string
	Input     json.RawMessage
	// Reason is why the classifier didn't allow the tool on its own, when
	// the CLI says.
	Reason string
	ToolID string
}

// ParseControlRequest reports whether line is a control_request and, if so,
// returns it.
func ParseControlRequest(line []byte) (ControlRequest, bool) {
	var m struct {
		Type      string `json:"type"`
		RequestID string `json:"request_id"`
		Request   struct {
			Subtype        string          `json:"subtype"`
			ToolName       string          `json:"tool_name"`
			Input          json.RawMessage `json:"input"`
			DecisionReason json.RawMessage `json:"decision_reason"`
			Reason         string          `json:"reason"`
			ToolUseID      string          `json:"tool_use_id"`
		} `json:"request"`
	}
	if json.Unmarshal(line, &m) != nil || m.Type != "control_request" || m.RequestID == "" {
		return ControlRequest{}, false
	}
	reason := m.Request.Reason
	if reason == "" && len(m.Request.DecisionReason) > 0 {
		var s string
		if json.Unmarshal(m.Request.DecisionReason, &s) == nil {
			reason = s
		} else {
			var o struct {
				Reason string `json:"reason"`
			}
			if json.Unmarshal(m.Request.DecisionReason, &o) == nil {
				reason = o.Reason
			}
		}
	}
	return ControlRequest{
		RequestID: m.RequestID, Subtype: m.Request.Subtype, ToolName: m.Request.ToolName,
		Input: m.Request.Input, Reason: reason, ToolID: m.Request.ToolUseID,
	}, true
}

// DenyControl is the control_response that refuses a can_use_tool request
// with message, which the agent sees as the reason.
func DenyControl(requestID, message string) []byte {
	return controlResponse(map[string]any{
		"subtype": "success", "request_id": requestID,
		"response": map[string]any{"behavior": "deny", "message": message},
	})
}

// UnsupportedControl answers a control request nacred doesn't handle, so the
// CLI never waits on it.
func UnsupportedControl(requestID, subtype string) []byte {
	return controlResponse(map[string]any{
		"subtype": "error", "request_id": requestID,
		"error": "nacred does not handle " + subtype + " requests",
	})
}

func controlResponse(resp map[string]any) []byte {
	out, _ := json.Marshal(map[string]any{"type": "control_response", "response": resp})
	return append(out, '\n')
}
