// Package assist runs the short headless `claude -p` calls behind the Nacre
// app's Refine, Draft and board-assistant features, so they use this
// computer's Claude subscription rather than an API key on the phone.
//
// It is deliberately much simpler than the run manager: one call, one process,
// no queue and no history. The caller gets the JSON answer back, plus the
// transcript events on the way if it wants to stream them.
package assist

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// DefaultTimeout bounds one call. A board question with a few bd reads takes
// well under a minute; this only has to stop a wedged process.
const DefaultTimeout = 8 * time.Minute

// maxStderrTail is how many stderr lines are kept for an error message.
const maxStderrTail = 10

// Runner executes headless calls with the configured claude binary.
type Runner struct {
	active     atomic.Int64
	ClaudePath string
	Timeout    time.Duration
	// Host names this computer in errors a person has to fix here.
	Host string

	turnsMu sync.Mutex
	turns   map[string]*Turn
}

// Outcome is what one call produced.
type Outcome struct {
	// SessionID identifies the conversation, for a later Headless.Resume.
	SessionID string
	// Output is the answer validated against the request's schema.
	Output json.RawMessage
	// Text is the plain final message, when no schema was given.
	Text       string
	CostUSD    float64
	DurationMS int64
}

// Run executes h with cwd as the working directory. Every transcript event is
// passed to emit as it arrives (emit may be nil). The error describes what a
// person can do about it.
func (r *Runner) Run(ctx context.Context, cwd string, h claude.Headless, emit func(api.RunEvent)) (Outcome, error) {
	r.active.Add(1)
	defer r.active.Add(-1)
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.ClaudePath, h.Args()...)
	cmd.Dir = cwd
	cmd.Env = claude.RunEnv()
	cmd.Stdin = nil
	// Own process group, so the whole tree dies with the context.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Outcome{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Outcome{}, err
	}
	if err := cmd.Start(); err != nil {
		return Outcome{}, fmt.Errorf("start claude: %w", err)
	}

	var (
		mu     sync.Mutex
		out    Outcome
		result *claude.Result
		tail   []string
		wg     sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanLines(stdout, func(line []byte) {
			p, perr := claude.ParseLine(line)
			if perr != nil {
				return // not stream-json; nothing a client can use
			}
			mu.Lock()
			if p.Init != nil && p.Init.SessionID != "" {
				out.SessionID = p.Init.SessionID
			}
			if p.Result != nil {
				result = p.Result
			}
			mu.Unlock()
			if emit != nil {
				for _, ev := range p.Events {
					emit(ev)
				}
			}
		})
	}()
	go func() {
		defer wg.Done()
		scanLines(stderr, func(line []byte) {
			text := strings.TrimSpace(string(line))
			if text == "" {
				return
			}
			mu.Lock()
			tail = append(tail, claude.Truncate(text, 400))
			if len(tail) > maxStderrTail {
				tail = tail[1:]
			}
			mu.Unlock()
		})
	}()
	// Both pipes must be drained before Wait, which closes them.
	wg.Wait()
	waitErr := cmd.Wait()

	switch {
	case result != nil && result.IsError:
		return out, r.signInError(fmt.Errorf("claude: %s", claude.OneLine(result.Text, 400)))
	case result == nil:
		return out, r.signInError(exitError(ctx, waitErr, tail))
	}
	if result.SessionID != "" {
		out.SessionID = result.SessionID
	}
	out.Output, out.Text = result.Structured, result.Text
	out.CostUSD, out.DurationMS = result.CostUSD, result.DurationMS
	if len(h.Schema) > 0 && len(out.Output) == 0 {
		return out, errors.New("claude answered without the structured output this request asked for")
	}
	return out, nil
}

// signedOut matches what Claude Code says when its sign-in has lapsed.
var signedOut = []string{"Failed to authenticate", "OAuth session expired", "Invalid API key", "Please run /login", "not logged in"}

// signInError turns a lapsed sign-in into what to do about it, on which
// computer; other errors pass through.
func (r *Runner) signInError(err error) error {
	if err == nil {
		return nil
	}
	for _, s := range signedOut {
		if strings.Contains(err.Error(), s) {
			host := r.Host
			if host == "" {
				host = "this computer"
			}
			return fmt.Errorf("Claude is signed out on %s. Sign it in from the app with Sign in to Claude, then try again.", host)
		}
	}
	return err
}

// exitError explains a claude process that produced no result.
func exitError(ctx context.Context, waitErr error, tail []string) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errors.New("claude took too long and was stopped")
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return context.Canceled
	}
	msg := "claude exited without an answer"
	var ee *exec.ExitError
	if errors.As(waitErr, &ee) {
		msg = fmt.Sprintf("claude exited with status %d", ee.ExitCode())
	} else if waitErr != nil {
		msg = waitErr.Error()
	}
	if len(tail) > 0 {
		msg += ": " + tail[len(tail)-1]
	}
	return errors.New(msg)
}

// scanLines calls fn for each line of rd, with no line-length limit.
func scanLines(rd io.Reader, fn func(line []byte)) {
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

// Available reports whether the claude binary can be executed at all, so the
// app can say "this computer can't run Claude" instead of failing per call.
func (r *Runner) Available() bool {
	if r.ClaudePath == "" {
		return false
	}
	if strings.ContainsRune(r.ClaudePath, os.PathSeparator) {
		st, err := os.Stat(r.ClaudePath)
		return err == nil && !st.IsDir()
	}
	_, err := exec.LookPath(r.ClaudePath)
	return err == nil
}
