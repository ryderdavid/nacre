package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/attention"
	"github.com/ryderdavid/nacre/bridge/internal/decisions"
	"github.com/ryderdavid/nacre/bridge/internal/sse"
)

// attentionRig is a decision rig whose Needs you holds a round ready to
// land, a round asking two questions, a bead waiting for sign-off and a
// decision bead.
func attentionRig(t *testing.T) (*decisionRig, []api.Round) {
	t.Helper()
	rg := newDecisionRig(t)
	ready := landRound()
	ready.RepoID, ready.RepoName = rg.repo, "repo"
	asking := api.Round{ID: "rdask", RepoID: rg.repo, State: api.RoundNeedsDecision, UpdatedAt: "2026-11-06T09:00:00Z",
		Beads: []api.RoundBead{{ID: "N-1", Title: "One"}},
		Questions: []api.Question{
			{ID: "q1", Text: "Keep drafts per chat?", Options: []string{"Per chat", "Per project"}, Recommended: "Per chat", Reason: "Threads differ"},
			{ID: "q2", Text: "Order N-2 before N-3?", Options: []string{"Yes", "No"}},
		}}
	roundsList := []api.Round{ready, asking}
	rg.facts.setRound(ready)
	rg.facts.setRound(asking)
	beads := []api.Issue{
		json.RawMessage(`{"id":"N-5","title":"Drafts survive","status":"open","labels":["signoff"],"metadata":{"signoff":"Kill the app mid-draft"},"updated_at":"2026-11-05T09:00:00Z"}`),
		json.RawMessage(`{"id":"N-8","title":"Pick the palette","status":"open","issue_type":"decision","updated_at":"2026-11-04T09:00:00Z"}`),
	}
	rg.facts.beads["N-5"] = BeadFacts{ID: "N-5", Title: "Drafts survive", Status: "open", Labels: []string{"signoff"}, Metadata: map[string]any{"signoff": "Kill the app mid-draft"}}
	rg.s.Attention = &attention.Builder{
		Repos:  rg.s.Repos.Refs,
		Issues: func(context.Context, string) ([]api.Issue, error) { return beads, nil },
		Rounds: func() []api.Round {
			rg.facts.mu.Lock()
			defer rg.facts.mu.Unlock()
			out := []api.Round{}
			for _, r := range roundsList {
				out = append(out, rg.facts.rounds[r.ID])
			}
			return out
		},
	}
	return rg, roundsList
}

func (rg *decisionRig) queueList(t *testing.T, path string) []decisions.Decision {
	t.Helper()
	var out struct{ Decisions []decisions.Decision }
	if code := rg.list(t, path, &out); code != 200 {
		t.Fatalf("%s: %d", path, code)
	}
	return out.Decisions
}

func byKind(ds []decisions.Decision) map[string][]decisions.Decision {
	out := map[string][]decisions.Decision{}
	for _, d := range ds {
		out[d.Kind] = append(out[d.Kind], d)
	}
	return out
}

func TestNeedsYouItemsAreDecisionsWithStableIDs(t *testing.T) {
	rg, _ := attentionRig(t)
	first := rg.queueList(t, "/v1/decisions")
	again := rg.queueList(t, "/v1/decisions?repo="+rg.repo)
	// Two questions, the decision bead, the parked bead, the sign-off and the land.
	if len(first) != 6 || len(again) != 6 {
		t.Fatalf("queue %+v", first)
	}
	for i := range first {
		if first[i].ID != again[i].ID || first[i].Source != "attention" || first[i].ChatID != nil || first[i].State != "pending" {
			t.Fatalf("unstable or wrong: %+v vs %+v", first[i], again[i])
		}
	}
	k := byKind(first)
	// Questions first: one per round question, then approvals and checks, then landing.
	if first[0].Kind != "question" || first[len(first)-1].Kind != "land" {
		t.Errorf("order %v", []string{first[0].Kind, first[len(first)-1].Kind})
	}
	land := k["land"][0]
	if land.Options[0].ID != "land" || !land.Options[0].Protected || land.Target.Commit != "a1b2c3d4e5" || !has(land.Consequences, "Closes 1 bead: N-2") {
		t.Errorf("land from Needs you %+v", land)
	}
	sign := k["signoff"][0]
	if sign.Body != "Kill the app mid-draft" || sign.RejectLabel != "Not right" {
		t.Errorf("signoff from Needs you %+v", sign)
	}
	var q1, q2 decisions.Decision
	var bead decisions.Decision
	for _, q := range k["question"] {
		switch {
		case q.Target.Question == "q1":
			q1 = q
		case q.Target.Question == "q2":
			q2 = q
		case q.Target.Bead == "N-8":
			bead = q
		}
	}
	if q1.Body != "Keep drafts per chat?" || len(q1.Options) != 3 || q1.Options[0].Label != "Per chat" || !q1.Options[0].Recommended ||
		q1.Options[2].ID != "own_words" || q1.Options[2].Input == nil || q1.Options[2].Input.Kind != "text" || !has(q1.Consequences, "Threads differ") {
		t.Errorf("q1 %+v", q1)
	}
	if q2.ID == "" || q2.ID == q1.ID {
		t.Errorf("q2 %+v", q2)
	}
	if bead.ID == "" || bead.Options[len(bead.Options)-1].ID != "answer" || len(bead.Evidence) == 0 {
		t.Errorf("bead decision %+v", bead)
	}

	// In my own words answers the question with the typed text.
	code, got := rg.answer(t, q1.ID, `{"option":"own_words","input":"keep it per chat"}`)
	calls := rg.executed()
	if code != 200 || got.State != "done" || len(calls) != 1 || calls[0].path != "/v1/rounds/rdask/answers" ||
		calls[0].body["question_id"] != "q1" || calls[0].body["answer"] != "keep it per chat" {
		t.Fatalf("own words: %d %+v %+v", code, got, calls)
	}
	// q2 keeps its id though q1 was answered, and its card is still live.
	asking, _ := rg.facts.Round("rdask")
	asking.Questions[0].Answer = "keep it per chat"
	rg.facts.setRound(asking)
	if code, got := rg.answer(t, q2.ID, `{"option":"yes"}`); code != 200 || got.State != "done" {
		t.Fatalf("q2: %d %+v", code, got)
	}
}

// The chat's land decision and the Needs you item for the same round are
// one decision: the queue shows it once, and answering it anywhere is seen
// everywhere.
func TestOneDecisionServesTheQueueAndTheChat(t *testing.T) {
	rg, _ := attentionRig(t)
	code, raised, raw := rg.raise(t, `{"kind":"land","target":{"round":"rdto3k"}}`)
	if code != 201 {
		t.Fatalf("raise %d %v", code, raw)
	}
	lands := byKind(rg.queueList(t, "/v1/decisions"))["land"]
	if len(lands) != 1 || lands[0].ID != raised.ID || lands[0].Source != "agent" {
		t.Fatalf("queue lands %+v", lands)
	}
	if code, got := rg.answer(t, raised.ID, `{"reject":true,"note":"after the device check"}`); code != 200 || got.State != "held" {
		t.Fatalf("held %d %+v", code, got)
	}
	lands = byKind(rg.queueList(t, "/v1/decisions"))["land"]
	if len(lands) != 1 || lands[0].State != "held" {
		t.Fatalf("after answering %+v", lands)
	}
	// Answering a Needs you decision stores it under the same id.
	sign := byKind(rg.queueList(t, "/v1/decisions"))["signoff"][0]
	if code, got := rg.answer(t, sign.ID, `{"defer":true}`); code != 200 || got.ID != sign.ID || got.State != "deferred" {
		t.Fatalf("defer %d %+v", code, got)
	}
	if again := byKind(rg.queueList(t, "/v1/decisions"))["signoff"]; len(again) != 1 || again[0].State != "deferred" {
		t.Fatalf("after defer %+v", again)
	}
	if d := rg.get(t, sign.ID); d.State != "deferred" {
		t.Fatalf("GET %+v", d)
	}
}

// A Needs you decision whose item changed state is no longer answered.
func TestANeedsYouDecisionExpiresWhenItsItemMoves(t *testing.T) {
	rg, _ := attentionRig(t)
	land := byKind(rg.queueList(t, "/v1/decisions"))["land"][0]
	// Deferred first, so it is stored and outlives its item.
	if code, _ := rg.answer(t, land.ID, `{"defer":true}`); code != 200 {
		t.Fatalf("defer %d", code)
	}
	r := landRound()
	r.RepoID = rg.repo
	r.State = api.RoundImplementing
	rg.facts.setRound(r)
	code, got := rg.answer(t, land.ID, `{"option":"land"}`)
	if code != http.StatusOK || got.State != "expired" || len(rg.executed()) != 0 {
		t.Fatalf("moved item: %d %+v", code, got)
	}
}

// A deferred Needs you sign-off whose check changes expires, and the card for
// the new check takes its place under its own id instead of being hidden
// behind the expired one.
func TestChangedCheckReplacesADeferredNeedsYouCard(t *testing.T) {
	rg, _ := attentionRig(t)
	old := revalidateEvery
	revalidateEvery = 0
	defer func() { revalidateEvery = old }()
	first := byKind(rg.queueList(t, "/v1/decisions"))["signoff"]
	if len(first) != 1 {
		t.Fatalf("signoff %+v", first)
	}
	if code, _ := rg.answer(t, first[0].ID, `{"defer":true}`); code != 200 {
		t.Fatalf("defer %d", code)
	}
	rg.facts.setBead(BeadFacts{ID: "N-5", Title: "Drafts survive", Status: "open", Labels: []string{"signoff"},
		Metadata: map[string]any{"signoff": "Kill the app mid-draft, then rotate it"}})
	var open []decisions.Decision
	for _, d := range rg.queueList(t, "/v1/decisions") {
		if d.Kind == "signoff" {
			open = append(open, d)
		}
	}
	if len(open) != 1 || open[0].ID == first[0].ID || open[0].State != "pending" || open[0].Body != "Kill the app mid-draft, then rotate it" {
		t.Fatalf("after the check changed: %+v", open)
	}
	if got := rg.get(t, first[0].ID); got.State != "expired" {
		t.Fatalf("old card %+v", got)
	}
}

// A Needs you decision is announced on /v1/events the first time it is seen
// after the first look; what already waited at start is not announced.
func TestNewNeedsYouDecisionsAreAnnounced(t *testing.T) {
	rg, _ := attentionRig(t)
	if rg.s.Broker == nil {
		rg.s.Broker = sse.NewBroker()
	}
	events, stop := rg.s.Broker.Subscribe()
	defer stop()
	a := decisions.Record{Decision: decisions.Decision{ID: "dc_a", Kind: "question", State: "pending"}}
	b := decisions.Record{Decision: decisions.Decision{ID: "dc_b", Kind: "question", State: "pending"}}
	rg.s.announce([]decisions.Record{a})
	rg.s.announce([]decisions.Record{a, b})
	rg.s.announce([]decisions.Record{a, b})
	var got []string
	for done := false; !done; {
		select {
		case m := <-events:
			if m.Event == "decision" {
				got = append(got, string(m.Data))
			}
		default:
			done = true
		}
	}
	if len(got) != 1 || !strings.Contains(got[0], `"dc_b"`) {
		t.Fatalf("announced %v", got)
	}
}

// A deferred Needs you card isn't expired by a list that couldn't be read:
// after a restart (nothing cached) with bd failing, it stays deferred.
func TestAnUnreadableListDoesntExpireWaitingCards(t *testing.T) {
	rg, _ := attentionRig(t)
	old := revalidateEvery
	revalidateEvery = 0
	defer func() { revalidateEvery = old }()
	sign := byKind(rg.queueList(t, "/v1/decisions"))["signoff"][0]
	if code, _ := rg.answer(t, sign.ID, `{"defer":true}`); code != 200 {
		t.Fatalf("defer %d", code)
	}
	rg.s.Attention = &attention.Builder{
		Repos:  rg.s.Repos.Refs,
		Issues: func(context.Context, string) ([]api.Issue, error) { return nil, context.DeadlineExceeded },
	}
	rg.queueList(t, "/v1/decisions")
	if got := rg.get(t, sign.ID); got.State != "deferred" {
		t.Fatalf("expired on an unreadable list: %+v", got)
	}
}

// A question handed back after its answer failed must not reuse the id of the
// answer stored as done, or the queue hides it and the round waits on nothing.
func TestAReopenedQuestionIsANewDecision(t *testing.T) {
	q := api.AttentionQuestion{ID: "q1"}
	first := questionDecisionID("r1", q)
	q.Reopened = 1
	again := questionDecisionID("r1", q)
	q.Reopened = 2
	third := questionDecisionID("r1", q)
	if first == again || again == third || first == third {
		t.Fatalf("reopened question kept an old id: %s %s %s", first, again, third)
	}
	if first != digestID("question", "r1", "q1") {
		t.Fatal("a question asked once keeps its existing id")
	}
}
