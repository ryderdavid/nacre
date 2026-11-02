package prompt

import (
	"fmt"
	"github.com/ryderdavid/nacre/bridge/internal/api"
	"strings"
)

// GoalOptions is shared by every implementation launch. Saved child choices
// remain meaningful when Enabled is false, but confer no git authority then.
type GoalOptions = api.GoalOptions

type GoalBead struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	Design             string `json:"design"`
	AcceptanceCriteria string `json:"acceptance_criteria"`
}

// BuildGoal preserves the bead's criteria verbatim, including whitespace and
// Markdown. The caller supplies the scope; agent-specific activation belongs
// to the runner, never to a second goal builder. In a round the agent only
// commits: nacred bumps the version and, if pushed was checked, pushes when it
// lands the round. A plain run's committed and pushed are the agent's to do.
func BuildGoal(beads []GoalBead, options GoalOptions, round bool) string {
	if !options.Enabled {
		return ""
	}
	o := options.Normalize()
	var s strings.Builder
	for _, b := range beads {
		fmt.Fprintf(&s, "## %s: %s\n\nDescription:\n%s\n\n", b.ID, b.Title, b.Description)
		if b.Design != "" {
			fmt.Fprintf(&s, "Design:\n%s\n\n", b.Design)
		}
		fmt.Fprintf(&s, "Acceptance criteria (quoted verbatim):\n%s\n\n", b.AcceptanceCriteria)
	}
	s.WriteString("Done when every acceptance criterion above is met")
	if round {
		// A round always needs its work committed, and nacred alone bumps
		// the version and pushes, on landing, whatever was checked.
		s.WriteString(" and the round's work has been committed on the round branch.\n")
		s.WriteString("The owner authorizes committing in this worktree only. Do not push or bump the version: nacred makes the version bump commit and runs the final checks")
		if o.Pushed {
			s.WriteString(", and pushes the work when it lands the round")
		} else {
			s.WriteString(", and lands the round")
		}
		s.WriteString(".\n")
	} else {
		if o.Committed {
			s.WriteString(" and a commit containing the work has been made")
			if o.Pushed {
				s.WriteString(" and pushed to the remote")
			}
		}
		s.WriteString(".\n")
		if o.Committed {
			s.WriteString("The owner authorizes making the required commit.")
			if o.Pushed {
				s.WriteString(" The owner also authorizes pushing that commit to the remote, without force pushing.")
			}
			s.WriteString("\n")
		}
	}
	s.WriteString("Verify each criterion with evidence; keep working and reworking until it holds. If a criterion needs unavailable access or human verification, report the blocker and leave the goal incomplete. A successful tool call or a summary alone is not acceptance.\n")
	return s.String()
}
