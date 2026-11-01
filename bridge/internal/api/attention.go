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
)

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
	CreatedAt string            `json:"created_at"`
}

// AttentionOption is either an exact API action (Type "action": Method, Path
// and Body) or a navigation (Type "open": Target).
type AttentionOption struct {
	Type   string           `json:"type"`
	Label  string           `json:"label"`
	Method string           `json:"method,omitempty"`
	Path   string           `json:"path,omitempty"`
	Body   map[string]any   `json:"body,omitempty"`
	Target *AttentionTarget `json:"target,omitempty"`
}

// AttentionTarget is where an "open" option navigates: a bead, change or round.
type AttentionTarget struct {
	Kind     string `json:"kind"`
	Repo     string `json:"repo,omitempty"`
	ID       string `json:"id"`
	Revision string `json:"revision,omitempty"`
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
	// Error is the plain reason a change needs attention, stopped or went stale.
	Error     string `json:"error,omitempty"`
	UpdatedAt string `json:"updated_at"`
}

// SectionSummary is one task section of a change and its evidence.
type SectionSummary struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	BeadID   string `json:"bead_id,omitempty"`
	Evidence string `json:"evidence"`
}
