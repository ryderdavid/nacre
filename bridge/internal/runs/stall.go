package runs

import (
	"fmt"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

// watchStalls flags running runs whose claude has gone quiet for longer than
// opts.StallAfter. A run once sat "running" for four hours on a dead API
// connection; the flag puts it in front of the person instead. The process is
// not touched: a quiet run might yet finish, and only the person knows
// whether to wait, or cancel and follow up.
func (m *Manager) watchStalls() {
	tick := time.NewTicker(min(time.Minute, m.opts.StallAfter/4))
	defer tick.Stop()
	for {
		select {
		case <-m.stop:
			return
		case t := <-tick.C:
			m.checkStalls(t)
		}
	}
}

// checkStalls flags the runs that have been quiet past the stall window.
func (m *Manager) checkStalls(at time.Time) {
	m.mu.Lock()
	runs := make([]*run, 0, m.running)
	for _, r := range m.runs {
		runs = append(runs, r)
	}
	m.mu.Unlock()
	for _, r := range runs {
		r.mu.Lock()
		quiet := at.Sub(r.lastOutput)
		if r.pgid == 0 || r.meta.Status != api.StatusRunning || r.meta.Stalled || quiet < m.opts.StallAfter {
			r.mu.Unlock()
			continue
		}
		r.meta.Stalled = true
		r.meta.StalledSince = r.lastOutput.UTC().Format(time.RFC3339)
		r.addEvent(api.RunEvent{Kind: api.KindSystem, Text: fmt.Sprintf(
			"Claude has printed nothing for %s, so it may be stuck. Wait, or cancel it and send a follow-up to carry on from here.",
			minutes(quiet))})
		r.changed()
		_ = r.save()
		snap := r.meta
		r.mu.Unlock()
		m.opts.Logger.Printf("runs: %s stalled (no output for %s)", snap.ID, quiet.Round(time.Second))
		m.notify(snap)
	}
}

// heard records output from the run's claude, clearing a stall flag. Caller
// holds r.mu; it returns true when the flag was cleared, and the caller then
// owes a save and a notification.
func (r *run) heard() bool {
	r.lastOutput = time.Now()
	if !r.meta.Stalled {
		return false
	}
	r.meta.Stalled, r.meta.StalledSince = false, ""
	r.addEvent(api.RunEvent{Kind: api.KindSystem, Text: "Claude is printing again."})
	r.changed()
	return true
}

// minutes says how long d is, to the minute, in words.
func minutes(d time.Duration) string {
	if n := int(d.Round(time.Minute) / time.Minute); n != 1 {
		return fmt.Sprintf("%d minutes", max(n, 1))
	}
	return "1 minute"
}
