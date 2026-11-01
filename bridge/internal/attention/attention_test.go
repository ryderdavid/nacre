package attention

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
)

func issues(t *testing.T, beads ...map[string]any) []api.Issue {
	t.Helper()
	out := []api.Issue{}
	for _, b := range beads {
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, raw)
	}
	return out
}

type fixture struct {
	b      *Builder
	reads  int
	beads  []api.Issue
	rounds []api.Round
	change []api.ChangeSummary
	now    time.Time
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{now: time.Unix(1_800_000_000, 0)}
	f.b = &Builder{
		Repos: func() []repos.Ref { return []repos.Ref{{ID: "r1", Name: "nacre", Path: "/src/nacre"}} },
		Issues: func(context.Context, string) ([]api.Issue, error) {
			f.reads++
			return f.beads, nil
		},
		Rounds:  func() []api.Round { return f.rounds },
		Changes: func(string) ([]api.ChangeSummary, error) { return f.change, nil },
		Now:     func() time.Time { return f.now },
	}
	return f
}

func kinds(items []api.AttentionItem) []string {
	out := []string{}
	for _, i := range items {
		out = append(out, i.Kind)
	}
	return out
}

func TestItemsCoverBeadsRoundsAndChangesInOrder(t *testing.T) {
	f := newFixture(t)
	f.beads = issues(t,
		map[string]any{"id": "N-1", "title": "Pick a palette", "status": "open", "issue_type": "decision", "updated_at": "2026-11-01T01:00:00Z"},
		map[string]any{"id": "N-2", "title": "Check on the Fold", "status": "open", "issue_type": "task", "labels": []string{"human"}, "updated_at": "2026-11-01T02:00:00Z"},
		map[string]any{"id": "N-3", "title": "Done decision", "status": "closed", "issue_type": "decision"},
		map[string]any{"id": "N-4", "title": "Ordinary", "status": "open", "issue_type": "task"},
		map[string]any{"id": "N-5", "title": "Retain drafts", "status": "open", "issue_type": "feature"},
	)
	f.change = []api.ChangeSummary{
		{ID: "sf1", ChangeName: "retain-drafts", Phase: "awaiting_approval", Revision: "abcdef123456", SeedIDs: []string{"N-5"}, UpdatedAt: "2026-11-01T03:00:00Z"},
		{ID: "sf2", ChangeName: "other", Phase: "ready", SeedIDs: []string{"N-6"}},
		{ID: "sf3", ChangeName: "stale-one", Phase: "stale", Error: "AGENTS.md changed", SeedIDs: []string{"N-7"}},
		{ID: "sf4", ChangeName: "", Phase: "interrupted", SeedIDs: []string{"N-5"}},
		{ID: "sf5", ChangeName: "blocked", Phase: "needs_attention", SeedIDs: []string{"N-8"}},
	}
	f.rounds = []api.Round{
		{ID: "R1", RepoID: "r1", State: api.RoundNeedsDecision, UpdatedAt: "2026-11-01T04:00:00Z",
			Beads: []api.RoundBead{{ID: "N-4", Title: "Ordinary"}},
			Questions: []api.Question{
				{ID: "q1", Beads: []string{"N-4"}, Text: "Enter sends or inserts a newline?", Options: []string{"Sends", "Newline"}},
				{ID: "q2", Beads: []string{"N-4"}, Text: "Answered", Options: []string{"A"}, Answer: "A"},
			}},
		{ID: "R2", RepoID: "r1", State: api.RoundImplementing, Beads: []api.RoundBead{{ID: "N-9", State: api.BeadParked, Reason: "Gates failed twice"}}},
		{ID: "R3", RepoID: "r1", State: api.RoundReadyToLand, Target: "main", FinalCommit: "0123456789ab", Beads: []api.RoundBead{{ID: "N-10"}}},
		{ID: "R4", RepoID: "elsewhere", State: api.RoundReadyToLand},
	}
	items := f.b.Items(context.Background())
	got := kinds(items)
	want := []string{
		api.AttentionRoundQuestion, api.AttentionBeadDecision, api.AttentionBeadDecision,
		api.AttentionBeadParked, api.AttentionReviewBlocked, api.AttentionWorkflowInterrupted,
		api.AttentionProposalReady, api.AttentionApprovalStale, api.AttentionRoundReady,
	}
	count := func(list []string) map[string]int {
		m := map[string]int{}
		for _, k := range list {
			m[k]++
		}
		return m
	}
	gc, wc := count(got), count(want)
	for k, n := range wc {
		if gc[k] != n {
			t.Fatalf("kinds=%v want counts %v", got, wc)
		}
	}
	if len(got) != len(want) || got[0] != api.AttentionRoundQuestion || got[len(got)-1] != api.AttentionRoundReady {
		t.Fatalf("order=%v", got)
	}
	for i := range items {
		if group(items[i].Kind) == 1 && i > 0 && group(items[i-1].Kind) == 2 {
			t.Fatalf("approvals after landing: %v", got)
		}
	}
	for _, it := range items {
		if it.Why == "" || it.Title == "" || it.ID == "" || len(it.Options) == 0 || it.Repo != "r1" {
			t.Fatalf("item doesn't explain itself: %+v", it)
		}
		switch it.Kind {
		case api.AttentionRoundQuestion:
			if len(it.Options) != 3 || it.Options[0].Path != "/v1/rounds/R1/answers" || it.Options[0].Body["question_id"] != "q1" {
				t.Fatalf("question options=%+v", it.Options)
			}
		case api.AttentionProposalReady:
			if it.Options[1].Body["revision"] != "abcdef123456" || it.Options[1].Path != "/v1/repos/r1/issues/N-5/change" {
				t.Fatalf("approve option=%+v", it.Options[1])
			}
		case api.AttentionWorkflowInterrupted:
			if it.Title != "Retain drafts was interrupted" {
				t.Fatalf("unnamed change falls back to the seed title: %q", it.Title)
			}
		case api.AttentionApprovalStale:
			if it.Why != "AGENTS.md changed" {
				t.Fatalf("stale why=%q", it.Why)
			}
		case api.AttentionBeadParked:
			if it.Options[0].Path != "/v1/rounds/R2/beads/N-9/retry" {
				t.Fatalf("parked options=%+v", it.Options)
			}
		}
	}
}

func TestItemsAreStableCachedAndDisappearWhenAnswered(t *testing.T) {
	f := newFixture(t)
	f.beads = issues(t, map[string]any{"id": "N-1", "title": "Decide", "status": "open", "issue_type": "decision"})
	first := f.b.Items(context.Background())
	second := f.b.Items(context.Background())
	if len(first) != 1 || first[0].ID != second[0].ID {
		t.Fatalf("unstable ids: %+v %+v", first, second)
	}
	if f.reads != 1 {
		t.Fatalf("beads read %d times within the refresh window", f.reads)
	}
	f.beads = issues(t, map[string]any{"id": "N-1", "title": "Decide", "status": "closed", "issue_type": "decision"})
	if got := f.b.Items(context.Background()); len(got) != 1 {
		t.Fatal("cache ignored before the refresh window")
	}
	f.b.Invalidate("/src/nacre")
	if got := f.b.Items(context.Background()); len(got) != 0 {
		t.Fatalf("closed decision still listed after invalidation: %+v", got)
	}
	f.beads = issues(t, map[string]any{"id": "N-2", "title": "Decide again", "status": "open", "issue_type": "decision"})
	f.now = f.now.Add(RefreshAfter + time.Second)
	if got := f.b.Items(context.Background()); len(got) != 1 || got[0].BeadID != "N-2" {
		t.Fatalf("not refreshed after the window: %+v", got)
	}
}

func TestUnreadableRepositoryContributesNothing(t *testing.T) {
	f := newFixture(t)
	f.b.Issues = func(context.Context, string) ([]api.Issue, error) { return nil, context.DeadlineExceeded }
	f.rounds = []api.Round{{ID: "R1", RepoID: "r1", State: api.RoundReadyToLand}}
	if got := kinds(f.b.Items(context.Background())); len(got) != 1 || got[0] != api.AttentionRoundReady {
		t.Fatalf("kinds=%v", got)
	}
}
