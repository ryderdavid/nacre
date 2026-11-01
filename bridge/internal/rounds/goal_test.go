package rounds

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/gitx"
)

const fakeGoalClaude = `#!/bin/sh
set -e
prompt="$2"
resume=no
for a in "$@"; do [ "$a" = "--resume" ] && resume=yes; done
if [ "$resume" = yes ]; then
 branch=$(git symbolic-ref --short HEAD)
 git push -q origin "HEAD:refs/heads/$branch"
else
echo '{"type":"system","subtype":"init","session_id":"goal-session","cwd":"'"$PWD"'"}'
case "$prompt" in
 /goal*)
   printf '%s' "$prompt" > goal-launch.txt
   echo a > a.txt; echo b > b.txt
   git add a.txt b.txt goal-launch.txt VERSION
   git commit -q -m 'Implement every round bead';;
 *"goal is still incomplete"*)
   branch=$(git symbolic-ref --short HEAD)
   git push -q origin "HEAD:refs/heads/$branch";;
esac
fi
echo '{"type":"result","subtype":"success","is_error":false,"result":"verified every criterion","session_id":"goal-session","total_cost_usd":0.1}'
`

func TestGoalRoundWorksEveryBeadInOneSessionAndChecksPush(t *testing.T) {
	for _, push := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "push"}[push], func(t *testing.T) {
			beads := newFakeBeads(&fakeBead{ID: "T-A", Title: "First", Type: "feature"}, &fakeBead{ID: "T-B", Title: "Second", After: []string{"T-A"}})
			plan := `{"summary":"work both","beads":[{"id":"T-A","needs":["emulator"]},{"id":"T-B"}],"findings":[]}`
			h := newHarnessWith(t, beads, plan, harnessOpts{claudeScript: fakeGoalClaude})
			ctx := context.Background()
			remote := t.TempDir()
			if _, err := gitx.Run(ctx, remote, "init", "--bare", "-q"); err != nil {
				t.Fatal(err)
			}
			if _, err := gitx.Run(ctx, h.repo, "remote", "add", "origin", remote); err != nil {
				t.Fatal(err)
			}
			r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-A", "T-B"}, Goal: api.GoalOptions{Enabled: true, Committed: true, Pushed: push}})
			if err != nil {
				t.Fatal(err)
			}
			done := h.wait(t, r.ID, api.RoundReadyToLand, api.RoundPaused)
			if done.State != api.RoundReadyToLand {
				t.Fatalf("goal round paused: %+v", done)
			}
			if len(done.RunIDs) != 1 || done.GoalRunID == "" {
				t.Fatalf("expected one session: %+v", done)
			}
			run, _ := h.runs.Run(done.GoalRunID)
			if push && run.Turns != 2 {
				t.Fatalf("missing push was not independently retried: %+v", run)
			}
			if run.Verification == nil || !run.Verification.OK || run.Verification.Commits != 1 {
				t.Fatalf("verification: %+v", run)
			}
			launch, err := os.ReadFile(filepath.Join(done.Workspace, "goal-launch.txt"))
			if err != nil {
				t.Fatal(err)
			}
			for _, part := range []string{"/goal ", "T-A: First", "T-B: Second", "Do First.", "Do Second.", "one combined implementation commit"} {
				if !strings.Contains(string(launch), part) {
					t.Fatalf("launch missing %q", part)
				}
			}
			for _, b := range done.Beads {
				if b.State != api.BeadIntegrated || b.RunID != done.GoalRunID {
					t.Fatalf("bead %+v", b)
				}
			}
			if push {
				sha, err := gitx.RevParse(ctx, remote, "refs/heads/"+done.Branch)
				if err != nil || sha != done.FinalCommit {
					t.Fatalf("remote %s: %v", sha, err)
				}
			}
			if len(h.m.leases.holders()) != 0 {
				t.Fatal("resource lease leaked")
			}
			loaded, err := h.m.load(h.m.get(r.ID).dir)
			if err != nil || loaded.meta.GoalText != done.GoalText || loaded.meta.Goal != done.Goal || loaded.meta.GoalRunID != done.GoalRunID {
				t.Fatalf("goal state did not persist: %v", err)
			}
			if !push {
				// Combined version commits must not use the old standalone
				// version-revert path if the target moves before landing.
				if err := os.WriteFile(filepath.Join(h.repo, "VERSION"), []byte("0.1.1\n"), 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := gitx.CommitAll(ctx, h.repo, "Target version moved"); err != nil {
					t.Fatal(err)
				}
				if _, err := h.m.Land(done.ID); err != nil {
					t.Fatal(err)
				}
				paused := h.wait(t, done.ID, api.RoundPaused)
				if !strings.Contains(paused.PauseReason, "target moved") || paused.FinalCommit != done.FinalCommit {
					t.Fatalf("unsafe goal landing: %+v", paused)
				}
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
 git add a.txt VERSION
 git commit -q -m 'Finish round goal'
 echo '{"type":"result","is_error":false,"result":"verified","session_id":"goal-session"}'
fi
`
	h := newHarnessWith(t, newFakeBeads(&fakeBead{ID: "T-A", Title: "First", Type: "feature"}), `{"beads":[{"id":"T-A"}],"findings":[]}`, harnessOpts{claudeScript: script})
	r, err := h.m.Create(CreateRequest{RepoID: "repo", Beads: []string{"T-A"}, Goal: api.GoalOptions{Enabled: true}})
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
	if done.GoalRunID != paused.GoalRunID || run.Turns != 2 || done.Version != paused.Version {
		t.Fatalf("retry lost its session or bumped again: %+v", done)
	}
}
