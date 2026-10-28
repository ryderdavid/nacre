package runs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

// A round run works in the worktree nacred made, not in the repository, and
// carries its round, branch and base.
func TestRunInWorkspace(t *testing.T) {
	repo, ws := t.TempDir(), t.TempDir()
	envOut := filepath.Join(t.TempDir(), "env")
	script := `#!/bin/sh
echo "$NACRE_RESOURCE_EMULATOR" > ` + envOut + `
for a in "$@"; do [ "$a" = "--worktree" ] && echo "--worktree passed" >> ` + envOut + `; done
echo '{"type":"system","subtype":"init","session_id":"s","model":"fake","cwd":"'"$PWD"'"}'
echo '{"type":"result","subtype":"success","is_error":false,"result":"ok","session_id":"s","total_cost_usd":0.1}'
`
	bin := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := New(Options{Dir: t.TempDir(), ClaudePath: bin})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown(time.Second)
	run, err := m.Start(StartRequest{
		SpecChangeID: "change", SpecRevision: "approved-r2", SpecSectionID: "opaque-section",
		RepoID: "r", RepoPath: repo, IssueID: "X-1", Mode: "auto", Prompt: "go", Worktree: true,
		RoundID: "rd1", Kind: "bead", Branch: "nacre/r/rd1/X-1", Workspace: ws, Base: "abc123",
		MaxBudgetUSD: 4, Env: []string{"NACRE_RESOURCE_EMULATOR=emulator-5554"},
	})
	if err != nil {
		t.Fatal(err)
	}
	done := waitStatus(t, m, run.ID, api.StatusSucceeded)
	if done.SpecChangeID != "change" || done.SpecRevision != "approved-r2" || done.SpecSectionID != "opaque-section" {
		t.Fatalf("run lost specification provenance: %+v", done)
	}
	realWS, _ := filepath.EvalSymlinks(ws)
	if got, _ := filepath.EvalSymlinks(done.SessionCWD); got != realWS {
		t.Errorf("ran in %q, want %q", done.SessionCWD, ws)
	}
	if done.RoundID != "rd1" || done.Kind != "bead" || done.Branch != "nacre/r/rd1/X-1" || done.Base != "abc123" || done.Workspace != ws {
		t.Errorf("round fields = %+v", done)
	}
	b, _ := os.ReadFile(envOut)
	if string(b) != "emulator-5554\n" {
		t.Errorf("env/args seen by claude: %q", b)
	}

	if err := m.SetVerification(run.ID, &api.Verification{OK: true, Commits: 2}); err != nil {
		t.Fatal(err)
	}
	if r, _ := m.Run(run.ID); r.Verification == nil || !r.Verification.OK || r.Verification.Commits != 2 {
		t.Errorf("verification = %+v", r.Verification)
	}
}
