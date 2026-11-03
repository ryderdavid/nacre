package rounds

import (
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

// workTime is how long the round has worked since implementation began. Time
// spent paused is left out: a paused round waits on Ryder, and that wait
// isn't the round's to count against its time limit.
func (r *round) workTime(now time.Time) time.Duration {
	start, err := time.Parse(time.RFC3339, r.meta.StartedAt)
	if err != nil {
		return 0
	}
	d := now.Sub(start) - time.Duration(r.meta.PausedSeconds)*time.Second
	if t, err := time.Parse(time.RFC3339, r.meta.PausedAt); err == nil {
		d -= now.Sub(t)
	}
	return max(d, 0)
}

// trackPause keeps count of paused time as the round goes in and out of
// paused. Every change is saved, so saving keeps it current.
func (r *round) trackPause(now time.Time) {
	paused := r.meta.State == api.RoundPaused
	switch {
	case paused && r.meta.PausedAt == "":
		r.meta.PausedAt = now.UTC().Format(time.RFC3339)
	case !paused && r.meta.PausedAt != "":
		if t, err := time.Parse(time.RFC3339, r.meta.PausedAt); err == nil {
			r.meta.PausedSeconds += int64(now.Sub(t) / time.Second)
		}
		r.meta.PausedAt = ""
	}
}

// backfillPauses works out from the log how long a round started before
// paused time was counted spent paused.
func (r *round) backfillPauses() {
	start, err := time.Parse(time.RFC3339, r.meta.StartedAt)
	if err != nil || r.meta.PausedSeconds > 0 || r.meta.PausedAt != "" {
		return
	}
	var since time.Time
	var total time.Duration
	for _, e := range r.events {
		if e.Kind != "state" {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, e.TS)
		if err != nil {
			continue
		}
		if t.Before(start) {
			t = start
		}
		switch {
		case strings.HasPrefix(e.Text, "Paused: "):
			if since.IsZero() {
				since = t
			}
		case !since.IsZero():
			total += t.Sub(since)
			since = time.Time{}
		}
	}
	r.meta.PausedSeconds = int64(total / time.Second)
	if !since.IsZero() && r.meta.State == api.RoundPaused {
		r.meta.PausedAt = since.UTC().Format(time.RFC3339)
	}
}

// moreTime gives a round whose time is up, and whose limits were raised
// without a new time limit, as long again as its limit was: raising the
// limits must let it carry on.
func (r *round) moreTime(minutes int) {
	if minutes > 0 || r.meta.TimeLimitMinutes <= 0 {
		return
	}
	if used := int(r.workTime(time.Now()) / time.Minute); used >= r.meta.TimeLimitMinutes {
		r.meta.TimeLimitMinutes += used
	}
}
