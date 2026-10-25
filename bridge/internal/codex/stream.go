package codex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// Parser turns `codex exec --json` lines into the claude.Parsed shape the
// run manager already folds into a run: thread.started becomes Init,
// turn.completed / turn.failed become Result, and items become text,
// thinking, tool_use and tool_result events.
//
// Tool calls are named after their Claude Code counterparts (Bash, Edit,
// WebSearch, TodoWrite, mcp__server__tool) so clients that know how to show
// those show Codex's the same way.
//
// A Parser is stateful (the turn's final message is its result text) and
// serves one turn; it is not safe for concurrent use.
type Parser struct {
	last    string          // newest agent message
	started map[string]bool // item ids whose tool_use was already emitted
}

type rawEvent struct {
	Type     string          `json:"type"`
	ThreadID string          `json:"thread_id"`
	Item     json.RawMessage `json:"item"`
	Message  string          `json:"message"` // top-level "error"
	Error    *struct {
		Message string `json:"message"`
	} `json:"error"` // turn.failed
}

type rawItem struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Text string `json:"text"`
	// command_execution
	Command          string `json:"command"`
	AggregatedOutput string `json:"aggregated_output"`
	ExitCode         *int   `json:"exit_code"`
	Status           string `json:"status"`
	// file_change
	Changes []struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
	} `json:"changes"`
	// mcp_tool_call
	Server    string          `json:"server"`
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
	Result    json.RawMessage `json:"result"`
	ItemError *struct {
		Message string `json:"message"`
	} `json:"error"`
	// web_search
	Query string `json:"query"`
	// todo_list
	Items []struct {
		Text      string `json:"text"`
		Completed bool   `json:"completed"`
	} `json:"items"`
	// error
	Message string `json:"message"`
}

// ParseLine parses one line of Codex's JSONL output. Unknown event types and
// blank lines yield an empty Parsed and no error; malformed JSON is an error.
func (p *Parser) ParseLine(line []byte) (claude.Parsed, error) {
	var out claude.Parsed
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return out, nil
	}
	var e rawEvent
	if err := json.Unmarshal(line, &e); err != nil {
		return out, fmt.Errorf("codex json: %w", err)
	}
	switch e.Type {
	case "thread.started":
		out.Init = &claude.Init{SessionID: e.ThreadID}
		out.Events = append(out.Events, api.RunEvent{Kind: api.KindSystem, Text: "Codex session started"})
	case "turn.completed":
		out.Result = &claude.Result{Subtype: "success", Text: p.last, NumTurns: 1}
		out.Events = append(out.Events, api.RunEvent{Kind: api.KindResult, Text: p.last, IsError: api.Bool(false)})
	case "turn.failed":
		msg := "turn failed"
		if e.Error != nil && e.Error.Message != "" {
			msg = e.Error.Message
		}
		out.Result = &claude.Result{Subtype: "error", IsError: true, Text: msg, NumTurns: 1}
		out.Events = append(out.Events, api.RunEvent{Kind: api.KindResult, Text: msg, IsError: api.Bool(true)})
	case "error":
		if e.Message != "" {
			out.Events = append(out.Events, api.RunEvent{Kind: api.KindSystem, Text: "Error: " + e.Message})
		}
	case "item.started", "item.completed":
		var it rawItem
		if len(e.Item) == 0 || json.Unmarshal(e.Item, &it) != nil {
			return out, nil
		}
		out.Events = p.item(it, e.Type == "item.completed")
	}
	return out, nil
}

// item maps one item event. Tool-like items emit tool_use when they start
// (or on completion if no start was seen) and tool_result when they complete.
func (p *Parser) item(it rawItem, done bool) []api.RunEvent {
	switch it.Type {
	case "agent_message":
		if !done || strings.TrimSpace(it.Text) == "" {
			return nil
		}
		p.last = strings.TrimSpace(it.Text)
		return []api.RunEvent{{Kind: api.KindText, Text: it.Text}}
	case "reasoning":
		if !done || strings.TrimSpace(it.Text) == "" {
			return nil
		}
		return []api.RunEvent{{Kind: api.KindThinking, Text: it.Text}}
	case "error":
		if !done || it.Message == "" {
			return nil
		}
		return []api.RunEvent{{Kind: api.KindSystem, Text: it.Message}}
	}
	use, ok := toolUse(it)
	if !ok {
		return nil
	}
	var evs []api.RunEvent
	if !p.started[it.ID] {
		if p.started == nil {
			p.started = map[string]bool{}
		}
		p.started[it.ID] = true
		evs = append(evs, use)
	}
	if done {
		text, isErr := toolResult(it)
		evs = append(evs, api.RunEvent{
			Kind: api.KindToolResult, ToolID: it.ID,
			Text: claude.Truncate(text, claude.MaxToolResult), IsError: api.Bool(isErr),
		})
		delete(p.started, it.ID)
	}
	return evs
}

func toolUse(it rawItem) (api.RunEvent, bool) {
	ev := api.RunEvent{Kind: api.KindToolUse, ToolID: it.ID}
	var input any
	switch it.Type {
	case "command_execution":
		ev.Tool = "Bash"
		input = map[string]string{"command": unwrapShell(it.Command)}
	case "file_change":
		ev.Tool = "Edit"
		paths := make([]string, 0, len(it.Changes))
		for _, c := range it.Changes {
			paths = append(paths, c.Path)
		}
		input = map[string]any{"file_path": strings.Join(paths, ", "), "changes": it.Changes}
	case "mcp_tool_call":
		ev.Tool = "mcp__" + it.Server + "__" + it.Tool
		if len(bytes.TrimSpace(it.Arguments)) > 0 && it.Arguments[0] == '{' {
			ev.Input = it.Arguments
		}
	case "web_search":
		ev.Tool = "WebSearch"
		input = map[string]string{"query": it.Query}
	case "todo_list":
		ev.Tool = "TodoWrite"
		todos := make([]map[string]string, 0, len(it.Items))
		for _, t := range it.Items {
			st := "pending"
			if t.Completed {
				st = "completed"
			}
			todos = append(todos, map[string]string{"content": t.Text, "status": st})
		}
		input = map[string]any{"todos": todos}
	default:
		return ev, false
	}
	if ev.Input == nil {
		if input == nil {
			input = map[string]any{}
		}
		ev.Input, _ = json.Marshal(input)
	}
	ev.Text = claude.ToolSummary(ev.Tool, ev.Input)
	return ev, true
}

func toolResult(it rawItem) (string, bool) {
	switch it.Type {
	case "command_execution":
		failed := it.Status == "failed" || (it.ExitCode != nil && *it.ExitCode != 0)
		text := it.AggregatedOutput
		if it.ExitCode != nil && *it.ExitCode != 0 {
			text = strings.TrimRight(text, "\n") + fmt.Sprintf("\n(exit code %d)", *it.ExitCode)
		}
		return strings.TrimLeft(text, "\n"), failed
	case "file_change":
		lines := make([]string, 0, len(it.Changes))
		for _, c := range it.Changes {
			lines = append(lines, c.Kind+" "+c.Path)
		}
		return strings.Join(lines, "\n"), it.Status == "failed"
	case "mcp_tool_call":
		if it.ItemError != nil && it.ItemError.Message != "" {
			return it.ItemError.Message, true
		}
		return string(it.Result), it.Status == "failed"
	case "todo_list":
		done := 0
		for _, t := range it.Items {
			if t.Completed {
				done++
			}
		}
		return fmt.Sprintf("%d of %d done", done, len(it.Items)), false
	}
	return "", it.Status == "failed"
}

// unwrapShell drops the login-shell wrapper Codex reports commands with
// (`/bin/zsh -lc 'cat a.txt'` → `cat a.txt`), so a command reads as typed.
func unwrapShell(cmd string) string {
	for _, sh := range []string{"/bin/zsh -lc ", "/bin/bash -lc ", "/bin/sh -c ", "bash -lc ", "zsh -lc "} {
		rest, ok := strings.CutPrefix(cmd, sh)
		if !ok {
			continue
		}
		if n := len(rest); n >= 2 && rest[0] == '\'' && rest[n-1] == '\'' && !strings.Contains(rest[1:n-1], "'") {
			return rest[1 : n-1]
		}
		if n := len(rest); n >= 2 && rest[0] == '"' && rest[n-1] == '"' {
			var s string
			if json.Unmarshal([]byte(rest), &s) == nil {
				return s
			}
			return rest[1 : n-1]
		}
		return rest
	}
	return cmd
}
