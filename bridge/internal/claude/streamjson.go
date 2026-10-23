// Package claude knows how to invoke the Claude Code CLI headless and how to
// translate its `--output-format stream-json --verbose` output into RunEvents.
package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

const (
	// MaxToolResult is the maximum length (in runes) of a tool_result event's text.
	MaxToolResult = 4000
	maxSummary    = 200
)

// Init is the session information from a `system/init` message.
type Init struct {
	SessionID string
	Model     string
	CWD       string
}

// Result is the end-of-turn summary from a `result` message.
type Result struct {
	Subtype    string
	IsError    bool
	Text       string
	CostUSD    float64
	DurationMS int64
	NumTurns   int
	SessionID  string
}

// Parsed is what one stream-json line contributes to a run. Events have no
// Seq/TS yet; the run manager assigns those.
type Parsed struct {
	Events []api.RunEvent
	Init   *Init
	Result *Result
}

// rawMessage covers every stream-json message type we understand.
type rawMessage struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	SessionID string `json:"session_id"`
	Model     string `json:"model"`
	CWD       string `json:"cwd"`
	// Message is an object with "content" on assistant/user lines, but a plain
	// string on system notices such as permission_denied.
	Message json.RawMessage `json:"message"`
	// result
	IsError      bool     `json:"is_error"`
	Result       string   `json:"result"`
	TotalCostUSD float64  `json:"total_cost_usd"`
	DurationMS   int64    `json:"duration_ms"`
	NumTurns     int      `json:"num_turns"`
	Errors       []string `json:"errors"`
	// system/api_retry etc.
	Attempt int    `json:"attempt"`
	Error   string `json:"error"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// ParseLine parses one line of stream-json output. Unknown message types and
// blank lines yield an empty Parsed and no error; malformed JSON is an error.
func ParseLine(line []byte) (Parsed, error) {
	var p Parsed
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return p, nil
	}
	var m rawMessage
	if err := json.Unmarshal(line, &m); err != nil {
		return p, fmt.Errorf("stream-json: %w", err)
	}
	switch m.Type {
	case "system":
		switch m.Subtype {
		case "init":
			p.Init = &Init{SessionID: m.SessionID, Model: m.Model, CWD: m.CWD}
			text := "Session started"
			if m.Model != "" {
				text += " · model " + m.Model
			}
			p.Events = append(p.Events, api.RunEvent{Kind: api.KindSystem, Text: text})
		case "permission_denied":
			text := "Not allowed in this permission mode"
			if msg := messageText(m); msg != "" {
				text += ": " + msg
			}
			p.Events = append(p.Events, api.RunEvent{Kind: api.KindSystem, Text: text})
		case "compact_boundary":
			p.Events = append(p.Events, api.RunEvent{Kind: api.KindSystem, Text: "Context compacted"})
		case "api_retry":
			text := "API retry"
			if m.Attempt > 0 {
				text += fmt.Sprintf(" (attempt %d)", m.Attempt)
			}
			if m.Error != "" {
				text += ": " + m.Error
			}
			p.Events = append(p.Events, api.RunEvent{Kind: api.KindSystem, Text: text})
		}
	case "assistant":
		for _, b := range blocks(m) {
			switch b.Type {
			case "text":
				if strings.TrimSpace(b.Text) != "" {
					p.Events = append(p.Events, api.RunEvent{Kind: api.KindText, Text: b.Text})
				}
			case "thinking":
				if strings.TrimSpace(b.Thinking) != "" {
					p.Events = append(p.Events, api.RunEvent{Kind: api.KindThinking, Text: b.Thinking})
				}
			case "tool_use":
				input := b.Input
				if len(bytes.TrimSpace(input)) == 0 || bytes.Equal(bytes.TrimSpace(input), []byte("null")) {
					input = json.RawMessage("{}")
				}
				p.Events = append(p.Events, api.RunEvent{
					Kind: api.KindToolUse, Tool: b.Name, ToolID: b.ID,
					Input: input, Text: ToolSummary(b.Name, input),
				})
			}
		}
	case "user":
		for _, b := range blocks(m) {
			if b.Type != "tool_result" {
				continue
			}
			p.Events = append(p.Events, api.RunEvent{
				Kind: api.KindToolResult, ToolID: b.ToolUseID,
				Text: Truncate(toolResultText(b.Content), MaxToolResult), IsError: api.Bool(b.IsError),
			})
		}
	case "result":
		text := m.Result
		if text == "" && m.IsError {
			if len(m.Errors) > 0 {
				text = strings.Join(m.Errors, "; ")
			} else {
				text = m.Subtype
			}
		}
		p.Result = &Result{
			Subtype: m.Subtype, IsError: m.IsError, Text: text, CostUSD: m.TotalCostUSD,
			DurationMS: m.DurationMS, NumTurns: m.NumTurns, SessionID: m.SessionID,
		}
		p.Events = append(p.Events, api.RunEvent{Kind: api.KindResult, Text: text, IsError: api.Bool(m.IsError)})
	}
	return p, nil
}

// blocks decodes message.content as a list of blocks. A plain-string content
// (e.g. an echoed prompt) yields nothing.
func blocks(m rawMessage) []contentBlock {
	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	if len(m.Message) == 0 || m.Message[0] != '{' || json.Unmarshal(m.Message, &msg) != nil {
		return nil
	}
	if len(msg.Content) == 0 || msg.Content[0] != '[' {
		return nil
	}
	var bs []contentBlock
	if err := json.Unmarshal(msg.Content, &bs); err != nil {
		return nil
	}
	return bs
}

// messageText returns the "message" field when it is a plain string.
func messageText(m rawMessage) string {
	var s string
	if json.Unmarshal(m.Message, &s) != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

// toolResultText flattens tool_result content: a string, or an array of
// blocks whose text parts are joined (images become "[image]").
func toolResultText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	var s string
	if raw[0] == '"' && json.Unmarshal(raw, &s) == nil {
		return s
	}
	var bs []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &bs) == nil {
		parts := make([]string, 0, len(bs))
		for _, b := range bs {
			switch b.Type {
			case "text":
				parts = append(parts, b.Text)
			case "image":
				parts = append(parts, "[image]")
			}
		}
		return strings.Join(parts, "\n")
	}
	return string(raw)
}

// ToolSummary returns a one-line human summary of a tool call's input.
func ToolSummary(tool string, input json.RawMessage) string {
	var fields map[string]any
	_ = json.Unmarshal(input, &fields)
	get := func(keys ...string) string {
		for _, k := range keys {
			if s, ok := fields[k].(string); ok && s != "" {
				return s
			}
		}
		return ""
	}
	var s string
	switch tool {
	case "Bash":
		s = get("command")
	case "Read", "Edit", "Write", "MultiEdit":
		s = get("file_path")
	case "NotebookEdit", "NotebookRead":
		s = get("notebook_path", "file_path")
	case "Grep", "Glob":
		s = get("pattern")
		if p := get("path"); s != "" && p != "" {
			s += " in " + p
		}
	case "Task", "Agent":
		s = get("description", "prompt")
	case "WebFetch":
		s = get("url")
	case "WebSearch":
		s = get("query")
	case "TodoWrite":
		if todos, ok := fields["todos"].([]any); ok {
			s = fmt.Sprintf("%d todos", len(todos))
		}
	}
	if s == "" {
		s = firstStringField(input)
	}
	return OneLine(s, maxSummary)
}

// firstStringField returns the first top-level string value of a JSON object,
// in document order.
func firstStringField(input json.RawMessage) string {
	dec := json.NewDecoder(bytes.NewReader(input))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return ""
	}
	for dec.More() {
		if _, err := dec.Token(); err != nil { // key
			return ""
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return ""
		}
		var s string
		if len(v) > 0 && v[0] == '"' && json.Unmarshal(v, &s) == nil && s != "" {
			return s
		}
	}
	return ""
}

// OneLine collapses whitespace runs (including newlines) to single spaces and
// truncates to n runes.
func OneLine(s string, n int) string {
	return Truncate(strings.Join(strings.Fields(s), " "), n)
}

// Truncate shortens s to at most n runes, marking the cut with "…".
func Truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}
