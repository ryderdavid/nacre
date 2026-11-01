package specflow

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/gitx"
)

func TestArchiveIntoCommitsTheArchiveAndEndsTheWorkflow(t *testing.T) {
	if _, err := exec.LookPath("openspec"); err != nil {
		t.Skip("the OpenSpec CLI isn't installed")
	}
	repo, _, _, m := adoptFixture(t)
	ctx := context.Background()
	for _, a := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "t@e.com"}, {"config", "user.name", "T"}, {"config", "commit.gpgsign", "false"}} {
		if _, err := gitx.Run(ctx, repo, a...); err != nil {
			t.Fatal(err)
		}
	}
	write(t, repo, "openspec/changes/retain-drafts/proposal.md", "# Retain drafts\n\n## Why\nDrafts vanish when people navigate away.\n\n## What Changes\n- Keep them.\n\n## Capabilities\n\n### New Capabilities\n- `drafts`: retaining drafts\n\n### Modified Capabilities\n\n## Impact\nThe editor.\n")
	c := approve(t, m, repo, act(t, m, repo, "NAC-1", Action{Action: "adopt", ChangeName: "retain-drafts", RecordedReview: true}))
	if _, err := gitx.CommitAll(ctx, repo, "work"); err != nil {
		t.Fatal(err)
	}

	name, err := m.ArchiveInto(ctx, repo, repo, "NAC-2")
	if err != nil || name != "" {
		t.Fatalf("an epic without a change: %q %v", name, err)
	}
	if name, err = m.ArchiveInto(ctx, repo, repo, c.EpicID); err != nil || name != "retain-drafts" {
		t.Fatalf("archive: %q %v", name, err)
	}
	if _, err := os.Stat(filepath.Join(repo, "openspec/changes/retain-drafts")); !os.IsNotExist(err) {
		t.Fatal("the change is still active")
	}
	spec, err := os.ReadFile(filepath.Join(repo, "openspec/specs/drafts/spec.md"))
	if err != nil {
		t.Fatal("the change's requirements weren't merged into the accepted specs")
	}
	if strings.Contains(string(spec), "TBD") || !strings.Contains(string(spec), "Drafts vanish when people navigate away") {
		t.Fatalf("the new spec's Purpose wasn't filled in from the change's Why:\n%s", spec)
	}
	if dirty, _ := gitx.Dirty(ctx, repo, ".beads/"); len(dirty) != 0 {
		t.Fatalf("the archive wasn't committed: %v", dirty)
	}
	if name, err = m.ArchiveInto(ctx, repo, repo, c.EpicID); err != nil || name != "retain-drafts" {
		t.Fatalf("archiving again must do nothing: %q %v", name, err)
	}

	m.MarkArchived(repo, c.EpicID)
	env, err := m.Get(ctx, repo, "NAC-1")
	if err != nil || env.Change.Phase != "archived" {
		t.Fatalf("after archive: %+v %v", env.Change, err)
	}
}

// Other specifications moving on stales an approval but leaves finished work
// archivable; editing the change's own files does not.
func TestFinishedWorkStaysArchivableWhenOnlyOtherSpecsMoved(t *testing.T) {
	repo, _, _, m := adoptFixture(t)
	ctx := context.Background()
	c := approve(t, m, repo, act(t, m, repo, "NAC-1", Action{Action: "adopt", ChangeName: "retain-drafts", RecordedReview: true}))
	write(t, repo, "openspec/changes/other-work/proposal.md", "## Why\nSomething else.\n")
	env, err := m.Get(ctx, repo, "NAC-1")
	if err != nil || env.Change.Phase != "stale" || !env.Change.AsApproved {
		t.Fatalf("after another change was added: phase=%s asApproved=%v err=%v", env.Change.Phase, env.Change.AsApproved, err)
	}
	if list, _, _ := m.Summaries(repo); len(list) != 1 || !list[0].AsApproved {
		t.Fatalf("summary doesn't say the change is as approved: %+v", list)
	}
	write(t, repo, "openspec/changes/retain-drafts/design.md", "Rewritten by hand.\n")
	env, _ = m.Get(ctx, repo, "NAC-1")
	if env.Change.AsApproved {
		t.Fatal("a change whose own files were edited is still as approved")
	}
	if _, err := m.ArchiveInto(ctx, repo, repo, c.EpicID); err == nil || !strings.Contains(err.Error(), "changed since its approved revision") {
		t.Fatalf("archiving an edited change: %v", err)
	}
}
