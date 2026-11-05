package chat

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
	"github.com/ryderdavid/nacre/bridge/internal/codex"
)

// interruptWait is how long a stopped Codex turn may take to wind down
// before its app-server is killed.
const interruptWait = 3 * time.Second

// CodexEnv is a Codex chat child's environment: nacred's own, with
// NACRED_URL set to nacred's listen address as for Claude chats.
func CodexEnv(nacredURL string) []string {
	env := claude.ChatEnv(nacredURL)
	out := env[:0:0]
	for _, kv := range env {
		if strings.HasPrefix(kv, "CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// codexThread is how the chat's Codex thread runs: its working directory,
// every board on the host writable, and the briefing.
func codexThread(cwd string, spec claude.Chat) codex.ChatThread {
	roots := []string{filepath.Clean(cwd)}
	seen := map[string]bool{roots[0]: true}
	for _, d := range spec.AddDirs {
		d = filepath.Clean(d)
		if d != "" && !seen[d] {
			seen[d] = true
			roots = append(roots, d)
		}
	}
	return codex.ChatThread{Cwd: cwd, WritableRoots: roots, Instructions: spec.Briefing}
}

// codexTurn runs one message of a Codex chat through `codex app-server`:
// thread/start on the first message (thread/resume with the stored thread id
// after), then turn/start with the "On screen" note first and the text. Its
// items become chat events, and its command and file-change approval
// requests become permission requests for the phone. Codex reports no cost.
func (m *Manager) codexTurn(ctx context.Context, c *chatState, t *turn, cwd string, env []string, p turnPlan) (*claude.Result, error) {
	thread := codexThread(cwd, p.spec)
	var (
		mu     sync.Mutex
		parser codex.Parser
		files  = map[string][]string{}
		end    *codex.TurnEnd
		ended  = make(chan struct{})
		srvPtr atomic.Pointer[codex.AppServer]
	)
	onNote := func(method string, params json.RawMessage) {
		switch method {
		case "item/started", "item/completed":
			mu.Lock()
			evs, info := parser.AppItem(params, method == "item/completed")
			if len(info.Files) > 0 {
				files[info.ID] = info.Files
			}
			mu.Unlock()
			for _, ev := range evs {
				if ev.Kind == api.KindResult {
					continue
				}
				m.emit(c, fromRunEvent(ev, t.id, RouteFull))
			}
		case "turn/completed":
			e := codex.ParseTurnCompleted(params)
			mu.Lock()
			if end == nil {
				end = &e
				close(ended)
			}
			mu.Unlock()
		}
	}
	onReq := func(id json.RawMessage, method string, params json.RawMessage) {
		srv := srvPtr.Load()
		if srv == nil {
			return
		}
		switch method {
		case codex.MethodCommandApproval, codex.MethodFileChangeApproval:
			a, ok := codex.ParseApproval(method, params, func(item string) []string {
				mu.Lock()
				defer mu.Unlock()
				return files[item]
			})
			if !ok {
				_ = srv.Respond(id, codex.ApprovalDecision(false))
				return
			}
			m.codexApproval(c, t, srv, id, a)
		default:
			_ = srv.RespondError(id, -32601, "Nacre's Chat can't answer "+method)
		}
	}

	// The app-server outlives ctx briefly, so a stopped turn is interrupted
	// rather than killed mid-write.
	procCtx, killProc := context.WithCancel(context.Background())
	defer killProc()
	srv, err := codex.StartAppServer(procCtx, codex.AppServerOptions{
		Path: m.opts.CodexPath, Dir: cwd, Env: env, OnNotification: onNote, OnRequest: onReq,
	})
	if err != nil {
		if t.wasStopped() {
			return nil, nil
		}
		return nil, err
	}
	srvPtr.Store(srv)
	defer srv.Close()

	threadID, resume := m.codexSession(c)
	var res json.RawMessage
	if resume {
		res, err = srv.Call(ctx, "thread/resume", thread.ResumeParams(threadID))
	} else {
		res, err = srv.Call(ctx, "thread/start", thread.StartParams())
	}
	if err != nil {
		return m.codexEnded(ctx, t, err)
	}
	if id := codex.ThreadID(res); id != "" && !resume {
		threadID = id
		m.threadStarted(c, id)
	}
	if threadID == "" {
		return nil, errors.New("codex app-server started no thread")
	}
	// A local image input for each of this message's stored images.
	paths := make([]string, 0, len(p.images))
	for _, img := range p.images {
		paths = append(paths, img.Path)
	}
	res, err = srv.Call(ctx, "turn/start", thread.TurnStartParams(threadID, p.note, p.text, paths))
	if err != nil {
		return m.codexEnded(ctx, t, err)
	}
	codexTurn := codex.TurnID(res)

	select {
	case <-ended:
	case <-srv.Done():
		select {
		case <-ended:
		default:
			if t.wasStopped() {
				return nil, nil
			}
			return nil, srv.Err()
		}
	case <-ctx.Done():
		if codexTurn != "" {
			ictx, cancel := context.WithTimeout(context.Background(), interruptWait)
			_, _ = srv.Call(ictx, "turn/interrupt", map[string]string{"threadId": threadID, "turnId": codexTurn})
			cancel()
		}
		return m.codexEnded(ctx, t, ctx.Err())
	}
	mu.Lock()
	e, last := *end, parser.LastText()
	mu.Unlock()
	switch e.Status {
	case "completed":
		return &claude.Result{Subtype: "success", Text: last, NumTurns: 1}, nil
	case "interrupted":
		if t.wasStopped() {
			return nil, nil
		}
		return &claude.Result{Subtype: "error", IsError: true, Text: "Codex interrupted the turn", NumTurns: 1}, nil
	default:
		msg := e.Error
		if msg == "" {
			msg = "Codex turn failed"
		}
		return &claude.Result{Subtype: "error", IsError: true, Text: msg, NumTurns: 1}, nil
	}
}

// codexEnded is the outcome of a Codex turn cut short by err.
func (m *Manager) codexEnded(ctx context.Context, t *turn, err error) (*claude.Result, error) {
	switch {
	case t.wasStopped():
		return nil, nil
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return nil, errors.New("codex ran too long and was stopped")
	}
	return nil, err
}

// codexSession is the chat's Codex thread id, and whether a thread was
// started to resume.
func (m *Manager) codexSession(c *chatState) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return c.rec.SessionID, c.rec.SessionStarted && c.rec.SessionID != ""
}

// threadStarted records the chat's Codex thread, so later turns resume it.
func (m *Manager) threadStarted(c *chatState, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c.rec.SessionID, c.rec.SessionStarted = id, true
	m.touchLocked(c)
}

// codexApproval records a Codex approval request as a permission request
// for the phone. The phone's answer, the 15-minute timeout, Stop and the
// turn's end decide it as they do Claude's; a deny's note is passed to the
// agent by steering the turn, since Codex's decline carries no message.
func (m *Manager) codexApproval(c *chatState, t *turn, srv *codex.AppServer, rpcID json.RawMessage, a codex.Approval) {
	reqID := newID("cx_")
	answer := func(allow bool, message string) {
		_ = srv.Respond(rpcID, codex.ApprovalDecision(allow))
		if allow || message == "" || t.wasStopped() || a.TurnID == "" || a.ThreadID == "" {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, _ = srv.Call(ctx, "turn/steer", map[string]any{
				"threadId": a.ThreadID, "expectedTurnId": a.TurnID,
				"input": []map[string]string{{"type": "text", "text": "Not allowed: " + a.Summary + ". " + message}},
			})
		}()
	}
	m.mu.Lock()
	if t.wasStopped() || c.turn != t || c.deleted {
		m.mu.Unlock()
		_ = srv.Respond(rpcID, codex.ApprovalDecision(false))
		return
	}
	m.appendLocked(c, Event{
		Kind: KindPermission, TurnID: t.id, RequestID: reqID, Tool: a.Tool,
		Text: a.Summary, Reason: a.Reason, Input: a.Input,
	})
	m.waitLocked(c, &pending{id: reqID, turn: t, answer: answer})
	m.mu.Unlock()
}
