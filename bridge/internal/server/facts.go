package server

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ryderdavid/nacre/bridge/internal/gitx"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/repoconfig"
	"github.com/ryderdavid/nacre/bridge/internal/rounds"
	"github.com/ryderdavid/nacre/bridge/internal/specflow"
)

// Facts is the state decisions and route protection read. Tests replace it;
// nacred reads its own managers (liveFacts).
type Facts interface {
	Round(id string) (api.Round, error)
	// Change is the specification change of a seed bead.
	Change(ctx context.Context, repoPath, bead string) (specflow.Envelope, error)
	// Changes summarizes a repository's changes.
	Changes(repoPath string) ([]api.ChangeSummary, error)
	Bead(ctx context.Context, repoPath, id string) (BeadFacts, error)
	// Children are the beads under a bead, closed ones included.
	Children(ctx context.Context, repoPath, id string) ([]BeadFacts, error)
	// RoundsOf are a repository's rounds, ended ones included.
	RoundsOf(repoID string) []api.Round
	// Intake is a New effort draft's intake verdict: one bounded, read-only
	// model call.
	Intake(ctx context.Context, repoPath string, draft specflow.IntakeDraft) (specflow.IntakeVerdict, error)
	// Land is nacre.yaml's landing target and whether landing pushes; push
	// is nil when nacre.yaml can't be read.
	Land(repoPath string) (target string, push *bool)
	// Head is the commit a local branch points at, or "" when it doesn't exist.
	Head(ctx context.Context, repoPath, branch string) string
}

// BeadFacts is what decisions read of a bead.
type BeadFacts struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Status      string         `json:"status"`
	Parent      string         `json:"parent"`
	Type        string         `json:"issue_type"`
	Labels      []string       `json:"labels"`
	Metadata    map[string]any `json:"metadata"`
	UpdatedAt   string         `json:"updated_at"`
}

// HasLabel reports whether the bead carries label.
func (b BeadFacts) HasLabel(label string) bool {
	for _, l := range b.Labels {
		if l == label {
			return true
		}
	}
	return false
}

func (s *Server) facts() Facts {
	if s.Facts != nil {
		return s.Facts
	}
	return liveFacts{s}
}

// round reads a round through the facts.
func (s *Server) round(id string) (api.Round, error) { return s.facts().Round(id) }

// liveFacts reads nacred's own managers.
type liveFacts struct{ s *Server }

func (f liveFacts) Round(id string) (api.Round, error) {
	if f.s.Rounds == nil {
		return api.Round{}, rounds.ErrNotFound
	}
	r, _, err := f.s.Rounds.Get(id)
	return r, err
}

func (f liveFacts) Change(ctx context.Context, repoPath, bead string) (specflow.Envelope, error) {
	if f.s.Changes == nil {
		return specflow.Envelope{}, specflow.ErrUnavailable
	}
	return f.s.Changes.Get(ctx, repoPath, bead)
}

func (f liveFacts) Changes(repoPath string) ([]api.ChangeSummary, error) {
	if f.s.Changes == nil {
		return nil, nil
	}
	list, _, err := f.s.Changes.Summaries(repoPath)
	return list, err
}

func (f liveFacts) Bead(ctx context.Context, repoPath, id string) (BeadFacts, error) {
	raw, err := f.s.BD.Get(ctx, repoPath, id)
	if err != nil {
		return BeadFacts{}, err
	}
	var b BeadFacts
	err = json.Unmarshal(raw, &b)
	return b, err
}

func (f liveFacts) Children(ctx context.Context, repoPath, id string) ([]BeadFacts, error) {
	raw, err := f.s.BD.Children(ctx, repoPath, id)
	if err != nil {
		return nil, err
	}
	out := make([]BeadFacts, 0, len(raw))
	for _, r := range raw {
		var b BeadFacts
		if err := json.Unmarshal(r, &b); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

func (f liveFacts) RoundsOf(repoID string) []api.Round {
	if f.s.Rounds == nil {
		return nil
	}
	return f.s.Rounds.List(repoID, false)
}

func (f liveFacts) Intake(ctx context.Context, repoPath string, draft specflow.IntakeDraft) (specflow.IntakeVerdict, error) {
	if f.s.Changes == nil {
		return specflow.IntakeVerdict{}, fmt.Errorf("%w: Specification workflows are unavailable on this bridge.", specflow.ErrUnavailable)
	}
	return f.s.Changes.Intake(ctx, repoPath, draft)
}

func (f liveFacts) Head(ctx context.Context, repoPath, branch string) string {
	sha, err := gitx.RevParse(ctx, repoPath, "refs/heads/"+branch)
	if err != nil {
		return ""
	}
	return sha
}

func (f liveFacts) Land(repoPath string) (string, *bool) {
	cfg, err := repoconfig.Load(repoPath)
	if err != nil || cfg == nil {
		return repoconfig.DefaultTarget, nil
	}
	push := cfg.Land.Push
	return cfg.Land.Target, &push
}
