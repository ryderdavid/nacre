package attention

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
			// One item asks the round's questions; answers live on each question.
			if len(it.Questions) != 1 || len(it.Questions[0].Options) != 2 || it.Questions[0].Options[0].Path != "/v1/rounds/R1/answers" ||
				it.Questions[0].Options[0].Body["question_id"] != "q1" || it.Options[len(it.Options)-1].Label != "Open round" {
				t.Fatalf("question=%+v options=%+v", it.Questions, it.Options)
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

func TestChangeArchivableOnlyWhenEverySectionLanded(t *testing.T) {
	landed := func(ev ...string) api.ChangeSummary {
		c := api.ChangeSummary{ID: "sf1", ChangeName: "retain-drafts", Phase: "ready", Revision: "rev", ApprovedRevision: "rev", EpicID: "E", SeedIDs: []string{"E"}}
		for i, e := range ev {
			c.Sections = append(c.Sections, api.SectionSummary{ID: "s" + string(rune('1'+i)), BeadID: "S-" + string(rune('1'+i)), Evidence: e})
		}
		return c
	}
	closed := func(ids ...string) []map[string]any {
		out := []map[string]any{}
		for _, id := range ids {
			out = append(out, map[string]any{"id": id, "title": id, "status": "closed", "issue_type": "task"})
		}
		return out
	}
	for _, tc := range []struct {
		name   string
		change api.ChangeSummary
		beads  []map[string]any
		want   bool
	}{
		{"all landed", landed(api.EvidenceLanded, api.EvidenceLanded), closed("S-1", "S-2"), true},
		{"one verified only", landed(api.EvidenceLanded, api.EvidenceVerified), closed("S-1", "S-2"), false},
		{"reopened", landed(api.EvidenceLanded, api.EvidenceLanded), append(closed("S-1"), map[string]any{"id": "S-2", "status": "open"}), false},
		{"open found work", landed(api.EvidenceLanded), append(closed("S-1"), map[string]any{"id": "F-1", "status": "open", "labels": []string{"found:sf1"}}), false},
		{"closed found work", landed(api.EvidenceLanded), append(closed("S-1"), map[string]any{"id": "F-1", "status": "closed", "labels": []string{"found:sf1"}}), true},
		{"unapproved revision", func() api.ChangeSummary { c := landed(api.EvidenceLanded); c.Revision = "newer"; return c }(), closed("S-1"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.beads = issues(t, tc.beads...)
			f.change = []api.ChangeSummary{tc.change}
			items := f.b.Items(context.Background())
			got := len(items) == 1 && items[0].Kind == api.AttentionChangeArchivable
			if got != tc.want {
				t.Fatalf("archivable=%v want %v: %+v", got, tc.want, items)
			}
			if got && (items[0].Options[0].Path != "/v1/repos/r1/epics/E/finish" || items[0].Options[1].Target.Kind != "change" || items[0].BeadID != "E") {
				t.Fatalf("item=%+v", items[0])
			}
		})
	}
}

func TestStaleItemUsesTheNamedReason(t *testing.T) {
	f := newFixture(t)
	f.beads = issues(t)
	f.change = []api.ChangeSummary{{ID: "sf1", ChangeName: "x", Phase: "stale", Error: "The proposal or project context changed after review.", StaleReason: "Active change resilient-rounds changed.", SeedIDs: []string{"N-1"}}}
	items := f.b.Items(context.Background())
	if len(items) != 1 || items[0].Why != "Active change resilient-rounds changed." {
		t.Fatalf("items=%+v", items)
	}
}

func TestFoundWorkItems(t *testing.T) {
	f := newFixture(t)
	change := api.ChangeSummary{ID: "sf1", ChangeName: "retain-drafts", Phase: "ready", Revision: "r", ApprovedRevision: "r", EpicID: "E", SeedIDs: []string{"E"},
		Sections: []api.SectionSummary{{ID: "s1", BeadID: "S-1", Evidence: api.EvidenceLanded}, {ID: "s2", BeadID: "S-2", Evidence: api.EvidenceInRound}}}
	f.change = []api.ChangeSummary{change}
	f.beads = issues(t,
		map[string]any{"id": "F-1", "title": "Archive needs checks", "description": "Archive needs human checks closed.\n\nFound in: S-1", "status": "open", "labels": []string{"found:sf1", "found-amend"}},
		map[string]any{"id": "F-2", "title": "Order", "status": "open", "labels": []string{"found:sf1"}},
		map[string]any{"id": "S-2", "title": "Section two", "status": "open"},
	)
	f.rounds = []api.Round{{ID: "R1", RepoID: "r1", State: api.RoundImplementing, Beads: []api.RoundBead{
		{ID: "S-1", SpecChangeID: "sf1", State: api.BeadIntegrated},
		{ID: "S-2", SpecChangeID: "sf1", State: api.BeadRunning},
	}}}
	comments := map[string][]api.Comment{
		"S-1": {{ID: "c1", Text: "Found: export has no pagination"}, {ID: "c2", Text: "Found: answered already"}, {ID: "c3", Text: api.FoundMarker("c2") + " dismissed"}},
		"S-2": {{ID: "c4", Text: "Found: still running"}},
	}
	f.b.Comments = func(_ context.Context, _, id string) ([]api.Comment, error) { return comments[id], nil }

	items := f.b.Items(context.Background())
	var amend, comment *api.AttentionItem
	for i := range items {
		if items[i].Kind != api.AttentionFoundWork {
			t.Fatalf("unexpected item %+v", items[i])
		}
		if items[i].BeadID == "F-1" {
			amend = &items[i]
		} else {
			comment = &items[i]
		}
	}
	if len(items) != 2 || amend == nil || comment == nil {
		t.Fatalf("items=%+v", items)
	}
	if !strings.Contains(amend.Why, "reopens 1 merged section;") || amend.Options[0].Body["answer"] != "revise" || amend.Options[0].Disabled != "" {
		t.Fatalf("amend item=%+v", amend)
	}
	if tg := amend.Options[1].Target; tg.Kind != "new_effort" || tg.FollowOnOf != "F-1" || tg.Related[0] != "E" || tg.Note != "Archive needs human checks closed." {
		t.Fatalf("follow-on target=%+v", tg)
	}
	if tg := comment.Options[0].Target; comment.BeadID != "S-1" || tg.Kind != "found_something" || tg.CommentID != "c1" || tg.Note != "export has no pagination" {
		t.Fatalf("comment item=%+v", comment)
	}
	if comment.Options[1].Body["answer"] != "dismiss" {
		t.Fatalf("dismiss=%+v", comment.Options[1])
	}

	f.change[0].PendingFound = []string{"F-1"}
	f.change[0].Active = "Round R1 is still active."
	f.b.Invalidate("/src/nacre")
	for _, it := range f.b.Items(context.Background()) {
		if it.BeadID == "F-1" {
			t.Fatal("found work a revision carries is still offered for revising")
		}
	}
	f.change[0].PendingFound = nil
	f.b.Invalidate("/src/nacre")
	for _, it := range f.b.Items(context.Background()) {
		if it.BeadID == "F-1" && it.Options[0].Disabled != "Round R1 is still active." {
			t.Fatalf("Revise now not disabled during active work: %+v", it.Options[0])
		}
	}
}

func TestContextForTheMacAssistant(t *testing.T) {
	if got := Context(nil); !strings.Contains(got, "Nothing is waiting on Ryder.") {
		t.Fatalf("empty context=%q", got)
	}
	items := []api.AttentionItem{{Kind: api.AttentionProposalReady, RepoName: "nacre", BeadID: "N-5", ChangeID: "sf1", Title: "Proposal ready: retain-drafts", Why: "Approve it."}}
	got := Context(items)
	if !strings.Contains(got, "- proposal_ready [nacre N-5 sf1] Proposal ready: retain-drafts: Approve it.") {
		t.Fatalf("context=%q", got)
	}
	many := make([]api.AttentionItem, 400)
	for i := range many {
		many[i] = api.AttentionItem{Kind: "bead_decision", RepoName: "nacre", BeadID: "N-1", Title: strings.Repeat("t", 40), Why: strings.Repeat("w", 40)}
	}
	if got := Context(many); len(got) > MaxContext+100 || !strings.Contains(got, "more.") {
		t.Fatalf("unbounded context: %d bytes", len(got))
	}
}

func TestFoundCommentsOutliveTheirRound(t *testing.T) {
	f := newFixture(t)
	f.change = []api.ChangeSummary{{ID: "sf1", ChangeName: "x", Phase: "ready", EpicID: "E", SeedIDs: []string{"E"}}}
	f.rounds = []api.Round{{ID: "R1", RepoID: "r1", State: api.RoundLanded, Beads: []api.RoundBead{{ID: "S-1", SpecChangeID: "sf1", State: api.BeadIntegrated}}}}
	f.b.Comments = func(context.Context, string, string) ([]api.Comment, error) {
		return []api.Comment{{ID: "c1", Text: "Found: something outside"}}, nil
	}
	items := f.b.Items(context.Background())
	if len(items) != 1 || items[0].Kind != api.AttentionFoundWork {
		t.Fatalf("items=%+v", items)
	}
}

// A slow or failing read keeps what was last known, so Needs you doesn't
// flicker: found comments survive a comment read that fails, and a whole
// repository's items survive a bead read that fails.
func TestSlowReadsKeepWhatWasKnown(t *testing.T) {
	f := newFixture(t)
	f.change = []api.ChangeSummary{{ID: "sf1", ChangeName: "x", Phase: "ready", EpicID: "E", SeedIDs: []string{"E"}}}
	f.rounds = []api.Round{{ID: "R1", RepoID: "r1", State: api.RoundLanded, Beads: []api.RoundBead{
		{ID: "S-1", SpecChangeID: "sf1", State: api.BeadIntegrated},
		{ID: "S-2", SpecChangeID: "sf1", State: api.BeadIntegrated},
	}}}
	fail := false
	f.b.Comments = func(_ context.Context, _ string, id string) ([]api.Comment, error) {
		if fail && id == "S-1" {
			return nil, context.DeadlineExceeded
		}
		return []api.Comment{{ID: "c-" + id, Text: "Found: something in " + id}}, nil
	}
	if n := len(f.b.Items(context.Background())); n != 2 {
		t.Fatalf("first read: %d items", n)
	}
	fail = true
	f.now = f.now.Add(RefreshAfter + time.Second)
	if items := f.b.Items(context.Background()); len(items) != 2 {
		t.Fatalf("a failed comment read dropped items: %+v", items)
	}
	f.b.Issues = func(context.Context, string) ([]api.Issue, error) { return nil, context.DeadlineExceeded }
	f.now = f.now.Add(RefreshAfter + time.Second)
	if items := f.b.Items(context.Background()); len(items) != 2 {
		t.Fatalf("a failed bead read dropped items: %+v", items)
	}
}

func TestFinishedEpicsAndStuckBeads(t *testing.T) {
	f := newFixture(t)
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "nacre.yaml"), []byte("gates: []"), 0600); err != nil {
		t.Fatal(err)
	}
	f.b.Repos = func() []repos.Ref { return []repos.Ref{{ID: "r1", Name: "nacre", Path: repo}} }
	old := f.now.Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	recent := f.now.Add(-time.Hour).UTC().Format(time.RFC3339)
	working := map[string]bool{"N-busy": true}
	f.b.Working = func(_, id string) bool { return working[id] }
	f.beads = issues(t,
		map[string]any{"id": "N-E", "title": "Drafts", "issue_type": "epic", "status": "open"},
		map[string]any{"id": "N-1", "title": "a", "issue_type": "task", "status": "closed", "parent": "N-E", "updated_at": old},
		map[string]any{"id": "N-O", "title": "Open epic", "issue_type": "epic", "status": "open"},
		map[string]any{"id": "N-2", "title": "b", "issue_type": "task", "status": "open", "parent": "N-O"},
		map[string]any{"id": "N-C", "title": "Change epic", "issue_type": "epic", "status": "open"},
		map[string]any{"id": "N-3", "title": "c", "issue_type": "task", "status": "closed", "parent": "N-C"},
		map[string]any{"id": "N-stuck", "title": "Forgotten", "issue_type": "task", "status": "in_progress", "updated_at": old},
		map[string]any{"id": "N-busy", "title": "Running", "issue_type": "task", "status": "in_progress", "updated_at": old},
		map[string]any{"id": "N-new", "title": "Just claimed", "issue_type": "task", "status": "in_progress", "updated_at": recent},
	)
	f.change = []api.ChangeSummary{{ID: "c1", ChangeName: "drafts", Phase: "ready", EpicID: "N-C"}}
	byKind := map[string][]api.AttentionItem{}
	for _, it := range f.b.Items(context.Background()) {
		byKind[it.Kind] = append(byKind[it.Kind], it)
	}
	epics := byKind[api.AttentionEpicFinishable]
	if len(epics) != 1 || epics[0].BeadID != "N-E" {
		t.Fatalf("finishable epics = %+v", epics)
	}
	if o := epics[0].Options[0]; o.Method != "POST" || o.Path != "/v1/repos/r1/epics/N-E/finish" {
		t.Fatalf("finish option = %+v", o)
	}
	stuck := byKind[api.AttentionBeadStuck]
	if len(stuck) != 1 || stuck[0].BeadID != "N-stuck" {
		t.Fatalf("stuck = %+v", stuck)
	}
	work := stuck[0].Options[0]
	if work.Path != "/v1/repos/r1/rounds" || work.Body["land_when_ready"] != true {
		t.Fatalf("work option = %+v", work)
	}
	if back := stuck[0].Options[1]; back.Method != "PATCH" || back.Body["status"] != "open" {
		t.Fatalf("back option = %+v", back)
	}
}

func TestStaleButAsApprovedChangeCanStillBeFinished(t *testing.T) {
	f := newFixture(t)
	f.beads = issues(t,
		map[string]any{"id": "E", "title": "Drafts", "issue_type": "epic", "status": "open"},
		map[string]any{"id": "S1", "title": "one", "issue_type": "task", "status": "closed", "parent": "E"},
	)
	c := api.ChangeSummary{ID: "c1", ChangeName: "drafts", Phase: "stale", Revision: "r", ApprovedRevision: "r", EpicID: "E", SeedIDs: []string{"E"},
		StaleReason: "Active change other-work was added", Sections: []api.SectionSummary{{ID: "s1", BeadID: "S1", Evidence: api.EvidenceLanded}}}
	f.change = []api.ChangeSummary{c}
	if items := f.b.Items(context.Background()); len(items) != 1 || items[0].Kind != api.AttentionApprovalStale {
		t.Fatalf("a stale change with edited files: %v", kinds(items))
	}
	c.AsApproved = true
	f.change = []api.ChangeSummary{c}
	f.b.Invalidate("/src/nacre")
	items := f.b.Items(context.Background())
	if len(items) != 1 || items[0].Kind != api.AttentionChangeArchivable || items[0].Options[0].Path != "/v1/repos/r1/epics/E/finish" {
		t.Fatalf("a stale change still as approved: %+v", items)
	}
}

func TestDoneButUnclosedBeadsOfferCloseAnyway(t *testing.T) {
	f := newFixture(t)
	f.beads = issues(t,
		map[string]any{"id": "N-1", "title": "Assistant everywhere", "issue_type": "task", "status": "open"},
		map[string]any{"id": "N-2", "title": "Closed since", "issue_type": "task", "status": "closed"},
	)
	f.rounds = []api.Round{{ID: "rd1", RepoID: "r1", State: api.RoundLanded, Beads: []api.RoundBead{
		{ID: "N-1", Title: "Assistant everywhere", State: api.BeadIntegrated, CloseError: "cannot close N-1: blocked by open issues [N-9]"},
		{ID: "N-2", Title: "Closed since", State: api.BeadIntegrated, CloseError: "database is locked"},
	}}}
	var got []api.AttentionItem
	for _, it := range f.b.Items(context.Background()) {
		if it.Kind == api.AttentionBeadUnclosed {
			got = append(got, it)
		}
	}
	if len(got) != 1 || got[0].BeadID != "N-1" || !strings.Contains(got[0].Why, "blocked by open issues") {
		t.Fatalf("unclosed items = %+v", got)
	}
	if o := got[0].Options[0]; o.Path != "/v1/repos/r1/issues/N-1/close" || o.Body["force"] != true {
		t.Fatalf("close option = %+v", o)
	}
}

func TestStuckBeadsSuppressedWithoutRoundConfiguration(t *testing.T) {
	f := newFixture(t)
	f.b.Working = func(string, string) bool { return false }
	f.beads = issues(t, map[string]any{"id": "N-1", "issue_type": "task", "status": "in_progress", "updated_at": f.now.Add(-48 * time.Hour).UTC().Format(time.RFC3339)})
	repo := t.TempDir()
	f.b.Repos = func() []repos.Ref { return []repos.Ref{{ID: "r1", Path: repo}} }
	if got := f.b.Items(context.Background()); len(got) != 0 {
		t.Fatalf("missing nacre.yaml: %+v", got)
	}
	if err := os.WriteFile(filepath.Join(repo, "nacre.yaml"), []byte("gates: []"), 0600); err != nil {
		t.Fatal(err)
	}
	f.b.Invalidate(repo)
	if got := f.b.Items(context.Background()); len(got) != 1 || got[0].Kind != api.AttentionBeadStuck {
		t.Fatalf("configured repo: %+v", got)
	}
}

// A round's questions share one item; each lists its recommended answer
// first, and the item offers to accept them all once every one has one.
func TestRoundQuestionsShareOneItemWithRecommendationsFirst(t *testing.T) {
	r := api.Round{ID: "R9", State: api.RoundNeedsDecision, Beads: []api.RoundBead{{ID: "N-1", Title: "Pending proposals"}}, Questions: []api.Question{
		{ID: "q1", Beads: []string{"N-1"}, Text: "Which moment?", Options: []string{"Gap", "Drafting", "All"}, Recommended: "Drafting", Reason: "Covers the visible wait."},
		{ID: "q2", Beads: []string{"N-1"}, Text: "Spec?", Options: []string{"Amend", "Exception"}, Recommended: "Amend", Reason: "Keeps the spec true."},
		{ID: "q3", Beads: []string{"N-1"}, Text: "Done already", Options: []string{"A"}, Answer: "A"},
	}}
	items := roundItems(repos.Ref{ID: "r1", Name: "Nacre"}, r)
	if len(items) != 1 || len(items[0].Questions) != 2 {
		t.Fatalf("items=%+v", items)
	}
	q := items[0].Questions[0]
	if !q.Options[0].Recommended || q.Options[0].Label != "Drafting" || len(q.Options) != 3 || q.Reason == "" {
		t.Fatalf("recommended first: %+v", q)
	}
	if o := items[0].Options[0]; !o.Recommended || o.Path != "/v1/rounds/R9/answers/recommended" || o.Label != "Accept all 2 recommendations" {
		t.Fatalf("accept all: %+v", items[0].Options)
	}
	r.Questions[1].Recommended = ""
	if o := roundItems(repos.Ref{ID: "r1"}, r)[0].Options; len(o) != 1 || o[0].Label != "Open round" {
		t.Fatalf("accept all offered before every question has a recommendation: %+v", o)
	}
}

func TestABlockedChangeWithAGuideSaysWhatToDoNext(t *testing.T) {
	f := newFixture(t)
	f.change = []api.ChangeSummary{{ID: "sf5", ChangeName: "placeholders", Phase: "needs_attention", SeedIDs: []string{"N-8"},
		Error: "claude: Reached maximum budget ($0.85)", Status: "Reviewed three times; one point still blocks it.", Recommended: "Revise with stable order"}}
	items := f.b.Items(context.Background())
	if len(items) != 1 {
		t.Fatalf("items=%+v", items)
	}
	it := items[0]
	if it.Title != "placeholders needs your decision" || strings.Contains(it.Why, "maximum budget") ||
		it.Why != "Reviewed three times; one point still blocks it. Recommended: Revise with stable order." ||
		len(it.Options) != 1 || it.Options[0].Label != "See recommendation" || it.Options[0].Target.Kind != "change" {
		t.Fatalf("item=%+v", it)
	}
}

func TestABeadLeftForYourCheckAsksYouToSignOff(t *testing.T) {
	f := newFixture(t)
	f.beads = issues(t,
		map[string]any{"id": "N-1", "title": "Placeholder", "status": "open", "issue_type": "task", "labels": []string{"signoff"}, "metadata": map[string]any{"signoff": "Check the spinner on a device."}},
		map[string]any{"id": "N-2", "title": "Ordinary", "status": "open", "issue_type": "task"},
		map[string]any{"id": "N-3", "title": "Done", "status": "closed", "issue_type": "task", "labels": []string{"signoff"}},
	)
	items := f.b.Items(context.Background())
	if len(items) != 1 || items[0].Kind != api.AttentionBeadSignoff || items[0].BeadID != "N-1" {
		t.Fatalf("%+v", items)
	}
	it := items[0]
	if !strings.Contains(it.Why, "only your check is left: Check the spinner on a device.") {
		t.Errorf("why: %q", it.Why)
	}
	if len(it.Options) == 0 || it.Options[0].Label != "Sign off" || it.Options[0].Path != "/v1/repos/r1/issues/N-1/signoff" {
		t.Errorf("options: %+v", it.Options)
	}
}
