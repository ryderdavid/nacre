package claude

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
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
	// Model is set only for a light turn (the chat's light model); a full
	// turn uses the owner's own default model.
	Model string
	// PermissionMode is "plan" for a light turn; empty is "auto", the full
	// harness.
	PermissionMode string
}

// Args returns the argv (without the binary) for the turn.
func (c Chat) Args() []string {
	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json", "--verbose",
		"--permission-mode", cmpOr(c.PermissionMode, "auto"),
		"--permission-prompts", "host",
		// Without it the CLI answers host prompts itself (a denial) rather
		// than sending can_use_tool control requests on stdout; recorded in
		// internal/chat/testdata/README.md.
		"--permission-prompt-tool", "stdio",
		"--append-system-prompt", c.Briefing,
	}
	if c.Resume != "" {
		args = append(args, "--resume", c.Resume)
	} else if c.SessionID != "" {
		args = append(args, "--session-id", c.SessionID)
	}
	args = append(args, budgetArgs(c.MaxBudgetUSD, ChatRunawayUSD)...)
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	for _, d := range c.AddDirs {
		args = append(args, "--add-dir", d)
	}
	return args
}

func cmpOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// InterruptControl asks the CLI to stop its turn at once. It answers with a
// control_response and then its result line (subtype error_during_execution),
// which still carries the turn's total_cost_usd; recorded in
// internal/chat/testdata/light_exit_plan.jsonl.
func InterruptControl(requestID string) []byte {
	out, _ := json.Marshal(map[string]any{
		"type": "control_request", "request_id": requestID,
		"request": map[string]any{"subtype": "interrupt"},
	})
	return append(out, '\n')
}

// ModelRejected reports whether a failed call's text says the CLI rejected
// the --model it was given. Claude Code answers an unknown or unavailable
// model with an api_error result: "There's an issue with the selected model
// (<name>). It may not exist or you may not have access to it."
func ModelRejected(text string) bool {
	t := strings.ToLower(text)
	return strings.Contains(t, "issue with the selected model") ||
		strings.Contains(t, "may not exist or you may not have access") ||
		strings.Contains(t, "unrecognized_model") || strings.Contains(t, "model_not_found")
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
// "On screen" note, then for each image in order an "Image N: <name>" text
// block and its image block, then the text, as separate blocks. It ends with
// a newline. An empty text (a handoff's continuation, or a message of images
// alone) leaves the text block out.
func ChatUserMessage(note string, images []StagedImage, text string) ([]byte, error) {
	content := []map[string]any{}
	if strings.TrimSpace(note) != "" {
		content = append(content, map[string]any{"type": "text", "text": note})
	}
	for i, img := range images {
		data, err := os.ReadFile(img.Path)
		if err != nil {
			return nil, fmt.Errorf("read image %d: %w", i+1, err)
		}
		content = append(content,
			map[string]any{"type": "text", "text": ImageLabel(i+1, img.Name)},
			map[string]any{"type": "image", "source": map[string]any{
				"type": "base64", "media_type": img.MediaType, "data": base64.StdEncoding.EncodeToString(data),
			}},
		)
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
	// Description is the CLI's one-line description of the step, when it
	// gives one.
	Description string
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
			Description    string          `json:"description"`
			ReasonType     string          `json:"decision_reason_type"`
			RuleSource     string          `json:"decision_reason_rule_source"`
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
	if reason == "" && m.Request.ReasonType == "rule" {
		reason = "A permission rule asks before this step"
		if m.Request.RuleSource != "" {
			reason += " (" + m.Request.RuleSource + ")"
		}
	}
	return ControlRequest{
		RequestID: m.RequestID, Subtype: m.Request.Subtype, ToolName: m.Request.ToolName,
		Input: m.Request.Input, Reason: reason, ToolID: m.Request.ToolUseID,
		Description: m.Request.Description,
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

// AllowControl is the control_response that lets a can_use_tool request run
// with its input unchanged.
func AllowControl(requestID string, input json.RawMessage) []byte {
	in := json.RawMessage(`{}`)
	if len(input) > 0 && input[0] == '{' && json.Valid(input) {
		in = input
	}
	return controlResponse(map[string]any{
		"subtype": "success", "request_id": requestID,
		"response": map[string]any{"behavior": "allow", "updatedInput": in},
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

// ChatMinVersion is the Claude Code release Chat's host permission prompts
// were recorded from (internal/chat/testdata). Older releases may lack
// --permission-prompts host.
const ChatMinVersion = "2.1.318"

// ChatSupport says whether this computer's Claude Code can serve Chat.
type ChatSupport struct {
	OK      bool
	Version string
	// Reason says why not, naming the Claude Code version Chat needs.
	Reason string
}

// CheckChat reports whether the claude at path supports host permission
// prompts (--permission-prompts host), which Chat needs so a step the
// classifier won't allow alone can be asked on the phone.
func CheckChat(ctx context.Context, path string) ChatSupport {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ver, verr := Version(ctx, path)
	if verr != nil {
		return ChatSupport{Reason: "Claude Code isn't installed on this computer. Chat needs Claude Code " + ChatMinVersion + " or later."}
	}
	cmd := exec.CommandContext(ctx, path, "--help")
	cmd.Env = ChildEnv()
	out, _ := cmd.Output()
	return ChatSupportFromHelp(ver, string(out))
}

// ChatSupportFromHelp decides support from `claude --help` output.
func ChatSupportFromHelp(version, help string) ChatSupport {
	i := strings.Index(help, "--permission-prompts")
	if i >= 0 && strings.Contains(help[i:min(len(help), i+400)], `"host"`) {
		return ChatSupport{OK: true, Version: version}
	}
	have := "an older Claude Code"
	if version != "" {
		have = "Claude Code " + version
	}
	return ChatSupport{Version: version, Reason: "Chat needs Claude Code " + ChatMinVersion +
		" or later, which can ask the phone before a step (--permission-prompts host). This computer has " + have + "; update it with `claude update`."}
}
