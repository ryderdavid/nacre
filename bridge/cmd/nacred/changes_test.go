package main

import (
	"os"
	"path/filepath"
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
