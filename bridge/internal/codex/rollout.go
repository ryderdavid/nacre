package codex

// Rollout files are Codex's on-disk record of a session, the counterpart of
// Claude Code's transcripts (see claude/transcript.go). Codex appends one to
// ~/.codex/sessions/<yyyy>/<mm>/<dd>/rollout-<local time>-<thread id>.jsonl for
// every session on the machine — the TUI, the desktop app and `codex exec`
// alike — so tailing one is how a Codex session nacred did not start becomes
// a monitorable run. The day directories are the session's local start date;
// a session keeps appending to that file however long it runs.
//
// Format notes, from real files written by Codex CLI 0.159 (NAC-j5z.11):
//
//   - Every line is {timestamp, ordinal, type, payload}. The first is always
//     session_meta: id (the thread id), timestamp, cwd, originator ("codex-tui",
//     "Codex Desktop", "codex_exec"), cli_version, source ("vscode", "exec", or
//     {"subagent": …} for a helper thread such as an approval reviewer, which
//     also carries parent_thread_id), model_provider and the whole system
//     prompt (base_instructions, tens of KB).
//   - There is no pid, no process start time and no session registry: nothing
//     on disk says whether the owning process is still alive. The only signals
//     are the file's mtime and the turn lifecycle below.
//   - event_msg payloads carry the lifecycle: task_started opens a turn,
//     task_complete or turn_aborted closes it. `codex exec` runs exactly one
//     turn and exits, so for it a closed turn means the process is gone.
//   - event_msg item_completed carries each finished item in a structured,
//     self-contained form: UserMessage, AgentMessage, Reasoning (summaries are
//     usually empty: the raw reasoning is encrypted), CommandExecution (argv,
//     exit_code, aggregated_output, status), FileChange (changes keyed by path),
//     McpToolCall, WebSearch, Extension (kind "web.search"), ImageView and
//     ContextCompaction. These are what become run events.
//   - response_item lines are the raw model conversation (messages, reasoning,
//     and tool calls that are JavaScript snippets driving Codex's own tools);
//     they duplicate item_completed in a less readable form and are ignored,
//     as are token_count / token_usage_record (token counts and rate limits;
//     no cost is ever written), world_state, compacted and thread settings.
//   - turn_context precedes each turn with its cwd and model.
//
// ~/.codex/session_index.jsonl is a separate append-only list of
// {id, thread_name, updated_at}: the title Codex gave a thread. It only covers
// threads that got a title — `codex exec` sessions are absent from it — so it
// is a source of names, not of sessions.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// Rollout entry types that mark the turn lifecycle. ParseRolloutEntry reports
// them as TranscriptParsed.Type so a caller's Observe hook can follow it.
const (
	RolloutTurnStarted  = "task_started"
	RolloutTurnComplete = "task_complete"
	RolloutTurnAborted  = "turn_aborted"
)

// OriginatorExec is session_meta.originator for a `codex exec` session.
const OriginatorExec = "codex_exec"

// RolloutMeta is a rollout's session_meta: who wrote the file, and where.
type RolloutMeta struct {
	ID         string // thread id
	Started    time.Time
	CWD        string
	Originator string
	Version    string // cli_version
	// Source is session_meta.source as a string ("vscode", "exec"), or
	// "subagent" for a helper thread.
	Source string
	// Subagent reports a helper thread Codex spawned for another one (an
	// approval reviewer, a delegated agent). It is not a session anyone
	// started, and nacred never adopts it.
	Subagent bool
}

type rolloutLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type rolloutMetaPayload struct {
	ID             string          `json:"id"`
	Timestamp      string          `json:"timestamp"`
	CWD            string          `json:"cwd"`
	Originator     string          `json:"originator"`
	CLIVersion     string          `json:"cli_version"`
	Source         json.RawMessage `json:"source"`
	ParentThreadID string          `json:"parent_thread_id"`
}

func (p rolloutMetaPayload) meta() RolloutMeta {
	m := RolloutMeta{ID: p.ID, CWD: p.CWD, Originator: p.Originator, Version: p.CLIVersion}
	if t, err := time.Parse(time.RFC3339, p.Timestamp); err == nil {
		m.Started = t.UTC()
	}
	var src string
	switch {
	case json.Unmarshal(p.Source, &src) == nil:
		m.Source = src
	case bytes.Contains(p.Source, []byte(`"subagent"`)):
		m.Source, m.Subagent = "subagent", true
	}
	if p.ParentThreadID != "" {
		m.Subagent = true
	}
	return m
}

// ReadRolloutMeta reads a rollout's first line, its session_meta.
func ReadRolloutMeta(path string) (RolloutMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return RolloutMeta{}, err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, 64<<10)
	line, err := readLine(br, claude.MaxTranscriptLine)
	if err != nil {
		return RolloutMeta{}, fmt.Errorf("rollout %s: %w", path, err)
	}
	var l rolloutLine
	if err := json.Unmarshal(line, &l); err != nil {
		return RolloutMeta{}, fmt.Errorf("rollout %s: %w", path, err)
	}
	if l.Type != "session_meta" {
		return RolloutMeta{}, fmt.Errorf("rollout %s: first entry is %q, not session_meta", path, l.Type)
	}
	var p rolloutMetaPayload
	if err := json.Unmarshal(l.Payload, &p); err != nil {
		return RolloutMeta{}, fmt.Errorf("rollout %s: %w", path, err)
	}
	return p.meta(), nil
}

// readLine reads one newline-terminated line of at most max bytes.
func readLine(br *bufio.Reader, max int) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := br.ReadSlice('\n')
		buf = append(buf, chunk...)
		if len(buf) > max {
			return nil, fmt.Errorf("line longer than %d bytes", max)
		}
		switch err {
		case nil:
			return buf, nil
		case bufio.ErrBufferFull:
			continue
		default:
			return nil, err
		}
	}
}

// ReadSessionIndex reads ~/.codex/session_index.jsonl into thread id → name.
// A thread renamed later appears again further down, so the last line wins. A
// missing index yields an empty map and no error.
func ReadSessionIndex(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range bytes.Split(data, []byte("\n")) {
		var e struct {
			ID         string `json:"id"`
			ThreadName string `json:"thread_name"`
		}
		if json.Unmarshal(line, &e) == nil && e.ID != "" && e.ThreadName != "" {
			out[e.ID] = e.ThreadName
		}
	}
	return out, nil
}

// rolloutPayload is the union of the event_msg and turn_context fields the
// adapter reads.
type rolloutPayload struct {
	Type    string          `json:"type"`
	Item    json.RawMessage `json:"item"`
	Reason  string          `json:"reason"`  // turn_aborted
	Message string          `json:"message"` // error
	CWD     string          `json:"cwd"`     // turn_context
	Model   string          `json:"model"`   // turn_context
}

// rolloutItem is an item_completed item. Only the fields the adapter uses are
// declared.
type rolloutItem struct {
	Type    string          `json:"type"`
	ID      string          `json:"id"`
	Kind    string          `json:"kind"` // Extension
	Content json.RawMessage `json:"content"`
	Summary json.RawMessage `json:"summary_text"`
	Status  string          `json:"status"`
	// CommandExecution
	Command          json.RawMessage `json:"command"`
	AggregatedOutput string          `json:"aggregated_output"`
	ExitCode         *int            `json:"exit_code"`
	// FileChange
	Changes map[string]struct {
		Type string `json:"type"`
	} `json:"changes"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	// McpToolCall
	Server    string          `json:"server"`
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
	Result    json.RawMessage `json:"result"`
	Error     json.RawMessage `json:"error"`
	// WebSearch, Extension web.search
	Query  string `json:"query"`
	Action struct {
		Query   *string  `json:"query"`
		Queries []string `json:"queries"`
		URL     string   `json:"url"`
	} `json:"action"`
	Results []struct {
		Title string `json:"title"`
		URL   string `json:"url"`
	} `json:"results"`
	// ImageView
	Path string `json:"path"`
}

// ParseRolloutEntry parses one line of a Codex rollout into the shape a
// claude.TranscriptTail folds (pass it as TranscriptOptions.Parse). Tool calls
// are named after their Claude Code counterparts, as the `codex exec --json`
// parser does, so a client shows both alike and the session watcher finds bd
// commands in Bash calls whichever agent ran them.
func ParseRolloutEntry(line []byte) (claude.TranscriptParsed, error) {
	var p claude.TranscriptParsed
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return p, nil
	}
	var l rolloutLine
	if err := json.Unmarshal(line, &l); err != nil {
		return p, fmt.Errorf("rollout: %w", err)
	}
	p.Type, p.TS = l.Type, l.Timestamp
	switch l.Type {
	case "session_meta":
		var m rolloutMetaPayload
		if json.Unmarshal(l.Payload, &m) == nil {
			p.SessionID, p.CWD, p.Version = m.ID, m.CWD, m.CLIVersion
		}
		return p, nil
	case "turn_context":
		var c rolloutPayload
		if json.Unmarshal(l.Payload, &c) == nil {
			p.CWD, p.Model = c.CWD, c.Model
		}
		return p, nil
	case "event_msg":
	default:
		p.Ignored = true
		return p, nil
	}

	var e rolloutPayload
	if err := json.Unmarshal(l.Payload, &e); err != nil {
		return p, fmt.Errorf("rollout: %w", err)
	}
	switch e.Type {
	case RolloutTurnStarted, RolloutTurnComplete:
		p.Type = e.Type
	case RolloutTurnAborted:
		p.Type = e.Type
		text := "Turn interrupted"
		if e.Reason != "" && e.Reason != "interrupted" {
			text += " (" + e.Reason + ")"
		}
		p.Events = []api.RunEvent{{Kind: api.KindSystem, Text: text}}
	case "error":
		if e.Message != "" {
			p.Events = []api.RunEvent{{Kind: api.KindSystem, Text: "Error: " + claude.Truncate(e.Message, claude.MaxTranscriptText), IsError: api.Bool(true)}}
		}
	case "item_completed":
		var it rolloutItem
		if len(e.Item) == 0 || json.Unmarshal(e.Item, &it) != nil {
			p.Ignored = true
			return p, nil
		}
		p.Events, p.Assistant = rolloutItemEvents(it)
		if len(p.Events) == 0 && !p.Assistant {
			p.Ignored = true
		}
	default:
		p.Ignored = true
	}
	return p, nil
}

// rolloutItemEvents translates one completed item. assistant reports that the
// model produced it (anything but the human's own message), which is what the
// session watcher's bd-command scan keys on.
func rolloutItemEvents(it rolloutItem) (evs []api.RunEvent, assistant bool) {
	switch it.Type {
	case "UserMessage":
		if text := strings.TrimSpace(contentText(it.Content)); text != "" {
			evs = append(evs, api.RunEvent{Kind: api.KindUser, Text: claude.Truncate(text, claude.MaxTranscriptText)})
		}
		return evs, false
	case "AgentMessage":
		if text := contentText(it.Content); strings.TrimSpace(text) != "" {
			evs = append(evs, api.RunEvent{Kind: api.KindText, Text: claude.Truncate(text, claude.MaxTranscriptText)})
		}
		return evs, true
	case "Reasoning":
		if text := strings.TrimSpace(contentText(it.Summary)); text != "" {
			evs = append(evs, api.RunEvent{Kind: api.KindThinking, Text: claude.Truncate(text, claude.MaxTranscriptText)})
		}
		return evs, true
	case "ContextCompaction":
		return []api.RunEvent{{Kind: api.KindSystem, Text: "Context compacted"}}, false
	}

	tool, input, ok := rolloutToolUse(it)
	if !ok {
		return nil, false
	}
	raw, _ := json.Marshal(input)
	if it.Type == "McpToolCall" && len(bytes.TrimSpace(it.Arguments)) > 0 && it.Arguments[0] == '{' {
		raw = it.Arguments
	}
	text, isErr := rolloutToolResult(it)
	return []api.RunEvent{
		{Kind: api.KindToolUse, Tool: tool, ToolID: it.ID, Input: raw, Text: claude.ToolSummary(tool, raw)},
		{Kind: api.KindToolResult, ToolID: it.ID, Text: claude.Truncate(text, claude.MaxToolResult), IsError: api.Bool(isErr)},
	}, true
}

func rolloutToolUse(it rolloutItem) (tool string, input any, ok bool) {
	switch it.Type {
	case "CommandExecution":
		return "Bash", map[string]string{"command": commandLine(it.Command)}, true
	case "FileChange":
		return "Edit", map[string]string{"file_path": strings.Join(changedPaths(it), ", ")}, true
	case "McpToolCall":
		return "mcp__" + it.Server + "__" + it.Tool, map[string]any{}, true
	case "WebSearch":
		return "WebSearch", map[string]string{"query": searchQuery(it)}, true
	case "Extension":
		if it.Kind == "web.search" {
			return "WebSearch", map[string]string{"query": searchQuery(it)}, true
		}
	case "ImageView":
		return "ViewImage", map[string]string{"file_path": strings.TrimPrefix(it.Path, "file://")}, true
	}
	return "", nil, false
}

func rolloutToolResult(it rolloutItem) (string, bool) {
	failed := it.Status == "failed"
	switch it.Type {
	case "CommandExecution":
		text := it.AggregatedOutput
		if it.ExitCode != nil && *it.ExitCode != 0 {
			failed = true
			text = strings.TrimRight(text, "\n") + fmt.Sprintf("\n(exit code %d)", *it.ExitCode)
		}
		return strings.TrimLeft(text, "\n"), failed
	case "FileChange":
		if failed && strings.TrimSpace(it.Stderr) != "" {
			return it.Stderr, true
		}
		lines := make([]string, 0, len(it.Changes))
		for _, path := range changedPaths(it) {
			lines = append(lines, it.Changes[path].Type+" "+path)
		}
		return strings.Join(lines, "\n"), failed
	case "McpToolCall":
		if msg := errorMessage(it.Error); msg != "" {
			return msg, true
		}
		var res struct {
			Content json.RawMessage `json:"content"`
			IsError bool            `json:"isError"`
		}
		if json.Unmarshal(it.Result, &res) == nil && len(res.Content) > 0 {
			return contentText(res.Content), failed || res.IsError
		}
		return string(it.Result), failed
	case "WebSearch", "Extension":
		lines := make([]string, 0, len(it.Results))
		for _, r := range it.Results {
			lines = append(lines, strings.TrimSpace(r.Title+" "+r.URL))
		}
		return strings.Join(lines, "\n"), failed
	}
	return "", failed
}

// commandLine renders CommandExecution.command, an argv such as
// ["/bin/zsh", "-lc", "bd show X"], as the command the agent typed.
func commandLine(raw json.RawMessage) string {
	var argv []string
	if json.Unmarshal(raw, &argv) != nil {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return unwrapShell(s)
		}
		return ""
	}
	if n := len(argv); n == 3 && (argv[1] == "-lc" || argv[1] == "-c") {
		return argv[2]
	}
	return strings.Join(argv, " ")
}

func changedPaths(it rolloutItem) []string {
	paths := make([]string, 0, len(it.Changes))
	for p := range it.Changes {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

func searchQuery(it rolloutItem) string {
	switch {
	case it.Query != "":
		return it.Query
	case it.Action.Query != nil && *it.Action.Query != "":
		return *it.Action.Query
	case len(it.Action.Queries) > 0:
		return strings.Join(it.Action.Queries, "; ")
	}
	return it.Action.URL
}

// contentText flattens the content shapes rollout items use: a list of
// {type, text} blocks (any case of "text"), a list of strings, or a string.
func contentText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		var str string
		if json.Unmarshal(item, &str) == nil {
			parts = append(parts, str)
			continue
		}
		var b struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(item, &b) == nil && b.Text != "" {
			parts = append(parts, b.Text)
		} else if b.Type == "image" || b.Type == "Image" {
			parts = append(parts, "[image]")
		}
	}
	return strings.Join(parts, "\n")
}

// errorMessage returns an item error's message: a string, or {message}.
func errorMessage(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var e struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &e) == nil {
		return e.Message
	}
	return ""
}
