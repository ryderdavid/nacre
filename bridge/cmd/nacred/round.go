package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/gitx"
	"github.com/ryderdavid/nacre/bridge/internal/workspace"
)

const roundUsage = `usage: nacred round <command>
  start [--repo dir|name] [--epic ID] [--budget USD] [--minutes N] BEAD...
                         start a round on this computer's nacred
                         linked sections require --spec-change ID --spec-revision REV
  status [ROUND]         list active rounds, or show one in detail
  answer ROUND QUESTION ANSWER...
  land ROUND             fast-forward the target and push (as nacre.yaml says)
  cancel ROUND | resume ROUND | retry ROUND BEAD
  limits ROUND [--budget USD] [--minutes N]
  choose ROUND CHOICE [--budget USD] [--minutes N]
                         take one of a stopped round's choices (see status)
  gc                     remove worktrees of landed, cancelled or missing rounds`

// runRound is `nacred round …`, a client of the local nacred so a Claude
// Code session in a terminal can hand a round to it.
func runRound(cfg *config.Config, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(roundUsage)
	}
	cmd, args := args[0], args[1:]
	if cmd == "gc" {
		return roundGC(cfg, out)
	}
	c := &roundClient{base: "http://" + cfg.ResolveListen("", 3*time.Second), token: cfg.Token}
	switch cmd {
	case "start":
		return c.start(args, out)
	case "status":
		if len(args) == 0 {
			return c.list(out)
		}
		return c.show(args[0], out)
	case "answer":
		if len(args) < 3 {
			return errors.New("usage: nacred round answer ROUND QUESTION ANSWER...")
		}
		return c.post("/v1/rounds/"+args[0]+"/answers", map[string]string{"question_id": args[1], "answer": strings.Join(args[2:], " ")}, out)
	case "land", "cancel", "resume":
		if len(args) != 1 {
			return fmt.Errorf("usage: nacred round %s ROUND", cmd)
		}
		return c.post("/v1/rounds/"+args[0]+"/"+cmd, map[string]string{}, out)
	case "retry":
		if len(args) != 2 {
			return errors.New("usage: nacred round retry ROUND BEAD")
		}
		return c.post("/v1/rounds/"+args[0]+"/beads/"+args[1]+"/retry", map[string]string{}, out)
	case "choose":
		if len(args) < 2 {
			return errors.New("usage: nacred round choose ROUND CHOICE [--budget USD] [--minutes N]")
		}
		fs := flag.NewFlagSet("nacred round choose", flag.ContinueOnError)
		budget := fs.Float64("budget", 0, "extra budget for retry-with-budget, or the new budget for raise-limits")
		minutes := fs.Int("minutes", 0, "new time limit in minutes")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		return c.post("/v1/rounds/"+args[0]+"/choose", map[string]any{"choice": args[1], "budget_usd": *budget, "time_limit_minutes": *minutes}, out)
	case "limits":
		if len(args) < 1 {
			return errors.New("usage: nacred round limits ROUND [--budget USD] [--minutes N]")
		}
		fs := flag.NewFlagSet("nacred round limits", flag.ContinueOnError)
		budget := fs.Float64("budget", 0, "budget in USD")
		minutes := fs.Int("minutes", 0, "time limit in minutes")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return c.post("/v1/rounds/"+args[0]+"/limits", map[string]any{"budget_usd": *budget, "time_limit_minutes": *minutes}, out)
	}
	return errors.New(roundUsage)
}

type roundClient struct {
	base, token string
}

func (c *roundClient) do(method, path string, body any, v any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, c.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("can't reach nacred at %s: %w", c.base, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var e api.Error
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return errors.New(e.Error)
		}
		return fmt.Errorf("nacred answered %s", resp.Status)
	}
	if v != nil {
		return json.Unmarshal(data, v)
	}
	return nil
}

func (c *roundClient) start(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("nacred round start", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repository directory, name or id")
	epic := fs.String("epic", "", "start from this epic's open children")
	budget := fs.Float64("budget", 0, "budget in USD (default from nacred's config)")
	minutes := fs.Int("minutes", 0, "time limit in minutes")
	specChange := fs.String("spec-change", "", "approved specification workflow id")
	specRevision := fs.String("spec-revision", "", "exact approved specification revision")
	var beads []string
	for {
		if err := fs.Parse(args); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			break
		}
		beads, args = append(beads, fs.Arg(0)), fs.Args()[1:]
	}
	if len(beads) == 0 && *epic == "" {
		return errors.New("name the beads to work on, or --epic")
	}
	repoID, name, err := c.findRepo(*repo)
	if err != nil {
		return err
	}
	var r api.Round
	if err := c.do("POST", "/v1/repos/"+repoID+"/rounds", map[string]any{
		"beads": beads, "epic": *epic, "budget_usd": *budget, "time_limit_minutes": *minutes,
		"spec_change_id": *specChange, "spec_revision": *specRevision,
	}, &r); err != nil {
		return err
	}
	fmt.Fprintf(out, "Round %s started in %s with %d bead(s), budget $%.2f, from %s at %s.\n", r.ID, name, len(r.Beads), r.BudgetUSD, r.Target, shortSHA(r.Base))
	fmt.Fprintf(out, "Follow it with `nacred round status %s`, or in the Nacre app.\n", r.ID)
	return nil
}

// findRepo resolves a directory, name or id to a repo nacred knows.
func (c *roundClient) findRepo(ref string) (id, name string, err error) {
	var list struct {
		Repos []api.Repo `json:"repos"`
	}
	if err := c.do("GET", "/v1/repos", nil, &list); err != nil {
		return "", "", err
	}
	dir := ""
	if abs, err := filepath.Abs(ref); err == nil {
		if top, err := gitx.Run(context.Background(), abs, "rev-parse", "--path-format=absolute", "--git-common-dir"); err == nil {
			dir = filepath.Dir(top) // the main checkout, even from a worktree
		}
	}
	for _, r := range list.Repos {
		if r.ID == ref || strings.EqualFold(r.Name, ref) || (dir != "" && samePath(r.Path, dir)) {
			return r.ID, r.Name, nil
		}
	}
	return "", "", fmt.Errorf("nacred doesn't know a repository %q; give --repo a directory, name or id it lists", ref)
}

func samePath(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}

func (c *roundClient) list(out io.Writer) error {
	var list struct {
		Rounds []api.Round `json:"rounds"`
	}
	if err := c.do("GET", "/v1/rounds?active=1", nil, &list); err != nil {
		return err
	}
	if len(list.Rounds) == 0 {
		fmt.Fprintln(out, "No active rounds.")
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ROUND\tREPO\tSTATE\tBEADS\tSPENT")
	for _, r := range list.Rounds {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t$%.2f of $%.2f\n", r.ID, r.RepoName, r.State, beadCounts(r.Beads), r.SpentUSD, r.BudgetUSD)
	}
	return tw.Flush()
}

func (c *roundClient) show(id string, out io.Writer) error {
	var got struct {
		Round  api.Round        `json:"round"`
		Events []api.RoundEvent `json:"events"`
	}
	if err := c.do("GET", "/v1/rounds/"+id, nil, &got); err != nil {
		return err
	}
	r := got.Round
	fmt.Fprintf(out, "Round %s in %s: %s\n", r.ID, r.RepoName, r.State)
	if r.PauseReason != "" {
		fmt.Fprintf(out, "  %s\n", r.PauseReason)
	}
	if s := r.Stop; s != nil {
		if d := s.Diagnosis; d != nil {
			fmt.Fprintf(out, "  Diagnosis: %s\n", d.Summary)
			if d.Cause != "" {
				fmt.Fprintf(out, "  Cause: %s\n", d.Cause)
			}
		}
		if len(s.Attempted) > 0 {
			fmt.Fprintf(out, "  Already tried: %s\n", strings.Join(s.Attempted, ", "))
		}
		for _, b := range s.Spend {
			done := ""
			if b.Done {
				done = " (done)"
			}
			fmt.Fprintf(out, "  %s spent $%.2f%s\n", b.Bead, b.SpentUSD, done)
		}
		if s.EstimateUSD > 0 {
			fmt.Fprintf(out, "  The unfinished beads need about $%.2f more.\n", s.EstimateUSD)
		}
		fmt.Fprintln(out, "  Choices:")
		for _, ch := range s.Choices {
			fmt.Fprintf(out, "    nacred round choose %s %s", r.ID, ch.ID)
			switch ch.ID {
			case api.ChoiceRetryWithBudget:
				fmt.Fprint(out, " [--budget USD]")
			case api.ChoiceRaiseLimits:
				fmt.Fprint(out, " --budget USD | --minutes N")
			}
			fmt.Fprintf(out, "\n      %s", ch.Label)
			if ch.Explanation != "" {
				fmt.Fprintf(out, ": %s", ch.Explanation)
			}
			fmt.Fprintln(out)
		}
	}
	fmt.Fprintf(out, "Spent $%.2f of $%.2f. Integration branch %s.\n", r.SpentUSD, r.BudgetUSD, r.Branch)
	if r.FinalCommit != "" {
		fmt.Fprintf(out, "Final commit %s, version %s.\n", shortSHA(r.FinalCommit), r.Version)
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "\nBEAD\tSTATE\tNOTE")
	for _, b := range r.Beads {
		note := b.Reason
		if note == "" && len(b.After) > 0 && b.State == api.BeadWaiting {
			note = "after " + strings.Join(b.After, ", ")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", b.ID, b.State, note)
	}
	tw.Flush()
	for _, q := range r.Questions {
		if q.Answer == "" {
			fmt.Fprintf(out, "\nQuestion %s (%s): %s\n", q.ID, strings.Join(q.Beads, ", "), q.Text)
			for _, o := range q.Options {
				fmt.Fprintf(out, "  - %s\n", o)
			}
			fmt.Fprintf(out, "Answer with: nacred round answer %s %s <answer>\n", r.ID, q.ID)
		}
	}
	evs := got.Events
	if len(evs) > 12 {
		evs = evs[len(evs)-12:]
	}
	fmt.Fprintln(out, "\nRecent:")
	for _, e := range evs {
		who := ""
		if e.Bead != "" {
			who = e.Bead + ": "
		}
		fmt.Fprintf(out, "  %s %s%s\n", clock(e.TS), who, e.Text)
	}
	return nil
}

func (c *roundClient) post(path string, body any, out io.Writer) error {
	var r api.Round
	if err := c.do("POST", path, body, &r); err != nil {
		return err
	}
	fmt.Fprintf(out, "Round %s: %s\n", r.ID, r.State)
	return nil
}

// roundGC removes worktrees of rounds that no longer need them. It reads the
// rounds' files directly, so it works while nacred is stopped.
func roundGC(cfg *config.Config, out io.Writer) error {
	entries, _ := os.ReadDir(cfg.RoundsDir())
	paths, live := map[string]string{}, map[string]bool{}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(cfg.RoundsDir(), e.Name(), "round.json"))
		if err != nil {
			continue
		}
		var r api.Round
		if json.Unmarshal(b, &r) != nil {
			continue
		}
		paths[r.RepoID] = r.RepoPath
		live[r.ID] = r.Active()
	}
	ws := &workspace.Manager{Root: cfg.WorktreesDir()}
	removed := ws.Sweep(context.Background(), paths, func(id string) bool { return live[id] })
	sort.Strings(removed)
	for _, d := range removed {
		fmt.Fprintln(out, "removed", d)
	}
	fmt.Fprintf(out, "%d worktree folder(s) removed; branches are kept.\n", len(removed))
	return nil
}

func beadCounts(beads []api.RoundBead) string {
	n := map[string]int{}
	for _, b := range beads {
		n[b.State]++
	}
	var parts []string
	for _, s := range []string{api.BeadIntegrated, api.BeadIntegrating, api.BeadVerifying, api.BeadRunning, api.BeadWaiting, api.BeadParked, api.BeadSkipped} {
		if n[s] > 0 {
			parts = append(parts, strconv.Itoa(n[s])+" "+s)
		}
	}
	return strings.Join(parts, ", ")
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

func clock(ts string) string {
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t.Local().Format("15:04")
	}
	return ts
}
