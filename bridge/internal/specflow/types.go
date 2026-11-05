// Package specflow connects reviewed OpenSpec changes to durable Beads work.
package specflow

import (
	"context"
	"errors"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/assist"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

var ErrConflict = errors.New("specification workflow conflict")
var ErrUnavailable = errors.New("specification workflow unavailable")

type Envelope struct {
	Available bool    `json:"available"`
	Reason    string  `json:"reason,omitempty"`
	Change    *Change `json:"change"`
	// Inventory says what a proposal is compared with; absent when the
	// repository's OpenSpec setup can't be read.
	Inventory *InventorySummary `json:"inventory,omitempty"`
	// Adoptable are active changes no workflow tracks yet, offered on a bead
	// without one so a change written outside Nacre can be taken over.
	Adoptable []Adoptable `json:"adoptable,omitempty"`
}
type Action struct {
	Action   string `json:"action"`
	Revision string `json:"revision,omitempty"`
	// Run starts and finishes the epic after successful approval/promotion.
	Run      bool   `json:"run,omitempty"`
	Feedback string `json:"feedback,omitempty"`
	// Intake is the New effort verdict a develop was confirmed with.
	Intake *IntakeHint `json:"intake,omitempty"`
	// ChangeName is the active change an adopt takes over.
	ChangeName string `json:"change_name,omitempty"`
	// RecordedReview lets an adopted change's review.md stand in for a new
	// independent review.
	RecordedReview bool `json:"recorded_review,omitempty"`
	// FoundBeads are the found beads a revise carries the notes of; set only
	// by Found, never by the HTTP change action.
	FoundBeads []string `json:"-"`
}
type Artifact struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}
type Finding struct {
	ID          string `json:"id"`
	Severity    string `json:"severity"`
	Summary     string `json:"summary"`
	Detail      string `json:"detail"`
	Disposition string `json:"disposition"`
	// NeedsOwner marks a finding only Ryder can settle: product direction,
	// scope, or a trade-off with no clearly better answer. Everything else
	// is the author's to fix without asking.
	NeedsOwner bool `json:"needs_owner,omitempty"`
	// Choices are the concrete ways to settle a NeedsOwner finding, best first.
	Choices []string `json:"choices,omitempty"`
}
type Section struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	BeadID    string   `json:"bead_id,omitempty"`
	DependsOn []string `json:"depends_on"`
}
type History struct {
	At      string `json:"at"`
	Kind    string `json:"kind"`
	Text    string `json:"text"`
	RoundID string `json:"round_id,omitempty"`
}
type RetiredSection struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
	BeadID string `json:"bead_id,omitempty"`
}
type Evidence struct {
	SectionID     string `json:"section_id"`
	BeadID        string `json:"bead_id"`
	RunID         string `json:"run_id,omitempty"`
	RoundState    string `json:"round_state,omitempty"`
	RoundID       string `json:"round_id,omitempty"`
	BuildVerified bool   `json:"build_verified"`
	Verified      bool   `json:"verified"`
	HumanCheck    bool   `json:"human_check"`
	Landed        bool   `json:"landed"`
	Note          string `json:"note"`
}
type Change struct {
	ID               string           `json:"id"`
	SeedID           string           `json:"seed_id"`
	EpicID           string           `json:"epic_id,omitempty"`
	ChangeName       string           `json:"change_name"`
	Phase            string           `json:"phase"`
	Revision         string           `json:"revision"`
	ApprovedRevision string           `json:"approved_revision,omitempty"`
	Summary          string           `json:"summary"`
	Route            string           `json:"route"`
	Rationale        string           `json:"rationale"`
	References       []string         `json:"references"`
	Artifacts        []Artifact       `json:"artifacts"`
	Findings         []Finding        `json:"findings"`
	Sections         []Section        `json:"sections"`
	RetiredSections  []RetiredSection `json:"retired_sections"`
	History          []History        `json:"history"`
	Evidence         []Evidence       `json:"evidence"`
	Error            string           `json:"error,omitempty"`
	// Intake is the New effort verdict development started from, if any.
	Intake *IntakeHint `json:"intake,omitempty"`
	// StaleReason names what broke the approval while the phase is stale.
	StaleReason string `json:"stale_reason,omitempty"`
	// Adoption is set when the change was written outside Nacre and adopted.
	Adoption *Adoption `json:"adoption,omitempty"`
	// AsApproved: the approved revision's own files are unchanged, whatever
	// happened elsewhere in the specifications. Set on each read.
	AsApproved bool    `json:"as_approved,omitempty"`
	CostUSD    float64 `json:"cost_usd"`
	// Guide is the plain summary and recommended next step while the change
	// waits on the owner; absent until written for its current state.
	Guide *Guide `json:"guide,omitempty"`
	// CreatedAt is when the proposal first existed: its authored artifacts
	// first passed strict validation, a covered disposition was recorded, or
	// the change was adopted. Set once and never cleared, so a later
	// revision is never reported as a draft.
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at"`
}
type Beads interface {
	Get(context.Context, string, string) (api.Issue, error)
	List(context.Context, string) ([]api.Issue, error)
	Create(context.Context, string, bd.CreateParams) (string, error)
	Update(context.Context, string, string, bd.UpdateParams) error
	Reopen(context.Context, string, string) error
	Close(ctx context.Context, repo, id, reason string) error
	DepAdd(context.Context, string, string, string, string) error
	DepRemove(context.Context, string, string, string) error
	Comments(context.Context, string, string) ([]api.Comment, error)
	AddComment(context.Context, string, string, string) error
}
type Headless interface {
	Run(context.Context, string, claude.Headless, func(api.RunEvent)) (assist.Outcome, error)
}
type Options struct {
	Dir      string
	Beads    Beads
	Headless Headless
	// Validate runs strict noninteractive OpenSpec validation. Nil discovers the installed CLI.
	Validate func(context.Context, string, string) error
	// RunEpic starts a round for the promoted epic with automatic landing and
	// finishing. It is called after ready is persisted, outside manager locks.
	RunEpic func(ctx context.Context, repoPath, epicID string) (roundID string, err error)
	// Active returns a description/link of an active linked implementation, or empty.
	Active func(repoPath, changeID string) string
	// Notify may publish a changed repository; it must not block.
	Evidence func(repoPath string, change Change) []Evidence
	Notify   func(repoPath string)
	// EvidenceStamp changes whenever evidence for the repository may have
	// moved (its rounds); summaries reuse cached evidence while it holds.
	// Nil recomputes evidence on every summary.
	EvidenceStamp func(repoPath string) string
	// Changed is told about every phase transition; it must not block.
	Changed func(repoPath string, change Change)
}

// MaxActionCostUSD is what one proposal action (drafting and its reviews) may
// spend in all: only a guard against a runaway, not a budget for the work. A
// pass usually costs about $2, so ordinary drafting stays well below it.
const MaxActionCostUSD = 30.0

// MaxPasses bounds author/reviewer passes in one action. The author keeps
// fixing the reviewer's findings until none it can settle are left.
const MaxPasses = 5

// OperationTimeout only stops a wedged operation; MaxActionCostUSD is what
// bounds a runaway one. Its author and reviewer calls run under it too.
const OperationTimeout = 2 * time.Hour

// Contract is immutable, exact-revision input for an implementation round.
type Contract struct {
	ChangeID   string     `json:"change_id"`
	Revision   string     `json:"revision"`
	SectionID  string     `json:"section_id"`
	BeadID     string     `json:"bead_id"`
	SeedID     string     `json:"seed_id"`
	EpicID     string     `json:"epic_id"`
	ChangeName string     `json:"change_name"`
	Artifacts  []Artifact `json:"artifacts"`
	Sections   []Section  `json:"sections"`
	Summary    string     `json:"summary"`
	Rationale  string     `json:"rationale"`
	References []string   `json:"references"`
}
