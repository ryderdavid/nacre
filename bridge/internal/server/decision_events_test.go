package server

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/chat"
	"github.com/ryderdavid/nacre/bridge/internal/decisions"
)

// A decision raised in a chat is a "decision" event on that chat's stream
// each time it is created or changes, and a "decision" notification on
// /v1/events, as a permission request reaches the phone.
func TestDecisionEventsReachTheChatAndTheNotifications(t *testing.T) {
	rg := newDecisionRig(t)
	notes, unsubscribe := rg.s.Broker.Subscribe()
	defer unsubscribe()
	rg.facts.beads["N-5"] = BeadFacts{ID: "N-5", Status: "open", Labels: []string{"signoff"}}
	_, d, _ := rg.raise(t, `{"kind":"signoff","target":{"bead":"N-5"}}`)

	select {
	case msg := <-notes:
		var got decisions.Decision
		if msg.Event != "decision" || json.Unmarshal(msg.Data, &got) != nil || got.ID != d.ID || got.State != "pending" {
			t.Fatalf("notification %s %s", msg.Event, msg.Data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no notification for a new pending decision")
	}

	rg.answer(t, d.ID, `{"option":"signoff"}`)
	_, events, err := rg.s.Chats.Get(rg.chatID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var states []string
	for _, ev := range events {
		if ev.Kind != chat.KindDecision {
			continue
		}
		var got decisions.Decision
		if err := json.Unmarshal(ev.Decision, &got); err != nil || got.ID != d.ID {
			t.Fatalf("event %+v", ev)
		}
		states = append(states, got.State)
	}
	if len(states) != 3 || states[0] != "pending" || states[1] != "executing" || states[2] != "done" {
		t.Fatalf("chat decision events %v", states)
	}
	// The stream carries them, so the app's card updates in place.
	snap, err := rg.s.Chats.Since(rg.chatID, 0)
	if err != nil || len(snap.Events) != len(events) {
		t.Fatalf("stream %d events, log %d", len(snap.Events), len(events))
	}
}
