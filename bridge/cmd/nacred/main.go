// Command nacred is the Mac-side bridge for the Nacre Android app: it serves
// beads data (via the bd CLI) and runs Claude Code headless over HTTP+SSE.
//
// Usage:
//
//	nacred [serve] [--listen host:port] [--config path]
//	nacred pair | token | version | install | uninstall [--config path]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mdp/qrterminal/v3"

	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/discovery"
	"github.com/ryderdavid/nacre/bridge/internal/launchd"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
	"github.com/ryderdavid/nacre/bridge/internal/server"
	"github.com/ryderdavid/nacre/bridge/internal/sse"
	"github.com/ryderdavid/nacre/bridge/internal/version"
)

func main() {
	cmd := "serve"
	args := os.Args[1:]
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("nacred "+cmd, flag.ExitOnError)
	configPath := fs.String("config", config.DefaultPath(), "config file path")
	listen := fs.String("listen", "", "listen address host:port (overrides config)")
	_ = fs.Parse(args)

	if cmd == "version" {
		fmt.Println(version.Name, version.Version)
		return
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal(err)
	}
	switch cmd {
	case "serve":
		err = serve(cfg, *listen)
	case "pair":
		err = pair(cfg, *listen)
	case "token":
		fmt.Println(cfg.Token)
	case "install":
		cp := cfg.Path()
		if cp == config.DefaultPath() {
			cp = "" // keep the plist minimal when using the default
		}
		err = launchd.Install(launchd.DefaultPaths(config.Home()), cp, os.Stdout)
	case "uninstall":
		err = launchd.Uninstall(launchd.DefaultPaths(config.Home()), os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\nusage: nacred [serve|pair|token|version|install|uninstall] [--config path] [--listen host:port]\n", cmd)
		os.Exit(2)
	}
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "nacred:", err)
	os.Exit(1)
}

func scanner(cfg *config.Config) repos.Scanner {
	return func() []string {
		expand := func(in []string) []string {
			out := make([]string, len(in))
			for i, p := range in {
				out[i] = config.Expand(p)
			}
			return out
		}
		return discovery.Scan(discovery.Options{
			Roots: expand(cfg.ScanRoots), Depth: cfg.ScanDepth, Home: config.Home(),
			Exclude: cfg.Exclude, Extra: expand(cfg.ExtraRepos), Hidden: expand(cfg.HiddenRepos),
		})
	}
}

func serve(cfg *config.Config, listenFlag string) error {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	addr := cfg.ResolveListen(listenFlag, 60*time.Second)

	broker := sse.NewBroker()
	client := bd.New(cfg.BdPath, cfg.Actor)
	registry := repos.New(scanner(cfg), client)
	manager, err := runs.New(runs.Options{
		Dir: cfg.RunsDir(), ClaudePath: cfg.ClaudePath, MaxConcurrent: cfg.MaxConcurrentRuns,
		Notify: broker.Publish,
		RepoChanged: func(repoID string) {
			registry.Invalidate(repoID)
			broker.Publish("repo", map[string]string{"repo_id": repoID})
		},
		Logger: logger,
	})
	if err != nil {
		return fmt.Errorf("load runs: %w", err)
	}
	registry.ActiveRuns = manager.ActiveCount
	go registry.Rescan() // warm the repo list

	srv := &server.Server{Config: cfg, Repos: registry, BD: client, Runs: manager, Broker: broker, Log: logger}
	// Cancelled on shutdown so long-lived SSE handlers return promptly.
	baseCtx, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return baseCtx },
		// No WriteTimeout: SSE responses are long-lived.
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	logger.Printf("%s %s listening on http://%s (config %s, bd %s, claude %s)", version.Name, version.Version, addr, cfg.Path(), cfg.BdPath, cfg.ClaudePath)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.Serve(ln) }()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	logger.Printf("shutting down")
	cancelBase()
	manager.Shutdown(8 * time.Second)
	shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

// pair prints the pairing deep link as a terminal QR code and as text.
func pair(cfg *config.Config, listenFlag string) error {
	addr := cfg.ResolveListen(listenFlag, 0)
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		if ts := config.TailscaleIPv4(); ts != "" {
			host = ts
		}
	}
	base := "http://" + net.JoinHostPort(host, port)
	h := cfg.Host()
	link := "nacre://pair?url=" + url.QueryEscape(base) + "&token=" + url.QueryEscape(cfg.Token) +
		"&name=" + url.QueryEscape(h.Name) + "&host_id=" + url.QueryEscape(h.ID)

	qrterminal.GenerateWithConfig(link, qrterminal.Config{
		Level: qrterminal.L, Writer: os.Stdout, HalfBlocks: true,
		BlackChar: qrterminal.BLACK_BLACK, WhiteChar: qrterminal.WHITE_WHITE,
		BlackWhiteChar: qrterminal.BLACK_WHITE, WhiteBlackChar: qrterminal.WHITE_BLACK,
		QuietZone: 2,
	})
	fmt.Printf("\nScan with Nacre, or enter manually:\n  URL:   %s\n  Token: %s\n\n  %s\n", base, cfg.Token, link)
	return nil
}
