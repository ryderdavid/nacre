package daemonupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/launchd"
	"github.com/ryderdavid/nacre/bridge/internal/version"
)

const helperLabel = launchd.Label + ".updater"

func service() string { return "gui/" + strconv.Itoa(os.Getuid()) }
func Supported(home string) string {
	if runtime.GOOS != "darwin" {
		return "Automatic updates require the macOS LaunchAgent installation."
	}
	p := launchd.DefaultPaths(home)
	self, err := os.Executable()
	if err != nil {
		return "Cannot identify this daemon installation."
	}
	self, _ = filepath.EvalSymlinks(self)
	binary, _ := filepath.EvalSymlinks(p.Binary)
	if self == "" || self != binary {
		return "Install nacred as the standard LaunchAgent once to enable updates from the app."
	}
	if _, err = os.Stat(p.Plist); err != nil {
		return "Install the nacred LaunchAgent once to enable updates from the app."
	}
	return ""
}
func NewService(cfg *config.Config, addr string) *Manager {
	return New(Options{Dir: cfg.UpdatesDir(), Version: version.Version, Commit: version.Commit, HostID: cfg.Host().ID, Unavailable: Supported(config.Home()), Launch: func(s Status) error { return launch(cfg, addr, s) }})
}
func launch(cfg *config.Config, addr string, s Status) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// Do not restart an unrelated manually launched process at the standard path.
	b, err := command(ctx, "", "launchctl", "print", service()+"/"+launchd.Label)
	pid := regexp.MustCompile(`(?m)^\s*pid = ([0-9]+)\s*$`).FindSubmatch(b)
	if err != nil || len(pid) != 2 || string(pid[1]) != strconv.Itoa(os.Getpid()) {
		return fmt.Errorf("This daemon is not the active nacred LaunchAgent; install it manually first.")
	}
	// Reject a different --config/--listen service instance before persisting work.
	plist, err := os.ReadFile(launchd.DefaultPaths(config.Home()).Plist)
	if err != nil {
		return err
	}
	if !bytes.Contains(plist, []byte("<string>"+xmlText(launchd.DefaultPaths(config.Home()).Binary)+"</string>")) {
		return fmt.Errorf("LaunchAgent executable does not match this installation")
	}
	dir := cfg.UpdatesDir()
	worker := filepath.Join(dir, "worker")
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if err = copyAtomic(self, worker); err != nil {
		return err
	}
	args := []string{worker, "update-worker", "--config", cfg.Path(), "--listen", addr, s.ID}
	var a strings.Builder
	for _, v := range args {
		a.WriteString("<string>" + xmlText(v) + "</string>")
	}
	p := `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>Label</key><string>` + helperLabel + `</string><key>ProgramArguments</key><array>` + a.String() + `</array><key>RunAtLoad</key><true/><key>EnvironmentVariables</key><dict><key>HOME</key><string>` + xmlText(config.Home()) + `</string><key>PATH</key><string>` + xmlText(os.Getenv("PATH")) + `</string></dict><key>StandardOutPath</key><string>` + xmlText(filepath.Join(dir, "helper.log")) + `</string><key>StandardErrorPath</key><string>` + xmlText(filepath.Join(dir, "helper.log")) + `</string></dict></plist>`
	pp := filepath.Join(dir, "helper.plist")
	if err = os.WriteFile(pp, []byte(p), 0o600); err != nil {
		return err
	}
	_ = exec.CommandContext(ctx, "launchctl", "bootout", service()+"/"+helperLabel).Run()
	if _, err = command(ctx, "", "launchctl", "bootstrap", service(), pp); err != nil {
		return fmt.Errorf("Could not start the independent updater. Check this computer's LaunchAgent.")
	}
	return nil
}
func xmlText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
func copyAtomic(src, dst string) error {
	in, e := os.Open(src)
	if e != nil {
		return e
	}
	defer in.Close()
	if e = os.MkdirAll(filepath.Dir(dst), 0o700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(dst), ".binary-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0o700); e == nil {
		_, e = io.Copy(f, in)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(f.Name(), dst)
}
func RunWorker(cfg *config.Config, addr, id string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("updates require macOS")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	dir := cfg.UpdatesDir()
	p := launchd.DefaultPaths(config.Home())
	backup := filepath.Join(dir, "previous-nacred")
	return Work(ctx, dir, id, Environment{
		Latest: latest, Build: func(ctx context.Context, r Release) (string, error) { return build(ctx, dir, r) },
		Backup: func() error { return copyAtomic(p.Binary, backup) }, Install: func(candidate string) error { return copyAtomic(candidate, p.Binary) },
		Restart: func(ctx context.Context) error {
			_, err := command(ctx, "", "launchctl", "kickstart", "-k", service()+"/"+launchd.Label)
			return err
		},
		Healthy: func(ctx context.Context, v, c, h string) error { return healthy(ctx, addr, cfg.Token, v, c, h) },
		Restore: func() error { return copyAtomic(backup, p.Binary) },
	})
}
func healthy(ctx context.Context, addr, token, v, c, h string) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for {
		req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+"/v1/health", nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err == nil {
			var actual struct {
				Version string `json:"version"`
				Commit  string `json:"commit"`
				HostID  string `json:"host_id"`
			}
			err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&actual)
			resp.Body.Close()
			if err == nil && resp.StatusCode == 200 && actual.Version == v && actual.Commit == c && actual.HostID == h {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("daemon health did not confirm the expected version, commit and computer identity")
		case <-time.After(time.Second):
		}
	}
}
