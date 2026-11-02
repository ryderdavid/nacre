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

// fakeGoalClaude commits the round's work once and stops; it never pushes.
const fakeGoalClaude = `#!/bin/sh
set -e
prompt="$2"
echo '{"type":"system","subtype":"init","session_id":"goal-session","cwd":"'"$PWD"'"}'
case "$prompt" in
 /goal*)
   printf '%s' "$prompt" > goal-launch.txt
   echo a > a.txt; echo b > b.txt
   git add a.txt b.txt goal-launch.txt
   git commit -q -m 'Implement every round bead';;
esac
echo '{"type":"result","subtype":"success","is_error":false,"result":"verified every criterion","session_id":"goal-session","total_cost_usd":0.1}'
`

// A goal round follows round-integration like every round (NAC-ow0): the
// agent only commits, nacred makes its own version bump commit after the
// session, and pushing happens only when nacred lands the round, if pushed
// was checked.
func TestGoalRoundAgentCommitsAndNacredBumpsAndPushes(t *testing.T) {
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
			if run.Turns != 1 {
				t.Fatalf("the session was resumed although its committed work passed: %+v", run)
			}
			if run.Verification == nil || !run.Verification.OK || run.Verification.Commits != 2 {
				t.Fatalf("verification: %+v", run.Verification)
			}
			launch, err := os.ReadFile(filepath.Join(done.Workspace, "goal-launch.txt"))
			if err != nil {
				t.Fatal(err)
			}
			for _, part := range []string{"/goal ", "T-A: First", "T-B: Second", "Do First.", "Do Second.", "Commit your work on this round branch", "Do not bump or edit the version, push"} {
				if !strings.Contains(string(launch), part) {
					t.Fatalf("launch missing %q", part)
				}
			}
			for _, part := range []string{"git push", "already bumped", "authorizes pushing"} {
				if strings.Contains(string(launch), part) {
					t.Fatalf("launch still hands the agent %q:\n%s", part, launch)
				}
			}
			// The round ends on nacred's own bump commit, above the agent's.
			if subj, _ := gitx.Run(ctx, done.Workspace, "log", "-1", "--format=%s", done.FinalCommit); !strings.HasPrefix(subj, "Bump version (minor) for round "+done.ID) {
				t.Fatalf("final commit %q is not nacred's version bump", subj)
			}
			if files, _ := gitx.Run(ctx, done.Workspace, "diff", "--name-only", done.Base, done.FinalCommit+"^"); strings.Contains(files, "VERSION") {
				t.Fatalf("the agent's commit changed the version: %s", files)
			}
			if !strings.Contains(done.Version, "0.2.0") {
				t.Fatalf("version %q, want a minor bump to 0.2.0", done.Version)
			}
			for _, b := range done.Beads {
				if b.State != api.BeadIntegrated || b.RunID != done.GoalRunID {
					t.Fatalf("bead %+v", b)
				}
			}
			if refs, _ := gitx.Run(ctx, remote, "for-each-ref"); refs != "" {
				t.Fatalf("something was pushed before landing: %s", refs)
			}
			if len(h.m.leases.holders()) != 0 {
				t.Fatal("resource lease leaked")
			}
			loaded, err := h.m.load(h.m.get(r.ID).dir)
			if err != nil || loaded.meta.GoalText != done.GoalText || loaded.meta.Goal != done.Goal || loaded.meta.GoalRunID != done.GoalRunID {
				t.Fatalf("goal state did not persist: %v", err)
			}
			// The target ships a release of its own before the round lands:
			// nacred's bump commit is undone, the target merged, the bump
			// redone on top of it, and the gates and build run again.
			if err := os.WriteFile(filepath.Join(h.repo, "VERSION"), []byte("0.2.0\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(h.repo, "hotfix.txt"), []byte("fix\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := gitx.CommitAll(ctx, h.repo, "hotfix 0.2.0"); err != nil {
				t.Fatal(err)
			}
			if _, err := h.m.Land(done.ID); err != nil {
				t.Fatal(err)
			}
			landed := h.wait(t, done.ID, api.RoundLanded, api.RoundPaused)
			if landed.State != api.RoundLanded {
				t.Fatalf("goal round did not land on a moved target: %s %q", landed.State, landed.PauseReason)
			}
			if b, _ := os.ReadFile(filepath.Join(h.repo, "VERSION")); strings.TrimSpace(string(b)) != "0.3.0" {
				t.Errorf("main VERSION = %q, want 0.3.0 (a minor bump on top of main's 0.2.0)", b)
			}
			if !strings.Contains(landed.Version, "0.3.0") || landed.Tag != "v0.3.0" {
				t.Errorf("round version %q, tag %q; want 0.3.0", landed.Version, landed.Tag)
			}
			for _, f := range []string{"hotfix.txt", "a.txt", "b.txt"} {
				if _, err := os.Stat(filepath.Join(h.repo, f)); err != nil {
					t.Errorf("main lacks %s", f)
				}
			}
			if !gitx.IsAncestor(ctx, h.repo, done.FinalCommit, landed.LandedCommit) {
				t.Errorf("landed %s doesn't contain the round's final commit %s", landed.LandedCommit, done.FinalCommit)
			}
			if log, _ := gitx.Run(ctx, h.repo, "log", "-1", "--format=%s", "main"); !strings.Contains(log, "on top of main") {
				t.Errorf("last commit on main = %q, want the redone bump", log)
			}
			// nacred, not the agent, pushes on landing when pushed was checked
			// (the repository's own landing policy here doesn't push).
			sha, err := gitx.RevParse(ctx, remote, "refs/heads/main")
			if push {
				if err != nil || sha != landed.LandedCommit || !landed.Pushed {
					t.Errorf("remote main %s (%v), pushed %v; want nacred to push %s on landing", sha, err, landed.Pushed, landed.LandedCommit)
				}
				if _, err := gitx.RevParse(ctx, remote, "refs/tags/v0.3.0"); err != nil {
					t.Errorf("tag not pushed: %v", err)
				}
			} else if err == nil || landed.Pushed {
				t.Errorf("landing pushed main to %s although pushed wasn't checked", sha)
			}
			if _, err := gitx.RevParse(ctx, remote, "refs/heads/"+done.Branch); err == nil {
				t.Errorf("the round branch %s was pushed", done.Branch)
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
	if done.GoalRunID != paused.GoalRunID || run.Turns != 2 {
		t.Fatalf("retry lost its session: %+v", done)
	}
	if paused.Version != "" || done.Version == "" {
		t.Fatalf("version %q before the session finished, %q after; want nacred to bump once it did", paused.Version, done.Version)
	}
	if n, _ := gitx.Run(context.Background(), done.Workspace, "log", "--oneline", "--fixed-strings", "--grep", "for round "+done.ID+": ", done.Base+"..HEAD"); strings.Count(n, "\n") != 0 || n == "" {
		t.Fatalf("want exactly one bump commit, got:\n%s", n)
	}
}
