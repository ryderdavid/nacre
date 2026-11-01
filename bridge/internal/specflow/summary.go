package specflow

import (
	"sort"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

// MaxSummaries bounds the changes summary; changes still being planned are
// listed first, so the Proposals lane is complete unless more are planned.
const MaxSummaries = 50

// InventorySummary says what a proposal for this repository is compared with.
type InventorySummary struct {
	AcceptedSpecs int `json:"accepted_specs"`
	ActiveChanges int `json:"active_changes"`
}

func summarize(inv inventory) *InventorySummary {
	s := &InventorySummary{}
	changes := map[string]bool{}
	for _, f := range inv.Files {
		parts := strings.Split(f.Path, "/")
		switch {
		case len(parts) == 4 && parts[0] == "openspec" && parts[1] == "specs" && parts[3] == "spec.md":
			s.AcceptedSpecs++
		case len(parts) >= 4 && parts[0] == "openspec" && parts[1] == "changes" && parts[2] != "archive":
			changes[parts[2]] = true
		}
	}
	s.ActiveChanges = len(changes)
	return s
}

// EvidenceState folds a section's evidence into the one state a card shows.
func EvidenceState(e Evidence) string {
	switch {
	case e.Landed:
		return api.EvidenceLanded
	case e.Verified:
		return api.EvidenceVerified
	case e.RoundID != "" && e.RoundState != api.RoundLanded && e.RoundState != api.RoundCancelled:
		return api.EvidenceInRound
	}
	return api.EvidenceNone
}

// SeedIDs lists every bead that seeded the change: the original and any
// later seeds that amended it.
func (c Change) seedIDs(aliases []string) []string {
	ids := []string{c.SeedID}
	for _, a := range aliases {
		if a != c.SeedID && !contains(ids, a) {
			ids = append(ids, a)
		}
	}
	return ids
}

type evidenceCache struct {
	key    string
	states map[string]string
}

// Summaries lists the repository's changes, planned ones first, at most
// MaxSummaries, with the total. Section evidence is recomputed only when the
// change or the repository's rounds move (Options.EvidenceStamp).
func (m *Manager) Summaries(repo string) ([]api.ChangeSummary, int, error) {
	repo, e := canonicalRepo(repo)
	if e != nil {
		return nil, 0, e
	}
	m.mu.Lock()
	var recs []*record
	for _, r := range m.records {
		if r.Repo == repo {
			recs = append(recs, clone(r))
		}
	}
	m.mu.Unlock()
	sort.SliceStable(recs, func(i, j int) bool {
		pi, pj := recs[i].Change.EpicID == "", recs[j].Change.EpicID == ""
		if pi != pj {
			return pi
		}
		return recs[i].Change.UpdatedAt > recs[j].Change.UpdatedAt
	})
	total := len(recs)
	if len(recs) > MaxSummaries {
		recs = recs[:MaxSummaries]
	}
	stamp := ""
	if m.opts.EvidenceStamp != nil {
		stamp = m.opts.EvidenceStamp(repo)
	}
	out := make([]api.ChangeSummary, 0, len(recs))
	for _, r := range recs {
		c := r.Change
		states := m.evidenceStates(r, stamp)
		s := api.ChangeSummary{ID: c.ID, ChangeName: c.ChangeName, Phase: c.Phase, Revision: c.Revision,
			ApprovedRevision: c.ApprovedRevision, EpicID: c.EpicID, SeedIDs: c.seedIDs(r.Aliases),
			Sections: []api.SectionSummary{}, Error: c.Error, StaleReason: c.StaleReason, UpdatedAt: c.UpdatedAt}
		for _, sec := range c.Sections {
			ev := states[sec.ID]
			if ev == "" {
				ev = api.EvidenceNone
			}
			s.Sections = append(s.Sections, api.SectionSummary{ID: sec.ID, Title: sec.Title, BeadID: sec.BeadID, Evidence: ev})
		}
		out = append(out, s)
	}
	return out, total, nil
}

func (m *Manager) evidenceStates(r *record, stamp string) map[string]string {
	c := r.Change
	if m.opts.Evidence == nil || len(c.Sections) == 0 {
		return nil
	}
	key := c.Revision + "|" + c.ApprovedRevision + "|" + c.Phase + "|" + c.UpdatedAt + "|" + stamp
	m.mu.Lock()
	cached, ok := m.evidence[c.ID]
	m.mu.Unlock()
	if ok && stamp != "" && cached.key == key {
		return cached.states
	}
	states := map[string]string{}
	for _, e := range m.opts.Evidence(r.Repo, c) {
		states[e.SectionID] = EvidenceState(e)
	}
	m.mu.Lock()
	m.evidence[c.ID] = evidenceCache{key, states}
	m.mu.Unlock()
	return states
}
