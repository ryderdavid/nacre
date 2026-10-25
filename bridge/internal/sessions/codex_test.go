package sessions

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// codexHome lays out a ~/.codex with a sessions directory for today.
func codexHome(t *testing.T) (home, day string) {
	t.Helper()
	home = t.TempDir()
	now := time.Now()
	day = filepath.Join(home, "sessions", now.Format("2006"), now.Format("01"), now.Format("02"))
	if err := os.MkdirAll(day, 0o700); err != nil {
		t.Fatal(err)
	}
	return home, day
}

// rolloutMetaLine is a session_meta line as Codex writes it. source is raw
// JSON ("\"vscode\"", or a subagent object).
func rolloutMetaLine(id, cwd, originator, source string, started time.Time) string {
	return fmt.Sprintf(`{"timestamp":%q,"ordinal":0,"type":"session_meta","payload":{"id":%q,"timestamp":%q,"cwd":%q,"originator":%q,"cli_version":"0.159.2","source":%s,"base_instructions":{"text":"You are Codex."}}}`+"\n",
		started.UTC().Format(time.RFC3339Nano), id, started.UTC().Format(time.RFC3339Nano), cwd, originator, source)
}

func rolloutEvent(ts time.Time, payload string) string {
	return fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":%s}`+"\n", ts.UTC().Format(time.RFC3339Nano), payload)
}

func rolloutCommand(ts time.Time, id, cmd string) string {
	return rolloutEvent(ts, fmt.Sprintf(`{"type":"item_completed","item":{"type":"CommandExecution","id":%q,"command":["/bin/zsh","-lc",%q],"status":"completed","aggregated_output":"","exit_code":0}}`, id, cmd))
}

func rolloutText(ts time.Time, text string) string {
	return rolloutEvent(ts, fmt.Sprintf(`{"type":"item_completed","item":{"type":"AgentMessage","id":"m","content":[{"type":"Text","text":%q}]}}`, text))
}

func codexWatcher(home, repoPath string, owned func(string) bool, got *[]Session) *Watcher {
	return New(Options{
		Dir: filepath.Join(home, "no-claude-registry"), Projects: filepath.Join(home, "no-projects"),
		Procs: fakeProcs{}, CodexHome: home, CodexQuietAfter: 30 * time.Minute,
		MainCheckout: func(string) string { return "" },
		Repos:        func() []Repo { return []Repo{{ID: "r1", Name: "repo", Path: repoPath}} },
		Beads:        func(string) []Bead { return []Bead{{ID: "NAC-7", Title: "Seven", Status: "in_progress"}} },
		Owned:        owned,
		Sync:         func(live []Session) { *got = live },
		Logger:       log.New(io.Discard, "", 0),
	})
}

func TestPollAdoptsCodexSessionsFromRollouts(t *testing.T) {
	home, day := codexHome(t)
	repoPath := t.TempDir()
	start := time.Now().Add(-5 * time.Minute)

	agentID := "01a17081-0000-7000-8000-000000000001"
	agent := filepath.Join(day, "rollout-a-"+agentID+".jsonl")
	mustWrite(t, agent, rolloutMetaLine(agentID, filepath.Join(repoPath, "sub"), "codex-tui", `"vscode"`, start)+
		rolloutEvent(start.Add(time.Second), `{"type":"task_started","turn_id":"t1"}`)+
		rolloutText(start.Add(2*time.Second), "Claiming the bead.")+
		rolloutCommand(start.Add(3*time.Second), "exec-1", "bd update NAC-7 --claim"))
	mustWrite(t, filepath.Join(home, "session_index.jsonl"), fmt.Sprintf(`{"id":%q,"thread_name":"Fix the slow check","updated_at":"2026-10-24T00:00:00Z"}`+"\n", agentID))

	// None of these may be adopted.
	sub := "01a17081-0000-7000-8000-000000000002"
	mustWrite(t, filepath.Join(day, "rollout-b-"+sub+".jsonl"), rolloutMetaLine(sub, repoPath, "Codex Desktop", `{"subagent":{"other":"guardian"}}`, start))
	own := "01a17081-0000-7000-8000-000000000003"
	mustWrite(t, filepath.Join(day, "rollout-c-"+own+".jsonl"), rolloutMetaLine(own, repoPath, "codex_exec", `"exec"`, start))
	elsewhere := "01a17081-0000-7000-8000-000000000004"
	mustWrite(t, filepath.Join(day, "rollout-d-"+elsewhere+".jsonl"), rolloutMetaLine(elsewhere, "/not/a/repo", "codex-tui", `"vscode"`, start))
	young := "01a17081-0000-7000-8000-000000000005"
	mustWrite(t, filepath.Join(day, "rollout-e-"+young+".jsonl"), rolloutMetaLine(young, repoPath, "codex_exec", `"exec"`, time.Now()))
	quiet := "01a17081-0000-7000-8000-000000000006"
	quietPath := filepath.Join(day, "rollout-f-"+quiet+".jsonl")
	mustWrite(t, quietPath, rolloutMetaLine(quiet, repoPath, "codex-tui", `"vscode"`, start.Add(-2*time.Hour)))
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(quietPath, old, old); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(day, "notes.txt"), "not a rollout")

	var got []Session
	w := codexWatcher(home, repoPath, func(sid string) bool { return sid == own }, &got)
	w.Poll()

	if len(got) != 1 {
		t.Fatalf("adopted %d sessions, want 1: %+v", len(got), got)
	}
	s := got[0]
	if s.Agent != api.AgentCodex || s.SessionID != agentID || s.PID != 0 || s.Repo.ID != "r1" {
		t.Fatalf("session = %+v", s)
	}
	if s.Name != "Fix the slow check" || s.Entrypoint != "codex-tui" || s.Version != "0.159.2" || s.Transcript != agent {
		t.Fatalf("record = %+v", s.Record)
	}
	if s.IssueID != "NAC-7" || s.LinkedBy != LinkTranscript {
		t.Fatalf("link = %q by %q, want NAC-7 from the bd claim in the rollout", s.IssueID, s.LinkedBy)
	}
	if s.Status == claude.ActivityIdle {
		t.Fatal("a session mid-turn is reported idle")
	}
	if len(s.Events) != 3 || s.Events[0].Kind != api.KindText || s.Events[1].Tool != "Bash" || !s.FromStart {
		t.Fatalf("events = %+v", s.Events)
	}

	// The turn finishes: an interactive session stays, parked between turns.
	appendTo(t, agent, rolloutEvent(start.Add(4*time.Second), `{"type":"task_complete","turn_id":"t1"}`))
	w.Poll()
	if len(got) != 1 || got[0].Status != claude.ActivityIdle || len(got[0].Events) != 0 || got[0].FromStart {
		t.Fatalf("after turn = %+v", got)
	}

	// Quiet for longer than CodexQuietAfter: the session has ended.
	if err := os.Chtimes(agent, old, old); err != nil {
		t.Fatal(err)
	}
	w.Poll()
	if len(got) != 0 || len(w.tails) != 0 || len(w.codex.turns) != 0 {
		t.Fatalf("after going quiet: %d sessions, %d tails, %d turns", len(got), len(w.tails), len(w.codex.turns))
	}

	// Writing again brings it back, read from the start.
	appendTo(t, agent, rolloutEvent(time.Now(), `{"type":"task_started","turn_id":"t2"}`))
	w.Poll()
	if len(got) != 1 || got[0].SessionID != agentID || !got[0].FromStart || got[0].Status == claude.ActivityIdle {
		t.Fatalf("after resuming = %+v", got)
	}
}

// codex exec runs one turn and exits, so its session ends when the turn does.
func TestCodexExecSessionEndsWithItsTurn(t *testing.T) {
	home, day := codexHome(t)
	repoPath := t.TempDir()
	start := time.Now().Add(-time.Minute)
	id := "01a17081-0000-7000-8000-000000000011"
	path := filepath.Join(day, "rollout-x-"+id+".jsonl")
	mustWrite(t, path, rolloutMetaLine(id, repoPath, "codex_exec", `"exec"`, start)+
		rolloutEvent(start.Add(time.Second), `{"type":"task_started","turn_id":"t1"}`)+
		rolloutCommand(start.Add(2*time.Second), "exec-1", "bd show NAC-7"))

	var got []Session
	w := codexWatcher(home, repoPath, nil, &got)
	w.Poll()
	if len(got) != 1 || got[0].Kind != "exec" || got[0].IssueID != "NAC-7" {
		t.Fatalf("running exec = %+v", got)
	}

	appendTo(t, path, rolloutText(start.Add(3*time.Second), "done")+rolloutEvent(start.Add(4*time.Second), `{"type":"task_complete","turn_id":"t1"}`))
	// The poll that sees the turn close still delivers its last events...
	w.Poll()
	if len(got) != 1 || len(got[0].Events) != 1 || got[0].Events[0].Text != "done" {
		t.Fatalf("closing poll = %+v", got)
	}
	// ...and the next one ends the session.
	w.Poll()
	if len(got) != 0 {
		t.Fatalf("finished exec still live: %+v", got)
	}
	// A finished exec is remembered by mtime rather than re-read every poll.
	if _, ok := w.codex.done[path]; !ok {
		t.Fatal("finished exec rollout not remembered")
	}
	w.Poll()
	if len(got) != 0 || len(w.tails) != 0 {
		t.Fatalf("second poll after exit: %d sessions, %d tails", len(got), len(w.tails))
	}
}

// An exec session that has already finished when the watcher first sees it
// (nacred restarted after it ran) is not adopted only to be ended.
func TestFinishedCodexExecIsNotAdopted(t *testing.T) {
	home, day := codexHome(t)
	repoPath := t.TempDir()
	start := time.Now().Add(-time.Minute)
	id := "01a17081-0000-7000-8000-000000000031"
	path := filepath.Join(day, "rollout-x-"+id+".jsonl")
	mustWrite(t, path, rolloutMetaLine(id, repoPath, "codex_exec", `"exec"`, start)+
		rolloutEvent(start.Add(time.Second), `{"type":"task_started","turn_id":"t1"}`)+
		rolloutText(start.Add(2*time.Second), "done")+
		rolloutEvent(start.Add(3*time.Second), `{"type":"task_complete","turn_id":"t1"}`))

	var got []Session
	w := codexWatcher(home, repoPath, nil, &got)
	w.Poll()
	if len(got) != 0 || len(w.tails) != 0 {
		t.Fatalf("finished exec adopted: %+v", got)
	}
}

// Without CodexHome the watcher reads nothing of Codex's.
func TestCodexIsOffWithoutCodexHome(t *testing.T) {
	home, day := codexHome(t)
	repoPath := t.TempDir()
	id := "01a17081-0000-7000-8000-000000000021"
	mustWrite(t, filepath.Join(day, "rollout-x-"+id+".jsonl"), rolloutMetaLine(id, repoPath, "codex-tui", `"vscode"`, time.Now().Add(-time.Minute)))

	var got []Session
	w := codexWatcher(home, repoPath, nil, &got)
	w.opts.CodexHome = ""
	w.Poll()
	if len(got) != 0 {
		t.Fatalf("adopted with Codex off: %+v", got)
	}
}
