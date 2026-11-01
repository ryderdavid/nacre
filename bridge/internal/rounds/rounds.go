// Package rounds carries a set of beads in one repository from a pre-flight
// review of their intentions to one commit that passes the repository's
// gates and builds, ready to land. It runs on the Mac without any client
// attached and survives nacred restarting.
//
// Each round is advanced by its own goroutine: advance() looks at the
// round's state and starts whatever can start (a pre-flight call, a bead's
// run, a verification, a merge, the final build). Long operations run in
// the background and record their result on the round when they finish;
// advance() then runs again. The design is in
// openspec/changes/semi-attended-rounds.
package rounds

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/assist"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
	"github.com/ryderdavid/nacre/bridge/internal/gates"
	"github.com/ryderdavid/nacre/bridge/internal/repoconfig"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
	"github.com/ryderdavid/nacre/bridge/internal/workspace"
)

// Runs is the run manager, as rounds use it.
type Runs interface {
	Start(runs.StartRequest) (api.Run, error)
	FollowupWith(id string, f runs.Followup) (api.Run, error)
	Cancel(id string) (api.Run, error)
	Run(id string) (api.Run, error)
	SetVerification(id string, v *api.Verification) error
}

// Beads is the bd client, as rounds use it.
type Beads interface {
	Get(ctx context.Context, repo, id string) (api.Issue, error)
	List(ctx context.Context, repo string) ([]api.Issue, error)
	Comments(ctx context.Context, repo, id string) ([]api.Comment, error)
	AddComment(ctx context.Context, repo, id, text string) error
	Update(ctx context.Context, repo, id string, p bd.UpdateParams) error
	Close(ctx context.Context, repo, id, reason string) error
	DepAdd(ctx context.Context, repo, issue, dependsOn, typ string) error
}

// Headless runs one structured `claude -p` call (the pre-flight review).
type Headless interface {
	Run(ctx context.Context, cwd string, h claude.Headless, emit func(api.RunEvent)) (assist.Outcome, error)
}

// Gates runs nacre.yaml commands.
type Gates interface {
	RunAll(ctx context.Context, dir string, cfg *repoconfig.Config, steps []repoconfig.Step, stopOnFail bool) []gates.Result
}

// Defaults for rounds that don't set their own limits.
const (
	DefaultBudgetUSD        = 25
	DefaultTimeLimitMinutes = 240
)

// Options configure a Manager.
type Options struct {
	// Dir holds one directory per round.
	Dir        string
	Runs       Runs
	Beads      Beads
	Headless   Headless
	Gates      Gates
	Workspaces *workspace.Manager
	// Repo resolves a repository id to its name and path.
	Repo func(id string) (name, path string, err error)
	// SpecContract returns the current approved contract, nil for ordinary
	// beads, and an error for linked work whose approval cannot be proved.
	SpecContract func(context.Context, string, string) (*api.SpecContract, error)
	// Requirements returns the repository's requirement index for the
	// spec-drift check; nil, or an error, skips the check.
	Requirements func(repoPath string) (*api.RequirementIndex, error)
	// Develop starts proposal development for a bead with feedback, for the
	// spec-drift answer "Amend the spec first". An error is why it was refused.
	Develop func(ctx context.Context, repoPath, beadID, feedback string) error
	// Mode and Model are used for the round's runs ("" model = CLI default).
	Mode, Model string
	// BudgetUSD and TimeLimitMinutes are the defaults for new rounds.
	BudgetUSD        float64
	TimeLimitMinutes int
	// Notify publishes a global notification (event "round" with an
	// api.Round). It must not block.
	Notify func(event string, data any)
	// RepoChanged is called after the round writes to a repository's beads.
	RepoChanged func(repoID string)
	Logger      *log.Logger
}

// Errors returned to API callers.
var (
	ErrNotFound = errors.New("unknown round")
	ErrConflict = errors.New("conflict")
)

// conflict is an ErrConflict with a message for the person.
type conflict string

func (c conflict) Error() string { return string(c) }
func (c conflict) Unwrap() error { return ErrConflict }

// Manager owns every round on this computer.
type Manager struct {
	opts Options

	mu         sync.Mutex
	rounds     map[string]*round
	leases     *leases
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	background atomic.Int64
}

// round is one round's live state. mu guards everything; it is never held
// while waiting on git, gates or claude.
type round struct {
	mu   sync.Mutex
	m    *Manager
	dir  string
	meta api.Round
	cfg  *repoconfig.Config
	// ops are the background operations in flight, by key ("preflight",
	// "verify:NAC-1", "merge", ...). They are not persisted: after a
	// restart advance() starts whatever is still needed.
	ops     map[string]bool
	events  []api.RoundEvent
	nextSeq int64
	kick    chan struct{}
	subs    map[chan struct{}]struct{}
	ver     uint64
	// stallCancelled marks runs the round cancelled because they stalled,
	// so the cancellation is answered with a resume rather than counted as
	// a failure.
	stallCancelled map[string]bool
	// sinceKick says the run states advance() last saw, to notice changes.
	seenRun map[string]string
}

// New loads the rounds in opts.Dir and starts every active one.
func New(opts Options) (*Manager, error) {
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}
	if opts.BudgetUSD <= 0 {
		opts.BudgetUSD = DefaultBudgetUSD
	}
	if opts.TimeLimitMinutes <= 0 {
		opts.TimeLimitMinutes = DefaultTimeLimitMinutes
	}
	if opts.Mode == "" {
		opts.Mode = "auto"
	}
	if err := os.MkdirAll(opts.Dir, 0o700); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{opts: opts, rounds: map[string]*round{}, leases: newLeases(), ctx: ctx, cancel: cancel}
	entries, err := os.ReadDir(opts.Dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		r, err := m.load(filepath.Join(opts.Dir, e.Name()))
		if err != nil {
			opts.Logger.Printf("rounds: skipping %s: %v", e.Name(), err)
			continue
		}
		m.rounds[r.meta.ID] = r
	}
	return m, nil
}

// Start begins advancing every active round. Call it once the run manager is
// ready to take runs.
func (m *Manager) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rounds {
		if r.meta.Active() {
			m.spawn(r)
		}
	}
}

// Shutdown stops advancing rounds. Background operations finish or are
// cancelled; the rounds resume on the next start.
func (m *Manager) Shutdown(timeout time.Duration) {
	m.cancel()
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

func (m *Manager) load(dir string) (*round, error) {
	b, err := os.ReadFile(filepath.Join(dir, "round.json"))
	if err != nil {
		return nil, err
	}
	var meta api.Round
	if err := json.Unmarshal(b, &meta); err != nil {
		return nil, err
	}
	r := m.newRound(meta, dir)
	r.events, r.nextSeq = readEvents(dir)
	if meta.Active() {
		cfgDir := meta.Workspace
		if _, err := os.Stat(cfgDir); err != nil {
			cfgDir = meta.RepoPath
		}
		cfg, err := repoconfig.Load(cfgDir)
		if err != nil {
			reason := "nacre.yaml can't be read: " + err.Error()
			if r.meta.State != api.RoundPaused {
				r.meta.PausedFrom = meta.State
			}
			r.meta.State, r.meta.PauseReason = api.RoundPaused, reason
			r.meta.Stop = &api.Stop{Kind: api.StopConfig, Reason: reason, Choices: legacyStop(reason).Choices}
		}
		r.cfg = cfg
		if r.meta.State == api.RoundPaused && r.meta.Stop == nil {
			// Paused before stops existed.
			r.meta.Stop = legacyStop(r.meta.PauseReason)
		}
	}
	return r, nil
}

func (m *Manager) newRound(meta api.Round, dir string) *round {
	return &round{
		m: m, dir: dir, meta: meta, ops: map[string]bool{}, nextSeq: 1,
		kick: make(chan struct{}, 1), subs: map[chan struct{}]struct{}{},
		stallCancelled: map[string]bool{}, seenRun: map[string]string{},
	}
}

// CreateRequest starts a round.
type CreateRequest struct {
	RepoID string
	// Beads lists bead ids; Epic, if set instead, takes the epic's open
	// children.
	Beads []string
	Epic  string
	// Target overrides nacre.yaml's land.target.
	Target           string
	BudgetUSD        float64
	TimeLimitMinutes int
	SpecChangeID     string
	SpecRevision     string
}

// Create validates the request, makes the integration workspace and starts
// the round in the planning state.
func (m *Manager) Create(req CreateRequest) (api.Round, error) {
	ctx := m.ctx
	name, path, err := m.opts.Repo(req.RepoID)
	if err != nil {
		return api.Round{}, err
	}
	cfg, err := repoconfig.Load(path)
	if errors.Is(err, repoconfig.ErrNotFound) {
		return api.Round{}, conflict(name + " needs a " + repoconfig.FileName + " before rounds can run in it: it says which tests and build decide that work is done")
	}
	if err != nil {
		return api.Round{}, conflict(err.Error())
	}

	ids := req.Beads
	if req.Epic != "" {
		if ids, err = m.epicChildren(ctx, path, req.Epic); err != nil {
			return api.Round{}, err
		}
		if len(ids) == 0 {
			return api.Round{}, conflict(req.Epic + " has no open children to work on")
		}
	}
	ids = dedupe(ids)
	if len(ids) == 0 {
		return api.Round{}, conflict("name at least one bead")
	}
	beads := make([]api.RoundBead, 0, len(ids))
	contracts := map[string]api.SpecContract{}
	for _, id := range ids {
		raw, err := m.opts.Beads.Get(ctx, path, id)
		if err != nil {
			return api.Round{}, conflict(fmt.Sprintf("%s: %v", id, err))
		}
		is := parseIssue(raw)
		if is.Status == "closed" {
			return api.Round{}, conflict(id + " is closed")
		}
		b := api.RoundBead{ID: id, Title: is.Title, Type: is.Type, Priority: is.Priority, State: api.BeadWaiting}
		if is.hasLabel(pendingAmendmentLabel) {
			return api.Round{}, conflict(id + " is a pending amendment to its specification change: revise the change or make it a follow-on")
		}
		if m.opts.SpecContract != nil {
			c, err := m.opts.SpecContract(ctx, path, id)
			if err != nil {
				return api.Round{}, conflict(fmt.Sprintf("%s: %v", id, err))
			}
			if c != nil {
				if is.Type == "epic" {
					return api.Round{}, conflict("Choose this change's section beads; its epic is not an implementation task")
				}
				pinned := *c
				pinned.Artifacts = append([]api.SpecArtifact{}, c.Artifacts...)
				pinned.References = append([]string{}, c.References...)
				contracts[id] = pinned
				b.SpecChangeID, b.SpecRevision, b.SpecSectionID = c.ChangeID, c.Revision, c.SectionID
			} else if specLabelled(is.Labels) {
				return api.Round{}, conflict("The bead's specification mapping is missing; restore the workflow before starting it")
			}
		} else if specLabelled(is.Labels) {
			return api.Round{}, conflict("Specification workflow state is unavailable; restore it before starting this bead")
		}
		beads = append(beads, b)
	}
	if err := validateSpecRequest(beads, contracts, req.SpecChangeID, req.SpecRevision); err != nil {
		return api.Round{}, conflict(err.Error())
	}
	m.mu.Lock()
	for _, other := range m.rounds {
		if !other.meta.Active() || other.meta.RepoID != req.RepoID {
			continue
		}
		for _, b := range other.meta.Beads {
			for _, id := range ids {
				if b.ID == id {
					m.mu.Unlock()
					return api.Round{}, conflict(fmt.Sprintf("%s is already in round %s, which hasn't landed or been cancelled", id, other.meta.ID))
				}
			}
		}
	}
	m.mu.Unlock()

	target := req.Target
	if target == "" {
		target = cfg.Land.Target
	}
	id := newID()
	ws, err := m.opts.Workspaces.CreateIntegration(ctx, path, req.RepoID, id, target)
	if err != nil {
		return api.Round{}, fmt.Errorf("integration workspace: %w", err)
	}
	if len(contracts) > 0 {
		if err := installSpecSnapshot(ctx, ws.Path, contracts); err != nil {
			cleanupErr := m.opts.Workspaces.RemoveRound(ctx, path, req.RepoID, id, nil)
			if cleanupErr != nil {
				return api.Round{}, conflict(fmt.Sprintf("The approved specification could not be installed: %v. Its unused workspace remains at %s: %v", err, ws.Path, cleanupErr))
			}
			return api.Round{}, conflict("The approved specification could not be installed: " + err.Error())
		}
	}
	budget, limit := req.BudgetUSD, req.TimeLimitMinutes
	if budget <= 0 {
		budget = m.opts.BudgetUSD
	}
	if limit <= 0 {
		limit = m.opts.TimeLimitMinutes
	}
	now := stamp()
	meta := api.Round{
		ID: id, RepoID: req.RepoID, RepoName: name, RepoPath: path, Epic: req.Epic,
		State: api.RoundPlanning, Target: target, Base: ws.Base, Branch: ws.Branch, Workspace: ws.Path,
		Beads: beads, BudgetUSD: budget, TimeLimitMinutes: limit, Mode: m.opts.Mode, Model: m.opts.Model,
		AutoLand: cfg.Land.Auto && len(contracts) == 0, CreatedAt: now, UpdatedAt: now,
		SpecContracts: contracts,
	}
	dir := filepath.Join(m.opts.Dir, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return api.Round{}, err
	}
	r := m.newRound(meta, dir)
	r.cfg = cfg
	r.mu.Lock()
	r.event("state", "", fmt.Sprintf("Round started with %d beads from %s at %s", len(beads), target, short(ws.Base)))
	r.saveLocked()
	snap := clone(r.meta)
	r.mu.Unlock()

	for _, b := range beads {
		_ = m.opts.Beads.Update(ctx, path, b.ID, bd.UpdateParams{AddLabels: []string{"round:" + id}})
	}
	m.repoChanged(req.RepoID)

	m.mu.Lock()
	m.rounds[id] = r
	m.spawn(r)
	m.mu.Unlock()
	return snap, nil
}

func (m *Manager) epicChildren(ctx context.Context, repo, epic string) ([]string, error) {
	all, err := m.opts.Beads.List(ctx, repo)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, raw := range all {
		is := parseIssue(raw)
		if is.Parent == epic && is.Status != "closed" && is.Type != "epic" {
			ids = append(ids, is.ID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// Get returns a round and its log.
func (m *Manager) Get(id string) (api.Round, []api.RoundEvent, error) {
	r := m.get(id)
	if r == nil {
		return api.Round{}, nil, ErrNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return clone(r.meta), append([]api.RoundEvent{}, r.events...), nil
}

// List returns rounds, newest first, optionally for one repository.
// Busy includes terminal rounds whose cleanup or cancelled operation is still
// running. Check the counter after snapshots to cover terminal transitions.
func (m *Manager) Busy() bool { return len(m.List("", true)) > 0 || m.background.Load() > 0 }

func (m *Manager) List(repoID string, activeOnly bool) []api.Round {
	m.mu.Lock()
	list := make([]*round, 0, len(m.rounds))
	for _, r := range m.rounds {
		list = append(list, r)
	}
	m.mu.Unlock()
	out := []api.Round{}
	for _, r := range list {
		r.mu.Lock()
		meta := clone(r.meta)
		r.mu.Unlock()
		if (repoID == "" || meta.RepoID == repoID) && (!activeOnly || meta.Active()) {
			out = append(out, meta)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// Alive reports whether a round still needs its worktrees.
func (m *Manager) Alive(id string) bool {
	r := m.get(id)
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.meta.Active() || r.meta.State == api.RoundCancelled
}

func (m *Manager) get(id string) *round {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rounds[id]
}

// OnRun is told about every run change; rounds react to their own runs. It
// never blocks.
func (m *Manager) OnRun(run api.Run) {
	if run.RoundID == "" {
		return
	}
	if r := m.get(run.RoundID); r != nil {
		r.poke()
	}
}

// Subscription follows one round.
type Subscription struct {
	C     <-chan struct{}
	r     *round
	ch    chan struct{}
	close sync.Once
}

// Subscribe follows round id; C fires on every change.
func (m *Manager) Subscribe(id string) (*Subscription, error) {
	r := m.get(id)
	if r == nil {
		return nil, ErrNotFound
	}
	ch := make(chan struct{}, 1)
	r.mu.Lock()
	r.subs[ch] = struct{}{}
	r.mu.Unlock()
	return &Subscription{C: ch, r: r, ch: ch}, nil
}

// EventsAfter returns log entries with seq > after.
func (s *Subscription) EventsAfter(after int64) []api.RoundEvent {
	s.r.mu.Lock()
	defer s.r.mu.Unlock()
	var out []api.RoundEvent
	for _, e := range s.r.events {
		if e.Seq > after {
			out = append(out, e)
		}
	}
	return out
}

// Snapshot returns the round and a version that changes with it.
func (s *Subscription) Snapshot() (api.Round, uint64) {
	s.r.mu.Lock()
	defer s.r.mu.Unlock()
	return clone(s.r.meta), s.r.ver
}

// Close ends the subscription.
func (s *Subscription) Close() {
	s.close.Do(func() {
		s.r.mu.Lock()
		delete(s.r.subs, s.ch)
		s.r.mu.Unlock()
	})
}

// ---- persistence and notification (callers hold r.mu) ----

func (r *round) saveLocked() {
	r.meta.UpdatedAt = stamp()
	r.ver++
	b, _ := json.MarshalIndent(r.meta, "", "  ")
	tmp := filepath.Join(r.dir, "round.json.tmp")
	if err := os.WriteFile(tmp, b, 0o600); err == nil {
		_ = os.Rename(tmp, filepath.Join(r.dir, "round.json"))
	}
	for ch := range r.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	if r.m.opts.Notify != nil {
		r.m.opts.Notify("round", clone(r.meta))
	}
}

// event appends a log line. kind is "state", "plan", "question", "answer",
// "bead", "verify", "merge", "conflict", "gates", "build", "land", "note"
// or "error".
func (r *round) event(kind, bead, text string) api.RoundEvent {
	return r.eventWith(api.RoundEvent{Kind: kind, Bead: bead, Text: text})
}

func (r *round) eventWith(ev api.RoundEvent) api.RoundEvent {
	ev.Seq, ev.TS = r.nextSeq, time.Now().UTC().Format(time.RFC3339Nano)
	r.nextSeq++
	r.events = append(r.events, ev)
	if f, err := os.OpenFile(filepath.Join(r.dir, "events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		b, _ := json.Marshal(ev)
		f.Write(append(b, '\n'))
		f.Close()
	}
	return ev
}

func readEvents(dir string) ([]api.RoundEvent, int64) {
	f, err := os.Open(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		return nil, 1
	}
	defer f.Close()
	var evs []api.RoundEvent
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var ev api.RoundEvent
		if json.Unmarshal(sc.Bytes(), &ev) == nil {
			evs = append(evs, ev)
		}
	}
	next := int64(1)
	if n := len(evs); n > 0 {
		next = evs[n-1].Seq + 1
	}
	return evs, next
}

func (m *Manager) repoChanged(repoID string) {
	if m.opts.RepoChanged != nil {
		m.opts.RepoChanged(repoID)
	}
}

// ---- small helpers ----

// issue is the part of a bd issue rounds read.
type issue struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	Design             string   `json:"design"`
	AcceptanceCriteria string   `json:"acceptance_criteria"`
	Notes              string   `json:"notes"`
	Status             string   `json:"status"`
	Priority           int      `json:"priority"`
	Type               string   `json:"issue_type"`
	Labels             []string `json:"labels"`
	Parent             string   `json:"parent"`
	Dependencies       []struct {
		IssueID     string `json:"issue_id"`
		DependsOnID string `json:"depends_on_id"`
		Type        string `json:"type"`
	} `json:"dependencies"`
}

func parseIssue(raw api.Issue) issue {
	var is issue
	_ = json.Unmarshal(raw, &is)
	return is
}

func (is issue) hasLabel(l string) bool {
	for _, x := range is.Labels {
		if x == l {
			return true
		}
	}
	return false
}

func dedupe(ids []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func stamp() string { return time.Now().UTC().Format(time.RFC3339) }

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

const idAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

// newID is time-sortable: "rd" + base36 seconds + 3 random characters.
func newID() string {
	n := big.NewInt(time.Now().Unix())
	s := n.Text(36)
	var b strings.Builder
	for i := 0; i < 3; i++ {
		k, _ := rand.Int(rand.Reader, big.NewInt(int64(len(idAlphabet))))
		b.WriteByte(idAlphabet[k.Int64()])
	}
	return "rd" + s + b.String()
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// clone deep-copies a round, so callers outside r.mu never share its slices
// with the engine.
func clone(r api.Round) api.Round {
	c := r
	c.Beads = make([]api.RoundBead, len(r.Beads))
	for i, b := range r.Beads {
		b.After = append([]string(nil), b.After...)
		b.Needs = append([]string(nil), b.Needs...)
		if b.Verification != nil {
			v := *b.Verification
			b.Verification = &v
		}
		c.Beads[i] = b
	}
	c.Questions = append([]api.Question(nil), r.Questions...)
	c.RunIDs = append([]string(nil), r.RunIDs...)
	c.Artifacts = append([]string(nil), r.Artifacts...)
	if r.Plan != nil {
		p := *r.Plan
		p.Beads = append([]api.PlanBead(nil), r.Plan.Beads...)
		p.Findings = append([]api.Finding(nil), r.Plan.Findings...)
		c.Plan = &p
	}
	if r.Integration != nil {
		st := *r.Integration
		c.Integration = &st
	}
	if r.Stop != nil {
		st := *r.Stop
		st.Choices = append([]api.StopChoice(nil), r.Stop.Choices...)
		st.Attempted = append([]string(nil), r.Stop.Attempted...)
		st.Spend = append([]api.BeadSpend(nil), r.Stop.Spend...)
		if r.Stop.Diagnosis != nil {
			d := *r.Stop.Diagnosis
			st.Diagnosis = &d
		}
		c.Stop = &st
	}
	return c
}
