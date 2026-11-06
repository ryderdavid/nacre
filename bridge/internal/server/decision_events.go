package server

import (
	"encoding/json"

	"github.com/ryderdavid/nacre/bridge/internal/decisions"
)

// UseDecisions sets the decision store and has it report its changes.
func (s *Server) UseDecisions(store *decisions.Store) {
	s.Decisions = store
	store.OnChange = s.decisionChanged
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
	}
}
