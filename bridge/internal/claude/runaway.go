package claude

import "strconv"

// Runaway ceilings: the most one claude process nacred starts may spend.
// They aren't budgets for the work; they only stop a process stuck in a
// token-heavy loop with no way out. A caller asking for more is held to them.
const (
	// RunawayUSD holds one call: a chat answer, a draft, a review.
	RunawayUSD = 10.0
	// SessionRunawayUSD holds a working session, such as a bead's goal run,
	// which re-reads a growing context for hours and can rightly cost far
	// more than any one call.
	SessionRunawayUSD = 50.0
)

// budgetArgs is the --max-budget-usd for a process asked to stay under max
// and held to ceiling; 0 means it has no limit of its own.
func budgetArgs(max, ceiling float64) []string {
	if max <= 0 || max > ceiling {
		max = ceiling
	}
	return []string{"--max-budget-usd", strconv.FormatFloat(max, 'f', 2, 64)}
}
