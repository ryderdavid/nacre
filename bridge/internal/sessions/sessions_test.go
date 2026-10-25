package sessions

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// fakeProcs is a ProcTable with hand-written process start times.
type fakeProcs map[int]time.Time

func (f fakeProcs) StartTimes(pids []int) map[int]time.Time {
	out := map[int]time.Time{}
	for _, pid := range pids {
		if t, ok := f[pid]; ok {
			out[pid] = t
		}
	}
	return out
}

func writeRecord(t *testing.T, dir string, rec Record) Record {
	t.Helper()
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, strconv.Itoa(rec.PID)+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	rec.File = path
	return rec
}

// record builds a plausible registry record for a session started at start.
func record(pid int, sid, cwd, name string, start time.Time) Record {
	return Record{
		PID: pid, SessionID: sid, CWD: cwd, Name: name,
		StartedAt: start.UnixMilli(), UpdatedAt: start.UnixMilli(),
		ProcStart: start.UTC().Format(procStartLayout),
		Version:   "2.1.304", Kind: "interactive", Entrypoint: "claude-desktop",
	}
}

func TestRecordProcStartIsUTC(t *testing.T) {
	// Verified against a real record: procStart is formatted in UTC, while
	// ps(1) prints the same instant in local time.
	rec := Record{ProcStart: "Fri Oct 23 18:16:48 2026", StartedAt: 1792779408936}
	want := time.Date(2026, time.October, 23, 18, 16, 48, 0, time.UTC)
	if got := rec.ProcStarted(); !got.Equal(want) {
		t.Fatalf("ProcStarted = %s, want %s", got, want)
	}
	if got := rec.Started(); !got.Equal(want.Add(936 * time.Millisecond)) {
		t.Fatalf("Started = %s", got)
	}
	// A space-padded single-digit day (asctime style) parses too.
	if got := (Record{ProcStart: "Fri Oct  2 08:00:00 2026"}).ProcStarted(); got.Day() != 2 {
		t.Fatalf("padded day: %s", got)
	}
	if got := (Record{ProcStart: "nonsense"}).ProcStarted(); !got.IsZero() {
		t.Fatalf("unparseable procStart = %s, want zero", got)
	}
}

func TestReadDirSkipsNonRecords(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	writeRecord(t, dir, record(200, "s-200", "/a", "two", now))
	writeRecord(t, dir, record(100, "s-100", "/a", "one", now))
	// A record still missing its session id, a torn write, and the .key files
	// that share the directory must all be ignored.
	writeRecord(t, dir, record(300, "", "/a", "none", now))
	mustWrite(t, filepath.Join(dir, "400.json"), `{"pid":400,`)
	mustWrite(t, filepath.Join(dir, "100.abc.key"), "secret")

	recs, err := ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || recs[0].PID != 100 || recs[1].PID != 200 {
		t.Fatalf("records = %+v", recs)
	}
	if recs[0].File != filepath.Join(dir, "100.json") {
		t.Fatalf("File = %q", recs[0].File)
	}
	// A registry that does not exist yet is not an error.
	if recs, err := ReadDir(filepath.Join(dir, "missing")); err != nil || recs != nil {
		t.Fatalf("missing dir: %v %+v", err, recs)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLiveRejectsDeadAndReusedPids(t *testing.T) {
	start := time.Now().Truncate(time.Second)
	rec := record(42, "s", "/a", "n", start)
	cases := []struct {
		name  string
		procs fakeProcs
		want  bool
	}{
		{"alive", fakeProcs{42: start}, true},
		{"alive within skew", fakeProcs{42: start.Add(time.Second)}, true},
		{"dead", fakeProcs{}, false},
		{"pid reused by a newer process", fakeProcs{42: start.Add(time.Hour)}, false},
		{"pid reused by an older process", fakeProcs{42: start.Add(-time.Hour)}, false},
	}
	for _, c := range cases {
		if got := live(rec, c.procs.StartTimes([]int{42})); got != c.want {
			t.Errorf("%s: live = %v, want %v", c.name, got, c.want)
		}
	}
	// Without a usable procStart the record is dropped rather than matched
	// against whatever now holds the pid.
	noStart := rec
	noStart.ProcStart = ""
	if live(noStart, fakeProcs{42: start}.StartTimes([]int{42})) {
		t.Error("record with no procStart must not be live")
	}
}

func TestParsePSLine(t *testing.T) {
	pid, start, ok := parsePSLine("  27721 Fri Oct 23 11:16:48 2026    ")
	if !ok || pid != 27721 {
		t.Fatalf("ok=%v pid=%d", ok, pid)
	}
	want := time.Date(2026, time.October, 23, 11, 16, 48, 0, time.Local)
	if !start.Equal(want) {
		t.Fatalf("start = %s, want %s", start, want)
	}
	for _, bad := range []string{"", "   ", "pid lstart", "123", "x Fri Oct 23 11:16:48 2026"} {
		if _, _, ok := parsePSLine(bad); ok {
			t.Errorf("parsePSLine(%q) should fail", bad)
		}
	}
}

func TestResolveRepo(t *testing.T) {
	list := []Repo{
		{ID: "a", Name: "Nacre", Path: "/code/Nacre"},
		{ID: "b", Name: "inner", Path: "/code/Nacre/vendor/inner"},
	}
	// An external worktree of Nacre; its main checkout is /code/Nacre.
	main := func(dir string) string {
		if dir == "/tmp/Nacre-wt" {
			return "/code/Nacre"
		}
		return ""
	}
	cases := []struct {
		cwd  string
		want string
	}{
		{"/code/Nacre", "a"},
		{"/code/Nacre/bridge/internal", "a"},
		{"/code/Nacre/.claude/worktrees/wt", "a"}, // worktree inside the repo
		{"/code/Nacre/vendor/inner/x", "b"},       // the nested repo wins
		{"/tmp/Nacre-wt", "a"},                    // worktree outside every repo
		{"/tmp/Nacre-wt/bridge", ""},              // git lookup answers for the cwd only
		{"/elsewhere", ""},
		{"", ""},
	}
	for _, c := range cases {
		got, ok := resolveRepo(c.cwd, list, main)
		if (c.want == "") == ok || got.ID != c.want {
			t.Errorf("resolveRepo(%q) = %q,%v want %q", c.cwd, got.ID, ok, c.want)
		}
	}
	// Without a worktree resolver only the discovered paths are considered.
	if _, ok := resolveRepo("/tmp/Nacre-wt", list, nil); ok {
		t.Error("no main-checkout resolver: should not match")
	}
}

func TestMentionFrom(t *testing.T) {
	cases := []struct {
		cmd    string
		id     string
		weight int
	}{
		{"bd update NAC-4gu --claim", "NAC-4gu", weightClaim},
		{"bd update --claim -- NAC-j5z.1", "NAC-j5z.1", weightClaim},
		{"bd update NAC-9 --status in_progress", "NAC-9", weightClaim},
		{"bd update NAC-9 --status=hooked", "NAC-9", weightClaim},
		{"/opt/homebrew/bin/bd -C /code/Nacre update NAC-9 --claim", "NAC-9", weightClaim},
		{"bd close NAC-h1m.4", "NAC-h1m.4", weightClose},
		{"bd show NAC-h1m.4", "NAC-h1m.4", weightRead},
		{"bd comments NAC-h1m.4", "NAC-h1m.4", weightRead},
		// Not a statement about who is working on what.
		{"bd update NAC-9 --description hello", "", 0},
		{"bd update NAC-9 --status closed", "", 0},
		{"bd ready", "", 0},
		{"bd list --status=open", "", 0},
		{"bdg show NAC-9", "", 0},
		{"echo bd show NAC-9", "", 0},
		{"bd", "", 0},
	}
	for _, c := range cases {
		got := commandMentions(c.cmd)
		if c.id == "" {
			if len(got) != 0 {
				t.Errorf("%q = %+v, want none", c.cmd, got)
			}
			continue
		}
		if len(got) != 1 || got[0].id != c.id || got[0].weight != c.weight {
			t.Errorf("%q = %+v, want %s/%d", c.cmd, got, c.id, c.weight)
		}
	}
	// A compound command yields the bd invocation inside it.
	got := commandMentions("cd /code/Nacre && bd update NAC-9 --claim | tee log")
	if len(got) != 1 || got[0].id != "NAC-9" {
		t.Fatalf("compound = %+v", got)
	}
}

// transcriptLine renders one assistant entry carrying a Bash tool call.
func transcriptLine(cmd string, sidechain bool) string {
	ent := map[string]any{
		"type":        "assistant",
		"isSidechain": sidechain,
		"message": map[string]any{"content": []any{
			map[string]any{"type": "tool_use", "id": "t1", "name": "Bash",
				"input": map[string]any{"command": cmd}},
		}},
	}
	data, _ := json.Marshal(ent)
	return string(data) + "\n"
}

// parsed runs one transcript line through the parser the transcript tail uses.
func parsed(t *testing.T, line string) claude.TranscriptParsed {
	t.Helper()
	p, err := claude.ParseTranscriptEntry([]byte(line))
	if err != nil {
		t.Fatalf("parse %q: %v", line, err)
	}
	return p
}

func TestBdMentionsIgnoresEverythingElse(t *testing.T) {
	if got := bdMentions(parsed(t, transcriptLine("bd show NAC-9", false))); len(got) != 1 {
		t.Fatalf("assistant Bash call = %+v", got)
	}
	// Subagent turns live in the same file but are a different agent's work.
	if got := bdMentions(parsed(t, transcriptLine("bd update NAC-9 --claim", true))); got != nil {
		t.Errorf("sidechain = %+v, want none", got)
	}
	for _, line := range []string{
		`{"type":"user","message":{"content":"run bd show NAC-9 please"}}`,
		`{"type":"cost-state","totalCostUSD":0.5}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"I will bd show NAC-9"}]}}`,
		`{"type":"assistant","message":{"content":"bd show NAC-9"}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"command":"bd show NAC-9"}}]}}`,
	} {
		if got := bdMentions(parsed(t, line)); got != nil {
			t.Errorf("%s\n = %+v, want none", line, got)
		}
	}
	// A command too big for the tail to retain is still seen whole.
	big := strings.Repeat("x", claude.MaxTranscriptToolInput) + " && bd close NAC-9"
	if got := bdMentions(parsed(t, transcriptLine(big, false))); len(got) != 1 || got[0].id != "NAC-9" {
		t.Errorf("long command = %+v", got)
	}
}

// observeLines feeds transcript lines to the scanner as the tail would.
func observeLines(t *testing.T, s *scanner, sid string, lines ...string) {
	t.Helper()
	for _, l := range lines {
		s.observe(sid, parsed(t, l))
	}
}

func TestScannerPrefersTheStrongestSignal(t *testing.T) {
	s := newScanner()
	valid := func(id string) bool { return id == "NAC-1" || id == "NAC-2" }

	observeLines(t, s, "sid", transcriptLine("bd show NAC-1", false))
	if id, ok := s.best("sid", valid); !ok || id != "NAC-1" {
		t.Fatalf("first = %q,%v", id, ok)
	}
	// Nothing new: the best so far is kept.
	if id, _ := s.best("sid", valid); id != "NAC-1" {
		t.Fatalf("again = %q, want NAC-1", id)
	}
	// A claim appearing later outranks the earlier read.
	observeLines(t, s, "sid", transcriptLine("bd update NAC-2 --claim", false))
	if id, ok := s.best("sid", valid); !ok || id != "NAC-2" {
		t.Fatalf("after claim = %q,%v", id, ok)
	}
	// A weaker signal afterwards does not displace it.
	observeLines(t, s, "sid", transcriptLine("bd show NAC-1", false))
	if id, _ := s.best("sid", valid); id != "NAC-2" {
		t.Fatalf("after later read = %q, want NAC-2", id)
	}
	// An id that is not a bead of this repo is never trusted.
	observeLines(t, s, "sid", transcriptLine("bd update OTHER-1 --claim", false))
	if id, _ := s.best("sid", valid); id != "NAC-2" {
		t.Fatalf("unvalidated id = %q, want NAC-2", id)
	}
	// A restarted transcript forgets what came before it, but not what was
	// read from it since.
	mark := s.mark("sid")
	observeLines(t, s, "sid", transcriptLine("bd show NAC-1", false))
	s.restart("sid", mark)
	if id, _ := s.best("sid", valid); id != "NAC-1" {
		t.Fatalf("after restart = %q, want NAC-1", id)
	}

	if id, ok := s.best("other", valid); ok || id != "" {
		t.Fatalf("nothing observed = %q,%v", id, ok)
	}
	s.forget("sid")
	if _, ok := s.state["sid"]; ok {
		t.Fatal("forget left state behind")
	}
}

func appendTo(t *testing.T, path, body string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(body); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func TestTranscriptPath(t *testing.T) {
	projects := t.TempDir()
	sid := "c704d257-8d1e-4bf5-9821-9cc7b84c5131"
	small := filepath.Join(projects, "-code-Nacre")
	big := filepath.Join(projects, "-code-Nacre--worktrees-wt")
	for _, d := range []string{small, big} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(t, filepath.Join(small, sid+".jsonl"), "a\n")
	mustWrite(t, filepath.Join(big, sid+".jsonl"), "aaaaaaaaaa\n")
	// The cwd-to-directory escaping is lossy, so the id is globbed for; when
	// more than one file matches, the one with content is the live transcript.
	if got := transcriptPath(projects, sid); got != filepath.Join(big, sid+".jsonl") {
		t.Fatalf("transcriptPath = %q", got)
	}
	if got := transcriptPath(projects, "unknown-0000"); got != "" {
		t.Fatalf("unknown session = %q", got)
	}
	// A session id is interpolated into a glob, so anything unlike a uuid is
	// refused rather than matched loosely.
	if got := transcriptPath(projects, "../*"); got != "" {
		t.Fatalf("glob metacharacters = %q", got)
	}
}

func TestLinkPrecedence(t *testing.T) {
	projects := t.TempDir()
	dir := filepath.Join(projects, "-code-Nacre")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	sid := "aaaaaaaa-1111-2222-3333-444444444444"
	transcript := filepath.Join(dir, sid+".jsonl")
	mustWrite(t, transcript, transcriptLine("bd update NAC-2 --claim", false))

	start := time.Now().Add(-time.Minute)
	beads := []Bead{
		{ID: "NAC-1", Title: "named", Status: "in_progress"},
		{ID: "NAC-2", Title: "claimed in the transcript", Status: "in_progress"},
		{ID: "NAC-3", Title: "claimed per bd", Status: "in_progress", StartedAt: start.Add(10 * time.Second).UTC().Format(time.RFC3339)},
	}

	newWatcher := func() *Watcher {
		w := New(Options{Projects: projects, Procs: fakeProcs{}, Logger: log.New(io.Discard, "", 0)})
		// The transcript's bd commands reach link through the tail's pass.
		w.tail(&Session{Record: record(1, sid, "/code/Nacre", "", start), Transcript: transcript}, claude.TranscriptOptions{})
		return w
	}
	w := newWatcher()
	// (a) The session name wins when it names a real bead.
	rec := record(1, sid, "/code/Nacre", "Nacre · NAC-1", start)
	if got := w.link(rec, beads, transcript, nil); got.issueID != "NAC-1" || got.by != LinkName || got.title != "named" {
		t.Fatalf("name link = %+v", got)
	}
	// (b) Otherwise the bd command the agent actually ran.
	w = newWatcher()
	rec.Name = "nacre-eb" // a derived name, matching no bead
	if got := w.link(rec, beads, transcript, nil); got.issueID != "NAC-2" || got.by != LinkTranscript {
		t.Fatalf("transcript link = %+v", got)
	}
	// (c) Otherwise bd's own claim state.
	w = New(Options{Projects: projects, Procs: fakeProcs{}, Logger: log.New(io.Discard, "", 0)})
	if got := w.link(rec, beads, "", nil); got.issueID != "NAC-3" || got.by != LinkBdState {
		t.Fatalf("bd-state link = %+v", got)
	}
	// A bead already linked to another session is not handed out twice by the
	// weakest signal.
	w = New(Options{Projects: projects, Procs: fakeProcs{}, Logger: log.New(io.Discard, "", 0)})
	if got := w.link(rec, beads, "", map[string]bool{"NAC-3": true}); got.issueID != "" {
		t.Fatalf("taken bead = %+v", got)
	}
	// Nothing resolves: reported with no bead rather than linked to a guess.
	w = New(Options{Projects: projects, Procs: fakeProcs{}, Logger: log.New(io.Discard, "", 0)})
	if got := w.link(rec, beads[:2], "", nil); got.issueID != "" || got.by != "" {
		t.Fatalf("no signal = %+v", got)
	}
	// With no bd list an id cannot be validated, so no link is made...
	w = newWatcher()
	if got := w.link(rec, nil, transcript, nil); got.issueID != "" {
		t.Fatalf("no beads = %+v", got)
	}
	// ...and the bd commands already read wait for one.
	if got := w.link(rec, beads, transcript, nil); got.issueID != "NAC-2" || got.by != LinkTranscript {
		t.Fatalf("once bd answers = %+v", got)
	}
	rec.Name = "Nacre · NAC-1"
	if got := w.link(rec, nil, transcript, nil); got.issueID != "" {
		t.Fatalf("no beads, named = %+v", got)
	}
}

func TestClaimedDuringNeedsAnUnambiguousAnswer(t *testing.T) {
	start := time.Now()
	at := func(d time.Duration) string { return start.Add(d).UTC().Format(time.RFC3339) }
	beads := []Bead{
		{ID: "old", Status: "in_progress", StartedAt: at(-time.Hour)},   // claimed long before
		{ID: "open", Status: "open", StartedAt: at(time.Minute)},        // not claimed
		{ID: "slack", Status: "hooked", StartedAt: at(-time.Minute)},    // just before, within slack
		{ID: "late", Status: "in_progress", StartedAt: at(time.Minute)}, // the latest claim
	}
	got, ok := claimedDuring(beads, start, nil)
	if !ok || got.ID != "late" {
		t.Fatalf("claimedDuring = %+v,%v want late", got, ok)
	}
	// A tie is a coin toss, so nothing is reported.
	tied := append(append([]Bead(nil), beads...), Bead{ID: "also", Status: "in_progress", StartedAt: at(time.Minute)})
	if _, ok := claimedDuring(tied, start, nil); ok {
		t.Error("a tie must not resolve")
	}
	if _, ok := claimedDuring(beads, time.Time{}, nil); ok {
		t.Error("unknown session start must not resolve")
	}
	if _, ok := claimedDuring(beads[:1], start, nil); ok {
		t.Error("a bead claimed before the session must not resolve")
	}
}

func TestPollAdoptsOnlyLiveAgentSessionsInKnownRepos(t *testing.T) {
	dir, projects := t.TempDir(), t.TempDir()
	repoPath := t.TempDir()
	pdir := filepath.Join(projects, "-repo")
	if err := os.MkdirAll(pdir, 0o700); err != nil {
		t.Fatal(err)
	}
	start := time.Now().Add(-2 * time.Minute).Truncate(time.Second)

	agent := record(11, "aaaaaaaa-0000-0000-0000-000000000011", repoPath, "repo-eb", start)
	mustWrite(t, filepath.Join(pdir, agent.SessionID+".jsonl"), transcriptLine("bd update NAC-7 --claim", false))
	dead := record(12, "aaaaaaaa-0000-0000-0000-000000000012", repoPath, "dead", start)
	reused := record(13, "aaaaaaaa-0000-0000-0000-000000000013", repoPath, "reused", start)
	ownByID := record(14, "aaaaaaaa-0000-0000-0000-000000000014", repoPath, "whatever", start)
	ownByName := record(15, "aaaaaaaa-0000-0000-0000-000000000015", repoPath, "Nacre · NAC-7", start)
	elsewhere := record(16, "aaaaaaaa-0000-0000-0000-000000000016", "/not/a/repo", "other", start)
	for _, rec := range []Record{agent, dead, reused, ownByID, ownByName, elsewhere} {
		writeRecord(t, dir, rec)
	}

	procs := fakeProcs{11: start, 13: start.Add(time.Hour), 14: start, 15: start, 16: start}
	var got []Session
	w := New(Options{
		Dir: dir, Projects: projects, Procs: procs,
		OwnNamePrefix: "Nacre · ",
		MainCheckout:  func(string) string { return "" },
		Repos:         func() []Repo { return []Repo{{ID: "r1", Name: "repo", Path: repoPath}} },
		Beads:         func(string) []Bead { return []Bead{{ID: "NAC-7", Title: "Seven", Status: "in_progress"}} },
		Owned:         func(sid string) bool { return sid == ownByID.SessionID },
		Sync:          func(live []Session) { got = live },
		Logger:        log.New(io.Discard, "", 0),
	})
	w.Poll()

	if len(got) != 1 {
		t.Fatalf("adopted %d sessions, want 1: %+v", len(got), got)
	}
	s := got[0]
	if s.SessionID != agent.SessionID || s.Repo.ID != "r1" {
		t.Fatalf("session = %+v", s)
	}
	if s.IssueID != "NAC-7" || s.LinkedBy != LinkTranscript || s.IssueTitle != "Seven" {
		t.Fatalf("link = %+v", s)
	}
	if s.Transcript != filepath.Join(pdir, agent.SessionID+".jsonl") {
		t.Fatalf("transcript = %q", s.Transcript)
	}

	// A linked session is not re-linked, so a steady state costs no bd calls.
	calls := 0
	w.opts.Beads = func(string) []Bead { calls++; return nil }
	w.Poll()
	if len(got) != 1 || got[0].IssueID != "NAC-7" || calls != 0 {
		t.Fatalf("second poll: %+v, %d bd calls", got, calls)
	}

	// When the process goes away the session drops out of the set, and its
	// caches are released.
	delete(procs, 11)
	w.Poll()
	if len(got) != 0 {
		t.Fatalf("after exit = %+v", got)
	}
	if len(w.links) != 0 || len(w.scan.state) != 0 {
		t.Fatalf("caches not released: %d links, %d scans", len(w.links), len(w.scan.state))
	}
}

// Each poll tails every adopted session's transcript: the session carries the
// run events gained since the previous poll, the transcript's running state
// and its last write, and the tail is released with the session.
func TestPollTailsEachSessionsTranscript(t *testing.T) {
	dir, projects, repoPath := t.TempDir(), t.TempDir(), t.TempDir()
	pdir := filepath.Join(projects, "-repo")
	if err := os.MkdirAll(pdir, 0o700); err != nil {
		t.Fatal(err)
	}
	start := time.Now().Add(-2 * time.Minute).Truncate(time.Second)
	agent := record(21, "aaaaaaaa-0000-0000-0000-000000000021", repoPath, "agent", start)
	writeRecord(t, dir, agent)
	path := filepath.Join(pdir, agent.SessionID+".jsonl")
	mustWrite(t, path, transcriptLine("go test ./...", false))

	procs := fakeProcs{21: start}
	var got []Session
	w := New(Options{
		Dir: dir, Projects: projects, Procs: procs,
		MainCheckout: func(string) string { return "" },
		Repos:        func() []Repo { return []Repo{{ID: "r1", Name: "repo", Path: repoPath}} },
		Sync:         func(live []Session) { got = live },
		Logger:       log.New(io.Discard, "", 0),
	})
	w.Poll()
	if len(got) != 1 {
		t.Fatalf("sessions = %+v", got)
	}
	s := got[0]
	if len(s.Events) != 1 || s.Events[0].Kind != "tool_use" || s.Events[0].Tool != "Bash" {
		t.Fatalf("events = %+v", s.Events)
	}
	if !s.FromStart || s.TranscriptState.ToolCalls != 1 || s.TranscriptState.AssistantTurns != 1 {
		t.Fatalf("first poll: from start %v, state %+v", s.FromStart, s.TranscriptState)
	}
	fi, _ := os.Stat(path)
	if !s.LastWrite.Equal(fi.ModTime()) || !s.LastActive().Equal(fi.ModTime()) {
		t.Fatalf("last write = %v, last active = %v, want the transcript mtime %v", s.LastWrite, s.LastActive(), fi.ModTime())
	}

	// Nothing new: no events, and the tail carries on where it was.
	w.Poll()
	if s := got[0]; len(s.Events) != 0 || s.FromStart || s.TranscriptState.ToolCalls != 1 {
		t.Fatalf("quiet poll = events %+v, from start %v, state %+v", s.Events, s.FromStart, s.TranscriptState)
	}
	appendTo(t, path, transcriptLine("bd show NAC-7", false))
	w.Poll()
	if s := got[0]; len(s.Events) != 1 || s.FromStart || s.TranscriptState.ToolCalls != 2 {
		t.Fatalf("after append = events %+v, from start %v, state %+v", s.Events, s.FromStart, s.TranscriptState)
	}

	delete(procs, 21)
	w.Poll()
	if len(got) != 0 || len(w.tails) != 0 {
		t.Fatalf("after exit: %d sessions, %d tails", len(got), len(w.tails))
	}
}

func TestLastActiveFallsBackToTheSessionStart(t *testing.T) {
	start := time.Now().Add(-time.Hour).Truncate(time.Millisecond)
	s := Session{Record: record(1, "s", "/x", "", start)}
	if !s.LastActive().Equal(start.UTC()) {
		t.Fatalf("no transcript: last active = %v, want the start %v", s.LastActive(), start)
	}
	entry := start.Add(time.Minute)
	s.TranscriptState.LastActivity = entry
	if !s.LastActive().Equal(entry) {
		t.Fatalf("last active = %v, want the newest entry %v", s.LastActive(), entry)
	}
	s.LastWrite = entry.Add(time.Minute)
	if !s.LastActive().Equal(s.LastWrite) {
		t.Fatalf("last active = %v, want the last write %v", s.LastActive(), s.LastWrite)
	}
}

// The bd command that links a session comes out of the same transcript read
// that produces its run events: on the poll where the claim is appended, the
// session carries both the claim's tool_use event and the link it implies.
func TestPollLinksFromTheSharedTranscriptPass(t *testing.T) {
	dir, projects, repoPath := t.TempDir(), t.TempDir(), t.TempDir()
	pdir := filepath.Join(projects, "-repo")
	if err := os.MkdirAll(pdir, 0o700); err != nil {
		t.Fatal(err)
	}
	start := time.Now().Add(-2 * time.Minute).Truncate(time.Second)
	agent := record(31, "aaaaaaaa-0000-0000-0000-000000000031", repoPath, "repo-eb", start)
	writeRecord(t, dir, agent)
	path := filepath.Join(pdir, agent.SessionID+".jsonl")
	mustWrite(t, path, transcriptLine("bd update OTHER-1 --claim", false))

	var got []Session
	w := New(Options{
		Dir: dir, Projects: projects, Procs: fakeProcs{31: start},
		MainCheckout: func(string) string { return "" },
		Repos:        func() []Repo { return []Repo{{ID: "r1", Name: "repo", Path: repoPath}} },
		// NAC-9 was claimed long before the session, so bd's claim state
		// cannot link it; only the transcript can.
		Beads: func(string) []Bead {
			return []Bead{
				{ID: "NAC-7", Title: "Seven", Status: "open"},
				{ID: "NAC-9", Title: "Nine", Status: "in_progress", StartedAt: start.Add(-time.Hour).UTC().Format(time.RFC3339)},
			}
		},
		Sync:   func(live []Session) { got = live },
		Logger: log.New(io.Discard, "", 0),
	})
	w.Poll()
	if len(got) != 1 || got[0].IssueID != "" {
		t.Fatalf("an id that is not a bead linked: %+v", got)
	}

	// A read, then a claim: the claim wins, on the very poll that reads it.
	appendTo(t, path, transcriptLine("bd show NAC-9", false)+transcriptLine("cd x && bd update NAC-7 --claim", false))
	w.Poll()
	s := got[0]
	if s.IssueID != "NAC-7" || s.LinkedBy != LinkTranscript || s.IssueTitle != "Seven" {
		t.Fatalf("link = %q by %q (%q)", s.IssueID, s.LinkedBy, s.IssueTitle)
	}
	if len(s.Events) != 2 || s.Events[1].Kind != "tool_use" || !strings.Contains(s.Events[1].Text, "bd update NAC-7") {
		t.Fatalf("events = %+v", s.Events)
	}
	fi, _ := os.Stat(path)
	if off := w.tails[agent.SessionID].Offset(); off != fi.Size() {
		t.Fatalf("tail at %d of %d bytes", off, fi.Size())
	}
	// Linked, the session's mentions are no longer collected.
	if len(w.scan.state) != 0 {
		t.Fatalf("scan state kept after linking: %+v", w.scan.state)
	}
	appendTo(t, path, transcriptLine("bd update NAC-9 --claim", false))
	w.Poll()
	if got[0].IssueID != "NAC-7" || len(w.scan.state) != 0 {
		t.Fatalf("after link: %q, %d scan states", got[0].IssueID, len(w.scan.state))
	}
}
