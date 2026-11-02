// Package attention builds Needs you: every item on this computer that waits
// on Ryder, derived from current bead, round and change state on each request.
// See openspec/changes/surface-spec-workflow (needs-you).
package attention

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
)

// RefreshAfter bounds how often one repository's beads and changes are read.
const RefreshAfter = 30 * time.Second

// Builder computes the attention list. Rounds are read live on every call;
// each repository's bead and change items are cached for RefreshAfter or until
// Invalidate is called for it.
type Builder struct {
	Repos   func() []repos.Ref
	Issues  func(ctx context.Context, path string) ([]api.Issue, error)
	Rounds  func() []api.Round
	Changes func(path string) ([]api.ChangeSummary, error)
	// Comments reads a bead's comments, for agents' `Found:` reports; nil
	// leaves them out.
	Comments func(ctx context.Context, path, id string) ([]api.Comment, error)
	// Working reports whether a run or round is working on a bead; nil
	// leaves stuck beads out of the list.
	Working func(repoID, beadID string) bool
	// Now is replaced in tests.
	Now func() time.Time

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	at    time.Time
	items []api.AttentionItem
	beads map[string]bead
}

// Invalidate drops the cached items of the repository at path.
func (b *Builder) Invalidate(path string) {
	b.mu.Lock()
	delete(b.cache, path)
	b.mu.Unlock()
}

func (b *Builder) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

// Items returns everything waiting on Ryder, questions and decisions first,
// then approvals, then landing and archive, newest first within each.
func (b *Builder) Items(ctx context.Context) []api.AttentionItem {
	refs := b.Repos()
	byID := map[string]repos.Ref{}
	for _, ref := range refs {
		byID[ref.ID] = ref
	}
	var mu sync.Mutex
	items := []api.AttentionItem{}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, ref := range refs {
		wg.Add(1)
		go func(ref repos.Ref) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			found := b.repoItems(ctx, ref)
			mu.Lock()
			items = append(items, found...)
			mu.Unlock()
		}(ref)
	}
	wg.Wait()
	if b.Rounds != nil {
		for _, r := range b.Rounds() {
			if ref, ok := byID[r.RepoID]; ok {
				items = append(items, roundItems(ref, r)...)
				b.mu.Lock()
				beads := b.cache[ref.Path].beads
				b.mu.Unlock()
				items = append(items, unclosedItems(ref, r, beads)...)
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		gi, gj := group(items[i].Kind), group(items[j].Kind)
		if gi != gj {
			return gi < gj
		}
		return items[i].CreatedAt > items[j].CreatedAt
	})
	return items
}

// group orders kinds: questions and decisions, approvals, landing and archive.
func group(kind string) int {
	switch kind {
	case api.AttentionRoundQuestion, api.AttentionBeadDecision, api.AttentionBeadParked, api.AttentionReviewBlocked,
		api.AttentionWorkflowInterrupted, api.AttentionFoundWork:
		return 0
	case api.AttentionProposalReady, api.AttentionApprovalStale:
		return 1
	}
	return 2
}

func (b *Builder) repoItems(ctx context.Context, ref repos.Ref) []api.AttentionItem {
	b.mu.Lock()
	if b.cache == nil {
		b.cache = map[string]cached{}
	}
	c, ok := b.cache[ref.Path]
	b.mu.Unlock()
	if ok && b.now().Sub(c.at) < RefreshAfter {
		return c.items
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	raw, err := b.Issues(ctx, ref.Path)
	if err != nil {
		// A repository bd can't read contributes nothing rather than failing
		// the whole list; its board already shows the error.
		return nil
	}
	beads := parseBeads(raw)
	items := beadItems(ref, beads)
	items = append(items, b.stuckItems(ref, beads)...)
	if b.Changes != nil {
		if changes, err := b.Changes(ref.Path); err == nil {
			items = append(items, epicItems(ref, beads, changes)...)
			items = append(items, changeItems(ref, changes, beads)...)
			items = append(items, foundBeadItems(ref, changes, beads)...)
			items = append(items, b.foundCommentItems(ctx, ref, changes)...)
		}
	}
	b.mu.Lock()
	b.cache[ref.Path] = cached{b.now(), items, beads}
	b.mu.Unlock()
	return items
}

type bead struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	Type        string   `json:"issue_type"`
	Labels      []string `json:"labels"`
	Parent      string   `json:"parent"`
	UpdatedAt   string   `json:"updated_at"`
}

func parseBeads(raw []api.Issue) map[string]bead {
	out := map[string]bead{}
	for _, r := range raw {
		var b bead
		if json.Unmarshal(r, &b) == nil && b.ID != "" {
			out[b.ID] = b
		}
	}
	return out
}

func has(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func digest(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:12])
}

func open(label, kind, repo, id string) api.AttentionOption {
	return api.AttentionOption{Type: "open", Label: label, Target: &api.AttentionTarget{Kind: kind, Repo: repo, ID: id}}
}

func action(label, method, path string, body map[string]any) api.AttentionOption {
	return api.AttentionOption{Type: "action", Label: label, Method: method, Path: path, Body: body}
}

// beadItems lists open beads already marked as needing Ryder: the human label
// or the decision type, the board's own "Needs you".
func beadItems(ref repos.Ref, beads map[string]bead) []api.AttentionItem {
	var out []api.AttentionItem
	for _, b := range beads {
		if b.Status == "closed" || (!has(b.Labels, "human") && b.Type != "decision") {
			continue
		}
		why := b.ID + " is labelled human: only you can do or check it."
		if b.Type == "decision" {
			why = b.ID + " is a decision waiting on you."
		}
		out = append(out, api.AttentionItem{
			ID: digest(api.AttentionBeadDecision, ref.ID, b.ID, b.Status), Kind: api.AttentionBeadDecision,
			Repo: ref.ID, RepoName: ref.Name, BeadID: b.ID, Title: b.Title, Why: why, CreatedAt: b.UpdatedAt,
			Options: []api.AttentionOption{open("Open bead", "bead", ref.ID, b.ID)},
		})
	}
	return out
}

func changeItems(ref repos.Ref, changes []api.ChangeSummary, beads map[string]bead) []api.AttentionItem {
	var out []api.AttentionItem
	for _, c := range changes {
		seed := ""
		if len(c.SeedIDs) > 0 {
			seed = c.SeedIDs[0]
		}
		name := c.ChangeName
		if name == "" {
			name = beads[seed].Title
		}
		if name == "" {
			name = seed
		}
		changePath := "/v1/repos/" + ref.ID + "/issues/" + seed + "/change"
		openChange := api.AttentionOption{Type: "open", Label: "Open change",
			Target: &api.AttentionTarget{Kind: "change", Repo: ref.ID, ID: seed, Revision: c.Revision}}
		item := api.AttentionItem{Repo: ref.ID, RepoName: ref.Name, BeadID: seed, ChangeID: c.ID, CreatedAt: c.UpdatedAt}
		reason := strings.TrimSpace(c.Error)
		switch c.Phase {
		case "awaiting_approval":
			item.Kind = api.AttentionProposalReady
			item.Title = "Proposal ready: " + name
			item.Why = "The proposal passed independent review. Approve revision " + short(c.Revision) + " to create its beads, or open it to give feedback."
			item.Options = []api.AttentionOption{
				{Type: "open", Label: "Review and approve", Target: openChange.Target},
				action("Approve", "POST", changePath, map[string]any{"action": "approve", "revision": c.Revision}),
			}
		case "needs_attention":
			item.Kind = api.AttentionReviewBlocked
			item.Title = "Review found problems in " + name
			item.Why = or(reason, "The reviewer left blocking findings. Open the change and give feedback to start another revision.")
			item.Options = []api.AttentionOption{openChange}
		case "stale":
			if c.AsApproved && archivable(c, beads) {
				item.Kind = api.AttentionChangeArchivable
				item.BeadID = c.EpicID
				item.Title = name + " is ready to archive"
				item.Why = fmt.Sprintf("All %d sections merged, and the change's own files are still the approved ones (%s). Merging archives it, merges its epic's work into the main branch with every gate and the build, pushes it and closes the epic.", len(c.Sections), strings.ToLower(or(c.StaleReason, "other specifications changed")))
				item.Options = []api.AttentionOption{finishEpic(ref.ID, c.EpicID, "Archive spec and merge into main"), openChange}
				break
			}
			item.Kind = api.AttentionApprovalStale
			item.Title = name + " needs a new review"
			item.Why = or(c.StaleReason, or(reason, "The project's specifications changed since this was reviewed."))
			item.Options = []api.AttentionOption{action("Review again", "POST", changePath, map[string]any{"action": "revise"}), openChange}
		case "interrupted":
			item.Kind = api.AttentionWorkflowInterrupted
			item.Title = name + " was interrupted"
			item.Why = or(reason, "The computer stopped during this operation. Resume to continue from the saved checkpoint.")
			item.Options = []api.AttentionOption{action("Resume", "POST", changePath, map[string]any{"action": "resume"}), openChange}
		case "ready":
			if !archivable(c, beads) {
				continue
			}
			item.Kind = api.AttentionChangeArchivable
			item.BeadID = c.EpicID
			item.Title = name + " is ready to archive"
			item.Why = fmt.Sprintf("All %d sections merged with evidence for the approved revision. Merging archives the change, merges its epic's work into the main branch with every gate and the build, pushes it and closes the epic.", len(c.Sections))
			item.Options = []api.AttentionOption{finishEpic(ref.ID, c.EpicID, "Archive spec and merge into main"), openChange}
		default:
			continue
		}
		item.ID = digest(item.Kind, ref.ID, c.ID, c.Phase, c.Revision)
		out = append(out, item)
	}
	return out
}

func finishEpic(repo, epic, label string) api.AttentionOption {
	return action(label, "POST", "/v1/repos/"+repo+"/epics/"+epic+"/finish", nil)
}

// epicItems lists open epics whose beads are all closed, other than a
// change's epic, which is finished from its archive item.
func epicItems(ref repos.Ref, beads map[string]bead, changes []api.ChangeSummary) []api.AttentionItem {
	ofChange := map[string]bool{}
	for _, c := range changes {
		if c.EpicID != "" {
			ofChange[c.EpicID] = true
		}
	}
	children := map[string][]bead{}
	for _, b := range beads {
		if b.Parent != "" {
			children[b.Parent] = append(children[b.Parent], b)
		}
	}
	var out []api.AttentionItem
	for _, e := range beads {
		kids := children[e.ID]
		if e.Type != "epic" || e.Status == "closed" || len(kids) == 0 || ofChange[e.ID] {
			continue
		}
		last := ""
		done := true
		for _, k := range kids {
			done = done && k.Status == "closed"
			if k.UpdatedAt > last {
				last = k.UpdatedAt
			}
		}
		if !done {
			continue
		}
		out = append(out, api.AttentionItem{
			ID: digest(api.AttentionEpicFinishable, ref.ID, e.ID, last), Kind: api.AttentionEpicFinishable,
			Repo: ref.ID, RepoName: ref.Name, BeadID: e.ID, Title: e.Title + " is done",
			Why:       fmt.Sprintf("All %d beads under %s are closed. Merging the epic merges its branch into the main branch with every gate and the build, pushes it and closes the epic.", len(kids), e.ID),
			CreatedAt: last,
			Options:   []api.AttentionOption{finishEpic(ref.ID, e.ID, "Merge epic into main"), open("Open epic", "bead", ref.ID, e.ID)},
		})
	}
	return out
}

// StuckAfter is how long a bead can sit in progress with nothing working on
// it before Needs you asks what to do with it.
const StuckAfter = 24 * time.Hour

// stuckItems lists beads marked in progress that no run or round is working
// on and that haven't changed for StuckAfter.
func (b *Builder) stuckItems(ref repos.Ref, beads map[string]bead) []api.AttentionItem {
	if b.Working == nil {
		return nil
	}
	var out []api.AttentionItem
	for _, x := range beads {
		if x.Status != "in_progress" || x.Type == "epic" || b.Working(ref.ID, x.ID) {
			continue
		}
		at, err := time.Parse(time.RFC3339, x.UpdatedAt)
		if err != nil || b.now().Sub(at) < StuckAfter {
			continue
		}
		out = append(out, api.AttentionItem{
			ID: digest(api.AttentionBeadStuck, ref.ID, x.ID, x.UpdatedAt), Kind: api.AttentionBeadStuck,
			Repo: ref.ID, RepoName: ref.Name, BeadID: x.ID, Title: x.Title + " is stuck in progress",
			Why:       x.ID + " has been in progress since " + at.Format("Jan 2") + " with nothing working on it. Work it until it's done (it is merged and closed when it passes every gate), or put it back in Ready.",
			CreatedAt: x.UpdatedAt,
			Options: []api.AttentionOption{
				action("Work until done", "POST", "/v1/repos/"+ref.ID+"/rounds", map[string]any{"beads": []string{x.ID}, "land_when_ready": true}),
				action("Back to Ready", "PATCH", "/v1/repos/"+ref.ID+"/issues/"+x.ID, map[string]any{"status": "open"}),
				open("Open bead", "bead", ref.ID, x.ID),
			},
		})
	}
	return out
}

// unclosedItems lists beads whose work a round finished but couldn't close,
// while they are still open.
func unclosedItems(ref repos.Ref, r api.Round, beads map[string]bead) []api.AttentionItem {
	var out []api.AttentionItem
	for _, rb := range r.Beads {
		if rb.CloseError == "" || rb.Closed {
			continue
		}
		if b, ok := beads[rb.ID]; !ok || b.Status == "closed" {
			continue
		}
		reason := fmt.Sprintf("Done in round %s; gates and build pass", r.ID)
		out = append(out, api.AttentionItem{
			ID: digest(api.AttentionBeadUnclosed, ref.ID, r.ID, rb.ID, rb.CloseError), Kind: api.AttentionBeadUnclosed,
			Repo: ref.ID, RepoName: ref.Name, BeadID: rb.ID, RoundID: r.ID, Title: rb.Title + " is done but still open",
			Why:       "Round " + r.ID + " finished its work, but closing it failed: " + rb.CloseError,
			CreatedAt: r.UpdatedAt,
			Options: []api.AttentionOption{
				action("Close anyway", "POST", "/v1/repos/"+ref.ID+"/issues/"+rb.ID+"/close", map[string]any{"reason": reason, "force": true}),
				open("Open bead", "bead", ref.ID, rb.ID),
			},
		})
	}
	return out
}

func roundItems(ref repos.Ref, r api.Round) []api.AttentionItem {
	var out []api.AttentionItem
	roundPath := "/v1/rounds/" + r.ID
	openRound := open("Open round", "round", ref.ID, r.ID)
	titles := map[string]string{}
	for _, b := range r.Beads {
		titles[b.ID] = b.Title
	}
	if r.State == api.RoundNeedsDecision {
		for _, q := range r.Questions {
			if q.Answer != "" {
				continue
			}
			opts := make([]api.AttentionOption, 0, len(q.Options)+1)
			for _, o := range q.Options {
				opts = append(opts, action(o, "POST", roundPath+"/answers", map[string]any{"question_id": q.ID, "answer": o}))
			}
			opts = append(opts, openRound)
			why := q.Text
			if q.Problem != "" {
				why += " " + q.Problem
			}
			beadID := ""
			if len(q.Beads) > 0 {
				beadID = q.Beads[0]
			}
			out = append(out, api.AttentionItem{
				ID: digest(api.AttentionRoundQuestion, r.ID, q.ID, q.Problem), Kind: api.AttentionRoundQuestion,
				Repo: ref.ID, RepoName: ref.Name, BeadID: beadID, RoundID: r.ID,
				Title: "Round question about " + beadList(q.Beads, titles), Why: why, CreatedAt: r.UpdatedAt, Options: opts,
			})
		}
	}
	if r.Active() {
		for _, b := range r.Beads {
			if b.State != api.BeadParked {
				continue
			}
			out = append(out, api.AttentionItem{
				ID: digest(api.AttentionBeadParked, r.ID, b.ID, b.Reason), Kind: api.AttentionBeadParked,
				Repo: ref.ID, RepoName: ref.Name, BeadID: b.ID, RoundID: r.ID,
				Title: fmt.Sprintf("%s parked in round %s", b.ID, r.ID),
				Why:   or(b.Reason, b.Title+" stopped and is waiting for you to retry it or leave it out."), CreatedAt: r.UpdatedAt,
				Options: []api.AttentionOption{action("Retry", "POST", roundPath+"/beads/"+b.ID+"/retry", nil), openRound},
			})
		}
	}
	if r.State == api.RoundReadyToLand {
		out = append(out, api.AttentionItem{
			ID: digest(api.AttentionRoundReady, r.ID, r.FinalCommit), Kind: api.AttentionRoundReady,
			Repo: ref.ID, RepoName: ref.Name, RoundID: r.ID,
			Title: fmt.Sprintf("Round %s is ready to merge", r.ID),
			Why: fmt.Sprintf("Its %d beads passed the gates and the final build. Merging moves %s to %s.",
				len(r.Beads), or(r.Target, "the target"), short(r.FinalCommit)),
			CreatedAt: r.UpdatedAt,
			Options:   []api.AttentionOption{action("Merge into "+or(r.Target, "the target"), "POST", roundPath+"/land", nil), openRound},
		})
	}
	return out
}

func beadList(ids []string, titles map[string]string) string {
	if len(ids) == 1 {
		if t := titles[ids[0]]; t != "" {
			return ids[0] + " (" + t + ")"
		}
	}
	return strings.Join(ids, ", ")
}

func short(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func or(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// archivable reports whether every current section of the approved revision
// landed with matching evidence, no section bead was reopened or waits on a
// person, and no work found along the way is still open under the epic.
func archivable(c api.ChangeSummary, beads map[string]bead) bool {
	if c.ApprovedRevision == "" || c.ApprovedRevision != c.Revision || len(c.Sections) == 0 {
		return false
	}
	for _, s := range c.Sections {
		b, ok := beads[s.BeadID]
		if s.Evidence != api.EvidenceLanded || !ok || b.Status != "closed" {
			return false
		}
	}
	found := "found:" + c.ID
	for _, b := range beads {
		if b.Status != "closed" && has(b.Labels, found) {
			return false
		}
	}
	return true
}
