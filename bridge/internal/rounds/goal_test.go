package rounds

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/gitx"
	"github.com/ryderdavid/nacre/bridge/internal/prompt"
	"github.com/ryderdavid/nacre/bridge/internal/repoconfig"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
)

// Every worker has a native goal scoped to its own bead and commits in its
// own workspace. Repository policy alone controls the later push.
func TestGoalRoundFansOutAndFollowsRepositoryPushPolicy(t *testing.T) {
	for _, push := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "push"}[push], func(t *testing.T) {
			beads := newFakeBeads(&fakeBead{ID: "T-A", Title: "First", Type: "feature"}, &fakeBead{ID: "T-B", Title: "Second"})
			repo := testRepo(t)
			yaml, _ := os.ReadFile(filepath.Join(repo, "nacre.yaml"))
			if push {
				if err := os.WriteFile(filepath.Join(repo, "nacre.yaml"), []byte(strings.Replace(string(yaml), "push: false", "push: true", 1)), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := gitx.CommitAll(context.Background(), repo, "Configure landing"); err != nil && push {
				t.Fatal(err)
			}
			script := `#!/bin/sh
set -e
prompt="$2"
bead=$(printf '%s\n' "$prompt" | sed -n 's/^You are working on bead \([^ ]*\) .*/\1/p')
echo '{"type":"system","subtype":"init","session_id":"'"$bead"'","cwd":"'"$PWD"'"}'
printf '%s' "$prompt" > "$bead-launch.txt"
echo done > "$bead.txt"
git add -A
git commit -q -m "Implement $bead"
echo '{"type":"result","subtype":"success","is_error":false,"session_id":"'"$bead"'","total_cost_usd":0.1}'
`
			h := newHarnessWith(t, beads, emptyPlan, harnessOpts{repo: repo, claudeScript: script})
			ctx := context.Background()
			remote := t.TempDir()
			if _, err := gitx.Run(ctx, remote, "init", "--bare", "-q"); err != nil {
				t.Fatal(err)
			}
			if _, err := gitx.Run(ctx, repo, "remote", "add", "origin", remote); err != nil {
				t.Fatal(err)
			}
			// Deliberately request the opposite push policy. Goal options
			// are accepted for compatibility but never grant git authority.
			r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-A", "T-B"}, Goal: api.GoalOptions{Enabled: push, Pushed: !push}})
			if err != nil {
				t.Fatal(err)
			}
			done := h.wait(t, r.ID, api.RoundReadyToLand, api.RoundPaused)
			if done.State != api.RoundReadyToLand || len(done.RunIDs) != 2 || done.GoalRunID != "" {
				t.Fatalf("round: %+v", done)
			}
			for i, b := range done.Beads {
				run, err := h.runs.Run(b.RunID)
				if err != nil || run.GoalText == "" || run.Kind != "bead" || run.Verification == nil || !run.Verification.OK {
					t.Fatalf("worker: %+v %v", run, err)
				}
				if b.Workspace == done.Workspace || b.Workspace == done.Beads[1-i].Workspace || b.Branch == done.Beads[1-i].Branch {
					t.Fatalf("shared workspace: %+v", done.Beads)
				}
				other := done.Beads[1-i].ID
				if strings.Contains(run.GoalText, other+":") || !strings.Contains(run.GoalText, "Do "+b.Title+".") {
					t.Fatalf("wrong goal scope: %s", run.GoalText)
				}
				launch, err := os.ReadFile(filepath.Join(done.Workspace, b.ID+"-launch.txt"))
				if err != nil || !strings.HasPrefix(string(launch), "/goal ") || !strings.Contains(string(launch), "committed on the round branch") {
					t.Fatalf("launch: %s %v", launch, err)
				}
			}
			if len(h.m.leases.holders()) != 0 {
				t.Fatal("resource lease leaked")
			}
			if _, err := h.m.Land(r.ID); err != nil {
				t.Fatal(err)
			}
			landed := h.wait(t, r.ID, api.RoundLanded, api.RoundPaused)
			if landed.State != api.RoundLanded || landed.Pushed != push {
				t.Fatalf("land: %+v", landed)
			}
			sha, err := gitx.RevParse(ctx, remote, "refs/heads/main")
			if push && (err != nil || sha != landed.LandedCommit) {
				t.Fatalf("remote: %s %v", sha, err)
			}
			if !push && err == nil {
				t.Fatal("pushed against repository policy")
			}
		})
	}
}

func TestGoalRoundExplicitRetryResumesItsSession(t *testing.T) {
	script := `#!/bin/sh
set -e
resume=no
for a in "$@"; do [ "$a" = --resume ] && resume=yes; done
echo '{"type":"system","subtype":"init","session_id":"goal-session","cwd":"'"$PWD"'"}'
if [ "$resume" = no ]; then
 echo '{"type":"result","is_error":true,"result":"temporary blocker","session_id":"goal-session"}'
else
 echo a > a.txt
 git add a.txt
 git commit -q -m 'Finish round goal'
 echo '{"type":"result","is_error":false,"result":"verified","session_id":"goal-session"}'
fi
`
	h := newHarnessWith(t, newFakeBeads(&fakeBead{ID: "T-A", Title: "First", Type: "feature"}), `{"beads":[{"id":"T-A"}],"findings":[]}`, harnessOpts{claudeScript: script})
	r, err := startLegacyGoal(t, h, "T-A")
	if err != nil {
		t.Fatal(err)
	}
	paused := h.wait(t, r.ID, api.RoundPaused)
	if h.beads.bead("T-A").Status == "closed" {
		t.Fatal("closed incomplete goal")
	}
	if _, err := h.m.Choose(r.ID, api.ChoiceRetryWithBudget, 1, 0); err != nil {
		t.Fatal(err)
	}
	done := h.wait(t, r.ID, api.RoundReadyToLand)
	run, _ := h.runs.Run(done.GoalRunID)
	if done.GoalRunID != paused.GoalRunID || run.Turns != 2 {
		t.Fatalf("retry lost its session: %+v", done)
	}
	if paused.Version != "" || done.Version == "" {
		t.Fatalf("version %q before the session finished, %q after; want nacred to bump once it did", paused.Version, done.Version)
	}
	if n, _ := gitx.Run(context.Background(), done.Workspace, "log", "--oneline", "--fixed-strings", "--grep", "for round "+done.ID+": ", done.Base+"..HEAD"); strings.Count(n, "\n") != 0 || n == "" {
		t.Fatalf("want exactly one bump commit, got:\n%s", n)
	}
	// A persisted legacy session must also land after main moved, undoing
	// and redoing nacred's bump as before.
	if err := os.WriteFile(filepath.Join(h.repo, "VERSION"), []byte("0.2.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := gitx.CommitAll(context.Background(), h.repo, "Release on main"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.m.Land(done.ID); err != nil {
		t.Fatal(err)
	}
	landed := h.wait(t, done.ID, api.RoundLanded, api.RoundPaused)
	if landed.State != api.RoundLanded || readVersion(t, h.repo, "main") != "0.3.0" {
		t.Fatalf("legacy landing: %+v", landed)
	}
}

// Recreate a persisted shared goal session from before fan-out was introduced.
func startLegacyGoal(t *testing.T, h *harness, bead string) (api.Round, error) {
	t.Helper()
	id := "legacy-goal"
	ws, err := h.m.opts.Workspaces.CreateIntegration(context.Background(), h.repo, "repo", id, "main")
	if err != nil {
		return api.Round{}, err
	}
	goal := prompt.BuildGoal([]prompt.GoalBead{{ID: bead, Title: "First", AcceptanceCriteria: "Do First."}}, api.GoalOptions{Enabled: true}, true)
	run, err := h.runs.Start(runs.StartRequest{RepoID: "repo", RepoPath: h.repo, RoundID: id, Kind: "goal", Workspace: ws.Path, Branch: ws.Branch, Base: ws.Base, Goal: goal, Prompt: goalCommitRule})
	if err != nil {
		return api.Round{}, err
	}
	meta := api.Round{ID: id, RepoID: "repo", RepoPath: h.repo, State: api.RoundImplementing, Target: "main", Base: ws.Base, Branch: ws.Branch, Workspace: ws.Path, GoalText: goal, GoalRunID: run.ID, RunIDs: []string{run.ID}, Beads: []api.RoundBead{{ID: bead, Title: "First", Type: "feature", State: api.BeadRunning, RunID: run.ID}}, BudgetUSD: 25, TimeLimitMinutes: 240, StartedAt: time.Now().UTC().Format(time.RFC3339)}
	dir := filepath.Join(h.m.opts.Dir, id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return api.Round{}, err
	}
	cfg, err := repoconfig.Load(h.repo)
	if err != nil {
		return api.Round{}, err
	}
	r := h.m.newRound(meta, dir)
	r.setCfg(cfg)
	r.mu.Lock()
	r.saveLocked()
	r.mu.Unlock()
	h.m.mu.Lock()
	h.m.rounds[id] = r
	h.m.spawn(r)
	h.m.mu.Unlock()
	return meta, nil
}
