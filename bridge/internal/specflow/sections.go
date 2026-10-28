package specflow

import (
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

// prepareSections treats tasks.md as authoritative: every checkbox belongs to
// exactly one level-two section and is retained in its generated bead body.
func prepareSections(text string, previous []Section, retired []RetiredSection, deps []sectionDependency, historical ...Section) (string, []Section, error) {
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
	var output []string
	var sections []Section
	seen := map[string]bool{}
	remap := map[string]string{}
	pending := ""
	index := -1
	flush := func() {
		if index >= 0 {
			sections[index].Body = strings.TrimSpace(sections[index].Body)
		}
	}
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
			flush()
			title := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			if title == "" {
				return "", nil, fmt.Errorf("empty task section heading")
			}
			id := pending
			pending = ""
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
			s := Section{ID: id, Title: title, DependsOn: []string{}}
			if prior, ok := old[id]; ok {
				s.BeadID = prior.BeadID
			}
			sections = append(sections, s)
			index = len(sections) - 1
			output = append(output, "<!-- nacre-section: "+id+" -->", line)
			continue
		}
		if pending != "" {
			if trimmed == "" {
				continue
			}
			return "", nil, fmt.Errorf("task marker must immediately precede a level-two heading")
		}
		if checkboxPattern.MatchString(line) && index < 0 {
			return "", nil, fmt.Errorf("every task checkbox must belong to a level-two section")
		}
		if index >= 0 {
			sections[index].Body += line + "\n"
		}
		output = append(output, line)
	}
	if pending != "" {
		return "", nil, fmt.Errorf("task marker is missing its heading")
	}
	flush()
	if len(sections) == 0 || len(sections) > 40 {
		return "", nil, fmt.Errorf("tasks.md needs between one and forty sections")
	}
	for _, s := range sections {
		checks := 0
		for _, l := range strings.Split(s.Body, "\n") {
			if checkboxPattern.MatchString(l) {
				checks++
			}
		}
		if checks == 0 {
			return "", nil, fmt.Errorf("section %q needs at least one task checkbox", s.Title)
		}
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
