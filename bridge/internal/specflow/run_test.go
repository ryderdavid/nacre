package specflow

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestApproveAndRunRecordsRoundOnHistory(t *testing.T) {
	repo, _, _, opts := fixture(t)
	var m *Manager
	calls := 0
	opts.RunEpic = func(ctx context.Context, path, epic string, finish bool) (string, error) {
		calls++
		if !finish {
			return "", fmt.Errorf("approve and run must finish the epic")
		}
		canonical, _ := filepath.EvalSymlinks(repo)
		if path != canonical || epic != "NAC-1" {
			return "", fmt.Errorf("wrong epic: %s %s", path, epic)
		}
		// This is the same contract lookup the round callback performs.
		m.mu.Lock()
		c := clone(m.findLocked(path, epic).Change)
		m.mu.Unlock()
		if c.Phase != "ready" {
			return "", fmt.Errorf("not ready")
		}
		for _, s := range c.Sections {
			if _, err := m.Contract(ctx, path, s.BeadID); err != nil {
				return "", err
			}
		}
		return "rd-implementation", nil
	}
	m = newManager(t, opts)
	c := develop(t, m, repo)
	var action Action
	if err := json.Unmarshal([]byte(`{"action":"approve","revision":"`+c.Revision+`","run":true}`), &action); err != nil {
		t.Fatal(err)
	}
	c = act(t, m, repo, c.SeedID, action)
	if calls != 1 || c.Phase != "ready" || c.Error != "" {
		t.Fatalf("calls=%d change=%+v", calls, c)
	}
	last := c.History[len(c.History)-1]
	if last.Kind != "round" || last.RoundID != "rd-implementation" {
		t.Fatalf("history: %+v", c.History)
	}
	raw, _ := json.Marshal(last)
	if !strings.Contains(string(raw), `"round_id":"rd-implementation"`) {
		t.Fatal(string(raw))
	}
	reloaded := newManager(t, opts)
	env, err := reloaded.Get(context.Background(), repo, c.SeedID)
	if err != nil || env.Change.History[len(c.History)-1].RoundID != last.RoundID {
		t.Fatalf("history lost: %+v %v", env, err)
	}
	approve(t, m, repo, c)
	if calls != 1 {
		t.Fatal("approval retry started another round")
	}
}

// Approve, run, I'll land: the epic's round runs but doesn't finish the
// epic into the landing target.
func TestApproveRunHoldDoesNotFinishTheEpic(t *testing.T) {
	repo, _, _, opts := fixture(t)
	var finishes []bool
	opts.RunEpic = func(_ context.Context, _, _ string, finish bool) (string, error) {
		finishes = append(finishes, finish)
		return "rd-held", nil
	}
	m := newManager(t, opts)
	c := develop(t, m, repo)
	c = act(t, m, repo, c.SeedID, Action{Action: "approve", Revision: c.Revision, Run: true, Hold: true})
	if c.Phase != "ready" || len(finishes) != 1 || finishes[0] {
		t.Fatalf("change %+v finishes=%v", c, finishes)
	}
	if last := c.History[len(c.History)-1]; last.RoundID != "rd-held" {
		t.Fatalf("history %+v", c.History)
	}
}

func TestApproveRunFailurePreservesReadyApproval(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		t.Run(fmt.Sprint(unavailable), func(t *testing.T) {
			repo, _, _, opts := fixture(t)
			if !unavailable {
				opts.RunEpic = func(context.Context, string, string, bool) (string, error) {
					return "", fmt.Errorf("repository needs nacre.yaml")
				}
			}
			m := newManager(t, opts)
			c := develop(t, m, repo)
			c = act(t, m, repo, c.SeedID, Action{Action: "approve", Revision: c.Revision, Run: true})
			if c.Phase != "ready" || c.ApprovedRevision != c.Revision || !strings.Contains(c.Error, "starting the epic round failed") {
				t.Fatalf("approval lost: %+v", c)
			}
			last := c.History[len(c.History)-1]
			if last.Kind != "run_error" || last.Text != c.Error || last.RoundID != "" {
				t.Fatalf("failure not recorded: %+v", last)
			}
		})
	}
}

func TestApprovalRunsOnlyAfterSuccessfulPromotion(t *testing.T) {
	for _, run := range []bool{false, true} {
		t.Run(fmt.Sprint(run), func(t *testing.T) {
			repo, b, _, opts := fixture(t)
			calls := 0
			opts.RunEpic = func(context.Context, string, string, bool) (string, error) { calls++; return "rd-resumed", nil }
			m := newManager(t, opts)
			c := develop(t, m, repo)
			// Stop after an idempotently recorded promotion comment, then
			// resume. The paid review and approved revision are retained.
			b.loseComment = true
			c = act(t, m, repo, c.SeedID, Action{Action: "approve", Revision: c.Revision, Run: run})
			if c.Phase != "interrupted" || calls != 0 {
				t.Fatalf("ran before promotion: %+v calls=%d", c, calls)
			}
			c = act(t, m, repo, c.SeedID, Action{Action: "resume"})
			want := 0
			if run {
				want = 1
			}
			if c.Phase != "ready" || calls != want {
				t.Fatalf("resumed: %+v calls=%d", c, calls)
			}
		})
	}
}
