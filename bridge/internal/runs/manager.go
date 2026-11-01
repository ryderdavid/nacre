// Package runs manages agent runs: queueing under a concurrency limit,
// launching `claude -p` (or, for Agent "codex", `codex exec --json`)
// processes, recording their output as RunEvents, follow-ups via --resume, cancellation, and persistence so run
// history survives restarts.
package runs

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
	"github.com/ryderdavid/nacre/bridge/internal/codex"
	"github.com/ryderdavid/nacre/bridge/internal/peer"
)

// Errors returned by Manager methods.
var (
	ErrNotFound = errors.New("unknown run")
	ErrConflict = errors.New("conflict")
)

// conflict wraps ErrConflict with a specific message.
type conflict string

func (c conflict) Error() string { return string(c) }
func (c conflict) Unwrap() error { return ErrConflict }

// Options configures a Manager.
type Options struct {
	Dir        string // run history directory
	ClaudePath string
	// CodexPath is the Codex CLI, for runs started with Agent api.AgentCodex.
	CodexPath     string
	MaxConcurrent int
	Keep          int // number of runs to keep on disk (default 200)
	// Notify publishes a global notification (event "run" with an api.Run).
	// It may be called with internal locks held, so it must not block or
	// call back into the Manager.
	Notify func(event string, data any)
	// RepoChanged is called when a run finishes (Claude may have edited beads).
	RepoChanged func(repoID string)
	// AdoptIdleAfter is how long an adopted session may go without writing
	// its transcript before it counts as idle and stale (default
	// DefaultAdoptIdleAfter).
	AdoptIdleAfter time.Duration
	// StallAfter is how long a running claude may print nothing before the
	// run is flagged api.Run.Stalled. 0 turns the check off.
	StallAfter time.Duration
	// Live delivers follow-ups into live adopted Claude Code sessions over
	// their messaging socket (*peer.Client). nil leaves every adopted run
	// read-only. Its inbound frames go to Manager.PeerFrame.
	Live LiveSender
	// LiveHost is the pidDomain of sessions on this computer (default
	// peer.HostDomain()).
	LiveHost string
	// LiveConfirmAfter is how long a live send may go without a receipt or a
	// transcript entry before it is reported unconfirmed (default
	// DefaultLiveConfirmAfter).
	LiveConfirmAfter time.Duration
	Logger           *log.Logger
}

// StartRequest describes a new run.
type StartRequest struct {
	SpecChangeID, SpecRevision, SpecSectionID string
	RepoID, RepoName, RepoPath                string
	IssueID, IssueTitle                       string
	Mode, Model                               string
	Effort                                    string // "" = the agent's default
	Worktree                                  bool
	Prompt                                    string
	// Agent is api.AgentCodex to run the bead with the Codex CLI; "" (or
	// api.AgentClaudeCode) runs it with Claude Code.
	Agent string
	// Round fields: the run works in Workspace (a worktree nacred made, on
	// Branch from Base) instead of the repository, for round RoundID.
	RoundID, Kind           string
	Branch, Workspace, Base string
	MaxBudgetUSD            float64
	Env                     []string
}

// Followup is a message to resume a run's session with.
type Followup struct {
	Message string
	// Mode "" keeps the run's current mode.
	Mode         string
	MaxBudgetUSD float64
	Env          []string
}

// Filter selects runs in List.
type Filter struct {
	RepoID, IssueID string
	ActiveOnly      bool
	// IncludeAdopted adds adopted runs (agent-owned Claude Code sessions) to
	// the result. They are excluded by default: a client that does not know
	// about them would offer cancel and follow-up for a session nacred cannot
	// control, and would count it as work nacred is doing.
	IncludeAdopted bool
	// AdoptedOnly keeps only adopted runs. With IncludeAdopted it is how the
	// adopted sessions are counted separately from the runs nacred owns.
	AdoptedOnly bool
	// IncludeStale keeps adopted runs whose session has gone quiet for longer
	// than the idle window (api.Run.Stale). They are left out by default: a
	// session left open is not a job in progress, so the runs list and the
	// adopted count skip it, while a bead's own view still wants to show it.
	IncludeStale bool
}

// adoptedRefusal prefixes the conflicts returned for operations that are not
// safe against a session nacred does not own.
const adoptedRefusal = "run is an adopted Claude Code session that nacred does not own"

// Manager is safe for concurrent use.
type Manager struct {
	opts Options

	mu      sync.Mutex
	runs    map[string]*run
	queue   []*run
	running int
	closing bool
	wg      sync.WaitGroup // running processes
	// adopted indexes adopted runs by Claude Code session id, so a registry
	// rescan or a bridge restart finds the existing run instead of adopting
	// the same session a second time.
	adopted map[string]string
	// peerMsgs maps the msg_id of each live send (and of its idle
	// subscription) to the run and message it belongs to, so receipts
	// arriving at the inbox find their run.
	peerMsgs map[string]peerRef
	stop     chan struct{} // closed by Shutdown; ends the stall watch
}

// New loads run history from opts.Dir. Runs that were active when the bridge
// stopped are marked failed ("bridge restarted") — except adopted runs, whose
// processes nacred never owned: a restart says nothing about them, so they are
// left active for SyncAdopted to re-resolve against the live session registry.
func New(opts Options) (*Manager, error) {
	if opts.MaxConcurrent <= 0 {
		opts.MaxConcurrent = 3
	}
	if opts.Keep <= 0 {
		opts.Keep = 200
	}
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}
	if opts.AdoptIdleAfter <= 0 {
		opts.AdoptIdleAfter = DefaultAdoptIdleAfter
	}
	if opts.LiveHost == "" {
		opts.LiveHost = peer.HostDomain()
	}
	if opts.LiveConfirmAfter <= 0 {
		opts.LiveConfirmAfter = DefaultLiveConfirmAfter
	}
	if err := os.MkdirAll(opts.Dir, 0o700); err != nil {
		return nil, err
	}
	m := &Manager{opts: opts, runs: map[string]*run{}, adopted: map[string]string{}, peerMsgs: map[string]peerRef{}, stop: make(chan struct{})}
	ids, err := listRunDirs(opts.Dir)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		dir := filepath.Join(opts.Dir, id)
		meta, err := readMeta(dir)
		if err != nil || meta.ID != id {
			opts.Logger.Printf("runs: skipping %s: unreadable meta (%v)", id, err)
			continue
		}
		r := newRun(meta, dir)
		switch {
		case meta.Adopted():
			if meta.SessionID != "" {
				m.adopted[meta.SessionID] = id
			}
			// Re-derived from the live record on the first poll.
			r.meta.CanSendLive = false
		case meta.Active():
			r.mu.Lock()
			r.ensureLoaded()
			r.meta.Error = "bridge restarted"
			r.meta.EndedAt = now()
			r.setStatus(api.StatusFailed)
			r.save()
			r.mu.Unlock()
		}
		m.runs[id] = r
	}
	m.prune()
	if opts.StallAfter > 0 {
		go m.watchStalls()
	}
	return m, nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// Start creates and enqueues a run.
func (m *Manager) Start(req StartRequest) (api.Run, error) {
	t := time.Now()
	id := newID(t)
	meta := api.Run{
		SpecChangeID: req.SpecChangeID, SpecRevision: req.SpecRevision, SpecSectionID: req.SpecSectionID,
		ID: id, RepoID: req.RepoID, RepoName: req.RepoName, RepoPath: req.RepoPath,
		IssueID: req.IssueID, IssueTitle: req.IssueTitle, Status: api.StatusQueued,
		Mode: req.Mode, Model: req.Model, Effort: req.Effort, Worktree: req.Worktree,
		CreatedAt: t.UTC().Format(time.RFC3339), Turns: 1,
		RoundID: req.RoundID, Kind: req.Kind, Branch: req.Branch, Workspace: req.Workspace, Base: req.Base,
	}
	if req.Agent == api.AgentCodex {
		meta.Agent = api.AgentCodex
	}
	r := newRun(meta, filepath.Join(m.opts.Dir, id))
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return api.Run{}, fmt.Errorf("persist run: %w", err)
	}
	r.loaded = true
	r.pending = &claude.Turn{
		Prompt: req.Prompt, Mode: req.Mode, Model: req.Model, Effort: req.Effort,
		Worktree: req.Worktree && req.Workspace == "", Name: claude.NamePrefix + req.IssueID,
		MaxBudgetUSD: req.MaxBudgetUSD, Env: req.Env,
	}
	r.mu.Lock()
	r.addEvent(api.RunEvent{Kind: api.KindStatus, Status: api.StatusQueued})
	r.addEvent(api.RunEvent{Kind: api.KindUser, Text: req.Prompt})
	err := r.save()
	snap := r.meta
	r.mu.Unlock()
	if err != nil {
		return api.Run{}, fmt.Errorf("persist run: %w", err)
	}

	m.mu.Lock()
	m.runs[id] = r
	m.queue = append(m.queue, r)
	m.notify(snap) // before schedule, so "queued" is published before "running"
	m.schedule()
	m.mu.Unlock()
	m.prune()
	return m.snapshot(r), nil
}

// Followup sends message to the run's Claude session (claude --resume).
// mode "" keeps the run's current mode.
func (m *Manager) Followup(id, message, mode string) (api.Run, error) {
	return m.FollowupWith(id, Followup{Message: message, Mode: mode})
}

// FollowupWith resumes the run's session with f.
func (m *Manager) FollowupWith(id string, f Followup) (api.Run, error) {
	message, mode := f.Message, f.Mode
	m.mu.Lock()
	r := m.runs[id]
	if r == nil {
		m.mu.Unlock()
		return api.Run{}, ErrNotFound
	}
	r.mu.Lock()
	switch {
	case r.meta.Adopted():
		r.mu.Unlock()
		m.mu.Unlock()
		return api.Run{}, conflict(adoptedRefusal + "; resuming it would fork the agent's session")
	case r.meta.Active():
		r.mu.Unlock()
		m.mu.Unlock()
		return api.Run{}, conflict("run is " + r.meta.Status + "; wait for it to finish before sending a follow-up")
	case r.meta.SessionID == "":
		r.mu.Unlock()
		m.mu.Unlock()
		return api.Run{}, conflict("run has no session to resume")
	}
	r.ensureLoaded()
	if mode != "" {
		r.meta.Mode = mode
	}
	r.meta.Turns++
	r.meta.Error = ""
	r.meta.EndedAt = ""
	r.cancelReq = false
	r.pending = &claude.Turn{Prompt: message, Mode: r.meta.Mode, Model: r.meta.Model, Effort: r.meta.Effort, Resume: r.meta.SessionID,
		MaxBudgetUSD: f.MaxBudgetUSD, Env: f.Env}
	r.setStatus(api.StatusQueued)
	r.addEvent(api.RunEvent{Kind: api.KindUser, Text: message})
	_ = r.save()
	snap := r.meta
	r.mu.Unlock()
	m.queue = append(m.queue, r)
	m.notify(snap)
	m.schedule()
	m.mu.Unlock()
	return m.snapshot(r), nil
}

// Cancel stops a queued or running run. Cancelling a finished run is a no-op
// that returns the run unchanged.
func (m *Manager) Cancel(id string) (api.Run, error) {
	m.mu.Lock()
	r := m.runs[id]
	if r == nil {
		m.mu.Unlock()
		return api.Run{}, ErrNotFound
	}
	r.mu.Lock()
	if r.meta.Adopted() {
		r.mu.Unlock()
		m.mu.Unlock()
		return api.Run{}, conflict(adoptedRefusal + "; signalling it would kill work nacred did not start")
	}
	switch r.meta.Status {
	case api.StatusQueued:
		for i, q := range m.queue {
			if q == r {
				m.queue = append(m.queue[:i], m.queue[i+1:]...)
				break
			}
		}
		r.pending = nil
		r.meta.Error = "cancelled"
		r.meta.EndedAt = now()
		r.setStatus(api.StatusCancelled)
		_ = r.save()
		snap := r.meta
		r.mu.Unlock()
		m.mu.Unlock()
		m.notify(snap)
		return snap, nil
	case api.StatusRunning:
		r.cancelReq = true
		r.terminate()
	}
	snap := r.meta
	r.mu.Unlock()
	m.mu.Unlock()
	return snap, nil
}

// SetVerification records nacred's check of a run's work and publishes it.
func (m *Manager) SetVerification(id string, v *api.Verification) error {
	r := m.get(id)
	if r == nil {
		return ErrNotFound
	}
	r.mu.Lock()
	r.meta.Verification = v
	r.changed()
	err := r.save()
	snap := r.meta
	r.mu.Unlock()
	m.notify(snap)
	return err
}

// Get returns a run and all its events.
func (m *Manager) Get(id string) (api.Run, []api.RunEvent, error) {
	r := m.get(id)
	if r == nil {
		return api.Run{}, nil, ErrNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureLoaded()
	return r.meta, append([]api.RunEvent{}, r.events...), nil
}

// Run returns a run's metadata.
func (m *Manager) Run(id string) (api.Run, error) {
	r := m.get(id)
	if r == nil {
		return api.Run{}, ErrNotFound
	}
	return m.snapshot(r), nil
}

// List returns runs matching f, newest first.
func (m *Manager) List(f Filter) []api.Run {
	m.mu.Lock()
	all := make([]*run, 0, len(m.runs))
	for _, r := range m.runs {
		all = append(all, r)
	}
	m.mu.Unlock()
	out := []api.Run{}
	for _, r := range all {
		s := m.snapshot(r)
		if (f.RepoID != "" && s.RepoID != f.RepoID) || (f.IssueID != "" && s.IssueID != f.IssueID) || (f.ActiveOnly && !s.Active()) {
			continue
		}
		if s.Adopted() && !f.IncludeAdopted {
			continue
		}
		if f.AdoptedOnly && !s.Adopted() {
			continue
		}
		if s.Stale && !f.IncludeStale {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// ActiveCount returns the number of queued or running runs for repoID
// ("" = all). Adopted runs are not counted: they occupy no concurrency slot
// and are not work nacred is doing.
func (m *Manager) ActiveCount(repoID string) int {
	return len(m.List(Filter{RepoID: repoID, ActiveOnly: true}))
}

// AdoptedCount returns the number of live adopted sessions for repoID
// ("" = all): agent-owned work nacred is watching but did not start, which is
// exactly what ActiveCount leaves out. The two together are the whole picture
// of what is running on this computer. Stale sessions, alive but quiet past
// the idle window, are not counted: they are left open, not working.
func (m *Manager) AdoptedCount(repoID string) int {
	return len(m.List(Filter{RepoID: repoID, ActiveOnly: true, IncludeAdopted: true, AdoptedOnly: true}))
}

// Subscription delivers change signals for one run.
type Subscription struct {
	// C receives a value whenever new events or metadata are available.
	C  <-chan struct{}
	ch chan struct{}
	r  *run
}

// Subscribe registers for change signals on run id.
func (m *Manager) Subscribe(id string) (*Subscription, error) {
	r := m.get(id)
	if r == nil {
		return nil, ErrNotFound
	}
	ch := make(chan struct{}, 1)
	r.mu.Lock()
	r.ensureLoaded()
	r.subs[ch] = struct{}{}
	r.mu.Unlock()
	return &Subscription{C: ch, ch: ch, r: r}, nil
}

// Subscribers returns the number of open subscriptions on run id (diagnostics/tests).
func (m *Manager) Subscribers(id string) int {
	r := m.get(id)
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.subs)
}

// EventsAfter returns stored events with seq > after.
func (s *Subscription) EventsAfter(after int64) []api.RunEvent {
	s.r.mu.Lock()
	defer s.r.mu.Unlock()
	evs := s.r.events
	i := sort.Search(len(evs), func(i int) bool { return evs[i].Seq > after })
	return append([]api.RunEvent(nil), evs[i:]...)
}

// Meta returns the run metadata and its version (bumped on every significant change).
func (s *Subscription) Meta() (api.Run, uint64) {
	s.r.mu.Lock()
	defer s.r.mu.Unlock()
	return s.r.meta, s.r.metaVer
}

// Close unregisters the subscription.
func (s *Subscription) Close() {
	s.r.mu.Lock()
	delete(s.r.subs, s.ch)
	s.r.mu.Unlock()
}

// Shutdown terminates running processes and waits (bounded) for them to exit.
func (m *Manager) Shutdown(timeout time.Duration) {
	m.mu.Lock()
	if !m.closing {
		close(m.stop)
	}
	m.closing = true
	m.queue = nil
	for _, r := range m.runs {
		r.mu.Lock()
		if r.meta.Status == api.StatusRunning {
			r.shutdownReq = true
			r.terminate()
		}
		r.mu.Unlock()
	}
	m.mu.Unlock()
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

func (m *Manager) get(id string) *run {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runs[id]
}

func (m *Manager) snapshot(r *run) api.Run {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.meta
}

func (m *Manager) notify(meta api.Run) {
	if m.opts.Notify != nil {
		m.opts.Notify("run", meta)
	}
}

// schedule starts queued runs while below the concurrency limit. Caller holds m.mu.
func (m *Manager) schedule() {
	for !m.closing && m.running < m.opts.MaxConcurrent && len(m.queue) > 0 {
		r := m.queue[0]
		m.queue = m.queue[1:]
		r.mu.Lock()
		turn := r.pending
		r.pending = nil
		if turn == nil || r.meta.Status != api.StatusQueued {
			r.mu.Unlock()
			continue
		}
		cwd := r.meta.RepoPath
		if r.meta.Workspace != "" {
			cwd = r.meta.Workspace
		}
		if turn.Resume != "" && r.meta.SessionCWD != "" {
			if fi, err := os.Stat(r.meta.SessionCWD); err == nil && fi.IsDir() {
				cwd = r.meta.SessionCWD
			}
		}
		if r.meta.StartedAt == "" {
			r.meta.StartedAt = now()
		}
		r.turnResult = nil
		r.stderrTail = nil
		r.setStatus(api.StatusRunning)
		_ = r.save()
		snap := r.meta
		agent := r.meta.Agent
		r.mu.Unlock()
		m.running++
		m.wg.Add(1)
		go m.execute(r, agent, *turn, cwd)
		m.notify(snap)
	}
}

// prune deletes the oldest finished runs beyond opts.Keep.
func (m *Manager) prune() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.runs) <= m.opts.Keep {
		return
	}
	ids := make([]string, 0, len(m.runs))
	for id := range m.runs {
		ids = append(ids, id)
	}
	sort.Strings(ids) // oldest first (ids are time-sortable)
	excess := len(m.runs) - m.opts.Keep
	for _, id := range ids {
		if excess == 0 {
			break
		}
		r := m.runs[id]
		r.mu.Lock()
		active := r.meta.Active()
		dir := r.dir
		r.mu.Unlock()
		if active {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			m.opts.Logger.Printf("runs: prune %s: %v", id, err)
			continue
		}
		delete(m.runs, id)
		excess--
	}
	for sid, id := range m.adopted {
		if m.runs[id] == nil {
			delete(m.adopted, sid)
		}
	}
}

// shellQuote single-quotes s for a POSIX shell.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func resumeCommand(meta api.Run) string {
	if meta.SessionID == "" {
		return ""
	}
	dir := meta.SessionCWD
	if dir == "" {
		dir = meta.RepoPath
	}
	if meta.Agent == api.AgentCodex {
		return codex.ResumeCommand(dir, meta.SessionID)
	}
	return fmt.Sprintf("cd %s && claude --resume %s", shellQuote(dir), meta.SessionID)
}
