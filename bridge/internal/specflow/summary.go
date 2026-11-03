package specflow

import (
	"fmt"
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

// startedAt is when the change record was started: its first history entry,
// or created_at when it has no history. It never changes.
func startedAt(c Change) string {
	if len(c.History) > 0 && c.History[0].At != "" {
		return c.History[0].At
	}
	return c.CreatedAt
}

// failedDraft: drafting stopped before the proposal was created.
func failedDraft(c Change) bool {
	return c.CreatedAt == "" && (c.Phase == "needs_attention" || c.Phase == "interrupted")
}

// draftOf derives a change's draft from its phase and created_at: pending
// while the first proposal is drafted, failed (with the change's error) if
// drafting stopped before it was created, unless that was dismissed.
func draftOf(r *record) *api.ChangeDraft {
	c := r.Change
	switch {
	case c.CreatedAt != "":
		return nil
	case c.Phase == "drafting":
		return &api.ChangeDraft{Kind: api.DraftPending}
	case failedDraft(c) && !r.DraftDismissed:
		return &api.ChangeDraft{Kind: api.DraftFailed, Message: c.Error}
	}
	return nil
}

// DismissDraft hides a change's failed draft until a new develop is
// accepted, and returns the change's summary. It is idempotent: a pending,
// created or already dismissed change is left as it is. changed reports
// whether the dismissal was recorded now.
func (m *Manager) DismissDraft(repo, changeID string) (summary api.ChangeSummary, changed bool, err error) {
	repo, e := canonicalRepo(repo)
	if e != nil {
		return api.ChangeSummary{}, false, e
	}
	m.mu.Lock()
	live := m.records[changeID]
	if live == nil || live.Repo != repo {
		m.mu.Unlock()
		return api.ChangeSummary{}, false, fmt.Errorf("%w: no specification change %s in this repository", ErrConflict, changeID)
	}
	r := clone(live)
	if !r.DraftDismissed && failedDraft(r.Change) {
		r.DraftDismissed = true
		history(r, "draft_dismissed", "The owner dismissed the failed draft.")
		if e := m.saveLocked(r); e != nil {
			m.mu.Unlock()
			return api.ChangeSummary{}, false, e
		}
		changed = true
	}
	m.mu.Unlock()
	stamp := ""
	if m.opts.EvidenceStamp != nil {
		stamp = m.opts.EvidenceStamp(repo)
	}
	return m.summary(r, stamp), changed, nil
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
		out = append(out, m.summary(r, stamp))
	}
	return out, total, nil
}

// summary is one change's entry in the changes summary.
func (m *Manager) summary(r *record, stamp string) api.ChangeSummary {
	repo := r.Repo
	c := r.Change
	c.AsApproved = asApproved(c, repo)
	r.Change.AsApproved = c.AsApproved
	states := m.evidenceStates(r, stamp)
	s := api.ChangeSummary{ID: c.ID, ChangeName: c.ChangeName, Phase: c.Phase, Revision: c.Revision,
		ApprovedRevision: c.ApprovedRevision, EpicID: c.EpicID, SeedIDs: c.seedIDs(r.Aliases), AsApproved: c.AsApproved,
		Sections: []api.SectionSummary{}, Error: c.Error, StaleReason: c.StaleReason, PendingFound: r.PendingFound, UpdatedAt: c.UpdatedAt,
		CreatedAt: c.CreatedAt, StartedAt: startedAt(c), Draft: draftOf(r)}
	if g := c.Guide; g != nil && g.For == guideKey(c) {
		s.Status, s.Recommended = g.Status, g.Recommended.Label
	}
	if m.opts.Active != nil && c.EpicID != "" {
		s.Active = m.opts.Active(repo, c.ID)
	}
	for _, sec := range c.Sections {
		ev := states[sec.ID]
		if ev == "" {
			ev = api.EvidenceNone
		}
		s.Sections = append(s.Sections, api.SectionSummary{ID: sec.ID, Title: sec.Title, BeadID: sec.BeadID, Evidence: ev})
	}
	return s
}

func (m *Manager) evidenceStates(r *record, stamp string) map[string]string {
	c := r.Change
	if m.opts.Evidence == nil || len(c.Sections) == 0 {
		return nil
	}
	key := c.Revision + "|" + c.ApprovedRevision + "|" + c.Phase + "|" + c.UpdatedAt + "|" + stamp + fmt.Sprint(c.AsApproved)
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

// OpenSpecReady reports whether the repository's OpenSpec setup is valid, so
// New effort, intake and the Proposals lane apply to it.
func OpenSpecReady(repo string) bool {
	repo, e := canonicalRepo(repo)
	if e != nil {
		return false
	}
	_, e = collect(repo)
	return e == nil
}
