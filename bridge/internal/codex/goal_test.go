package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoalActivationAndResume(t *testing.T) {
	tn := Turn{Prompt: "work", Goal: "full bead"}
	args := tn.Args()
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--enable goals") || !strings.HasPrefix(args[len(args)-1], "/goal full bead\n") || !strings.Contains(joined, "First call create_goal") {
		t.Fatal(args)
	}
	tn.Resume = "thread"
	args = tn.Args()
	if args[len(args)-1] != "work" || !strings.Contains(strings.Join(args, " "), "--enable goals") {
		t.Fatal(args)
	}
	if strings.Contains(strings.Join((Turn{Prompt: "work"}).Args(), " "), "goals") {
		t.Fatal("ordinary run enabled goals")
	}
}

func TestGoalStateUsesProtocolAndRejectsMissingGoal(t *testing.T) {
	for _, state := range []string{"complete", "active", "blocked", ""} {
		t.Run(state, func(t *testing.T) {
			bin := filepath.Join(t.TempDir(), "codex")
			goal := `{"status":"` + state + `"}`
			if state == "" {
				goal = "null"
			}
			script := `#!/bin/sh
read init
echo '{"id":1,"result":{}}'
read initialized
read request
echo '{"method":"notification"}'
echo '{"id":2,"result":{"goal":` + goal + `}}'
cat >/dev/null
`
			if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			got, err := GoalState(context.Background(), bin, "thread", os.Environ())
			if state == "" {
				if err == nil {
					t.Fatal("accepted missing goal")
				}
			} else if err != nil || got != state {
				t.Fatalf("%s: %v", got, err)
			}
		})
	}
}
