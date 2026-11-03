package assist

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// TurnRetention is how long a finished turn can still be read back. It covers
// a phone that lost its connection near the end of a turn and was asleep or
// between networks for a while: the app keeps reattaching for up to three
// minutes of awake time, which can span far more wall-clock time than that.
const TurnRetention = 30 * time.Minute

// maxTurnEvents bounds one turn's buffered transcript. A board question is a
// few dozen events; this only stops a runaway turn from growing without end.
const maxTurnEvents = 5000

// Turn is one assist call that runs on its own, apart from the HTTP request
// that started it. A phone on a flaky connection can drop and reattach
// without losing the answer: events are numbered and kept until the turn has
// been finished for TurnRetention.
type Turn struct {
	ID string

	cancel context.CancelFunc

	mu       sync.Mutex
	events   []api.RunEvent
	changed  chan struct{} // closed and replaced whenever something is added
	done     bool
	out      Outcome
	err      error
	finished time.Time
}

// TurnState is a turn's progress as of one read.
type TurnState struct {
	// Events are those after the seq the reader asked for, in order.
	Events []api.RunEvent
	Done   bool
	Out    Outcome
	Err    error
	// Changed is closed when there is something new to read.
	Changed <-chan struct{}
}

// Start runs h in cwd as a detached turn and returns at once. The turn ends
// when claude does, when it times out, or when Cancel is called; a reader
// going away does not stop it.
func (r *Runner) Start(cwd string, h claude.Headless) *Turn {
	t, _ := r.StartID("", cwd, h)
	return t
}

// StartID is Start with the id the app chose for the turn. When a turn with
// that id is already known it is returned as it is, with started false, and
// nothing new runs: the app sends the same request again when its connection
// dropped before it heard whether the first one arrived, and must not get two
// answers for one question. An empty id gets a fresh one.
func (r *Runner) StartID(id, cwd string, h claude.Headless) (t *Turn, started bool) {
	r.turnsMu.Lock()
	if r.turns == nil {
		r.turns = map[string]*Turn{}
	}
	r.pruneLocked(time.Now())
	if id != "" {
		if t := r.turns[id]; t != nil {
			r.turnsMu.Unlock()
			return t, false
		}
	} else {
		id = newTurnID()
	}
	// Reserve before starting the goroutine so maintenance cannot race it.
	r.active.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	t = &Turn{ID: id, cancel: cancel, changed: make(chan struct{})}
	r.turns[t.ID] = t
	r.turnsMu.Unlock()

	go func() {
		defer r.active.Add(-1)
		defer cancel()
		var seq int64
		out, err := r.Run(ctx, cwd, h, func(ev api.RunEvent) {
			seq++
			ev.Seq, ev.TS = seq, time.Now().UTC().Format(time.RFC3339Nano)
			t.add(ev)
		})
		t.finish(out, err)
	}()
	return t, true
}

// ValidTurnID reports whether id can name a turn the app chose: the app's
// "c_" prefix and 8 to 64 letters, digits, '-' or '_'. Server-made ids start
// "t_", so the two never collide.
func ValidTurnID(id string) bool {
	if !strings.HasPrefix(id, "c_") || len(id) < 10 || len(id) > 66 {
		return false
	}
	for _, c := range id[2:] {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// Turn returns the turn with this id, or nil once it is unknown or expired.
func (r *Runner) Turn(id string) *Turn {
	r.turnsMu.Lock()
	defer r.turnsMu.Unlock()
	r.pruneLocked(time.Now())
	return r.turns[id]
}

// pruneLocked forgets turns that finished more than TurnRetention ago.
func (r *Runner) pruneLocked(now time.Time) {
	for id, t := range r.turns {
		t.mu.Lock()
		expired := t.done && now.Sub(t.finished) > TurnRetention
		t.mu.Unlock()
		if expired {
			delete(r.turns, id)
		}
	}
}

// Cancel stops the turn's claude process. A finished turn is unaffected.
func (t *Turn) Cancel() { t.cancel() }

// State returns the events after seq and whether the turn has finished.
func (t *Turn) State(after int64) TurnState {
	t.mu.Lock()
	defer t.mu.Unlock()
	st := TurnState{Done: t.done, Out: t.out, Err: t.err, Changed: t.changed}
	for _, ev := range t.events {
		if ev.Seq > after {
			st.Events = append(st.Events, ev)
		}
	}
	return st
}

func (t *Turn) add(ev api.RunEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.events) >= maxTurnEvents {
		return
	}
	t.events = append(t.events, ev)
	t.signalLocked()
}

func (t *Turn) finish(out Outcome, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.done, t.out, t.err, t.finished = true, out, err, time.Now()
	t.signalLocked()
}

func (t *Turn) signalLocked() {
	close(t.changed)
	t.changed = make(chan struct{})
}

func newTurnID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return "t_" + hex.EncodeToString(b[:])
}
