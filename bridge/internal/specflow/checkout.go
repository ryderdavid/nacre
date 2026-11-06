package specflow

import (
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

// ErrNoChange: the checkout has no such untracked active change.
var ErrNoChange = errors.New("no such change in the checkout")

// untrackedSectionPrefix starts the ids of an untracked change's sections:
// they come from tasks.md's headings, in order, not from a workflow record.
const untrackedSectionPrefix = "tasks-"

// checkoutChange is one active change folder as it stands on disk.
type checkoutChange struct {
	name      string
	artifacts []Artifact
	sections  []Section
	done      int
	total     int
	proposal  bool
	review    bool
	updated   time.Time
	// archivedAs is an archived folder of the same change, when one exists.
	archivedAs string
}

// tracked are the change names a workflow record of repo still tracks. An
// archived record no longer does: its change lives in the archive, and a
// folder of that name in openspec/changes is what the checkout holds now.
func (m *Manager) tracked(repo string) map[string]bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]bool{}
	for _, r := range m.records {
		if r.Repo == repo && r.Change.ChangeName != "" && r.Change.Phase != "archived" {
			out[r.Change.ChangeName] = true
		}
	}
	return out
}

// Untracked lists the checkout's active changes (folders under
// openspec/changes other than archive) that no workflow record tracks, so the
// Spec panel lists every change the checkout holds, not only Nacre's own.
// Each gets a phase from its files:
//
//   - drafting: no proposal.md, or no task checkboxes yet
//   - awaiting_approval: written, with no task checked
//   - ready: some tasks checked; a section whose every task is checked is
//     "landed", so a change with every task checked reads as ready to archive
//
// They carry Untracked, never seeds or an epic, and are not Needs you items.
func (m *Manager) Untracked(repo string) ([]api.ChangeSummary, error) {
	repo, e := canonicalRepo(repo)
	if e != nil {
		return nil, e
	}
	list, e := readCheckout(repo, m.tracked(repo))
	if e != nil {
		return nil, e
	}
	out := make([]api.ChangeSummary, 0, len(list))
	for _, c := range list {
		out = append(out, c.summary())
	}
	return out, nil
}

// UntrackedChange reads one untracked active change in full, for the Spec
// panel's overview and reader. It is never runnable: adopting it is asked for
// in chat.
func (m *Manager) UntrackedChange(repo, name string) (Envelope, error) {
	repo, e := canonicalRepo(repo)
	if e != nil {
		return Envelope{}, e
	}
	if !slugPattern.MatchString(name) || name == "archive" {
		return Envelope{}, fmt.Errorf("%w: %q", ErrNoChange, name)
	}
	if m.tracked(repo)[name] {
		return Envelope{}, fmt.Errorf("%w: %s has a workflow record; open it through its bead", ErrNoChange, name)
	}
	root, e := os.OpenRoot(repo)
	if e != nil {
		return Envelope{}, e
	}
	defer root.Close()
	c, ok, e := readCheckoutChange(root, name)
	if e != nil {
		return Envelope{}, e
	}
	if !ok {
		return Envelope{}, fmt.Errorf("%w: %s", ErrNoChange, name)
	}
	c.archivedAs = archivedAs(root, name)
	s := c.summary()
	ch := Change{ID: s.ID, ChangeName: name, Phase: s.Phase, Revision: s.Revision,
		References: []string{}, Artifacts: c.artifacts, Findings: []Finding{}, Sections: c.sections,
		RetiredSections: []RetiredSection{}, History: []History{}, Evidence: []Evidence{}, UpdatedAt: s.UpdatedAt,
		Untracked: true}
	for _, a := range c.artifacts {
		if strings.HasSuffix(a.Path, "/proposal.md") {
			ch.Summary = oneParagraph(whyText(a.Content), 400)
		}
	}
	return Envelope{Available: true, Change: &ch}, nil
}

// readCheckout reads every untracked active change folder, sorted by name.
// A folder that can't be read is left out rather than failing the list.
func readCheckout(repo string, tracked map[string]bool) ([]checkoutChange, error) {
	root, e := os.OpenRoot(repo)
	if e != nil {
		return nil, e
	}
	defer root.Close()
	if noLinks(root, "openspec/changes") != nil {
		return nil, nil
	}
	dir, e := root.Open("openspec/changes")
	if errors.Is(e, os.ErrNotExist) {
		return nil, nil
	} else if e != nil {
		return nil, e
	}
	entries, e := dir.ReadDir(-1)
	dir.Close()
	if e != nil {
		return nil, e
	}
	var out []checkoutChange
	for _, ent := range entries {
		name := ent.Name()
		if !ent.IsDir() || name == "archive" || !slugPattern.MatchString(name) || tracked[name] {
			continue
		}
		c, ok, e := readCheckoutChange(root, name)
		if e != nil || !ok {
			continue
		}
		c.archivedAs = archivedAs(root, name)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

// readCheckoutChange reads a change folder's canonical artifacts. ok is false
// when the folder holds none of them.
func readCheckoutChange(root *os.Root, name string) (checkoutChange, bool, error) {
	base := "openspec/changes/" + name
	if e := noLinks(root, base); e != nil {
		return checkoutChange{}, false, e
	}
	if st, e := root.Lstat(base); e != nil || !st.IsDir() {
		return checkoutChange{}, false, nil
	}
	c := checkoutChange{name: name}
	add := func(p string) {
		st, e := root.Lstat(p)
		if e != nil || !st.Mode().IsRegular() {
			return
		}
		s, e := readFile(root, p)
		if e != nil {
			return
		}
		c.artifacts = append(c.artifacts, Artifact{Path: p, Content: s})
		if st.ModTime().After(c.updated) {
			c.updated = st.ModTime()
		}
	}
	for _, f := range []string{"proposal.md", "design.md", "tasks.md"} {
		add(base + "/" + f)
	}
	if d, e := root.Open(base + "/specs"); e == nil {
		caps, _ := d.ReadDir(-1)
		d.Close()
		for _, sub := range caps {
			if sub.IsDir() && slugPattern.MatchString(sub.Name()) && len(c.artifacts) < maxArtifacts {
				add(base + "/specs/" + sub.Name() + "/spec.md")
			}
		}
	}
	if len(c.artifacts) == 0 {
		return c, false, nil
	}
	if st, e := root.Lstat(base + "/review.md"); e == nil && st.Mode().IsRegular() {
		c.review = true
	}
	sort.Slice(c.artifacts, func(i, j int) bool { return c.artifacts[i].Path < c.artifacts[j].Path })
	for _, a := range c.artifacts {
		switch path.Base(a.Path) {
		case "proposal.md":
			if path.Dir(a.Path) == base {
				c.proposal = true
			}
		case "tasks.md":
			c.sections = taskSections(a.Content)
			c.done, c.total = taskCounts(a.Content)
		}
	}
	return c, true, nil
}

// archivedAs names an archived folder of the same change ("2026-11-02-<name>"),
// or "" when there is none.
func archivedAs(root *os.Root, name string) string {
	d, e := root.Open("openspec/changes/archive")
	if e != nil {
		return ""
	}
	defer d.Close()
	entries, _ := d.ReadDir(-1)
	found := ""
	for _, ent := range entries {
		// Archived folders are "<YYYY-MM-DD>-<name>".
		if n := ent.Name(); ent.IsDir() && len(n) == len(name)+11 && strings.HasSuffix(n, "-"+name) && n > found {
			found = n
		}
	}
	return found
}

// taskSections splits tasks.md into its level-two sections that hold task
// checkboxes. Ids follow the headings' order; there are no beads.
func taskSections(text string) []Section {
	var out []Section
	var cur *Section
	var body []string
	flush := func() {
		if cur == nil {
			return
		}
		cur.Body = strings.TrimSpace(strings.Join(body, "\n"))
		if _, total := taskCounts(cur.Body); total > 0 {
			cur.ID = fmt.Sprintf("%s%d", untrackedSectionPrefix, len(out)+1)
			out = append(out, *cur)
		}
		cur, body = nil, nil
	}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if t, ok := strings.CutPrefix(line, "## "); ok {
			flush()
			cur = &Section{Title: strings.TrimSpace(t), DependsOn: []string{}}
			continue
		}
		if cur != nil && !markerPattern.MatchString(strings.TrimSpace(line)) {
			body = append(body, line)
		}
	}
	flush()
	return out
}

// phase is an untracked change's phase, from its files and tasks.
func (c checkoutChange) phase() string {
	switch {
	case !c.proposal || c.total == 0:
		return "drafting"
	case c.done == 0:
		return "awaiting_approval"
	}
	return "ready"
}

func (c checkoutChange) summary() api.ChangeSummary {
	var b strings.Builder
	for _, a := range c.artifacts {
		b.WriteString(a.Path + "\x00" + a.Content + "\x00")
	}
	s := api.ChangeSummary{ID: c.name, ChangeName: c.name, Phase: c.phase(), Revision: digest(b.String()),
		SeedIDs: []string{}, Sections: []api.SectionSummary{}, Untracked: true,
		TasksDone: c.done, TasksTotal: c.total, HasReview: c.review, ArchivedAs: c.archivedAs}
	if !c.updated.IsZero() {
		s.UpdatedAt = c.updated.UTC().Format(time.RFC3339)
	}
	for _, sec := range c.sections {
		ev := api.EvidenceNone
		if s.Phase == "ready" && allChecked(sec.Body) {
			ev = api.EvidenceLanded
		}
		s.Sections = append(s.Sections, api.SectionSummary{ID: sec.ID, Title: sec.Title, Evidence: ev})
	}
	return s
}
