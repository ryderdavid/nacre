package attention

import (
	"fmt"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

// MaxContext bounds the Needs you list given to the Mac assistant.
const MaxContext = 8 << 10

// Context renders the list for the board assistant's prompt, so it can answer
// "what needs me?" and "what next?" from it without a read tool.
func Context(items []api.AttentionItem) string {
	var s strings.Builder
	s.WriteString("\n\n## Needs you (waiting on Ryder now, on this computer)\n")
	if len(items) == 0 {
		s.WriteString("Nothing is waiting on Ryder.\n")
		return s.String()
	}
	s.WriteString("Start from this list when Ryder asks what to work on next or what needs him. Each line: kind, repository, ids, title, why.\n")
	for i, it := range items {
		ids := strings.Join(nonEmpty(it.BeadID, it.ChangeID, it.RoundID), " ")
		line := fmt.Sprintf("- %s [%s %s] %s: %s\n", it.Kind, it.RepoName, ids, it.Title, it.Why)
		if s.Len()+len(line) > MaxContext {
			fmt.Fprintf(&s, "…and %d more.\n", len(items)-i)
			break
		}
		s.WriteString(line)
	}
	return s.String()
}

func nonEmpty(v ...string) []string {
	var out []string
	for _, s := range v {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
