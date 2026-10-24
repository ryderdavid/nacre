package runs

import (
	"sync"
	"syscall"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// killGrace is how long a cancelled process group gets between SIGTERM and SIGKILL.
const killGrace = 5 * time.Second

// run is the in-memory state of one run. All fields are guarded by mu, which
// is never held while acquiring Manager.mu.
type run struct {
	mu   sync.Mutex
	meta api.Run
	dir  string

	events  []api.RunEvent // loaded lazily for runs from previous sessions
	loaded  bool
	nextSeq int64
	metaVer uint64
	subs    map[chan struct{}]struct{}

	pending     *claude.Turn // turn waiting in the queue
	pgid        int          // process group of the running claude, 0 if none
	cancelReq   bool         // user asked to cancel the current turn
	shutdownReq bool         // bridge is shutting down
	turnResult  *claude.Result
	stderrTail  []string
}

func newRun(meta api.Run, dir string) *run {
	// Derived, not stored: a run loaded from a meta file written before the
	// capability flags existed still reports them correctly.
	meta.SetCapabilities()
	return &run{meta: meta, dir: dir, nextSeq: 1, subs: map[chan struct{}]struct{}{}}
}

// ensureLoaded reads events from disk on first use.
func (r *run) ensureLoaded() {
	if r.loaded {
		return
	}
	r.loaded = true
	evs, _ := readEvents(r.dir)
	r.events = evs
	if n := len(evs); n > 0 {
		r.nextSeq = evs[n-1].Seq + 1
	}
	r.trimEvents()
}

// addEvent assigns seq/ts, stores, persists and signals subscribers.
func (r *run) addEvent(ev api.RunEvent) {
	r.ensureLoaded()
	ev.Seq = r.nextSeq
	r.nextSeq++
	ev.TS = time.Now().UTC().Format(time.RFC3339Nano)
	r.events = append(r.events, ev)
	_ = appendEvent(r.dir, ev)
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
	r.signal()
}

// maxAdoptedEvents bounds the events an adopted run holds in memory. An agent's
// session can run for days, and its whole transcript is replayed over SSE on
// every reconnect, so only the newest are kept, as the transcript tail does.
const maxAdoptedEvents = claude.MaxTranscriptEvents

// trimEvents drops an adopted run's oldest in-memory events beyond
// maxAdoptedEvents. Seq numbers are untouched, so EventsAfter keeps working;
// the full log stays on disk.
func (r *run) trimEvents() {
	if !r.meta.Adopted() || len(r.events) <= maxAdoptedEvents {
		return
	}
	r.events = append([]api.RunEvent(nil), r.events[len(r.events)-maxAdoptedEvents:]...)
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

// terminate sends SIGTERM to the run's process group and SIGKILL after
// killGrace if it is still the same process group.
func (r *run) terminate() {
	pgid := r.pgid
	if pgid <= 0 {
		return // not started yet; execute checks cancelReq after start
	}
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	time.AfterFunc(killGrace, func() {
		r.mu.Lock()
		still := r.pgid == pgid
		r.mu.Unlock()
		if still {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
	})
}
