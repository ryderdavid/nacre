package specflow

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// Guide says in plain words where a change waiting on the owner stands and
// what to do next, so the change screen leads with one recommended step.
type Guide struct {
	// For is the guideKey the guide was written for; a guide for an older
	// state is never shown.
	For         string        `json:"for"`
	Summary     string        `json:"summary"`
	Status      string        `json:"status"`
	Blocking    []string      `json:"blocking"`
	Recommended GuideOption   `json:"recommended"`
	Options     []GuideOption `json:"options"`
}

// GuideOption is one next step: approve, revise (with Feedback as the
// owner's guidance), resume, or close (shelve the bead).
type GuideOption struct {
	Action   string `json:"action"`
	Label    string `json:"label"`
	Feedback string `json:"feedback,omitempty"`
	Why      string `json:"why"`
}

const guideSystem = "You help a busy owner decide the next step on an OpenSpec change that has stopped and is waiting on them. " +
	"Write short, plain sentences without jargon or file paths. summary: one or two sentences on what the change does for the user of the app. " +
	"status: one sentence on how it went: reviews, cost so far, and why it stopped. blocking: each unresolved blocker or high finding, and a medium one when few remain, " +
	"as one plain sentence each; empty when nothing blocks. recommended: the single best next step. For revise, write feedback as the concrete decisions the author " +
	"should make, resolving every blocking point, in at most three sentences the owner can accept as is. why: one sentence on why this step. " +
	"options: two to four real alternatives, each with why saying what it costs or leads to. A different decision on a blocking point is a revise option with its own feedback. " +
	"Use only the allowed actions. Labels are verb-first, sentence case, at most six words, and never mention cost."

var guideSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []string{"summary", "status", "blocking", "recommended", "options"},
	"properties": map[string]any{
		"summary":     map[string]any{"type": "string"},
		"status":      map[string]any{"type": "string"},
		"blocking":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"recommended": guideOptionSchema,
		"options":     map[string]any{"type": "array", "items": guideOptionSchema},
	},
}

var guideOptionSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []string{"action", "label", "feedback", "why"},
	"properties": map[string]any{
		"action":   map[string]any{"type": "string"},
		"label":    map[string]any{"type": "string"},
		"feedback": map[string]any{"type": "string"},
		"why":      map[string]any{"type": "string"},
	},
}

// guidedPhases are the phases that wait on the owner.
var guidedPhases = map[string]bool{"awaiting_approval": true, "needs_attention": true, "stale": true, "covered": true}

func guideKey(c Change) string {
	return fmt.Sprintf("%s|%s|%s|%d", c.Phase, c.Revision, c.Error, len(c.Findings))
}

// guideActions are the next steps the change's state allows.
func guideActions(r *record) []string {
	out := []string{"revise", "close"}
	switch r.Change.Phase {
	case "awaiting_approval":
		if r.Change.Revision != "" {
			out = append(out, "approve")
		}
	case "needs_attention", "interrupted":
		out = append(out, "resume")
	case "stale":
		if r.Change.Adoption != nil && r.Change.ApprovedRevision == "" {
			out = append(out, "resume")
		}
	}
	return out
}

// maybeGuide starts writing a guide for a change waiting on the owner when it
// has none for its current state. Called with m.mu held.
func (m *Manager) maybeGuideLocked(r *record) {
	if r == nil || !guidedPhases[r.Change.Phase] || m.busy[r.Repo] || m.opts.Headless == nil {
		return
	}
	key := guideKey(r.Change)
	if g := r.Change.Guide; (g != nil && g.For == key) || m.guiding[r.Change.ID] == key {
		return
	}
	m.guiding[r.Change.ID] = key
	snap := clone(r)
	m.wg.Add(1)
	go m.writeGuide(snap, key)
}

func (m *Manager) writeGuide(r *record, key string) {
	defer m.wg.Done()
	ctx, cancel := context.WithTimeout(m.ctx, OperationTimeout)
	defer cancel()
	allowed := guideActions(r)
	schema, _ := json.Marshal(guideSchema)
	out, e := m.opts.Headless.Run(ctx, r.Repo, claude.Headless{Prompt: guidePrompt(r, allowed), System: guideSystem, Schema: schema}, nil)
	var g Guide
	if e == nil {
		e = json.Unmarshal(out.Output, &g)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	live := m.records[r.Change.ID]
	if live == nil {
		return
	}
	live.Change.CostUSD += out.CostUSD
	if e != nil || !keepAllowed(&g, allowed) || guideKey(live.Change) != key || m.busy[live.Repo] {
		// The screen falls back to its own next action; this state is not tried again.
		_ = m.persistLocked(live)
		return
	}
	g.For = key
	live.Change.Guide = &g
	if m.persistLocked(live) == nil && m.opts.Notify != nil {
		m.opts.Notify(live.Repo)
	}
}

// keepAllowed drops options outside the allowed actions; the guide is unusable
// when its recommendation is.
func keepAllowed(g *Guide, allowed []string) bool {
	ok := func(o GuideOption) bool { return contains(allowed, o.Action) && strings.TrimSpace(o.Label) != "" }
	if !ok(g.Recommended) {
		return false
	}
	kept := g.Options[:0]
	for _, o := range g.Options {
		if ok(o) {
			kept = append(kept, o)
		}
	}
	g.Options = kept
	return true
}

// persistLocked saves the record without moving its updated time, so writing
// a guide never reorders the board.
func (m *Manager) persistLocked(r *record) error {
	return atomicJSON(filepath.Join(m.opts.Dir, r.Change.ID+".json"), r)
}

func guidePrompt(r *record, allowed []string) string {
	c := r.Change
	var s strings.Builder
	fmt.Fprintf(&s, "Bead %s: %s\n%s\n\n", r.Seed.ID, r.Seed.Title, r.Seed.Description)
	fmt.Fprintf(&s, "Change %s (route %s), phase %s, $%.2f spent so far.\nSummary: %s\n", c.ChangeName, c.Route, c.Phase, c.CostUSD, c.Summary)
	if c.Error != "" {
		fmt.Fprintf(&s, "Why it stopped: %s\n", c.Error)
	}
	if c.StaleReason != "" {
		fmt.Fprintf(&s, "What changed since review: %s\n", c.StaleReason)
	}
	s.WriteString("\nReview findings:\n")
	for _, f := range c.Findings {
		fmt.Fprintf(&s, "- [%s, %s] %s: %s\n", f.Severity, f.Disposition, f.Summary, f.Detail)
	}
	s.WriteString("\nRecent history:\n")
	h := c.History
	if len(h) > 20 {
		h = h[len(h)-20:]
	}
	for _, e := range h {
		fmt.Fprintf(&s, "- %s: %s\n", e.Kind, e.Text)
	}
	fmt.Fprintf(&s, "\nAllowed actions: %s.\n", strings.Join(allowed, ", "))
	s.WriteString("approve: approve the reviewed revision and run its work. revise: draft and review the change again with the feedback, up to $5. " +
		"resume: continue the saved workflow without starting over. close: shelve the bead; nothing has been built.\n")
	return s.String()
}
