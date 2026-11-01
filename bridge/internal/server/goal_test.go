package server

import (
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/codex"
)

func TestRunAPIUsesSharedGoalForBothAgents(t *testing.T) {
	st, checks := codex.Status{Version: "1"}, 0
	ts, repo, mgr := codexServer(t, &st, &checks)
	for _, agent := range []string{api.AgentClaudeCode, api.AgentCodex} {
		for _, enabled := range []bool{false, true} {
			body := `{"agent":"` + agent + `","goal":{"enabled":` + map[bool]string{true: "true", false: "false"}[enabled] + `,"pushed":true}}`
			resp, result := postJSON(t, ts.URL+"/v1/repos/"+repo+"/issues/X-1/runs", body)
			if resp.StatusCode != 201 {
				t.Fatalf("%d %v", resp.StatusCode, result)
			}
			run, events, err := mgr.Get(result["id"].(string))
			if err != nil {
				t.Fatal(err)
			}
			if enabled {
				for _, part := range []string{"X-1: Do the thing", "Full detail", "- exact criterion", "made and pushed to the remote"} {
					if !strings.Contains(run.GoalText, part) {
						t.Fatalf("goal missing %q: %s", part, run.GoalText)
					}
				}
				if len(events) < 2 || !strings.HasPrefix(events[1].Text, "/goal ") {
					t.Fatalf("no explicit goal: %+v", events)
				}
			} else if run.GoalText != "" || strings.Contains(events[1].Text, "/goal ") {
				t.Fatal("disabled request changed launch")
			}
		}
	}
}
