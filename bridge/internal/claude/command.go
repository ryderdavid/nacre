package claude

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// NamePrefix is the --name prefix nacred gives the sessions it launches, so
// that they can be told apart from agent-owned sessions in Claude Code's
// session registry.
const NamePrefix = "Nacre · "

// Turn describes one `claude -p` invocation.
type Turn struct {
	Prompt   string
	Mode     string // --permission-mode
	Model    string // --model ("" = CLI default)
	Effort   string // --effort ("" = CLI default)
	Worktree bool   // --worktree (first turn only)
	Name     string // --name (first turn only)
	Resume   string // --resume <session id> for follow-ups
	// MaxBudgetUSD caps the turn's spending (--max-budget-usd); 0 = no cap.
	MaxBudgetUSD float64
	// Env is added to the process environment, e.g. a leased resource.
	Env []string
}

// Args returns the argv (without the binary) for a turn.
func (t Turn) Args() []string {
	prompt := t.Prompt
	// The prompt is a positional argument; keep a leading "-" from being
	// parsed as a flag.
	if strings.HasPrefix(prompt, "-") {
		prompt = " " + prompt
	}
	args := []string{"-p", prompt}
	if t.Resume != "" {
		args = append(args, "--resume", t.Resume)
	}
	args = append(args, "--output-format", "stream-json", "--verbose")
	if t.Mode != "" {
		args = append(args, "--permission-mode", t.Mode)
	}
	if t.Model != "" {
		args = append(args, "--model", t.Model)
	}
	if t.Effort != "" {
		args = append(args, "--effort", t.Effort)
	}
	if t.MaxBudgetUSD > 0 {
		args = append(args, "--max-budget-usd", strconv.FormatFloat(t.MaxBudgetUSD, 'f', 2, 64))
	}
	if t.Resume == "" {
		if t.Worktree {
			args = append(args, "--worktree")
		}
		if t.Name != "" {
			args = append(args, "--name", t.Name)
		}
	}
	return args
}

// ChildEnv returns the environment for a claude child process: the bridge's
// own environment minus variables that mark it as running inside another
// Claude Code session (which would change the child's behaviour when nacred
// itself was started from Claude Code).
func ChildEnv() []string {
	env := os.Environ()
	out := env[:0:0]
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if k == "CLAUDECODE" || k == "CLAUDE_PID" || k == "CLAUDE_EFFORT" ||
			strings.HasPrefix(k, "CLAUDE_CODE_") || strings.HasPrefix(k, "CLAUDE_AGENT_SDK") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// Version returns `claude --version` output (e.g. "2.1.263 (Claude Code)" → "2.1.263").
func Version(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	cmd.Env = ChildEnv()
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(out))
	if f := strings.Fields(s); len(f) > 0 {
		return f[0], nil
	}
	return s, nil
}
