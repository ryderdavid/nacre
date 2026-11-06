package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/specflow"
)

func TestSpecRepositoryIdentityResolvesAliases(t *testing.T) {
	real := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	if !sameRepoPath(real, alias) {
		t.Fatal("repo symlink lost active work identity")
	}
	if sameRepoPath(real, t.TempDir()) {
		t.Fatal("different repos matched")
	}
}

func TestSpecEvidenceRequiresCurrentSuccessfulVerification(t *testing.T) {
	c := specflow.Change{ID: "change", Phase: "ready", Revision: "r2", ApprovedRevision: "r2"}
	s := specflow.Section{ID: "section", BeadID: "N-1"}
	base := api.Run{IssueID: "N-1", Status: api.StatusSucceeded, SpecChangeID: "change", SpecRevision: "r2", SpecSectionID: "section",
		EndedAt: "2026-10-28T01:00:00Z", Verification: &api.Verification{OK: true, At: "2026-10-28T01:01:00Z"}}
	bead := beadEvidenceState{ID: "N-1", Status: "closed", UpdatedAt: "2026-10-28T01:02:00Z"}
	if !matchingVerification(c, s, base, bead, true) {
		t.Fatal("exact verified result should count")
	}
	for _, tc := range []struct {
		name string
		edit func(*api.Run)
	}{
		{"old revision", func(r *api.Run) { r.SpecRevision = "r1" }},
		{"wrong section", func(r *api.Run) { r.SpecSectionID = "other" }},
		{"wrong change", func(r *api.Run) { r.SpecChangeID = "other" }},
		{"failed followup", func(r *api.Run) { r.Status = api.StatusFailed }},
		{"running", func(r *api.Run) { r.Status = api.StatusRunning }},
		{"unverified", func(r *api.Run) { r.Verification = nil }},
		{"old verification", func(r *api.Run) { r.EndedAt = "2026-10-28T01:03:00Z" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			tc.edit(&r)
			if matchingVerification(c, s, r, bead, true) {
				t.Fatal("unproven work counted")
			}
		})
	}
	if matchingVerification(c, s, base, bead, false) {
		t.Fatal("unreadable Beads counted")
	}
	bead.Status = "open"
	if matchingVerification(c, s, base, bead, true) {
		t.Fatal("reopened work counted")
	}
	bead.UpdatedAt = "2026-10-28T00:59:00Z"
	if !matchingVerification(c, s, base, bead, true) {
		t.Fatal("verified open work should count without implying landed")
	}
	c.Revision = "r3"
	if matchingVerification(c, s, base, bead, true) {
		t.Fatal("stale approval counted")
	}
}

func TestSpecExecutionContractKeepsReviewedBytesAndSection(t *testing.T) {
	c := specflow.Contract{ChangeID: "c", Revision: "r", SectionID: "s", BeadID: "N-1", ChangeName: "change",
		Sections:  []specflow.Section{{ID: "s", Title: "Storage", Body: "- [ ] Preserve drafts"}},
		Artifacts: []specflow.Artifact{{Path: "openspec/changes/change/proposal.md", Content: "exact\nbytes\n"}}}
	got := executionContract(c)
	if got.SectionBody != c.Sections[0].Body || got.Artifacts[0].Content != c.Artifacts[0].Content || got.Revision != "r" {
		t.Fatalf("contract lost reviewed context: %#v", got)
	}
}

func TestSpecEvidenceDistinguishesActiveStaleAndBuilt(t *testing.T) {
	path := t.TempDir()
	c := specflow.Change{ID: "c", Phase: "ready", Revision: "r", ApprovedRevision: "r", Sections: []specflow.Section{{ID: "s", BeadID: "N-1"}}}
	v := &api.Verification{OK: true, At: "2026-10-28T01:01:00Z"}
	run := api.Run{ID: "run", IssueID: "N-1", Status: api.StatusSucceeded, SpecChangeID: "c", SpecRevision: "r", SpecSectionID: "s", EndedAt: "2026-10-28T01:00:00Z", Verification: v}
	r := api.Round{ID: "round", RepoPath: path, State: api.RoundReadyToLand, FinalCommit: "final", Beads: []api.RoundBead{{ID: "N-1", SpecChangeID: "c", SpecRevision: "r", SpecSectionID: "s", RunID: "run", State: api.BeadIntegrated, MergeCommit: "merge", Verification: v}}}
	states := map[string]beadEvidenceState{"N-1": {ID: "N-1", Status: "open", UpdatedAt: "2026-10-28T00:59:00Z"}}
	read := func(string) (api.Run, error) { return run, nil }
	assess := func() specflow.Evidence { return assessChangeEvidence(path, c, []api.Round{r}, states, nil, read)[0] }
	e := assess()
	if !e.Verified || !e.BuildVerified || e.Landed {
		t.Fatalf("built pending landing: %#v", e)
	}
	r.FinalCommit = ""
	e = assess()
	if !e.Verified || e.BuildVerified {
		t.Fatalf("individual verification is not a final build: %#v", e)
	}
	r.Beads[0].State = api.BeadParked
	if assess().Verified {
		t.Fatal("parked work retained old verification")
	}
	c.Phase = "stale"
	c.Revision = "new"
	r.State = api.RoundPaused
	e = assess()
	if e.RoundID != "round" || e.RoundState != api.RoundPaused || e.Verified || e.BuildVerified || e.Landed {
		t.Fatalf("stale active work must remain reachable without approval: %#v", e)
	}
}

func TestSpecEvidenceCountsSectionsFinishedBeforeAdoption(t *testing.T) {
	done := specflow.Section{ID: "done", BeadID: "N-1", Body: "- [x] Save on Back\n- [x] Save on rotation"}
	open := specflow.Section{ID: "open", BeadID: "N-2", Body: "- [x] Restore\n- [ ] Restore after restart"}
	c := specflow.Change{ID: "change", Phase: "ready", Revision: "r1", ApprovedRevision: "r1", Sections: []specflow.Section{done, open},
		Adoption: &specflow.Adoption{Completed: []string{"done", "open"}}}
	states := map[string]beadEvidenceState{"N-1": {ID: "N-1", Status: "closed"}, "N-2": {ID: "N-2", Status: "closed"}}
	read := func(string) (api.Run, error) { return api.Run{}, os.ErrNotExist }
	got := assessChangeEvidence("/repo", c, nil, states, nil, read)
	if !got[0].Landed || specflow.EvidenceState(got[0]) != api.EvidenceLanded {
		t.Fatalf("finished section before adoption: %+v", got[0])
	}
	if got[1].Landed {
		t.Fatal("a section with an unchecked task counted as finished")
	}
	states["N-1"] = beadEvidenceState{ID: "N-1", Status: "open"}
	if got = assessChangeEvidence("/repo", c, nil, states, nil, read); got[0].Landed {
		t.Fatal("a reopened bead still counted as finished")
	}
	states["N-1"] = beadEvidenceState{ID: "N-1", Status: "closed"}
	c.Phase, c.AsApproved = "stale", true
	if got = assessChangeEvidence("/repo", c, nil, states, nil, read); !got[0].Landed {
		t.Fatal("finished work stopped counting because other specifications moved on")
	}
	c.AsApproved = false
	if got = assessChangeEvidence("/repo", c, nil, states, nil, read); got[0].Landed {
		t.Fatal("evidence counted for a change whose own files changed")
	}
	c.Phase, c.ApprovedRevision = "ready", "r0"
	if got = assessChangeEvidence("/repo", c, nil, states, nil, read); got[0].Landed {
		t.Fatal("evidence counted without a current approval")
	}
}

// NAC-pjq.8: a section whose round landed with its work merged says so even
// once the change went stale, so its progress agrees with the round; it
// still isn't Landed (accepted) without a current approval.
func TestSpecEvidenceSaysTheRoundMergedAStaleChangesSection(t *testing.T) {
	path := t.TempDir()
	c := specflow.Change{ID: "c", Phase: "stale", Revision: "r", ApprovedRevision: "r", Sections: []specflow.Section{{ID: "s", BeadID: "N-1"}, {ID: "t", BeadID: "N-2"}}}
	v := &api.Verification{OK: true, At: "2026-10-28T01:01:00Z"}
	r := api.Round{ID: "rdtn", RepoPath: path, State: api.RoundLanded, FinalCommit: "final", LandedCommit: "landed", Beads: []api.RoundBead{
		{ID: "N-1", SpecChangeID: "c", SpecRevision: "r", SpecSectionID: "s", State: api.BeadIntegrated, MergeCommit: "merge", Verification: v},
		{ID: "N-2", SpecChangeID: "c", SpecRevision: "r", SpecSectionID: "t", State: api.BeadParked},
	}}
	read := func(string) (api.Run, error) { return api.Run{}, errors.New("gone") }
	got := assessChangeEvidence(path, c, []api.Round{r}, nil, nil, read)
	if !got[0].Merged || got[0].Landed || specflow.EvidenceState(got[0]) == api.EvidenceLanded || !strings.Contains(got[0].Note, "Round rdtn: landed") {
		t.Fatalf("merged section of a stale change: %+v", got[0])
	}
	if got[1].Merged {
		t.Fatalf("a parked bead wasn't merged: %+v", got[1])
	}
}

func TestSpecEvidenceKeepsArchivedSectionsLanded(t *testing.T) {
	path := t.TempDir()
	c := specflow.Change{ID: "c", Phase: "archived", Revision: "r", ApprovedRevision: "r",
		Sections: []specflow.Section{{ID: "s", BeadID: "N-1"}, {ID: "old", BeadID: "N-2", Body: "- [x] Done"}},
		Adoption: &specflow.Adoption{Completed: []string{"old"}}}
	v := &api.Verification{OK: true, At: "2026-10-28T01:01:00Z"}
	run := api.Run{ID: "run", IssueID: "N-1", Status: api.StatusSucceeded, SpecChangeID: "c", SpecRevision: "r", SpecSectionID: "s", EndedAt: "2026-10-28T01:00:00Z", Verification: v}
	r := api.Round{ID: "round", RepoPath: path, State: api.RoundLanded, FinalCommit: "final", LandedCommit: "landed",
		Beads: []api.RoundBead{{ID: "N-1", SpecChangeID: "c", SpecRevision: "r", SpecSectionID: "s", RunID: "run", State: api.BeadIntegrated, MergeCommit: "merge", Verification: v}}}
	states := map[string]beadEvidenceState{"N-1": {ID: "N-1", Status: "closed"}, "N-2": {ID: "N-2", Status: "closed"}}
	read := func(string) (api.Run, error) { return run, nil }
	got := assessChangeEvidence(path, c, []api.Round{r}, states, nil, read)
	for _, e := range got {
		if !e.Verified || !e.BuildVerified || !e.Landed || specflow.EvidenceState(e) != api.EvidenceLanded {
			t.Fatalf("archived section lost its landed evidence: %+v", e)
		}
	}
	c.ApprovedRevision = "r0"
	for _, e := range assessChangeEvidence(path, c, []api.Round{r}, states, nil, read) {
		if e.Landed {
			t.Fatalf("archived change without a matching approval counted as landed: %+v", e)
		}
	}
	c.Phase, c.ApprovedRevision = "draft", "r"
	for _, e := range assessChangeEvidence(path, c, []api.Round{r}, states, nil, read) {
		if e.Landed {
			t.Fatalf("unapproved phase counted as landed: %+v", e)
		}
	}
}
