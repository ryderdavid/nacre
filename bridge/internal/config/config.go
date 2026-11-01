// Package config loads and creates nacred's JSON configuration file
// (~/.config/nacred/config.json by default).
package config

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

// DefaultPort is the TCP port nacred listens on unless configured otherwise.
const DefaultPort = "7420"

// Config is the on-disk configuration. Paths may use a leading "~/".
type Config struct {
	// Listen is host:port. Empty means auto: the Mac's Tailscale IPv4 on
	// DefaultPort, falling back to 127.0.0.1.
	Listen string `json:"listen"`
	// Token is the bearer token clients must present.
	Token string `json:"token"`
	// HostID is a stable random id for this computer, generated on first run,
	// so clients paired with several computers can tell them apart.
	HostID string `json:"host_id"`
	// HostName is the display name shown in the app. Empty means the hostname.
	HostName string `json:"host_name,omitempty"`

	ScanRoots   []string `json:"scan_roots"`
	ScanDepth   int      `json:"scan_depth"`
	Exclude     []string `json:"exclude"`
	ExtraRepos  []string `json:"extra_repos"`
	HiddenRepos []string `json:"hidden_repos"`

	BdPath     string `json:"bd_path"`
	ClaudePath string `json:"claude_path"`
	// CodexPath is the OpenAI Codex CLI, for beads run with Codex.
	CodexPath string `json:"codex_path"`
	// Actor is passed as `bd --actor` on writes.
	Actor string `json:"actor"`

	MaxConcurrentRuns int `json:"max_concurrent_runs"`
	// MaxConcurrentGates bounds how many gate and build commands (nacre.yaml)
	// run at once on this computer, across single runs and rounds. Builds,
	// not agents, are what a machine runs out of. Default 2; a laptop sets 1.
	MaxConcurrentGates int `json:"max_concurrent_gates,omitempty"`
	// RoundBudgetUSD and RoundTimeLimitMinutes are the limits a new round
	// gets unless it sets its own. Defaults 25 and 240.
	RoundBudgetUSD        float64 `json:"round_budget_usd,omitempty"`
	RoundTimeLimitMinutes int     `json:"round_time_limit_minutes,omitempty"`
	DefaultMode           string  `json:"default_mode"`
	DefaultModel          string  `json:"default_model"`
	// PromptTemplate supports {{id}}, {{title}}, {{repo}} and {{instructions}}.
	PromptTemplate string `json:"prompt_template"`

	// AdoptSessions enables watching Claude Code's session registry and
	// Codex's rollout files (~/.codex/sessions) so that sessions an agent
	// started on its own are adopted as read-only runs (NAC-j5z, NAC-j5z.11). Off by default; even when on, adopted runs are only listed
	// to clients that ask for them (GET /v1/runs?include=adopted).
	AdoptSessions bool `json:"adopt_sessions"`
	// AdoptPollSeconds is how often the session registry is polled. 0 uses
	// the watcher's own default.
	AdoptPollSeconds int `json:"adopt_poll_seconds,omitempty"`
	// AdoptIdleMinutes is how long an adopted session may go without writing
	// its transcript before it counts as idle. Past it the session is "stale":
	// still shown on its bead, but left out of the runs list and the adopted
	// count, because a session left open is not a job in progress. Default 15.
	AdoptIdleMinutes int `json:"adopt_idle_minutes"`
	// StallMinutes is how long a run's claude process may print nothing
	// before the run is flagged stalled. Longer than the longest foreground
	// tool call Claude Code allows (10 minutes), so a slow build is not
	// mistaken for a hang. Default 15; negative turns the check off.
	StallMinutes int `json:"stall_minutes,omitempty"`

	// DataDir holds run history. Default ~/Library/Application Support/nacred.
	DataDir string `json:"data_dir,omitempty"`

	path string // file this config was loaded from
}

// DefaultPromptTemplate is the prompt sent to Claude for a new run. When the
// template does not reference {{instructions}}, non-empty instructions are
// appended using InstructionsSuffix.
const DefaultPromptTemplate = "You are working on bead {{id}} (\"{{title}}\") from this repository's beads (bd) issue tracker, dispatched remotely from the Nacre Android app.\n\n" +
	"1. Run `bd show {{id}}` and `bd comments {{id}}` to read the full bead, including design notes and acceptance criteria.\n" +
	"2. Claim it: `bd update {{id}} --claim`.\n" +
	"3. Do the work described, following this repository's CLAUDE.md / AGENTS.md conventions.\n" +
	"4. Verify the acceptance criteria (run tests/builds as appropriate).\n" +
	"5. If complete, close it: `bd close {{id}} --reason \"<one-line summary>\"`. Otherwise leave it in progress and record what remains with `bd update {{id}} --append-notes \"...\"`.\n" +
	"6. Finish with a concise summary of what changed and how it was verified."

// DefaultExclude lists directory basenames (or ~/absolute paths) skipped during discovery.
var DefaultExclude = []string{
	"node_modules", "Library", ".Trash", ".git", ".cache", "build", "dist", "out",
	"target", ".build", "DerivedData", ".gradle", "Pods", "vendor", ".venv", "venv",
	"__pycache__", ".next", "~/.traycer",
}

// DefaultPath returns ~/.config/nacred/config.json.
func DefaultPath() string {
	return filepath.Join(Home(), ".config", "nacred", "config.json")
}

// Home returns the user's home directory.
func Home() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return os.Getenv("HOME")
}

// Expand replaces a leading "~" with the home directory and cleans the path.
func Expand(p string) string {
	if p == "~" {
		return Home()
	}
	if strings.HasPrefix(p, "~/") {
		p = filepath.Join(Home(), p[2:])
	}
	return filepath.Clean(p)
}

// Path returns the file the config was loaded from.
func (c *Config) Path() string { return c.path }

// Load reads the config at path, creating it with defaults (and a fresh random
// token) when it does not exist. Missing fields are filled with defaults in
// memory; the file is only rewritten when it was created or lacked a token.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath()
	}
	path = Expand(path)
	c := &Config{path: path}
	data, err := os.ReadFile(path)
	created := false
	switch {
	case errors.Is(err, os.ErrNotExist):
		created = true
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(data, c); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	needSave := created
	if c.Token == "" {
		tok, err := NewToken()
		if err != nil {
			return nil, err
		}
		c.Token = tok
		needSave = true
	}
	if c.HostID == "" {
		id, err := NewHostID()
		if err != nil {
			return nil, err
		}
		c.HostID = id
		needSave = true
	}
	c.fillDefaults()
	if needSave {
		// On first run this persists the full default set so it is visible and editable.
		if err := c.Save(); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// fillDefaults sets every zero-valued field to its default.
func (c *Config) fillDefaults() {
	if c.ScanRoots == nil {
		c.ScanRoots = []string{"~/Code", "~/Documents", "~/Beads"}
	}
	if c.ScanDepth <= 0 {
		c.ScanDepth = 4
	}
	if c.Exclude == nil {
		c.Exclude = append([]string(nil), DefaultExclude...)
	}
	if c.ExtraRepos == nil {
		c.ExtraRepos = []string{}
	}
	if c.HiddenRepos == nil {
		c.HiddenRepos = []string{}
	}
	if c.BdPath == "" {
		c.BdPath = findBinary("bd", "/opt/homebrew/bin/bd", "/usr/local/bin/bd")
	}
	if c.ClaudePath == "" {
		c.ClaudePath = findBinary("claude", filepath.Join(Home(), ".local/bin/claude"), "/opt/homebrew/bin/claude", "/usr/local/bin/claude")
	}
	if c.CodexPath == "" {
		c.CodexPath = findBinary("codex", "/opt/homebrew/bin/codex", "/usr/local/bin/codex", filepath.Join(Home(), ".local/bin/codex"))
	}
	if c.Actor == "" {
		c.Actor = defaultActor()
	}
	if c.MaxConcurrentRuns <= 0 {
		c.MaxConcurrentRuns = 3
	}
	if c.MaxConcurrentGates <= 0 {
		c.MaxConcurrentGates = 2
	}
	if c.DefaultMode == "" {
		c.DefaultMode = "auto"
	}
	if c.PromptTemplate == "" {
		c.PromptTemplate = DefaultPromptTemplate
	}
	if c.AdoptIdleMinutes <= 0 {
		c.AdoptIdleMinutes = DefaultAdoptIdleMinutes
	}
}

// DefaultAdoptIdleMinutes is the default AdoptIdleMinutes.
const DefaultAdoptIdleMinutes = 15

// AdoptIdleAfter returns the adopted-session idle window.
func (c *Config) AdoptIdleAfter() time.Duration {
	if c.AdoptIdleMinutes <= 0 {
		return DefaultAdoptIdleMinutes * time.Minute
	}
	return time.Duration(c.AdoptIdleMinutes) * time.Minute
}

// DefaultStallMinutes is the default StallMinutes.
const DefaultStallMinutes = 15

// StallAfter returns the stall window, or 0 when the check is off.
func (c *Config) StallAfter() time.Duration {
	switch {
	case c.StallMinutes < 0:
		return 0
	case c.StallMinutes == 0:
		return DefaultStallMinutes * time.Minute
	}
	return time.Duration(c.StallMinutes) * time.Minute
}

// AdoptInterval returns the configured session-registry poll period, or 0 to
// use the watcher's default.
func (c *Config) AdoptInterval() time.Duration {
	if c.AdoptPollSeconds <= 0 {
		return 0
	}
	return time.Duration(c.AdoptPollSeconds) * time.Second
}

// RunsDir is where run history is stored.
func (c *Config) RunsDir() string { return filepath.Join(c.dataDir(), "runs") }

// RoundsDir holds round state.
func (c *Config) RoundsDir() string { return filepath.Join(c.dataDir(), "rounds") }

// ChangesDir holds recoverable OpenSpec workflow journals.
func (c *Config) ChangesDir() string { return filepath.Join(c.dataDir(), "changes") }

func (c *Config) UpdatesDir() string { return filepath.Join(c.dataDir(), "updates") }

// WorktreesDir holds the worktrees rounds work in.
func (c *Config) WorktreesDir() string { return filepath.Join(c.dataDir(), "worktrees") }

// AssistBoardsDir is where a board assistant call that spans several boards
// runs: one symlink per board, read with `bd -C <link>`. It stays put so a
// conversation's later turns can resume the session claude saved there.
func (c *Config) AssistBoardsDir() string { return filepath.Join(c.dataDir(), "assist-boards") }

func (c *Config) dataDir() string {
	d := c.DataDir
	if d == "" {
		d = filepath.Join(Home(), "Library", "Application Support", "nacred")
	}
	return Expand(d)
}

// Save writes the config atomically with 0600 permissions.
func (c *Config) Save() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// NewHostID returns "h_" plus 12 random hex characters.
func NewHostID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "h_" + hex.EncodeToString(b), nil
}

// Host returns this computer's id and display name.
func (c *Config) Host() api.Host {
	name := c.HostName
	if name == "" {
		h, _ := os.Hostname()
		name = strings.TrimSuffix(h, ".local")
	}
	return api.Host{ID: c.HostID, Name: name}
}

// NewToken returns 32 random bytes encoded as unpadded base64url.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func findBinary(name string, candidates ...string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return name
}

func defaultActor() string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "config", "--global", "user.name").Output()
	name := strings.TrimSpace(string(out))
	if err != nil || name == "" {
		return "Nacre"
	}
	return name + " via Nacre"
}

// TailscaleIPv4 returns the Mac's Tailscale IPv4 address, or "" if unavailable.
func TailscaleIPv4() string {
	for _, bin := range []string{"/Applications/Tailscale.app/Contents/MacOS/Tailscale", "tailscale", "/opt/homebrew/bin/tailscale", "/usr/local/bin/tailscale"} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		out, err := exec.CommandContext(ctx, bin, "ip", "-4").Output()
		cancel()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			ip := net.ParseIP(strings.TrimSpace(line))
			if ip != nil && ip.To4() != nil {
				return ip.String()
			}
		}
	}
	return ""
}

// ResolveListen returns the effective listen address. An explicit override
// (flag) wins, then the config value; otherwise the Tailscale IPv4 is used,
// retrying for up to wait (Tailscale may still be starting at login), then
// 127.0.0.1.
func (c *Config) ResolveListen(override string, wait time.Duration) string {
	if override != "" {
		return override
	}
	if c.Listen != "" {
		return c.Listen
	}
	deadline := time.Now().Add(wait)
	for {
		if ip := TailscaleIPv4(); ip != "" {
			return net.JoinHostPort(ip, DefaultPort)
		}
		if time.Now().After(deadline) {
			return net.JoinHostPort("127.0.0.1", DefaultPort)
		}
		time.Sleep(3 * time.Second)
	}
}
