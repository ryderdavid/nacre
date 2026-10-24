package runs

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
	"github.com/ryderdavid/nacre/bridge/internal/sessions"
)

// liveProcs reports every pid as alive since start: the watcher's liveness
// check is not what these tests are about.
type liveProcs struct{ start time.Time }

func (p liveProcs) StartTimes(pids []int) map[int]time.Time {
	out := map[int]time.Time{}
	for _, pid := range pids {
		out[pid] = p.start
	}
	return out
}

const agentSID = "aaaaaaaa-0000-0000-0000-0000000000a1"

// transcriptFixture is a short agent session as Claude Code writes it to
// ~/.claude/projects: a prompt, a turn with text and a Bash call, the tool's
// result, a second turn and a cost-state entry with the running totals.
func transcriptFixture(at time.Time) string {
	ts := func(sec int) string {
		return at.Add(time.Duration(sec) * time.Second).UTC().Format("2006-01-02T15:04:05.000Z")
	}
	return strings.Join([]string{
		`{"type":"user","timestamp":"` + ts(0) + `","sessionId":"` + agentSID + `","message":{"role":"user","content":"Fix the parser"}}`,
		`{"type":"assistant","timestamp":"` + ts(1) + `","message":{"model":"claude-opus-5-5","content":[{"type":"text","text":"Looking at the parser."},{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"go test ./..."}}]}}`,
		`{"type":"user","timestamp":"` + ts(2) + `","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok  parser"}]}}`,
		`{"type":"assistant","timestamp":"` + ts(3) + `","message":{"model":"claude-opus-5-5","content":[{"type":"text","text":"Tests pass."}]}}`,
		`{"type":"cost-state","timestamp":"` + ts(4) + `","totalCostUSD":1.25,"totalDuration":61000,"modelUsage":{"claude-opus-5-5":{}}}`,
	}, "\n") + "\n"
}

// agentWatcher is a session watcher over a temporary registry and projects
// directory holding one live agent session in repo "rid", feeding m.
func agentWatcher(t *testing.T, m *Manager, transcript string) (w *sessions.Watcher, path string) {
	t.Helper()
	reg, projects, repoPath := t.TempDir(), t.TempDir(), t.TempDir()
	start := time.Now().Add(-time.Hour).Truncate(time.Second)
	rec := fmt.Sprintf(`{"pid":4242,"sessionId":%q,"cwd":%q,"startedAt":%d,"procStart":%q,"name":"agent"}`,
		agentSID, repoPath, start.UnixMilli(), start.UTC().Format("Mon Jan _2 15:04:05 2006"))
	if err := os.WriteFile(filepath.Join(reg, "4242.json"), []byte(rec), 0o600); err != nil {
		t.Fatal(err)
	}
	pdir := filepath.Join(projects, "-repo")
	if err := os.MkdirAll(pdir, 0o700); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(pdir, agentSID+".jsonl")
	if err := os.WriteFile(path, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	w = sessions.New(sessions.Options{
		Dir: reg, Projects: projects, Procs: liveProcs{start},
		MainCheckout: func(string) string { return "" },
		Repos:        func() []sessions.Repo { return []sessions.Repo{{ID: "rid", Name: "repo", Path: repoPath}} },
		Owned:        m.OwnsSession,
		Sync:         m.SyncAdopted,
		Logger:       log.New(io.Discard, "", 0),
	})
	return w, path
}

func kindCounts(evs []api.RunEvent) map[string]int {
	out := map[string]int{}
	for _, ev := range evs {
		out[ev.Kind]++
	}
	return out
}

// The acceptance criterion: GET /v1/runs/{id} for an adopted run returns what
// the agent is doing — its text and tool calls from the live transcript — and
// the run's cost and counts come from the transcript's cost-state entry.
func TestAdoptedRunCarriesTheTranscript(t *testing.T) {
	m := quietManager(t, t.TempDir())
	w, path := agentWatcher(t, m, transcriptFixture(time.Now().Add(-10*time.Second)))
	w.Poll()

	list := m.List(Filter{IncludeAdopted: true})
	if len(list) != 1 {
		t.Fatalf("adopted runs = %+v", list)
	}
	meta, evs, err := m.Get(list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	k := kindCounts(evs)
	if k[api.KindText] != 2 || k[api.KindToolUse] != 1 || k[api.KindToolResult] != 1 || k[api.KindUser] != 1 {
		t.Fatalf("events by kind = %v: want the agent's text, tool call, result and prompt", k)
	}
	if evs[0].Kind != api.KindSystem || !strings.Contains(evs[0].Text, "adopted Claude Code session") {
		t.Fatalf("first event = %+v, want the adoption notice", evs[0])
	}
	var tool api.RunEvent
	for _, ev := range evs {
		if ev.Kind == api.KindToolUse {
			tool = ev
		}
	}
	if tool.Tool != "Bash" || tool.ToolID != "toolu_1" || !strings.Contains(tool.Text, "go test") {
		t.Fatalf("tool event = %+v", tool)
	}
	if !strings.HasSuffix(tool.TS, "Z") || !strings.Contains(tool.TS, ".") {
		t.Fatalf("tool event ts = %q, want the transcript's own timestamp", tool.TS)
	}
	for i := 1; i < len(evs); i++ {
		if evs[i].Seq <= evs[i-1].Seq {
			t.Fatalf("seq not increasing at %d: %+v", i, evs)
		}
	}
	if meta.CostUSD != 1.25 || meta.DurationMS != 61000 || meta.ToolCalls != 1 || meta.NumTurns != 2 {
		t.Fatalf("meta totals = cost %v, duration %v, tools %d, turns %d", meta.CostUSD, meta.DurationMS, meta.ToolCalls, meta.NumTurns)
	}
	if meta.LastText != "Tests pass." || meta.Model != "claude-opus-5-5" || meta.Turns != 1 || meta.LastActivityAt == "" {
		t.Fatalf("meta = %+v", meta)
	}
	if meta.Activity != api.ActivityWorking || meta.Stale {
		t.Fatalf("activity = %q stale = %v, want working: the transcript was just written", meta.Activity, meta.Stale)
	}

	// The next poll adds only what the transcript gained.
	before := len(evs)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	fmt.Fprintf(f, `{"type":"assistant","timestamp":%q,"message":{"model":"claude-opus-5-5","content":[{"type":"tool_use","id":"toolu_2","name":"Read","input":{"file_path":"/x/parser.go"}}]}}`+"\n", ts)
	f.Close()
	w.Poll()
	meta, evs, _ = m.Get(list[0].ID)
	if len(evs) != before+1 || evs[len(evs)-1].ToolID != "toolu_2" {
		t.Fatalf("after append: %d events (was %d), last %+v", len(evs), before, evs[len(evs)-1])
	}
	if meta.ToolCalls != 2 || meta.NumTurns != 3 {
		t.Fatalf("after append: tools %d turns %d", meta.ToolCalls, meta.NumTurns)
	}

	// A poll with nothing new changes nothing.
	w.Poll()
	if _, again, _ := m.Get(list[0].ID); len(again) != len(evs) {
		t.Fatalf("idle poll added events: %d -> %d", len(evs), len(again))
	}
}

// A restarted nacred tails every transcript from the beginning again; what the
// run already holds must not be appended a second time.
func TestAdoptedTranscriptReplayAfterRestartIsNotDuplicated(t *testing.T) {
	dir := t.TempDir()
	m := quietManager(t, dir)
	w, path := agentWatcher(t, m, transcriptFixture(time.Now().Add(-10*time.Second)))
	w.Poll()
	id := m.List(Filter{IncludeAdopted: true})[0].ID
	_, evs, _ := m.Get(id)

	m2 := quietManager(t, dir)
	w2, _ := agentWatcherAt(t, m2, path)
	w2.Poll()
	meta, again, err := m2.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != len(evs) {
		t.Fatalf("after restart: %d events, want %d (no replayed duplicates)", len(again), len(evs))
	}
	if meta.CostUSD != 1.25 || meta.ToolCalls != 1 {
		t.Fatalf("after restart meta = %+v", meta)
	}
}

// agentWatcherAt is agentWatcher over an existing transcript file, as after a
// restart of nacred.
func agentWatcherAt(t *testing.T, m *Manager, path string) (*sessions.Watcher, string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	w, p := agentWatcher(t, m, string(data))
	// Keep the transcript's mtime, which agentWatcher's rewrite reset.
	fi, _ := os.Stat(path)
	_ = os.Chtimes(p, fi.ModTime(), fi.ModTime())
	return w, p
}

// Ryder's decision 1 (NAC-j5z): an adopted session counts as an active job
// only while its transcript keeps changing. One quiet past the idle window is
// idle and stale: still a live run, so its bead keeps showing it, but out of
// the runs list and the adopted count.
func TestAdoptedActivityFollowsTranscriptFreshness(t *testing.T) {
	m := quietManager(t, t.TempDir())
	s := agentSession("sess-agent", "NAC-7")
	s.LastWrite = time.Now()
	m.SyncAdopted([]sessions.Session{s})
	run := m.List(Filter{IncludeAdopted: true})[0]
	if run.Activity != api.ActivityWorking || run.Stale {
		t.Fatalf("fresh session: activity %q stale %v", run.Activity, run.Stale)
	}
	if n := m.AdoptedCount("rid"); n != 1 {
		t.Fatalf("AdoptedCount = %d, want 1", n)
	}

	// Parked by Claude Code but written a moment ago: idle, waiting for its
	// human, and still a job in progress.
	s.Record.Status = "idle"
	m.SyncAdopted([]sessions.Session{s})
	run = m.List(Filter{IncludeAdopted: true})[0]
	if run.Activity != api.ActivityIdle || run.Stale {
		t.Fatalf("parked session: activity %q stale %v", run.Activity, run.Stale)
	}

	// Quiet for longer than the window, whatever the record says: the record's
	// status alone would still say working.
	s.Record.Status = ""
	s.LastWrite = time.Now().Add(-DefaultAdoptIdleAfter - time.Minute)
	s.Record.StartedAt = time.Now().Add(-time.Hour).UnixMilli()
	m.SyncAdopted([]sessions.Session{s})
	all := m.List(Filter{IncludeAdopted: true, IncludeStale: true})
	if len(all) != 1 || all[0].Activity != api.ActivityIdle || !all[0].Stale || !all[0].Active() {
		t.Fatalf("quiet session = %+v, want a live, idle, stale run", all)
	}
	if listed := m.List(Filter{IncludeAdopted: true}); len(listed) != 0 {
		t.Fatalf("runs list = %+v, want the stale session left out", listed)
	}
	if n := m.AdoptedCount("rid"); n != 0 {
		t.Fatalf("AdoptedCount = %d, want 0 for a stale session", n)
	}
	// The bead's own view still finds it.
	if onBead := m.List(Filter{IssueID: "NAC-7", IncludeAdopted: true, IncludeStale: true}); len(onBead) != 1 {
		t.Fatalf("bead view = %+v", onBead)
	}

	// The agent picks the session back up.
	s.LastWrite = time.Now()
	m.SyncAdopted([]sessions.Session{s})
	if back := m.List(Filter{IncludeAdopted: true}); len(back) != 1 || back[0].Activity != api.ActivityWorking || back[0].Stale {
		t.Fatalf("resumed session = %+v", back)
	}

	// An ended session is no longer stale, just over.
	s.LastWrite = time.Now().Add(-time.Hour)
	m.SyncAdopted([]sessions.Session{s})
	m.SyncAdopted(nil)
	if ended := m.List(Filter{IncludeAdopted: true}); len(ended) != 1 || ended[0].Stale || ended[0].Activity != api.ActivityEnded {
		t.Fatalf("ended session = %+v", ended)
	}
}

func TestAdoptIdleWindowIsConfigurable(t *testing.T) {
	m, err := New(Options{Dir: t.TempDir(), ClaudePath: "/nonexistent", AdoptIdleAfter: time.Hour,
		Logger: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	s := agentSession("sess-agent", "NAC-7")
	s.LastWrite = time.Now().Add(-30 * time.Minute)
	m.SyncAdopted([]sessions.Session{s})
	if got := m.List(Filter{IncludeAdopted: true}); len(got) != 1 || got[0].Activity != api.ActivityWorking || got[0].Stale {
		t.Fatalf("30 minutes quiet with a one-hour window = %+v", got)
	}

	short, err := New(Options{Dir: t.TempDir(), ClaudePath: "/nonexistent", AdoptIdleAfter: time.Minute,
		Logger: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	s.LastWrite = time.Now().Add(-2 * time.Minute)
	short.SyncAdopted([]sessions.Session{s})
	if got := short.List(Filter{IncludeAdopted: true, IncludeStale: true}); len(got) != 1 || !got[0].Stale {
		t.Fatalf("2 minutes quiet with a one-minute window = %+v", got)
	}
}

// A session with no transcript yet is as fresh as its own start.
func TestNewSessionWithoutTranscriptIsFreshFromItsStart(t *testing.T) {
	m := quietManager(t, t.TempDir())
	fresh := agentSession("sess-new", "")
	old := agentSession("sess-old", "")
	old.Record.PID = 4343
	old.Record.StartedAt = time.Now().Add(-time.Hour).UnixMilli()
	m.SyncAdopted([]sessions.Session{fresh, old})
	all := m.List(Filter{IncludeAdopted: true, IncludeStale: true})
	if len(all) != 2 {
		t.Fatalf("runs = %+v", all)
	}
	for _, r := range all {
		if want := r.SessionID == "sess-old"; r.Stale != want {
			t.Errorf("%s stale = %v, want %v", r.SessionID, r.Stale, want)
		}
	}
}

// A long-lived session must cost bounded memory: only the newest events are
// held, while seq numbers keep counting.
func TestAdoptedRunEventsAreBounded(t *testing.T) {
	m := quietManager(t, t.TempDir())
	s := agentSession("sess-agent", "NAC-7")
	s.LastWrite = time.Now()
	base := time.Now().Add(-time.Hour)
	for i := 0; i < maxAdoptedEvents+50; i++ {
		s.Events = append(s.Events, api.RunEvent{Kind: api.KindText, Text: fmt.Sprint(i),
			TS: base.Add(time.Duration(i) * time.Millisecond).UTC().Format(time.RFC3339Nano)})
	}
	m.SyncAdopted([]sessions.Session{s})
	id := m.List(Filter{IncludeAdopted: true})[0].ID
	_, evs, _ := m.Get(id)
	if len(evs) != maxAdoptedEvents {
		t.Fatalf("held %d events, want %d", len(evs), maxAdoptedEvents)
	}
	if last := evs[len(evs)-1]; last.Text != fmt.Sprint(maxAdoptedEvents+49) || last.Seq != int64(maxAdoptedEvents+52) {
		t.Fatalf("newest event = %+v", last)
	}
}

func TestAdoptedActivityVocabularyMatchesTheAdapter(t *testing.T) {
	if claude.ActivityWorking != api.ActivityWorking || claude.ActivityIdle != api.ActivityIdle || claude.ActivityEnded != api.ActivityEnded {
		t.Fatal("claude.DeriveSessionStatus's activities are stored on api.Run as they are")
	}
}

// bulkyEvents is n transcript events of about 1 KiB each, so a few thousand
// carry an adopted run's log past adoptedLogBytes.
func bulkyEvents(n, from int, base time.Time) []api.RunEvent {
	pad := strings.Repeat("x", 1024)
	out := make([]api.RunEvent, 0, n)
	for i := from; i < from+n; i++ {
		out = append(out, api.RunEvent{Kind: api.KindText, Text: fmt.Sprint(i, " ", pad),
			TS: base.Add(time.Duration(i) * time.Millisecond).UTC().Format(time.RFC3339Nano)})
	}
	return out
}

// A long-lived session must cost bounded disk too: once the log passes the
// threshold it is rewritten to the newest events, seq numbers intact, through
// a temporary file that does not outlive the rename.
func TestAdoptedRunLogIsCompacted(t *testing.T) {
	dir := t.TempDir()
	m := quietManager(t, dir)
	s := agentSession("sess-agent", "NAC-7")
	s.LastWrite = time.Now()
	n := adoptedLogBytes/1024 + 500
	s.Events = bulkyEvents(n, 0, time.Now().Add(-time.Hour))
	m.SyncAdopted([]sessions.Session{s})
	id := m.List(Filter{IncludeAdopted: true})[0].ID
	rdir := filepath.Join(dir, id)

	if size := eventsSize(rdir); size > adoptedLogBytes {
		t.Fatalf("log is %d bytes, want at most %d", size, adoptedLogBytes)
	}
	if _, err := os.Stat(filepath.Join(rdir, eventsFile+".tmp")); !os.IsNotExist(err) {
		t.Fatalf("temporary log left behind: %v", err)
	}
	disk, _ := readEvents(rdir, 0)
	if len(disk) != maxAdoptedEvents {
		t.Fatalf("log holds %d events, want %d", len(disk), maxAdoptedEvents)
	}
	// Two events (the adoption notice and the running status) precede the
	// transcript, so the newest transcript event i has seq i+3.
	if last := disk[len(disk)-1]; !strings.HasPrefix(last.Text, fmt.Sprint(n-1, " ")) || last.Seq != int64(n+2) {
		t.Fatalf("newest logged event = seq %d %.10q", last.Seq, last.Text)
	}
	for i := 1; i < len(disk); i++ {
		if disk[i].Seq != disk[i-1].Seq+1 {
			t.Fatalf("seq gap at %d: %d then %d", i, disk[i-1].Seq, disk[i].Seq)
		}
	}

	// Appends after a compaction land on the compacted log.
	s.Events = bulkyEvents(10, n, time.Now().Add(-time.Hour))
	m.SyncAdopted([]sessions.Session{s})
	disk, _ = readEvents(rdir, 0)
	if len(disk) != maxAdoptedEvents+10 || disk[len(disk)-1].Seq != int64(n+12) {
		t.Fatalf("after append: %d events, newest seq %d", len(disk), disk[len(disk)-1].Seq)
	}
}

// A restart reads back the compacted tail, and seq numbers carry on from it.
func TestAdoptedRunRestoresCompactedLogAfterRestart(t *testing.T) {
	dir := t.TempDir()
	m := quietManager(t, dir)
	s := agentSession("sess-agent", "NAC-7")
	s.LastWrite = time.Now()
	n := adoptedLogBytes/1024 + 500
	s.Events = bulkyEvents(n, 0, time.Now().Add(-time.Hour))
	m.SyncAdopted([]sessions.Session{s})
	id := m.List(Filter{IncludeAdopted: true})[0].ID
	_, before, _ := m.Get(id)

	m2 := quietManager(t, dir)
	_, after, err := m2.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) || after[0].Seq != before[0].Seq || after[len(after)-1].Seq != before[len(before)-1].Seq {
		t.Fatalf("restored seq %d..%d (%d events), want %d..%d (%d)", after[0].Seq, after[len(after)-1].Seq,
			len(after), before[0].Seq, before[len(before)-1].Seq, len(before))
	}
	m2.SyncAdopted([]sessions.Session{agentSession("sess-agent", "NAC-8")})
	_, evs, _ := m2.Get(id)
	if last := evs[len(evs)-1]; last.Seq != before[len(before)-1].Seq+1 || len(evs) != maxAdoptedEvents {
		t.Fatalf("after restart: %d events, newest = %+v", len(evs), last)
	}
}

// A log left oversized by an older nacred is read back only as its tail and
// compacted on load.
func TestAdoptedRunCompactsAnOversizedLogOnLoad(t *testing.T) {
	dir := t.TempDir()
	m := quietManager(t, dir)
	m.SyncAdopted([]sessions.Session{agentSession("sess-agent", "NAC-7")})
	id := m.List(Filter{IncludeAdopted: true})[0].ID
	rdir := filepath.Join(dir, id)
	n := adoptedLogBytes/1024 + 500
	evs := bulkyEvents(n, 0, time.Now().Add(-time.Hour))
	for i := range evs {
		evs[i].Seq = int64(i + 1)
	}
	if err := appendEvent(rdir, evs...); err != nil {
		t.Fatal(err)
	}

	m2 := quietManager(t, dir)
	_, got, _ := m2.Get(id)
	if len(got) != maxAdoptedEvents || got[len(got)-1].Seq != int64(n) {
		t.Fatalf("loaded %d events, newest seq %d", len(got), got[len(got)-1].Seq)
	}
	if size := eventsSize(rdir); size > adoptedLogBytes {
		t.Fatalf("log is still %d bytes", size)
	}
}

// Runs started from Nacre keep their whole log, however large.
func TestDispatchedRunLogIsNotCompacted(t *testing.T) {
	rdir := t.TempDir()
	r := newRun(api.Run{ID: "r_X", Status: api.StatusRunning}, rdir)
	r.loaded = true
	n := adoptedLogBytes/1024 + 500
	evs := bulkyEvents(n, 0, time.Now())
	r.mu.Lock()
	r.importEvents(evs)
	r.mu.Unlock()
	if len(r.events) != n {
		t.Fatalf("held %d events, want %d", len(r.events), n)
	}
	if disk, _ := readEvents(rdir, 0); len(disk) != n {
		t.Fatalf("log holds %d events, want %d", len(disk), n)
	}
}
