// Package gates runs a repository's gate and build commands from nacre.yaml
// and reports what happened. nacred uses it to decide whether work is done
// instead of taking an agent's word for it.
//
// A Runner holds the computer-wide limit on how many gate commands run at
// once (max_concurrent_gates): on the Mac Studio, builds, not agents, are
// what the machine runs out of.
package gates

import (
	"bytes"
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

	"github.com/ryderdavid/nacre/bridge/internal/repoconfig"
)

// TailLines is how many lines of a command's combined output are kept.
const TailLines = 200

// DefaultMaxConcurrent is the default computer-wide gate limit.
const DefaultMaxConcurrent = 2

// killGrace is how long a timed-out command gets between SIGTERM and SIGKILL.
const killGrace = 5 * time.Second

// Result is the outcome of one gate or build command.
type Result struct {
	Name       string `json:"name"`
	OK         bool   `json:"ok"`
	ExitCode   int    `json:"exit_code"`
	TimedOut   bool   `json:"timed_out,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	// Tail is the end of the command's combined stdout and stderr.
	Tail string `json:"tail,omitempty"`
	// Error explains a command that could not be started or was cancelled.
	Error string `json:"error,omitempty"`
}

// Summary describes a failed result in one line, for prompts and notes.
func (r Result) Summary() string {
	switch {
	case r.OK:
		return fmt.Sprintf("gate %q passed", r.Name)
	case r.TimedOut:
		return fmt.Sprintf("gate %q timed out after %s", r.Name, time.Duration(r.DurationMS)*time.Millisecond)
	case r.Error != "":
		return fmt.Sprintf("gate %q could not run: %s", r.Name, r.Error)
	}
	return fmt.Sprintf("gate %q failed with exit status %d", r.Name, r.ExitCode)
}

// Runner runs gate commands under a computer-wide concurrency limit.
type Runner struct {
	sem chan struct{}
	// Shell runs each command; "/bin/sh" unless set.
	Shell string
	// BaseEnv is the environment commands start from; os.Environ() if nil.
	BaseEnv []string
}

// NewRunner returns a Runner that allows max commands at once (at least one).
func NewRunner(max int) *Runner {
	if max <= 0 {
		max = DefaultMaxConcurrent
	}
	return &Runner{sem: make(chan struct{}, max)}
}

// Run executes one step in dir with env added to the environment, waiting for
// a free slot first. A non-zero exit, a timeout, or a cancelled ctx is a
// failure; Run itself never returns an error.
func (r *Runner) Run(ctx context.Context, dir string, env map[string]string, step repoconfig.Step, timeout time.Duration) Result {
	res := Result{Name: step.Name, ExitCode: -1}
	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		res.Error = "cancelled while waiting for a free gate slot"
		return res
	}

	shell := r.Shell
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell, "-c", step.Run)
	cmd.Dir = dir
	cmd.Env = r.environ(env)
	cmd.Stdin = nil
	// Own process group, so a timeout takes Gradle's children with it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	tail := newTail(TailLines)
	cmd.Stdout, cmd.Stderr = tail, tail

	start := time.Now()
	if err := cmd.Start(); err != nil {
		res.Error = err.Error()
		return res
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var timer <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		timer = t.C
	}
	var waitErr error
	select {
	case waitErr = <-done:
	case <-timer:
		res.TimedOut = true
		waitErr = stop(cmd, done)
	case <-ctx.Done():
		res.Error = "cancelled"
		waitErr = stop(cmd, done)
	}
	res.DurationMS = time.Since(start).Milliseconds()
	res.Tail = tail.String()

	var ee *exec.ExitError
	switch {
	case waitErr == nil:
		res.ExitCode, res.OK = 0, !res.TimedOut && res.Error == ""
	case errors.As(waitErr, &ee):
		res.ExitCode = ee.ExitCode()
	default:
		if res.Error == "" {
			res.Error = waitErr.Error()
		}
	}
	return res
}

// RunAll runs steps in order and returns every result. With stopOnFail it
// stops after the first failure, since later gates would only repeat it at
// the cost of minutes of builds.
func (r *Runner) RunAll(ctx context.Context, dir string, cfg *repoconfig.Config, steps []repoconfig.Step, stopOnFail bool) []Result {
	timeout := time.Duration(cfg.Limits.GateTimeoutMinutes) * time.Minute
	var out []Result
	for _, s := range steps {
		res := r.Run(ctx, dir, cfg.Env, s, timeout)
		out = append(out, res)
		if !res.OK && stopOnFail {
			break
		}
	}
	return out
}

// AllOK reports whether every result passed (and there was at least one).
func AllOK(results []Result) bool {
	for _, r := range results {
		if !r.OK {
			return false
		}
	}
	return len(results) > 0
}

// stop terminates the command's process group and waits for it.
func stop(cmd *exec.Cmd, done <-chan error) error {
	pgid := cmd.Process.Pid
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	select {
	case err := <-done:
		return err
	case <-time.After(killGrace):
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		return <-done
	}
}

// environ is the base environment with env layered on top; a leading "~/"
// in a value means the home directory.
func (r *Runner) environ(env map[string]string) []string {
	base := r.BaseEnv
	if base == nil {
		base = os.Environ()
	}
	out := make([]string, 0, len(base)+len(env))
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		if _, override := env[k]; !override {
			out = append(out, kv)
		}
	}
	home, _ := os.UserHomeDir()
	for k, v := range env {
		if strings.HasPrefix(v, "~/") && home != "" {
			v = home + v[1:]
		}
		out = append(out, k+"="+v)
	}
	return out
}

// tailBuffer keeps the last n lines written to it.
type tailBuffer struct {
	mu      sync.Mutex
	n       int
	lines   []string
	partial bytes.Buffer
}

func newTail(n int) *tailBuffer { return &tailBuffer{n: n} }

var _ io.Writer = (*tailBuffer)(nil)

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.partial.Write(p)
	for {
		b := t.partial.Bytes()
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			break
		}
		t.push(string(b[:i]))
		t.partial.Next(i + 1)
	}
	// A single enormous line still has to stay bounded.
	if t.partial.Len() > 64*1024 {
		t.push(t.partial.String())
		t.partial.Reset()
	}
	return len(p), nil
}

func (t *tailBuffer) push(line string) {
	t.lines = append(t.lines, strings.TrimRight(line, "\r"))
	if len(t.lines) > t.n {
		t.lines = t.lines[len(t.lines)-t.n:]
	}
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	lines := t.lines
	if t.partial.Len() > 0 {
		lines = append(append([]string(nil), lines...), t.partial.String())
		if len(lines) > t.n {
			lines = lines[len(lines)-t.n:]
		}
	}
	return strings.Join(lines, "\n")
}
