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
}
type Action struct {
	Action   string `json:"action"`
	Revision string `json:"revision,omitempty"`
	Feedback string `json:"feedback,omitempty"`
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
}
type Section struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	BeadID    string   `json:"bead_id,omitempty"`
	DependsOn []string `json:"depends_on"`
}
type History struct {
	At   string `json:"at"`
	Kind string `json:"kind"`
	Text string `json:"text"`
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
	// StaleReason names what broke the approval while the phase is stale.
	StaleReason string  `json:"stale_reason,omitempty"`
	CostUSD     float64 `json:"cost_usd"`
	UpdatedAt   string  `json:"updated_at"`
}
type Beads interface {
	Get(context.Context, string, string) (api.Issue, error)
	List(context.Context, string) ([]api.Issue, error)
	Create(context.Context, string, bd.CreateParams) (string, error)
	Update(context.Context, string, string, bd.UpdateParams) error
	Reopen(context.Context, string, string) error
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

const MaxActionCostUSD = 5.0
const MaxPasses = 3
const OperationTimeout = 30 * time.Minute

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
