package sessions

import (
	"context"
	"log"
	"strings"
	"time"
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
	// poll. A session that drops out of it has ended.
	Sync func([]Session)

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
	return &Watcher{opts: opts, scan: newScanner(), links: map[string]linkResult{}}
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

// sessions resolves the registry into the sessions nacred can adopt.
func (w *Watcher) sessions() []Session {
	recs, err := ReadDir(w.opts.Dir)
	if err != nil {
		w.opts.Logger.Printf("sessions: read %s: %v", w.opts.Dir, err)
		return nil
	}
	recs = w.candidates(recs)
	starts := w.opts.Procs.StartTimes(pids(recs))

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

	out := make([]Session, 0, len(recs))
	seen := make(map[string]bool, len(recs))
	taken := map[string]bool{}
	for _, rec := range recs {
		if !live(rec, starts) {
			continue
		}
		repo, ok := resolveRepo(rec.CWD, repoList, w.opts.MainCheckout)
		if !ok {
			continue
		}
		seen[rec.SessionID] = true
		s := Session{Record: rec, Repo: repo, Transcript: transcriptPath(w.opts.Projects, rec.SessionID)}
		l, cached := w.links[rec.SessionID]
		if !cached || l.issueID == "" {
			l = w.link(rec, beads(repo.ID), s.Transcript, taken)
			w.links[rec.SessionID] = l
		}
		if l.issueID != "" {
			taken[l.issueID] = true
		}
		s.IssueID, s.IssueTitle, s.LinkedBy = l.issueID, l.title, l.by
		out = append(out, s)
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

// forget releases the per-session caches of sessions that are gone.
func (w *Watcher) forget(seen map[string]bool) {
	for sid := range w.links {
		if !seen[sid] {
			delete(w.links, sid)
			w.scan.forget(sid)
		}
	}
}

func pids(recs []Record) []int {
	out := make([]int, 0, len(recs))
	for _, r := range recs {
		out = append(out, r.PID)
	}
	return out
}
