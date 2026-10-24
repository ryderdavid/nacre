package server

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
)

// actorSuffix is what config's default actor appends to the git user name.
const actorSuffix = " via Nacre"

// withElsewhere returns issues with active_elsewhere added to each bead that
// is claimed in bd but has no live run on this computer (api.ActiveElsewhere).
// issues is the registry's cached slice, so it is copied, never edited.
func (s *Server) withElsewhere(repoID string, issues []api.Issue) []api.Issue {
	local := localRuns{live: map[string]bool{}, ended: map[string]time.Time{}}
	if s.Runs != nil {
		// Stale adopted sessions are left out on purpose: a session left open
		// is not work in progress, so the claim in bd is the better signal.
		for _, r := range s.Runs.List(runs.Filter{RepoID: repoID, IncludeAdopted: true}) {
			if r.Active() {
				local.live[r.IssueID] = true
				continue
			}
			if r.Adopted() {
				continue
			}
			if end, err := time.Parse(time.RFC3339, r.EndedAt); err == nil && end.After(local.ended[r.IssueID]) {
				local.ended[r.IssueID] = end
			}
		}
	}
	actor := ""
	if s.Config != nil {
		actor = s.Config.Actor
	}
	out := make([]api.Issue, len(issues))
	for i, raw := range issues {
		out[i] = markElsewhere(raw, actor, local)
	}
	return out
}

// localRuns is what this computer's runs say about the beads of one repo.
type localRuns struct {
	// live holds beads with a run working on them now.
	live map[string]bool
	// ended holds, per bead, when the last run nacred dispatched for it ended.
	// The agent in a Nacre run claims its bead under the git user, not the
	// actor, so a bead the run left in progress looks claimed by the person;
	// a claim made before that run ended is the run's own, not another
	// session's.
	ended map[string]time.Time
}

// markElsewhere returns raw with active_elsewhere added when the bead is
// claimed by someone other than actor and no local run accounts for the
// claim; otherwise raw itself.
func markElsewhere(raw api.Issue, actor string, local localRuns) api.Issue {
	var b struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		Assignee  string `json:"assignee"`
		StartedAt string `json:"started_at"`
		UpdatedAt string `json:"updated_at"`
	}
	if json.Unmarshal(raw, &b) != nil {
		return raw
	}
	assignee := strings.TrimSpace(b.Assignee)
	if (b.Status != "in_progress" && b.Status != "hooked") || assignee == "" || assignee == actor || local.live[b.ID] {
		return raw
	}
	since := b.StartedAt
	if since == "" {
		since = b.UpdatedAt
	}
	if end, ok := local.ended[b.ID]; ok {
		// Claimed before (or while) a Nacre run on it ended: the run's claim.
		if claimed, err := time.Parse(time.RFC3339, since); err != nil || !claimed.After(end) {
			return raw
		}
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return raw
	}
	self := strings.HasSuffix(actor, actorSuffix) && assignee == strings.TrimSuffix(actor, actorSuffix)
	mark, err := json.Marshal(api.ActiveElsewhere{Assignee: assignee, Since: since, Self: self})
	if err != nil {
		return raw
	}
	fields["active_elsewhere"] = mark
	out, err := json.Marshal(fields)
	if err != nil {
		return raw
	}
	return out
}
