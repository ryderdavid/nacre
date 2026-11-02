package rounds

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// recommendSystem asks for one recommended answer per question, so Ryder can
// accept them in one tap instead of working each question out himself.
const recommendSystem = "You recommend answers to questions a pre-flight review asked before implementation work starts. " +
	"For each question pick exactly one of its options, verbatim: the one that best meets the beads' acceptance criteria " +
	"with the least risk and keeps the work moving. Read the repository when it helps. Give the reason in one or two plain " +
	"sentences a busy owner can check at a glance."

var recommendSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []string{"answers"},
	"properties": map[string]any{
		"answers": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"question_id", "recommended", "reason"},
				"properties": map[string]any{
					"question_id": map[string]any{"type": "string"},
					"recommended": map[string]any{"type": "string"},
					"reason":      map[string]any{"type": "string"},
				},
			},
		},
	},
}

// needsRecommendation says whether an unanswered question still lacks a
// recommendation that hasn't been tried yet.
func (r *round) needsRecommendation() bool {
	for _, q := range r.meta.Questions {
		if q.Answer == "" && q.Recommended == "" && !q.RecommendTried && len(q.Options) > 0 {
			return true
		}
	}
	return false
}

// startRecommend asks Claude once for a recommended answer to every open
// question without one. A failure leaves the questions as they are.
func (r *round) startRecommend() {
	meta := clone(r.meta)
	r.goOp("recommend", func(ctx context.Context) func() {
		var s strings.Builder
		s.WriteString("Beads in this round:\n")
		for _, b := range meta.Beads {
			raw, err := r.m.opts.Beads.Get(ctx, meta.RepoPath, b.ID)
			if err != nil {
				continue
			}
			is := parseIssue(raw)
			fmt.Fprintf(&s, "\n## %s: %s\n%s\n", is.ID, is.Title, is.Description)
			if is.AcceptanceCriteria != "" {
				fmt.Fprintf(&s, "Acceptance criteria:\n%s\n", is.AcceptanceCriteria)
			}
		}
		s.WriteString("\nQuestions:\n")
		var asked []string
		for _, q := range meta.Questions {
			if q.Answer != "" || q.Recommended != "" || len(q.Options) == 0 {
				continue
			}
			asked = append(asked, q.ID)
			fmt.Fprintf(&s, "\n- question_id %s (%s, about %s): %s\n  Options:\n", q.ID, q.Kind, strings.Join(q.Beads, ", "), q.Text)
			for _, o := range q.Options {
				fmt.Fprintf(&s, "  - %s\n", o)
			}
		}
		schema, _ := json.Marshal(recommendSchema)
		h := claude.Headless{Prompt: s.String(), System: recommendSystem, Schema: schema, Model: meta.Model, AllowedTools: claude.BdReadTools, ReadRepo: true}
		dir := meta.Workspace
		if dir == "" {
			dir = meta.RepoPath
		}
		out, err := r.m.opts.Headless.Run(ctx, dir, h, nil)
		var got struct {
			Answers []struct {
				QuestionID  string `json:"question_id"`
				Recommended string `json:"recommended"`
				Reason      string `json:"reason"`
			} `json:"answers"`
		}
		if err == nil {
			err = json.Unmarshal(out.Output, &got)
		}
		return func() {
			r.meta.OverheadUSD += out.CostUSD
			for i := range r.meta.Questions {
				q := &r.meta.Questions[i]
				if !contains(asked, q.ID) {
					continue
				}
				q.RecommendTried = true
				for _, a := range got.Answers {
					if a.QuestionID == q.ID && contains(q.Options, strings.TrimSpace(a.Recommended)) {
						q.Recommended, q.Reason = strings.TrimSpace(a.Recommended), strings.TrimSpace(a.Reason)
					}
				}
			}
			if err != nil {
				r.event("plan", "", "Couldn't get recommended answers: "+firstLine(err.Error()))
			}
		}
	})
}

// AcceptRecommended answers every open question that has a recommendation
// with that recommendation.
func (m *Manager) AcceptRecommended(id string) (api.Round, error) {
	r := m.get(id)
	if r == nil {
		return api.Round{}, ErrNotFound
	}
	r.mu.Lock()
	open := 0
	for _, q := range r.meta.Questions {
		if q.Answer == "" {
			open++
			if q.Recommended == "" {
				r.mu.Unlock()
				return clone(r.meta), conflict("not every question has a recommendation yet")
			}
		}
	}
	var todo [][2]string
	for _, q := range r.meta.Questions {
		if q.Answer == "" {
			todo = append(todo, [2]string{q.ID, q.Recommended})
		}
	}
	r.mu.Unlock()
	if open == 0 {
		return api.Round{}, conflict("no question is waiting for an answer")
	}
	var meta api.Round
	for _, a := range todo {
		var err error
		if meta, err = m.Answer(id, a[0], a[1]); err != nil {
			return meta, err
		}
	}
	return meta, nil
}
