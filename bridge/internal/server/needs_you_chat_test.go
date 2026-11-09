package server

import (
	"strings"
	"testing"
)

// The walk-through's hooks: a raised decision becomes the chat's ask, and
// only that repository's chats may ask about it.
func TestNeedsYouHooks(t *testing.T) {
	rg := newDecisionRig(t)
	rg.s.UseDecisions(rg.s.Decisions)
	rg.facts.setChange("N-1", awaitingChange("4c0ffee1234"))
	code, d, raw := rg.raise(t, `{"kind":"approve","target":{"bead":"N-1"}}`)
	if code != 201 {
		t.Fatalf("raise: %d %v", code, raw)
	}
	rec, _, err := rg.s.Chats.Get(rg.chatID, 0)
	if err != nil || rec.Asking != d.ID {
		t.Fatalf("asking = %q, want the raised %s (%v)", rec.Asking, d.ID, err)
	}
	if !rg.s.askable(rg.repo, d.ID) || rg.s.askable("other-repo", d.ID) || rg.s.askable(rg.repo, "dc_nope") {
		t.Fatal("askable is wrong")
	}
	kinds := rg.s.pendingKinds(rg.repo)
	if len(kinds) == 0 || kinds[0] != "approve" {
		t.Fatalf("pending kinds = %v", kinds)
	}
	if st := rg.s.decisionState(d.ID); !strings.Contains(st, "pending") || !strings.HasPrefix(st, d.Title) {
		t.Fatalf("state = %q", st)
	}
	if code, _ := rg.answer(t, d.ID, `{"defer":true}`); code != 200 {
		t.Fatalf("skip: %d", code)
	}
	if st := rg.s.decisionState(d.ID); !strings.Contains(st, "skipped") {
		t.Fatalf("state after skip = %q", st)
	}
	for _, k := range rg.s.pendingKinds(rg.repo) {
		if k == "approve" {
			t.Fatal("a skipped decision still counts")
		}
	}
}
