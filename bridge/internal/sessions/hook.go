package sessions

import (
	"path/filepath"
	"strings"
	"time"
)

// Hook hints (NAC-j5z.10). A Claude Code hook (`nacred hook`, installed by
// hand) posts each SessionStart, PostToolUse and Stop to POST /v1/sessions,
// which hands it to Watcher.Nudge. A hint never adopts anything by itself: it
// makes the next poll happen now rather than up to an interval later, gives
// that poll the session's exact transcript path, and, for a bd claim or close,
// an explicit bead link. Every session still has to be in the registry, live,
// in a known repo and not nacred's own, exactly as without the hook, so the
// hook and the poll can only ever produce one adopted run per session, and a
// machine without the hook behaves exactly as before.

// LinkHook comes from a Claude Code hook reporting a bd claim or close the
// session just ran: the explicit form of LinkTranscript.
const LinkHook = "hook"

const (
	// hintWait is how long a hint for a session the registry does not show
	// (yet) is kept. SessionStart can fire a moment before Claude Code writes
	// its registry record.
	hintWait = 15 * time.Second
	// hintRetry is how soon a nudge whose session was not found polls again,
	// and hintRetries how many times.
	hintRetry   = time.Second
	hintRetries = 5
	// maxInbox bounds the hints queued between two polls.
	maxInbox = 256
)

// Hint is what a Claude Code hook reports about one session. Only SessionID
// is required.
type Hint struct {
	SessionID string
	// TranscriptPath is the session's transcript as Claude Code itself names
	// it. It replaces the glob under Options.Projects.
	TranscriptPath string
	// CWD is the session's working directory.
	CWD string
	// Command is the shell command a PostToolUse hook saw run, when the tool
	// was Bash. A bd claim or close in it links the session to that bead.
	Command string
}

// hintState is what the watcher keeps from a session's hints. It is only
// touched by the polling goroutine.
type hintState struct {
	transcript string
	cwd        string
	// pending are bd claims and closes not yet validated against the repo's
	// bd list, oldest first.
	pending  []mention
	received time.Time
	// seen is set once the session has been reported by a poll.
	seen bool
}

// ValidSessionID reports whether id has the shape of a Claude Code session id.
func ValidSessionID(id string) bool { return sessionIDRe.MatchString(id) }

// LinksBead reports whether a shell command line runs a bd command that would
// link a session to a bead from a hook (a claim or a close). The hook command
// uses it to stay silent for every other tool call.
func LinksBead(cmd string) bool {
	for _, m := range commandMentions(cmd) {
		if m.weight >= weightClose {
			return true
		}
	}
	return false
}

// Nudge queues h for the next poll and wakes Run to poll now. It is safe to
// call from any goroutine and never blocks. It returns false, and does
// nothing, for a hint without a valid session id.
func (w *Watcher) Nudge(h Hint) bool {
	if !ValidSessionID(h.SessionID) {
		return false
	}
	w.hintMu.Lock()
	if len(w.inbox) < maxInbox {
		w.inbox = append(w.inbox, h)
	}
	w.hintMu.Unlock()
	select {
	case w.nudge <- struct{}{}:
	default: // a poll is already due
	}
	return true
}

// takeHints folds the hints queued since the last poll into w.hints.
func (w *Watcher) takeHints(now time.Time) {
	w.hintMu.Lock()
	inbox := w.inbox
	w.inbox = nil
	w.hintMu.Unlock()
	for _, h := range inbox {
		hs := w.hints[h.SessionID]
		if hs == nil {
			hs = &hintState{}
			w.hints[h.SessionID] = hs
		}
		hs.received = now
		if p := filepath.Clean(h.TranscriptPath); h.TranscriptPath != "" && filepath.IsAbs(p) && strings.HasSuffix(p, ".jsonl") {
			hs.transcript = p
		}
		if h.CWD != "" && filepath.IsAbs(h.CWD) {
			hs.cwd = filepath.Clean(h.CWD)
		}
		for _, m := range commandMentions(h.Command) {
			if m.weight >= weightClose {
				hs.pending = append(hs.pending, m)
			}
		}
	}
}

// waiting reports whether a recent hint names a session no poll has found
// yet, so Run should look again shortly.
func (w *Watcher) waiting(now time.Time) bool {
	for _, hs := range w.hints {
		if !hs.seen && now.Sub(hs.received) < hintWait {
			return true
		}
	}
	return false
}

// hookLink applies the bd claims and closes a hook reported for a session to
// its current link l. A claim always wins: it is the agent saying, right now,
// which bead it took on. A close links a session that has no bead yet but
// does not move one that is linked elsewhere, so tidying up an old bead does
// not steal the session from the one it is working on. Mentions stay pending
// while bd is unavailable to validate them.
func hookLink(hs *hintState, beads []Bead, l linkResult) linkResult {
	if hs == nil || len(hs.pending) == 0 || len(beads) == 0 {
		return l
	}
	known := make(map[string]Bead, len(beads))
	for _, b := range beads {
		known[b.ID] = b
	}
	for _, m := range hs.pending {
		b, ok := known[m.id]
		if !ok {
			continue
		}
		if m.weight >= weightClaim || l.issueID == "" || l.issueID == m.id {
			l = linkResult{issueID: b.ID, title: b.Title, by: LinkHook}
		}
	}
	hs.pending = nil
	return l
}

// forgetHints drops the hints of sessions that have gone, and of sessions
// the registry never showed within hintWait.
func (w *Watcher) forgetHints(seen map[string]bool, now time.Time) {
	for sid, hs := range w.hints {
		if seen[sid] {
			continue
		}
		if hs.seen || now.Sub(hs.received) >= hintWait {
			delete(w.hints, sid)
		}
	}
}
