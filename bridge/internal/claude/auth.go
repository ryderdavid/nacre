package claude

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// The long-lived (one-year) sign-in token `claude setup-token` issues. nacred
// keeps it in its config folder and gives it to every claude it starts, so the
// computer's everyday sign-in, which lapses and has to be renewed at the
// computer, no longer matters.
var token struct {
	sync.RWMutex
	path, value string
}

// UseTokenFile reads the token from path, if there is one, and saves later
// tokens there.
func UseTokenFile(path string) error {
	token.Lock()
	defer token.Unlock()
	token.path = path
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	token.value = strings.TrimSpace(string(b))
	return nil
}

// HasToken reports whether claude runs with nacred's own token.
func HasToken() bool {
	token.RLock()
	defer token.RUnlock()
	return token.value != ""
}

// SaveToken keeps value as the token from now on.
func SaveToken(value string) error {
	token.Lock()
	defer token.Unlock()
	if token.path == "" {
		return errors.New("nacred has no place to keep the token")
	}
	if err := os.MkdirAll(filepath.Dir(token.path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(token.path, []byte(value+"\n"), 0o600); err != nil {
		return err
	}
	token.value = value
	return nil
}

// AuthEnv is what a claude child needs to use the token; nothing without one.
func AuthEnv() []string {
	token.RLock()
	defer token.RUnlock()
	if token.value == "" {
		return nil
	}
	return []string{"CLAUDE_CODE_OAUTH_TOKEN=" + token.value}
}

// RunEnv is ChildEnv with the token, for a claude nacred starts.
func RunEnv() []string { return append(ChildEnv(), AuthEnv()...) }

var (
	linkRe  = regexp.MustCompile(`\x1b\]8;[^;]*;(https://[^\x07\x1b]+)`)
	tokenRe = regexp.MustCompile(`sk-ant-oat\d+-[A-Za-z0-9_-]{20,}`)
	gapRe   = regexp.MustCompile(`\x1b\[\d*C`)
	ansiRe  = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07]*(\x07|\x1b\\)`)
)

// plain is terminal output as text: cursor gaps become spaces.
func plain(b []byte) string {
	return string(ansiRe.ReplaceAll(gapRe.ReplaceAll(b, []byte(" ")), nil))
}

// SignIn drives `claude setup-token` for someone signing in from another
// device: Start gives the sign-in link to open there, and Finish passes back
// the code the page shows and keeps the token. It needs a terminal, so it runs
// under script(1), as on macOS.
type SignIn struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	closeOnce sync.Once

	mu      sync.Mutex
	out     []byte
	changed chan struct{}
	exited  bool
}

// SignInTimeout is how long a started sign-in waits for its code.
const SignInTimeout = 15 * time.Minute

// StartSignIn starts signing in and returns the link to open.
func StartSignIn(claudePath string) (*SignIn, string, error) {
	cmd := exec.Command("script", "-q", "/dev/null", "sh", "-c", `stty cols 1000 2>/dev/null; exec "$0" setup-token`, claudePath)
	// Never open a browser on this computer; the link goes to the phone.
	cmd.Env = append(ChildEnv(), "BROWSER=/usr/bin/true")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, "", err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, "", err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("start claude setup-token: %w", err)
	}
	s := &SignIn{cmd: cmd, stdin: stdin, changed: make(chan struct{})}
	go s.read(stdout)
	var link string
	err = s.await(30*time.Second, func(out []byte) bool {
		if m := linkRe.FindSubmatch(out); m != nil {
			link = string(m[1])
			return true
		}
		return false
	})
	if err != nil {
		s.Close()
		return nil, "", fmt.Errorf("claude didn't offer a sign-in link: %w", err)
	}
	// A sign-in nobody finishes ends on its own.
	time.AfterFunc(SignInTimeout, s.Close)
	return s, link, nil
}

func (s *SignIn) read(r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		s.mu.Lock()
		s.out = append(s.out, buf[:n]...)
		if err != nil {
			s.exited = true
		}
		close(s.changed)
		s.changed = make(chan struct{})
		s.mu.Unlock()
		if err != nil {
			return
		}
	}
}

// await waits until done accepts the output so far.
func (s *SignIn) await(limit time.Duration, done func(out []byte) bool) error {
	deadline := time.After(limit)
	for {
		s.mu.Lock()
		out, exited, changed := s.out, s.exited, s.changed
		s.mu.Unlock()
		if done(out) {
			return nil
		}
		if exited {
			return fmt.Errorf("claude stopped: %s", tail(plain(out)))
		}
		select {
		case <-changed:
		case <-deadline:
			return errors.New("it took too long")
		}
	}
}

// Finish passes the code from the sign-in page and keeps the token claude
// hands back. The sign-in ends either way.
func (s *SignIn) Finish(code string) error {
	defer s.Close()
	s.mu.Lock()
	from := len(s.out)
	s.mu.Unlock()
	if _, err := io.WriteString(s.stdin, strings.TrimSpace(code)+"\r"); err != nil {
		return err
	}
	var tok string
	var bad bool
	err := s.await(90*time.Second, func(out []byte) bool {
		text := plain(out[from:])
		if m := tokenRe.FindString(text); m != "" {
			tok = m
			return true
		}
		bad = strings.Contains(text, "OAuth error")
		return bad
	})
	switch {
	case bad:
		return errors.New("That code didn't work. Copy the whole code from the sign-in page and start again.")
	case err != nil:
		return fmt.Errorf("claude didn't hand back a token: %w", err)
	}
	return SaveToken(tok)
}

// Close ends the sign-in; closing again does nothing.
func (s *SignIn) Close() {
	s.closeOnce.Do(func() {
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		go func() { _ = s.cmd.Wait() }()
	})
}

func tail(s string) string {
	s = strings.TrimSpace(string(bytes.ToValidUTF8([]byte(s), nil)))
	if len(s) > 300 {
		s = s[len(s)-300:]
	}
	return s
}
