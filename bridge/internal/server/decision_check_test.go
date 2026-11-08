package server

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/assist"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
	"github.com/ryderdavid/nacre/bridge/internal/decisions"
	"github.com/ryderdavid/nacre/bridge/internal/specflow"
)

// checkRig records the card checks' prompts and answers each from verdicts.
type checkRig struct {
	mu      sync.Mutex
	prompts []string
	tools   []string
	answer  func(prompt string) string
}

func (c *checkRig) run(_ context.Context, _ string, h claude.Headless) (assist.Outcome, error) {
	c.mu.Lock()
	c.prompts = append(c.prompts, h.Prompt)
	c.tools = h.AllowedTools
	ans := c.answer(h.Prompt)
	c.mu.Unlock()
	return assist.Outcome{Output: json.RawMessage(ans), CostUSD: 0.04}, nil
}

func (c *checkRig) calls() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string{}, c.prompts...)
}

// Opening the feed checks the open cards: one the check finds handled is
// closed with its reason and leaves the queue, the other stays; opening it
// again soon looks at nothing; a reopened card waits again and is never
// checked again.
func TestCardCheckClosesHandledCards(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.verdict = intakeVerdict(specflow.VerdictNew, "")
	_, done, _ := rg.raise(t, `{"kind":"intake","draft":{"title":"Export chats as markdown","description":"Export a chat thread as a markdown file."}}`)
	_, open, _ := rg.raise(t, `{"kind":"intake","draft":{"title":"Palette picker","description":"Pick the app's palette."}}`)
	chk := &checkRig{answer: func(string) string {
		return `{"cards":[{"id":"` + done.ID + `","verdict":"handled","reason":"NAC-50 shipped markdown export and is closed.","evidence":["NAC-50"]},` +
			`{"id":"` + open.ID + `","verdict":"still_needed","reason":"Nothing filed for a palette picker.","evidence":[]}]}`
	}}
	rg.s.CardCheckRun = chk.run

	code, body := rg.do(t, "POST", "/v1/repos/"+rg.repo+"/decisions/check", "")
	if code != 202 || body["running"] != true || body["checking"] != float64(2) || len(body["cards"].([]any)) != 2 {
		t.Fatalf("start: %d %v", code, body)
	}
	rg.s.following.Wait()
	if p := chk.calls(); len(p) != 1 || !strings.Contains(p[0], done.ID) || !strings.Contains(p[0], "Export chats as markdown") {
		t.Fatalf("prompts %v", p)
	}
	if !slices.Contains(chk.tools, "Bash(bd show:*)") || slices.Contains(chk.tools, "Bash(bd close:*)") {
		t.Fatalf("tools %v", chk.tools)
	}
	var st CardCheck
	rg.list(t, "/v1/repos/"+rg.repo+"/decisions/check", &st)
	if st.Running || len(st.Handled) != 1 || st.Handled[0] != done.ID || st.FinishedAt == "" {
		t.Fatalf("status %+v", st)
	}
	got := rg.get(t, done.ID)
	if got.State != decisions.StateExpired || got.Handled == nil || got.Handled.Reason != "NAC-50 shipped markdown export and is closed." ||
		!strings.HasPrefix(got.Reason, "Handled elsewhere: ") || !got.Acknowledged || len(rg.executed()) != 0 {
		t.Fatalf("closed card %+v", got)
	}
	if rg.get(t, open.ID).State != decisions.StatePending {
		t.Fatal("the card still needed was closed")
	}
	for _, d := range rg.queueList(t, "/v1/decisions") {
		if d.ID == done.ID {
			t.Fatal("the handled card is still in the queue")
		}
	}

	// Again within the window: nothing is due, nothing runs.
	if code, body := rg.do(t, "POST", "/v1/repos/"+rg.repo+"/decisions/check", ""); code != 200 || body["running"] != false {
		t.Fatalf("again: %d %v", code, body)
	}
	rg.s.following.Wait()
	if len(chk.calls()) != 1 {
		t.Fatal("checked again within the window")
	}

	// Reopened, it waits again and the check leaves it alone.
	code, body = rg.do(t, "POST", "/v1/decisions/"+done.ID+"/reopen", "")
	if code != 200 || body["state"] != "pending" || body["handled"] != nil {
		t.Fatalf("reopen: %d %v", code, body)
	}
	if code, _ := rg.do(t, "POST", "/v1/decisions/"+open.ID+"/reopen", ""); code != 409 {
		t.Fatalf("reopening a card the check didn't close: %d", code)
	}
	old := cardCheckEvery
	cardCheckEvery = 0
	defer func() { cardCheckEvery = old }()
	rg.do(t, "POST", "/v1/repos/"+rg.repo+"/decisions/check", "")
	rg.s.following.Wait()
	p := chk.calls()
	if len(p) != 2 || strings.Contains(p[1], done.ID) || !strings.Contains(p[1], open.ID) {
		t.Fatalf("after reopen the check looked at %v", p[len(p)-1])
	}
	if rg.get(t, done.ID).State != decisions.StatePending {
		t.Fatal("the reopened card was closed again")
	}
}

// Cards whose answer is bound to a revision, or is Ryder's alone, are never
// checked: approvals, landings, finishes, sign-offs and limits.
func TestCardCheckSkipsProtectedKinds(t *testing.T) {
	for _, kind := range []string{decisions.KindApprove, decisions.KindLand, decisions.KindFinish, decisions.KindSignoff, decisions.KindLimit} {
		if checkable(decisions.Record{Decision: decisions.Decision{Kind: kind, State: decisions.StatePending}}) {
			t.Fatalf("%s is checkable", kind)
		}
	}
	for _, kind := range []string{decisions.KindQuestion, decisions.KindFound, decisions.KindRecovery, decisions.KindIntake} {
		if !checkable(decisions.Record{Decision: decisions.Decision{Kind: kind, State: decisions.StateDeferred}}) {
			t.Fatalf("%s isn't checkable", kind)
		}
	}
	if checkable(decisions.Record{Decision: decisions.Decision{Kind: decisions.KindFound, State: decisions.StateExecuting}}) {
		t.Fatal("an executing card is checkable")
	}
}

// A Needs you item nacred built but never stored (a decision bead) is stored
// closed when the check finds it handled, so the queue stops showing it;
// reopened, it shows again.
func TestCardCheckClosesUnstoredNeedsYouItems(t *testing.T) {
	rg, _ := attentionRig(t)
	var bead decisions.Decision
	for _, d := range rg.queueList(t, "/v1/decisions") {
		if d.Target.Bead == "N-8" {
			bead = d
		}
	}
	if bead.ID == "" || bead.Kind != decisions.KindQuestion {
		t.Fatalf("no card for N-8: %+v", bead)
	}
	if _, stored := rg.s.Decisions.Get(bead.ID); stored {
		t.Fatal("the card is already stored")
	}
	chk := &checkRig{answer: func(string) string {
		return `{"cards":[{"id":"` + bead.ID + `","verdict":"handled","reason":"Commit 1a2b3c picked the palette.","evidence":["1a2b3c"]}]}`
	}}
	rg.s.CardCheckRun = chk.run
	rg.do(t, "POST", "/v1/repos/"+rg.repo+"/decisions/check", "")
	rg.s.following.Wait()
	for _, d := range rg.queueList(t, "/v1/decisions") {
		if d.ID == bead.ID {
			t.Fatalf("still queued: %+v", d)
		}
	}
	if got := rg.get(t, bead.ID); got.State != decisions.StateExpired || got.Handled == nil {
		t.Fatalf("stored %+v", got)
	}
	if code, _ := rg.do(t, "POST", "/v1/decisions/"+bead.ID+"/reopen", ""); code != 200 {
		t.Fatalf("reopen %d", code)
	}
	found := false
	for _, d := range rg.queueList(t, "/v1/decisions") {
		found = found || (d.ID == bead.ID && d.State == decisions.StatePending)
	}
	if !found {
		t.Fatal("the reopened card isn't queued")
	}
}
