package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/attention"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
	"github.com/ryderdavid/nacre/bridge/internal/rounds"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
	"github.com/ryderdavid/nacre/bridge/internal/specflow"
	"github.com/ryderdavid/nacre/bridge/internal/sse"
)

func executionContract(c specflow.Contract) *api.SpecContract {
	out := &api.SpecContract{ChangeID: c.ChangeID, Revision: c.Revision, SectionID: c.SectionID,
		BeadID: c.BeadID, ChangeName: c.ChangeName, Summary: c.Summary, Rationale: c.Rationale,
		References: append([]string{}, c.References...)}
	for _, a := range c.Artifacts {
		out.Artifacts = append(out.Artifacts, api.SpecArtifact{Path: a.Path, Content: a.Content})
	}
	for _, s := range c.Sections {
		if s.ID == c.SectionID {
			out.SectionTitle, out.SectionBody = s.Title, s.Body
		}
	}
	return out
}

func activeChange(path, changeID string, rm *rounds.Manager, runMgr *runs.Manager) string {
	if rm != nil {
		for _, r := range rm.List("", true) {
			if !sameRepoPath(r.RepoPath, path) {
				continue
			}
			for _, b := range r.Beads {
				if b.SpecChangeID == changeID {
					return "Round " + r.ID + " is still active. Merge or cancel it before revising the change."
				}
			}
		}
	}
	for _, r := range runMgr.List(runs.Filter{ActiveOnly: true}) {
		if sameRepoPath(r.RepoPath, path) && r.SpecChangeID == changeID {
			return "Run " + r.ID + " is still active."
		}
	}
	return ""
}

func changeEvidence(path string, c specflow.Change, rm *rounds.Manager, runMgr *runs.Manager, beads *bd.Client) []specflow.Evidence {
	var history []api.Round
	if rm != nil {
		history = rm.List("", false)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	current, readErr := beads.List(ctx, path)
	states := make(map[string]beadEvidenceState)
	for _, raw := range current {
		var state beadEvidenceState
		if json.Unmarshal(raw, &state) == nil {
			states[state.ID] = state
		}
	}
	return assessChangeEvidence(path, c, history, states, readErr, runMgr.Run)
}

func assessChangeEvidence(path string, c specflow.Change, history []api.Round, states map[string]beadEvidenceState, readErr error, getRun func(string) (api.Run, error)) []specflow.Evidence {
	result := make([]specflow.Evidence, 0, len(c.Sections))
	for _, s := range c.Sections {
		e := specflow.Evidence{SectionID: s.ID, BeadID: s.BeadID, Note: "No verified implementation for this revision."}
		found := false
		for _, r := range history {
			if !sameRepoPath(r.RepoPath, path) {
				continue
			}
			for _, b := range r.Beads {
				if b.ID != s.BeadID || b.SpecChangeID != c.ID || b.SpecSectionID != s.ID ||
					(b.SpecRevision != c.Revision && !r.Active()) {
					continue
				}
				e.RoundID, e.RunID, e.HumanCheck = r.ID, b.RunID, b.HumanCheck != ""
				e.RoundState = r.State
				e.Note = fmt.Sprintf("Round %s: %s. %s", r.ID, r.State, b.Reason)
				if run, err := getRun(b.RunID); err == nil {
					e.Verified = (b.State == api.BeadIntegrating || b.State == api.BeadIntegrated) &&
						b.Verification != nil && b.Verification.OK &&
						matchingVerification(c, s, run, states[s.BeadID], readErr == nil)
				}
				e.BuildVerified = e.Verified && b.State == api.BeadIntegrated && r.FinalCommit != "" &&
					(r.State == api.RoundReadyToLand || r.State == api.RoundLanded)
				e.Merged = r.State == api.RoundLanded && r.LandedCommit != "" && b.State == api.BeadIntegrated && b.MergeCommit != ""
				e.Landed = e.BuildVerified && r.State == api.RoundLanded && r.LandedCommit != "" && b.MergeCommit != ""
				if e.HumanCheck {
					e.Note += " Human acceptance is still required: " + b.HumanCheck
				}
				if !e.Landed {
					e.Note += " Merging and final acceptance remain separate."
				}
				found = true
				break
			}
			if found {
				break
			}
		}
		if !found && readErr == nil && c.CompletedBeforeAdoption(s) && states[s.BeadID].Status == "closed" {
			// Finished before Nacre took the change over: there is no round to point to.
			e.Verified, e.BuildVerified, e.Landed = true, true, true
			e.Note = "Finished before Nacre adopted this change: every task was checked and its bead is closed. There is no round evidence for it."
		}
		if readErr != nil {
			e.Note = "Beads could not be read; implementation evidence is not confirmed."
		}
		if !evidencePhase(c, true) || c.ApprovedRevision == "" || c.ApprovedRevision != c.Revision {
			e.Verified, e.BuildVerified, e.Landed = false, false, false
			e.Note = "Review and approve the current revision before its implementation can be verified. " + e.Note
		}
		result = append(result, e)
	}
	return result
}

// evidencePhase reports whether a change's phase lets its sections carry
// implementation evidence: an approved change ready for work, or one archived
// after its epic finished (its approved revision is what landed). allowStale
// also admits a stale change whose own files are still as approved.
func evidencePhase(c specflow.Change, allowStale bool) bool {
	switch c.Phase {
	case "ready", "archived":
		return true
	case "stale":
		return allowStale && c.AsApproved
	}
	return false
}

func sameRepoPath(a, b string) bool {
	a, errA := filepath.Abs(a)
	b, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return false
	}
	a, errA = filepath.EvalSymlinks(a)
	b, errB = filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && a == b
}

type beadEvidenceState struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
}

func matchingVerification(c specflow.Change, section specflow.Section, run api.Run, bead beadEvidenceState, readable bool) bool {
	if !readable || !evidencePhase(c, false) || c.ApprovedRevision == "" || c.ApprovedRevision != c.Revision ||
		bead.ID != section.BeadID || run.IssueID != section.BeadID || run.Status != api.StatusSucceeded ||
		run.SpecChangeID != c.ID || run.SpecRevision != c.ApprovedRevision || run.SpecSectionID != section.ID ||
		run.Verification == nil || !run.Verification.OK {
		return false
	}
	verified, err := time.Parse(time.RFC3339Nano, run.Verification.At)
	ended, endErr := time.Parse(time.RFC3339Nano, run.EndedAt)
	if err != nil || endErr != nil || verified.Before(ended) {
		return false
	}
	// A reopened or edited open bead needs fresh evidence. Closed status alone
	// never supplies verification; the matching passing run above is mandatory.
	if bead.Status != "closed" {
		updated, err1 := time.Parse(time.RFC3339Nano, bead.UpdatedAt)
		if err1 != nil || updated.After(verified) {
			return false
		}
	}
	return true
}

// roundsStamp changes whenever a round in the repository at path moves, so
// cached change evidence is recomputed only then.
func roundsStamp(path string, rm *rounds.Manager) string {
	if rm == nil {
		return ""
	}
	var b strings.Builder
	for _, r := range rm.List("", false) {
		if sameRepoPath(r.RepoPath, path) {
			b.WriteString(r.ID + "@" + r.State + "@" + r.UpdatedAt + ";")
		}
	}
	return b.String()
}

// changeEvent is the /v1/events "change" payload for a phase transition.
func changeEvent(repoID string, c specflow.Change) map[string]string {
	return map[string]string{"repo_id": repoID, "change_id": c.ID, "change_name": c.ChangeName,
		"phase": c.Phase, "seed_id": c.SeedID, "epic_id": c.EpicID}
}

// invalidateAttention drops a repository's cached Needs you items whenever
// nacred reports it changed; a dropped event only delays the refresh.
func invalidateAttention(broker *sse.Broker, registry *repos.Registry, b *attention.Builder) {
	ch, _ := broker.Subscribe()
	for msg := range ch {
		if msg.Event != "repo" && msg.Event != "change" {
			continue
		}
		var ev struct {
			RepoID string `json:"repo_id"`
		}
		if json.Unmarshal(msg.Data, &ev) != nil {
			continue
		}
		if path, err := registry.Path(ev.RepoID); err == nil {
			b.Invalidate(path)
		}
	}
}
