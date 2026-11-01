package specflow

import (
	"context"
	"sort"
	"strings"
	"time"
)

// GenericStaleReason is given when nothing more specific can be named.
const GenericStaleReason = "The project's specifications changed since this was approved."

// staleReason names what broke a change's approval: the files that changed
// since its baseline, a seed whose intent moved, or a section bead edited
// after synchronization. section is that bead when sectionCurrent failed.
func (m *Manager) staleReason(ctx context.Context, r *record, section string) string {
	if section != "" {
		return "Section bead " + section + " was edited after its beads were synchronized."
	}
	if len(r.Baseline) == 0 {
		return GenericStaleReason
	}
	inv, e := collect(r.Repo)
	if e != nil {
		return e.Error()
	}
	if causes := fileCauses(r.Change.ChangeName, r.Baseline, inv.Digests); len(causes) > 0 {
		return strings.Join(causes, "; ") + "."
	}
	fresh := clone(r)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if e := m.refreshIntents(ctx, fresh); e == nil {
		if intentOf(fresh.Seed) != intentOf(r.Seed) {
			return "Seed bead " + r.Change.SeedID + " changed."
		}
		for i, is := range fresh.ExtraSeeds {
			if i >= len(r.ExtraSeeds) || intentOf(is) != intentOf(r.ExtraSeeds[i]) {
				return "Seed bead " + is.ID + " changed."
			}
		}
	}
	return GenericStaleReason
}

// fileCauses describes the inventory files that differ from the baseline,
// at most three, grouped by what they belong to.
func fileCauses(own string, baseline, current map[string]string) []string {
	seen := map[string]bool{}
	var causes []string
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			causes = append(causes, s)
		}
	}
	var paths []string
	for p := range baseline {
		paths = append(paths, p)
	}
	for p := range current {
		if _, ok := baseline[p]; !ok {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	for _, p := range paths {
		before, had := baseline[p]
		after, has := current[p]
		if had && has && before == after {
			continue
		}
		parts := strings.Split(p, "/")
		switch {
		case p == "AGENTS.md" || p == "openspec/config.yaml":
			add(p + " changed")
		case len(parts) >= 3 && parts[1] == "specs":
			add("Accepted spec " + parts[2] + " changed")
		case len(parts) >= 3 && parts[1] == "changes" && parts[2] == own:
			if strings.HasSuffix(p, "/tasks.md") {
				add("tasks.md was edited after approval. Progress is tracked in beads; revise to change the plan")
			} else {
				add("This change's " + strings.Join(parts[3:], "/") + " was edited after review")
			}
		case len(parts) >= 3 && parts[1] == "changes" && !had:
			add("Active change " + parts[2] + " was added")
		case len(parts) >= 3 && parts[1] == "changes" && !has:
			add("Active change " + parts[2] + " was removed or archived")
		case len(parts) >= 3 && parts[1] == "changes":
			add("Active change " + parts[2] + " changed")
		default:
			add(p + " changed")
		}
	}
	if len(causes) > 3 {
		causes = append(causes[:3], "and more")
	}
	return causes
}

// Refresh re-checks every approved or awaiting change of the repository
// against its current context, marking those that went stale with a reason,
// as reading each change does.
func (m *Manager) Refresh(ctx context.Context, repo string) {
	repo, e := canonicalRepo(repo)
	if e != nil {
		return
	}
	m.mu.Lock()
	var seeds []string
	for _, r := range m.records {
		if r.Repo == repo && (r.Change.Phase == "ready" || r.Change.Phase == "awaiting_approval" || r.Change.Phase == "covered") {
			seeds = append(seeds, r.Change.SeedID)
		}
	}
	m.mu.Unlock()
	for _, id := range seeds {
		_, _ = m.Get(ctx, repo, id)
	}
}
