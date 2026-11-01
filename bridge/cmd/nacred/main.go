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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mdp/qrterminal/v3"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/assist"
	"github.com/ryderdavid/nacre/bridge/internal/attention"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/daemonupdate"
	"github.com/ryderdavid/nacre/bridge/internal/discovery"
	"github.com/ryderdavid/nacre/bridge/internal/gates"
	"github.com/ryderdavid/nacre/bridge/internal/launchd"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
	"github.com/ryderdavid/nacre/bridge/internal/rounds"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
	"github.com/ryderdavid/nacre/bridge/internal/server"
	"github.com/ryderdavid/nacre/bridge/internal/sessions"
	"github.com/ryderdavid/nacre/bridge/internal/specflow"
	"github.com/ryderdavid/nacre/bridge/internal/sse"
	"github.com/ryderdavid/nacre/bridge/internal/version"
	"github.com/ryderdavid/nacre/bridge/internal/workspace"
)

func main() {
	cmd := "serve"
	args := os.Args[1:]
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}
	if cmd == "gates" {
		// Runs on its own: no config, no server, and flags of its own.
		if err := runGates(args, os.Stdout); err != nil {
			if !errors.Is(err, errGatesFailed) {
				fmt.Fprintln(os.Stderr, "nacred gates:", err)
			}
			os.Exit(1)
		}
		return
	}
	fs := flag.NewFlagSet("nacred "+cmd, flag.ExitOnError)
	configPath := fs.String("config", config.DefaultPath(), "config file path")
	listen := fs.String("listen", "", "listen address host:port (overrides config)")
	jsonVersion := fs.Bool("json", false, "report build identity as JSON (version command)")
	_ = fs.Parse(args)

	if cmd == "version" {
		if *jsonVersion {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"name": version.Name, "version": version.Version, "commit": version.Commit, "update_protocol": 1})
			return
		}
		fmt.Println(version.Name, version.Version, version.Commit)
		return
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal(err)
	}
	switch cmd {
	case "update-worker":
		if len(fs.Args()) != 1 || *listen == "" {
			err = fmt.Errorf("update-worker requires an operation ID and service address")
			break
		}
		err = daemonupdate.RunWorker(cfg, *listen, fs.Args()[0])
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
	case "round":
		err = runRound(cfg, fs.Args(), os.Stdout)
	case "uninstall":
		err = launchd.Uninstall(launchd.DefaultPaths(config.Home()), os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\nusage: nacred [serve|pair|token|version|install|uninstall|gates|round] [--config path] [--listen host:port]\n", cmd)
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
	// Rounds hear about their runs through the run notifications. They are
	// created after the run manager, so the hook is bound late.
	var roundsRef atomic.Pointer[rounds.Manager]
	manager, err := runs.New(runs.Options{
		Dir: cfg.RunsDir(), ClaudePath: cfg.ClaudePath, CodexPath: cfg.CodexPath, MaxConcurrent: cfg.MaxConcurrentRuns,
		AdoptIdleAfter: cfg.AdoptIdleAfter(),
		StallAfter:     cfg.StallAfter(),
		Notify: func(event string, data any) {
			broker.Publish(event, data)
			if run, ok := data.(api.Run); ok {
				if rm := roundsRef.Load(); rm != nil {
					rm.OnRun(run)
				}
			}
		},
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
	registry.AdoptedRuns = manager.AdoptedCount
	go registry.Rescan() // warm the repo list

	assistant := &assist.Runner{ClaudePath: cfg.ClaudePath}
	changes, err := specflow.New(specflow.Options{
		Dir: cfg.ChangesDir(), Beads: client, Headless: assistant,
		Active: func(path, changeID string) string {
			return activeChange(path, changeID, roundsRef.Load(), manager)
		},
		Evidence: func(path string, change specflow.Change) []specflow.Evidence {
			return changeEvidence(path, change, roundsRef.Load(), manager, client)
		},
		Notify: func(path string) {
			for _, ref := range registry.Refs() {
				if sameRepoPath(ref.Path, path) {
					registry.Invalidate(ref.ID)
					broker.Publish("repo", map[string]string{"repo_id": ref.ID})
				}
			}
		},
		EvidenceStamp: func(path string) string {
			return roundsStamp(path, roundsRef.Load())
		},
		Changed: func(path string, c specflow.Change) {
			for _, ref := range registry.Refs() {
				if sameRepoPath(ref.Path, path) {
					broker.Publish("change", changeEvent(ref.ID, c))
				}
			}
		},
	})
	if err != nil {
		return fmt.Errorf("load specification workflows: %w", err)
	}
	defer changes.Shutdown(5 * time.Second)
	workspaces := &workspace.Manager{Root: cfg.WorktreesDir()}
	roundsMgr, err := rounds.New(rounds.Options{
		SpecContract: func(ctx context.Context, path, beadID string) (*api.SpecContract, error) {
			contract, err := changes.Contract(ctx, path, beadID)
			if err != nil || contract == nil {
				return nil, err
			}
			return executionContract(*contract), nil
		},
		Requirements:   specflow.RequirementIndex,
		ArchiveChange:  changes.ArchiveInto,
		ChangeArchived: changes.MarkArchived,
		Develop: func(ctx context.Context, path, beadID, feedback string) error {
			_, err := changes.Act(ctx, path, beadID, specflow.Action{Action: "develop", Feedback: feedback})
			return err
		},
		Dir: cfg.RoundsDir(), Runs: manager, Beads: client, Headless: assistant,
		Gates: gates.NewRunner(cfg.MaxConcurrentGates), Workspaces: workspaces,
		Repo: func(id string) (string, string, error) {
			rp, err := registry.Summary(id)
			if err != nil {
				return "", "", err
			}
			return rp.Name, rp.Path, nil
		},
		Mode: cfg.DefaultMode, Model: cfg.DefaultModel,
		BudgetUSD: cfg.RoundBudgetUSD, TimeLimitMinutes: cfg.RoundTimeLimitMinutes,
		Notify: broker.Publish,
		RepoChanged: func(repoID string) {
			registry.Invalidate(repoID)
			broker.Publish("repo", map[string]string{"repo_id": repoID})
		},
		Logger: logger,
	})
	if err != nil {
		return fmt.Errorf("load rounds: %w", err)
	}
	roundsRef.Store(roundsMgr)
	roundsMgr.Start()
	go sweepWorktrees(workspaces, registry, roundsMgr, logger)
	needsYou := &attention.Builder{
		Repos: registry.Refs, Issues: client.List, Comments: client.Comments,
		// Ended rounds too: their spec beads' Found: comments still wait on Ryder.
		Rounds: func() []api.Round { return roundsMgr.List("", false) },
		Changes: func(path string) ([]api.ChangeSummary, error) {
			// Approvals that went stale for any reason are named before listing.
			changes.Refresh(context.Background(), path)
			list, _, err := changes.Summaries(path)
			return list, err
		},
		Working: func(repoID, beadID string) bool {
			if len(manager.List(runs.Filter{RepoID: repoID, IssueID: beadID, ActiveOnly: true, IncludeAdopted: true})) > 0 {
				return true
			}
			for _, r := range roundsMgr.List(repoID, true) {
				for _, b := range r.Beads {
					if b.ID == beadID {
						return true
					}
				}
			}
			return false
		},
	}
	go invalidateAttention(broker, registry, needsYou)
	srv := &server.Server{Config: cfg, Repos: registry, BD: client, Runs: manager, Assist: assistant, Rounds: roundsMgr, Changes: changes, Attention: needsYou, Broker: broker, Log: logger}
	srv.Updates = daemonupdate.NewService(cfg, addr)
	// Cancelled on shutdown so long-lived SSE handlers return promptly.
	baseCtx, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()
	if cfg.AdoptSessions {
		go adoptSessions(baseCtx, cfg, registry, manager, logger)
	} else {
		// End adopted runs left active by an earlier start that had adoption
		// enabled; nothing will report their sessions as finished now.
		manager.SyncAdopted(nil)
	}
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
	logger.Printf("%s %s (%s) listening on http://%s (config %s, bd %s, claude %s)", version.Name, version.Version, version.Commit, addr, cfg.Path(), cfg.BdPath, cfg.ClaudePath)

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
	roundsMgr.Shutdown(5 * time.Second)
	manager.Shutdown(8 * time.Second)
	shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

// adoptSessions watches Claude Code's session registry and Codex's rollout
// files so that sessions an agent started on its own show up as read-only
// adopted runs (NAC-j5z, NAC-j5z.11).
func adoptSessions(ctx context.Context, cfg *config.Config, registry *repos.Registry, manager *runs.Manager, logger *log.Logger) {
	w := sessions.New(sessions.Options{
		Dir:           sessions.DefaultDir(config.Home()),
		Projects:      sessions.DefaultProjectsDir(config.Home()),
		Interval:      cfg.AdoptInterval(),
		OwnNamePrefix: claude.NamePrefix,
		Repos: func() []sessions.Repo {
			refs := registry.Refs()
			out := make([]sessions.Repo, len(refs))
			for i, ref := range refs {
				out[i] = sessions.Repo(ref)
			}
			return out
		},
		Beads: func(repoID string) []sessions.Bead {
			_, issues, err := registry.Issues(repoID)
			if err != nil {
				return nil
			}
			return sessions.ParseBeads(issues)
		},
		Owned: manager.OwnsSession,
		Sync:  manager.SyncAdopted,
		// A Codex session has no pid, so it ends once its rollout has been
		// quiet for twice the idle window: idle and stale first, then ended.
		CodexHome:       sessions.DefaultCodexHome(config.Home()),
		CodexQuietAfter: 2 * cfg.AdoptIdleAfter(),
		Logger:          logger,
	})
	logger.Printf("adopting agent-owned Claude Code sessions from %s and Codex sessions from %s", sessions.DefaultDir(config.Home()), sessions.DefaultCodexHome(config.Home()))
	w.Run(ctx)
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

// sweepWorktrees removes worktrees left by rounds that have landed or no
// longer exist, once the repo list is known.
func sweepWorktrees(ws *workspace.Manager, registry *repos.Registry, rm *rounds.Manager, logger *log.Logger) {
	paths := map[string]string{}
	for _, rp := range registry.List(false) {
		paths[rp.ID] = rp.Path
	}
	for _, dir := range ws.Sweep(context.Background(), paths, rm.Alive) {
		logger.Printf("rounds: removed leftover worktree %s", dir)
	}
}
