package claude

import (
	"strings"
	"testing"
)

func TestTurnBudget(t *testing.T) {
	args := strings.Join(Turn{Prompt: "x", MaxBudgetUSD: 6}.Args(), " ")
	if !strings.Contains(args, "--max-budget-usd 6.00") {
		t.Errorf("args = %s", args)
	}
	// Without a budget, or with one above it, the runaway ceiling holds.
	if args := strings.Join(Turn{Prompt: "x"}.Args(), " "); !strings.Contains(args, "--max-budget-usd 50.00") {
		t.Errorf("a working session without the session ceiling: %s", args)
	}
	if args := strings.Join(Headless{Prompt: "x"}.Args(), " "); !strings.Contains(args, "--max-budget-usd 10.00") {
		t.Errorf("a call without the runaway ceiling: %s", args)
	}
	if args := strings.Join(Headless{Prompt: "x", MaxBudgetUSD: 40}.Args(), " "); !strings.Contains(args, "--max-budget-usd 10.00") {
		t.Errorf("a budget above the runaway ceiling: %s", args)
	}
}

func TestHeadlessReadRepo(t *testing.T) {
	args := strings.Join(Headless{Prompt: "x", ReadRepo: true, AllowedTools: BdReadTools}.Args(), " ")
	for _, want := range []string{"--tools Bash,Read,Grep,Glob", "Bash(bd show:*)", "Bash(git log:*)", "Bash(bd update:*)"} {
		if !strings.Contains(args, want) {
			t.Errorf("args lack %q: %s", want, args)
		}
	}
	if strings.Contains(args, " Edit") || strings.Contains(args, "Write") {
		t.Errorf("write tools offered: %s", args)
	}
}
