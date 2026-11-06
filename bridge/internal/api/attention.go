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
	// AttentionBeadSignoff: a bead whose work is built and merged, with only
	// Ryder's own check left before it closes.
	AttentionBeadSignoff = "bead_signoff"
)

// A bead whose only remaining work is Ryder checking it carries the
// SignoffLabel, and SignoffKey in its metadata says what to check. Signing
// off closes it; anything else is ordinary open work.
const (
	SignoffLabel = "signoff"
	SignoffKey   = "signoff"
)

// HumanLabel marks a bead only a person can check. An open one that isn't a
// decision or an epic is a human check, shown like a sign-off: Sign off
// closes it, Not right adds the note as a comment and leaves it open.
const HumanLabel = "human"

// HumanCheck reports whether a bead with these labels and type is a human
// check (see HumanLabel). A decision is answered, and an epic finished, not
// signed off.
func HumanCheck(labels []string, issueType string) bool {
	if issueType == "decision" || issueType == "epic" {
		return false
	}
	for _, l := range labels {
		if l == HumanLabel {
			return true
		}
	}
	return false
}

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
	// Reopened counts the times the question came back after its answer
	// failed; each asking is its own decision.
	Reopened int `json:"reopened,omitempty"`
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
	// CreatedAt is when the change's proposal was first created; empty
	// while it has never existed.
	CreatedAt string `json:"created_at,omitempty"`
	// StartedAt is when the change record was started (its first history
	// entry). It never changes, so the Proposals lane can sort by it.
	StartedAt string `json:"started_at,omitempty"`
	// Draft is present while the change's proposal has not been created yet.
	Draft *ChangeDraft `json:"draft,omitempty"`
	// Status and Recommended come from the change's guide while it waits on
	// the owner: how it went, and the label of the recommended next step.
	Status      string `json:"status,omitempty"`
	Recommended string `json:"recommended,omitempty"`
	// Missing: the change's proposal was written, it isn't archived, and its
	// folder is gone from the checkout's openspec/changes.
	Missing bool `json:"missing,omitempty"`

	// The rest describe an untracked change: an active change folder in the
	// checkout that no workflow record tracks, listed in GET .../changes
	// under "untracked" (never in "changes", nor in Needs you). Its phase
	// comes from its files and tasks.md, and a section whose every task is
	// checked reads as landed.
	Untracked  bool `json:"untracked,omitempty"`
	TasksDone  int  `json:"tasks_done,omitempty"`
	TasksTotal int  `json:"tasks_total,omitempty"`
	HasReview  bool `json:"has_review,omitempty"`
	// ArchivedAs names an archived folder of the same change, when one
	// exists: the active folder may be a leftover copy.
	ArchivedAs string `json:"archived_as,omitempty"`
}

// Withdrawn reports whether a change was proven given up: its folder is gone
// from the checkout and every bead that owns it (seeds and epic) is closed.
// closed reports a bead's state; it returns false for a bead it doesn't know,
// so an unread board never withdraws anything. A change whose folder is gone
// while one of its beads is still open stays listed: that may be an accident
// with live work.
func (c ChangeSummary) Withdrawn(closed func(id string) bool) bool {
	if !c.Missing || c.Phase == "archived" {
		return false
	}
	owners := append([]string{}, c.SeedIDs...)
	if c.EpicID != "" {
		owners = append(owners, c.EpicID)
	}
	owned := false
	for _, id := range owners {
		if id == "" {
			continue
		}
		if !closed(id) {
			return false
		}
		owned = true
	}
	return owned
}

// Draft kinds: a proposal still being drafted, or drafting that stopped
// before the proposal was created.
const (
	DraftPending = "pending"
	DraftFailed  = "failed"
)

// ChangeDraft is a change whose proposal doesn't exist yet. Message is the
// change's error when the draft failed.
type ChangeDraft struct {
	Kind    string `json:"kind"`
	Message string `json:"message,omitempty"`
}

// SectionSummary is one task section of a change and its evidence.
type SectionSummary struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	BeadID   string `json:"bead_id,omitempty"`
	Evidence string `json:"evidence"`
}
