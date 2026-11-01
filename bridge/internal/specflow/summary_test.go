package specflow

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

func TestSummariesListPlannedFirstWithAliasesAndEvidence(t *testing.T) {
	repo, b, _, opts := fixture(t)
	var mu sync.Mutex
	calls := 0
	opts.Evidence = func(_ string, c Change) []Evidence {
		mu.Lock()
		calls++
		mu.Unlock()
		out := []Evidence{}
		for i, s := range c.Sections {
			out = append(out, Evidence{SectionID: s.ID, BeadID: s.BeadID, Landed: i == 0, RoundID: "r1", RoundState: api.RoundImplementing})
		}
		return out
	}
	stamp := "one"
	opts.EvidenceStamp = func(string) string { return stamp }
	m := newManager(t, opts)
	first := approve(t, m, repo, develop(t, m, repo))
	b.edit("NAC-2", func(is *issue) {
		*is = issue{ID: "NAC-2", Title: "Second", Description: "Another effort", Type: "task", Status: "open"}
	})
	m.mu.Lock()
	for _, r := range m.records {
		r.Aliases = append(r.Aliases, "NAC-9")
	}
	m.mu.Unlock()
	mu.Lock()
	calls = 0
	mu.Unlock()

	list, total, err := m.Summaries(repo)
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("summaries=%+v total=%d err=%v", list, total, err)
	}
	s := list[0]
	if s.ID != first.ID || s.EpicID != first.EpicID || s.Phase != "ready" || len(s.SeedIDs) != 2 || s.SeedIDs[1] != "NAC-9" {
		t.Fatalf("summary=%+v", s)
	}
	if s.Sections[0].Evidence != api.EvidenceLanded || s.Sections[1].Evidence != api.EvidenceInRound {
		t.Fatalf("evidence=%+v", s.Sections)
	}
	m.Summaries(repo)
	if calls != 1 {
		t.Fatalf("evidence recomputed with an unchanged stamp: %d calls", calls)
	}
	stamp = "two"
	m.Summaries(repo)
	if calls != 2 {
		t.Fatalf("evidence not recomputed after rounds moved: %d calls", calls)
	}
}

func TestPhaseTransitionsAreReported(t *testing.T) {
	repo, _, _, opts := fixture(t)
	var mu sync.Mutex
	var phases []string
	opts.Changed = func(_ string, c Change) {
		mu.Lock()
		phases = append(phases, c.Phase)
		mu.Unlock()
	}
	m := newManager(t, opts)
	approve(t, m, repo, develop(t, m, repo))
	mu.Lock()
	defer mu.Unlock()
	seen := map[string]bool{}
	for i, p := range phases {
		if i > 0 && phases[i-1] == p {
			t.Fatalf("phase %s reported twice in a row: %v", p, phases)
		}
		seen[p] = true
	}
	for _, want := range []string{"drafting", "awaiting_approval", "promoting", "ready"} {
		if !seen[want] {
			t.Fatalf("missing %s in %v", want, phases)
		}
	}
}

func TestChangeGetCarriesInventorySummary(t *testing.T) {
	repo, _, _, opts := fixture(t)
	write(t, repo, "openspec/specs/boards/spec.md", "## Purpose\nBoards.\n")
	write(t, repo, "openspec/changes/other/proposal.md", "## Why\nOther.\n")
	write(t, repo, "openspec/changes/other/tasks.md", "## 1. Work\n- [ ] 1.1 Do it\n")
	m := newManager(t, opts)
	env, err := m.Get(context.Background(), repo, "NAC-1")
	if err != nil || env.Inventory == nil || env.Inventory.AcceptedSpecs != 1 || env.Inventory.ActiveChanges != 1 {
		t.Fatalf("inventory=%+v err=%v", env.Inventory, err)
	}
}

func TestAmendingSeedClosesOnceItsRevisionIsApproved(t *testing.T) {
	repo, b, model, opts := fixture(t)
	m := newManager(t, opts)
	first := approve(t, m, repo, develop(t, m, repo))
	b.edit("NAC-2", func(is *issue) {
		*is = issue{ID: "NAC-2", Title: "Restore focus", Description: "Restore keyboard focus with the draft", Type: "task", Status: "open"}
	})
	model.fn = func(ctx context.Context, h claude.Headless, n int) (any, float64, error) {
		if h.System == authorSystem {
			a := goodAuthor(first)
			for i := range a.Artifacts {
				if strings.HasSuffix(a.Artifacts[i].Path, "/design.md") {
					a.Artifacts[i].Content += "\nRestore keyboard focus."
				}
			}
			return a, 0.1, nil
		}
		return normalModel(ctx, h, n)
	}
	second := act(t, m, repo, "NAC-2", Action{Action: "develop"})
	b.edit("NAC-2", func(is *issue) {
		if is.Status == "closed" {
			t.Fatal("seed closed before its revision was approved")
		}
	})
	approve(t, m, repo, second)
	b.edit("NAC-2", func(is *issue) {
		if is.Status != "closed" {
			t.Fatalf("amending seed still %s after approval", is.Status)
		}
	})
	b.edit("NAC-1", func(is *issue) {
		if is.Status == "closed" {
			t.Fatal("the original seed (the epic) was closed")
		}
	})
	list, _, _ := m.Summaries(repo)
	if len(list) != 1 || len(list[0].SeedIDs) != 2 || list[0].SeedIDs[1] != "NAC-2" {
		t.Fatalf("summary seeds=%+v", list)
	}
}
