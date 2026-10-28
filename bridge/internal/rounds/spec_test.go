package rounds

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

func specFixture() (api.Round, api.SpecContract) {
	c := api.SpecContract{ChangeID: "wf-1", Revision: "rev-1", SectionID: "sec-1", BeadID: "T-1", ChangeName: "test-change", SectionTitle: "Backend", SectionBody: "- [ ] Implement behavior", Summary: "Preserve drafts", Rationale: "Avoid losing work", Artifacts: []api.SpecArtifact{
		{Path: "openspec/changes/test-change/proposal.md", Content: "## Why\nRetain drafts.\n"},
		{Path: "openspec/changes/test-change/design.md", Content: "## Design\nRetain automatically.\n"},
		{Path: "openspec/changes/test-change/tasks.md", Content: "<!-- nacre-section: sec-1 -->\n## Backend\n- [ ] Implement behavior\n"},
		{Path: "openspec/changes/test-change/specs/drafts/spec.md", Content: "## ADDED Requirements\nKeep drafts.\n"},
	}}
	meta := api.Round{RepoPath: "repo", SpecContracts: map[string]api.SpecContract{"T-1": c}, Beads: []api.RoundBead{{ID: "T-1", SpecChangeID: c.ChangeID, SpecRevision: c.Revision, SpecSectionID: c.SectionID}}}
	return meta, c
}

func TestSpecSelectionRejectsMixedAndStale(t *testing.T) {
	m, c := specFixture()
	for _, pair := range [][2]string{{"", ""}, {c.ChangeID, ""}, {"", c.Revision}} {
		if err := validateSpecRequest(m.Beads, m.SpecContracts, pair[0], pair[1]); err == nil {
			t.Fatal("accepted omitted approval identity")
		}
	}
	if err := validateSpecRequest(m.Beads, m.SpecContracts, c.ChangeID, c.Revision); err != nil {
		t.Fatal(err)
	}
	if err := validateSpecSelection(m.Beads, m.SpecContracts, c.ChangeID, c.Revision); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name             string
		change, revision string
		beads            []api.RoundBead
		contracts        map[string]api.SpecContract
	}{
		{"stale", c.ChangeID, "old", m.Beads, m.SpecContracts},
		{"wrong change", "different", c.Revision, m.Beads, m.SpecContracts},
		{"mixed", "", "", append(append([]api.RoundBead{}, m.Beads...), api.RoundBead{ID: "T-2"}), m.SpecContracts},
		{"missing snapshot", "", "", m.Beads, nil},
		{"missing provenance", "", "", []api.RoundBead{{ID: "T-1"}}, m.SpecContracts},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateSpecSelection(tt.beads, tt.contracts, tt.change, tt.revision); err == nil {
				t.Fatal("accepted invalid plan")
			}
		})
	}
	other := c
	other.BeadID = "T-2"
	other.SectionID = "sec-2"
	other.Revision = "rev-2"
	m.SpecContracts[other.BeadID] = other
	m.Beads = append(m.Beads, api.RoundBead{ID: other.BeadID, SpecChangeID: other.ChangeID, SpecRevision: other.Revision, SpecSectionID: other.SectionID})
	if err := validateSpecSelection(m.Beads, m.SpecContracts, "", ""); err == nil {
		t.Fatal("accepted mixed revisions")
	}
}

func TestSpecArtifactConfinement(t *testing.T) {
	_, original := specFixture()
	for _, path := range []string{"../escape.md", "/tmp/escape.md", "openspec/changes/other/proposal.md", "openspec/changes/test-change/../proposal.md", "openspec/changes/test-change/review.md", "openspec/changes/test-change/specs/../../config.yaml", "openspec/changes/test-change/specs/.hidden/spec.md", "openspec/changes/test-change/specs/drafts/spec.md\\x"} {
		t.Run(path, func(t *testing.T) {
			c := original
			c.Artifacts = append(append([]api.SpecArtifact{}, original.Artifacts...), api.SpecArtifact{Path: path, Content: "escape"})
			if _, err := specArtifacts(map[string]api.SpecContract{c.BeadID: c}); err == nil {
				t.Fatal("accepted unsafe artifact")
			}
		})
	}
	c := original
	c.Artifacts = append(append([]api.SpecArtifact{}, original.Artifacts...), original.Artifacts[0])
	if _, err := specArtifacts(map[string]api.SpecContract{c.BeadID: c}); err == nil {
		t.Fatal("accepted duplicate")
	}
	c = original
	c.ChangeName = "archive"
	if _, err := specArtifacts(map[string]api.SpecContract{c.BeadID: c}); err == nil {
		t.Fatal("accepted archive target")
	}
	c = original
	c.Artifacts = c.Artifacts[:3]
	if _, err := specArtifacts(map[string]api.SpecContract{c.BeadID: c}); err == nil {
		t.Fatal("accepted proposal without a behavior spec")
	}
}

func TestSpecSnapshotExactBytesAndNoSymlink(t *testing.T) {
	m, c := specFixture()
	root := t.TempDir()
	artifacts, err := specArtifacts(m.SpecContracts)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeSpecArtifacts(root, artifacts); err != nil {
		t.Fatal(err)
	}
	if err = verifySpecArtifacts(root, m.SpecContracts); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, c.Artifacts[0].Path)
	if err = os.WriteFile(p, []byte(strings.TrimSpace(c.Artifacts[0].Content)), 0600); err != nil {
		t.Fatal(err)
	}
	if err = verifySpecArtifacts(root, m.SpecContracts); err == nil {
		t.Fatal("ignored exact-byte change")
	}
	if err = os.Remove(p); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err = os.WriteFile(outside, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outside, p); err != nil {
		t.Fatal(err)
	}
	if err = writeSpecArtifacts(root, artifacts); err == nil {
		t.Fatal("followed file symlink")
	}
	if got, _ := os.ReadFile(outside); string(got) != "untouched" {
		t.Fatal("modified symlink destination")
	}
	root2 := t.TempDir()
	if err = os.Symlink(t.TempDir(), filepath.Join(root2, "openspec")); err != nil {
		t.Fatal(err)
	}
	if err = writeSpecArtifacts(root2, artifacts); err == nil {
		t.Fatal("followed ancestor symlink")
	}
}

func TestSpecSnapshotRejectsDisagreement(t *testing.T) {
	m, c := specFixture()
	c.BeadID = "T-2"
	c.SectionID = "sec-2"
	c.Artifacts = append([]api.SpecArtifact{}, c.Artifacts...)
	c.Artifacts[0].Content = "different"
	m.SpecContracts[c.BeadID] = c
	if _, err := specArtifacts(m.SpecContracts); err == nil {
		t.Fatal("accepted inconsistent snapshots")
	}
}

func TestSpecGuardRequiresCurrentApproval(t *testing.T) {
	meta, c := specFixture()
	m := &Manager{}
	if err := m.guardSpec(context.Background(), meta, ""); err == nil {
		t.Fatal("accepted absent resolver")
	}
	m.opts.SpecContract = func(context.Context, string, string) (*api.SpecContract, error) { return &c, nil }
	if err := m.guardSpec(context.Background(), meta, ""); err != nil {
		t.Fatal(err)
	}
	changed := c
	changed.Revision = "new"
	m.opts.SpecContract = func(context.Context, string, string) (*api.SpecContract, error) { return &changed, nil }
	if err := m.guardSpec(context.Background(), meta, ""); err == nil {
		t.Fatal("accepted changed approval")
	}
	m.opts.SpecContract = func(context.Context, string, string) (*api.SpecContract, error) {
		return nil, errors.New("workflow missing")
	}
	if err := m.guardSpec(context.Background(), meta, ""); err == nil {
		t.Fatal("accepted missing workflow")
	}
	m.opts.SpecContract = func(context.Context, string, string) (*api.SpecContract, error) { return nil, nil }
	if err := m.guardSpec(context.Background(), meta, ""); err == nil {
		t.Fatal("accepted removed mapping")
	}
}

func TestSpecPromptIncludesPinnedContract(t *testing.T) {
	_, c := specFixture()
	p := specContractPrompt(c)
	for _, want := range []string{c.Revision, c.SectionID, c.SectionBody, c.Rationale, c.Artifacts[0].Content, "Do not edit these artifacts"} {
		if !strings.Contains(p, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if !specLabelled([]string{"specflow:wf-1"}) || !specLabelled([]string{"spec-section:sec-1"}) || specLabelled([]string{"ordinary"}) {
		t.Fatal("workflow label detection")
	}
}
