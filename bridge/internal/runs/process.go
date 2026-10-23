package runs

import (
	"bufio"
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
)

const (
	maxLastText      = 280
	maxStderrEvents  = 500  // per turn
	maxStderrLine    = 2000 // runes
	stderrTailLines  = 20
	pipeDrainTimeout = 3 * time.Second
)

// execute runs one turn of r as a `claude -p` process and records its output.
// It runs on its own goroutine; m.wg and m.running were incremented by schedule.
func (m *Manager) execute(r *run, t claude.Turn, cwd string) {
	defer m.wg.Done()
	err := m.runProcess(r, t, cwd)
	m.finishTurn(r, err)
}

func (m *Manager) runProcess(r *run, t claude.Turn, cwd string) error {
	cmd := exec.Command(m.opts.ClaudePath, t.Args()...)
	cmd.Dir = cwd
	cmd.Env = claude.ChildEnv()
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
		return fmt.Errorf("start claude: %w", startErr)
	}
	pgid := cmd.Process.Pid

	r.mu.Lock()
	r.pgid = pgid
	if r.cancelReq || r.shutdownReq {
		r.terminate() // cancel arrived before the process existed
	}
	r.mu.Unlock()

	var readers sync.WaitGroup
	readers.Add(2)
	go func() { defer readers.Done(); m.readStdout(r, outR) }()
	go func() { defer readers.Done(); m.readStderr(r, errR) }()

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

func (m *Manager) readStdout(r *run, rd io.Reader) {
	readLines(rd, func(line []byte) {
		p, err := claude.ParseLine(line)
		r.mu.Lock()
		defer r.mu.Unlock()
		if err != nil {
			// Not JSON: surface it rather than dropping it.
			r.addEvent(api.RunEvent{Kind: api.KindStderr, Text: claude.Truncate(strings.TrimRight(string(line), "\r\n"), maxStderrLine)})
			return
		}
		m.apply(r, p)
	})
}

func (m *Manager) readStderr(r *run, rd io.Reader) {
	n := 0
	readLines(rd, func(line []byte) {
		text := claude.Truncate(strings.TrimRight(string(line), "\r\n"), maxStderrLine)
		if strings.TrimSpace(text) == "" {
			return
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		// Claude Code writes some stream-json notices (e.g. permission_denied) to
		// stderr; show those as events rather than raw JSON.
		if strings.HasPrefix(strings.TrimSpace(text), "{") {
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
	case res != nil && !res.IsError:
		status = api.StatusSucceeded
	case res != nil && res.IsError:
		status, errText = api.StatusFailed, res.Text
	default:
		status = api.StatusFailed
		errText = "claude exited without a result"
		var ee *exec.ExitError
		if errors.As(waitErr, &ee) {
			errText = fmt.Sprintf("claude exited with status %d", ee.ExitCode())
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
