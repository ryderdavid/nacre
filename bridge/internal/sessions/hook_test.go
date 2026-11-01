package sessions

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func hookWatcher(t *testing.T, dir, projects, repoPath string, procs fakeProcs, beads []Bead, sync func([]Session)) *Watcher {
	t.Helper()
	return New(Options{
		Dir: dir, Projects: projects, Procs: procs,
		OwnNamePrefix: "Nacre · ",
		MainCheckout:  func(string) string { return "" },
		Repos:         func() []Repo { return []Repo{{ID: "r1", Name: "repo", Path: repoPath}} },
		Beads:         func(string) []Bead { return beads },
		Sync:          sync,
		Logger:        log.New(io.Discard, "", 0),
	})
}

// A hint changes where the transcript is read from and how the session is
// linked, never whether it is adopted: that still takes a live registry
// record in a known repo.
func TestHintGivesTheExactTranscriptAndAnExplicitLink(t *testing.T) {
	dir, projects, repoPath, elsewhere := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	start := time.Now().Add(-2 * time.Minute).Truncate(time.Second)
	agent := record(41, "aaaaaaaa-0000-0000-0000-000000000041", repoPath, "plain", start)
	writeRecord(t, dir, agent)
	// The transcript is not under Projects at all (CLAUDE_CONFIG_DIR, say):
	// the glob cannot find it, the hook's path can.
	exact := filepath.Join(elsewhere, agent.SessionID+".jsonl")
	mustWrite(t, exact, transcriptLine("echo hi", false))

	beads := []Bead{
		{ID: "NAC-7", Title: "Seven", Status: "open"},
		{ID: "NAC-9", Title: "Nine", Status: "open"},
	}
	var got []Session
	w := hookWatcher(t, dir, projects, repoPath, fakeProcs{41: start}, beads, func(l []Session) { got = l })
	w.Poll()
	if len(got) != 1 || got[0].Transcript != "" || got[0].IssueID != "" {
		t.Fatalf("without a hint: %+v", got)
	}

	if w.Nudge(Hint{SessionID: "not a session"}) {
		t.Fatal("a hint without a session id was taken")
	}
	if !w.Nudge(Hint{SessionID: agent.SessionID, TranscriptPath: exact, CWD: repoPath}) {
		t.Fatal("hint refused")
	}
	w.Poll()
	if len(got) != 1 || got[0].Transcript != exact || len(got[0].Events) != 1 {
		t.Fatalf("with the hint: %+v", got)
	}

	// A bd show is not a link; a claim links at once, by hook, even though
	// the transcript never shows it.
	w.Nudge(Hint{SessionID: agent.SessionID, Command: "bd show NAC-9"})
	w.Poll()
	if got[0].IssueID != "" {
		t.Fatalf("a read linked: %+v", got[0])
	}
	w.Nudge(Hint{SessionID: agent.SessionID, Command: "cd x && bd update NAC-7 --claim"})
	w.Poll()
	if s := got[0]; s.IssueID != "NAC-7" || s.LinkedBy != LinkHook || s.IssueTitle != "Seven" {
		t.Fatalf("claim: %+v", s)
	}
	// Closing another bead does not move a linked session; claiming one does.
	w.Nudge(Hint{SessionID: agent.SessionID, Command: "bd close NAC-9"})
	w.Poll()
	if got[0].IssueID != "NAC-7" {
		t.Fatalf("a close moved the link: %+v", got[0])
	}
	w.Nudge(Hint{SessionID: agent.SessionID, Command: "bd update NAC-9 --status in_progress"})
	w.Poll()
	if got[0].IssueID != "NAC-9" || got[0].LinkedBy != LinkHook {
		t.Fatalf("second claim: %+v", got[0])
	}
	// An id that is not a bead of the repo is ignored.
	w.Nudge(Hint{SessionID: agent.SessionID, Command: "bd update OTHER-1 --claim"})
	w.Poll()
	if got[0].IssueID != "NAC-9" {
		t.Fatalf("unknown bead linked: %+v", got[0])
	}

	// When the session ends its hints go with it.
	if err := os.Remove(filepath.Join(dir, "41.json")); err != nil {
		t.Fatal(err)
	}
	w.Poll()
	if len(got) != 0 || len(w.hints) != 0 {
		t.Fatalf("after exit: %d sessions, %d hints", len(got), len(w.hints))
	}
}

// A hint alone adopts nothing, and duplicates of a session collapse into the
// one the registry shows.
func TestHintsNeverAdoptOnTheirOwn(t *testing.T) {
	dir, projects, repoPath := t.TempDir(), t.TempDir(), t.TempDir()
	start := time.Now().Add(-time.Minute).Truncate(time.Second)
	own := record(51, "aaaaaaaa-0000-0000-0000-000000000051", repoPath, "Nacre · NAC-7", start)
	agent := record(52, "aaaaaaaa-0000-0000-0000-000000000052", repoPath, "plain", start)
	writeRecord(t, dir, own)
	writeRecord(t, dir, agent)
	var got []Session
	w := hookWatcher(t, dir, projects, repoPath, fakeProcs{51: start, 52: start}, nil, func(l []Session) { got = l })

	ghost := "aaaaaaaa-0000-0000-0000-000000000053"
	for _, sid := range []string{own.SessionID, agent.SessionID, agent.SessionID, ghost} {
		w.Nudge(Hint{SessionID: sid, CWD: repoPath})
	}
	w.Poll()
	if len(got) != 1 || got[0].SessionID != agent.SessionID {
		t.Fatalf("adopted %+v", got)
	}
	if !w.waiting(time.Now()) {
		t.Fatal("a fresh hint for a session not in the registry yet is not waited for")
	}
	// Past hintWait the unmatched hints are dropped.
	w.forgetHints(map[string]bool{agent.SessionID: true}, time.Now().Add(hintWait))
	if _, ok := w.hints[ghost]; ok || w.waiting(time.Now()) {
		t.Fatalf("stale hints kept: %+v", w.hints)
	}
	if _, ok := w.hints[agent.SessionID]; !ok {
		t.Fatal("a live session's hint dropped")
	}
}

// Run polls as soon as it is nudged, not an interval later, and looks again
// shortly for a session the registry did not show yet.
func TestRunPollsOnNudge(t *testing.T) {
	dir, projects, repoPath := t.TempDir(), t.TempDir(), t.TempDir()
	start := time.Now().Add(-time.Minute).Truncate(time.Second)
	var mu sync.Mutex
	polls := 0
	var got []Session
	w := hookWatcher(t, dir, projects, repoPath, fakeProcs{61: start}, nil, func(l []Session) {
		mu.Lock()
		defer mu.Unlock()
		polls++
		got = l
	})
	w.opts.Interval = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	state := func() (int, int) {
		mu.Lock()
		defer mu.Unlock()
		return polls, len(got)
	}
	waitFor := func(what string, ok func(polls, adopted int) bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			p, n := state()
			if ok(p, n) {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: %d polls, %d adopted", what, p, n)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitFor("first poll", func(p, _ int) bool { return p >= 1 })

	// SessionStart can beat the registry record: the nudge polls and finds
	// nothing, then a retry a second later finds the record.
	sid := "aaaaaaaa-0000-0000-0000-000000000061"
	w.Nudge(Hint{SessionID: sid})
	waitFor("nudged poll", func(p, _ int) bool { return p >= 2 })
	writeRecord(t, dir, record(61, sid, repoPath, "plain", start))
	waitFor("retry adopts", func(_, n int) bool { return n == 1 })
}

func TestLinksBead(t *testing.T) {
	for cmd, want := range map[string]bool{
		"bd update NAC-1 --claim":          true,
		"cd x && bd close NAC-1":           true,
		"bd show NAC-1":                    false,
		"bd update NAC-1 --notes hi":       false,
		"ls -la":                           false,
		"":                                 false,
		"bd --dir /x update NAC-1 --claim": true,
	} {
		if got := LinksBead(cmd); got != want {
			t.Errorf("LinksBead(%q) = %v, want %v", cmd, got, want)
		}
	}
}
