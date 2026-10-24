package rounds

import (
	"context"
	"fmt"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/gates"
	"github.com/ryderdavid/nacre/bridge/internal/gitx"
	"github.com/ryderdavid/nacre/bridge/internal/repoconfig"
)

// beadGates are the gates checked for one bead: all but final_only.
func beadGates(cfg *repoconfig.Config) []repoconfig.Step {
	var steps []repoconfig.Step
	for _, g := range cfg.Gates {
		if !g.FinalOnly {
			steps = append(steps, g)
		}
	}
	return steps
}

// finalSteps are every gate and then the build.
func finalSteps(cfg *repoconfig.Config) []repoconfig.Step {
	return append(append([]repoconfig.Step{}, cfg.Gates...), cfg.Build...)
}

// ignoredPaths are left out of "uncommitted": bd writes here itself.
var ignoredPaths = []string{".beads/"}

// check is nacred's own test of work in a worktree: committed, on its
// branch, and passing the gates.
func (m *Manager) check(ctx context.Context, ws, base string, cfg *repoconfig.Config, steps []repoconfig.Step) *api.Verification {
	v := &api.Verification{At: stamp()}
	dirty, err := gitx.Dirty(ctx, ws, ignoredPaths...)
	if err != nil {
		v.Reason = "git couldn't read the worktree: " + err.Error()
		return v
	}
	v.Uncommitted = dirty
	if base != "" {
		v.Commits, _ = gitx.CommitsBetween(ctx, ws, base, "HEAD")
	}
	switch {
	case len(dirty) > 0:
		v.Reason = "uncommitted changes: " + list(dirty, 8)
		return v
	case base != "" && v.Commits == 0:
		v.Reason = "no commits on the branch"
		return v
	}
	results := m.opts.Gates.RunAll(ctx, ws, cfg, steps, true)
	v.Gates = toAPI(results)
	if !gates.AllOK(results) {
		for _, r := range results {
			if !r.OK {
				v.Reason = r.Summary()
				break
			}
		}
		if v.Reason == "" {
			v.Reason = "no gates ran"
		}
		return v
	}
	v.OK = true
	return v
}

func toAPI(rs []gates.Result) []api.GateResult {
	out := make([]api.GateResult, len(rs))
	for i, r := range rs {
		out[i] = api.GateResult{Name: r.Name, OK: r.OK, ExitCode: r.ExitCode, TimedOut: r.TimedOut, DurationMS: r.DurationMS, Tail: r.Tail, Error: r.Error}
	}
	return out
}

// startVerify checks a bead whose turn ended and decides what happens next.
func (r *round) startVerify(b *api.RoundBead) {
	id, ws, base, runID, cfg := b.ID, b.Workspace, b.Base, b.RunID, r.cfg
	r.goOp("verify:"+id, func(ctx context.Context) func() {
		v := r.m.check(ctx, ws, base, cfg, beadGates(cfg))
		if runID != "" {
			_ = r.m.opts.Runs.SetVerification(runID, v)
		}
		return func() {
			b := r.bead(id)
			if b == nil || b.State != api.BeadVerifying {
				return
			}
			b.Verification = v
			ev := api.RoundEvent{Kind: "verify", Bead: id, Gates: v.Gates}
			switch {
			case v.OK:
				b.State = api.BeadIntegrating
				r.m.leases.release(r.meta.ID + "/" + id)
				ev.Text = fmt.Sprintf("Verified: %d commit(s), every gate passed", v.Commits)
				r.note(id, fmt.Sprintf("Round %s verified this bead on %s: %d commit(s), gates passed. Waiting to be merged.", r.meta.ID, b.Branch, v.Commits))
			case b.FixAttempts < r.cfg.Limits.FixAttempts:
				b.FixAttempts++
				ev.Text = "Not done yet: " + v.Reason
				r.followUp(b, verifyPrompt(v))
			default:
				ev.Text = "Not done: " + v.Reason
				r.eventWith(ev)
				r.park(b, fmt.Sprintf("It still failed verification after %d follow-up(s): %s", b.FixAttempts, v.Reason))
				return
			}
			r.eventWith(ev)
		}
	})
}

func list(items []string, max int) string {
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + fmt.Sprintf(" and %d more", len(items)-max)
}
