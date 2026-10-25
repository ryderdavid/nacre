package runs

import (
	"errors"
	"io"
	"log"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/sessions"
)

// agentSession is a live agent-owned Claude Code session as the watcher
// reports it.
func agentSession(sid, issue string) sessions.Session {
	s := sessions.Session{
		Record: sessions.Record{
			PID: 4242, SessionID: sid, CWD: "/code/Nacre", Name: "nacre-eb",
			StartedAt: time.Now().Add(-time.Minute).UnixMilli(),
			Version:   "2.1.304", Entrypoint: "claude-desktop",
		},
		Repo: sessions.Repo{ID: "rid", Name: "Nacre", Path: "/code/Nacre"},
	}
	if issue != "" {
		s.IssueID, s.IssueTitle, s.LinkedBy = issue, "Seven", sessions.LinkTranscript
	}
	return s
}

func quietManager(t *testing.T, dir string) *Manager {
	t.Helper()
	m, err := New(Options{Dir: dir, ClaudePath: "/nonexistent", MaxConcurrent: 1,
		Logger: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestAdoptedRunIsReadOnlyAndOptIn(t *testing.T) {
	m := quietManager(t, t.TempDir())
	s := agentSession("sess-agent", "NAC-7")
	m.SyncAdopted([]sessions.Session{s})

	adopted := m.List(Filter{IncludeAdopted: true})
	if len(adopted) != 1 {
		t.Fatalf("adopted runs = %+v", adopted)
	}
	got := adopted[0]
	if !strings.HasPrefix(got.ID, "r_") || len(got.ID) != 28 {
		t.Fatalf("adopted id %q: must be a normal run id so the store and prune still work", got.ID)
	}
	if got.Origin != api.OriginAdopted || !got.Adopted() || got.Status != api.StatusRunning {
		t.Fatalf("run = %+v", got)
	}
	if got.IssueID != "NAC-7" || got.IssueTitle != "Seven" || got.SessionID != "sess-agent" ||
		got.RepoID != "rid" || got.SessionCWD != "/code/Nacre" {
		t.Fatalf("run = %+v", got)
	}
	if !strings.Contains(got.ResumeCommand, "claude --resume sess-agent") {
		t.Fatalf("resume command = %q", got.ResumeCommand)
	}

	// Invisible to a client that has not asked for adopted runs, and not work
	// nacred is doing.
	if plain := m.List(Filter{}); len(plain) != 0 {
		t.Fatalf("GET /v1/runs without include=adopted = %+v", plain)
	}
	if n := m.ActiveCount(""); n != 0 {
		t.Fatalf("ActiveCount = %d, want 0: an adopted run holds no concurrency slot", n)
	}
	if n := m.ActiveCount("rid"); n != 0 {
		t.Fatalf("repo ActiveCount = %d, want 0", n)
	}

	// Neither operation is safe against a session nacred does not own.
	if _, err := m.Cancel(got.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("Cancel = %v, want a conflict", err)
	}
	if _, err := m.Followup(got.ID, "hi", ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("Followup = %v, want a conflict", err)
	}
	// It is not one of nacred's own sessions, so the watcher must still see it.
	if m.OwnsSession("sess-agent") {
		t.Fatal("OwnsSession must only cover runs nacred dispatched")
	}

	// A queued run still starts: the adopted run never entered the queue.
	if _, evs, err := m.Get(got.ID); err != nil || len(evs) != 2 ||
		evs[0].Kind != api.KindSystem || !strings.Contains(evs[0].Text, "adopted Claude Code session") {
		t.Fatalf("events = %+v (%v)", evs, err)
	}
}

func TestSyncAdoptedIsIdempotentAndEndsVanishedSessions(t *testing.T) {
	dir := t.TempDir()
	m := quietManager(t, dir)
	s := agentSession("sess-agent", "NAC-7")
	m.SyncAdopted([]sessions.Session{s})
	first := m.List(Filter{IncludeAdopted: true})[0].ID

	for i := 0; i < 3; i++ {
		m.SyncAdopted([]sessions.Session{s})
	}
	again := m.List(Filter{IncludeAdopted: true})
	if len(again) != 1 || again[0].ID != first {
		t.Fatalf("rescans adopted the same session twice: %+v", again)
	}

	// The process is gone: the run ends, with no error, because nobody
	// reports a result for a session nacred did not run.
	m.SyncAdopted(nil)
	ended := m.List(Filter{IncludeAdopted: true})[0]
	if ended.Active() || ended.Status != api.StatusSucceeded || ended.EndedAt == "" || ended.Error != "" {
		t.Fatalf("ended run = %+v", ended)
	}
	_, evs, _ := m.Get(first)
	if last := evs[len(evs)-2]; last.Kind != api.KindSystem || !strings.Contains(last.Text, "no longer running") {
		t.Fatalf("end events = %+v", evs)
	}
	// Ending twice is a no-op.
	m.SyncAdopted(nil)
	if _, evs2, _ := m.Get(first); len(evs2) != len(evs) {
		t.Fatalf("second end wrote %d more events", len(evs2)-len(evs))
	}
	// And a session that comes back resumes the same run.
	m.SyncAdopted([]sessions.Session{s})
	back := m.List(Filter{IncludeAdopted: true})
	if len(back) != 1 || back[0].ID != first || back[0].Status != api.StatusRunning || back[0].EndedAt != "" {
		t.Fatalf("resumed run = %+v", back)
	}
}

func TestAdoptedRunSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	m := quietManager(t, dir)
	s := agentSession("sess-agent", "NAC-7")
	m.SyncAdopted([]sessions.Session{s})
	id := m.List(Filter{IncludeAdopted: true})[0].ID

	// A restart must not bury live agent work: nacred never owned the
	// process, so its own restart says nothing about the session.
	m2 := quietManager(t, dir)
	after := m2.List(Filter{IncludeAdopted: true})
	if len(after) != 1 || after[0].ID != id || after[0].Status != api.StatusRunning || after[0].Error != "" {
		t.Fatalf("after restart = %+v", after)
	}
	// The session is re-resolved from the registry, not adopted again.
	m2.SyncAdopted([]sessions.Session{s})
	if again := m2.List(Filter{IncludeAdopted: true}); len(again) != 1 || again[0].ID != id {
		t.Fatalf("re-resolve = %+v", again)
	}
	// Restarting with adoption switched off ends them instead of leaving them
	// running forever.
	m3 := quietManager(t, dir)
	m3.SyncAdopted(nil)
	if off := m3.List(Filter{IncludeAdopted: true}); len(off) != 1 || off[0].Active() {
		t.Fatalf("adoption disabled = %+v", off)
	}
}

func TestAdoptedRunPicksUpALaterBeadLink(t *testing.T) {
	m := quietManager(t, t.TempDir())
	unlinked := agentSession("sess-agent", "")
	m.SyncAdopted([]sessions.Session{unlinked})
	got := m.List(Filter{IncludeAdopted: true})[0]
	if got.IssueID != "" {
		t.Fatalf("a session that has moved no bead must not be linked to one: %+v", got)
	}
	_, evs, _ := m.Get(got.ID)
	if !strings.Contains(evs[0].Text, "no bead linked") {
		t.Fatalf("notice = %q", evs[0].Text)
	}

	// The agent claims a bead; the next poll links the existing run.
	m.SyncAdopted([]sessions.Session{agentSession("sess-agent", "NAC-7")})
	linked := m.List(Filter{IncludeAdopted: true})
	if len(linked) != 1 || linked[0].ID != got.ID || linked[0].IssueID != "NAC-7" {
		t.Fatalf("linked = %+v", linked)
	}
	_, evs, _ = m.Get(got.ID)
	if last := evs[len(evs)-1]; !strings.Contains(last.Text, "linked to NAC-7 (transcript)") {
		t.Fatalf("link event = %+v", last)
	}
	// Repeating the same reading changes nothing.
	before := len(evs)
	m.SyncAdopted([]sessions.Session{agentSession("sess-agent", "NAC-7")})
	if _, evs, _ = m.Get(got.ID); len(evs) != before {
		t.Fatalf("re-link wrote %d more events", len(evs)-before)
	}
}

func TestOwnsSessionCoversDispatchedRuns(t *testing.T) {
	m, err := New(Options{Dir: t.TempDir(), ClaudePath: fakeClaude(t), MaxConcurrent: 1,
		Logger: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	r, err := m.Start(StartRequest{RepoID: "rid", RepoPath: t.TempDir(), IssueID: "X-1", Prompt: "do it"})
	if err != nil {
		t.Fatal(err)
	}
	waitStatus(t, m, r.ID, api.StatusSucceeded, api.StatusFailed)
	// fakeClaude reports session "sess-1"; the watcher must not adopt it a
	// second time when it finds it in the registry.
	if !m.OwnsSession("sess-1") {
		t.Fatal("OwnsSession(sess-1) = false, want true")
	}
	if m.OwnsSession("") || m.OwnsSession("sess-other") {
		t.Fatal("OwnsSession matched a session nacred never started")
	}
}

func TestAdoptedRunCarriesOriginAgentAndCapabilityFlags(t *testing.T) {
	m := quietManager(t, t.TempDir())
	s := agentSession("sess-agent", "NAC-7")
	m.SyncAdopted([]sessions.Session{s})
	got := m.List(Filter{IncludeAdopted: true})[0]

	if got.Origin != api.OriginAdopted || got.Agent != api.AgentClaudeCode {
		t.Fatalf("origin/agent = %q/%q", got.Origin, got.Agent)
	}
	if got.SessionName != "nacre-eb" || got.OwnerPID != 4242 {
		t.Fatalf("session name %q, owner pid %d: a client must be able to name the session it cannot control",
			got.SessionName, got.OwnerPID)
	}
	// Alive and not parked, but still one of the five statuses clients key on.
	if got.Status != api.StatusRunning || got.Activity != api.ActivityWorking {
		t.Fatalf("status/activity = %q/%q", got.Status, got.Activity)
	}
	if got.CanCancel == nil || *got.CanCancel || got.CanFollowup == nil || *got.CanFollowup {
		t.Fatalf("capability flags = %v/%v, want both present and false", got.CanCancel, got.CanFollowup)
	}

	// Claude Code parks the session: still running, now idle.
	parked := s
	parked.Status = "idle"
	m.SyncAdopted([]sessions.Session{parked})
	if idle := m.List(Filter{IncludeAdopted: true})[0]; idle.Status != api.StatusRunning || idle.Activity != api.ActivityIdle {
		t.Fatalf("parked run = %q/%q", idle.Status, idle.Activity)
	}
	// A resumed session runs under a new pid; the registry reading wins.
	moved := s
	moved.PID, moved.Name = 4343, "nacre-eb-2"
	m.SyncAdopted([]sessions.Session{moved})
	if again := m.List(Filter{IncludeAdopted: true})[0]; again.OwnerPID != 4343 || again.SessionName != "nacre-eb-2" {
		t.Fatalf("after resume = pid %d, name %q", again.OwnerPID, again.SessionName)
	}
	// And the process going away is activity, not an error.
	m.SyncAdopted(nil)
	if ended := m.List(Filter{IncludeAdopted: true})[0]; ended.Activity != api.ActivityEnded || ended.Error != "" {
		t.Fatalf("ended run = %q (error %q)", ended.Activity, ended.Error)
	}
}

func TestDispatchedRunOmitsTheAdoptedFields(t *testing.T) {
	m, err := New(Options{Dir: t.TempDir(), ClaudePath: fakeClaude(t), MaxConcurrent: 1,
		Logger: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	r, err := m.Start(StartRequest{RepoID: "rid", RepoPath: t.TempDir(), IssueID: "X-1", Prompt: "do it"})
	if err != nil {
		t.Fatal(err)
	}
	got := waitStatus(t, m, r.ID, api.StatusSucceeded, api.StatusFailed)
	// The flags are absent, not true: a run nacred dispatched must serialize
	// exactly as it did before adopted runs existed.
	if got.Origin != "" || got.Agent != "" || got.Activity != "" || got.SessionName != "" || got.OwnerPID != 0 {
		t.Fatalf("dispatched run carries adopted-only fields: %+v", got)
	}
	if got.CanCancel != nil || got.CanFollowup != nil {
		t.Fatalf("capability flags = %v/%v, want nil so they stay off the wire", got.CanCancel, got.CanFollowup)
	}
	if n := m.AdoptedCount(""); n != 0 {
		t.Fatalf("AdoptedCount = %d, want 0: nacred started this one itself", n)
	}
}

func TestAdoptedCountIsSeparateFromActiveCount(t *testing.T) {
	m, err := New(Options{Dir: t.TempDir(), ClaudePath: fakeClaude(t), MaxConcurrent: 2,
		Logger: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	// One run nacred owns, still running, plus two sessions it only watches.
	own, err := m.Start(StartRequest{RepoID: "rid", RepoPath: t.TempDir(), IssueID: "X-1", Prompt: "SLEEP"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = m.Cancel(own.ID); m.Shutdown(2 * time.Second) })
	waitStatus(t, m, own.ID, api.StatusRunning)
	m.SyncAdopted([]sessions.Session{agentSession("sess-a", "NAC-7"), agentSession("sess-b", "")})

	// active_runs keeps its meaning: work nacred is doing, and nothing else.
	if n := m.ActiveCount(""); n != 1 {
		t.Fatalf("ActiveCount = %d, want 1 (only the dispatched run)", n)
	}
	if n := m.ActiveCount("rid"); n != 1 {
		t.Fatalf("repo ActiveCount = %d, want 1", n)
	}
	if n := m.AdoptedCount(""); n != 2 {
		t.Fatalf("AdoptedCount = %d, want 2", n)
	}
	if n := m.AdoptedCount("rid"); n != 2 {
		t.Fatalf("repo AdoptedCount = %d, want 2", n)
	}
	if n := m.AdoptedCount("other"); n != 0 {
		t.Fatalf("AdoptedCount(other) = %d, want 0", n)
	}
	// Ended sessions stop counting, the same way finished runs do.
	m.SyncAdopted(nil)
	if n := m.AdoptedCount(""); n != 0 {
		t.Fatalf("AdoptedCount after the sessions ended = %d, want 0", n)
	}
}

// TestCancelNeverSignalsAnAdoptedSession pins the guarantee behind the 409:
// not just that nacred refuses, but that the agent's process is untouched.
func TestCancelNeverSignalsAnAdoptedSession(t *testing.T) {
	victim := exec.Command("/bin/sh", "-c", "sleep 30")
	if err := victim.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = victim.Process.Kill(); _, _ = victim.Process.Wait() }()

	m := quietManager(t, t.TempDir())
	s := agentSession("sess-agent", "NAC-7")
	s.PID = victim.Process.Pid
	m.SyncAdopted([]sessions.Session{s})
	got := m.List(Filter{IncludeAdopted: true})[0]
	_, before, _ := m.Get(got.ID)

	if _, err := m.Cancel(got.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("Cancel = %v, want a conflict", err)
	}
	if _, err := m.Followup(got.ID, "carry on", ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("Followup = %v, want a conflict", err)
	}
	// SIGTERM to a human's interactive session is the thing being prevented.
	if err := victim.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("the agent's process was signalled: %v", err)
	}
	// And the refusal leaves no trace on the run either.
	after := m.List(Filter{IncludeAdopted: true})[0]
	if after.Status != api.StatusRunning || after.EndedAt != "" || after.Error != "" {
		t.Fatalf("refused cancel changed the run: %+v", after)
	}
	if _, evs, _ := m.Get(got.ID); len(evs) != len(before) {
		t.Fatalf("refused cancel wrote %d events", len(evs)-len(before))
	}
}

// A Codex session rides the same adopted-run path: agent "codex", no pid, a
// codex resume line, and the same read-only flags. Its rollout's events are
// folded in like a Claude Code transcript's, and it ends when the watcher
// stops reporting it.
func TestAdoptedCodexSession(t *testing.T) {
	m := quietManager(t, t.TempDir())
	s := agentSession("01a17081-82ba-7853-aaa7-9229fb3a78e2", "NAC-7")
	s.Agent, s.PID, s.Name, s.Version, s.Entrypoint, s.Status = api.AgentCodex, 0, "Fix the slow check", "0.159.2", "codex-tui", ""
	ts := time.Now().UTC().Format(time.RFC3339)
	s.Events = []api.RunEvent{
		{Kind: api.KindText, Text: "Claiming the bead.", TS: ts},
		{Kind: api.KindToolUse, Tool: "Bash", ToolID: "exec-1", Text: "bd update NAC-7 --claim", TS: ts},
	}
	s.FromStart = true
	s.LastWrite = time.Now()
	s.TranscriptState.Entries, s.TranscriptState.ToolCalls, s.TranscriptState.Model = 4, 1, "gpt-6.1-sol"
	s.TranscriptState.LastText, s.TranscriptState.LastEntryTS = "Claiming the bead.", ts
	m.SyncAdopted([]sessions.Session{s})

	got := m.List(Filter{IncludeAdopted: true})
	if len(got) != 1 {
		t.Fatalf("runs = %+v", got)
	}
	r := got[0]
	if r.Origin != api.OriginAdopted || r.Agent != api.AgentCodex || r.OwnerPID != 0 || r.IssueID != "NAC-7" {
		t.Fatalf("run = %+v", r)
	}
	if r.Model != "gpt-6.1-sol" || r.ToolCalls != 1 || r.LastText != "Claiming the bead." || r.Activity != api.ActivityWorking {
		t.Fatalf("folded state = model %q, tools %d, last %q, activity %q", r.Model, r.ToolCalls, r.LastText, r.Activity)
	}
	if !strings.Contains(r.ResumeCommand, "codex resume "+s.SessionID) {
		t.Fatalf("resume command = %q", r.ResumeCommand)
	}
	if r.CanCancel == nil || *r.CanCancel || r.CanFollowup == nil || *r.CanFollowup {
		t.Fatalf("capability flags = %v/%v", r.CanCancel, r.CanFollowup)
	}
	_, evs, _ := m.Get(r.ID)
	if !strings.Contains(evs[0].Text, "adopted Codex session") || strings.Contains(evs[0].Text, "pid 0") {
		t.Fatalf("notice = %q", evs[0].Text)
	}
	var tools int
	for _, ev := range evs {
		if ev.Kind == api.KindToolUse && ev.Tool == "Bash" {
			tools++
		}
	}
	if tools != 1 {
		t.Fatalf("events = %+v", evs)
	}

	// Between turns the watcher marks it idle, as Claude Code does.
	parked := s
	parked.Events, parked.FromStart, parked.Status = nil, false, "idle"
	m.SyncAdopted([]sessions.Session{parked})
	if idle := m.List(Filter{IncludeAdopted: true})[0]; idle.Status != api.StatusRunning || idle.Activity != api.ActivityIdle {
		t.Fatalf("parked = %q/%q", idle.Status, idle.Activity)
	}

	m.SyncAdopted(nil)
	ended, evs, _ := m.Get(r.ID)
	if ended.Activity != api.ActivityEnded || ended.Status != api.StatusSucceeded || ended.Error != "" {
		t.Fatalf("ended = %+v", ended)
	}
	if last := evs[len(evs)-2]; !strings.Contains(last.Text, "Codex") {
		t.Fatalf("end notice = %q", last.Text)
	}
}
