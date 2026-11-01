// Package codex knows how to run the OpenAI Codex CLI headless (`codex exec
// --json`) for a bead, and how to translate its JSONL event stream into the
// same RunEvents a Claude Code run produces, so a client renders both alike.
package codex

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// Turn describes one `codex exec` invocation.
type Turn struct {
	Prompt string
	Goal   string // activates native goal tools in a persistent exec thread
	// Mode is a run permission mode (api.Modes); see sandboxArgs for how each
	// maps onto Codex's sandbox and approval settings.
	Mode  string
	Model string // --model ("" = Codex's configured default)
	// Effort is the reasoning effort ("" = Codex's configured default),
	// passed as a -c override so a follow-up (exec resume) keeps it too.
	Effort   string
	Worktree bool   // --worktree (first turn only)
	Resume   string // thread id to resume for follow-ups
}

// Args returns the argv (without the binary) for a turn.
func (t Turn) Args() []string {
	prompt := t.Prompt
	if t.Goal != "" && t.Resume == "" {
		prompt = "/goal " + t.Goal + "\nFirst call create_goal with the full goal above as its objective, before implementing. Do not merely describe or plan to activate it. Keep the native goal active until every condition holds, then call update_goal with status complete.\n\nLaunch instructions:\n" + prompt
	}
	// The prompt is a positional argument; keep a leading "-" from being
	// parsed as a flag (a lone "-" would mean "read stdin").
	if strings.HasPrefix(prompt, "-") {
		prompt = " " + prompt
	}
	args := []string{"exec"}
	if t.Resume != "" {
		args = append(args, "resume")
	}
	args = append(args, "--json")
	if t.Goal != "" {
		args = append(args, "--enable", "goals")
	}
	args = append(args, sandboxArgs(t.Mode)...)
	if t.Model != "" {
		args = append(args, "--model", t.Model)
	}
	if t.Effort != "" {
		args = append(args, "-c", `model_reasoning_effort="`+t.Effort+`"`)
	}
	if t.Resume == "" && t.Worktree {
		args = append(args, "--worktree")
	}
	if t.Resume != "" {
		args = append(args, t.Resume)
	}
	return append(args, prompt)
}

// sandboxArgs maps a run permission mode onto Codex. `codex exec` never asks
// for approval (there is nobody to ask), so the sandbox is what decides what
// a run may do. They are passed as -c overrides because `codex exec resume`
// does not take --sandbox, and a follow-up must keep the run's mode.
//
//   - plan: read-only sandbox; Codex can look but not change anything.
//   - acceptEdits, dontAsk: workspace-write; edits in the repository, no
//     network, nothing outside it.
//   - auto: workspace-write with network access, the nearest match to Claude
//     Code's auto mode (bd and git need no more than this).
//   - bypassPermissions: no sandbox at all.
func sandboxArgs(mode string) []string {
	switch mode {
	case "plan":
		return []string{"-c", `sandbox_mode="read-only"`}
	case "bypassPermissions":
		return []string{"--dangerously-bypass-approvals-and-sandbox"}
	case "auto":
		return []string{"-c", `sandbox_mode="workspace-write"`, "-c", "sandbox_workspace_write.network_access=true"}
	default:
		return []string{"-c", `sandbox_mode="workspace-write"`}
	}
}

// ResumeCommand is the shell command that continues a Codex session in a
// terminal on the computer.
func ResumeCommand(dir, threadID string) string {
	return "cd " + shellQuote(dir) + " && codex resume " + threadID
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Status is whether Codex can run here, and why not when it cannot.
type Status struct {
	Version string // `codex --version`, e.g. "0.159.2"; "" when not installed
	// Reason says, in a sentence for the person, why Codex runs are not
	// available; "" when they are.
	Reason string
}

// Available reports whether Codex runs can start.
func (s Status) Available() bool { return s.Reason == "" }

// Check finds out whether the Codex CLI at path is installed and signed in.
// env is the child environment (claude.ChildEnv()). An empty path means no
// Codex CLI is configured.
func Check(ctx context.Context, path string, env []string) Status {
	if path == "" {
		return Status{Reason: "Codex CLI isn't installed on this computer"}
	}
	if _, err := exec.LookPath(path); err != nil {
		return Status{Reason: "Codex CLI isn't installed on this computer"}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	v := exec.CommandContext(ctx, path, "--version")
	v.Env = env
	out, err := v.Output()
	if err != nil {
		return Status{Reason: "Codex CLI on this computer doesn't start (" + err.Error() + ")"}
	}
	st := Status{Version: version(string(out))}
	l := exec.CommandContext(ctx, path, "login", "status")
	l.Env = env
	msg, err := l.CombinedOutput()
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		st.Reason = "Codex isn't signed in on this computer; run `codex login` there"
	case err != nil:
		st.Reason = "Couldn't check Codex sign-in (" + err.Error() + ")"
	case strings.Contains(strings.ToLower(string(msg)), "not logged in"):
		st.Reason = "Codex isn't signed in on this computer; run `codex login` there"
	}
	return st
}

// version turns "codex-cli 0.159.2" into "0.159.2".
func version(out string) string {
	f := strings.Fields(strings.TrimSpace(out))
	if len(f) == 0 {
		return ""
	}
	return f[len(f)-1]
}
