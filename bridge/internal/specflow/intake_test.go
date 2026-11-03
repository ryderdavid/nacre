package specflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

func intakeRepo(t *testing.T) (string, *fakeModel, Options) {
	repo, _, model, opts := fixture(t)
	write(t, repo, "openspec/changes/resilient-rounds/proposal.md", "## Why\nRounds should recover before they stop.\n\n## What Changes\n- x\n")
	write(t, repo, "openspec/changes/resilient-rounds/specs/round-recovery/spec.md", "## ADDED Requirements\n\n### Requirement: Retrying a parked bead\nIt SHALL.\n")
	write(t, repo, "openspec/specs/boards/spec.md", "### Requirement: Enter inserts a newline\nIt SHALL.\n")
	return repo, model, opts
}

func answer(v map[string]any) func(context.Context, claude.Headless, int) (any, float64, error) {
	return func(context.Context, claude.Headless, int) (any, float64, error) { return v, 0.04, nil }
}

const retryPath = "openspec/changes/resilient-rounds/specs/round-recovery/spec.md"

func TestIntakeVerdicts(t *testing.T) {
	for _, tc := range []struct {
		name           string
		model          map[string]any
		verdict, name2 string
		fallback       bool
	}{
		{"bead", map[string]any{"verdict": "bead", "change_name": "", "rationale": "A typo.", "references": []any{}}, VerdictBead, "", false},
		{"amend cited", map[string]any{"verdict": "amend", "change_name": "resilient-rounds", "rationale": "Changes retry.", "references": []any{map[string]any{"path": retryPath, "requirement": "Retrying a parked bead"}}}, VerdictAmend, "resilient-rounds", false},
		{"amend uncited", map[string]any{"verdict": "amend", "change_name": "resilient-rounds", "rationale": "Changes retry.", "references": []any{}}, VerdictNew, "", true},
		{"amend unknown change", map[string]any{"verdict": "amend", "change_name": "nope", "rationale": "x", "references": []any{map[string]any{"path": retryPath, "requirement": "Retrying a parked bead"}}}, VerdictNew, "", true},
		{"covered cited", map[string]any{"verdict": "covered", "change_name": "", "rationale": "Already asked.", "references": []any{map[string]any{"path": "openspec/specs/boards/spec.md", "requirement": "Enter inserts a newline"}}}, VerdictCovered, "", false},
		{"covered uncited", map[string]any{"verdict": "covered", "change_name": "", "rationale": "Already asked.", "references": []any{map[string]any{"path": "openspec/specs/boards/spec.md", "requirement": "Made up"}}}, VerdictNew, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, model, opts := intakeRepo(t)
			model.fn = answer(tc.model)
			m := newManager(t, opts)
			v, err := m.Intake(context.Background(), repo, IntakeDraft{Title: "Retry a parked bead with another model", Type: "feature"})
			if err != nil {
				t.Fatal(err)
			}
			if v.Verdict != tc.verdict || v.ChangeName != tc.name2 || (v.Fallback != "") != tc.fallback || v.CostUSD != 0.04 || len(v.ActiveChanges) != 1 || v.ActiveChanges[0] != "resilient-rounds" {
				t.Fatalf("verdict=%+v", v)
			}
			h := model.calls[0]
			if h.MaxBudgetUSD != 0 || len(h.AllowedTools) != 0 || h.ReadRepo || h.ReadSource || h.Persist {
				t.Fatalf("intake call is not bounded and read-only: %+v", h)
			}
			if !strings.Contains(h.Prompt, "resilient-rounds: Rounds should recover before they stop.") || !strings.Contains(h.Prompt, "Requirement: Retrying a parked bead") {
				t.Fatalf("prompt lacks the index or active changes:\n%s", h.Prompt)
			}
			if len(m.records) != 0 {
				t.Fatal("intake created a workflow record")
			}
			if _, err := os.Stat(filepath.Join(repo, "openspec/changes/retry")); err == nil {
				t.Fatal("intake wrote files")
			}
		})
	}
}

func TestIntakeNamesActiveWorkOnATrackedChange(t *testing.T) {
	repo, _, model, opts := fixture(t)
	m := newManager(t, opts)
	c := approve(t, m, repo, develop(t, m, repo))
	m.opts.Active = func(string, string) string { return "Round R1 is still active." }
	model.fn = answer(map[string]any{"verdict": "amend", "change_name": c.ChangeName, "rationale": "Same area.", "references": []any{map[string]any{"path": "openspec/changes/" + c.ChangeName + "/proposal.md", "requirement": ""}}})
	v, err := m.Intake(context.Background(), repo, IntakeDraft{Title: "More drafts"})
	if err != nil || v.Verdict != VerdictAmend || !v.TargetTracked || v.TargetActive != "Round R1 is still active." {
		t.Fatalf("verdict=%+v err=%v", v, err)
	}
}

func TestIntakeFailuresAndUnavailability(t *testing.T) {
	repo, model, opts := intakeRepo(t)
	model.fn = func(context.Context, claude.Headless, int) (any, float64, error) {
		return nil, 0.5, errors.New("budget exceeded")
	}
	m := newManager(t, opts)
	if _, err := m.Intake(context.Background(), repo, IntakeDraft{Title: "x"}); err == nil || !strings.Contains(err.Error(), "budget exceeded") {
		t.Fatalf("err=%v", err)
	}
	model.fn = answer(map[string]any{"rationale": "no verdict"})
	if _, err := m.Intake(context.Background(), repo, IntakeDraft{Title: "x"}); err == nil || !strings.Contains(err.Error(), "couldn't be read") {
		t.Fatalf("malformed accepted: %v", err)
	}
	if _, err := m.Intake(context.Background(), t.TempDir(), IntakeDraft{Title: "x"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no OpenSpec: %v", err)
	}
	if _, err := m.Intake(context.Background(), repo, IntakeDraft{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("untitled draft: %v", err)
	}
}

func TestDevelopRecordsAnIntakeTheAuthorOverruled(t *testing.T) {
	repo, _, _, opts := fixture(t)
	m := newManager(t, opts)
	c := act(t, m, repo, "NAC-1", Action{Action: "develop", Feedback: "Intake: amend other", Intake: &IntakeHint{Verdict: VerdictAmend, ChangeName: "other"}})
	if c.Intake == nil || c.Intake.ChangeName != "other" {
		t.Fatalf("intake not stored: %+v", c.Intake)
	}
	found := false
	for _, h := range c.History {
		found = found || (h.Kind == "intake" && strings.Contains(h.Text, "Intake suggested amend other; reconciliation chose a new change because"))
	}
	if !found {
		t.Fatalf("disagreement not recorded: %+v", c.History)
	}
}
