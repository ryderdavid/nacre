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
