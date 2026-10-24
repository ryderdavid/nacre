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
	if strings.Contains(strings.Join(Turn{Prompt: "x"}.Args(), " "), "max-budget") {
		t.Error("budget flag without a budget")
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
