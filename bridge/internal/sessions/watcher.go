package sessions

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// DefaultInterval is how often the session registry is polled when Options
// does not say.
const DefaultInterval = 10 * time.Second

// Options configures a Watcher. Repos and Sync are required; everything else
// has a working default.
type Options struct {
	// Dir is Claude Code's session registry (default: ~/.claude/sessions).
	Dir string
	// Projects is where Claude Code keeps transcripts (default:
	// ~/.claude/projects).
	Projects string
	// Interval is the poll period (default: DefaultInterval).
	Interval time.Duration
	// MaxEvents caps the events each transcript tail retains between polls
	// (default: claude.MaxTranscriptEvents).
	MaxEvents int

	// Repos lists the discovered beads repositories. It is called once per
	// poll, so it must not block on bd.
	Repos func() []Repo
	// Beads returns a repo's bd issues, or nil when they are unavailable. It
	// is only called for sessions that still need a bead link, so a steady
	// state of linked sessions costs no bd calls.
	Beads func(repoID string) []Bead
	// Owned reports whether a Claude Code session is one nacred launched
	// itself. Those are visible in the registry like any other and must never
	// be adopted a second time.
	Owned func(sessionID string) bool
	// OwnNamePrefix is the session-name prefix nacred gives its own sessions.
	// A record carrying it is never adopted even before its run has reported
	// a session id, which closes the race between launching a session and
	// seeing it appear in the registry.
	OwnNamePrefix string
	// Sync receives the complete set of live adopted sessions after every
	// poll, Claude Code and Codex together. A session that drops out of it
	// has ended.
	Sync func([]Session)

	// CodexHome is Codex's state directory (normally ~/.codex). When set, the
	// watcher also adopts Codex sessions from the rollout files under
	// CodexHome/sessions; "" leaves Codex out. See codex.go for the liveness
	// rule, which has no pid to go on.
	CodexHome string
	// CodexQuietAfter is how long a Codex rollout may go unwritten before its
	// session counts as ended (default: DefaultCodexQuietAfter).
	CodexQuietAfter time.Duration

	// Procs checks pid liveness (default: SystemProcs()).
	Procs ProcTable
	// MainCheckout resolves a worktree to its main checkout (default: git).
	MainCheckout func(dir string) string
	Logger       *log.Logger
}

// Watcher polls Claude Code's session registry and reports the live sessions
// nacred can adopt. It is not safe for concurrent use: Poll is called from
// Run's single goroutine.
type Watcher struct {
	opts Options
	scan *scanner
	// links caches a resolved bead per session id, so a session is linked
	// once rather than re-resolved on every poll.
	links map[string]linkResult
	// tails follows each live session's transcript, by session id, so each
	// poll reads only what was appended since the last one.
	tails map[string]*claude.TranscriptTail
	// codex holds what the Codex source remembers between polls.
	codex codexState
}

// New returns a Watcher. It does not read anything until Poll or Run is called.
func New(opts Options) *Watcher {
	if opts.Interval <= 0 {
		opts.Interval = DefaultInterval
	}
	if opts.Procs == nil {
		opts.Procs = SystemProcs()
	}
	if opts.MainCheckout == nil {
		opts.MainCheckout = gitMainCheckout
	}
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}
	if opts.CodexQuietAfter <= 0 {
		opts.CodexQuietAfter = DefaultCodexQuietAfter
	}
	return &Watcher{opts: opts, scan: newScanner(), links: map[string]linkResult{}, tails: map[string]*claude.TranscriptTail{}, codex: newCodexState()}
}

// Run polls immediately and then every Interval until ctx is done.
func (w *Watcher) Run(ctx context.Context) {
	t := time.NewTicker(w.opts.Interval)
	defer t.Stop()
	w.Poll()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.Poll()
		}
	}
}

// Poll reads the registry once and hands the live adopted sessions to Sync.
func (w *Watcher) Poll() {
	live := w.sessions()
	if w.opts.Sync != nil {
		w.opts.Sync(live)
	}
}

// sessions resolves the registry, and Codex's rollouts when enabled, into the
// sessions nacred can adopt.
func (w *Watcher) sessions() []Session {
	var repoList []Repo
	if w.opts.Repos != nil {
		repoList = w.opts.Repos()
	}
	// bd lists are fetched at most once per repo per poll, and only when a
	// session in it still needs a bead.
	beadCache := map[string][]Bead{}
	beads := func(repoID string) []Bead {
		if bs, ok := beadCache[repoID]; ok {
			return bs
		}
		var bs []Bead
		if w.opts.Beads != nil {
			bs = w.opts.Beads(repoID)
		}
		beadCache[repoID] = bs
		return bs
	}

	var out []Session
	seen := map[string]bool{}
	taken := map[string]bool{}
	// finish links a session that resolved to a repo and adds it to the set.
	// Its tail must already have been read: the same pass collects the bd
	// commands the link resolves from.
	finish := func(s Session) {
		sid := s.SessionID
		seen[sid] = true
		l, cached := w.links[sid]
		if !cached || l.issueID == "" {
			l = w.link(s.Record, beads(s.Repo.ID), s.Transcript, taken)
			w.links[sid] = l
		}
		if l.issueID != "" {
			taken[l.issueID] = true
			// A linked session is never re-linked, so its mentions are no
			// longer collected.
			w.scan.forget(sid)
		}
		s.IssueID, s.IssueTitle, s.LinkedBy = l.issueID, l.title, l.by
		out = append(out, s)
	}

	recs, err := ReadDir(w.opts.Dir)
	if err != nil {
		// Codex sessions can still be reported; Claude Code ones are not
		// known this poll, so they end.
		w.opts.Logger.Printf("sessions: read %s: %v", w.opts.Dir, err)
	}
	recs = w.candidates(recs)
	starts := w.opts.Procs.StartTimes(pids(recs))
	for _, rec := range recs {
		if !live(rec, starts) {
			continue
		}
		repo, ok := resolveRepo(rec.CWD, repoList, w.opts.MainCheckout)
		if !ok {
			continue
		}
		s := Session{Record: rec, Agent: api.AgentClaudeCode, Repo: repo, Transcript: transcriptPath(w.opts.Projects, rec.SessionID)}
		w.tail(&s, claude.TranscriptOptions{})
		finish(s)
	}
	if w.opts.CodexHome != "" {
		for _, s := range w.codexSessions(repoList, time.Now()) {
			if !seen[s.SessionID] {
				finish(s)
			}
		}
	}
	w.forget(seen)
	return out
}

// candidates drops the records that must never be adopted: nacred's own
// sessions and records too incomplete to resolve.
func (w *Watcher) candidates(recs []Record) []Record {
	out := recs[:0:0]
	for _, rec := range recs {
		switch {
		case rec.SessionID == "" || rec.CWD == "":
		case w.opts.OwnNamePrefix != "" && strings.HasPrefix(rec.Name, w.opts.OwnNamePrefix):
		case w.opts.Owned != nil && w.opts.Owned(rec.SessionID):
		default:
			out = append(out, rec)
		}
	}
	return out
}

// tail reads what s's transcript gained since the previous poll into s. A
// session whose transcript has not appeared yet simply has nothing to report.
//
// opts carries the agent-specific parts of the tail: Parse for a transcript
// that is not Claude Code's, and an extra Observe hook. MaxEvents and the bd
// scan are added here.
func (w *Watcher) tail(s *Session, opts claude.TranscriptOptions) {
	if s.Transcript == "" {
		return
	}
	t := w.tails[s.SessionID]
	if t == nil || t.Path() != s.Transcript {
		// A session that moved between worktrees can switch files; the new
		// one is read from the beginning, and so are the bd commands in it.
		sid := s.SessionID
		extra := opts.Observe
		opts.MaxEvents = w.opts.MaxEvents
		opts.Observe = func(p claude.TranscriptParsed) {
			if extra != nil {
				extra(p)
			}
			if w.links[sid].issueID == "" {
				w.scan.observe(sid, p)
			}
		}
		t = claude.NewTranscriptTail(s.Transcript, opts)
		w.tails[sid] = t
		w.scan.forget(sid)
	}
	fromStart := t.Offset() == 0
	mark := w.scan.mark(s.SessionID)
	d, err := t.Read()
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		w.opts.Logger.Printf("sessions: read transcript %s: %v", s.Transcript, err)
	}
	if d.Restarted {
		w.scan.restart(s.SessionID, mark)
	}
	s.Events = d.Events
	s.FromStart = fromStart || d.Restarted
	s.LastWrite = d.ModTime
	s.TranscriptState = t.State()
}

// forget releases the per-session caches of sessions that are gone.
func (w *Watcher) forget(seen map[string]bool) {
	for sid := range w.links {
		if !seen[sid] {
			delete(w.links, sid)
			w.scan.forget(sid)
		}
	}
	for sid := range w.tails {
		if !seen[sid] {
			delete(w.tails, sid)
		}
	}
	w.codex.forget(seen)
}

func pids(recs []Record) []int {
	out := make([]int, 0, len(recs))
	for _, r := range recs {
		out = append(out, r.PID)
	}
	return out
}
