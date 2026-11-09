package specflow

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// promptFeedback is the owner_feedback and the seed notes one author or
// review call was given.
func promptFeedback(t *testing.T, h claude.Headless) ([]string, string) {
	t.Helper()
	var p struct {
		Seed          issue    `json:"seed"`
		OwnerFeedback []string `json:"owner_feedback"`
	}
	if err := json.Unmarshal([]byte(h.Prompt), &p); err != nil {
		t.Fatalf("prompt isn't JSON: %v", err)
	}
	return p.OwnerFeedback, p.Seed.Notes
}

func TestOwnerFeedbackAccumulatesAcrossRevisions(t *testing.T) {
	repo, b, model, opts := fixture(t)
	b.edit("NAC-1", func(is *issue) { is.Notes = "- Decided: per chat\n- Ruled out: a new tab" })
	m := newManager(t, opts)
	c := act(t, m, repo, "NAC-1", Action{Action: "develop", Feedback: "Intake verdict: New change.", OwnerNote: "keep it to the folded screen"})
	if c.Phase != "awaiting_approval" {
		t.Fatalf("develop: %s %s", c.Phase, c.Error)
	}
	if got := m.records[c.ID].Feedback; got != "Intake verdict: New change.\n\nRyder's note: keep it to the folded screen" {
		t.Errorf("develop feedback %q", got)
	}
	for _, note := range []string{"second: rename the setting", "third: no new tab"} {
		c = act(t, m, repo, "NAC-1", Action{Action: "revise", Feedback: note})
		if c.Phase != "awaiting_approval" {
			t.Fatalf("revise %q: %s %s", note, c.Phase, c.Error)
		}
	}
	log := m.records[c.ID].FeedbackLog
	if len(log) != 3 || log[0].Action != "develop" || log[2].Text != "third: no new tab" || log[2].At == "" {
		t.Fatalf("log %+v", log)
	}
	if m.records[c.ID].Feedback != "third: no new tab" {
		t.Errorf("Feedback isn't the newest: %q", m.records[c.ID].Feedback)
	}

	// The last author and review calls see every piece, oldest first, and
	// the brainstorm in the seed's notes.
	model.mu.Lock()
	calls := append([]claude.Headless{}, model.calls...)
	model.mu.Unlock()
	var author, reviewer *claude.Headless
	for i := range calls {
		switch calls[i].System {
		case authorSystem:
			author = &calls[i]
		case reviewerSystem:
			reviewer = &calls[i]
		}
	}
	if author == nil || reviewer == nil {
		t.Fatal("no author or review call")
	}
	for name, h := range map[string]*claude.Headless{"author": author, "reviewer": reviewer} {
		fb, notes := promptFeedback(t, *h)
		if len(fb) != 3 || !strings.Contains(fb[0], "(develop): Intake verdict: New change.") || !strings.Contains(fb[0], "folded screen") ||
			!strings.Contains(fb[1], "(revise): second") || !strings.Contains(fb[2], "(revise): third") {
			t.Errorf("%s owner_feedback %q", name, fb)
		}
		if !strings.Contains(notes, "Ruled out: a new tab") {
			t.Errorf("%s seed notes %q", name, notes)
		}
	}
	if !strings.Contains(authorSystem, "owner_feedback") || !strings.Contains(reviewerSystem, "owner_feedback") {
		t.Error("the system prompts don't explain owner_feedback")
	}
}

func TestOwnerFeedbackKeepsTheFirstAndTheNewest(t *testing.T) {
	var log []FeedbackEntry
	for i := 1; i <= 12; i++ {
		log = append(log, FeedbackEntry{At: fmt.Sprintf("t%02d", i), Action: "revise", Text: fmt.Sprintf("note %d", i)})
	}
	got := ownerFeedback(log)
	if len(got) != 1+1+feedbackKeep || got[0] != "t01 (revise): note 1" || got[1] != "(4 earlier entries left out)" ||
		got[2] != "t06 (revise): note 6" || got[len(got)-1] != "t12 (revise): note 12" {
		t.Errorf("trimmed log %q", got)
	}
	if short := ownerFeedback(log[:8]); len(short) != 8 || strings.Contains(strings.Join(short, ""), "left out") {
		t.Errorf("a log within bounds was trimmed: %q", short)
	}
}

func TestFeedbackLogBackfilledFromOlderRecords(t *testing.T) {
	r := record{Feedback: "make it smaller", Change: Change{History: []History{{At: "t1"}, {At: "t2"}}}}
	backfillFeedback(&r)
	if len(r.FeedbackLog) != 1 || r.FeedbackLog[0] != (FeedbackEntry{At: "t2", Action: "develop", Text: "make it smaller"}) {
		t.Errorf("backfilled %+v", r.FeedbackLog)
	}
	backfillFeedback(&r)
	if len(r.FeedbackLog) != 1 {
		t.Error("backfilled twice")
	}
	empty := record{}
	backfillFeedback(&empty)
	if empty.FeedbackLog != nil {
		t.Error("a record without feedback got a log")
	}
}
