package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/chat"
	"github.com/ryderdavid/nacre/bridge/internal/decisions"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
)

// slowClaude prints an init line and then nothing for a while.
func slowClaude(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "claude")
	script := "#!/bin/sh\necho '{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"s\"}'\nsleep 3\n" +
		"echo '{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"result\":\"ok\",\"session_id\":\"s\",\"total_cost_usd\":0.01}'\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// workRig is a decision rig with a run manager whose stall threshold is
// stallAfter, and one quiet run in the rig's repository.
func workRig(t *testing.T, stallAfter time.Duration) (*decisionRig, api.Run) {
	t.Helper()
	rg := newDecisionRig(t)
	mgr, err := runs.New(runs.Options{Dir: t.TempDir(), ClaudePath: slowClaude(t), StallAfter: stallAfter})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mgr.Shutdown(5 * time.Second) })
	rg.s.Runs = mgr
	path, _ := rg.s.Repos.Path(rg.repo)
	run, err := mgr.Start(runs.StartRequest{RepoID: rg.repo, RepoPath: path, IssueID: "N-1", IssueTitle: "Drafts", Mode: "plan", Prompt: "go"})
	if err != nil {
		t.Fatal(err)
	}
	rg.s.Decisions.LinkWork(run.ID, rg.chatID)
	return rg, run
}

func (rg *decisionRig) work(t *testing.T) []WorkSummary {
	t.Helper()
	var out struct{ Work []WorkSummary }
	if code := rg.list(t, "/v1/repos/"+rg.repo+"/work", &out); code != 200 {
		t.Fatalf("work: %d", code)
	}
	return out.Work
}

// The stall signal is nacred's own: it fires past the configured
// threshold, and not before.
func TestWorkStallFollowsTheConfiguredThreshold(t *testing.T) {
	quick, run := workRig(t, 200*time.Millisecond)
	deadline := time.Now().Add(3 * time.Second)
	var ws []WorkSummary
	for time.Now().Before(deadline) {
		ws = quick.work(t)
		if len(ws) == 1 && ws[0].Stalled {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(ws) != 1 || !ws[0].Stalled || ws[0].Kind != "run" || ws[0].ID != run.ID || ws[0].State != "running" ||
		ws[0].Title != "Drafts" || ws[0].ChatID == nil || *ws[0].ChatID != quick.chatID || ws[0].DecisionID != nil || ws[0].Gates == nil {
		t.Fatalf("with a 200ms threshold: %+v", ws)
	}

	patient, _ := workRig(t, time.Hour)
	time.Sleep(600 * time.Millisecond)
	if ws := patient.work(t); len(ws) != 1 || ws[0].Stalled || ws[0].State != "running" {
		t.Fatalf("with a 1h threshold: %+v", ws)
	}
}

func TestRoundSummary(t *testing.T) {
	rg := newDecisionRig(t)
	round := api.Round{ID: "rdto3k", State: api.RoundReadyToLand, Branch: "epic/nac-axe", Epic: "NAC-axe", BudgetUSD: 50, SpentUSD: 6.4,
		CreatedAt: "2026-11-06T08:00:00Z", StartedAt: "2026-11-06T08:05:00Z", UpdatedAt: "2026-11-06T09:00:00Z", RunIDs: []string{"r1", "r2"},
		SpecContracts: map[string]api.SpecContract{"N-2": {ChangeName: "offline-chat-resume"}},
		Beads: []api.RoundBead{
			{ID: "N-2", State: api.BeadIntegrated, Verification: &api.Verification{OK: true, Gates: []api.GateResult{{Name: "bridge", OK: true}, {Name: "app", OK: true}}}},
			{ID: "N-3", State: api.BeadIntegrated, Verification: &api.Verification{Gates: []api.GateResult{{Name: "bridge", OK: true}, {Name: "app", OK: false}}}},
			{ID: "N-4", State: api.BeadRunning},
		}}
	runsByID := map[string]api.Run{
		"r1": {ID: "r1", Status: api.StatusSucceeded, LastActivityAt: "2026-11-06T09:30:00Z"},
		"r2": {ID: "r2", Status: api.StatusRunning, Stalled: true},
	}
	held := decisions.Decision{ID: "dc_held01", State: decisions.StateHeld, Reason: "Held by you: device check", Target: decisions.Target{Round: "rdto3k"}}
	ws := rg.s.roundSummary(round, runsByID, []string{"bridge", "app", "specs"}, []decisions.Decision{held})
	gates, _ := json.Marshal(ws.Gates)
	if ws.State != "held" || ws.Reason != "Held by you: device check" || ws.DecisionID == nil || *ws.DecisionID != "dc_held01" ||
		ws.Landed != 2 || ws.Total != 3 || ws.Spend != 6.4 || ws.Budget != 50 || ws.Change != "offline-chat-resume" ||
		ws.StartedAt != "2026-11-06T08:05:00Z" || ws.LastOutputAt != "2026-11-06T09:30:00Z" || !ws.Stalled ||
		string(gates) != `[{"name":"bridge","state":"pass"},{"name":"app","state":"fail"},{"name":"specs","state":"pending"}]` {
		t.Fatalf("summary %+v gates %s", ws, gates)
	}
	round.State = api.RoundImplementing
	if ws := rg.s.roundSummary(round, nil, nil, nil); ws.State != "running" || ws.Stalled || ws.DecisionID != nil {
		t.Fatalf("running %+v", ws)
	}
}

// Chats per checkout carry their title, agent, repo, what waits and their
// last line.
func TestChatListPerCheckout(t *testing.T) {
	rg := newDecisionRig(t)
	if _, err := rg.s.Chats.Create(chat.CreateRequest{Agent: chat.AgentClaude}); err != nil {
		t.Fatal(err)
	}
	if code, b := rg.do(t, "POST", "/v1/chats/"+rg.chatID+"/messages", `{"turn_id":"c_lastline","text":"What's blocked?"}`); code != 202 {
		t.Fatalf("send %d %v", code, b)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if rec, _, _ := rg.s.Chats.Get(rg.chatID, 0); rec.State == chat.StateIdle {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	rg.facts.setRound(landRound())
	if code, _, _ := rg.raise(t, `{"kind":"land","target":{"round":"rdto3k"}}`); code != 201 {
		t.Fatal("raise")
	}
	var all, one struct{ Chats []map[string]any }
	rg.list(t, "/v1/chats", &all)
	rg.list(t, "/v1/chats?repo="+rg.repo, &one)
	if len(all.Chats) != 2 || len(one.Chats) != 1 {
		t.Fatalf("all %d, this checkout %d", len(all.Chats), len(one.Chats))
	}
	c := one.Chats[0]
	if c["id"] != rg.chatID || c["title"] != "What's blocked?" || c["agent"] != "claude" || c["repo"] != rg.repo ||
		c["waiting"] != 1.0 || c["last_line"] != "done here" || c["updated_at"] == "" {
		t.Fatalf("chat %v", c)
	}
	var none struct{ Chats []map[string]any }
	if rg.list(t, "/v1/chats?repo=nope", &none); none.Chats == nil || len(none.Chats) != 0 {
		t.Fatalf("unknown repo %v", none)
	}
}

func TestProjectsSummarizeEachRepository(t *testing.T) {
	rg, run := workRig(t, time.Hour)
	if code, _ := rg.do(t, "POST", "/v1/chats/"+rg.chatID+"/messages", `{"turn_id":"c_project","text":"Land it when ready"}`); code != 202 {
		t.Fatal("send")
	}
	rg.facts.setRound(landRound())
	if code, _, _ := rg.raise(t, `{"kind":"land","target":{"round":"rdto3k"}}`); code != 201 {
		t.Fatal("raise")
	}
	// The raised decision's round is in repo r1; make one for this repo too.
	r := landRound()
	r.ID, r.RepoID = "rdhere", rg.repo
	rg.facts.setRound(r)
	if code, _, _ := rg.raise(t, `{"kind":"land","target":{"round":"rdhere"}}`); code != 201 {
		t.Fatal("raise here")
	}
	var out struct{ Projects []Project }
	if code := rg.list(t, "/v1/projects", &out); code != 200 || len(out.Projects) != 1 {
		t.Fatalf("projects %d %+v", code, out)
	}
	p := out.Projects[0]
	if p.Repo != rg.repo || p.Name == "" || p.HostName != "Mac-Studio" || p.Status != "working" || p.NeedsYou != 1 ||
		p.Work == nil || p.Work.ID != run.ID || p.StatusText != "Run running · 1 needs you" || p.LastLine == "" {
		t.Fatalf("project %+v work %+v", p, p.Work)
	}
}

func TestSpecsListTheCurrentCapabilities(t *testing.T) {
	rg := newDecisionRig(t)
	path, _ := rg.s.Repos.Path(rg.repo)
	var empty struct{ Specs []SpecSummary }
	if code := rg.list(t, "/v1/repos/"+rg.repo+"/specs", &empty); code != 200 || empty.Specs == nil || len(empty.Specs) != 0 {
		t.Fatalf("without openspec: %d %+v", code, empty)
	}
	for name, body := range map[string]string{
		"agent-chat": "# agent-chat\n\n## Requirements\n\n### Requirement: Chat runs the full agent harness on the host\nText.\n\n### Requirement: Every message carries the screen\n",
		"needs-you":  "### Requirement: Inbox\n",
	} {
		dir := filepath.Join(path, "openspec", "specs", name)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "spec.md"), []byte(body), 0o644)
	}
	os.MkdirAll(filepath.Join(path, "openspec", "specs", "no-spec-file"), 0o755)
	var got struct{ Specs []SpecSummary }
	rg.list(t, "/v1/repos/"+rg.repo+"/specs", &got)
	if len(got.Specs) != 2 || got.Specs[0].Name != "agent-chat" || got.Specs[0].Path != "openspec/specs/agent-chat/spec.md" ||
		len(got.Specs[0].Requirements) != 2 || got.Specs[0].Requirements[0] != "Chat runs the full agent harness on the host" ||
		got.Specs[1].Name != "needs-you" || got.Specs[1].Requirements[0] != "Inbox" {
		t.Fatalf("specs %+v", got.Specs)
	}
	if code := rg.list(t, "/v1/repos/nope/specs", &got); code != 404 {
		t.Fatalf("unknown repo %d", code)
	}
}
