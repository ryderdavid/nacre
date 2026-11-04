package specflow

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
var markerPattern = regexp.MustCompile(`^<!-- nacre-section: ([a-z0-9][a-z0-9-]{5,63}) -->$`)
var checkboxPattern = regexp.MustCompile(`^\s*[-*] \[[ xX]\] .+`)

type sectionDependency struct {
	SectionID string   `json:"section_id"`
	DependsOn []string `json:"depends_on"`
}

// beadRefPattern finds tokens shaped like bead IDs ("dta-7ou.1", "NAC-axe");
// only the ones the repository's board actually has count as references.
var beadRefPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9]*-[A-Za-z0-9]+(?:\.[0-9]+)*`)
var tableSeparatorPattern = regexp.MustCompile(`^\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)*\|?$`)

// tableRows returns a body's Markdown table data rows, without header and
// separator rows.
func tableRows(lines []string) []string {
	var rows []string
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if !strings.HasPrefix(t, "|") || tableSeparatorPattern.MatchString(t) {
			continue
		}
		if i+1 < len(lines) && tableSeparatorPattern.MatchString(strings.TrimSpace(lines[i+1])) {
			continue
		}
		rows = append(rows, t)
	}
	return rows
}

func citesBead(row string, known func(string) bool) bool {
	if known == nil {
		return false
	}
	for _, id := range beadRefPattern.FindAllString(row, -1) {
		if known(id) {
			return true
		}
	}
	return false
}

// sectionKind says what a level-two section of tasks.md holds. Work is a
// checkbox list or a task table and becomes one generated bead. A reference
// section is a table whose every row cites a bead that already exists, the
// convention of repositories where Beads alone tracks task state; it stays
// in tasks.md but generates nothing, since its work is already tracked.
func sectionKind(lines []string, known func(string) bool) (work, reference bool) {
	for _, l := range lines {
		if checkboxPattern.MatchString(l) {
			return true, false
		}
	}
	rows := tableRows(lines)
	if len(rows) == 0 {
		return false, false
	}
	for _, row := range rows {
		if !citesBead(row, known) {
			return true, false
		}
	}
	return false, true
}

type taskBlock struct {
	marker string
	title  string
	line   string
	body   []string
}

// prepareSections treats tasks.md as authoritative: every checkbox belongs to
// exactly one level-two section and is retained in its generated bead body.
// known reports whether a bead ID exists on the repository's board; it lets
// reference sections (tables of existing beads) stand without generating
// beads of their own.
func prepareSections(text string, previous []Section, retired []RetiredSection, deps []sectionDependency, known func(string) bool, historical ...Section) (string, []Section, error) {
	old := map[string]Section{}
	required := map[string]bool{}
	for _, s := range historical {
		old[s.ID] = s
	}
	for _, s := range previous {
		old[s.ID] = s
		required[s.ID] = true
	}
	retiredByID := map[string]string{}
	for _, r := range retired {
		if _, ok := old[r.ID]; !ok {
			return "", nil, fmt.Errorf("retirement names unknown section %s", r.ID)
		}
		if strings.TrimSpace(r.Reason) == "" {
			return "", nil, fmt.Errorf("retired section %s needs a reason", r.ID)
		}
		if _, dup := retiredByID[r.ID]; dup {
			return "", nil, fmt.Errorf("duplicate retirement %s", r.ID)
		}
		retiredByID[r.ID] = r.Reason
	}
	var preamble []string
	var blocks []*taskBlock
	pending := ""
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(line, "nacre-section:") {
			match := markerPattern.FindStringSubmatch(trimmed)
			if len(match) != 2 || pending != "" {
				return "", nil, fmt.Errorf("malformed or repeated task section marker")
			}
			pending = match[1]
			continue
		}
		if strings.HasPrefix(line, "## ") {
			title := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			if title == "" {
				return "", nil, fmt.Errorf("empty task section heading")
			}
			blocks = append(blocks, &taskBlock{marker: pending, title: title, line: line})
			pending = ""
			continue
		}
		if pending != "" {
			if trimmed == "" {
				continue
			}
			return "", nil, fmt.Errorf("task marker must immediately precede a level-two heading")
		}
		if len(blocks) == 0 {
			if checkboxPattern.MatchString(line) {
				return "", nil, fmt.Errorf("every task checkbox must belong to a level-two section")
			}
			preamble = append(preamble, line)
			continue
		}
		b := blocks[len(blocks)-1]
		b.body = append(b.body, line)
	}
	if pending != "" {
		return "", nil, fmt.Errorf("task marker is missing its heading")
	}
	output := append([]string{}, preamble...)
	var sections []Section
	seen := map[string]bool{}
	remap := map[string]string{}
	for _, b := range blocks {
		work, reference := sectionKind(b.body, known)
		if !work && !reference {
			return "", nil, fmt.Errorf("section %q has no tasks: list its work as checkboxes or table rows, or make it a table whose every row cites an existing bead", b.title)
		}
		if reference {
			if _, was := old[b.marker]; was && b.marker != "" {
				if _, gone := retiredByID[b.marker]; !gone {
					return "", nil, fmt.Errorf("section %q was tracked work (%s) and can only become a reference table if it is retired with a reason", b.title, b.marker)
				}
			}
			output = append(output, b.line)
			output = append(output, b.body...)
			continue
		}
		id := b.marker
		original := id
		if id == "" {
			id = newID()
		} else if _, known := old[id]; !known {
			id = newID()
		}
		if original != "" {
			if _, dup := remap[original]; dup {
				return "", nil, fmt.Errorf("duplicate task section marker %s", original)
			}
			remap[original] = id
		}
		if seen[id] {
			return "", nil, fmt.Errorf("duplicate section %s", id)
		}
		seen[id] = true
		if _, retired := retiredByID[id]; retired {
			return "", nil, fmt.Errorf("section %s cannot be both active and retired", id)
		}
		s := Section{ID: id, Title: b.title, DependsOn: []string{}, Body: strings.TrimSpace(strings.Join(b.body, "\n"))}
		if prior, ok := old[id]; ok {
			s.BeadID = prior.BeadID
		}
		sections = append(sections, s)
		output = append(output, "<!-- nacre-section: "+id+" -->", b.line)
		output = append(output, b.body...)
	}
	if len(sections) == 0 {
		return "", nil, fmt.Errorf("tasks.md needs at least one section of new work (checkboxes or a task table); reference tables of existing beads generate no work")
	}
	if len(sections) > 40 {
		return "", nil, fmt.Errorf("tasks.md needs between one and forty sections")
	}
	for id := range required {
		if !seen[id] {
			if _, ok := retiredByID[id]; !ok {
				return "", nil, fmt.Errorf("section %s disappeared; preserve its marker or give an explicit retirement reason", id)
			}
		}
	}
	depSeen := map[string]bool{}
	for _, d := range deps {
		id := d.SectionID
		if n, ok := remap[id]; ok {
			id = n
		}
		if !seen[id] {
			return "", nil, fmt.Errorf("dependency names unknown section %s", id)
		}
		if depSeen[id] {
			return "", nil, fmt.Errorf("repeated dependency declaration for %s", id)
		}
		depSeen[id] = true
		var targets []string
		targetSeen := map[string]bool{}
		for _, target := range d.DependsOn {
			if n, ok := remap[target]; ok {
				target = n
			}
			if !seen[target] || target == id {
				return "", nil, fmt.Errorf("invalid dependency from %s to %s", id, target)
			}
			if !targetSeen[target] {
				targets = append(targets, target)
				targetSeen[target] = true
			}
		}
		for i := range sections {
			if sections[i].ID == id {
				sections[i].DependsOn = targets
			}
		}
	}
	// A missing declaration preserves known dependencies only while both sections remain.
	for i := range sections {
		s := &sections[i]
		if depSeen[s.ID] {
			continue
		}
		if p, ok := old[s.ID]; ok {
			for _, d := range p.DependsOn {
				if seen[d] {
					s.DependsOn = append(s.DependsOn, d)
				}
			}
		}
	}
	byID := map[string]Section{}
	for _, s := range sections {
		byID[s.ID] = s
	}
	visiting, done := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("section dependencies contain a cycle")
		}
		if done[id] {
			return nil
		}
		visiting[id] = true
		for _, d := range byID[id].DependsOn {
			if err := visit(d); err != nil {
				return err
			}
		}
		visiting[id] = false
		done[id] = true
		return nil
	}
	for id := range byID {
		if e := visit(id); e != nil {
			return "", nil, e
		}
	}
	return strings.TrimSpace(strings.Join(output, "\n")) + "\n", sections, nil
}

// boardLookup reports whether a bead ID exists in the repository, listing the
// board only the first time it is asked. err holds a listing failure, which
// callers report instead of a misleading "no tasks".
func (m *Manager) boardLookup(ctx context.Context, repo string) (known func(string) bool, err func() error) {
	var ids map[string]bool
	var listErr error
	known = func(id string) bool {
		if ids == nil && listErr == nil {
			ids = map[string]bool{}
			all, e := m.opts.Beads.List(ctx, repo)
			if e != nil {
				listErr = fmt.Errorf("cannot check bead references in tasks.md: %w", e)
			}
			for _, raw := range all {
				if is, e := parseIssue(raw); e == nil && is.ID != "" {
					ids[is.ID] = true
				}
			}
		}
		return ids[id]
	}
	return known, func() error { return listErr }
}
