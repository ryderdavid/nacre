package server

import (
	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/rounds"
)

// Facts is the state decisions and route protection read.
type Facts interface {
	Round(id string) (api.Round, error)
}

// round reads a round through s.Facts when tests set it, else from the
// rounds manager.
func (s *Server) round(id string) (api.Round, error) {
	if s.Facts != nil {
		return s.Facts.Round(id)
	}
	if s.Rounds == nil {
		return api.Round{}, rounds.ErrNotFound
	}
	r, _, err := s.Rounds.Get(id)
	return r, err
}
