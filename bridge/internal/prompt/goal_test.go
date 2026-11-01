package prompt

import (
	"strings"
	"testing"
)

func TestGoalCompletionConditionsAndVerbatimBeads(t *testing.T) {
	beads := []GoalBead{{ID: "X-1", Title: "First", Description: "Full description\nincluding detail", AcceptanceCriteria: "  - first\n\n- `second`\n"}, {ID: "X-2", Title: "Second", Description: "Another description", AcceptanceCriteria: "Exact text"}}
	for _, tc := range []struct {
		name      string
		options   GoalOptions
		condition string
	}{
		{"off", GoalOptions{Committed: true, Pushed: true}, ""},
		{"criteria", GoalOptions{Enabled: true}, "Done when every acceptance criterion above is met."},
		{"commit", GoalOptions{Enabled: true, Committed: true}, "Done when every acceptance criterion above is met and a commit containing the work has been made."},
		{"push implies commit", GoalOptions{Enabled: true, Pushed: true}, "Done when every acceptance criterion above is met and a commit containing the work has been made and pushed to the remote."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildGoal(beads, tc.options, false)
			if !tc.options.Enabled {
				if got != "" {
					t.Fatal(got)
				}
				return
			}
			if !strings.Contains(got, tc.condition) {
				t.Fatal(got)
			}
			for _, b := range beads {
				for _, text := range []string{b.ID, b.Title, b.Description, b.AcceptanceCriteria} {
					if !strings.Contains(got, text) {
						t.Fatalf("missing %q in %s", text, got)
					}
				}
			}
		})
	}
	got := BuildGoal(beads, GoalOptions{Enabled: true, Pushed: true}, true)
	if !strings.Contains(got, "a single commit containing all of the round's work") {
		t.Fatal(got)
	}
}
