package server

import (
	"github.com/ryderdavid/nacre/bridge/internal/decisions"
)

// UseDecisions sets the decision store and has it report its changes.
func (s *Server) UseDecisions(store *decisions.Store) {
	s.Decisions = store
	store.OnChange = s.decisionChanged
}

// decisionChanged tells clients about a created or changed decision: a
// "decision" notification on /v1/events.
func (s *Server) decisionChanged(d decisions.Decision) {
	if s.Broker != nil {
		s.Broker.Publish("decision", d)
	}
}
