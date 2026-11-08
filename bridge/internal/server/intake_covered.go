package server

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/decisions"
)

// An intake card can wait a while for Ryder. Meanwhile the same work may be
// filed, or finished, from somewhere else: another chat, a desktop session,
// the board. Each open intake card is compared with the beads filed or
// closed since it was raised; one that looks like the same work is offered
// first, as "Already covered by <bead>", and the card says so.

// coveredByPrefix starts the id of the option naming the bead that covers a
// card's draft.
const coveredByPrefix = "covered_by:"

// coverEvery is how often one open intake card is compared with the beads
// when the queue is listed; tests shorten it.
var coverEvery = 60 * time.Second

// coverMatch is a bead that looks like an intake card's draft.
type coverMatch struct {
	bead   BeadFacts
	filed  bool // filed after the card was raised, not only closed after
	closed bool
}

// coverOpenIntake compares each open intake card not yet offering a covering
// bead with the beads touched since it was raised, at most every coverEvery.
func (s *Server) coverOpenIntake(ctx context.Context) {
	if s.Decisions == nil {
		return
	}
	now := time.Now()
	for _, rec := range s.Decisions.List() {
		if !needsCoverCheck(rec) {
			continue
		}
		key := "cover:" + rec.Decision.ID
		s.checkedMu.Lock()
		if s.checked == nil {
			s.checked = map[string]time.Time{}
		}
		last, seen := s.checked[key]
		due := !seen || now.Sub(last) >= coverEvery
		if due {
			s.checked[key] = now
		}
		s.checkedMu.Unlock()
		if due {
			s.coverIntake(ctx, rec)
		}
	}
}

// needsCoverCheck: an open intake card that offers no covering bead yet.
func needsCoverCheck(rec decisions.Record) bool {
	d := rec.Decision
	if d.Kind != decisions.KindIntake || !d.Answerable() {
		return false
	}
	for _, o := range d.Options {
		if strings.HasPrefix(o.ID, coveredByPrefix) {
			return false
		}
	}
	return true
}

// coverIntake looks for a bead covering an intake card's draft and, when one
// is found, offers it on the card. It reports whether the card changed.
func (s *Server) coverIntake(ctx context.Context, rec decisions.Record) (decisions.Record, bool) {
	if !needsCoverCheck(rec) {
		return rec, false
	}
	ref, err := s.repoRef(rec.Decision.Repo)
	if err != nil {
		return rec, false
	}
	title, desc := s.intakeDraftText(ctx, ref.Path, rec)
	if title == "" {
		return rec, false
	}
	beads, err := s.facts().Beads(ctx, ref.Path)
	if err != nil {
		return rec, false
	}
	m, ok := coveringBead(title, desc, rec.Decision.CreatedAt, rec.Decision.Target.Bead, beads)
	if !ok {
		return rec, false
	}
	changed := false
	out, err := s.Decisions.Update(rec.Decision.ID, func(r *decisions.Record) bool {
		if !needsCoverCheck(*r) {
			return false
		}
		offerCover(&r.Decision, m)
		changed = true
		return true
	})
	if err != nil {
		return rec, false
	}
	return out, changed
}

// intakeDraftText is the draft an intake card was raised for: the title and
// description its create actions carry, or the existing bead's.
func (s *Server) intakeDraftText(ctx context.Context, repoPath string, rec decisions.Record) (string, string) {
	for _, act := range rec.Actions {
		if t, _ := act.Body["title"].(string); strings.TrimSpace(t) != "" {
			d, _ := act.Body["description"].(string)
			return t, d
		}
	}
	if id := rec.Decision.Target.Bead; id != "" {
		if b, err := s.facts().Bead(ctx, repoPath, id); err == nil {
			return b.Title, b.Description
		}
	}
	return "", ""
}

// offerCover puts the covering bead first on the card, recommended, and
// says why.
func offerCover(d *decisions.Decision, m coverMatch) {
	b := m.bead
	what := "filed"
	switch {
	case m.filed && m.closed:
		what = "filed and closed"
	case m.closed:
		what = "closed"
	}
	named := b.ID + " “" + firstLine(b.Title, 70) + "”"
	opts := []decisions.Option{{
		ID: coveredByPrefix + b.ID, Label: "Already covered by " + b.ID, Recommended: true,
		Detail: named + " was " + what + " after this was raised and looks like the same work. Records that; nothing is created or spent, and this chat follows " + b.ID,
	}}
	for _, o := range d.Options {
		o.Recommended = false
		opts = append(opts, o)
	}
	d.Options = opts
	d.Consequences = append([]string{"Since this was raised, " + named + " was " + what + "; it looks like the same work"}, d.Consequences...)
	d.Evidence = append([]decisions.Evidence{{Label: b.ID, Kind: "bead", Ref: b.ID, Repo: d.Repo}}, d.Evidence...)
}

// recheckCover runs before an intake card is answered: when a covering bead
// turned up since the card was last looked at, the answer is refused so
// Ryder sees it first. Answering again goes ahead.
func (s *Server) recheckCover(ctx context.Context, rec decisions.Record, option string) error {
	if strings.HasPrefix(option, coveredByPrefix) {
		return nil
	}
	out, changed := s.coverIntake(ctx, rec)
	if !changed {
		return nil
	}
	label := "a bead"
	if len(out.Decision.Options) > 0 {
		label = strings.TrimPrefix(out.Decision.Options[0].ID, coveredByPrefix)
	}
	return refuse(http.StatusConflict, "%s was filed or closed since this was raised and looks like the same work; the card now offers it. Answer again to go ahead anyway.", label)
}

// coverMinShared is how many words a bead's title must share with the
// draft's; coverMinTitle and coverMinText bound the title overlap (of the
// shorter title) and the whole-text Jaccard similarity.
const (
	coverMinShared = 2
	coverMinTitle  = 0.5
	coverMinText   = 0.12
)

// coveringBead is the bead most like the draft among those filed or closed
// after since, other than skip (the bead the card is about).
func coveringBead(title, desc, since, skip string, beads []BeadFacts) (coverMatch, bool) {
	at, err := time.Parse(time.RFC3339, since)
	if err != nil {
		return coverMatch{}, false
	}
	after := func(ts string) bool {
		t, err := time.Parse(time.RFC3339, ts)
		return err == nil && t.After(at)
	}
	dt := words(title)
	dx := union(dt, words(clip(desc, 800)))
	var best coverMatch
	bestScore := 0.0
	for _, b := range beads {
		if b.ID == skip {
			continue
		}
		filed, closed := after(b.CreatedAt), after(b.ClosedAt)
		if !filed && !closed {
			continue
		}
		bt := words(b.Title)
		shared := len(intersect(dt, bt))
		if shared < coverMinShared || float64(shared)/float64(max(1, min(len(dt), len(bt)))) < coverMinTitle {
			continue
		}
		bx := union(bt, words(clip(b.Description, 800)))
		inter := len(intersect(dx, bx))
		score := float64(inter) / float64(max(1, len(dx)+len(bx)-inter))
		if score >= coverMinText && score > bestScore {
			best, bestScore = coverMatch{bead: b, filed: filed, closed: closed && b.Status == "closed"}, score
		}
	}
	return best, bestScore > 0
}

var wordRE = regexp.MustCompile(`[a-z0-9]+`)

var stopWords = map[string]bool{}

func init() {
	for _, w := range strings.Fields("the and for with that this from into are was were has have not but can its their them they you your our out all any each when then than also only just more most some such what which who why how about after before over under onto off per via does did") {
		stopWords[w] = true
	}
}

// words are a text's significant words, lightly stemmed.
func words(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range wordRE.FindAllString(strings.ToLower(s), -1) {
		if len(w) < 3 || stopWords[w] {
			continue
		}
		out[stem(w)] = true
	}
	return out
}

func stem(w string) string {
	for _, suf := range []string{"ing", "ies", "es", "ed", "s"} {
		if strings.HasSuffix(w, suf) && len(w)-len(suf) >= 3 {
			if suf == "ies" {
				return w[:len(w)-3] + "y"
			}
			return w[:len(w)-len(suf)]
		}
	}
	return w
}

func union(a, b map[string]bool) map[string]bool {
	out := make(map[string]bool, len(a)+len(b))
	for w := range a {
		out[w] = true
	}
	for w := range b {
		out[w] = true
	}
	return out
}

func intersect(a, b map[string]bool) map[string]bool {
	out := map[string]bool{}
	for w := range a {
		if b[w] {
			out[w] = true
		}
	}
	return out
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
