package specflow

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

const untrackedProposal = "## Why\n\nGates are set up by hand today.\n\n## What Changes\n\n- Draft nacre.yaml\n"

// NAC-pjq.3: the Spec panel lists the checkout's active changes, not only
// those a workflow record tracks. Each untracked one gets its phase from its
// files and tasks.
func TestTheCheckoutsUntrackedChangesAreListed(t *testing.T) {
	repo, _, _, opts := fixture(t)
	m := newManager(t, opts)
	tracked := develop(t, m, repo)

	write(t, repo, "openspec/changes/set-up-gates/proposal.md", untrackedProposal)
	write(t, repo, "openspec/changes/set-up-gates/tasks.md", "# Tasks\n\n## 1. Draft and save nacre.yaml\n\n- [x] 1.1 Draft\n- [x] 1.2 Save\n")
	write(t, repo, "openspec/changes/set-up-gates/specs/gates/spec.md", "## ADDED Requirements\n")
	write(t, repo, "openspec/changes/half-done/proposal.md", untrackedProposal)
	write(t, repo, "openspec/changes/half-done/tasks.md", "## 1. First\n\n- [x] 1.1 a\n\n## 2. Second (human)\n\n- [ ] 2.1 b\n- [x] 2.2 c\n\n## Workflow follow-up\n\n- Archive it\n")
	write(t, repo, "openspec/changes/fresh/proposal.md", untrackedProposal)
	write(t, repo, "openspec/changes/fresh/tasks.md", "## 1. Only\n\n- [ ] 1.1 a\n")
	write(t, repo, "openspec/changes/fresh/review.md", "PASS\n")
	write(t, repo, "openspec/changes/idea/tasks.md", "## 1. Only\n\n- [ ] 1.1 a\n")
	write(t, repo, "openspec/changes/leftover/proposal.md", untrackedProposal)
	write(t, repo, "openspec/changes/leftover/tasks.md", "## 1. Only\n\n- [ ] 1.1 a\n")
	write(t, repo, "openspec/changes/archive/2026-11-02-leftover/proposal.md", untrackedProposal)
	write(t, repo, "openspec/changes/.hidden/proposal.md", untrackedProposal)
	write(t, repo, "openspec/changes/empty/notes.txt", "nothing canonical\n")

	list, err := m.Untracked(repo)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]api.ChangeSummary{}
	var names []string
	for _, c := range list {
		got[c.ChangeName] = c
		names = append(names, c.ChangeName)
	}
	if strings.Join(names, ",") != "fresh,half-done,idea,leftover,set-up-gates" {
		t.Fatalf("untracked=%v (tracked %s must not be listed)", names, tracked.ChangeName)
	}
	for _, c := range list {
		if !c.Untracked || c.ID != c.ChangeName || len(c.SeedIDs) != 0 || c.EpicID != "" || c.Revision == "" || c.UpdatedAt == "" {
			t.Fatalf("untracked summary=%+v", c)
		}
	}
	if c := got["set-up-gates"]; c.Phase != "ready" || c.TasksDone != 2 || c.TasksTotal != 2 || len(c.Sections) != 1 || c.Sections[0].Evidence != api.EvidenceLanded {
		t.Fatalf("every task checked reads as ready to archive: %+v", c)
	}
	if c := got["half-done"]; c.Phase != "ready" || c.TasksDone != 2 || c.TasksTotal != 3 || len(c.Sections) != 2 ||
		c.Sections[0].Evidence != api.EvidenceLanded || c.Sections[1].Evidence != api.EvidenceNone || c.Sections[1].Title != "2. Second (human)" {
		t.Fatalf("in progress, by section: %+v", c)
	}
	if c := got["fresh"]; c.Phase != "awaiting_approval" || !c.HasReview || c.Sections[0].Evidence != api.EvidenceNone {
		t.Fatalf("written, nothing started: %+v", c)
	}
	if c := got["idea"]; c.Phase != "drafting" {
		t.Fatalf("no proposal yet: %+v", c)
	}
	if c := got["leftover"]; c.ArchivedAs != "2026-11-02-leftover" {
		t.Fatalf("a copy of an archived change says so: %+v", c)
	}

	// Its overview and reader: every canonical file, sections from tasks.md.
	env, err := m.UntrackedChange(repo, "half-done")
	if err != nil || !env.Available || env.Change == nil {
		t.Fatalf("env=%+v err=%v", env, err)
	}
	c := env.Change
	if !c.Untracked || c.Phase != "ready" || c.Summary != "Gates are set up by hand today." || len(c.Artifacts) != 2 ||
		len(c.Sections) != 2 || !strings.Contains(c.Sections[1].Body, "2.1 b") || c.Sections[0].ID == c.Sections[1].ID {
		t.Fatalf("change=%+v", c)
	}
	for _, name := range []string{tracked.ChangeName, "missing", "../" + tracked.ChangeName, "archive"} {
		if _, err := m.UntrackedChange(repo, name); !errors.Is(err, ErrNoChange) {
			t.Fatalf("%q: err=%v", name, err)
		}
	}

	// Once its record is archived, a folder of that name is the checkout's own.
	m.mu.Lock()
	for _, r := range m.records {
		r.Change.Phase = "archived"
	}
	m.mu.Unlock()
	list, _ = m.Untracked(repo)
	if len(list) != 6 {
		t.Fatalf("after archiving the record: %+v", list)
	}
}

// A written change whose folder is gone from the checkout says so; one still
// being drafted has no folder yet and isn't missing.
func TestSummariesSayWhenAChangesFolderIsGone(t *testing.T) {
	repo, _, _, opts := fixture(t)
	m := newManager(t, opts)
	c := develop(t, m, repo)
	list, _, _ := m.Summaries(repo)
	if len(list) != 1 || list[0].Missing {
		t.Fatalf("%+v", list)
	}
	if err := os.RemoveAll(filepath.Join(repo, "openspec", "changes", c.ChangeName)); err != nil {
		t.Fatal(err)
	}
	list, _, _ = m.Summaries(repo)
	if !list[0].Missing {
		t.Fatalf("folder gone: %+v", list[0])
	}
	closed := func(id string) bool { return true }
	if !list[0].Withdrawn(closed) || list[0].Withdrawn(func(string) bool { return false }) {
		t.Fatal("withdrawn only when every owning bead is closed")
	}
}
