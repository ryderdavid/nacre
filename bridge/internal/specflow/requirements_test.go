package specflow

import (
	"strings"
	"testing"
)

func TestRequirementIndexListsAcceptedAndActiveRequirements(t *testing.T) {
	repo, _, _, _ := fixture(t)
	write(t, repo, "openspec/specs/boards/spec.md", "## Purpose\nBoards.\n\n### Requirement: Enter inserts a newline\nIt SHALL.\n")
	write(t, repo, "openspec/changes/chat/specs/composer/spec.md", "## ADDED Requirements\n\n### Requirement: Send button sends\nIt SHALL.\n")
	write(t, repo, "openspec/changes/archive/old/specs/x/spec.md", "### Requirement: Archived\n")
	ix, err := RequirementIndex(repo)
	if err != nil || len(ix.Requirements) != 2 || len(ix.Omitted) != 0 {
		t.Fatalf("index=%+v err=%v", ix, err)
	}
	first := ix.Requirements[0]
	if first.Capability != "boards" || first.Title != "Enter inserts a newline" || first.Path != "openspec/specs/boards/spec.md" || first.Line != 4 {
		t.Fatalf("accepted requirement=%+v", first)
	}
	if ix.Requirements[1].Capability != "composer (change chat)" {
		t.Fatalf("active change requirement=%+v", ix.Requirements[1])
	}
	if _, ok := ix.Find("openspec/specs/boards/spec.md", "Enter inserts a newline"); !ok {
		t.Fatal("Find missed a listed requirement")
	}
	if !strings.Contains(ix.Text(), "openspec/specs/boards/spec.md:4 Requirement: Enter inserts a newline") {
		t.Fatalf("text=%s", ix.Text())
	}
}

func TestRequirementIndexTruncatesWholeCapabilities(t *testing.T) {
	repo, _, _, _ := fixture(t)
	var big strings.Builder
	for i := 0; i < 400; i++ {
		big.WriteString("### Requirement: A long requirement title that takes room number " + strings.Repeat("x", 20) + "\n")
	}
	write(t, repo, "openspec/specs/aaa/spec.md", big.String())
	write(t, repo, "openspec/specs/bbb/spec.md", "### Requirement: Small\n")
	ix, err := RequirementIndex(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Omitted) != 1 || ix.Omitted[0] != "aaa" || len(ix.Requirements) != 1 || ix.Requirements[0].Capability != "bbb" {
		t.Fatalf("omitted=%v reqs=%d", ix.Omitted, len(ix.Requirements))
	}
	if len(ix.Text()) > MaxRequirementIndex {
		t.Fatal("index exceeds its bound")
	}
}

func TestRequirementIndexNeedsValidOpenSpec(t *testing.T) {
	if _, err := RequirementIndex(t.TempDir()); err == nil {
		t.Fatal("a repository without OpenSpec has an index")
	}
}
