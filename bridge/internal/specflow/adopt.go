package specflow

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Adoption records that a change was written outside Nacre (by hand or in a
// CLI session) and taken over as it stood, instead of developed by the author.
type Adoption struct {
	At string `json:"at"`
	// RecordedReview means the owner let the change's own review.md stand in
	// for a new independent review. Cleared once Nacre revises the change.
	RecordedReview bool `json:"recorded_review,omitempty"`
	// Completed are sections whose every task was already checked.
	Completed []string `json:"completed,omitempty"`
	// Linked are sections mapped to beads that already existed under the seed.
	Linked []string `json:"linked,omitempty"`
}

// Adoptable is an active change on disk that no workflow tracks yet.
type Adoptable struct {
	ChangeName string `json:"change_name"`
	Why        string `json:"why"`
	TasksDone  int    `json:"tasks_done"`
	TasksTotal int    `json:"tasks_total"`
	HasReview  bool   `json:"has_review"`
	// Suggested: this bead looks like the change's own. It parents beads
	// named after the change, or names the change itself.
	Suggested bool `json:"suggested"`
	// Problem says why the change can't be adopted as it stands.
	Problem string `json:"problem,omitempty"`
}

const adoptedRoute = "adopted"

// CompletedBeforeAdoption reports whether a section was finished before Nacre
// took the change over and its tasks are all still checked.
func (c Change) CompletedBeforeAdoption(s Section) bool {
	return c.Adoption != nil && contains(c.Adoption.Completed, s.ID) && allChecked(s.Body)
}

func taskCounts(body string) (done, total int) {
	for _, l := range strings.Split(body, "\n") {
		if checkboxPattern.MatchString(l) {
			total++
			if t := strings.TrimSpace(l); strings.HasPrefix(t[2:], "[x]") || strings.HasPrefix(t[2:], "[X]") {
				done++
			}
		}
	}
	return done, total
}

func allChecked(body string) bool {
	done, total := taskCounts(body)
	return total > 0 && done == total
}

// changeFiles returns a change's canonical artifacts from the inventory.
func changeFiles(inv inventory, name string) []Artifact {
	prefix := "openspec/changes/" + name + "/"
	var out []Artifact
	for _, a := range inv.Files {
		if !strings.HasPrefix(a.Path, prefix) {
			continue
		}
		rel := strings.TrimPrefix(a.Path, prefix)
		parts := strings.Split(rel, "/")
		if rel == "proposal.md" || rel == "design.md" || rel == "tasks.md" || (len(parts) == 3 && parts[0] == "specs" && slugPattern.MatchString(parts[1]) && parts[2] == "spec.md") {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func activeNames(inv inventory) []string {
	seen := map[string]bool{}
	var names []string
	for p := range inv.Digests {
		parts := strings.Split(p, "/")
		if len(parts) >= 4 && parts[0] == "openspec" && parts[1] == "changes" && parts[2] != "archive" && !seen[parts[2]] {
			seen[parts[2]] = true
			names = append(names, parts[2])
		}
	}
	sort.Strings(names)
	return names
}

func normTitle(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// namesChange reports whether a bead title is the change's: "<change> <rest>".
func namesChange(title, name string) (string, bool) {
	t := normTitle(title)
	if !strings.HasPrefix(t, name) {
		return "", false
	}
	return strings.TrimSpace(strings.TrimLeft(t[len(name):], ":-–— ")), true
}

// adoptable lists the repository's untracked active changes for a bead
// without a workflow, suggesting the ones that look like the bead's own.
// Callers hold no lock.
func (m *Manager) adoptable(ctx context.Context, repo string, inv inventory, seed issue) []Adoptable {
	m.mu.Lock()
	tracked := map[string]bool{}
	for _, r := range m.records {
		if r.Repo == repo && r.Change.ChangeName != "" {
			tracked[r.Change.ChangeName] = true
		}
	}
	m.mu.Unlock()
	var children []issue
	if all, e := m.opts.Beads.List(ctx, repo); e == nil {
		for _, raw := range all {
			if is, e := parseIssue(raw); e == nil && issueParent(is) == seed.ID {
				children = append(children, is)
			}
		}
	}
	out := []Adoptable{}
	for _, name := range activeNames(inv) {
		if tracked[name] {
			continue
		}
		files := changeFiles(inv, name)
		a := Adoptable{ChangeName: name}
		for _, f := range files {
			switch strings.TrimPrefix(f.Path, "openspec/changes/"+name+"/") {
			case "proposal.md":
				a.Why = oneParagraph(whyText(f.Content), 400)
			case "tasks.md":
				a.TasksDone, a.TasksTotal = taskCounts(f.Content)
			}
		}
		_, a.HasReview = inv.Digests["openspec/changes/"+name+"/review.md"]
		if e := validateArtifactPaths(name, files); e != nil {
			a.Problem = e.Error()
		}
		a.Suggested = strings.Contains(normTitle(seed.Title+" "+seed.Description), name)
		for _, c := range children {
			if _, ok := namesChange(c.Title, name); ok {
				a.Suggested = true
			}
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Suggested && !out[j].Suggested })
	return out
}

func oneParagraph(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

// startAdoption builds the record for adopting an existing change. The caller
// holds m.mu and has checked that the bead has no workflow yet.
func (m *Manager) startAdoption(ctx context.Context, repo, id string, a Action) (*record, error) {
	name := strings.TrimSpace(a.ChangeName)
	if !slugPattern.MatchString(name) || name == "archive" {
		return nil, fmt.Errorf("%w: choose an active change to adopt", ErrConflict)
	}
	for _, other := range m.records {
		if other.Repo == repo && other.Change.ChangeName == name {
			return nil, fmt.Errorf("%w: %s is already tracked by Nacre through %s", ErrConflict, name, other.Change.SeedID)
		}
	}
	if !a.RecordedReview {
		if reason := m.available(); reason != "" {
			return nil, fmt.Errorf("%w: %s", ErrUnavailable, reason)
		}
	}
	raw, e := m.opts.Beads.Get(ctx, repo, id)
	if e != nil {
		return nil, e
	}
	seed, e := parseIssue(raw)
	if e != nil {
		return nil, e
	}
	if hasWorkflowLabel(seed) {
		return nil, fmt.Errorf("%w: specification labels have no saved workflow; restore the workflow record", ErrConflict)
	}
	inv, e := collect(repo)
	if e != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, e)
	}
	if e := validateArtifactPaths(name, changeFiles(inv, name)); e != nil {
		return nil, fmt.Errorf("%w: %s can't be adopted as it stands: %v", ErrConflict, name, e)
	}
	if _, ok := inv.Digests["openspec/changes/"+name+"/review.md"]; a.RecordedReview && !ok {
		return nil, fmt.Errorf("%w: %s has no review.md to use; adopt it with a new independent review", ErrConflict, name)
	}
	r := &record{Repo: repo, Seed: seed, Inventory: inv, Baseline: clone(inv.Digests), Stage: "adopt", Generated: map[string]generated{}, Planned: map[string]generated{},
		Change: Change{ID: newID(), SeedID: id, ChangeName: name, Route: adoptedRoute, Phase: "drafting", References: []string{}, Artifacts: []Artifact{}, Findings: []Finding{}, Sections: []Section{}, RetiredSections: []RetiredSection{}, History: []History{}, Evidence: []Evidence{},
			Adoption: &Adoption{At: stamp(), RecordedReview: a.RecordedReview}}}
	how := "with one independent review"
	if a.RecordedReview {
		how = "with its recorded review.md standing in for a new review"
	}
	history(r, "adopt", "Adopting "+name+", written outside Nacre, as it stands "+how+".")
	return r, nil
}

// applyAdoption takes the change's files as written: tasks.md gets durable
// section markers (existing ones are kept) and sections are linked to beads
// that already exist under the seed. Nothing else in the change is rewritten.
func (m *Manager) applyAdoption(ctx context.Context, r *record) error {
	name := r.Change.ChangeName
	inv, e := collect(r.Repo)
	if e != nil {
		return e
	}
	artifacts := changeFiles(inv, name)
	if e := validateArtifactPaths(name, artifacts); e != nil {
		return fmt.Errorf("%s can't be adopted as it stands: %v", name, e)
	}
	var sections []Section
	for i := range artifacts {
		if !strings.HasSuffix(artifacts[i].Path, "/tasks.md") {
			continue
		}
		var marked []Section
		for _, line := range strings.Split(artifacts[i].Content, "\n") {
			if match := markerPattern.FindStringSubmatch(strings.TrimSpace(line)); len(match) == 2 {
				marked = append(marked, Section{ID: match[1]})
			}
		}
		body, parsed, e := prepareSections(artifacts[i].Content, nil, nil, nil, marked...)
		if e != nil {
			return fmt.Errorf("tasks.md: %v", e)
		}
		artifacts[i].Content, sections = body, parsed
	}
	adoption := r.Change.Adoption
	adoption.Completed, adoption.Linked = nil, nil
	for _, s := range sections {
		if allChecked(s.Body) {
			adoption.Completed = append(adoption.Completed, s.ID)
		}
	}
	r.Generated = map[string]generated{}
	if e := m.linkSections(ctx, r, sections); e != nil {
		return e
	}
	if e := writeArtifacts(r.Repo, artifacts, inv.Digests); e != nil {
		return e
	}
	if inv, e = collect(r.Repo); e != nil {
		return e
	}
	refs := []string{}
	for _, a := range artifacts {
		refs = append(refs, a.Path)
	}
	why := ""
	for _, a := range artifacts {
		if strings.HasSuffix(a.Path, "/proposal.md") {
			why = oneParagraph(whyText(a.Content), 600)
		}
	}
	r.Inventory, r.Baseline = inv, clone(inv.Digests)
	r.Change.Artifacts, r.Change.Sections, r.Change.References = artifacts, sections, refs
	r.Change.Summary = why
	if r.Change.Summary == "" {
		r.Change.Summary = "Adopted " + name + " as written."
	}
	r.Change.Rationale = fmt.Sprintf("%s was written outside Nacre and adopted as it stands: its files are reviewed as written, not redrafted. %d of %d task sections were already complete; %d were linked to beads that already existed.",
		name, len(adoption.Completed), len(sections), len(adoption.Linked))
	r.Change.Revision = intentRevision(inv, r.Change.Artifacts, r.Seed, r.ExtraSeeds)
	r.Stage = "validate"
	r.Change.Phase = "reviewing"
	history(r, "recommendation", adoptedRoute+": "+r.Change.Rationale)
	return m.checkpoint(r)
}

// linkSections maps sections to existing beads under the seed whose titles
// are the section's ("1. Retain drafts" or "<change> 1. Retain drafts"), so
// adopting doesn't duplicate work that is already tracked or done.
func (m *Manager) linkSections(ctx context.Context, r *record, sections []Section) error {
	all, e := m.opts.Beads.List(ctx, r.Repo)
	if e != nil {
		return e
	}
	var children []issue
	for _, raw := range all {
		is, e := parseIssue(raw)
		if e != nil {
			return e
		}
		if issueParent(is) == r.Change.SeedID && !hasWorkflowLabel(is) {
			children = append(children, is)
		}
	}
	sort.Slice(children, func(i, j int) bool { return children[i].ID < children[j].ID })
	used := map[string]bool{}
	for i := range sections {
		s := &sections[i]
		want := normTitle(s.Title)
		for _, c := range children {
			rest, named := namesChange(c.Title, r.Change.ChangeName)
			if used[c.ID] || (normTitle(c.Title) != want && !(named && rest == want)) {
				continue
			}
			used[c.ID] = true
			s.BeadID = c.ID
			// The bead's own text is "previously generated", so promotion
			// replaces it with the section contract after preserving it.
			r.Generated[s.ID] = generated{Title: c.Title, Description: c.Description, Design: c.Design, Acceptance: c.Acceptance}
			r.Change.Adoption.Linked = append(r.Change.Adoption.Linked, s.ID)
			break
		}
	}
	return nil
}

// preserveLinked records a linked bead's own text on it before promotion
// replaces those fields with the section contract.
func (m *Manager) preserveLinked(ctx context.Context, r *record, is issue, s Section) error {
	marker := "[nacre-spec-adopted:" + r.Change.ID + ":" + s.ID + "]"
	comments, e := m.opts.Beads.Comments(ctx, r.Repo, is.ID)
	if e != nil {
		return e
	}
	for _, c := range comments {
		if strings.Contains(c.Text, marker) {
			return nil
		}
	}
	var b strings.Builder
	b.WriteString(marker + "\n\nAdopted into " + r.Change.ChangeName + " as section " + s.ID + ". Its title, description, design and acceptance now follow the section contract; what it said before:\n\nTitle: " + is.Title)
	for _, f := range []struct{ name, text string }{{"Description", is.Description}, {"Design", is.Design}, {"Acceptance", is.Acceptance}} {
		if strings.TrimSpace(f.text) != "" {
			b.WriteString("\n\n" + f.name + "\n" + f.text)
		}
	}
	return m.opts.Beads.AddComment(ctx, r.Repo, is.ID, b.String())
}

// closeCompletedAdopted closes the beads promotion created for sections that
// were already finished when the change was adopted.
func (m *Manager) closeCompletedAdopted(ctx context.Context, r *record) error {
	for _, s := range r.Change.Sections {
		if !r.Change.CompletedBeforeAdoption(s) || contains(r.Change.Adoption.Linked, s.ID) {
			continue
		}
		is, e := m.getIssue(ctx, r.Repo, s.BeadID)
		if e != nil {
			return e
		}
		if is.Status == "closed" {
			continue
		}
		if e = m.opts.Beads.Close(ctx, r.Repo, s.BeadID, "Every task in this section was already checked when "+r.Change.ChangeName+" was adopted."); e != nil {
			return fmt.Errorf("close finished section %s: %w", s.BeadID, e)
		}
	}
	return nil
}
