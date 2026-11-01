package rounds

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/gitx"
	"github.com/ryderdavid/nacre/bridge/internal/repoconfig"
)

// EpicBranch is the branch an epic's work lands on until the epic is
// finished and merged into the landing target.
func EpicBranch(epic string) string { return "epic/" + strings.ToLower(epic) }

// parent is the bead's parent, from its parent field or its parent-child
// dependency.
func (is issue) parent() string {
	if is.Parent != "" {
		return is.Parent
	}
	for _, d := range is.Dependencies {
		if d.Type == "parent-child" {
			return d.DependsOnID
		}
	}
	return ""
}

// sharedEpic is the epic every bead belongs to, or "" when none does. Beads
// from different epics, or epic work mixed with work outside any epic, can't
// share a round: each epic's work lands on its own branch.
func (m *Manager) sharedEpic(ctx context.Context, repo string, issues []issue) (string, error) {
	epics := map[string][]string{}
	isEpic := map[string]bool{}
	for _, is := range issues {
		p := is.parent()
		if p != "" {
			if _, known := isEpic[p]; !known {
				raw, err := m.opts.Beads.Get(ctx, repo, p)
				isEpic[p] = err == nil && parseIssue(raw).Type == "epic"
			}
			if !isEpic[p] {
				p = ""
			}
		}
		epics[p] = append(epics[p], is.ID)
	}
	if len(epics) <= 1 {
		for e := range epics {
			return e, nil
		}
		return "", nil
	}
	var groups []string
	for e, ids := range epics {
		name := e
		if name == "" {
			name = "no epic"
		}
		groups = append(groups, name+": "+strings.Join(ids, ", "))
	}
	sort.Strings(groups)
	return "", conflict("These beads belong to different epics, and each epic's work lands on its own branch. Start a round per epic (" + strings.Join(groups, "; ") + ")")
}

// ensureEpicBranch makes the epic's branch from the landing target the first
// time the epic's work lands.
func ensureEpicBranch(ctx context.Context, repo, epic, from string) (string, error) {
	branch := EpicBranch(epic)
	if gitx.BranchExists(ctx, repo, branch) {
		return branch, nil
	}
	if _, err := gitx.Run(ctx, repo, "branch", "--", branch, "refs/heads/"+from); err != nil {
		return "", fmt.Errorf("making %s's branch %s from %s: %w", epic, branch, from, err)
	}
	return branch, nil
}

// landsOnTarget reports whether a round lands on the repository's landing
// target, where versions are bumped and tagged, rather than an epic branch.
func landsOnTarget(meta api.Round, cfg *repoconfig.Config) bool {
	return meta.Target == cfg.Land.Target
}

// FinishEpic merges a finished epic's branch into the landing target: a round
// without beads that starts from the epic's branch, runs every gate and the
// build with one version bump, lands and pushes, then closes the epic and
// removes its branch. An epic whose work is already all on the target is
// closed straight away (closed is true and no round is made).
func (m *Manager) FinishEpic(repoID, epic string) (round api.Round, closed bool, err error) {
	ctx := m.ctx
	name, path, err := m.opts.Repo(repoID)
	if err != nil {
		return api.Round{}, false, err
	}
	cfg, err := repoconfig.Load(path)
	if err != nil {
		return api.Round{}, false, conflict(name + " needs a valid " + repoconfig.FileName + " to finish an epic: " + err.Error())
	}
	raw, err := m.opts.Beads.Get(ctx, path, epic)
	if err != nil {
		return api.Round{}, false, conflict(fmt.Sprintf("%s: %v", epic, err))
	}
	is := parseIssue(raw)
	if is.Status == "closed" {
		return api.Round{}, false, conflict(epic + " is already closed")
	}
	open, err := m.openChildren(ctx, path, epic)
	if err != nil {
		return api.Round{}, false, err
	}
	if len(open) > 0 {
		return api.Round{}, false, conflict("Finish " + epic + "'s open beads first: " + strings.Join(open, ", "))
	}
	branch, target := EpicBranch(epic), cfg.Land.Target
	change := ""
	if m.opts.ChangeOf != nil {
		change = m.opts.ChangeOf(path, epic)
	}
	merged := !gitx.BranchExists(ctx, path, branch) || gitx.IsAncestor(ctx, path, "refs/heads/"+branch, "refs/heads/"+target)
	if merged && change != "" {
		// A finish that landed the archive but stopped before closing the
		// epic: the change is no longer active on the target, so just end it.
		if _, err := gitx.Run(ctx, path, "cat-file", "-e", "refs/heads/"+target+":openspec/changes/"+change); err != nil {
			if m.opts.ChangeArchived != nil {
				m.opts.ChangeArchived(path, epic)
			}
			change = ""
		}
	}
	if merged && change == "" {
		if err := m.opts.Beads.Close(ctx, path, epic, "Every bead is done and its work is already on "+target+"."); err != nil {
			return api.Round{}, false, err
		}
		if gitx.BranchExists(ctx, path, branch) {
			_ = gitx.DeleteBranch(ctx, path, branch)
		}
		m.repoChanged(repoID)
		return api.Round{}, true, nil
	}
	m.mu.Lock()
	for _, other := range m.rounds {
		if other.meta.Active() && other.meta.RepoID == repoID && (other.meta.Target == branch || other.meta.FinishEpic == epic) {
			m.mu.Unlock()
			return api.Round{}, false, conflict(fmt.Sprintf("Round %s is still working on %s; let it land or cancel it first", other.meta.ID, branch))
		}
	}
	m.mu.Unlock()
	id := newID()
	// With nothing left on the branch, the round only archives the change.
	from := "refs/heads/" + branch
	if merged {
		from = "refs/heads/" + target
	}
	ws, err := m.opts.Workspaces.CreateIntegration(ctx, path, repoID, id, from)
	if err != nil {
		return api.Round{}, false, fmt.Errorf("integration workspace: %w", err)
	}
	now := stamp()
	meta := api.Round{
		ID: id, RepoID: repoID, RepoName: name, RepoPath: path, Epic: epic, FinishEpic: epic,
		State: api.RoundIntegrating, Target: target, Base: ws.Base, Branch: ws.Branch, Workspace: ws.Path,
		Beads: []api.RoundBead{}, Plan: &api.Plan{Summary: "Merge " + branch + " into " + target, Beads: []api.PlanBead{}, Findings: []api.Finding{}},
		BudgetUSD: m.opts.BudgetUSD, TimeLimitMinutes: m.opts.TimeLimitMinutes, Mode: m.opts.Mode, Model: m.opts.Model,
		AutoLand: true, CreatedAt: now, UpdatedAt: now,
	}
	dir := filepath.Join(m.opts.Dir, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return api.Round{}, false, err
	}
	r := m.newRound(meta, dir)
	r.cfg = cfg
	r.mu.Lock()
	what := fmt.Sprintf("merging %s into %s", branch, target)
	if merged {
		what = "its work is already on " + target
	}
	if change != "" {
		what = "archiving " + change + "; " + what
	}
	r.event("state", "", fmt.Sprintf("Finishing epic %s: %s, with every gate and the build", epic, what))
	r.saveLocked()
	out := clone(r.meta)
	r.mu.Unlock()
	m.mu.Lock()
	m.rounds[id] = r
	m.spawn(r)
	m.mu.Unlock()
	return out, false, nil
}

func (m *Manager) openChildren(ctx context.Context, repo, epic string) ([]string, error) {
	all, err := m.opts.Beads.List(ctx, repo)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, raw := range all {
		is := parseIssue(raw)
		if is.parent() == epic && is.Status != "closed" {
			ids = append(ids, is.ID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// finishedEpic closes a merged epic and removes its branch here and on the
// remote. Both are best effort: the merge has landed either way.
func (r *round) finishedEpic(ctx context.Context, meta api.Round, head string, pushed bool) []string {
	var notes []string
	epic, branch := meta.FinishEpic, EpicBranch(meta.FinishEpic)
	if err := r.m.opts.Beads.Close(ctx, meta.RepoPath, epic, fmt.Sprintf("Merged into %s at %s by round %s; gates and build pass", meta.Target, short(head), meta.ID)); err != nil {
		notes = append(notes, "Landed, but closing "+epic+" failed: "+err.Error())
	}
	r.m.repoChanged(meta.RepoID)
	if r.m.opts.ChangeArchived != nil {
		r.m.opts.ChangeArchived(meta.RepoPath, epic)
	}
	if !gitx.BranchExists(ctx, meta.RepoPath, branch) {
		return notes
	}
	if err := gitx.DeleteBranch(ctx, meta.RepoPath, branch); err != nil {
		notes = append(notes, "Removing "+branch+" failed: "+firstLine(err.Error()))
	}
	if pushed {
		if _, err := gitx.Run(ctx, meta.RepoPath, "push", "origin", "--delete", branch); err != nil {
			notes = append(notes, "Removing "+branch+" from origin failed: "+firstLine(err.Error()))
		}
	}
	return notes
}
