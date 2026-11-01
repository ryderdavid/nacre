package attention

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
)

const amendLabel = "found-amend"

func foundPath(ref repos.Ref, change string) string {
	return "/v1/repos/" + ref.ID + "/changes/" + change + "/found"
}

// foundBeadItems lists found work that would change what a change's "done"
// means, offering to revise the change with all of it, stating the cost, or
// to make it a follow-on change. Beads a revision under way carries are left
// out until that revision is approved or replaced.
func foundBeadItems(ref repos.Ref, changes []api.ChangeSummary, beads map[string]bead) []api.AttentionItem {
	var out []api.AttentionItem
	for _, c := range changes {
		if c.EpicID == "" {
			continue
		}
		pending := map[string]bool{}
		for _, id := range c.PendingFound {
			pending[id] = true
		}
		var waiting []bead
		for _, b := range beads {
			if b.Status != "closed" && has(b.Labels, amendLabel) && has(b.Labels, "found:"+c.ID) && !pending[b.ID] {
				waiting = append(waiting, b)
			}
		}
		if len(waiting) == 0 {
			continue
		}
		sort.Slice(waiting, func(i, j int) bool { return waiting[i].ID < waiting[j].ID })
		ids := make([]any, 0, len(waiting))
		for _, b := range waiting {
			ids = append(ids, b.ID)
		}
		landed := 0
		for _, s := range c.Sections {
			if s.Evidence == api.EvidenceLanded {
				landed++
			}
		}
		cost := "Revising " + c.ChangeName + " needs a new review and your approval (up to $5)."
		if landed > 0 {
			cost = fmt.Sprintf("Revising %s reopens %d landed section%s; they will need new rounds. Or make it a follow-on change.", c.ChangeName, landed, plural(landed))
		}
		revise := action("Revise now", "POST", foundPath(ref, c.ID), map[string]any{"answer": "revise", "found_beads": ids})
		if c.Active != "" {
			revise.Disabled = c.Active
		}
		for _, b := range waiting {
			note := firstParagraph(b.Description)
			out = append(out, api.AttentionItem{
				ID: digest(api.AttentionFoundWork, ref.ID, b.ID, b.Status), Kind: api.AttentionFoundWork,
				Repo: ref.ID, RepoName: ref.Name, BeadID: b.ID, ChangeID: c.ID,
				Title: "Found work changes " + c.ChangeName + ": " + b.Title,
				Why:   strings.TrimSpace(note + " " + cost), CreatedAt: b.UpdatedAt,
				Options: []api.AttentionOption{revise,
					{Type: "open", Label: "Make it a follow-on change", Target: &api.AttentionTarget{Kind: "new_effort", Repo: ref.ID, Note: note, Related: []string{c.EpicID}, FollowOnOf: b.ID}},
					open("Open bead", "bead", ref.ID, b.ID)},
			})
		}
	}
	return out
}

// foundCommentItems lists agents' `Found:` comments on spec section beads
// that have finished in a round, until each is answered or dismissed.
func (b *Builder) foundCommentItems(ctx context.Context, ref repos.Ref, changes []api.ChangeSummary) []api.AttentionItem {
	if b.Comments == nil || b.Rounds == nil {
		return nil
	}
	known := map[string]api.ChangeSummary{}
	for _, c := range changes {
		known[c.ID] = c
	}
	done := map[string]string{} // section bead → change id
	for _, r := range b.Rounds() {
		if r.RepoID != ref.ID {
			continue
		}
		for _, rb := range r.Beads {
			if rb.SpecChangeID == "" {
				continue
			}
			ended := rb.State == api.BeadIntegrated || rb.State == api.BeadParked || rb.State == api.BeadSkipped || !r.Active()
			if _, ok := known[rb.SpecChangeID]; ok && ended {
				done[rb.ID] = rb.SpecChangeID
			}
		}
	}
	var out []api.AttentionItem
	for beadID, changeID := range done {
		comments, err := b.Comments(ctx, ref.Path, beadID)
		if err != nil {
			continue
		}
		for _, c := range comments {
			text, ok := strings.CutPrefix(strings.TrimSpace(c.Text), "Found:")
			if !ok || answered(comments, c.ID) {
				continue
			}
			note := strings.TrimSpace(text)
			change := known[changeID]
			out = append(out, api.AttentionItem{
				ID: digest(api.AttentionFoundWork, ref.ID, c.ID), Kind: api.AttentionFoundWork,
				Repo: ref.ID, RepoName: ref.Name, BeadID: beadID, ChangeID: changeID,
				Title:     "Found while working " + beadID + ": " + firstLine(note, 70),
				Why:       note + " The agent working " + beadID + " (" + change.ChangeName + ") reported it instead of doing it.",
				CreatedAt: c.CreatedAt,
				Options: []api.AttentionOption{
					{Type: "open", Label: "Answer", Target: &api.AttentionTarget{Kind: "found_something", Repo: ref.ID, Change: changeID, FoundIn: beadID, CommentID: c.ID, Note: note}},
					action("Dismiss", "POST", foundPath(ref, changeID), map[string]any{"answer": "dismiss", "found_in": beadID, "comment_id": c.ID}),
					open("Open bead", "bead", ref.ID, beadID),
				},
			})
		}
	}
	return out
}

func answered(comments []api.Comment, id string) bool {
	marker := api.FoundMarker(id)
	for _, c := range comments {
		if strings.Contains(c.Text, marker) {
			return true
		}
	}
	return false
}

func firstParagraph(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "\n\n"); i >= 0 {
		s = s[:i]
	}
	return s
}

func firstLine(s string, n int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > n {
		s = strings.TrimSpace(s[:n]) + "…"
	}
	return s
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
