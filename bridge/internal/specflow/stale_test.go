package specflow

import (
	"context"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/bd"
)

func TestStaleApprovalsNameTheirCause(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(t *testing.T, repo string, b *fakeBeads, c Change)
		want string
	}{
		{"goal", func(t *testing.T, repo string, _ *fakeBeads, _ Change) { write(t, repo, "AGENTS.md", "A new goal") }, "AGENTS.md changed."},
		{"other change", func(t *testing.T, repo string, _ *fakeBeads, _ Change) {
			write(t, repo, "openspec/changes/resilient-rounds/proposal.md", "## Why\nRounds.\n")
		}, "Active change resilient-rounds was added."},
		{"accepted spec", func(t *testing.T, repo string, _ *fakeBeads, _ Change) {
			write(t, repo, "openspec/specs/boards/spec.md", "## Purpose\nBoards.\n")
		}, "Accepted spec boards changed."},
		{"plan ticked", func(t *testing.T, repo string, _ *fakeBeads, c Change) {
			for _, a := range c.Artifacts {
				if strings.HasSuffix(a.Path, "/tasks.md") {
					write(t, repo, a.Path, strings.Replace(a.Content, "- [ ]", "- [x]", 1))
				}
			}
		}, "tasks.md was edited after approval. Progress is tracked in beads; revise to change the plan."},
		{"seed", func(t *testing.T, _ string, b *fakeBeads, _ Change) {
			b.edit("NAC-1", func(is *issue) { is.Description += " and accounts" })
		}, "Seed bead NAC-1 changed."},
		{"section", func(t *testing.T, repo string, b *fakeBeads, c Change) {
			_ = b.Update(context.Background(), repo, c.Sections[0].BeadID, bd.UpdateParams{Title: ptr("Renamed by hand")})
		}, "was edited after its beads were synchronized."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, b, _, opts := fixture(t)
			m := newManager(t, opts)
			c := approve(t, m, repo, develop(t, m, repo))
			tc.edit(t, repo, b, c)
			m.Refresh(context.Background(), repo)
			list, _, err := m.Summaries(repo)
			if err != nil || len(list) != 1 || list[0].Phase != "stale" || !strings.Contains(list[0].StaleReason, tc.want) {
				t.Fatalf("summaries=%+v err=%v; want reason containing %q", list, err, tc.want)
			}
			env, _ := m.Get(context.Background(), repo, "NAC-1")
			if env.Change.StaleReason != list[0].StaleReason {
				t.Fatalf("change detail reason %q differs from summary %q", env.Change.StaleReason, list[0].StaleReason)
			}
		})
	}
}

func TestReviewAgainClearsTheStaleReason(t *testing.T) {
	repo, _, _, opts := fixture(t)
	m := newManager(t, opts)
	approve(t, m, repo, develop(t, m, repo))
	write(t, repo, "AGENTS.md", "A new goal")
	m.Refresh(context.Background(), repo)
	c := act(t, m, repo, "NAC-1", Action{Action: "revise"})
	if c.StaleReason != "" || c.Phase != "awaiting_approval" {
		t.Fatalf("after review: phase=%s reason=%q", c.Phase, c.StaleReason)
	}
}

func TestStaleReasonWithoutBaselineIsGeneric(t *testing.T) {
	if got := fileCauses("x", map[string]string{"AGENTS.md": "a"}, map[string]string{"AGENTS.md": "a"}); len(got) != 0 {
		t.Fatalf("no difference reported causes %v", got)
	}
	repo, _, _, opts := fixture(t)
	m := newManager(t, opts)
	r := &record{Repo: repo}
	if got := m.staleReason(context.Background(), r, ""); got != GenericStaleReason {
		t.Fatalf("reason=%q", got)
	}
}

// Model Beads normalizing the text nacred writes (for example trimming a
// trailing newline). The approval baseline must describe that stored text.
type normalizingBeads struct{ *fakeBeads }

func (b normalizingBeads) normalize(id string) {
	b.edit(id, func(is *issue) {
		is.Description = strings.TrimSpace(is.Description) + "\n"
		is.Design = strings.TrimSpace(is.Design) + "\n"
	})
}
func (b normalizingBeads) Create(ctx context.Context, repo string, p bd.CreateParams) (string, error) {
	id, err := b.fakeBeads.Create(ctx, repo, p)
	if err == nil {
		b.normalize(id)
	}
	return id, err
}
func (b normalizingBeads) Update(ctx context.Context, repo, id string, p bd.UpdateParams) error {
	err := b.fakeBeads.Update(ctx, repo, id, p)
	if err == nil && (p.Description != nil || p.Design != nil) {
		b.normalize(id)
	}
	return err
}

func TestOwnSectionSyncDoesNotStaleApproval(t *testing.T) {
	repo, b, _, opts := fixture(t)
	opts.Beads = normalizingBeads{b}
	m := newManager(t, opts)
	c := approve(t, m, repo, develop(t, m, repo))
	for pass := 0; pass < 2; pass++ {
		if pass == 1 {
			c = approve(t, m, repo, act(t, m, repo, c.SeedID, Action{Action: "revise", Feedback: "Clarify the implementation"}))
		}
		// Normal execution adds labels, notes, and comments after promotion.
		for _, s := range c.Sections {
			if err := b.Update(context.Background(), repo, s.BeadID, bd.UpdateParams{AddLabels: []string{"round:r1"}}); err != nil {
				t.Fatal(err)
			}
			b.edit(s.BeadID, func(is *issue) { is.Notes = "Verified a gate" })
		}
		m.Refresh(context.Background(), repo)
		env, err := m.Get(context.Background(), repo, c.SeedID)
		if err != nil || env.Change.Phase != "ready" {
			t.Fatalf("own sync staled approval: %+v %v", env, err)
		}
		// Persisted read-back baselines survive reload too.
		reloaded := newManager(t, opts)
		if _, err := reloaded.Contract(context.Background(), repo, c.Sections[0].BeadID); err != nil {
			t.Fatal(err)
		}
	}
	b.edit(c.Sections[0].BeadID, func(is *issue) { is.Description += "External scope change" })
	m.Refresh(context.Background(), repo)
	env, err := m.Get(context.Background(), repo, c.SeedID)
	if err != nil || env.Change.Phase != "stale" || !strings.Contains(env.Change.StaleReason, "Section bead") {
		t.Fatalf("external edit was accepted: %+v %v", env, err)
	}
}
