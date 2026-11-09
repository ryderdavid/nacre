package server

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/chat"
	"github.com/ryderdavid/nacre/bridge/internal/decisions"
)

// UseDecisions sets the decision store and has it report its changes.
func (s *Server) UseDecisions(store *decisions.Store) {
	s.Decisions = store
	store.OnChange = s.decisionChanged
	if s.Chats != nil {
		// A drafting chat's go-ahead raises intake through the same path as
		// the Draft card's button.
		s.Chats.SetIntakeRaiser(s.raiseFromChat)
		s.Chats.SetDecisionHooks(chat.DecisionHooks{
			PendingKinds: s.pendingKinds,
			Asked:        s.askable,
			State:        s.decisionState,
		})
	}
}

// hookTimeout bounds a chat's lookup of decisions.
const hookTimeout = 20 * time.Second

// pendingKinds are the kinds of the decisions waiting on Ryder in repo, in
// queue order, for a new chat's heads-up. Skipped (deferred) ones don't
// count.
func (s *Server) pendingKinds(repo string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
	defer cancel()
	var kinds []string
	for _, rec := range s.queue(ctx) {
		if d := rec.Decision; d.Repo == repo && d.Waiting() {
			kinds = append(kinds, d.Kind)
		}
	}
	return kinds
}

// askable reports whether a chat in repo may ask about decision id: it is
// still answerable and belongs to that repository.
func (s *Server) askable(repo, id string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
	defer cancel()
	rec, ok := s.lookup(ctx, id)
	return ok && rec.Decision.Answerable() && (repo == "" || rec.Decision.Repo == repo)
}

// decisionState is one line on a decision's current state, for the turn
// an "Answered: …" message starts.
func (s *Server) decisionState(id string) string {
	ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
	defer cancel()
	rec, ok := s.lookup(ctx, id)
	if !ok {
		return ""
	}
	return stateLine(rec.Decision)
}

// stateLine says where a decision stands: its state, the option chosen and
// nacred's message.
func stateLine(d decisions.Decision) string {
	state := d.State
	if state == decisions.StateDeferred {
		state = "skipped" // the app's word for Defer
	}
	parts := []string{state}
	if d.Answer != nil {
		switch {
		case d.Answer.Defer:
		case d.Answer.Option != "":
			label := d.Answer.Option
			for _, o := range d.Options {
				if o.ID == d.Answer.Option {
					label = o.Label
				}
			}
			parts = append(parts, "answered \""+label+"\"")
		}
	}
	if d.Result != nil && d.Result.Message != "" {
		parts = append(parts, d.Result.Message)
	} else if d.Reason != "" {
		parts = append(parts, d.Reason)
	}
	return d.Title + ": " + strings.Join(parts, " · ")
}

// decisionChanged tells clients about a created or changed decision: a
// "decision" notification on /v1/events (which the app turns into a phone
// notification for a new pending one, as for permission requests) and, for a
// decision a chat raised, a "decision" event on that chat's stream.
func (s *Server) decisionChanged(d decisions.Decision) {
	if s.Broker != nil {
		s.Broker.Publish("decision", d)
	}
	if d.ChatID != nil && s.Chats != nil {
		if b, err := json.Marshal(d); err == nil {
			s.Chats.AppendDecision(*d.ChatID, b)
		}
		if d.State == decisions.StatePending {
			// A decision the chat raised is the one it asks next.
			s.Chats.SetAsking(*d.ChatID, d.ID)
		}
	}
}
