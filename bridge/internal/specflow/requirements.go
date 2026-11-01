package specflow

import (
	"sort"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

// MaxRequirementIndex bounds the requirement index given to round preflight.
const MaxRequirementIndex = 24 << 10

// RequirementIndex lists the requirements of the repository's accepted specs
// and active changes, capability by capability, within MaxRequirementIndex.
// Whole capabilities are left out when it would overflow (accepted specs
// first) and named in Omitted. A repository whose OpenSpec setup is missing or
// invalid has no index: the error says why.
func RequirementIndex(repo string) (*api.RequirementIndex, error) {
	repo, e := canonicalRepo(repo)
	if e != nil {
		return nil, e
	}
	inv, e := collect(repo)
	if e != nil {
		return nil, e
	}
	type capability struct {
		name     string
		accepted bool
		reqs     []api.RequirementRef
	}
	caps := map[string]*capability{}
	for _, f := range inv.Files {
		parts := strings.Split(f.Path, "/")
		var name string
		accepted := false
		switch {
		case len(parts) == 4 && parts[1] == "specs" && parts[3] == "spec.md":
			name, accepted = parts[2], true
		case len(parts) == 6 && parts[1] == "changes" && parts[3] == "specs" && parts[5] == "spec.md":
			name = parts[4] + " (change " + parts[2] + ")"
		default:
			continue
		}
		c := caps[name]
		if c == nil {
			c = &capability{name: name, accepted: accepted}
			caps[name] = c
		}
		for i, line := range strings.Split(f.Content, "\n") {
			if title, ok := strings.CutPrefix(strings.TrimSpace(line), "### Requirement:"); ok && strings.TrimSpace(title) != "" {
				c.reqs = append(c.reqs, api.RequirementRef{Capability: name, Title: strings.TrimSpace(title), Path: f.Path, Line: i + 1})
			}
		}
	}
	ordered := make([]*capability, 0, len(caps))
	for _, c := range caps {
		if len(c.reqs) > 0 {
			ordered = append(ordered, c)
		}
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].accepted != ordered[j].accepted {
			return ordered[i].accepted
		}
		return ordered[i].name < ordered[j].name
	})
	ix := &api.RequirementIndex{Requirements: []api.RequirementRef{}}
	size := 0
	for _, c := range ordered {
		n := len(c.name) + 2
		for _, r := range c.reqs {
			n += len(api.CapabilityLine(r))
		}
		if size+n > MaxRequirementIndex {
			ix.Omitted = append(ix.Omitted, c.name)
			continue
		}
		size += n
		ix.Requirements = append(ix.Requirements, c.reqs...)
	}
	return ix, nil
}
