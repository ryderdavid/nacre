package chat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// TurnTimeout stops a turn that has run for this long. Chat turns can build
// and test for a long time; this only ends one that is wedged.
const TurnTimeout = 6 * time.Hour

// maxText bounds one message's text.
const maxText = 100_000

// SendRequest is POST /v1/chats/{id}/messages.
type SendRequest struct {
	TurnID  string   `json:"turn_id"`
	Text    string   `json:"text"`
	Context *Context `json:"context"`
}

// SendResult is the turn a message started, or the one a resend names.
type SendResult struct {
	TurnID string `json:"turn_id"`
	// Started is false when the turn id was already known.
	Started bool   `json:"started"`
	Chat    Record `json:"chat"`
}

// turn is a running agent process.
type turn struct {
	id     string
	cancel context.CancelFunc
	mu     sync.Mutex
	// stopped is set when the turn was cancelled on purpose.
	stopped bool
}

func (t *turn) stop() {
	t.markStopped()
	t.cancel()
}

// markStopped records that the turn is being stopped on purpose, so a
// permission request arriving now is denied at once.
func (t *turn) markStopped() {
	t.mu.Lock()
	t.stopped = true
	t.mu.Unlock()
}

func (t *turn) wasStopped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stopped
}

// Send starts a turn with the message, detached from the caller. A turn id
// the chat already knows returns that turn and starts nothing.
func (m *Manager) Send(id string, req SendRequest) (SendResult, error) {
	if strings.TrimSpace(req.Text) == "" {
		return SendResult{}, errors.New("text required")
	}
	if len(req.Text) > maxText {
		return SendResult{}, errors.New("text too long")
	}
	if req.TurnID == "" {
		req.TurnID = newID("t_")
	} else if !ValidTurnID(req.TurnID) {
		return SendResult{}, errors.New("invalid turn id")
	}
	m.mu.Lock()
	c := m.chats[id]
	if c == nil {
		m.mu.Unlock()
		return SendResult{}, ErrNotFound
	}
	for _, known := range c.rec.TurnIDs {
		if known == req.TurnID {
			res := SendResult{TurnID: known, Chat: c.rec}
			m.mu.Unlock()
			return res, nil
		}
	}
	switch {
	case c.rec.State == StateRunning || c.rec.State == StateWaitingPermission:
		m.mu.Unlock()
		return SendResult{}, ErrBusy
	case c.rec.State == StateLimitReached || c.rec.CostUSD >= claude.ChatRunawayUSD-0.005:
		if c.rec.State != StateLimitReached {
			c.rec.State, c.rec.Reason = StateLimitReached, LimitReason
			m.touchLocked(c)
		}
		m.mu.Unlock()
		return SendResult{}, ErrLimit
	case c.rec.Agent != AgentClaude:
		m.mu.Unlock()
		return SendResult{}, fmt.Errorf("%s chats can't run on this computer yet", c.rec.Agent)
	}

	note := Note(req.Context, c.rec.LastContext, m.opts.HostName, m.opts.RepoPath)
	m.loadLocked(c)
	firstMessage := true
	for _, ev := range c.events {
		if ev.Kind == KindUser {
			firstMessage = false
			break
		}
	}
	if firstMessage && !c.rec.NewEffort {
		if t := trimTitle(req.Text); t != "" {
			c.rec.Title = t
		}
	}
	var ctxCopy *Context
	if req.Context != nil {
		cc := *req.Context
		ctxCopy = &cc
	}
	m.appendLocked(c, Event{Kind: KindUser, TurnID: req.TurnID, Text: req.Text, Context: ctxCopy})
	c.rec.LastContext = ctxCopy
	c.rec.TurnIDs = append(c.rec.TurnIDs, req.TurnID)
	if len(c.rec.TurnIDs) > maxTurnIDs {
		c.rec.TurnIDs = c.rec.TurnIDs[len(c.rec.TurnIDs)-maxTurnIDs:]
	}
	c.rec.TurnID = req.TurnID
	c.rec.State, c.rec.Reason = StateRunning, ""

	spec := claude.Chat{
		Briefing:     Briefing(BriefingInput{Host: m.opts.HostName, NacredURL: m.opts.NacredURL, Phone: c.rec.Phone}),
		MaxBudgetUSD: Remaining(c.rec.CostUSD),
		AddDirs:      append([]string{}, c.rec.AddDirs...),
	}
	if c.rec.SessionStarted {
		spec.Resume = c.rec.SessionID
	} else {
		spec.SessionID = c.rec.SessionID
	}
	ctx, cancel := context.WithTimeout(context.Background(), TurnTimeout)
	t := &turn{id: req.TurnID, cancel: cancel}
	c.turn = t
	// Counted before the turn starts, so an update can't slip in between.
	m.active++
	m.touchLocked(c)
	res := SendResult{TurnID: req.TurnID, Started: true, Chat: c.rec}
	cwd, env := c.rec.Cwd, claude.ChatEnv(m.opts.NacredURL)
	m.mu.Unlock()

	go m.run(ctx, c, t, cwd, env, spec, note, req.Text)
	return res, nil
}

// Remaining is what is left of a chat's runaway ceiling after spent.
func Remaining(spent float64) float64 {
	r := claude.ChatRunawayUSD - spent
	if r < 0 {
		return 0
	}
	return r
}

// Cancel stops the chat's running turn, if any.
func (m *Manager) Cancel(id string) error {
	m.mu.Lock()
	c := m.chats[id]
	if c == nil {
		m.mu.Unlock()
		return ErrNotFound
	}
	t := c.turn
	m.mu.Unlock()
	if t != nil {
		// Pending requests are denied while the CLI still listens, then
		// the turn ends.
		t.markStopped()
		m.denyAll(c, StoppedReason, ByStop)
		t.stop()
	}
	return nil
}

// emit appends an event for the running turn.
func (m *Manager) emit(c *chatState, ev Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.appendLocked(c, ev)
}

// run is one turn's agent process, from start to the turn_end event.
func (m *Manager) run(ctx context.Context, c *chatState, t *turn, cwd string, env []string, spec claude.Chat, note, text string) {
	defer t.cancel()
	result, runErr := m.exec(ctx, c, t, cwd, env, spec, note, text)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.active--
	c.turn = nil
	if t.wasStopped() {
		m.closePendingLocked(c, StoppedReason, ByStop)
	} else {
		m.closePendingLocked(c, TurnOverReason, ByTurnEnd)
	}
	if result != nil {
		c.rec.CostUSD += result.CostUSD
		if result.SessionID != "" && result.SessionID == c.rec.SessionID {
			c.rec.SessionStarted = true
		}
	}
	endText := "Turn finished"
	isErr := false
	switch {
	case t.wasStopped():
		endText = "Stopped"
	case runErr != nil:
		m.appendLocked(c, Event{Kind: KindError, TurnID: t.id, Text: runErr.Error()})
		endText, isErr = "Turn ended with an error", true
	case result != nil && result.IsError:
		m.appendLocked(c, Event{Kind: KindError, TurnID: t.id, Text: claude.OneLine(result.Text, 400)})
		endText, isErr = "Turn ended with an error", true
	}
	end := Event{Kind: KindTurnEnd, TurnID: t.id, Text: endText}
	if isErr {
		end.IsError = api.Bool(true)
	}
	if result != nil {
		end.CostUSD = result.CostUSD
	}
	m.appendLocked(c, end)
	if c.rec.CostUSD >= claude.ChatRunawayUSD-0.005 {
		c.rec.State, c.rec.Reason = StateLimitReached, LimitReason
	} else {
		c.rec.State, c.rec.Reason = StateIdle, ""
	}
	m.touchLocked(c)
}

// exec runs the claude process for one turn and returns its result line,
// or an error when it ended without one.
func (m *Manager) exec(ctx context.Context, c *chatState, t *turn, cwd string, env []string, spec claude.Chat, note, text string) (*claude.Result, error) {
	msg, err := claude.ChatUserMessage(note, text)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, m.opts.ClaudePath, spec.Args()...)
	cmd.Dir = cwd
	cmd.Env = env
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
		return nil, fmt.Errorf("start claude: %w", err)
	}

	// Stdin stays open for the turn so nacred can answer host prompts on
	// the control channel; it is closed once the result arrives.
	var stdinMu sync.Mutex
	stdinOpen := true
	write := func(b []byte) {
		stdinMu.Lock()
		defer stdinMu.Unlock()
		if stdinOpen {
			_, _ = stdin.Write(b)
		}
	}
	closeStdin := func() {
		stdinMu.Lock()
		defer stdinMu.Unlock()
		if stdinOpen {
			stdinOpen = false
			_ = stdin.Close()
		}
	}
	defer closeStdin()
	write(msg)

	var (
		result *claude.Result
		tail   []string
		wg     sync.WaitGroup
		mu     sync.Mutex
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		scan(stdout, func(line []byte) {
			if req, ok := claude.ParseControlRequest(line); ok {
				m.control(c, t, req, write)
				return
			}
			p, perr := claude.ParseLine(line)
			if perr != nil {
				return
			}
			if p.Init != nil && p.Init.SessionID != "" {
				m.sessionStarted(c, p.Init.SessionID)
			}
			for _, ev := range p.Events {
				if ev.Kind == api.KindResult {
					continue // the turn_end event closes the turn
				}
				m.emit(c, fromRunEvent(ev, t.id))
			}
			if p.Result != nil {
				mu.Lock()
				result = p.Result
				mu.Unlock()
				closeStdin()
			}
		})
	}()
	go func() {
		defer wg.Done()
		scan(stderr, func(line []byte) {
			s := strings.TrimSpace(string(line))
			if s == "" {
				return
			}
			mu.Lock()
			tail = append(tail, claude.Truncate(s, 400))
			if len(tail) > 10 {
				tail = tail[1:]
			}
			mu.Unlock()
		})
	}()
	wg.Wait()
	waitErr := cmd.Wait()
	mu.Lock()
	defer mu.Unlock()
	if result != nil {
		return result, nil
	}
	if t.wasStopped() {
		return nil, nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, errors.New("claude ran too long and was stopped")
	}
	msgText := "claude exited without an answer"
	var ee *exec.ExitError
	if errors.As(waitErr, &ee) {
		msgText = fmt.Sprintf("claude exited with status %d without an answer", ee.ExitCode())
	} else if waitErr != nil {
		msgText = waitErr.Error()
	}
	if len(tail) > 0 {
		msgText += ": " + tail[len(tail)-1]
	}
	return nil, errors.New(msgText)
}

// sessionStarted records that the agent has the chat's session, so the next
// turn resumes it even if this one never finishes.
func (m *Manager) sessionStarted(c *chatState, sid string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sid == c.rec.SessionID && !c.rec.SessionStarted {
		c.rec.SessionStarted = true
		m.touchLocked(c)
	}
}

// fromRunEvent turns a transcript event into a chat event of the full route.
func fromRunEvent(ev api.RunEvent, turnID string) Event {
	out := Event{
		Kind: ev.Kind, TurnID: turnID, Text: ev.Text, Tool: ev.Tool, ToolID: ev.ToolID,
		Input: ev.Input, IsError: ev.IsError,
	}
	switch ev.Kind {
	case api.KindText, api.KindThinking, api.KindToolUse, api.KindToolResult:
		out.Route = RouteFull
	}
	return out
}

func scan(rd io.Reader, fn func([]byte)) {
	br := bufio.NewReaderSize(rd, 64*1024)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			fn(line)
		}
		if err != nil {
			return
		}
	}
}

// workDir is where an across-boards chat runs.
func (m *Manager) workDir() string { return filepath.Join(m.opts.Dir, "work") }
