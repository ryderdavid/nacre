package prompt

import (
	"fmt"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

// GoalOptions controls standalone runs. Round workers always use a goal and
// commit; nacre.yaml alone controls pushing.
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
// commits: nacred bumps the version and follows nacre.yaml land.push when it
// lands the round. A plain run's committed and pushed are the agent's to do.
func BuildGoal(beads []GoalBead, options GoalOptions, round bool) string {
	if !round && !options.Enabled {
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
		s.WriteString(", and lands the round according to nacre.yaml")
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
	// A long session gets summarized, and a summary can drop work that was
	// done; the bead and the branch keep it.
	s.WriteString("If the conversation has been summarized, re-read the bead (bd show) and the branch's commits (git log against its base) before deciding what is left; trust them over the summary. Finish by listing each criterion with the commit or check that meets it.\n")
	return s.String()
}
