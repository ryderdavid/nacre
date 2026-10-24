package claude

import (
	"encoding/json"
	"strings"
)

// Headless describes one short `claude -p` call whose answer is JSON validated
// against a schema. It is how the Nacre app runs its Refine, Draft and board
// assistant features on this computer's Claude subscription instead of paying
// per token with an API key from the phone.
//
// Unlike a bead Run (see [Turn]) these calls are cheap and sandboxed:
// --safe-mode drops hooks, MCP servers, CLAUDE.md, skills and plugins, so the
// request costs about a thousand input tokens instead of a hundred thousand
// and behaves the same on every computer. The caller supplies the whole system
// prompt, so no project context leaks in.
type Headless struct {
	Prompt string
	System string
	// Schema is the JSON Schema the answer must satisfy (--json-schema).
	Schema json.RawMessage
	Model  string
	Effort string
	// Resume continues an earlier session by id; empty starts a new one.
	Resume string
	// Persist keeps the session on disk so a later turn can resume it.
	Persist bool
	// AllowedTools are Claude Code tool permission patterns. Empty means no
	// tools at all; anything not listed is denied without prompting.
	AllowedTools []string
	// ReadRepo also lets the call read the working directory: Read, Grep,
	// Glob and read-only git. Nothing can be written either way.
	ReadRepo bool
}

// RepoReadTools are what a ReadRepo call may use besides AllowedTools.
var RepoReadTools = []string{
	"Read", "Grep", "Glob",
	"Bash(git log:*)", "Bash(git diff:*)", "Bash(git show:*)", "Bash(git status:*)",
	"Bash(git branch:*)", "Bash(ls:*)",
}

// BdReadTools are the only commands the board assistant may run: bd
// subcommands that cannot change anything. Writes never happen here — the
// assistant proposes them and the app applies them through the REST API once
// the user approves, so approval always happens on the phone.
var BdReadTools = []string{
	"Bash(bd list:*)",
	"Bash(bd show:*)",
	"Bash(bd comments:*)",
	"Bash(bd ready:*)",
	"Bash(bd blocked:*)",
	"Bash(bd stats:*)",
	"Bash(bd search:*)",
	"Bash(bd stale:*)",
	"Bash(bd orphans:*)",
	"Bash(bd dep tree:*)",
}

// bdWriteCommands are refused outright, in case a permission pattern above
// ever matches more broadly than intended.
var bdWriteCommands = []string{
	"Bash(bd create:*)", "Bash(bd update:*)", "Bash(bd close:*)", "Bash(bd reopen:*)",
	"Bash(bd delete:*)", "Bash(bd comment:*)", "Bash(bd dep add:*)", "Bash(bd dep remove:*)",
	"Bash(bd dolt:*)", "Bash(bd remember:*)", "Bash(bd forget:*)", "Bash(bd defer:*)",
}

// Args returns the argv (without the binary) for the call.
func (h Headless) Args() []string {
	prompt := h.Prompt
	// The prompt is a positional argument; keep a leading "-" from being
	// parsed as a flag.
	if strings.HasPrefix(prompt, "-") {
		prompt = " " + prompt
	}
	args := []string{"-p", prompt,
		// One predictable sandbox on every computer, and no permission
		// prompt can ever block a call that has nobody to answer it.
		"--safe-mode", "--permission-prompts", "none",
		"--output-format", "stream-json", "--verbose",
	}
	if h.System != "" {
		args = append(args, "--system-prompt", h.System)
	}
	if len(h.Schema) > 0 {
		args = append(args, "--json-schema", string(h.Schema))
	}
	if h.ReadRepo {
		args = append(args, "--tools", "Bash,Read,Grep,Glob")
		args = append(args, "--disallowed-tools")
		args = append(args, bdWriteCommands...)
		args = append(args, "--allowed-tools")
		args = append(args, append(append([]string{}, h.AllowedTools...), RepoReadTools...)...)
	} else if len(h.AllowedTools) > 0 {
		args = append(args, "--tools", "Bash")
		args = append(args, "--disallowed-tools")
		args = append(args, bdWriteCommands...)
		args = append(args, "--allowed-tools")
		args = append(args, h.AllowedTools...)
	} else {
		args = append(args, "--tools", "")
	}
	if h.Model != "" {
		args = append(args, "--model", h.Model)
	}
	if h.Effort != "" {
		args = append(args, "--effort", h.Effort)
	}
	if h.Resume != "" {
		args = append(args, "--resume", h.Resume)
	}
	if !h.Persist {
		args = append(args, "--no-session-persistence")
	}
	return args
}
