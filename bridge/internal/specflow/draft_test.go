package specflow

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// summaryOf is the change's entry in the changes summary.
func summaryOf(t *testing.T, m *Manager, repo, id string) api.ChangeSummary {
	t.Helper()
	list, _, err := m.Summaries(repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range list {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("change %s not in summary %+v", id, list)
	return api.ChangeSummary{}
}

// changeID is the change record behind the seed.
func changeID(t *testing.T, m *Manager, repo, seed string) string {
	t.Helper()
	env, err := m.Get(context.Background(), repo, seed)
	if err != nil || env.Change == nil {
		t.Fatalf("env=%+v err=%v", env, err)
	}
	return env.Change.ID
}

// gate holds the author's first call until released.
type gate struct{ entered, release chan struct{} }

func newGate() *gate { return &gate{make(chan struct{}), make(chan struct{})} }

// hold makes the model block every author call on the gate.
func (g *gate) hold(model *fakeModel) {
	model.fn = func(ctx context.Context, h claude.Headless, n int) (any, float64, error) {
		if h.System == authorSystem {
			select {
			case g.entered <- struct{}{}:
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			}
			select {
			case <-g.release:
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			}
		}
		return normalModel(ctx, h, n)
	}
}

func wantDraft(t *testing.T, s api.ChangeSummary, kind string) {
	t.Helper()
	switch {
	case kind == "" && s.Draft != nil:
		t.Fatalf("unexpected draft %+v in %+v", s.Draft, s)
	case kind != "" && (s.Draft == nil || s.Draft.Kind != kind):
		t.Fatalf("want %s draft, got %+v (phase %s, created %q)", kind, s.Draft, s.Phase, s.CreatedAt)
	}
}

func TestDraftPendingThenCreatedKeepsStartedAt(t *testing.T) {
	repo, _, model, opts := fixture(t)
	g := newGate()
	g.hold(model)
	m := newManager(t, opts)
	env, err := m.Act(context.Background(), repo, "NAC-1", Action{Action: "develop"})
	if err != nil {
		t.Fatal(err)
	}
	id := env.Change.ID
	<-g.entered
	pending := summaryOf(t, m, repo, id)
	wantDraft(t, pending, api.DraftPending)
	if pending.CreatedAt != "" || pending.StartedAt == "" || pending.StartedAt != env.Change.History[0].At {
		t.Fatalf("pending=%+v history=%+v", pending, env.Change.History)
	}
	close(g.release)
	m.Wait()
	created := summaryOf(t, m, repo, id)
	wantDraft(t, created, "")
	if created.Phase != "awaiting_approval" || created.CreatedAt == "" {
		t.Fatalf("created=%+v", created)
	}
	if created.StartedAt != pending.StartedAt || created.UpdatedAt <= pending.UpdatedAt {
		t.Fatalf("started_at moved or updated_at didn't: pending=%+v created=%+v", pending, created)
	}
	// Later saves, approval included, leave created_at and started_at alone.
	reviewed, err := m.Get(context.Background(), repo, "NAC-1")
	if err != nil {
		t.Fatal(err)
	}
	c := approve(t, m, repo, *reviewed.Change)
	ready := summaryOf(t, m, repo, id)
	if ready.CreatedAt != created.CreatedAt || ready.StartedAt != pending.StartedAt || c.CreatedAt != created.CreatedAt {
		t.Fatalf("ready=%+v created=%+v", ready, created)
	}
}

func TestCoveredDispositionMarksCreated(t *testing.T) {
	repo, _, model, opts := fixture(t)
	model.fn = func(ctx context.Context, h claude.Headless, n int) (any, float64, error) {
		if h.System == authorSystem {
			return authorResult{Route: "covered", Summary: "Already retained", Rationale: "Existing behavior covers it", References: []string{"AGENTS.md"}, Artifacts: []Artifact{}, Dependencies: []sectionDependency{}, Retired: []RetiredSection{}}, 0.1, nil
		}
		return normalModel(ctx, h, n)
	}
	m := newManager(t, opts)
	c := act(t, m, repo, "NAC-1", Action{Action: "develop"})
	if c.Phase != "covered" || c.CreatedAt == "" {
		t.Fatalf("%+v", c)
	}
	wantDraft(t, summaryOf(t, m, repo, c.ID), "")
}

func TestDraftFailsOnEveryStopBeforeCreation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		model func(context.Context, claude.Headless, int) (any, float64, error)
		want  string
	}{
		{"author error", func(context.Context, claude.Headless, int) (any, float64, error) {
			return nil, 0.1, errors.New("provider timeout")
		}, "provider timeout"},
		{"cost cap", func(context.Context, claude.Headless, int) (any, float64, error) {
			return nil, 1.6, errors.New("exceeded maximum budget")
		}, "ran out of its"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, _, model, opts := fixture(t)
			model.fn = tc.model
			m := newManager(t, opts)
			c := act(t, m, repo, "NAC-1", Action{Action: "develop"})
			s := summaryOf(t, m, repo, c.ID)
			wantDraft(t, s, api.DraftFailed)
			if c.Phase != "needs_attention" || !strings.Contains(s.Draft.Message, tc.want) || s.Draft.Message != c.Error {
				t.Fatalf("phase=%s draft=%+v error=%q", c.Phase, s.Draft, c.Error)
			}
		})
	}
	t.Run("shutdown", func(t *testing.T) {
		repo, _, model, opts := fixture(t)
		g := newGate()
		g.hold(model)
		m := newManager(t, opts)
		env, err := m.Act(context.Background(), repo, "NAC-1", Action{Action: "develop"})
		if err != nil {
			t.Fatal(err)
		}
		<-g.entered
		m.Shutdown(5 * time.Second)
		s := summaryOf(t, m, repo, env.Change.ID)
		wantDraft(t, s, api.DraftFailed)
		if s.Phase != "interrupted" || s.Draft.Message == "" {
			t.Fatalf("%+v", s)
		}
		// Still failed after the restart that follows, with nothing replayed.
		m2 := newManager(t, opts)
		wantDraft(t, summaryOf(t, m2, repo, env.Change.ID), api.DraftFailed)
	})
	t.Run("restart", func(t *testing.T) {
		repo, _, model, opts := fixture(t)
		g := newGate()
		g.hold(model)
		m := newManager(t, opts)
		env, err := m.Act(context.Background(), repo, "NAC-1", Action{Action: "develop"})
		if err != nil {
			t.Fatal(err)
		}
		<-g.entered
		// The computer stops without a shutdown: a copy of the record as it
		// was saved mid-drafting is what the next nacred finds.
		b, err := os.ReadFile(filepath.Join(opts.Dir, env.Change.ID+".json"))
		if err != nil {
			t.Fatal(err)
		}
		crashed := opts
		crashed.Dir = t.TempDir()
		if err := os.WriteFile(filepath.Join(crashed.Dir, env.Change.ID+".json"), b, 0o600); err != nil {
			t.Fatal(err)
		}
		m2 := newManager(t, crashed)
		s := summaryOf(t, m2, repo, env.Change.ID)
		wantDraft(t, s, api.DraftFailed)
		if s.Phase != "interrupted" || s.CreatedAt != "" || s.Draft.Message != s.Error {
			t.Fatalf("%+v", s)
		}
	})
}

func TestFailedDraftResumesToPending(t *testing.T) {
	repo, _, model, opts := fixture(t)
	model.fn = func(context.Context, claude.Headless, int) (any, float64, error) {
		return nil, 0.1, errors.New("provider timeout")
	}
	m := newManager(t, opts)
	c := act(t, m, repo, "NAC-1", Action{Action: "develop"})
	failed := summaryOf(t, m, repo, c.ID)
	wantDraft(t, failed, api.DraftFailed)
	g := newGate()
	g.hold(model)
	if _, err := m.Act(context.Background(), repo, "NAC-1", Action{Action: "resume"}); err != nil {
		t.Fatal(err)
	}
	<-g.entered
	pending := summaryOf(t, m, repo, c.ID)
	wantDraft(t, pending, api.DraftPending)
	if pending.StartedAt != failed.StartedAt {
		t.Fatalf("resume moved started_at: %s → %s", failed.StartedAt, pending.StartedAt)
	}
	close(g.release)
	m.Wait()
	wantDraft(t, summaryOf(t, m, repo, c.ID), "")
}

func TestRevisionOfCreatedChangeIsNoDraft(t *testing.T) {
	repo, _, model, opts := fixture(t)
	m := newManager(t, opts)
	c := develop(t, m, repo)
	g := newGate()
	g.hold(model)
	if _, err := m.Act(context.Background(), repo, "NAC-1", Action{Action: "revise", Feedback: "Mention focus."}); err != nil {
		t.Fatal(err)
	}
	<-g.entered
	s := summaryOf(t, m, repo, c.ID)
	if s.Phase != "drafting" || s.CreatedAt != c.CreatedAt {
		t.Fatalf("%+v", s)
	}
	wantDraft(t, s, "")
	close(g.release)
	m.Wait()
	// A revision that fails is no draft either.
	model.fn = func(context.Context, claude.Headless, int) (any, float64, error) {
		return nil, 0.1, errors.New("provider timeout")
	}
	c = act(t, m, repo, "NAC-1", Action{Action: "revise", Feedback: "Again."})
	if c.Phase != "needs_attention" {
		t.Fatalf("%+v", c)
	}
	wantDraft(t, summaryOf(t, m, repo, c.ID), "")
}

// olderRecord writes a record as an older nacred saved it: no format
// version and no created_at.
func olderRecord(t *testing.T, dir, repo, phase string, hist []History) string {
	t.Helper()
	repo, err := canonicalRepo(repo)
	if err != nil {
		t.Fatal(err)
	}
	id := newID()
	raw := map[string]any{"repo": repo, "stage": "author", "aliases": []string{},
		"change": map[string]any{"id": id, "seed_id": "NAC-1", "phase": phase, "history": hist, "updated_at": "2026-01-02T00:00:00Z"}}
	b, _ := json.Marshal(raw)
	if err := os.WriteFile(filepath.Join(dir, id+".json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	return id
}

func saved(t *testing.T, dir, id string) record {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var r record
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestOlderRecordsAreBackfilledAsCreated(t *testing.T) {
	t.Run("drafting with history", func(t *testing.T) {
		repo, _, _, opts := fixture(t)
		first := "2026-01-01T10:00:00Z"
		id := olderRecord(t, opts.Dir, repo, "drafting", []History{{At: first, Kind: "intent", Text: "Developing"}, {At: "2026-01-01T11:00:00Z", Kind: "call", Text: "Call"}})
		m := newManager(t, opts)
		s := summaryOf(t, m, repo, id)
		wantDraft(t, s, "")
		if s.CreatedAt != first || s.StartedAt != first {
			t.Fatalf("%+v", s)
		}
		r := saved(t, opts.Dir, id)
		if r.Version != recordVersion || r.Change.CreatedAt != first {
			t.Fatalf("backfill not saved: version=%d created=%q", r.Version, r.Change.CreatedAt)
		}
		// Loaded again, nothing changes.
		m.Shutdown(time.Second)
		m2 := newManager(t, opts)
		if again := summaryOf(t, m2, repo, id); again.CreatedAt != first || again.Draft != nil {
			t.Fatalf("%+v", again)
		}
	})
	t.Run("no history", func(t *testing.T) {
		repo, _, _, opts := fixture(t)
		id := olderRecord(t, opts.Dir, repo, "needs_attention", nil)
		before := stamp()
		m := newManager(t, opts)
		after := stamp()
		s := summaryOf(t, m, repo, id)
		wantDraft(t, s, "")
		if s.CreatedAt < before || s.CreatedAt > after || s.StartedAt != s.CreatedAt {
			t.Fatalf("created_at=%q not between %s and %s (started %q)", s.CreatedAt, before, after, s.StartedAt)
		}
		m.Shutdown(time.Second)
		time.Sleep(2 * time.Millisecond)
		m2 := newManager(t, opts)
		if again := summaryOf(t, m2, repo, id); again.CreatedAt != s.CreatedAt {
			t.Fatalf("second load moved created_at: %s → %s", s.CreatedAt, again.CreatedAt)
		}
	})
	t.Run("new record is not backfilled", func(t *testing.T) {
		repo, _, model, opts := fixture(t)
		model.fn = func(context.Context, claude.Headless, int) (any, float64, error) {
			return nil, 0.1, errors.New("provider timeout")
		}
		m := newManager(t, opts)
		c := act(t, m, repo, "NAC-1", Action{Action: "develop"})
		m.Shutdown(time.Second)
		if r := saved(t, opts.Dir, c.ID); r.Version != recordVersion || r.Change.CreatedAt != "" {
			t.Fatalf("version=%d created=%q", r.Version, r.Change.CreatedAt)
		}
		m2 := newManager(t, opts)
		s := summaryOf(t, m2, repo, c.ID)
		wantDraft(t, s, api.DraftFailed)
		if r := saved(t, opts.Dir, c.ID); r.Change.CreatedAt != "" {
			t.Fatalf("new record backfilled: %q", r.Change.CreatedAt)
		}
	})
}

func TestAdoptedChangeCountsAsCreated(t *testing.T) {
	repo, _, _, m := adoptFixture(t)
	c := act(t, m, repo, "NAC-1", Action{Action: "adopt", ChangeName: "retain-drafts"})
	if c.CreatedAt == "" || c.Adoption == nil {
		t.Fatalf("adopted change not created: %+v", c)
	}
	wantDraft(t, summaryOf(t, m, repo, c.ID), "")
}

func TestDismissFailedDraft(t *testing.T) {
	repo, _, model, opts := fixture(t)
	model.fn = func(context.Context, claude.Headless, int) (any, float64, error) {
		return nil, 0.1, errors.New("provider timeout")
	}
	m := newManager(t, opts)
	c := act(t, m, repo, "NAC-1", Action{Action: "develop"})
	s, changed, err := m.DismissDraft(repo, c.ID)
	if err != nil || !changed || s.Draft != nil || s.Phase != "needs_attention" || s.Error == "" {
		t.Fatalf("summary=%+v changed=%v err=%v", s, changed, err)
	}
	again, changed, err := m.DismissDraft(repo, c.ID)
	if err != nil || changed || again.Draft != nil || again.UpdatedAt != s.UpdatedAt {
		t.Fatalf("second dismissal: summary=%+v changed=%v err=%v", again, changed, err)
	}
	// The dismissal is kept across a restart.
	m.Shutdown(time.Second)
	m2 := newManager(t, opts)
	wantDraft(t, summaryOf(t, m2, repo, c.ID), "")
	if _, _, err := m2.DismissDraft(repo, "sfmissing"); !errors.Is(err, ErrConflict) {
		t.Fatalf("unknown change: %v", err)
	}

	// A new develop clears the dismissal and the draft is pending again.
	g := newGate()
	g.hold(model)
	env, err := m2.Act(context.Background(), repo, "NAC-1", Action{Action: "develop"})
	if err != nil || env.Change.Phase != "drafting" {
		t.Fatalf("env=%+v err=%v", env, err)
	}
	<-g.entered
	wantDraft(t, summaryOf(t, m2, repo, c.ID), api.DraftPending)
	if r := saved(t, opts.Dir, c.ID); r.DraftDismissed {
		t.Fatal("develop left draft_dismissed set")
	}
	// Dismissing a pending draft changes nothing.
	pending, changed, err := m2.DismissDraft(repo, c.ID)
	if err != nil || changed || pending.Draft == nil || pending.Draft.Kind != api.DraftPending {
		t.Fatalf("pending dismissal: %+v changed=%v err=%v", pending, changed, err)
	}
	close(g.release)
	m2.Wait()
	// Nor does dismissing a created change.
	before := summaryOf(t, m2, repo, c.ID)
	created, changed, err := m2.DismissDraft(repo, c.ID)
	if err != nil || changed || created.Draft != nil || created.UpdatedAt != before.UpdatedAt || created.Phase != "awaiting_approval" {
		t.Fatalf("created dismissal: %+v changed=%v err=%v", created, changed, err)
	}
	if r := saved(t, opts.Dir, c.ID); r.DraftDismissed {
		t.Fatal("dismissing a pending or created change recorded a dismissal")
	}
}

// The summary alone carries the draft: a client that missed every phase
// event, or a new nacred that published none, still sees it.
func TestDraftSummaryRebuiltWithoutEvents(t *testing.T) {
	repo, _, model, opts := fixture(t)
	model.fn = func(context.Context, claude.Headless, int) (any, float64, error) {
		return nil, 0.1, errors.New("provider timeout")
	}
	m := newManager(t, opts)
	c := act(t, m, repo, "NAC-1", Action{Action: "develop"})
	m.Shutdown(time.Second)
	events := 0
	opts.Changed = func(string, Change) { events++ }
	m2 := newManager(t, opts)
	s := summaryOf(t, m2, repo, c.ID)
	wantDraft(t, s, api.DraftFailed)
	if events != 0 || s.Draft.Message != c.Error || s.StartedAt != c.History[0].At {
		t.Fatalf("events=%d summary=%+v", events, s)
	}
}
