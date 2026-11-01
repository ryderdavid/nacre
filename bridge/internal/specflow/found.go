package specflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
)

// Found-work answers. See openspec/changes/surface-spec-workflow (effort-intake).
const (
	FoundHowOrder   = "how_order"
	FoundAmend      = "amend"
	FoundOutOfScope = "out_of_scope"
	FoundRevise     = "revise"
	FoundDismiss    = "dismiss"
)

// Found-work labels: every child of a spec epic carries the first; work that
// would change what "done" means also carries the second until it is revised
// into the change or made a follow-on.
const (
	FoundLabelPrefix = "found:"
	FoundAmendLabel  = "found-amend"
)

// FoundMarker is posted in a reply to a `Found:` comment once it is answered
// or dismissed, so it never raises a Needs you item again.
func FoundMarker(commentID string) string { return api.FoundMarker(commentID) }

// SpecEpicChange returns the change id of a spec epic, from its labels.
func SpecEpicChange(labels []string) string {
	for _, l := range labels {
		if id, ok := strings.CutPrefix(l, "specflow:"); ok {
			return id
		}
	}
	return ""
}

// FoundRequest answers discovered work for a change.
type FoundRequest struct {
	Answer        string   `json:"answer"`
	Note          string   `json:"note,omitempty"`
	FoundIn       string   `json:"found_in,omitempty"`
	BeforeSection string   `json:"before_section,omitempty"`
	CommentID     string   `json:"comment_id,omitempty"`
	FoundBeads    []string `json:"found_beads,omitempty"`
}

// FoundComment identifies a `Found:` comment by the bead it is on.
type FoundComment struct {
	Bead      string `json:"bead"`
	CommentID string `json:"comment_id"`
}

// NewEffortPrefill opens New effort for out-of-scope work.
type NewEffortPrefill struct {
	Title        string        `json:"title"`
	Description  string        `json:"description"`
	Related      []string      `json:"related"`
	FoundComment *FoundComment `json:"found_comment,omitempty"`
}

// FoundResult is what an answer did.
type FoundResult struct {
	BeadID  string            `json:"bead_id,omitempty"`
	Prefill *NewEffortPrefill `json:"prefill,omitempty"`
	Change  *Change           `json:"change,omitempty"`
}

// MaxFoundNote bounds one note of found work.
const MaxFoundNote = 2000

// Found carries out an answer to discovered work on a promoted change.
func (m *Manager) Found(ctx context.Context, repo, changeID string, req FoundRequest) (FoundResult, error) {
	repo, e := canonicalRepo(repo)
	if e != nil {
		return FoundResult{}, e
	}
	m.mu.Lock()
	var r *record
	if live := m.records[changeID]; live != nil && live.Repo == repo {
		r = clone(live)
	}
	m.mu.Unlock()
	if r == nil {
		return FoundResult{}, fmt.Errorf("%w: no specification change %s in this repository", ErrConflict, changeID)
	}
	if r.Change.EpicID == "" {
		return FoundResult{}, fmt.Errorf("%w: found work is answered once the change has an epic; give feedback on the proposal instead", ErrConflict)
	}
	note := strings.TrimSpace(req.Note)
	if len(note) > MaxFoundNote {
		return FoundResult{}, fmt.Errorf("%w: keep the note under %d characters", ErrConflict, MaxFoundNote)
	}
	if req.CommentID != "" {
		if req.FoundIn == "" {
			return FoundResult{}, fmt.Errorf("%w: name the bead the comment is on", ErrConflict)
		}
		text, e := m.foundComment(ctx, repo, req.FoundIn, req.CommentID)
		if e != nil {
			return FoundResult{}, e
		}
		if note == "" {
			note = text
		}
	}
	switch req.Answer {
	case FoundDismiss:
		if req.CommentID == "" {
			return FoundResult{}, fmt.Errorf("%w: only a found comment can be dismissed", ErrConflict)
		}
		return FoundResult{}, m.markFound(ctx, repo, req.FoundIn, req.CommentID, "dismissed")
	case FoundOutOfScope:
		p := &NewEffortPrefill{Title: firstLine(note, 80), Description: note, Related: []string{r.Change.EpicID}}
		if req.CommentID != "" {
			p.FoundComment = &FoundComment{Bead: req.FoundIn, CommentID: req.CommentID}
		}
		return FoundResult{Prefill: p}, nil
	case FoundHowOrder, FoundAmend:
		if note == "" {
			return FoundResult{}, fmt.Errorf("%w: say what was found", ErrConflict)
		}
		var before string
		if req.BeforeSection != "" {
			for _, s := range r.Change.Sections {
				if s.ID == req.BeforeSection {
					before = s.BeadID
				}
			}
			if before == "" {
				return FoundResult{}, fmt.Errorf("%w: %s is not a current section of this change", ErrConflict, req.BeforeSection)
			}
		}
		id, e := m.createFound(ctx, r, req, note)
		if e != nil {
			return FoundResult{}, e
		}
		if before != "" {
			if e = m.opts.Beads.DepAdd(ctx, repo, before, id, "blocks"); e != nil {
				return FoundResult{BeadID: id}, fmt.Errorf("%s was created but couldn't be made to come before %s: %w", id, before, e)
			}
		}
		if req.CommentID != "" {
			kind := "how or in what order"
			if req.Answer == FoundAmend {
				kind = "changes what done means"
			}
			if e = m.markFound(ctx, repo, req.FoundIn, req.CommentID, "answered: "+kind+" → "+id); e != nil {
				return FoundResult{BeadID: id}, e
			}
		}
		return FoundResult{BeadID: id}, nil
	case FoundRevise:
		return m.reviseFromFound(ctx, r, req.FoundBeads)
	}
	return FoundResult{}, fmt.Errorf("%w: choose how_order, amend, out_of_scope, revise or dismiss", ErrConflict)
}

// createFound files found work under the change's epic as ordinary work.
func (m *Manager) createFound(ctx context.Context, r *record, req FoundRequest, note string) (string, error) {
	labels := []string{FoundLabelPrefix + r.Change.ID}
	if req.Answer == FoundAmend {
		labels = append(labels, FoundAmendLabel)
	}
	desc := note
	if req.FoundIn != "" {
		desc += "\n\nFound in: " + req.FoundIn
		if req.CommentID != "" {
			desc += "; source comment: " + req.CommentID
		}
	}
	id, e := m.opts.Beads.Create(ctx, r.Repo, bd.CreateParams{Title: firstLine(note, 80), Description: desc, IssueType: "task",
		Parent: r.Change.EpicID, Labels: labels, NoInheritLabels: true})
	if e != nil {
		return "", e
	}
	// Found on the epic itself, the parent link already says where it came
	// from, and bd refuses a child's dependency on its own parent.
	if req.FoundIn != "" && req.FoundIn != r.Change.EpicID {
		if e = m.opts.Beads.DepAdd(ctx, r.Repo, id, req.FoundIn, "discovered-from"); e != nil {
			return id, fmt.Errorf("%s was created but its discovered-from link failed: %w", id, e)
		}
	}
	return id, nil
}

// reviseFromFound starts a revision carrying the notes of the given open
// found-amend beads; they are closed once that revision is approved.
func (m *Manager) reviseFromFound(ctx context.Context, r *record, ids []string) (FoundResult, error) {
	if len(ids) == 0 {
		return FoundResult{}, fmt.Errorf("%w: choose the found work to revise the change with", ErrConflict)
	}
	var notes strings.Builder
	notes.WriteString("Work found while implementing this change would change what \"done\" means. Revise the change to include it:\n")
	for _, id := range ids {
		is, e := m.getIssue(ctx, r.Repo, id)
		if e != nil {
			return FoundResult{}, e
		}
		if is.Status == "closed" || !contains(is.Labels, FoundAmendLabel) || !contains(is.Labels, FoundLabelPrefix+r.Change.ID) {
			return FoundResult{}, fmt.Errorf("%w: %s is not open found work that changes this change", ErrConflict, id)
		}
		fmt.Fprintf(&notes, "\n- %s: %s", id, strings.TrimSpace(is.Description))
	}
	if notes.Len() > 12000 {
		return FoundResult{}, fmt.Errorf("%w: too many notes for one revision; revise with fewer", ErrConflict)
	}
	env, e := m.Act(ctx, r.Repo, r.Change.SeedID, Action{Action: "revise", Feedback: notes.String(), FoundBeads: ids})
	if e != nil {
		return FoundResult{}, e
	}
	return FoundResult{Change: env.Change}, nil
}

// closePendingFound closes the found beads whose notes the approved revision
// carried, linking the revision. Idempotent for a resumed promotion.
func (m *Manager) closePendingFound(ctx context.Context, r *record) error {
	for _, id := range r.PendingFound {
		is, e := m.getIssue(ctx, r.Repo, id)
		if e != nil {
			return e
		}
		if is.Status == "closed" {
			continue
		}
		if e = m.opts.Beads.Close(ctx, r.Repo, id, "Revised into "+r.Change.ChangeName+" at revision "+short(r.Change.Revision)+"."); e != nil {
			return fmt.Errorf("close found work %s after its revision was approved: %w", id, e)
		}
	}
	r.PendingFound = nil
	return nil
}

// foundComment returns the text of a `Found:` comment on bead.
func (m *Manager) foundComment(ctx context.Context, repo, bead, commentID string) (string, error) {
	comments, e := m.opts.Beads.Comments(ctx, repo, bead)
	if e != nil {
		return "", e
	}
	for _, c := range comments {
		if c.ID == commentID {
			text, ok := strings.CutPrefix(strings.TrimSpace(c.Text), "Found:")
			if !ok {
				return "", fmt.Errorf("%w: that comment doesn't report found work", ErrConflict)
			}
			return strings.TrimSpace(text), nil
		}
	}
	return "", fmt.Errorf("%w: no comment %s on %s", ErrConflict, commentID, bead)
}

// markFound posts the reply that keeps an answered `Found:` comment answered.
func (m *Manager) markFound(ctx context.Context, repo, bead, commentID, what string) error {
	return m.opts.Beads.AddComment(ctx, repo, bead, FoundMarker(commentID)+" "+what)
}

// MarkFound is markFound for the create path's found_comment.
func (m *Manager) MarkFound(ctx context.Context, repo, bead, commentID, what string) error {
	repo, e := canonicalRepo(repo)
	if e != nil {
		return e
	}
	return m.markFound(ctx, repo, bead, commentID, what)
}

func firstLine(s string, n int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > n {
		s = strings.TrimSpace(s[:n]) + "…"
	}
	return s
}
