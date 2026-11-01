package specflow

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
	if _, err := os.Stat(filepath.Join(repo, "openspec/specs/drafts/spec.md")); err != nil {
		t.Fatal("the change's requirements weren't merged into the accepted specs")
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
