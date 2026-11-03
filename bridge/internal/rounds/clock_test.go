package rounds

import (
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

func at(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func TestTimePausedNeverCountsAgainstTheTimeLimit(t *testing.T) {
	start := time.Now().Add(-5 * time.Hour).Truncate(time.Second)
	r := &round{meta: api.Round{State: api.RoundImplementing, StartedAt: at(start), TimeLimitMinutes: 240}}

	// Worked an hour, then paused for four.
	r.meta.State = api.RoundPaused
	r.trackPause(start.Add(time.Hour))
	if got := r.workTime(start.Add(5 * time.Hour)); got != time.Hour {
		t.Fatalf("paused: worked %v, want 1h", got)
	}
	r.meta.State = api.RoundImplementing
	r.trackPause(start.Add(5 * time.Hour))
	if r.meta.PausedAt != "" || r.meta.PausedSeconds != 4*3600 {
		t.Fatalf("resumed: paused_at %q, paused %ds", r.meta.PausedAt, r.meta.PausedSeconds)
	}
	if got := r.workTime(start.Add(6 * time.Hour)); got != 2*time.Hour {
		t.Fatalf("resumed: worked %v, want 2h", got)
	}
}

func TestARoundPausedBeforeTheCountGetsItsPausesFromTheLog(t *testing.T) {
	start := time.Now().Add(-5 * time.Hour).Truncate(time.Second)
	ts := func(d time.Duration) string { return start.Add(d).UTC().Format(time.RFC3339Nano) }
	r := &round{meta: api.Round{State: api.RoundPaused, StartedAt: at(start), TimeLimitMinutes: 240}}
	r.events = []api.RoundEvent{
		{Kind: "state", TS: ts(-time.Hour), Text: "Implementing"},
		{Kind: "state", TS: ts(10 * time.Minute), Text: "Paused: Specification review required"},
		{Kind: "plan", TS: ts(time.Hour), Text: "Something else"},
		{Kind: "state", TS: ts(4 * time.Hour), Text: "Carrying on after a change"},
		{Kind: "state", TS: ts(4*time.Hour + time.Minute), Text: "Paused: The round has run for its 240-minute time limit."},
	}
	r.backfillPauses()
	if r.meta.PausedSeconds != int64((3*time.Hour+50*time.Minute)/time.Second) {
		t.Fatalf("paused %ds", r.meta.PausedSeconds)
	}
	if got := r.workTime(start.Add(5 * time.Hour)); got != 11*time.Minute {
		t.Fatalf("worked %v, want 11m", got)
	}
}

func TestRaisingTheLimitsOnATimeStopGivesTheRoundMoreTime(t *testing.T) {
	start := time.Now().Add(-5 * time.Hour).Truncate(time.Second)
	r := &round{meta: api.Round{State: api.RoundPaused, StartedAt: at(start), PausedAt: at(time.Now()), TimeLimitMinutes: 240}}
	r.moreTime(0)
	if r.meta.TimeLimitMinutes < 539 {
		t.Fatalf("limit %d minutes: still out of time", r.meta.TimeLimitMinutes)
	}
	r.meta.TimeLimitMinutes = 240
	r.moreTime(600)
	if r.meta.TimeLimitMinutes != 240 {
		t.Fatalf("a given limit was changed to %d", r.meta.TimeLimitMinutes)
	}
}
