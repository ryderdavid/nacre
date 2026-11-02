// Package launchd installs nacred as a per-user LaunchAgent.
package launchd

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// Label is the LaunchAgent label.
const Label = "com.ryderdavid.nacred"

// Paths are the files an install touches.
type Paths struct {
	Home   string
	Binary string // installed binary, ~/.local/bin/nacred
	Plist  string // ~/Library/LaunchAgents/<Label>.plist
	Log    string // ~/Library/Logs/nacred.log
}

// DefaultPaths derives the standard locations from home.
func DefaultPaths(home string) Paths {
	return Paths{
		Home:   home,
		Binary: filepath.Join(home, ".local", "bin", "nacred"),
		Plist:  filepath.Join(home, "Library", "LaunchAgents", Label+".plist"),
		Log:    filepath.Join(home, "Library", "Logs", "nacred.log"),
	}
}

// Plist renders the LaunchAgent property list. configPath may be empty.
func Plist(p Paths, configPath string) []byte {
	args := []string{p.Binary, "serve"}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}
	// ~/.local/bin first, as in Ryder's shell: tools installed both there and
	// by Homebrew (openspec, node) must be the same versions the daemon's
	// gates run as the ones he runs by hand.
	pathEnv := fmt.Sprintf("%s/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin", p.Home)
	esc := func(s string) string {
		var b bytes.Buffer
		_ = xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>` + Label + `</string>
  <key>ProgramArguments</key>
  <array>
`)
	for _, a := range args {
		b.WriteString("    <string>" + esc(a) + "</string>\n")
	}
	b.WriteString(`  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>` + esc(pathEnv) + `</string>
    <key>HOME</key>
    <string>` + esc(p.Home) + `</string>
  </dict>
  <key>WorkingDirectory</key>
  <string>` + esc(p.Home) + `</string>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ProcessType</key>
  <string>Interactive</string>
  <key>StandardOutPath</key>
  <string>` + esc(p.Log) + `</string>
  <key>StandardErrorPath</key>
  <string>` + esc(p.Log) + `</string>
</dict>
</plist>
`)
	return b.Bytes()
}

func domain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

// Install copies the running executable to p.Binary, writes the plist and
// (re)bootstraps the agent.
func Install(p Paths, configPath string, out io.Writer) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if self, err = filepath.EvalSymlinks(self); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.Binary), 0o755); err != nil {
		return err
	}
	if self != p.Binary {
		if err := copyFile(self, p.Binary); err != nil {
			return fmt.Errorf("install binary: %w", err)
		}
		fmt.Fprintf(out, "installed %s\n", p.Binary)
	}
	for _, d := range []string{filepath.Dir(p.Plist), filepath.Dir(p.Log)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	// Unload any previous instance; ignore "not loaded" failures.
	_ = exec.Command("launchctl", "bootout", domain()+"/"+Label).Run()
	if err := os.WriteFile(p.Plist, Plist(p, configPath), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote %s\n", p.Plist)
	if err := bootstrap(p.Plist); err != nil {
		return err
	}
	fmt.Fprintf(out, "loaded %s (logs: %s)\n", Label, p.Log)
	return nil
}

// bootstrap loads the agent. bootout returns before launchd has finished
// removing the old instance, and loading meanwhile fails with error 5, so wait
// for it to go and retry a few times rather than leave the daemon unloaded.
func bootstrap(plist string) error {
	var last error
	for attempt := 0; attempt < 8; attempt++ {
		for i := 0; i < 20 && exec.Command("launchctl", "print", domain()+"/"+Label).Run() == nil; i++ {
			time.Sleep(250 * time.Millisecond)
		}
		b, err := exec.Command("launchctl", "bootstrap", domain(), plist).CombinedOutput()
		if err == nil {
			return nil
		}
		last = fmt.Errorf("launchctl bootstrap: %v: %s", err, bytes.TrimSpace(b))
		time.Sleep(time.Second)
	}
	return last
}

// Uninstall unloads the agent and removes the plist (the binary is kept).
func Uninstall(p Paths, out io.Writer) error {
	if b, err := exec.Command("launchctl", "bootout", domain()+"/"+Label).CombinedOutput(); err != nil {
		fmt.Fprintf(out, "launchctl bootout: %s\n", bytes.TrimSpace(b))
	}
	if err := os.Remove(p.Plist); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Fprintf(out, "removed %s (binary left at %s)\n", p.Plist, p.Binary)
	return nil
}

// copyFile copies src to dst via a temp file + rename so a running binary is
// replaced atomically.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
