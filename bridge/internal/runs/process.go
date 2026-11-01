package runs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
	"github.com/ryderdavid/nacre/bridge/internal/codex"
)

const (
	maxLastText      = 280
	maxStderrEvents  = 500  // per turn
	maxStderrLine    = 2000 // runes
	stderrTailLines  = 20
	pipeDrainTimeout = 3 * time.Second
)

// execute runs one turn of r as a `claude -p` process (or `codex exec` for
// agent api.AgentCodex) and records its output. It runs on its own goroutine;
// m.wg and m.running were incremented by schedule.
func (m *Manager) execute(r *run, agent string, t claude.Turn, cwd string) {
	defer m.wg.Done()

	var err error
	for {
		err = m.runProcess(r, m.command(agent, t), cwd)
		if t.Goal == "" || err != nil {
			break
		}
		r.mu.Lock()
		session, cancelled, stopped, res := r.meta.SessionID, r.cancelReq, r.shutdownReq, r.turnResult
		r.mu.Unlock()
		if cancelled || stopped || res == nil || res.IsError {
			break
		}
		if agent != api.AgentCodex {
			if res.TerminalReason != "" && res.TerminalReason != "completed" {
				err = fmt.Errorf("native goal did not complete: %s", res.TerminalReason)
			}
			break // Claude's /goal Stop hook owns its continuation loop.
		}
		state, goalErr := codex.GoalState(context.Background(), m.command(agent, t).path, session, claude.ChildEnv())
		if goalErr != nil {
			err = fmt.Errorf("verify native goal: %w", goalErr)
			break
		}
		if state == "complete" {
			break
		}
		if state != "active" {
			err = fmt.Errorf("native goal is %s; completion has not been established", state)
			break
		}
		// codex exec exits at a turn boundary even with an active native goal.
		// Keep this run and concurrency slot alive and resume the SAME thread.
		r.mu.Lock()
		if r.cancelReq || r.shutdownReq {
			r.mu.Unlock()
			break
		}
		t.Resume, t.Worktree = session, false
		t.Prompt = "Continue working on the active native goal. Call get_goal, verify every acceptance criterion and selected commit/push condition, repair what remains, and call update_goal with status complete only when all conditions hold. Report a genuine blocker through the native goal if progress requires unavailable input."
		r.meta.Turns++
		r.turnResult, r.stderrTail = nil, nil
		r.addEvent(api.RunEvent{Kind: api.KindUser, Text: t.Prompt})
		_ = r.save()
		snap := r.meta
		if r.meta.SessionCWD != "" {
			cwd = r.meta.SessionCWD
		}
		r.mu.Unlock()
		m.notify(snap)
	}

	m.finishTurn(r, err)
}

// agentCommand is how to run one turn with one agent: the process to start
// and how to read its stdout.
type agentCommand struct {
	name  string // for messages: "claude", "codex"
	path  string
	args  []string
	env   []string // added to the child environment
	parse func(line []byte) (claude.Parsed, error)
	// stderrJSON is set when the agent writes stream-json notices to stderr.
	stderrJSON bool
	// timed adds the turn's wall time to the run's DurationMS, for an agent
	// whose result does not report it.
	timed bool
	// stopSig is how to ask the agent to stop (0 = SIGTERM); see run.stopSig.
	stopSig syscall.Signal
}

func (m *Manager) command(agent string, t claude.Turn) agentCommand {
	if agent == api.AgentCodex {
		ct := codex.Turn{Prompt: t.Prompt, Goal: t.Goal, Mode: t.Mode, Model: t.Model, Effort: t.Effort, Worktree: t.Worktree, Resume: t.Resume}
		path := m.opts.CodexPath
		if path == "" {
			path = "codex"
		}
		return agentCommand{name: "codex", path: path, args: ct.Args(), env: t.Env, parse: (&codex.Parser{}).ParseLine, timed: true, stopSig: syscall.SIGINT}
	}
	return agentCommand{name: "claude", path: m.opts.ClaudePath, args: t.Args(), env: t.Env, parse: claude.ParseLine, stderrJSON: true}
}

func (m *Manager) runProcess(r *run, ac agentCommand, cwd string) error {
	cmd := exec.Command(ac.path, ac.args...)
	cmd.Dir = cwd
	cmd.Env = append(claude.ChildEnv(), ac.env...)
	cmd.Stdin = nil // /dev/null
	// Own process group so cancel can signal the whole tree.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Use os.Pipe directly (not StdoutPipe) so Wait does not depend on the
	// pipes closing: a backgrounded grandchild may keep them open after
	// claude itself exits.
	outR, outW, err := os.Pipe()
	if err != nil {
		return err
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		outR.Close()
		outW.Close()
		return err
	}
	cmd.Stdout, cmd.Stderr = outW, errW
	startErr := cmd.Start()
	outW.Close()
	errW.Close()
	if startErr != nil {
		outR.Close()
		errR.Close()
		return fmt.Errorf("start %s: %w", ac.name, startErr)
	}
	pgid := cmd.Process.Pid
	started := time.Now()

	r.mu.Lock()
	r.pgid = pgid
	r.stopSig = ac.stopSig
	r.lastOutput = time.Now()
	if r.cancelReq || r.shutdownReq {
		r.terminate() // cancel arrived before the process existed
	}
	r.mu.Unlock()

	var readers sync.WaitGroup
	readers.Add(2)
	go func() { defer readers.Done(); m.readStdout(r, outR, ac.parse) }()
	go func() { defer readers.Done(); m.readStderr(r, errR, ac.stderrJSON) }()

	waitErr := cmd.Wait()

	drained := make(chan struct{})
	go func() { readers.Wait(); close(drained) }()
	select {
	case <-drained:
	case <-time.After(pipeDrainTimeout):
		// Leftover descendants hold the pipes open; kill the group.
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		outR.Close()
		errR.Close()
		<-drained
	}
	outR.Close()
	errR.Close()

	r.mu.Lock()
	r.pgid = 0
	if ac.timed {
		r.meta.DurationMS += time.Since(started).Milliseconds()
	}
	r.mu.Unlock()
	return waitErr
}

// readLines calls fn for each line of rd (no length limit).
func readLines(rd io.Reader, fn func(line []byte)) {
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

func (m *Manager) readStdout(r *run, rd io.Reader, parse func([]byte) (claude.Parsed, error)) {
	readLines(rd, func(line []byte) {
		p, err := parse(line)
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.heard() {
			_ = r.save()
			m.notify(r.meta)
		}
		if err != nil {
			// Not JSON: surface it rather than dropping it.
			r.addEvent(api.RunEvent{Kind: api.KindStderr, Text: claude.Truncate(strings.TrimRight(string(line), "\r\n"), maxStderrLine)})
			return
		}
		m.apply(r, p)
	})
}

func (m *Manager) readStderr(r *run, rd io.Reader, parseJSON bool) {
	n := 0
	readLines(rd, func(line []byte) {
		text := claude.Truncate(strings.TrimRight(string(line), "\r\n"), maxStderrLine)
		if strings.TrimSpace(text) == "" {
			return
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.heard() {
			_ = r.save()
			m.notify(r.meta)
		}
		// Claude Code writes some stream-json notices (e.g. permission_denied) to
		// stderr; show those as events rather than raw JSON.
		if parseJSON && strings.HasPrefix(strings.TrimSpace(text), "{") {
			if p, err := claude.ParseLine(line); err == nil && len(p.Events) > 0 {
				m.apply(r, p)
				return
			}
		}
		r.stderrTail = append(r.stderrTail, text)
		if len(r.stderrTail) > stderrTailLines {
			r.stderrTail = r.stderrTail[1:]
		}
		n++
		if n <= maxStderrEvents {
			r.addEvent(api.RunEvent{Kind: api.KindStderr, Text: text})
		}
	})
}

// apply folds one parsed stream-json line into the run. Caller holds r.mu.
func (m *Manager) apply(r *run, p claude.Parsed) {
	significant := false
	if p.Init != nil {
		if p.Init.SessionID != "" && p.Init.SessionID != r.meta.SessionID {
			r.meta.SessionID = p.Init.SessionID
			significant = true
		}
		if p.Init.CWD != "" && p.Init.CWD != r.meta.SessionCWD {
			r.meta.SessionCWD = p.Init.CWD
			significant = true
		}
	}
	for _, ev := range p.Events {
		switch ev.Kind {
		case api.KindToolUse:
			r.meta.ToolCalls++
		case api.KindText:
			r.meta.LastText = claude.OneLine(ev.Text, maxLastText)
		}
		r.addEvent(ev)
	}
	if res := p.Result; res != nil {
		r.turnResult = res
		r.meta.CostUSD += res.CostUSD
		r.meta.DurationMS += res.DurationMS
		r.meta.NumTurns += res.NumTurns
		r.meta.ResultText = res.Text
		if res.SessionID != "" {
			r.meta.SessionID = res.SessionID
		}
		significant = true
	}
	if significant {
		r.changed()
		_ = r.save()
		m.notify(r.meta)
	}
}

// finishTurn records the outcome of a turn and schedules queued work.
func (m *Manager) finishTurn(r *run, waitErr error) {
	r.mu.Lock()
	res := r.turnResult
	var status, errText string
	switch {
	case r.cancelReq:
		status, errText = api.StatusCancelled, "cancelled"
	case r.shutdownReq:
		status, errText = api.StatusFailed, "bridge stopped"
	case waitErr != nil && res != nil && !res.IsError:
		status, errText = api.StatusFailed, waitErr.Error()
	case res != nil && !res.IsError:
		status = api.StatusSucceeded
	case res != nil && res.IsError:
		status, errText = api.StatusFailed, res.Text
	default:
		status = api.StatusFailed
		agent := "claude"
		if r.meta.Agent == api.AgentCodex {
			agent = "codex"
		}
		errText = agent + " exited without a result"
		var ee *exec.ExitError
		if errors.As(waitErr, &ee) {
			errText = fmt.Sprintf("%s exited with status %d", agent, ee.ExitCode())
		} else if waitErr != nil {
			errText = waitErr.Error()
		}
		if n := len(r.stderrTail); n > 0 {
			errText += ": " + r.stderrTail[n-1]
		}
	}
	r.cancelReq = false
	r.meta.Error = errText
	r.meta.EndedAt = now()
	r.meta.Stalled, r.meta.StalledSince = false, ""
	r.setStatus(status)
	_ = r.save()
	snap := r.meta
	r.mu.Unlock()

	m.notify(snap)
	if m.opts.RepoChanged != nil {
		m.opts.RepoChanged(snap.RepoID)
	}
	m.mu.Lock()
	m.running--
	m.schedule()
	m.mu.Unlock()
}
