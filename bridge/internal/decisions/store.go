package decisions

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// AgentExpiry is how long a decision an agent raised waits for an answer.
const AgentExpiry = 24 * time.Hour

// DeferFor is how long a deferred decision stays out of the count.
const DeferFor = 24 * time.Hour

// keepEnded is how long an ended decision is kept on disk.
const keepEnded = 30 * 24 * time.Hour

// Reasons nacred gives for states it sets itself.
const (
	ReasonNoAnswer = "No answer within 24 hours. Ask again to raise it from the current state."
)

var (
	// ErrNotFound is an unknown decision.
	ErrNotFound = errors.New("unknown decision")
)

var idRe = regexp.MustCompile(`^dc_[a-z0-9_]{6,80}$`)

// ValidID reports whether id can name a decision.
func ValidID(id string) bool { return idRe.MatchString(id) }

// NewID is a fresh decision id.
func NewID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "dc_" + hex.EncodeToString(b[:])
}

// Store keeps decisions as one JSON file each (mode 0600) in a directory, so
// they survive restarts. It applies expiry and the end of a defer as it is
// read, and tells OnChange about every change, including those.
type Store struct {
	dir string
	now func() time.Time
	log *log.Logger

	mu   sync.Mutex
	recs map[string]*Record
	// links maps rounds and runs to the chats that started them.
	links map[string]string
	// OnChange hears about every created or changed decision, outside the
	// store's lock. Set it before use.
	OnChange func(Decision)
}

// Open loads the decisions in dir, creating it, and drops ended ones older
// than 30 days.
func Open(dir string, now func() time.Time, logger *log.Logger) (*Store, error) {
	if now == nil {
		now = time.Now
	}
	if logger == nil {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, now: now, log: logger, recs: map[string]*Record{}}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || !ValidID(strings.TrimSuffix(name, ".json")) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var rec Record
		if json.Unmarshal(data, &rec) != nil || rec.Decision.ID+".json" != name {
			logger.Printf("decisions: %s unreadable, skipped", name)
			continue
		}
		if !rec.Decision.Open() && s.age(rec.Decision) > keepEnded {
			_ = os.Remove(filepath.Join(dir, name))
			continue
		}
		r := rec
		s.recs[rec.Decision.ID] = &r
	}
	return s, nil
}

func (s *Store) stamp() string { return s.now().UTC().Format(time.RFC3339) }

func (s *Store) age(d Decision) time.Duration {
	at := d.CreatedAt
	if d.AnsweredAt != nil {
		at = *d.AnsweredAt
	}
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return 0
	}
	return s.now().Sub(t)
}

// Create stores a new decision as pending. An agent's decision expires after
// AgentExpiry. Older open decisions with the same key are superseded by it.
func (s *Store) Create(rec Record) (Record, error) {
	s.mu.Lock()
	if rec.Decision.ID == "" {
		rec.Decision.ID = NewID()
	}
	now := s.stamp()
	d := &rec.Decision
	d.CreatedAt, d.State, d.Result, d.AnsweredAt, d.Answer = now, StatePending, nil, nil, nil
	if d.Source == SourceAgent && d.ExpiresAt == nil {
		exp := s.now().Add(AgentExpiry).UTC().Format(time.RFC3339)
		d.ExpiresAt = &exp
	}
	if d.Consequences == nil {
		d.Consequences = []string{}
	}
	if d.Evidence == nil {
		d.Evidence = []Evidence{}
	}
	if d.Options == nil {
		d.Options = []Option{}
	}
	var changed []Decision
	if rec.Key != "" {
		for _, old := range s.recs {
			if old.Key != rec.Key || old.Decision.ID == d.ID {
				continue
			}
			if st := old.Decision.State; st == StatePending || st == StateDeferred || st == StateHeld ||
				(st == StateExpired || st == StateFailed || st == StateUnknown) && old.Decision.SupersededBy == "" {
				if st == StatePending || st == StateDeferred || st == StateHeld {
					old.Decision.State = StateSuperseded
					old.Decision.Reason = "A newer decision replaces this one."
				}
				old.Decision.SupersededBy = d.ID
				old.Decision.Acknowledged = true
				if err := s.saveLocked(old); err != nil {
					s.log.Printf("decisions: save %s: %v", old.Decision.ID, err)
				}
				changed = append(changed, old.Decision)
			}
		}
	}
	r := rec
	if err := s.saveLocked(&r); err != nil {
		s.mu.Unlock()
		return Record{}, err
	}
	s.recs[d.ID] = &r
	out := r
	s.mu.Unlock()
	s.notify(append(changed, out.Decision)...)
	return out, nil
}

// Put stores a record as it is (a Needs you item answered for the first
// time), without supersession or a new id. When one with its id is already
// stored, that one is returned unchanged.
func (s *Store) Put(rec Record) (Record, error) {
	s.mu.Lock()
	if old := s.recs[rec.Decision.ID]; old != nil {
		out := *old
		s.mu.Unlock()
		return out, nil
	}
	r := rec
	if err := s.saveLocked(&r); err != nil {
		s.mu.Unlock()
		return Record{}, err
	}
	s.recs[rec.Decision.ID] = &r
	s.mu.Unlock()
	s.notify(r.Decision)
	return r, nil
}

// Get returns a decision, with expiry applied.
func (s *Store) Get(id string) (Record, bool) {
	s.mu.Lock()
	r := s.recs[id]
	if r == nil {
		s.mu.Unlock()
		return Record{}, false
	}
	changed := s.tickLocked(r)
	out := *r
	s.mu.Unlock()
	if changed {
		s.notify(out.Decision)
	}
	return out, true
}

// List returns every decision, newest first, with expiry applied.
func (s *Store) List() []Record {
	s.mu.Lock()
	out := make([]Record, 0, len(s.recs))
	var changed []Decision
	for _, r := range s.recs {
		if s.tickLocked(r) {
			changed = append(changed, r.Decision)
		}
		out = append(out, *r)
	}
	s.mu.Unlock()
	s.notify(changed...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Decision.CreatedAt != out[j].Decision.CreatedAt {
			return out[i].Decision.CreatedAt > out[j].Decision.CreatedAt
		}
		return out[i].Decision.ID > out[j].Decision.ID
	})
	return out
}

// Sweep applies expiry to every decision, for a periodic tick.
func (s *Store) Sweep() { s.List() }

// Update changes a decision under the store's lock and saves it. f returns
// false to leave it unchanged.
func (s *Store) Update(id string, f func(*Record) bool) (Record, error) {
	s.mu.Lock()
	r := s.recs[id]
	if r == nil {
		s.mu.Unlock()
		return Record{}, ErrNotFound
	}
	ticked := s.tickLocked(r)
	next := *r
	changed := f(&next)
	if changed {
		if err := s.saveLocked(&next); err != nil {
			s.mu.Unlock()
			return *r, err
		}
		*r = next
	}
	out := *r
	s.mu.Unlock()
	if changed || ticked {
		s.notify(out.Decision)
	}
	return out, nil
}

// tickLocked expires an unanswered decision past its expiry and ends a
// defer that has run its day. It reports whether r changed.
func (s *Store) tickLocked(r *Record) bool {
	d := &r.Decision
	now := s.now()
	changed := false
	if d.State == StateDeferred && d.DeferredUntil != nil {
		if t, err := time.Parse(time.RFC3339, *d.DeferredUntil); err == nil && !now.Before(t) {
			d.State, d.DeferredUntil = StatePending, nil
			changed = true
		}
	}
	if (d.State == StatePending || d.State == StateDeferred) && d.ExpiresAt != nil {
		if t, err := time.Parse(time.RFC3339, *d.ExpiresAt); err == nil && !now.Before(t) {
			d.State, d.Reason, d.DeferredUntil = StateExpired, ReasonNoAnswer, nil
			changed = true
		}
	}
	if changed {
		if err := s.saveLocked(r); err != nil {
			s.log.Printf("decisions: save %s: %v", d.ID, err)
		}
	}
	return changed
}

// Now is the store's clock, as an RFC 3339 stamp.
func (s *Store) Now() string { return s.stamp() }

// Clock is the store's clock.
func (s *Store) Clock() time.Time { return s.now() }

func (s *Store) notify(ds ...Decision) {
	if s.OnChange == nil {
		return
	}
	for _, d := range ds {
		s.OnChange(d)
	}
}

// saveLocked writes a record atomically with mode 0600.
func (s *Store) saveLocked(r *Record) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, ".decision-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	err = tmp.Chmod(0o600)
	if err == nil {
		_, err = tmp.Write(data)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, filepath.Join(s.dir, r.Decision.ID+".json"))
	}
	if err != nil {
		os.Remove(name)
	}
	return err
}
