package specflow

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/bd"
)

func ptr(s string) *string           { return &s }
func workflowLabel(r *record) string { return "specflow:" + r.Change.ID }
func sectionLabel(s Section) string  { return "spec-section:" + s.ID }
func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}
func fieldsEqual(is issue, g generated) bool {
	return is.Title == g.Title && is.Description == g.Description && is.Design == g.Design && is.Acceptance == g.Acceptance
}
func issueParent(is issue) string {
	if is.Parent != "" {
		return is.Parent
	}
	for _, d := range is.Dependencies {
		if d.Type == "parent-child" {
			return d.DependsOnID
		}
	}
	return ""
}
func hasDep(is issue, id string) bool {
	for _, d := range is.Dependencies {
		if d.DependsOnID == id && (d.Type == "blocks" || d.Type == "") {
			return true
		}
	}
	return false
}
func generatedFields(r *record, s Section) generated {
	refs := strings.Join(r.Change.References, "\n- ")
	paths := make([]string, 0, len(r.Change.Artifacts))
	for _, a := range r.Change.Artifacts {
		paths = append(paths, a.Path)
	}
	body := s.Body + "\n\nSpecification contract\nChange: " + r.Change.ChangeName + "\nRevision: " + r.Change.Revision + "\nSection: " + s.ID + "\n\nApproved decision\n" + r.Change.Summary + "\n\n" + r.Change.Rationale + "\n\nArtifacts\n- " + strings.Join(paths, "\n- ")
	if refs != "" {
		body += "\n\nRequirement references\n- " + refs
	}
	return generated{Title: s.Title, Description: body, Design: "Implement section " + s.ID + " of " + r.Change.ChangeName + " at reviewed revision " + r.Change.Revision + ".\n\n" + r.Change.Rationale, Acceptance: "Complete every task in this section against the approved behavior scenarios. Run the gate commands specified in the section and project configuration. Report evidence, failed checks and remaining human verification; checked boxes or process success alone do not establish acceptance."}
}
func (m *Manager) getIssue(ctx context.Context, repo, id string) (issue, error) {
	raw, e := m.opts.Beads.Get(ctx, repo, id)
	if e != nil {
		return issue{}, e
	}
	return parseIssue(raw)
}
func (m *Manager) recoverSection(ctx context.Context, r *record, s Section) (string, error) {
	all, e := m.opts.Beads.List(ctx, r.Repo)
	if e != nil {
		return "", e
	}
	found := ""
	for _, raw := range all {
		is, e := parseIssue(raw)
		if e != nil {
			return "", e
		}
		if contains(is.Labels, workflowLabel(r)) && contains(is.Labels, sectionLabel(s)) {
			if found != "" {
				return "", fmt.Errorf("multiple beads have section identity %s; resolve the duplicate labels before resuming", s.ID)
			}
			if issueParent(is) != r.Change.EpicID {
				return "", fmt.Errorf("recovered section %s has a different parent; preserve and reconcile its manual changes", is.ID)
			}
			found = is.ID
		}
	}
	return found, nil
}
func (m *Manager) promote(ctx context.Context, r *record) error {
	if r.PromotionRevision == "" || r.PromotionRevision != r.Change.Revision || blockers(r.Change.Findings) {
		return fmt.Errorf("the current reviewed revision must be approved before promotion")
	}
	if e := m.current(r); e != nil {
		return e
	}
	if r.Change.EpicID == "" {
		r.Change.EpicID = r.Change.SeedID
	}
	if r.Planned == nil {
		r.Planned = map[string]generated{}
	}
	for _, s := range r.Change.Sections {
		r.Planned[s.ID] = generatedFields(r, s)
	}
	if e := m.checkpoint(r); e != nil {
		return e
	}
	epic, e := m.getIssue(ctx, r.Repo, r.Change.EpicID)
	if e != nil {
		return e
	}
	// Type promotion preserves all original intent fields, status, and comments.
	if epic.Type != "epic" || !contains(epic.Labels, workflowLabel(r)) {
		if e = m.opts.Beads.Update(ctx, r.Repo, epic.ID, bd.UpdateParams{IssueType: ptr("epic"), AddLabels: []string{workflowLabel(r)}}); e != nil {
			return e
		}
	}
	for _, alias := range r.Aliases {
		is, e := m.getIssue(ctx, r.Repo, alias)
		if e != nil {
			return e
		}
		if !contains(is.Labels, workflowLabel(r)) {
			if e = m.opts.Beads.Update(ctx, r.Repo, alias, bd.UpdateParams{AddLabels: []string{workflowLabel(r)}}); e != nil {
				return e
			}
		}
	}
	for i := range r.Change.Sections {
		s := &r.Change.Sections[i]
		desired := r.Planned[s.ID]
		if s.BeadID == "" {
			id, e := m.recoverSection(ctx, r, *s)
			if e != nil {
				return e
			}
			if id == "" {
				id, e = m.opts.Beads.Create(ctx, r.Repo, bd.CreateParams{Title: desired.Title, Description: desired.Description, Design: desired.Design, AcceptanceCriteria: desired.Acceptance, IssueType: "task", Parent: r.Change.EpicID, Labels: []string{workflowLabel(r), sectionLabel(*s)}})
				if e != nil {
					return fmt.Errorf("create section %s: %w; Resume will recover its stable label if creation succeeded", s.ID, e)
				}
			}
			s.BeadID = id
			if e = m.checkpoint(r); e != nil {
				return e
			}
		}
		is, e := m.getIssue(ctx, r.Repo, s.BeadID)
		if e != nil {
			return e
		}
		if issueParent(is) != r.Change.EpicID {
			return fmt.Errorf("%s changed parent outside this workflow; reconcile it before synchronization", is.ID)
		}
		if !fieldsEqual(is, desired) {
			previous, known := r.Generated[s.ID]
			if !known || !fieldsEqual(is, previous) {
				return fmt.Errorf("%s has manually edited generated fields. Your edits have been preserved; reconcile those fields with the last generated section before resuming.", is.ID)
			}
			if e = m.opts.Beads.Update(ctx, r.Repo, is.ID, bd.UpdateParams{Title: ptr(desired.Title), Description: ptr(desired.Description), Design: ptr(desired.Design), AcceptanceCriteria: ptr(desired.Acceptance), AddLabels: []string{workflowLabel(r), sectionLabel(*s)}}); e != nil {
				return e
			}
		} else if !contains(is.Labels, workflowLabel(r)) || !contains(is.Labels, sectionLabel(*s)) {
			return fmt.Errorf("%s lost its stable specification labels; restore them before synchronizing", is.ID)
		}
		if is.Status == "closed" && r.Change.ApprovedRevision != r.PromotionRevision {
			if e = m.opts.Beads.Reopen(ctx, r.Repo, is.ID); e != nil {
				return e
			}
		}
	}
	ids := map[string]string{}
	for _, s := range r.Change.Sections {
		ids[s.ID] = s.BeadID
	}
	for _, s := range r.Change.Sections {
		desired := r.Planned[s.ID]
		for _, dep := range s.DependsOn {
			target, ok := ids[dep]
			if !ok {
				return fmt.Errorf("missing section dependency mapping %s", dep)
			}
			desired.Dependencies = append(desired.Dependencies, target)
		}
		sort.Strings(desired.Dependencies)
		is, e := m.getIssue(ctx, r.Repo, s.BeadID)
		if e != nil {
			return e
		}
		previous := r.Generated[s.ID]
		for _, dep := range previous.Dependencies {
			if !contains(desired.Dependencies, dep) && hasDep(is, dep) {
				if e = m.opts.Beads.DepRemove(ctx, r.Repo, s.BeadID, dep); e != nil {
					return e
				}
			}
		}
		for _, dep := range desired.Dependencies {
			if !hasDep(is, dep) {
				if e = m.opts.Beads.DepAdd(ctx, r.Repo, s.BeadID, dep, "blocks"); e != nil {
					return e
				}
			}
		}
		r.Generated[s.ID] = desired
		if e = m.checkpoint(r); e != nil {
			return e
		}
	}
	if e = m.current(r); e != nil {
		return e
	}
	if epic.Status == "closed" {
		if e = m.opts.Beads.Reopen(ctx, r.Repo, epic.ID); e != nil {
			return e
		}
	}
	if e = m.decisionComments(ctx, r, "promotion"); e != nil {
		return e
	}
	r.Change.ApprovedRevision = r.Change.Revision
	r.Change.Phase = "ready"
	r.Change.Error = ""
	r.Stage = "ready"
	r.PromotionRevision = ""
	r.Planned = map[string]generated{}
	history(r, "promotion", fmt.Sprintf("Prepared epic %s with %d current section beads. Start a specification round to implement; acceptance, landing and archive remain explicit.", r.Change.EpicID, len(r.Change.Sections)))
	return m.checkpoint(r)
}
func (m *Manager) decisionComments(ctx context.Context, r *record, phase string) error {
	marker := "[nacre-spec-decision:" + r.Change.ID + ":" + r.Change.Revision + ":" + phase + "]"
	var body strings.Builder
	body.WriteString(marker + "\n\nOpenSpec " + phase + "\nRevision: " + r.Change.Revision + "\nRoute: " + r.Change.Route + "\n\n" + r.Change.Summary + "\n\nDecision rationale\n" + r.Change.Rationale)
	if len(r.Change.References) > 0 {
		body.WriteString("\n\nReferences\n- " + strings.Join(r.Change.References, "\n- "))
	}
	body.WriteString("\n\nIndependent review\n")
	if len(r.Change.Findings) == 0 {
		body.WriteString("No unresolved findings.")
	}
	for _, f := range r.Change.Findings {
		fmt.Fprintf(&body, "- %s (%s, %s): %s. %s\n", f.ID, f.Severity, f.Disposition, f.Summary, f.Detail)
	}
	for _, s := range r.Change.Sections {
		fmt.Fprintf(&body, "\nSection %s: %s", s.ID, s.Title)
		if s.BeadID != "" {
			body.WriteString(" → " + s.BeadID)
		}
	}
	for _, s := range r.Change.RetiredSections {
		fmt.Fprintf(&body, "\nRetired %s (%s): %s. Existing bead was retained without automatic closure.", s.ID, s.BeadID, s.Reason)
	}
	body.WriteString("\n\nImplementation requires current revision evidence, human review and landing. This workflow does not automatically close the epic or archive the change.")
	ids := []string{r.Change.SeedID}
	if r.Change.EpicID != "" {
		ids = append(ids, r.Change.EpicID)
	}
	ids = append(ids, r.Aliases...)
	if phase == "promotion" {
		for _, s := range r.Change.Sections {
			ids = append(ids, s.BeadID)
		}
	}
	done := map[string]bool{}
	for _, id := range ids {
		if id == "" || done[id] {
			continue
		}
		done[id] = true
		comments, e := m.opts.Beads.Comments(ctx, r.Repo, id)
		if e != nil {
			return e
		}
		exists := false
		for _, c := range comments {
			if strings.Contains(c.Text, marker) {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		if e = m.opts.Beads.AddComment(ctx, r.Repo, id, body.String()); e != nil {
			return fmt.Errorf("record decision on %s: %w; Resume safely checks the decision marker before retrying", id, e)
		}
	}
	return nil
}
