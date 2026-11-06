// Package decisions keeps owner decisions: one exact nacred action, bound to
// the revision it applies to, that only Ryder authorizes from a paired
// device. A chat's agent raises them for protected actions; Needs you items
// are shown as decisions too. See
// openspec/changes/chat-first-harness/specs/owner-decisions.
package decisions

// Decision kinds. The first six are protected actions an agent can raise;
// the rest come from Needs you.
const (
	KindApprove  = "approve"
	KindLand     = "land"
	KindFinish   = "finish"
	KindSignoff  = "signoff"
	KindLimit    = "limit"
	KindAdopt    = "adopt"
	KindQuestion = "question"
	KindRecovery = "recovery"
	KindFound    = "found"
	KindIntake   = "intake"
)

// ProtectedKinds are the actions only Ryder authorizes, in the order docs
// and refusals name them.
var ProtectedKinds = []string{KindApprove, KindLand, KindFinish, KindSignoff, KindLimit, KindAdopt}

// Sources of a decision.
const (
	SourceAgent     = "agent"
	SourceAttention = "attention"
)

// States a decision can be in.
const (
	StatePending    = "pending"
	StateExecuting  = "executing"
	StateDone       = "done"
	StateFailed     = "failed"
	StateUnknown    = "unknown"
	StateDeclined   = "declined"
	StateExpired    = "expired"
	StateSuperseded = "superseded"
	StateDeferred   = "deferred"
	StateHeld       = "held"
)
