package server

import (
	"context"

	"github.com/ryderdavid/nacre/bridge/internal/decisions"
)

// attentionDecisions are the Needs you items as decisions.
func (s *Server) attentionDecisions(ctx context.Context) []decisions.Record { return nil }

// attentionStill reports whether a Needs you item still waits.
func (s *Server) attentionStill(ctx context.Context, id string) bool { return true }
