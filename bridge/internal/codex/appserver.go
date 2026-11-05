package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

// AppServer is a JSON-RPC client of `codex app-server` over stdio, used by
// Chat: one process per turn, started with [StartAppServer].
//
// Responses to nacred's calls are matched by id. Notifications and requests
// from the server (approvals) go to the handlers given at start; a server
// request must be answered with [AppServer.Respond].
type AppServer struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser

	writeMu sync.Mutex
	mu      sync.Mutex
	nextID  int64
	calls   map[int64]chan rpcReply
	closed  bool
	done    chan struct{}
	// tail is the end of stderr, for errors.
	tail []string
	// exitErr is set once the process has gone.
	exitErr error
}

// AppServerOptions configure [StartAppServer].
type AppServerOptions struct {
	// Path is the codex binary.
	Path string
	Dir  string
	Env  []string
	// OnNotification gets every notification (method, params).
	OnNotification func(method string, params json.RawMessage)
	// OnRequest gets every request from the server; it must answer with
	// Respond or RespondError, on any goroutine.
	OnRequest func(id json.RawMessage, method string, params json.RawMessage)
}

// RPCError is an error answer to a call.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return e.Message }

type rpcReply struct {
	result json.RawMessage
	err    error
}

type rpcMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

// ErrAppServerGone is a call made after the app-server exited.
var ErrAppServerGone = errors.New("codex app-server exited")

// StartAppServer starts `codex app-server` and completes the initialize
// handshake. ctx bounds the process: cancelling it kills the process group.
func StartAppServer(ctx context.Context, o AppServerOptions) (*AppServer, error) {
	cmd := exec.CommandContext(ctx, o.Path, "app-server")
	cmd.Dir = o.Dir
	cmd.Env = o.Env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start codex app-server: %w", err)
	}
	a := &AppServer{cmd: cmd, stdin: stdin, calls: map[int64]chan rpcReply{}, done: make(chan struct{})}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		a.readLoop(stdout, o)
	}()
	go func() {
		defer wg.Done()
		br := bufio.NewReaderSize(stderr, 64*1024)
		for {
			line, err := br.ReadString('\n')
			if s := strings.TrimSpace(line); s != "" {
				a.mu.Lock()
				a.tail = append(a.tail, s)
				if len(a.tail) > 10 {
					a.tail = a.tail[1:]
				}
				a.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		werr := cmd.Wait()
		a.mu.Lock()
		a.closed = true
		a.exitErr = werr
		for id, ch := range a.calls {
			ch <- rpcReply{err: a.goneErrLocked()}
			delete(a.calls, id)
		}
		a.mu.Unlock()
		close(a.done)
	}()

	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := a.Call(initCtx, "initialize", map[string]any{
		"clientInfo": map[string]string{"name": "nacred", "title": "Nacre", "version": "1"},
	}); err != nil {
		a.Close()
		return nil, fmt.Errorf("codex app-server didn't start: %w", err)
	}
	if err := a.Notify("initialized", nil); err != nil {
		a.Close()
		return nil, err
	}
	return a, nil
}

func (a *AppServer) goneErrLocked() error {
	suffix := ""
	if a.exitErr != nil {
		suffix += " (" + a.exitErr.Error() + ")"
	}
	if n := len(a.tail); n > 0 {
		suffix += ": " + a.tail[n-1]
	}
	return fmt.Errorf("%w%s", ErrAppServerGone, suffix)
}

func (a *AppServer) readLoop(r io.Reader, o AppServerOptions) {
	br := bufio.NewReaderSize(r, 1024*1024)
	for {
		line, err := br.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			var m rpcMessage
			if json.Unmarshal(line, &m) == nil {
				a.dispatch(m, o)
			}
		}
		if err != nil {
			return
		}
	}
}

func (a *AppServer) dispatch(m rpcMessage, o AppServerOptions) {
	hasID := len(m.ID) > 0 && string(m.ID) != "null"
	switch {
	case m.Method != "" && hasID:
		if o.OnRequest != nil {
			o.OnRequest(m.ID, m.Method, m.Params)
		} else {
			_ = a.RespondError(m.ID, -32601, "nacred doesn't handle "+m.Method)
		}
	case m.Method != "":
		if o.OnNotification != nil {
			o.OnNotification(m.Method, m.Params)
		}
	case hasID:
		var id int64
		if json.Unmarshal(m.ID, &id) != nil {
			return
		}
		a.mu.Lock()
		ch := a.calls[id]
		delete(a.calls, id)
		a.mu.Unlock()
		if ch == nil {
			return
		}
		if m.Error != nil {
			ch <- rpcReply{err: m.Error}
		} else {
			ch <- rpcReply{result: m.Result}
		}
	}
}

func (a *AppServer) send(v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	_, err = a.stdin.Write(append(line, '\n'))
	return err
}

// Call sends a request and waits for its answer.
func (a *AppServer) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	a.mu.Lock()
	if a.closed {
		err := a.goneErrLocked()
		a.mu.Unlock()
		return nil, err
	}
	a.nextID++
	id := a.nextID
	ch := make(chan rpcReply, 1)
	a.calls[id] = ch
	a.mu.Unlock()
	msg := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	if err := a.send(msg); err != nil {
		a.mu.Lock()
		delete(a.calls, id)
		a.mu.Unlock()
		return nil, fmt.Errorf("%s: %w", method, err)
	}
	select {
	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("%s: %w", method, r.err)
		}
		return r.result, nil
	case <-ctx.Done():
		a.mu.Lock()
		delete(a.calls, id)
		a.mu.Unlock()
		return nil, fmt.Errorf("%s: %w", method, ctx.Err())
	}
}

// Notify sends a notification.
func (a *AppServer) Notify(method string, params any) error {
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		msg["params"] = params
	}
	return a.send(msg)
}

// Respond answers a server request.
func (a *AppServer) Respond(id json.RawMessage, result any) error {
	return a.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

// RespondError answers a server request with an error.
func (a *AppServer) RespondError(id json.RawMessage, code int, message string) error {
	return a.send(map[string]any{"jsonrpc": "2.0", "id": id, "error": RPCError{Code: code, Message: message}})
}

// Done is closed once the process has exited.
func (a *AppServer) Done() <-chan struct{} { return a.done }

// Err is why the process exited, once Done is closed.
func (a *AppServer) Err() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.goneErrLocked()
}

// Close ends the process: stdin is closed, and the process group killed if
// it hasn't exited shortly after.
func (a *AppServer) Close() {
	a.writeMu.Lock()
	_ = a.stdin.Close()
	a.writeMu.Unlock()
	select {
	case <-a.done:
	case <-time.After(3 * time.Second):
		if a.cmd.Process != nil {
			_ = syscall.Kill(-a.cmd.Process.Pid, syscall.SIGKILL)
		}
		<-a.done
	}
}

// ProbeAppServer reports whether `codex app-server` starts and answers the
// initialize handshake, for advertising Codex chats.
func ProbeAppServer(ctx context.Context, path string, env []string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	a, err := StartAppServer(ctx, AppServerOptions{Path: path, Env: env})
	if err != nil {
		return err
	}
	a.Close()
	return nil
}

// ChatThread is how a chat's Codex thread runs: in the chat's working
// directory, in the workspace-write sandbox with every board on the host
// writable and network access on, asking before anything beyond it
// (approval policy on-request), with the Nacre briefing as developer
// instructions.
type ChatThread struct {
	Cwd           string
	WritableRoots []string
	Instructions  string
}

// sandboxConfig is the config override that gives workspace-write its
// writable roots and network access.
func (t ChatThread) sandboxConfig() map[string]any {
	roots := append([]string{}, t.WritableRoots...)
	return map[string]any{
		"sandbox_workspace_write": map[string]any{
			"writable_roots": roots,
			"network_access": true,
		},
	}
}

// StartParams are thread/start's params.
func (t ChatThread) StartParams() map[string]any {
	return map[string]any{
		"cwd":                   t.Cwd,
		"sandbox":               "workspace-write",
		"approvalPolicy":        "on-request",
		"config":                t.sandboxConfig(),
		"developerInstructions": t.Instructions,
	}
}

// ResumeParams are thread/resume's params for threadID, with the same
// settings as the thread started with.
func (t ChatThread) ResumeParams(threadID string) map[string]any {
	p := t.StartParams()
	p["threadId"] = threadID
	return p
}

// SandboxPolicy is the turn's sandbox policy: workspace-write with the
// writable roots and network access.
func (t ChatThread) SandboxPolicy() map[string]any {
	roots := append([]string{}, t.WritableRoots...)
	return map[string]any{"type": "workspaceWrite", "writableRoots": roots, "networkAccess": true}
}

// TurnStartParams are turn/start's params: the "On screen" note first, then
// the message text, then a local image input for each image path.
func (t ChatThread) TurnStartParams(threadID, note, text string, images []string) map[string]any {
	input := []map[string]any{}
	if strings.TrimSpace(note) != "" {
		input = append(input, map[string]any{"type": "text", "text": note})
	}
	if strings.TrimSpace(text) != "" {
		input = append(input, map[string]any{"type": "text", "text": text})
	}
	for _, p := range images {
		input = append(input, map[string]any{"type": "localImage", "path": p})
	}
	return map[string]any{
		"threadId":       threadID,
		"input":          input,
		"cwd":            t.Cwd,
		"approvalPolicy": "on-request",
		"sandboxPolicy":  t.SandboxPolicy(),
	}
}

// ThreadID reads the thread id from a thread/start or thread/resume answer.
func ThreadID(result json.RawMessage) string {
	var r struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	_ = json.Unmarshal(result, &r)
	return r.Thread.ID
}

// TurnID reads the turn id from a turn/start answer.
func TurnID(result json.RawMessage) string {
	var r struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	_ = json.Unmarshal(result, &r)
	return r.Turn.ID
}

// TurnEnd is a turn/completed notification.
type TurnEnd struct {
	ThreadID string
	TurnID   string
	// Status is completed, interrupted or failed.
	Status string
	// Error is the failure's message.
	Error string
}

// ParseTurnCompleted reads a turn/completed notification's params.
func ParseTurnCompleted(params json.RawMessage) TurnEnd {
	var p struct {
		ThreadID string `json:"threadId"`
		Turn     struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"turn"`
	}
	_ = json.Unmarshal(params, &p)
	end := TurnEnd{ThreadID: p.ThreadID, TurnID: p.Turn.ID, Status: p.Turn.Status}
	if p.Turn.Error != nil {
		end.Error = p.Turn.Error.Message
	}
	return end
}

// Approval methods the app-server asks the client to decide.
const (
	MethodCommandApproval    = "item/commandExecution/requestApproval"
	MethodFileChangeApproval = "item/fileChange/requestApproval"
)

// Approval is a command or file-change approval request.
type Approval struct {
	// Tool is Bash for a command and Edit for a file change, as Claude Code
	// names them.
	Tool string
	// Summary names the step: the command, or the files to change.
	Summary string
	Reason  string
	// Input is the step as a tool input.
	Input    json.RawMessage
	ThreadID string
	TurnID   string
	ItemID   string
}

// ParseApproval reads an approval request's params. files gives the paths of
// an item's changes, when the file change was already seen; it may be nil.
func ParseApproval(method string, params json.RawMessage, filesOf func(itemID string) []string) (Approval, bool) {
	var p struct {
		ThreadID  string  `json:"threadId"`
		TurnID    string  `json:"turnId"`
		ItemID    string  `json:"itemId"`
		Command   *string `json:"command"`
		Cwd       *string `json:"cwd"`
		Reason    *string `json:"reason"`
		GrantRoot *string `json:"grantRoot"`
	}
	if json.Unmarshal(params, &p) != nil {
		return Approval{}, false
	}
	a := Approval{ThreadID: p.ThreadID, TurnID: p.TurnID, ItemID: p.ItemID}
	if p.Reason != nil {
		a.Reason = *p.Reason
	}
	switch method {
	case MethodCommandApproval:
		a.Tool = "Bash"
		cmd := ""
		if p.Command != nil {
			cmd = unwrapShell(*p.Command)
		}
		in := map[string]string{"command": cmd}
		if p.Cwd != nil {
			in["cwd"] = *p.Cwd
		}
		a.Input, _ = json.Marshal(in)
		a.Summary = "Bash: " + oneLine(cmd, 200)
		if cmd == "" {
			a.Summary = "Bash"
		}
	case MethodFileChangeApproval:
		a.Tool = "Edit"
		var files []string
		if filesOf != nil {
			files = filesOf(p.ItemID)
		}
		in := map[string]any{"file_path": strings.Join(files, ", ")}
		if p.GrantRoot != nil && *p.GrantRoot != "" {
			in["grant_root"] = *p.GrantRoot
		}
		a.Input, _ = json.Marshal(in)
		switch {
		case len(files) > 0:
			a.Summary = "Edit: " + oneLine(strings.Join(files, ", "), 200)
		case p.GrantRoot != nil && *p.GrantRoot != "":
			a.Summary = "Edit: write under " + *p.GrantRoot
		default:
			a.Summary = "Edit files"
		}
	default:
		return Approval{}, false
	}
	return a, true
}

// ApprovalDecision is an approval request's answer: accept or decline (the
// agent continues its turn either way).
func ApprovalDecision(allow bool) map[string]string {
	if allow {
		return map[string]string{"decision": "accept"}
	}
	return map[string]string{"decision": "decline"}
}

func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// AppItem maps an app-server item/started or item/completed notification to
// run events, as the exec Parser does for `codex exec --json`. It also
// returns the item's id, type and changed file paths.
func (p *Parser) AppItem(params json.RawMessage, done bool) (evs []api.RunEvent, item AppItemInfo) {
	var n struct {
		Item json.RawMessage `json:"item"`
	}
	if json.Unmarshal(params, &n) != nil || len(n.Item) == 0 {
		return nil, item
	}
	var it struct {
		ID               string          `json:"id"`
		Type             string          `json:"type"`
		Text             string          `json:"text"`
		Command          string          `json:"command"`
		AggregatedOutput *string         `json:"aggregatedOutput"`
		ExitCode         *int            `json:"exitCode"`
		Status           string          `json:"status"`
		Changes          []fileChange    `json:"changes"`
		Server           string          `json:"server"`
		Tool             string          `json:"tool"`
		Arguments        json.RawMessage `json:"arguments"`
		Result           json.RawMessage `json:"result"`
		Error            *struct {
			Message string `json:"message"`
		} `json:"error"`
		Query   string   `json:"query"`
		Summary []string `json:"summary"`
		Content []string `json:"content"`
	}
	if json.Unmarshal(n.Item, &it) != nil {
		return nil, item
	}
	item = AppItemInfo{ID: it.ID, Type: it.Type}
	for _, c := range it.Changes {
		item.Files = append(item.Files, c.Path)
	}
	raw := rawItem{
		ID: it.ID, Text: it.Text, Command: it.Command, ExitCode: it.ExitCode, Server: it.Server,
		Tool: it.Tool, Arguments: it.Arguments, Result: it.Result, Query: it.Query,
		Status: appStatus(it.Status),
	}
	if it.AggregatedOutput != nil {
		raw.AggregatedOutput = *it.AggregatedOutput
	}
	if it.Error != nil {
		raw.ItemError = &struct {
			Message string `json:"message"`
		}{Message: it.Error.Message}
	}
	for _, c := range it.Changes {
		raw.Changes = append(raw.Changes, struct {
			Path string `json:"path"`
			Kind string `json:"kind"`
		}{Path: c.Path, Kind: c.kind()})
	}
	switch it.Type {
	case "agentMessage":
		raw.Type = "agent_message"
	case "reasoning":
		raw.Type = "reasoning"
		parts := it.Summary
		if len(parts) == 0 {
			parts = it.Content
		}
		raw.Text = strings.Join(parts, "\n\n")
	case "commandExecution":
		raw.Type = "command_execution"
	case "fileChange":
		raw.Type = "file_change"
	case "mcpToolCall":
		raw.Type = "mcp_tool_call"
	case "webSearch":
		raw.Type = "web_search"
	default:
		return nil, item
	}
	return p.item(raw, done), item
}

// AppItemInfo identifies an app-server item.
type AppItemInfo struct {
	ID    string
	Type  string
	Files []string
}

// fileChange is an app-server FileUpdateChange; kind is a string in older
// servers and {"type": "add"} in newer ones.
type fileChange struct {
	Path string          `json:"path"`
	Kind json.RawMessage `json:"kind"`
}

func (c fileChange) kind() string {
	var s string
	if json.Unmarshal(c.Kind, &s) == nil {
		return s
	}
	var o struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(c.Kind, &o)
	return o.Type
}

// appStatus maps app-server item statuses (inProgress, completed, failed,
// declined) to the exec ones the Parser knows.
func appStatus(s string) string {
	switch s {
	case "failed", "declined":
		return "failed"
	case "inProgress":
		return "in_progress"
	}
	return s
}

// LastText is the turn's newest agent message.
func (p *Parser) LastText() string { return p.last }
