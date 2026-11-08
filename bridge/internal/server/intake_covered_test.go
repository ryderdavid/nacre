package server

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/specflow"
)

// The case that prompted this: a chat raised intake for the Chat and Jobs
// redesign; ten minutes later the same work was filed and shipped from a
// desktop session. That bead is found; an unrelated one filed since, and a
// matching one filed before the card, aren't.
func TestCoveringBeadFindsWorkFiledSinceTheCard(t *testing.T) {
	draftTitle := "Chat and Jobs: bring the screens up to Material 3 Expressive"
	draftDesc := "The Chat and Jobs tabs still look like classic Android even though the app runs MaterialExpressiveTheme. Convert them to the latest Material 3 Expressive look: shaped avatar and large app-bar header, morphing grouped bubbles, a floating-pill composer with a morphing send/stop button, expressive button groups and large selectable shapes in decision cards, wavy progress and tonal job rows, a floating toolbar for New chat."
	since := "2026-11-07T23:12:54Z"
	drawer := BeadFacts{ID: "NAC-3ik", Status: "closed", CreatedAt: "2026-11-07T23:22:08Z", ClosedAt: "2026-11-07T23:45:04Z",
		Title:       "Chat drawer: chats as foldable cards, job cards, expressive button group",
		Description: "Ryder can't start a second/third chat in a project or tell which chat works on what. Redesign the board's chat drawer: Chats | Jobs as an M3 Expressive connected button group (not the legacy tab row); each chat is a card showing its job (bead/spec, wavy progress, status) that unfolds into the conversation and folds back, other chats as slim rows while one is open; New chat split button at the bottom of the column; Jobs as foldable work cards with Open chat / Start a chat about this."}
	other := BeadFacts{ID: "NAC-69j", Status: "open", CreatedAt: "2026-11-08T00:30:00Z",
		Title:       "Intake cards don't notice work done elsewhere after they were raised",
		Description: "An app chat raised an intake card; ten minutes later the same work was filed and shipped from a desktop session."}
	older := drawer
	older.ID, older.CreatedAt, older.ClosedAt, older.Status = "NAC-old", "2026-11-01T10:00:00Z", "", "open"

	m, ok := coveringBead(draftTitle, draftDesc, since, "", []BeadFacts{other, older, drawer})
	if !ok || m.bead.ID != "NAC-3ik" || !m.filed || !m.closed {
		t.Fatalf("match %+v %v", m, ok)
	}
	if _, ok := coveringBead(draftTitle, draftDesc, since, "", []BeadFacts{other, older}); ok {
		t.Fatal("matched unrelated or older work")
	}
	// An old bead closed since the card counts: the work was finished.
	older.ClosedAt, older.Status = "2026-11-08T01:00:00Z", "closed"
	if m, ok := coveringBead(draftTitle, draftDesc, since, "", []BeadFacts{older}); !ok || m.filed || !m.closed {
		t.Fatalf("closed since: %+v %v", m, ok)
	}
	// The bead the card is about is never its own cover.
	if _, ok := coveringBead(draftTitle, draftDesc, since, "NAC-3ik", []BeadFacts{drawer}); ok {
		t.Fatal("the card's own bead matched")
	}
}

// Listing the queue offers a bead filed since an intake card was raised,
// first and recommended, and says so; choosing it records the answer,
// creates and spends nothing, and the chat follows that bead.
func TestOpenIntakeOffersWorkFiledSince(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.verdict = intakeVerdict(specflow.VerdictNew, "")
	_, d, _ := rg.raise(t, `{"kind":"intake","draft":{"title":"Keep drafts per chat thread","description":"Each chat thread keeps its own unsent draft when you switch threads."}}`)
	soon := time.Now().Add(time.Minute).UTC().Format(time.RFC3339)
	rg.facts.setBead(BeadFacts{ID: "N-40", Title: "Unsent drafts kept per chat thread", Description: "Switching threads keeps each chat's own draft.", Status: "in_progress", CreatedAt: soon})
	rg.facts.setBead(BeadFacts{ID: "N-41", Title: "Palette picker", Status: "open", CreatedAt: soon})

	ds := byKind(rg.queueList(t, "/v1/decisions"))["intake"]
	if len(ds) != 1 {
		t.Fatalf("intake cards %+v", ds)
	}
	c := ds[0]
	if c.Options[0].ID != "covered_by:N-40" || !c.Options[0].Recommended || c.Options[0].Label != "Already covered by N-40" ||
		!strings.Contains(c.Options[0].Detail, "was filed after this was raised") {
		t.Fatalf("first option %+v", c.Options[0])
	}
	for _, o := range c.Options[1:] {
		if o.Recommended {
			t.Fatalf("still recommended: %+v", o)
		}
	}
	if !strings.HasPrefix(c.Consequences[0], "Since this was raised, N-40 “Unsent drafts kept per chat thread” was filed") ||
		c.Evidence[0].Kind != "bead" || c.Evidence[0].Ref != "N-40" {
		t.Fatalf("card %+v", c)
	}
	// Listing again doesn't offer it twice.
	if again := byKind(rg.queueList(t, "/v1/decisions"))["intake"][0]; len(again.Options) != len(c.Options) {
		t.Fatalf("offered twice: %+v", again.Options)
	}

	code, ans := rg.answer(t, d.ID, `{"option":"covered_by:N-40"}`)
	if code != 200 || ans.State != "done" || ans.Result.Message != "Recorded: Already covered by N-40" || len(rg.executed()) != 0 {
		t.Fatalf("answer %d %+v %+v", code, ans, rg.executed())
	}
	if rg.s.Decisions.ChatOf(beadLink(rg.repo, "N-40")) != rg.chatID {
		t.Fatal("the chat doesn't follow N-40")
	}
}

// Answering a card that hasn't been listed since a covering bead was filed
// is refused once, so Ryder sees it; answering again goes ahead.
func TestAnsweringIntakeStopsOnceForWorkFiledSince(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.verdict = intakeVerdict(specflow.VerdictNew, "")
	_, d, _ := rg.raise(t, `{"kind":"intake","draft":{"title":"Export chats as markdown files","description":"Export a chat thread as a markdown file."}}`)
	rg.facts.setBead(BeadFacts{ID: "N-50", Title: "Export chat threads to markdown", Status: "closed",
		CreatedAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339), ClosedAt: time.Now().Add(2 * time.Minute).UTC().Format(time.RFC3339)})

	code, _ := rg.answer(t, d.ID, `{"option":"bead"}`)
	if code != http.StatusConflict || len(rg.executed()) != 0 {
		t.Fatalf("first answer: %d %+v", code, rg.executed())
	}
	if c := rg.get(t, d.ID); c.Options[0].ID != "covered_by:N-50" || !strings.Contains(c.Consequences[0], "was filed and closed") {
		t.Fatalf("card %+v", c)
	}
	rg.replies(map[string]func(execCall) (int, string){"/issues": func(execCall) (int, string) { return 201, `{"issue":{"id":"N-51","title":"x"}}` }})
	code, got := rg.answer(t, d.ID, `{"option":"bead"}`)
	if code != 200 || got.State != "done" || len(rg.executed()) != 1 {
		t.Fatalf("second answer: %d %+v %+v", code, got, rg.executed())
	}
}
