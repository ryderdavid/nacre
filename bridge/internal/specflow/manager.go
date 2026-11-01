package specflow

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

type issue struct {
	ID           string       `json:"id"`
	Title        string       `json:"title"`
	Description  string       `json:"description"`
	Design       string       `json:"design"`
	Acceptance   string       `json:"acceptance_criteria"`
	Notes        string       `json:"notes"`
	Type         string       `json:"issue_type"`
	Status       string       `json:"status"`
	Labels       []string     `json:"labels"`
	Parent       string       `json:"parent"`
	Dependencies []dependency `json:"dependencies"`
}
type dependency struct {
	IssueID        string `json:"issue_id,omitempty"`
	DependsOnID    string `json:"depends_on_id,omitempty"`
	Type           string `json:"type,omitempty"`
	ID             string `json:"id,omitempty"`
	DependencyType string `json:"dependency_type,omitempty"`
}
type generated struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Design       string   `json:"design"`
	Acceptance   string   `json:"acceptance"`
	Dependencies []string `json:"dependencies"`
}
type callResult struct {
	Kind   string          `json:"kind"`
	Output json.RawMessage `json:"output,omitempty"`
	Error  string          `json:"error,omitempty"`
}
type record struct {
	LastCall          *callResult          `json:"last_call,omitempty"`
	Repo              string               `json:"repo"`
	Change            Change               `json:"change"`
	Aliases           []string             `json:"aliases"`
	ExtraSeeds        []issue              `json:"extra_seeds,omitempty"`
	PendingPrepared   bool                 `json:"pending_prepared"`
	Seed              issue                `json:"seed"`
	Stage             string               `json:"stage"`
	Feedback          string               `json:"feedback"`
	Inventory         inventory            `json:"inventory"`
	Baseline          map[string]string    `json:"baseline"`
	PendingAuthor     *authorResult        `json:"pending_author,omitempty"`
	Generated         map[string]generated `json:"generated"`
	Planned           map[string]generated `json:"planned"`
	ActionCost        float64              `json:"action_cost"`
	ReservedCost      float64              `json:"reserved_cost"`
	ReviewCalls       int                  `json:"review_calls"`
	Pass              int                  `json:"pass"`
	PromotionRevision string               `json:"promotion_revision,omitempty"`
}
type Manager struct {
	opts            Options
	mu              sync.Mutex
	records         map[string]*record
	busy            map[string]bool
	evidence        map[string]evidenceCache
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	defaultValidate bool
}

func New(opts Options) (*Manager, error) {
	if opts.Dir == "" || opts.Beads == nil {
		return nil, fmt.Errorf("specflow requires a private data directory and Beads client")
	}
	if err := os.MkdirAll(opts.Dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(opts.Dir, 0o700); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{opts: opts, records: map[string]*record{}, busy: map[string]bool{}, evidence: map[string]evidenceCache{}, ctx: ctx, cancel: cancel, defaultValidate: opts.Validate == nil}
	if m.opts.Validate == nil {
		m.opts.Validate = strictValidator
	}
	entries, err := os.ReadDir(opts.Dir)
	if err != nil {
		cancel()
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		b, e := os.ReadFile(filepath.Join(opts.Dir, entry.Name()))
		if e != nil {
			cancel()
			return nil, e
		}
		var r record
		if e = json.Unmarshal(b, &r); e != nil || r.Change.ID == "" || filepath.Base(r.Change.ID) != r.Change.ID || entry.Name() != r.Change.ID+".json" || r.Repo == "" {
			cancel()
			return nil, fmt.Errorf("cannot recover workflow record %s; restore the saved record before continuing", entry.Name())
		}
		if r.Generated == nil {
			r.Generated = map[string]generated{}
		}
		if r.Planned == nil {
			r.Planned = map[string]generated{}
		}
		if r.Change.Phase == "drafting" || r.Change.Phase == "reviewing" || r.Change.Phase == "promoting" {
			r.Change.Phase = "interrupted"
			r.Change.Error = "The computer stopped during this operation. Resume to continue from the saved checkpoint."
			if r.ReservedCost > 0 {
				r.ActionCost += r.ReservedCost
				r.Change.CostUSD += r.ReservedCost
				r.ReservedCost = 0
				history(&r, "cost", "An interrupted AI call was charged its reserved budget conservatively; actual provider cost may be lower.")
			}
			history(&r, "interrupted", r.Change.Error)
			if e = atomicJSON(filepath.Join(opts.Dir, r.Change.ID+".json"), &r); e != nil {
				cancel()
				return nil, e
			}
		}
		for _, other := range m.records {
			if other.Repo == r.Repo && other.Change.ChangeName != "" && other.Change.ChangeName == r.Change.ChangeName {
				cancel()
				return nil, fmt.Errorf("duplicate workflow records for %s; restore a single canonical record", r.Change.ChangeName)
			}
		}
		m.records[r.Change.ID] = &r
	}
	return m, nil
}
func (m *Manager) Shutdown(timeout time.Duration) {
	m.cancel()
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}
func (m *Manager) Wait() { m.wg.Wait() }

// Busy reports unfinished author, reviewer or promotion work before maintenance.
func (m *Manager) Busy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.busy) != 0
}
func stamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func newID() string {
	b := make([]byte, 10)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return "sf" + hex.EncodeToString(b)
}
func history(r *record, kind, text string) {
	r.Change.History = append(r.Change.History, History{stamp(), kind, text})
	r.Change.UpdatedAt = stamp()
}
func clone[T any](value T) T {
	b, _ := json.Marshal(value)
	var out T
	_ = json.Unmarshal(b, &out)
	return out
}
func parseIssue(raw api.Issue) (issue, error) {
	var is issue
	e := json.Unmarshal(raw, &is)
	if e == nil && is.ID == "" {
		e = fmt.Errorf("Beads returned an issue without an id")
	}
	for i := range is.Dependencies {
		d := &is.Dependencies[i]
		if d.DependsOnID == "" {
			d.DependsOnID = d.ID
		}
		if d.Type == "" {
			d.Type = d.DependencyType
		}
	}
	return is, e
}
func hasWorkflowLabel(is issue) bool {
	for _, l := range is.Labels {
		if strings.HasPrefix(l, "specflow:") || strings.HasPrefix(l, "spec-section:") {
			return true
		}
	}
	return false
}
func (m *Manager) findLocked(repo, id string) *record {
	// Prefer a canonical change over a provisional seed if a crash happened
	// between saving its alias and removing the provisional record.
	var provisional *record
	for _, r := range m.records {
		if r.Repo != repo {
			continue
		}
		matched := r.Change.SeedID == id || r.Change.EpicID == id || contains(r.Aliases, id)
		for _, s := range r.Change.Sections {
			matched = matched || s.BeadID == id
		}
		for _, s := range r.Change.RetiredSections {
			matched = matched || s.BeadID == id
		}
		if matched {
			if r.Change.ChangeName != "" {
				return r
			}
			provisional = r
		}
	}
	return provisional
}

func (m *Manager) saveLocked(r *record) error {
	r.Change.UpdatedAt = stamp()
	if e := atomicJSON(filepath.Join(m.opts.Dir, r.Change.ID+".json"), r); e != nil {
		return fmt.Errorf("cannot persist change workflow: %w", e)
	}
	prev := m.records[r.Change.ID]
	m.records[r.Change.ID] = clone(r)
	if m.opts.Changed != nil && (prev == nil || prev.Change.Phase != r.Change.Phase) {
		m.opts.Changed(r.Repo, clone(r.Change))
	}
	return nil
}
func (m *Manager) checkpoint(r *record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.saveLocked(r)
	if e == nil && m.opts.Notify != nil {
		m.opts.Notify(r.Repo)
	}
	return e
}
func (m *Manager) available() string {
	if m.opts.Headless == nil {
		return "The proposal author is unavailable on this computer."
	}
	if m.defaultValidate {
		if _, e := exec.LookPath("openspec"); e != nil {
			return "Install the OpenSpec CLI on this computer to develop and validate proposals."
		}
	}
	return ""
}
func (m *Manager) snapshot(r *record) Envelope {
	env := Envelope{Available: true}
	if reason := m.available(); reason != "" {
		env.Available = false
		env.Reason = reason
	}
	if r != nil {
		c := clone(r.Change)
		if m.opts.Evidence != nil {
			c.Evidence = m.opts.Evidence(r.Repo, c)
		}
		env.Change = &c
	}
	return env
}

type intent struct{ ID, Title, Description, Design, Acceptance, Notes string }

func intentOf(is issue) intent {
	return intent{is.ID, is.Title, is.Description, is.Design, is.Acceptance, is.Notes}
}
func intentRevision(inv inventory, artifacts []Artifact, seed issue, extra []issue) string {
	intents := []intent{intentOf(seed)}
	for _, is := range extra {
		intents = append(intents, intentOf(is))
	}
	return digest(revision(inv, artifacts) + jsonText(intents))
}
func (m *Manager) refreshIntents(ctx context.Context, r *record) error {
	seed, e := m.getIssue(ctx, r.Repo, r.Change.SeedID)
	if e != nil {
		return e
	}
	r.Seed = seed
	extra := []issue{}
	for _, id := range r.Aliases {
		if id == r.Change.SeedID {
			continue
		}
		is, e := m.getIssue(ctx, r.Repo, id)
		if e != nil {
			return e
		}
		extra = append(extra, is)
	}
	r.ExtraSeeds = extra
	return nil
}
func (m *Manager) current(r *record) error {
	inv, e := collect(r.Repo)
	if e != nil {
		return e
	}
	fresh := clone(r)
	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	defer cancel()
	if e := m.refreshIntents(ctx, fresh); e != nil {
		return fmt.Errorf("Cannot verify current seed intent: %w", e)
	}
	if intentRevision(inv, r.Change.Artifacts, fresh.Seed, fresh.ExtraSeeds) != r.Change.Revision {
		return fmt.Errorf("The proposal or project context changed after review. Revise and review this change before continuing.")
	}
	return nil
}
func (m *Manager) Get(ctx context.Context, repo, id string) (Envelope, error) {
	repo, e := canonicalRepo(repo)
	if e != nil {
		return Envelope{}, e
	}
	m.mu.Lock()
	r := clone(m.findLocked(repo, id))
	busy := m.busy[repo]
	m.mu.Unlock()
	if r == nil {
		raw, e := m.opts.Beads.Get(ctx, repo, id)
		if e != nil {
			return Envelope{}, e
		}
		is, e := parseIssue(raw)
		if e != nil {
			return Envelope{}, e
		}
		if hasWorkflowLabel(is) {
			return Envelope{Available: false, Reason: "This bead has specification labels but its saved workflow is missing. Restore the workflow record before starting work."}, nil
		}
		env := m.snapshot(nil)
		inv, e := collect(repo)
		if e == nil {
			env.Inventory = summarize(inv)
		} else if env.Available {
			env.Available = false
			env.Reason = e.Error()
		}
		return env, nil
	}
	if !busy && r.Change.Revision != "" && (r.Change.Phase == "ready" || r.Change.Phase == "awaiting_approval" || r.Change.Phase == "covered") {
		currentErr := m.current(r)
		if currentErr == nil && r.Change.Phase == "ready" {
			for _, section := range r.Change.Sections {
				if e := m.sectionCurrent(ctx, r, section); e != nil {
					currentErr = e
					break
				}
			}
		}
		if err := currentErr; err != nil {
			r.Change.Phase = "stale"
			r.Change.Error = err.Error()
			history(r, "stale", err.Error())
			m.mu.Lock()
			live := m.records[r.Change.ID]
			if !m.busy[repo] && live != nil && live.Change.Revision == r.Change.Revision && (live.Change.Phase == "ready" || live.Change.Phase == "awaiting_approval" || live.Change.Phase == "covered") {
				err = m.saveLocked(r)
			} else {
				r = clone(live)
				err = nil
			}
			m.mu.Unlock()
			if err != nil {
				return Envelope{}, err
			}
		}
	}
	env := m.snapshot(r)
	if inv, e := collect(repo); e == nil {
		env.Inventory = summarize(inv)
	}
	return env, nil
}
func (m *Manager) Act(ctx context.Context, repo, id string, a Action) (Envelope, error) {
	if a.Action == "refresh" {
		return m.Get(ctx, repo, id)
	}
	if len(a.Feedback) > 12000 {
		return Envelope{}, fmt.Errorf("feedback exceeds 12000 characters")
	}
	repo, e := canonicalRepo(repo)
	if e != nil {
		return Envelope{}, e
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r := clone(m.findLocked(repo, id))
	if m.busy[repo] {
		if a.Action == "revise" || a.Action == "approve" || (a.Action == "develop" && strings.TrimSpace(a.Feedback) != "") {
			return Envelope{}, fmt.Errorf("%w: this repository already has a proposal operation in progress; wait for it to finish before submitting this action", ErrConflict)
		}
		if r != nil {
			return m.snapshot(r), nil
		}
		return Envelope{}, fmt.Errorf("%w: another proposal is being developed in this repository", ErrConflict)
	}
	if a.Action != "develop" && a.Action != "revise" && a.Action != "approve" && a.Action != "resume" {
		return Envelope{}, fmt.Errorf("unknown change action %q", a.Action)
	}
	if r == nil {
		if a.Action != "develop" {
			return Envelope{}, fmt.Errorf("%w: develop a proposal first", ErrConflict)
		}
		raw, e := m.opts.Beads.Get(ctx, repo, id)
		if e != nil {
			return Envelope{}, e
		}
		seed, e := parseIssue(raw)
		if e != nil {
			return Envelope{}, e
		}
		if hasWorkflowLabel(seed) {
			return Envelope{}, fmt.Errorf("%w: specification labels have no saved workflow; restore the workflow record", ErrConflict)
		}
		r = &record{Repo: repo, Seed: seed, Generated: map[string]generated{}, Planned: map[string]generated{}, Change: Change{ID: newID(), SeedID: id, References: []string{}, Artifacts: []Artifact{}, Findings: []Finding{}, Sections: []Section{}, RetiredSections: []RetiredSection{}, History: []History{}, Evidence: []Evidence{}}}
	} else if a.Action == "develop" {
		if strings.TrimSpace(a.Feedback) != "" {
			return Envelope{}, fmt.Errorf("%w: this bead already has a proposal; submit your feedback with Revise", ErrConflict)
		}
		return m.snapshot(r), nil
	}
	if a.Action == "revise" && r.Stage == "promote" {
		return Envelope{}, fmt.Errorf("%w: resume the partially synchronized proposal before starting a revision", ErrConflict)
	}
	if m.opts.Active != nil {
		if active := m.opts.Active(repo, r.Change.ID); active != "" {
			return Envelope{}, fmt.Errorf("%w: finish the active implementation before revising this change: %s", ErrConflict, active)
		}
	}
	if a.Action == "approve" {
		if r.Change.Phase == "ready" && a.Revision == r.Change.ApprovedRevision {
			return m.snapshot(r), nil
		}
		if a.Revision == "" || a.Revision != r.Change.Revision {
			return Envelope{}, fmt.Errorf("%w: approval must name the current reviewed revision", ErrConflict)
		}
		if r.Change.Phase != "awaiting_approval" && !(r.Stage == "promote" && (r.Change.Phase == "needs_attention" || r.Change.Phase == "interrupted")) {
			return Envelope{}, fmt.Errorf("%w: this proposal has not completed independent review", ErrConflict)
		}
		currentErr := m.current(r)
		if currentErr == nil && r.Change.Phase == "ready" {
			for _, section := range r.Change.Sections {
				if e := m.sectionCurrent(ctx, r, section); e != nil {
					currentErr = e
					break
				}
			}
		}
		if err := currentErr; err != nil {
			r.Change.Phase = "stale"
			r.Change.Error = err.Error()
			if e = m.saveLocked(r); e != nil {
				return Envelope{}, e
			}
			return Envelope{}, fmt.Errorf("%w: %v", ErrConflict, err)
		}
		r.Change.Phase = "promoting"
		r.Stage = "promote"
		r.PromotionRevision = a.Revision
		history(r, "approval", "Owner approved revision "+a.Revision+". Synchronizing the epic and section beads.")
	} else if a.Action == "resume" {
		if r.Change.Phase != "interrupted" && r.Change.Phase != "needs_attention" {
			return Envelope{}, fmt.Errorf("%w: this change has no interrupted operation to resume", ErrConflict)
		}
		if r.Stage == "promote" {
			if e = m.current(r); e != nil {
				return Envelope{}, fmt.Errorf("%w: %v", ErrConflict, e)
			}
			r.Change.Phase = "promoting"
		} else {
			if reason := m.available(); reason != "" {
				return Envelope{}, fmt.Errorf("%w: %s", ErrUnavailable, reason)
			}
			if r.Stage == "" {
				r.Stage = "author"
			}
			r.Change.Phase = "drafting"
		}
		history(r, "resume", "Resuming the saved operation without discarding completed steps.")
	} else {
		if reason := m.available(); reason != "" {
			return Envelope{}, fmt.Errorf("%w: %s", ErrUnavailable, reason)
		}
		inv, e := collect(repo)
		if e != nil {
			return Envelope{}, fmt.Errorf("%w: %v", ErrUnavailable, e)
		}
		if e := m.refreshIntents(ctx, r); e != nil {
			return Envelope{}, e
		}
		r.Inventory = inv
		r.Baseline = clone(inv.Digests)
		r.Stage = "author"
		r.Pass = 0
		r.ReviewCalls = 0
		r.ActionCost = 0
		r.ReservedCost = 0
		r.PendingAuthor = nil
		r.PendingPrepared = false
		r.LastCall = nil
		r.Feedback = a.Feedback
		r.Change.Error = ""
		r.Change.Phase = "drafting"
		history(r, "intent", "Developing the bead against the complete project specification inventory.")
	}
	if e = m.saveLocked(r); e != nil {
		return Envelope{}, e
	}
	m.busy[repo] = true
	m.wg.Add(1)
	snapshot := m.snapshot(r)
	go m.work(r)
	return snapshot, nil
}
func (m *Manager) work(r *record) {
	defer m.wg.Done()
	defer func() { m.mu.Lock(); delete(m.busy, r.Repo); m.mu.Unlock() }()
	ctx, cancel := context.WithTimeout(m.ctx, OperationTimeout)
	defer cancel()
	var err error
	if r.Stage == "promote" {
		err = m.promote(ctx, r)
	} else {
		err = m.cycle(ctx, r)
	}
	if err != nil {
		r.Change.Error = err.Error()
		r.Change.Phase = "needs_attention"
		if ctx.Err() != nil || r.Stage == "promote" || r.Stage == "decision" {
			r.Change.Phase = "interrupted"
		}
		history(r, "error", err.Error())
		if persistErr := m.checkpoint(r); persistErr != nil {
			// A storage failure must remain visible immediately, even when writing
			// the diagnostic itself fails. The on-disk checkpoint stays recoverable.
			r.Change.Phase = "interrupted"
			r.Change.Error = err.Error() + "; " + persistErr.Error() + ". Restore storage before resuming."
			m.mu.Lock()
			m.records[r.Change.ID] = clone(r)
			m.mu.Unlock()
		}
	}
}

// Contract fails closed for every linked bead, including orphaned labels.
func (m *Manager) Contract(ctx context.Context, repo, id string) (*Contract, error) {
	repo, e := canonicalRepo(repo)
	if e != nil {
		return nil, e
	}
	m.mu.Lock()
	r := clone(m.findLocked(repo, id))
	busy := m.busy[repo]
	m.mu.Unlock()
	if r == nil {
		raw, e := m.opts.Beads.Get(ctx, repo, id)
		if e != nil {
			return nil, e
		}
		is, e := parseIssue(raw)
		if e != nil {
			return nil, e
		}
		if hasWorkflowLabel(is) {
			return nil, fmt.Errorf("%w: this bead is specification-linked but its saved workflow is missing; restore the workflow record", ErrConflict)
		}
		return nil, nil
	}
	if r.Change.EpicID == id || r.Change.SeedID == id {
		return nil, fmt.Errorf("%w: this bead is a specification orchestration entry point; start a specification round with its section beads", ErrConflict)
	}
	if busy || r.Change.Phase != "ready" || r.Change.ApprovedRevision == "" || r.Change.ApprovedRevision != r.Change.Revision {
		return nil, fmt.Errorf("%w: the linked proposal must be reviewed, approved, and synchronized before implementation", ErrConflict)
	}
	if e = m.current(r); e != nil {
		return nil, fmt.Errorf("%w: %v", ErrConflict, e)
	}
	for _, s := range r.Change.Sections {
		if s.BeadID == id {
			if e := m.sectionCurrent(ctx, r, s); e != nil {
				return nil, e
			}
			return &Contract{ChangeID: r.Change.ID, Revision: r.Change.Revision, SectionID: s.ID, BeadID: id, SeedID: r.Change.SeedID, EpicID: r.Change.EpicID, ChangeName: r.Change.ChangeName, Artifacts: clone(r.Change.Artifacts), Sections: clone(r.Change.Sections), Summary: r.Change.Summary, Rationale: r.Change.Rationale, References: clone(r.Change.References)}, nil
		}
	}
	return nil, fmt.Errorf("%w: this bead is retired or not mapped to a current specification task section", ErrConflict)
}

// sectionCurrent checks the generated contract fields, while allowing ordinary
// execution status, assignee, priority and progress-note updates.
func (m *Manager) sectionCurrent(ctx context.Context, r *record, s Section) error {
	current, e := m.getIssue(ctx, r.Repo, s.BeadID)
	if e != nil {
		return e
	}
	expected, known := r.Generated[s.ID]
	if !known || !fieldsEqual(current, expected) || issueParent(current) != r.Change.EpicID || !contains(current.Labels, workflowLabel(r)) || !contains(current.Labels, sectionLabel(s)) {
		return fmt.Errorf("%w: %s changed after synchronization; reconcile its generated fields, parent and specification labels with the reviewed section before starting work", ErrConflict, s.BeadID)
	}
	for _, dep := range expected.Dependencies {
		if !hasDep(current, dep) {
			return fmt.Errorf("%w: %s lost a required specification dependency; restore it before starting work", ErrConflict, s.BeadID)
		}
	}
	return nil
}
