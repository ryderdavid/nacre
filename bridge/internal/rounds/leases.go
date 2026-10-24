package rounds

import (
	"sort"
	"strings"
	"sync"

	"github.com/ryderdavid/nacre/bridge/internal/repoconfig"
)

// leases hands out exclusive resources (nacre.yaml "resources") across every
// round on this computer: one holder per resource at a time. A bead holds
// its resources from its first turn until it is verified or parked, since
// its verification may need them too; a bead that stalls is resumed once and
// then parked, so no hung run holds one for long.
type leases struct {
	mu     sync.Mutex
	holder map[string]string   // resource → holder ("round/bead")
	held   map[string][]string // holder → resources
}

func newLeases() *leases {
	return &leases{holder: map[string]string{}, held: map[string][]string{}}
}

// acquire takes every resource in needs for holder, or none of them. It
// returns the environment that names them for the run.
func (l *leases) acquire(holder string, needs []string, cfg *repoconfig.Config) ([]string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, res := range needs {
		if h, taken := l.holder[res]; taken && h != holder {
			return nil, false
		}
	}
	for _, res := range needs {
		if l.holder[res] != holder {
			l.holder[res] = holder
			l.held[holder] = append(l.held[holder], res)
		}
	}
	return l.envLocked(holder, cfg), true
}

// release frees whatever holder has.
func (l *leases) release(holder string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, res := range l.held[holder] {
		if l.holder[res] == holder {
			delete(l.holder, res)
		}
	}
	delete(l.held, holder)
}

// env is NACRE_RESOURCE_<NAME>=<id> for each resource holder has.
func (l *leases) env(holder string, cfg *repoconfig.Config) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.envLocked(holder, cfg)
}

func (l *leases) envLocked(holder string, cfg *repoconfig.Config) []string {
	var env []string
	for _, res := range l.held[holder] {
		id := res
		if cfg != nil {
			for _, r := range cfg.Resources {
				if r.Name == res && r.ID != "" {
					id = r.ID
				}
			}
		}
		env = append(env, ResourceEnv(res)+"="+id)
	}
	sort.Strings(env)
	return env
}

// ResourceEnv is the environment variable naming a resource for a run.
func ResourceEnv(name string) string {
	return "NACRE_RESOURCE_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

// holders reports who holds what, for tests and the API.
func (l *leases) holders() map[string]string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := map[string]string{}
	for k, v := range l.holder {
		out[k] = v
	}
	return out
}
