package decisions

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// clock is a settable test clock.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func openStore(t *testing.T, dir string, c *clock) *Store {
	t.Helper()
	s, err := Open(dir, c.now, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func landRecord(round string) Record {
	chat := "ch_1"
	return Record{
		Decision: Decision{Repo: "r1", ChatID: &chat, Source: SourceAgent, Kind: KindLand, Title: "Land " + round,
			Target: Target{Round: round, Commit: "abc1234"}, Consequences: []string{"Merges into main"},
			Options:    []Option{{ID: "land", Label: "Merge into main and push", Recommended: true, Protected: true}},
			AllowDefer: true, AllowReject: true, RejectLabel: "Don't land"},
		Actions: map[string]Action{"land": {Method: "POST", Path: "/v1/rounds/" + round + "/land"}},
		Bound:   "abc1234", Key: "land:" + round, RejectHolds: true, Follow: true,
	}
}

func TestCreateLoadAndKeepActionsServerSide(t *testing.T) {
	dir := t.TempDir()
	c := &clock{t: time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)}
	s := openStore(t, dir, c)
	var heard []Decision
	s.OnChange = func(d Decision) { heard = append(heard, d) }
	rec, err := s.Create(landRecord("rd1"))
	if err != nil {
		t.Fatal(err)
	}
	d := rec.Decision
	if !ValidID(d.ID) || d.State != StatePending || d.CreatedAt != "2026-11-06T09:00:00Z" ||
		d.ExpiresAt == nil || *d.ExpiresAt != "2026-11-07T09:00:00Z" || d.AnsweredAt != nil || d.Result != nil {
		t.Fatalf("created %+v", d)
	}
	if len(heard) != 1 || heard[0].ID != d.ID {
		t.Fatalf("OnChange heard %+v", heard)
	}
	st, err := os.Stat(filepath.Join(dir, d.ID+".json"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("file %v %v", st, err)
	}

	// A new store (nacred restarted) has it, actions and binding included.
	again := openStore(t, dir, c)
	got, ok := again.Get(d.ID)
	if !ok || got.Bound != "abc1234" || got.Actions["land"].Path != "/v1/rounds/rd1/land" || !got.RejectHolds || !got.Follow ||
		got.Decision.Title != "Land rd1" || *got.Decision.ChatID != "ch_1" {
		t.Fatalf("reloaded %+v", got)
	}
	if _, ok := again.Get("dc_nope00"); ok {
		t.Fatal("unknown decision found")
	}
}

func TestExpiryAfterADay(t *testing.T) {
	c := &clock{t: time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)}
	s := openStore(t, t.TempDir(), c)
	rec, _ := s.Create(landRecord("rd1"))
	c.add(23 * time.Hour)
	if got, _ := s.Get(rec.Decision.ID); got.Decision.State != StatePending {
		t.Fatalf("after 23h: %s", got.Decision.State)
	}
	var heard []Decision
	s.OnChange = func(d Decision) { heard = append(heard, d) }
	c.add(time.Hour)
	list := s.List()
	if len(list) != 1 || list[0].Decision.State != StateExpired || list[0].Decision.Reason != ReasonNoAnswer || list[0].Decision.Open() {
		t.Fatalf("after 24h: %+v", list[0].Decision)
	}
	if len(heard) != 1 || heard[0].State != StateExpired {
		t.Fatalf("expiry not announced: %+v", heard)
	}
	// Needs you items have no expiry of their own.
	att := landRecord("rd2")
	att.Decision.Source, att.Decision.ChatID = SourceAttention, nil
	ar, _ := s.Create(att)
	if ar.Decision.ExpiresAt != nil {
		t.Fatalf("attention decision expires: %v", *ar.Decision.ExpiresAt)
	}
}

// A chat's intake and found cards expire asking the chat to raise them
// again: nacred can't rebuild them itself, so Ask again would be refused.
func TestChatIntakeAndFoundExpireAskingTheChat(t *testing.T) {
	c := &clock{t: time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)}
	s := openStore(t, t.TempDir(), c)
	var ids []string
	for _, kind := range []string{KindIntake, KindFound} {
		rec := landRecord("rd-" + kind)
		rec.Decision.Kind, rec.Key = kind, kind+":r1:x"
		created, err := s.Create(rec)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, created.Decision.ID)
	}
	c.add(24 * time.Hour)
	for _, id := range ids {
		got, _ := s.Get(id)
		if got.Decision.State != StateExpired || got.Decision.Reason != ReasonNoAnswerChat {
			t.Fatalf("%s: %+v", got.Decision.Kind, got.Decision)
		}
	}
}

func TestDeferEndsAfterADayAndExpiryStillApplies(t *testing.T) {
	c := &clock{t: time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)}
	s := openStore(t, t.TempDir(), c)
	rec := landRecord("rd1")
	rec.Decision.Source = SourceAttention
	created, _ := s.Create(rec)
	until := c.now().Add(DeferFor).UTC().Format(time.RFC3339)
	got, err := s.Update(created.Decision.ID, func(r *Record) bool {
		r.Decision.State, r.Decision.DeferredUntil = StateDeferred, &until
		return true
	})
	if err != nil || got.Decision.State != StateDeferred || !got.Decision.Open() || got.Decision.Waiting() {
		t.Fatalf("deferred %+v %v", got.Decision, err)
	}
	c.add(DeferFor)
	if got, _ := s.Get(created.Decision.ID); got.Decision.State != StatePending || got.Decision.DeferredUntil != nil || !got.Decision.Waiting() {
		t.Fatalf("after a day: %+v", got.Decision)
	}
}

func TestANewerDecisionSupersedesTheOpenOne(t *testing.T) {
	c := &clock{t: time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)}
	s := openStore(t, t.TempDir(), c)
	first, _ := s.Create(landRecord("rd1"))
	other, _ := s.Create(landRecord("rd2"))
	c.add(time.Minute)
	second, _ := s.Create(landRecord("rd1"))
	old, _ := s.Get(first.Decision.ID)
	if old.Decision.State != StateSuperseded || old.Decision.SupersededBy != second.Decision.ID || old.Decision.Open() {
		t.Fatalf("old %+v", old.Decision)
	}
	if o, _ := s.Get(other.Decision.ID); o.Decision.State != StatePending {
		t.Fatalf("another target was touched: %+v", o.Decision)
	}
	// An expired one links to its replacement without changing state.
	c.add(25 * time.Hour)
	s.Sweep()
	third, _ := s.Create(landRecord("rd1"))
	exp, _ := s.Get(second.Decision.ID)
	if exp.Decision.State != StateExpired || exp.Decision.SupersededBy != third.Decision.ID {
		t.Fatalf("expired one %+v", exp.Decision)
	}
}

func TestOpenAndWaiting(t *testing.T) {
	for _, c := range []struct {
		d             Decision
		open, waiting bool
	}{
		{Decision{State: StatePending}, true, true},
		{Decision{State: StateHeld}, true, true},
		{Decision{State: StateDeferred}, true, false},
		{Decision{State: StateExecuting}, true, false},
		{Decision{State: StateFailed}, true, true},
		{Decision{State: StateFailed, Acknowledged: true}, false, false},
		{Decision{State: StateUnknown}, true, true},
		{Decision{State: StateDone}, false, false},
		{Decision{State: StateDeclined}, false, false},
		{Decision{State: StateExpired}, false, false},
		{Decision{State: StateSuperseded}, false, false},
	} {
		if c.d.Open() != c.open || c.d.Waiting() != c.waiting {
			t.Errorf("%s ack=%v: open=%v waiting=%v", c.d.State, c.d.Acknowledged, c.d.Open(), c.d.Waiting())
		}
	}
}

func TestWorkLinksSurviveARestart(t *testing.T) {
	dir := t.TempDir()
	c := &clock{t: time.Now()}
	s := openStore(t, dir, c)
	s.LinkWork("rd1", "ch_a")
	s.LinkWork("", "ch_b")
	if again := openStore(t, dir, c); again.ChatOf("rd1") != "ch_a" || again.ChatOf("rd2") != "" {
		t.Fatal("links not reloaded")
	}
}
