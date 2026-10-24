package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/gates"
	"github.com/ryderdavid/nacre/bridge/internal/repoconfig"
)

// failTailLines is how much of a failing gate's output the terminal shows.
const failTailLines = 40

// runGates is `nacred gates [dir] [--build] [--skip-final] [--only name] [--json]`:
// it runs a repository's nacre.yaml gates the way nacred will when it checks
// work, so an agent or a person can check before handing the work over. It
// exits 1 if any step fails.
func runGates(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("nacred gates", flag.ContinueOnError)
	fs.SetOutput(out)
	build := fs.Bool("build", false, "also run the build steps, as at the end of a round")
	skipFinal := fs.Bool("skip-final", false, "skip final_only gates, as when a single bead is checked")
	only := fs.String("only", "", "run only the named gate or build step")
	asJSON := fs.Bool("json", false, "print the results as JSON")
	// Flags may come before or after the directory.
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			break
		}
		pos, args = append(pos, fs.Arg(0)), fs.Args()[1:]
	}
	dir := "."
	switch len(pos) {
	case 0:
	case 1:
		dir = pos[0]
	default:
		return fmt.Errorf("one directory at most, got %s", strings.Join(pos, " "))
	}
	root, err := repoRoot(dir)
	if err != nil {
		return err
	}
	cfg, err := repoconfig.Load(root)
	if errors.Is(err, repoconfig.ErrNotFound) {
		return fmt.Errorf("%s has no %s", root, repoconfig.FileName)
	}
	if err != nil {
		return err
	}

	var steps []repoconfig.Step
	for _, g := range cfg.Gates {
		if !(*skipFinal && g.FinalOnly) {
			steps = append(steps, g)
		}
	}
	if *build {
		steps = append(steps, cfg.Build...)
	}
	if *only != "" {
		var picked []repoconfig.Step
		for _, s := range append(append([]repoconfig.Step{}, cfg.Gates...), cfg.Build...) {
			if s.Name == *only {
				picked = append(picked, s)
			}
		}
		if len(picked) == 0 {
			return fmt.Errorf("no gate or build step named %q in %s", *only, repoconfig.FileName)
		}
		steps = picked
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	runner := gates.NewRunner(1)
	timeout := time.Duration(cfg.Limits.GateTimeoutMinutes) * time.Minute
	var results []gates.Result
	for _, s := range steps {
		if !*asJSON {
			fmt.Fprintf(out, "▶ %s: %s\n", s.Name, s.Run)
		}
		res := runner.Run(ctx, root, cfg.Env, s, timeout)
		results = append(results, res)
		if !*asJSON {
			report(out, res)
		}
		if ctx.Err() != nil {
			break
		}
	}
	if *asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			return err
		}
	}
	if !gates.AllOK(results) {
		return errGatesFailed
	}
	if !*asJSON {
		fmt.Fprintf(out, "All %d passed.\n", len(results))
	}
	return nil
}

var errGatesFailed = errors.New("gates failed")

func report(out io.Writer, r gates.Result) {
	took := (time.Duration(r.DurationMS) * time.Millisecond).Round(100 * time.Millisecond)
	if r.OK {
		fmt.Fprintf(out, "  ✓ passed in %s\n", took)
		return
	}
	fmt.Fprintf(out, "  ✗ %s (%s)\n", r.Summary(), took)
	lines := strings.Split(r.Tail, "\n")
	if len(lines) > failTailLines {
		lines = lines[len(lines)-failTailLines:]
	}
	for _, l := range lines {
		if l != "" {
			fmt.Fprintln(out, "    "+l)
		}
	}
}

// repoRoot finds the directory holding nacre.yaml at or above dir, stopping
// at the git root, so the command works from anywhere in a repository.
func repoRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for d := abs; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, repoconfig.FileName)); err == nil {
			return d, nil
		}
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil || filepath.Dir(d) == d {
			return abs, nil
		}
	}
}
