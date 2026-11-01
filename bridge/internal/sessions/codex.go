package sessions

// The Codex source. Codex keeps no session registry: there is no pid, no
// process start time and no record that is removed when a session ends (see
// internal/codex/rollout.go for the format). What it does keep is one rollout
// file per session, appended to as the session works, so a Codex session is
// adopted from its rollout and its liveness is read from the rollout too:
//
//   - A session is live while its rollout has been written within
//     Options.CodexQuietAfter (nacred uses twice adopt_idle_minutes, so a
//     session is first shown idle and stale, then ended). A TUI or desktop
//     session left open for longer than that ends; if its human comes back
//     and the rollout grows again, the run is picked up again where it was.
//   - A `codex exec` session runs one turn and exits, so it ends as soon as
//     its turn closes (task_complete or turn_aborted), without waiting out the
//     quiet window: the poll that sees the turn close still reports it, with
//     its last events, and the next one does not. One that has already
//     finished when first seen is not adopted at all.
//   - Between turns a live interactive session is reported idle (Record.Status
//     "idle"), the way Claude Code marks a parked session, so the run manager's
//     DeriveSessionStatus treats both agents alike.
//
// A session is attributed to a repo by session_meta's cwd or, failing that, by
// the directory its commands run in: the desktop app starts most threads in
// the home directory and runs each command in the project the user picked.
// Such a rollout is read as it grows (codexState.probes) until a command runs
// in a discovered repo; the session is then adopted, from the start of its
// rollout, and stays with that repo while it lives.
//
// Helper threads Codex spawns for another thread (approval reviewers and other
// subagents) are never adopted, and nor is a session nacred dispatched itself.

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
	"github.com/ryderdavid/nacre/bridge/internal/codex"
)

// DefaultCodexQuietAfter is how long a Codex rollout may go unwritten before
// its session counts as ended, when Options does not say.
const DefaultCodexQuietAfter = 30 * time.Minute

// codexScanDays is how many day directories back the rollout scan looks. A
// rollout stays in the directory of the day its session started, so this
// bounds how old a session may be and still be found; one already being
// followed is kept however old its directory is.
const codexScanDays = 7

// codexSettle is how old a rollout must be before it is considered. nacred's
// own `codex exec` runs report their thread id from the first line Codex
// prints, the moment the rollout is created; waiting a few seconds lets
// Options.Owned know the id before the rollout could be mistaken for an
// agent's own session.
const codexSettle = 15 * time.Second

// rolloutNameRe matches a rollout file name.
var rolloutNameRe = regexp.MustCompile(`^rollout-.*\.jsonl$`)

// codexState is what the Codex source remembers between polls.
type codexState struct {
	// meta caches each rollout's session_meta by path; it never changes.
	meta map[string]codex.RolloutMeta
	// turns follows each live session's turn lifecycle, by thread id.
	turns map[string]*codexTurn
	// done holds exec rollouts whose turn has closed, with the mtime at which
	// that was seen, so a finished session is not re-read every poll.
	done map[string]time.Time
	// paths are the rollouts of the sessions reported by the last poll.
	paths map[string]string
	// repos holds the repo of each reported session that was attributed by
	// where its commands ran rather than by its starting cwd.
	repos map[string]Repo
	// probes follow live rollouts that started outside every repo, by path,
	// for the directories their commands run in.
	probes map[string]*codexProbe
}

// codexProbe reads a rollout that started outside every repo for the
// directories its commands (and turns) ran in.
type codexProbe struct {
	tail *claude.TranscriptTail
	// fresh are the directories first seen since the previous poll, oldest
	// first; tried are all those seen so far, so each is resolved once.
	fresh []string
	tried map[string]bool
}

// maxCodexProbeDirs bounds how many distinct directories a probe resolves.
const maxCodexProbeDirs = 256

// codexTurn is where a session's turn lifecycle stands.
type codexTurn struct {
	started bool // a turn has begun at some point
	open    bool // the latest turn has not finished
}

func newCodexState() codexState {
	return codexState{meta: map[string]codex.RolloutMeta{}, turns: map[string]*codexTurn{}, done: map[string]time.Time{}, paths: map[string]string{}, repos: map[string]Repo{}, probes: map[string]*codexProbe{}}
}

// forget drops the turn state of sessions no longer reported.
func (c *codexState) forget(seen map[string]bool) {
	for sid := range c.turns {
		if !seen[sid] {
			delete(c.turns, sid)
		}
	}
	for sid := range c.paths {
		if !seen[sid] {
			delete(c.paths, sid)
		}
	}
	for sid := range c.repos {
		if !seen[sid] {
			delete(c.repos, sid)
		}
	}
}

// codexSessions returns the live Codex sessions in discovered repos, with
// their rollouts tailed. Linking is left to the caller, as for Claude Code.
func (w *Watcher) codexSessions(repoList []Repo, now time.Time) []Session {
	var index map[string]string // thread names, read only if something is live
	var out []Session
	probed := map[string]bool{}
	for _, path := range w.rolloutCandidates(now) {
		fi, err := os.Stat(path)
		if err != nil || now.Sub(fi.ModTime()) > w.opts.CodexQuietAfter {
			continue
		}
		if at, ok := w.codex.done[path]; ok && at.Equal(fi.ModTime()) {
			continue
		}
		m, ok := w.rolloutMeta(path)
		if !ok || m.Subagent || m.ID == "" || m.CWD == "" {
			continue
		}
		if !m.Started.IsZero() && now.Sub(m.Started) < codexSettle {
			continue
		}
		if w.opts.Owned != nil && w.opts.Owned(m.ID) {
			continue
		}
		repo, ok := resolveRepo(m.CWD, repoList, w.opts.MainCheckout)
		if !ok {
			repo, ok = w.codex.repos[m.ID]
		}
		if !ok {
			probed[path] = true
			if repo, ok = w.codexWorkRepo(path, repoList); !ok {
				continue
			}
			delete(w.codex.probes, path)
			w.codex.repos[m.ID] = repo
		}

		turn := w.codex.turns[m.ID]
		if turn == nil {
			turn = &codexTurn{}
			w.codex.turns[m.ID] = turn
		}
		s := Session{
			Record: Record{
				SessionID: m.ID, CWD: m.CWD, Version: m.Version,
				Entrypoint: m.Originator, Kind: "interactive",
				StartedAt: m.Started.UnixMilli(), UpdatedAt: fi.ModTime().UnixMilli(),
				File: path,
			},
			Agent: api.AgentCodex, Repo: repo, Transcript: path,
		}
		if m.Originator == codex.OriginatorExec {
			s.Kind = "exec"
		}
		w.tail(&s, claude.TranscriptOptions{
			Parse:   codex.ParseRolloutEntry,
			Observe: turn.observe,
		})
		if st := s.TranscriptState.CWD; st != "" {
			if _, attributed := w.codex.repos[m.ID]; !attributed {
				s.CWD = st
			} else if r, ok := resolveRepo(st, repoList, w.opts.MainCheckout); ok && r.ID == repo.ID {
				s.CWD = st
			} else {
				// A desktop thread's turns start back in the home directory;
				// it is shown working where its commands run.
				s.CWD = repo.Path
			}
		}
		if s.Kind == "exec" && turn.started && !turn.open {
			// codex exec has finished its one turn and exited. A session that
			// was being reported is reported once more, so the events of this
			// last read (the closing message) reach the run, and from the next
			// poll on it is skipped: ended. One first seen already finished is
			// never adopted, as a Claude Code session with a dead pid is not.
			w.codex.done[path] = fi.ModTime()
			if w.codex.paths[m.ID] == "" {
				continue
			}
		}
		if !turn.open {
			s.Status = claude.ActivityIdle
		}
		if index == nil {
			index, _ = codex.ReadSessionIndex(filepath.Join(w.opts.CodexHome, "session_index.jsonl"))
		}
		s.Name = index[m.ID]
		w.codex.paths[m.ID] = path
		out = append(out, s)
	}
	for path := range w.codex.probes {
		if !probed[path] {
			delete(w.codex.probes, path)
		}
	}
	w.pruneCodexCaches(now)
	return out
}

// codexWorkRepo reads what a rollout gained since the previous poll and
// returns the discovered repo of the latest directory a command (or turn) ran
// in, if any. Each directory is resolved once, the newest first, so a command
// run in a repo is found even if later entries moved back to the home
// directory before this poll.
func (w *Watcher) codexWorkRepo(path string, repoList []Repo) (Repo, bool) {
	pr := w.codex.probes[path]
	if pr == nil {
		pr = &codexProbe{tried: map[string]bool{}}
		pr.tail = claude.NewTranscriptTail(path, claude.TranscriptOptions{
			Parse: codex.ParseRolloutEntry, MaxEvents: 1,
			Observe: func(p claude.TranscriptParsed) {
				if p.CWD != "" && !pr.tried[p.CWD] && len(pr.tried) < maxCodexProbeDirs {
					pr.tried[p.CWD] = true
					pr.fresh = append(pr.fresh, p.CWD)
				}
			},
		})
		w.codex.probes[path] = pr
	}
	if _, err := pr.tail.Read(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		w.opts.Logger.Printf("sessions: read rollout %s: %v", path, err)
	}
	fresh := pr.fresh
	pr.fresh = nil
	for i := len(fresh) - 1; i >= 0; i-- {
		if r, ok := resolveRepo(fresh[i], repoList, w.opts.MainCheckout); ok {
			return r, true
		}
	}
	return Repo{}, false
}

// DefaultCodexHome returns Codex's state directory.
func DefaultCodexHome(home string) string { return filepath.Join(home, ".codex") }

// observe follows the turn lifecycle through a rollout's entries.
func (t *codexTurn) observe(p claude.TranscriptParsed) {
	switch p.Type {
	case codex.RolloutTurnStarted:
		t.started, t.open = true, true
	case codex.RolloutTurnComplete, codex.RolloutTurnAborted:
		t.open = false
	}
}

// rolloutCandidates lists the rollouts under the last codexScanDays day
// directories (local dates, as Codex names them), plus those of the sessions
// already being followed.
func (w *Watcher) rolloutCandidates(now time.Time) []string {
	root := filepath.Join(w.opts.CodexHome, "sessions")
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for d := 0; d < codexScanDays; d++ {
		day := now.AddDate(0, 0, -d)
		dir := filepath.Join(root, day.Format("2006"), day.Format("01"), day.Format("02"))
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && rolloutNameRe.MatchString(e.Name()) {
				add(filepath.Join(dir, e.Name()))
			}
		}
	}
	for _, p := range w.codex.paths {
		add(p)
	}
	return out
}

// rolloutMeta returns a rollout's session_meta, reading it once per file.
func (w *Watcher) rolloutMeta(path string) (codex.RolloutMeta, bool) {
	if m, ok := w.codex.meta[path]; ok {
		return m, true
	}
	m, err := codex.ReadRolloutMeta(path)
	if err != nil {
		// A rollout is created with its session_meta, so an unreadable first
		// line is a file mid-write; it is tried again next poll.
		return codex.RolloutMeta{}, false
	}
	w.codex.meta[path] = m
	return m, true
}

// pruneCodexCaches drops cached metadata and finished-session marks for
// rollouts that have aged out of the scan.
func (w *Watcher) pruneCodexCaches(now time.Time) {
	for path, at := range w.codex.done {
		if now.Sub(at) > w.opts.CodexQuietAfter {
			delete(w.codex.done, path)
		}
	}
	if len(w.codex.meta) < 4096 {
		return
	}
	for path, m := range w.codex.meta {
		if now.Sub(m.Started) > codexScanDays*24*time.Hour {
			delete(w.codex.meta, path)
		}
	}
}
