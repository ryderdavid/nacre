package api

// Attention item kinds: everything that waits on Ryder. See
// openspec/changes/surface-spec-workflow (needs-you).
const (
	AttentionBeadDecision        = "bead_decision"
	AttentionProposalReady       = "proposal_ready"
	AttentionReviewBlocked       = "review_blocked"
	AttentionApprovalStale       = "approval_stale"
	AttentionWorkflowInterrupted = "workflow_interrupted"
	AttentionRoundQuestion       = "round_question"
	AttentionBeadParked          = "bead_parked"
	AttentionRoundReady          = "round_ready"
	AttentionChangeArchivable    = "change_archivable"
	AttentionFoundWork           = "found_work"
	// AttentionEpicFinishable: every bead under an open epic is closed; its
	// branch can be merged into the landing target and the epic closed.
	AttentionEpicFinishable = "epic_finishable"
	// AttentionBeadStuck: a bead marked in progress that nothing is working on.
	AttentionBeadStuck = "bead_stuck"
	// AttentionBeadUnclosed: a round finished a bead's work but couldn't close it.
	AttentionBeadUnclosed = "bead_unclosed"
)

// FoundMarker is posted in a reply to a `Found:` comment once it is answered
// or dismissed, so it never raises a Needs you item again.
func FoundMarker(commentID string) string { return "[nacre-found:" + commentID + "]" }

// AttentionItem is one thing waiting on Ryder. ID is a digest of the kind and
// the underlying record and its state, so the same waiting state keeps its id
// across refreshes and computers.
type AttentionItem struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	Repo      string            `json:"repo"`
	RepoName  string            `json:"repo_name"`
	BeadID    string            `json:"bead_id,omitempty"`
	ChangeID  string            `json:"change_id,omitempty"`
	RoundID   string            `json:"round_id,omitempty"`
	Title     string            `json:"title"`
	Why       string            `json:"why"`
	SourceRef string            `json:"source_ref,omitempty"`
	Options   []AttentionOption `json:"options"`
	// Questions holds every unanswered question of a round, so one item
	// asks them all.
	Questions []AttentionQuestion `json:"questions,omitempty"`
	CreatedAt string              `json:"created_at"`
}

// AttentionQuestion is one round question with its answers, the
// recommended one first and marked.
type AttentionQuestion struct {
	ID      string            `json:"id"`
	Text    string            `json:"text"`
	Reason  string            `json:"reason,omitempty"`
	Options []AttentionOption `json:"options"`
}

// AttentionOption is either an exact API action (Type "action": Method, Path
// and Body) or a navigation (Type "open": Target).
type AttentionOption struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	// Recommended marks the answer Claude recommends.
	Recommended bool             `json:"recommended,omitempty"`
	Method      string           `json:"method,omitempty"`
	Path        string           `json:"path,omitempty"`
	Body        map[string]any   `json:"body,omitempty"`
	Target      *AttentionTarget `json:"target,omitempty"`
	// Disabled says why the option can't be used yet; the app shows it
	// instead of sending the action.
	Disabled string `json:"disabled,omitempty"`
}

// AttentionTarget is where an "open" option navigates: a bead, change or
// round, or a sheet opened with a prefill: "found_something" (Change,
// FoundIn, CommentID, Note) or "new_effort" (Note, Related, FollowOnOf,
// FoundIn and CommentID for the comment it answers).
type AttentionTarget struct {
	Kind       string   `json:"kind"`
	Repo       string   `json:"repo,omitempty"`
	ID         string   `json:"id,omitempty"`
	Revision   string   `json:"revision,omitempty"`
	Change     string   `json:"change,omitempty"`
	FoundIn    string   `json:"found_in,omitempty"`
	CommentID  string   `json:"comment_id,omitempty"`
	Note       string   `json:"note,omitempty"`
	Related    []string `json:"related,omitempty"`
	FollowOnOf string   `json:"follow_on_of,omitempty"`
}

// Change section evidence, from least to most.
const (
	EvidenceNone     = "none"
	EvidenceInRound  = "in_round"
	EvidenceVerified = "verified"
	EvidenceLanded   = "landed"
)

// ChangeSummary is one change on a repository's board: enough for card chips,
// the Proposals lane and the epic section strip.
type ChangeSummary struct {
	ID               string           `json:"id"`
	ChangeName       string           `json:"change_name"`
	Phase            string           `json:"phase"`
	Revision         string           `json:"revision,omitempty"`
	ApprovedRevision string           `json:"approved_revision,omitempty"`
	EpicID           string           `json:"epic_id,omitempty"`
	SeedIDs          []string         `json:"seed_ids"`
	Sections         []SectionSummary `json:"sections"`
	// Error is the plain reason a change needs attention or stopped.
	Error string `json:"error,omitempty"`
	// StaleReason names what broke the approval of a stale change.
	StaleReason string `json:"stale_reason,omitempty"`
	// PendingFound are found beads the revision under way carries.
	PendingFound []string `json:"pending_found,omitempty"`
	// Active names the round or run implementing the change, if any.
	Active string `json:"active,omitempty"`
	// AsApproved: the change's own files are still exactly the approved ones,
	// even if other specifications moved on, so its finished work can be
	// archived without another review.
	AsApproved bool   `json:"as_approved,omitempty"`
	UpdatedAt  string `json:"updated_at"`
}

// SectionSummary is one task section of a change and its evidence.
type SectionSummary struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	BeadID   string `json:"bead_id,omitempty"`
	Evidence string `json:"evidence"`
}
