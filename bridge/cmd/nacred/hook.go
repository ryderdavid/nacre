package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/sessions"
)

// hookTimeout bounds everything `nacred hook` does, from reading the config to
// nacred's answer. The hook runs inside the agent's session, so a slow or
// absent nacred must cost it no more than this.
const hookTimeout = 1500 * time.Millisecond

// maxHookInput bounds what is read from the hook's stdin. A PostToolUse
// payload carries the tool's whole output, which nacred does not need.
const maxHookInput = 8 << 20

// hookInput is the part of Claude Code's hook payload that nacred uses
// (https://code.claude.com/docs/en/hooks). It is also the body sent to
// POST /v1/sessions, so the tool's output never leaves the machine's hook.
type hookInput struct {
	SessionID      string         `json:"session_id"`
	TranscriptPath string         `json:"transcript_path,omitempty"`
	CWD            string         `json:"cwd,omitempty"`
	HookEventName  string         `json:"hook_event_name,omitempty"`
	ToolName       string         `json:"tool_name,omitempty"`
	ToolInput      *hookToolInput `json:"tool_input,omitempty"`
}

type hookToolInput struct {
	Command string `json:"command,omitempty"`
}

// runHook is `nacred hook`: it reads a Claude Code hook payload on stdin and
// posts it to this computer's nacred, fire and forget. It always returns 0
// and writes nothing to stdout (a SessionStart hook's stdout would become
// context for the agent), whatever happens: no config, adoption off, nacred
// down, a bad payload. `--settings` instead prints the settings.json snippet
// that installs it.
func runHook(args []string, stdin io.Reader, stdout io.Writer) int {
	fs := flag.NewFlagSet("nacred hook", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", config.DefaultPath(), "config file path")
	settings := fs.Bool("settings", false, "print the Claude Code settings that install this hook")
	if fs.Parse(args) != nil {
		return 0
	}
	if *settings {
		printHookSettings(stdout, *configPath)
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
	defer cancel()
	_ = postHook(ctx, *configPath, stdin, http.DefaultClient)
	return 0
}

// postHook does the work of runHook and reports why it sent nothing, for
// tests; runHook ignores the answer.
func postHook(ctx context.Context, configPath string, stdin io.Reader, client *http.Client) error {
	data, err := io.ReadAll(io.LimitReader(stdin, maxHookInput))
	if err != nil {
		return err
	}
	var in hookInput
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	if !sessions.ValidSessionID(in.SessionID) {
		return fmt.Errorf("no session id")
	}
	if in.HookEventName == "PostToolUse" || in.ToolName != "" {
		// Only a bd claim or close says anything nacred's poll would want
		// sooner; every other tool call stays local.
		if in.ToolName != "Bash" || in.ToolInput == nil || !sessions.LinksBead(in.ToolInput.Command) {
			return fmt.Errorf("not a bd claim or close")
		}
	}
	cfg, err := readHookConfig(configPath)
	if err != nil {
		return err
	}
	if !cfg.AdoptSessions {
		return fmt.Errorf("adopt_sessions is off")
	}
	addr, err := hookAddr(ctx, cfg)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(in)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/v1/sessions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("nacred answered %s", resp.Status)
	}
	return nil
}

// readHookConfig reads nacred's config without config.Load, which would
// create the file and a token on a machine that has none.
func readHookConfig(path string) (*config.Config, error) {
	data, err := os.ReadFile(config.Expand(path))
	if err != nil {
		return nil, err
	}
	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("no token in %s", path)
	}
	return &cfg, nil
}

// hookAddr is where this computer's nacred listens, as seen from the same
// computer: the configured address (a wildcard host means loopback), else the
// Tailscale address nacred picks when none is configured, else loopback.
func hookAddr(ctx context.Context, cfg *config.Config) (string, error) {
	if cfg.Listen != "" {
		host, port, err := net.SplitHostPort(cfg.Listen)
		if err != nil {
			return "", err
		}
		if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
			host = "127.0.0.1"
		}
		return net.JoinHostPort(host, port), nil
	}
	ch := make(chan string, 1)
	go func() { ch <- config.TailscaleIPv4() }()
	select {
	case ip := <-ch:
		if ip != "" {
			return net.JoinHostPort(ip, config.DefaultPort), nil
		}
		return net.JoinHostPort("127.0.0.1", config.DefaultPort), nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// printHookSettings writes the "hooks" block for a Claude Code settings file
// (~/.claude/settings.json for every session on this computer), pointing at
// this nacred binary. Nothing is written to any settings file: installing
// the hook is the user's own edit.
func printHookSettings(w io.Writer, configPath string) {
	exe, err := os.Executable()
	if err != nil {
		exe = "nacred"
	}
	command := shellQuote(exe) + " hook"
	if configPath != config.DefaultPath() {
		command += " --config " + shellQuote(configPath)
	}
	entry := func(matcher string) []map[string]any {
		e := map[string]any{"hooks": []map[string]any{{"type": "command", "command": command, "timeout": 5}}}
		if matcher != "" {
			e["matcher"] = matcher
		}
		return []map[string]any{e}
	}
	out := map[string]any{"hooks": map[string]any{
		"SessionStart": entry(""),
		"PostToolUse":  entry("Bash"),
		"Stop":         entry(""),
	}}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(out)
}

// shellQuote quotes s for sh when it needs it (the binary often lives under
// "Application Support").
func shellQuote(s string) string {
	safe := s != ""
	for _, r := range s {
		if !(r == '/' || r == '.' || r == '-' || r == '_' || r == '~' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
