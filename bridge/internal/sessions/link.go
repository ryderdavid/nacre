package sessions

import (
	"strings"
	"time"
	"unicode"
)

// bdStateSlack is how far before a session's start a bead may have been
// claimed and still be attributed to it. The bead and the session record are
// written by different processes a moment apart, so the two timestamps do not
// line up exactly.
const bdStateSlack = 2 * time.Minute

// linkResult is the outcome of linking a session to a bead.
type linkResult struct {
	issueID string
	title   string
	by      string
}

// link resolves the bead an adopted session is working on, taking the first
// signal that answers and recording which one it was. beads is the repo's bd
// list; it both validates the ids read out of the name and the transcript and
// provides the last-resort signal. taken holds the beads already linked to
// another session in this pass, so the weakest signal cannot attach one bead
// to two sessions.
//
// The transcript is not read here: the bd commands in it were collected by the
// session's transcript tail, so link must run after this poll's tail read.
//
// A session that resolves to nothing returns an empty result: it is reported
// as repo activity with no bead rather than linked to a guess.
func (w *Watcher) link(rec Record, beads []Bead, transcript string, taken map[string]bool) linkResult {
	known := make(map[string]Bead, len(beads))
	for _, b := range beads {
		known[b.ID] = b
	}
	valid := func(id string) bool { _, ok := known[id]; return ok }
	if len(known) == 0 {
		// bd is unavailable or the repo is empty; without it an id cannot be
		// validated, and an unvalidated id is a guess.
		return linkResult{}
	}
	finish := func(id, by string) linkResult {
		return linkResult{issueID: id, title: known[id].Title, by: by}
	}

	// (a) The session name. Nacre names its own sessions "Nacre · <id>"; any
	// agent that names its session after the bead links for free.
	for _, tok := range nameTokens(rec.Name) {
		if valid(tok) {
			return finish(tok, LinkName)
		}
	}

	// (b) The bd commands the agent actually ran, as the transcript tail saw
	// them. This is a causal link, not a correlation: the id came out of the
	// command that moved the bead.
	if transcript != "" {
		if id, ok := w.scan.best(rec.SessionID, valid); ok {
			return finish(id, LinkTranscript)
		}
	}

	// (c) bd's own claim state: the most recently claimed bead in this repo
	// that was taken on after the session started.
	if b, ok := claimedDuring(beads, rec.Started(), taken); ok {
		return finish(b.ID, LinkBdState)
	}
	return linkResult{}
}

// nameTokens splits a session name into the words that could be a bead id:
// runs of letters, digits and the separators a bead id may contain.
func nameTokens(name string) []string {
	return strings.FieldsFunc(name, func(r rune) bool {
		switch r {
		case '-', '.', '_':
			return false
		}
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// claimedDuring returns the bead most recently taken on at or after the
// session started. An empty session start, or a tie between two beads, yields
// nothing: both would be a coin toss.
func claimedDuring(beads []Bead, start time.Time, taken map[string]bool) (Bead, bool) {
	if start.IsZero() {
		return Bead{}, false
	}
	floor := start.Add(-bdStateSlack)
	var best Bead
	var bestAt time.Time
	tie := false
	for _, b := range beads {
		if taken[b.ID] || !activeStatuses[b.Status] {
			continue
		}
		at := b.Started()
		if at.IsZero() || at.Before(floor) {
			continue
		}
		switch {
		case bestAt.IsZero() || at.After(bestAt):
			best, bestAt, tie = b, at, false
		case at.Equal(bestAt):
			tie = true
		}
	}
	if bestAt.IsZero() || tie {
		return Bead{}, false
	}
	return best, true
}
