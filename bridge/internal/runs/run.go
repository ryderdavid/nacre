package runs

import (
	"sync"
	"syscall"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
	"github.com/ryderdavid/nacre/bridge/internal/peer"
)

// killGrace is how long a cancelled process group gets between SIGTERM and SIGKILL.
const killGrace = 5 * time.Second

// run is the in-memory state of one run. All fields are guarded by mu, which
// is never held while acquiring Manager.mu.
type run struct {
	mu   sync.Mutex
	meta api.Run
	dir  string

	events    []api.RunEvent // loaded lazily for runs from previous sessions
	loaded    bool
	nextSeq   int64
	compactAt int64 // adopted runs: compact events.jsonl once it passes this size
	metaVer   uint64
	subs      map[chan struct{}]struct{}

	pending *claude.Turn // turn waiting in the queue
	pgid    int          // process group of the running agent, 0 if none
	// stopSig is what terminate sends first (0 = SIGTERM). Codex runs each
	// command in a process group of its own, out of reach of a signal to
	// pgid, and only kills them itself on SIGINT.
	stopSig     syscall.Signal
	cancelReq   bool // user asked to cancel the current turn
	shutdownReq bool // bridge is shutting down
	turnResult  *claude.Result
	stderrTail  []string
	lastOutput  time.Time // when the running claude last printed anything
	// peer is the adopted session's messaging socket as its record last
	// described it, nil unless it passes the record half of the live-send
	// guard. In memory only: the first poll after a restart fills it in.
	peer *peer.Target
}

func newRun(meta api.Run, dir string) *run {
	// Derived, not stored: a run loaded from a meta file written before the
	// capability flags existed still reports them correctly.
	meta.SetCapabilities()
	return &run{meta: meta, dir: dir, nextSeq: 1, subs: map[chan struct{}]struct{}{}}
}

// ensureLoaded reads events from disk on first use. An adopted run holds only
// its newest maxAdoptedEvents, and a log left oversized (written before
// compaction existed) is compacted straight away.
func (r *run) ensureLoaded() {
	if r.loaded {
		return
	}
	r.loaded = true
	keep := 0
	if r.meta.Adopted() {
		keep = maxAdoptedEvents
	}
	evs, _ := readEvents(r.dir, keep)
	r.events = evs
	if n := len(evs); n > 0 {
		r.nextSeq = evs[n-1].Seq + 1
	}
	r.compactEvents()
}

// addEvent assigns seq/ts, stores, persists and signals subscribers.
func (r *run) addEvent(ev api.RunEvent) {
	r.ensureLoaded()
	ev.Seq = r.nextSeq
	r.nextSeq++
	ev.TS = time.Now().UTC().Format(time.RFC3339Nano)
	r.events = append(r.events, ev)
	_ = appendEvent(r.dir, ev)
	r.trimEvents()
	r.compactEvents()
	r.signal()
}

// importEvents appends events that already happened elsewhere — an adopted
// session's transcript — keeping the timestamps they carry, and persists them
// in one write.
func (r *run) importEvents(evs []api.RunEvent) {
	if len(evs) == 0 {
		return
	}
	r.ensureLoaded()
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	batch := make([]api.RunEvent, 0, len(evs))
	for _, ev := range evs {
		ev.Seq = r.nextSeq
		r.nextSeq++
		if ev.TS == "" {
			ev.TS = stamp
		}
		batch = append(batch, ev)
	}
	r.events = append(r.events, batch...)
	_ = appendEvent(r.dir, batch...)
	r.trimEvents()
	r.compactEvents()
	r.signal()
}

// maxAdoptedEvents bounds the events an adopted run holds in memory. An agent's
// session can run for days, and its whole transcript is replayed over SSE on
// every reconnect, so only the newest are kept, as the transcript tail does.
const maxAdoptedEvents = claude.MaxTranscriptEvents

// trimEvents drops an adopted run's oldest in-memory events beyond
// maxAdoptedEvents. Seq numbers are untouched, so EventsAfter keeps working;
// compactEvents bounds the log on disk the same way.
func (r *run) trimEvents() {
	if !r.meta.Adopted() || len(r.events) <= maxAdoptedEvents {
		return
	}
	r.events = append([]api.RunEvent(nil), r.events[len(r.events)-maxAdoptedEvents:]...)
}

// adoptedLogBytes is the size past which an adopted run's events.jsonl is
// rewritten to hold only the events kept in memory, so the log stays bounded
// however long the agent's session runs and a restart reads back only its tail.
const adoptedLogBytes = 4 << 20

// compactEvents rewrites an adopted run's event log to its in-memory events
// (the newest maxAdoptedEvents, seq numbers intact) once the log passes
// compactAt. The next threshold is at least twice the compacted size, so a
// tail of unusually large events can't force a rewrite on every append.
func (r *run) compactEvents() {
	if !r.meta.Adopted() {
		return
	}
	if r.compactAt == 0 {
		r.compactAt = adoptedLogBytes
	}
	if eventsSize(r.dir) <= r.compactAt {
		return
	}
	r.trimEvents()
	if writeEvents(r.dir, r.events) != nil {
		return
	}
	r.compactAt = max(adoptedLogBytes, 2*eventsSize(r.dir))
}

// changed marks a significant metadata change (status, session, cost, error).
func (r *run) changed() {
	r.meta.ResumeCommand = resumeCommand(r.meta)
	r.meta.SetCapabilities()
	r.metaVer++
	r.signal()
}

// setStatus updates the status and records a status event.
func (r *run) setStatus(status string) {
	r.meta.Status = status
	r.changed()
	r.addEvent(api.RunEvent{Kind: api.KindStatus, Status: status})
}

func (r *run) save() error { return writeMeta(r.dir, r.meta) }

func (r *run) signal() {
	for ch := range r.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// terminate sends SIGTERM (or stopSig) to the run's process group and
// SIGKILL after killGrace if it is still the same process group.
func (r *run) terminate() {
	pgid := r.pgid
	if pgid <= 0 {
		return // not started yet; execute checks cancelReq after start
	}
	sig := syscall.SIGTERM
	if r.stopSig != 0 {
		sig = r.stopSig
	}
	_ = syscall.Kill(-pgid, sig)
	time.AfterFunc(killGrace, func() {
		r.mu.Lock()
		still := r.pgid == pgid
		r.mu.Unlock()
		if still {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
	})
}
