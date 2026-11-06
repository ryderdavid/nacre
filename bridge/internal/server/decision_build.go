package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/decisions"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
	"github.com/ryderdavid/nacre/bridge/internal/rounds"
	"github.com/ryderdavid/nacre/bridge/internal/specflow"
)

// buildError is a decision nacred can't build: 400 for a bad request, 404
// for an unknown target, 409 when the action doesn't apply now.
type buildError struct {
	status int
	msg    string
}

func (e *buildError) Error() string { return e.msg }

func badDecision(format string, a ...any) error {
	return &buildError{http.StatusBadRequest, fmt.Sprintf(format, a...)}
}
func noTarget(format string, a ...any) error {
	return &buildError{http.StatusNotFound, fmt.Sprintf(format, a...)}
}
func notNow(format string, a ...any) error {
	return &buildError{http.StatusConflict, fmt.Sprintf(format, a...)}
}

// limitCeiling is the most a limit decision may raise a round's budget to:
// the runaway ceiling, four times its budget and at least $100.
func limitCeiling(budget float64) float64 { return math.Max(budget*4, 100) }

// buildDecision computes a decision from its kind and target: the exact
// action, the revision it binds, the consequences and the options. repoID
// may be empty for round targets.
func (s *Server) buildDecision(ctx context.Context, repoID, kind string, t decisions.Target) (decisions.Record, error) {
	switch kind {
	case decisions.KindLand:
		return s.buildLand(t)
	case decisions.KindLimit:
		return s.buildLimit(t)
	case decisions.KindApprove, decisions.KindAdopt, decisions.KindFinish, decisions.KindSignoff:
	case "":
		return decisions.Record{}, badDecision("kind required: one of %s", strings.Join(decisions.AgentKinds, ", "))
	default:
		return decisions.Record{}, badDecision("A chat can raise %s decisions, not %q.", strings.Join(decisions.AgentKinds, ", "), kind)
	}
	ref, err := s.repoRef(repoID)
	if err != nil {
		return decisions.Record{}, err
	}
	switch kind {
	case decisions.KindApprove:
		return s.buildApprove(ctx, ref, t)
	case decisions.KindAdopt:
		return s.buildAdopt(ctx, ref, t)
	case decisions.KindFinish:
		return s.buildFinish(ctx, ref, t)
	default:
		return s.buildSignoff(ctx, ref, t)
	}
}

// repoRef finds a repository on this computer by key.
func (s *Server) repoRef(id string) (repos.Ref, error) {
	if id == "" {
		return repos.Ref{}, badDecision("repo required: this chat isn't in a repository")
	}
	for _, ref := range s.Repos.Refs() {
		if ref.ID == id {
			return ref, nil
		}
	}
	return repos.Ref{}, noTarget("unknown repo")
}

func (s *Server) hostName() string {
	if s.Config == nil {
		return ""
	}
	return s.Config.Host().Name
}

// subtitle is "<project> · <computer> · <detail>…".
func (s *Server) subtitle(repoName string, details ...string) string {
	parts := []string{repoName}
	if h := s.hostName(); h != "" {
		parts = append(parts, h)
	}
	for _, d := range details {
		if d != "" {
			parts = append(parts, d)
		}
	}
	return strings.Join(parts, " · ")
}

func short(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

func usd(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("$%.0f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// pushLine says what landing does to the target branch.
func (s *Server) pushLine(target string, push *bool) string {
	switch {
	case push == nil:
		return "Whether it pushes " + target + " depends on nacre.yaml (land.push)"
	case *push:
		return "Pushes " + target + " to origin"
	}
	host := s.hostName()
	if host == "" {
		host = "this computer"
	}
	return "Moves " + target + " on " + host + " only; doesn't push"
}

func mergeLabel(target string, push *bool) string {
	switch {
	case push == nil:
		return "Merge into " + target
	case *push:
		return "Merge into " + target + " and push"
	}
	return "Merge, don't push"
}

var checkboxRe = regexp.MustCompile(`(?m)^\s*[-*]\s+\[[ xX]\]`)

// seedOf resolves an approve or adopt target to the change's seed bead.
func (s *Server) seedOf(ref repos.Ref, t decisions.Target) (string, error) {
	if t.Bead != "" {
		if !validID(t.Bead) {
			return "", badDecision("invalid bead id")
		}
		return t.Bead, nil
	}
	if t.Change == "" {
		return "", badDecision("target needs a bead (the change's seed) or a change")
	}
	list, err := s.facts().Changes(ref.Path)
	if err != nil {
		return "", err
	}
	for _, c := range list {
		if (c.ChangeName == t.Change || c.ID == t.Change) && len(c.SeedIDs) > 0 {
			return c.SeedIDs[0], nil
		}
	}
	return "", noTarget("no change %q in %s", t.Change, ref.Name)
}

func changeName(c *specflow.Change) string {
	if c.ChangeName != "" {
		return c.ChangeName
	}
	return c.SeedID
}

func (s *Server) buildApprove(ctx context.Context, ref repos.Ref, t decisions.Target) (decisions.Record, error) {
	seed, err := s.seedOf(ref, t)
	if err != nil {
		return decisions.Record{}, err
	}
	env, err := s.facts().Change(ctx, ref.Path, seed)
	if err != nil {
		return decisions.Record{}, err
	}
	c := env.Change
	if c == nil {
		return decisions.Record{}, notNow("%s has no change to approve", seed)
	}
	name := changeName(c)
	if c.Phase != "awaiting_approval" {
		return decisions.Record{}, notNow("%s isn't awaiting approval (it is %s)", name, strings.ReplaceAll(c.Phase, "_", " "))
	}
	tasks := 0
	for _, sec := range c.Sections {
		tasks += len(checkboxRe.FindAllString(sec.Body, -1))
	}
	target, push := s.facts().Land(ref.Path)
	budget := s.roundBudgetDefault()
	path := "/v1/repos/" + ref.ID + "/issues/" + seed + "/change"
	approve := func(run, hold bool) decisions.Action {
		body := map[string]any{"action": "approve", "revision": c.Revision}
		if run {
			body["run"] = true
		}
		if hold {
			body["hold"] = true
		}
		return decisions.Action{Method: "POST", Path: path, Body: body}
	}
	plan := fmt.Sprintf("Freezes the plan at revision %s: %s", short(c.Revision), plural(len(c.Sections), "section", "sections"))
	if tasks > 0 {
		plan += ", " + plural(tasks, "task", "tasks")
	}
	landDetail := "Merges the epic into " + target + " once every section passes"
	if push != nil && *push {
		landDetail += ", and pushes"
	} else if push != nil {
		landDetail += "; doesn't push"
	}
	blocking := 0
	for _, f := range c.Findings {
		if (f.Severity == "blocker" || f.Severity == "high") && f.Disposition != "resolved" && f.Disposition != "rejected" {
			blocking++
		}
	}
	review := "Review passed"
	if len(c.Findings) > 0 {
		review = "Review: " + plural(len(c.Findings), "finding", "findings")
	}
	rec := decisions.Record{
		Decision: decisions.Decision{
			Repo: ref.ID, RepoName: ref.Name, Kind: decisions.KindApprove,
			Title:    "Approve " + name + " and run it",
			Subtitle: s.subtitle(ref.Name, "revision "+short(c.Revision)),
			Target:   decisions.Target{Change: name, Bead: seed, Revision: c.Revision},
			Consequences: []string{plan,
				"Up to " + usd(budget) + " for the round that runs it",
				s.pushLine(target, push) + " when it lands"},
			Evidence: []decisions.Evidence{
				{Label: review, Kind: "review", Ref: c.ID, Repo: ref.ID},
				{Label: "Seed " + seed, Kind: "bead", Ref: seed, Repo: ref.ID},
			},
			Body: strings.TrimSpace(c.Summary),
			Options: []decisions.Option{
				{ID: "run_hold", Label: "Approve, run, I'll land", Detail: "Runs the sections and stops before " + target + " when the gates pass",
					Recommended: true, Protected: true},
				{ID: "run_land", Label: "Approve, run and land to " + target, Detail: landDetail, Protected: true},
				{ID: "approve_only", Label: "Approve only", Detail: "Creates the epic and section beads; nothing runs", Protected: true},
			},
			AllowDefer: true, AllowReject: true, RejectLabel: "Not right", RejectNeedsNote: true,
		},
		Actions: map[string]decisions.Action{
			"run_hold": approve(true, true), "run_land": approve(true, false), "approve_only": approve(false, false),
		},
		Reject: &decisions.Action{Method: "POST", Path: path, Body: map[string]any{"action": "revise"}, NoteField: "feedback"},
		Bound:  c.Revision, Key: "approve:" + ref.ID + ":" + seed, Follow: true,
	}
	if blocking > 0 {
		rec.Decision.Consequences = append(rec.Decision.Consequences, plural(blocking, "blocking finding is", "blocking findings are")+" still open")
	}
	return rec, nil
}

func (s *Server) buildAdopt(ctx context.Context, ref repos.Ref, t decisions.Target) (decisions.Record, error) {
	if t.Bead == "" || t.Change == "" {
		return decisions.Record{}, badDecision("adopt needs a bead and the change to adopt")
	}
	if !validID(t.Bead) {
		return decisions.Record{}, badDecision("invalid bead id")
	}
	env, err := s.facts().Change(ctx, ref.Path, t.Bead)
	if err != nil {
		return decisions.Record{}, err
	}
	if env.Change != nil {
		return decisions.Record{}, notNow("%s already has a change; open it instead of adopting another", t.Bead)
	}
	var a *specflow.Adoptable
	for i := range env.Adoptable {
		if env.Adoptable[i].ChangeName == t.Change {
			a = &env.Adoptable[i]
		}
	}
	if a == nil {
		return decisions.Record{}, notNow("%s isn't an active change Nacre can adopt for %s", t.Change, t.Bead)
	}
	if a.Problem != "" {
		return decisions.Record{}, notNow("%s can't be adopted as it stands: %s", t.Change, a.Problem)
	}
	files, err := changeFilesDigest(ref.Path, t.Change)
	if err != nil {
		return decisions.Record{}, err
	}
	path := "/v1/repos/" + ref.ID + "/issues/" + t.Bead + "/change"
	opts := []decisions.Option{{ID: "adopt", Label: "Adopt as written",
		Detail: "Takes the files over as they stand; an independent review runs before you approve", Recommended: true, Protected: true}}
	actions := map[string]decisions.Action{"adopt": {Method: "POST", Path: path, Body: map[string]any{"action": "adopt", "change_name": t.Change}}}
	if a.HasReview {
		opts = append(opts, decisions.Option{ID: "adopt_recorded_review", Label: "Adopt with its recorded review",
			Detail: "Lets its own review.md stand in for a new independent review", Protected: true})
		actions["adopt_recorded_review"] = decisions.Action{Method: "POST", Path: path,
			Body: map[string]any{"action": "adopt", "change_name": t.Change, "recorded_review": true}}
	}
	return decisions.Record{
		Decision: decisions.Decision{
			Repo: ref.ID, RepoName: ref.Name, Kind: decisions.KindAdopt,
			Title:    "Adopt " + t.Change + " as written",
			Subtitle: s.subtitle(ref.Name, t.Bead, "files "+short(files)),
			Target:   decisions.Target{Change: t.Change, Bead: t.Bead, Revision: files},
			Consequences: []string{
				fmt.Sprintf("Takes %s over as written: %d of %d tasks already checked", t.Change, a.TasksDone, a.TasksTotal),
				"Nacre tracks it from here; its sections become beads once you approve it",
			},
			Evidence:   []decisions.Evidence{{Label: "Seed " + t.Bead, Kind: "bead", Ref: t.Bead, Repo: ref.ID}},
			Body:       a.Why,
			Options:    opts,
			AllowDefer: true, AllowReject: true, RejectLabel: "Not now",
		},
		Actions: actions,
		Bound:   files,
		Key:     "adopt:" + ref.ID + ":" + t.Bead,
	}, nil
}

// landConsequences lists what landing a round does.
func (s *Server) landConsequences(round api.Round, push *bool) []string {
	out := []string{"Merges commit " + short(round.FinalCommit) + " into " + round.Target}
	if round.Epic != "" && round.Target == rounds.EpicBranch(round.Epic) {
		out[0] += ", the epic's branch; main moves when the epic finishes"
	}
	out = append(out, s.pushLine(round.Target, push))
	var closes, check, open []string
	for _, b := range round.Beads {
		switch {
		case b.State == api.BeadIntegrated && b.HumanCheck == "":
			closes = append(closes, b.ID)
		case b.State == api.BeadIntegrated:
			check = append(check, b.ID+" ("+b.HumanCheck+")")
		default:
			why := b.Reason
			if why == "" {
				why = strings.ReplaceAll(b.State, "_", " ")
			}
			open = append(open, b.ID+" ("+why+")")
		}
	}
	if len(closes) > 0 {
		out = append(out, "Closes "+plural(len(closes), "bead", "beads")+": "+strings.Join(closes, ", "))
	}
	if len(check) > 0 {
		out = append(out, "Leaves open for your check: "+strings.Join(check, "; "))
	}
	if len(open) > 0 {
		out = append(out, "Leaves open: "+strings.Join(open, "; "))
	}
	if round.Version != "" {
		out = append(out, "Version "+round.Version)
	}
	return out
}

func (s *Server) buildLand(t decisions.Target) (decisions.Record, error) {
	if t.Round == "" {
		return decisions.Record{}, badDecision("land needs a round")
	}
	round, err := s.round(t.Round)
	if err != nil {
		if errors.Is(err, rounds.ErrNotFound) {
			return decisions.Record{}, noTarget("unknown round")
		}
		return decisions.Record{}, err
	}
	if round.State != api.RoundReadyToLand {
		return decisions.Record{}, notNow("round %s isn't ready to land (it is %s)", round.ID, strings.ReplaceAll(round.State, "_", " "))
	}
	push := round.LandPush
	if push == nil && round.RepoPath != "" {
		_, push = s.facts().Land(round.RepoPath)
	}
	path := "/v1/rounds/" + round.ID
	body := ""
	if len(round.Artifacts) > 0 {
		body = "Build: " + strings.Join(round.Artifacts, "\n")
	}
	title := "Land round " + round.ID
	if round.Epic != "" {
		title += " (" + round.Epic + ")"
	}
	return decisions.Record{
		Decision: decisions.Decision{
			Repo: round.RepoID, RepoName: round.RepoName, Kind: decisions.KindLand,
			Title:        title + " into " + round.Target,
			Subtitle:     s.subtitle(round.RepoName, short(round.FinalCommit)),
			Target:       decisions.Target{Round: round.ID, Commit: round.FinalCommit, Bead: round.Epic},
			Consequences: s.landConsequences(round, push),
			Evidence: []decisions.Evidence{
				{Label: "Review the changes", Kind: "changes", Ref: round.ID},
				{Label: "Gates passed", Kind: "gates", Ref: round.ID},
				{Label: "Round " + round.ID, Kind: "round", Ref: round.ID},
			},
			Body: body,
			Options: []decisions.Option{
				{ID: "land", Label: mergeLabel(round.Target, push), Detail: "Lands commit " + short(round.FinalCommit), Recommended: true, Protected: true},
				{ID: "cancel", Label: "Cancel round", Detail: "Stops the round; its branches are kept"},
			},
			AllowDefer: true, AllowReject: true, RejectLabel: "Don't land",
		},
		Actions: map[string]decisions.Action{
			"land":   {Method: "POST", Path: path + "/land"},
			"cancel": {Method: "POST", Path: path + "/cancel"},
		},
		RejectHolds: true, Bound: round.FinalCommit, Key: "land:" + round.ID, Follow: true,
	}, nil
}

func (s *Server) buildFinish(ctx context.Context, ref repos.Ref, t decisions.Target) (decisions.Record, error) {
	epic := t.Bead
	changes, _ := s.facts().Changes(ref.Path)
	change := ""
	for _, c := range changes {
		if (t.Change != "" && (c.ChangeName == t.Change || c.ID == t.Change)) || (epic != "" && c.EpicID == epic) {
			if epic == "" {
				epic = c.EpicID
			}
			change = c.ChangeName
		}
	}
	if epic == "" {
		return decisions.Record{}, badDecision("finish needs the epic (target.bead) or its change")
	}
	if !validID(epic) {
		return decisions.Record{}, badDecision("invalid bead id")
	}
	b, err := s.facts().Bead(ctx, ref.Path, epic)
	if err != nil {
		return decisions.Record{}, beadErr(epic, err)
	}
	if b.Status == "closed" {
		return decisions.Record{}, notNow("%s is already closed", epic)
	}
	kids, err := s.facts().Children(ctx, ref.Path, epic)
	if err != nil {
		return decisions.Record{}, err
	}
	// Bound to the epic's beads as they are: one added, reopened or closed
	// since makes a different finish.
	parts := []string{"epic", b.Status}
	sort.Slice(kids, func(i, j int) bool { return kids[i].ID < kids[j].ID })
	open := 0
	for _, k := range kids {
		parts = append(parts, k.ID+"="+k.Status)
		if k.Status != "closed" {
			open++
		}
	}
	// And to what it would merge, where and how: the epic branch's commit and
	// nacre.yaml's landing target and push.
	target, push := s.facts().Land(ref.Path)
	pushes := "unknown"
	if push != nil {
		pushes = fmt.Sprint(*push)
	}
	parts = append(parts, "head="+s.facts().Head(ctx, ref.Path, rounds.EpicBranch(epic)), "target="+target, "push="+pushes)
	bound := hashOf(parts...)
	cons := []string{"Merges " + rounds.EpicBranch(epic) + " into " + target + " with every gate and the build"}
	title := "Finish " + epic
	if b.Title != "" {
		title += ": " + b.Title
	}
	if change != "" {
		cons = append(cons, "Archives "+change+" (openspec archive) in the same commit")
		title = "Archive " + change + " and merge into " + target
	}
	cons = append(cons, s.pushLine(target, push), "Closes "+epic+" and deletes its branch")
	if len(kids) > 0 {
		line := "Covers " + plural(len(kids), "bead", "beads") + " under " + epic
		if open > 0 {
			line += fmt.Sprintf("; %d still open", open)
		}
		cons = append(cons, line)
	}
	return decisions.Record{
		Decision: decisions.Decision{
			Repo: ref.ID, RepoName: ref.Name, Kind: decisions.KindFinish,
			Title: title, Subtitle: s.subtitle(ref.Name, epic),
			Target:       decisions.Target{Bead: epic, Change: change, Revision: bound},
			Consequences: cons,
			Evidence:     []decisions.Evidence{{Label: "Epic " + epic, Kind: "bead", Ref: epic, Repo: ref.ID}},
			Options: []decisions.Option{{ID: "finish", Label: mergeLabel(target, push), Detail: "Finishes " + epic,
				Recommended: true, Protected: true}},
			AllowDefer: true, AllowReject: true, RejectLabel: "Don't land",
		},
		Actions:     map[string]decisions.Action{"finish": {Method: "POST", Path: "/v1/repos/" + ref.ID + "/epics/" + epic + "/finish"}},
		RejectHolds: true, Bound: bound, Key: "finish:" + ref.ID + ":" + epic, Follow: true,
	}, nil
}

func (s *Server) buildSignoff(ctx context.Context, ref repos.Ref, t decisions.Target) (decisions.Record, error) {
	if t.Bead == "" {
		return decisions.Record{}, badDecision("signoff needs a bead")
	}
	if !validID(t.Bead) {
		return decisions.Record{}, badDecision("invalid bead id")
	}
	b, err := s.facts().Bead(ctx, ref.Path, t.Bead)
	if err != nil {
		return decisions.Record{}, beadErr(t.Bead, err)
	}
	if b.Status == "closed" || !b.HasLabel(api.SignoffLabel) {
		return decisions.Record{}, notNow("%s isn't waiting for your sign-off", t.Bead)
	}
	check, _ := b.Metadata[api.SignoffKey].(string)
	// Bound to what Ryder is asked to check: the bead's title, description
	// and check text.
	bound := hashOf("signoff", b.Title, b.Description, check)
	path := "/v1/repos/" + ref.ID + "/issues/" + t.Bead + "/signoff"
	return decisions.Record{
		Decision: decisions.Decision{
			Repo: ref.ID, RepoName: ref.Name, Kind: decisions.KindSignoff,
			Title: "Sign off " + t.Bead + ": " + b.Title, Subtitle: s.subtitle(ref.Name, t.Bead),
			Target: decisions.Target{Bead: t.Bead, Revision: bound},
			Consequences: []string{"Closes " + t.Bead + " as signed off",
				"Not right sends it back with your note; it stays open as ordinary work"},
			Evidence: []decisions.Evidence{{Label: t.Bead, Kind: "bead", Ref: t.Bead, Repo: ref.ID}},
			Body:     strings.TrimSpace(check),
			Options: []decisions.Option{{ID: "signoff", Label: "Sign off", Detail: "Closes " + t.Bead,
				Recommended: true, Protected: true}},
			AllowDefer: true, AllowReject: true, RejectLabel: "Not right", RejectNeedsNote: true,
		},
		Actions: map[string]decisions.Action{"signoff": {Method: "POST", Path: path, Body: map[string]any{"approve": true}, NoteField: "note"}},
		Reject:  &decisions.Action{Method: "POST", Path: path, Body: map[string]any{"approve": false}, NoteField: "note"},
		Bound:   bound, Key: "signoff:" + ref.ID + ":" + t.Bead,
	}, nil
}

func (s *Server) buildLimit(t decisions.Target) (decisions.Record, error) {
	if t.Round == "" {
		return decisions.Record{}, badDecision("limit needs a round")
	}
	round, err := s.round(t.Round)
	if err != nil {
		if errors.Is(err, rounds.ErrNotFound) {
			return decisions.Record{}, noTarget("unknown round")
		}
		return decisions.Record{}, err
	}
	if !round.Active() {
		return decisions.Record{}, notNow("round %s has ended", round.ID)
	}
	ceiling := limitCeiling(round.BudgetUSD)
	spent := round.SpentUSD
	cons := []string{"Spent " + usd(spent) + " of " + usd(round.BudgetUSD)}
	if round.State == api.RoundPaused && round.Stop != nil && round.Stop.Kind == api.StopLimits {
		cons = append(cons, "The round stopped at its limit; raising it resumes the round")
		if round.Stop.EstimateUSD > 0 {
			cons = append(cons, "nacred estimates "+usd(round.Stop.EstimateUSD)+" more to finish")
		}
	}
	cons = append(cons, "At most "+usd(ceiling)+" (the runaway ceiling)")
	return decisions.Record{
		Decision: decisions.Decision{
			Repo: round.RepoID, RepoName: round.RepoName, Kind: decisions.KindLimit,
			Title: "Raise round " + round.ID + "'s budget", Subtitle: s.subtitle(round.RepoName, round.ID),
			Target: decisions.Target{Round: round.ID}, Consequences: cons,
			Evidence: []decisions.Evidence{{Label: "Round " + round.ID, Kind: "round", Ref: round.ID}},
			Options: []decisions.Option{{ID: "raise", Label: "Raise the limit", Detail: "Sets the round's budget",
				Recommended: true, Protected: true,
				Input: &decisions.Input{Kind: decisions.InputAmount, Label: "New budget", Min: &spent, Max: &ceiling, Unit: "usd", Required: true}}},
			AllowDefer: true, AllowReject: true, RejectLabel: "Keep the limit",
		},
		Actions: map[string]decisions.Action{"raise": {Method: "POST", Path: "/v1/rounds/" + round.ID + "/limits", InputField: "budget_usd"}},
		Bound:   fmt.Sprintf("%.2f", round.BudgetUSD), Key: "limit:" + round.ID,
	}, nil
}

// expiryReason says why a decision bound to old expired now that fresh is
// current, naming both where they are revisions.
func expiryReason(d decisions.Decision, old, fresh string) string {
	switch d.Kind {
	case decisions.KindApprove:
		return "Expired: it was for revision " + short(old) + ", and revision " + short(fresh) + " is current. Ask again to decide on that."
	case decisions.KindLand:
		return "Expired: it was for commit " + short(old) + ", and commit " + short(fresh) + " is current. Ask again to decide on that."
	case decisions.KindLimit:
		return "Expired: it was for a budget of $" + old + ", and the budget is $" + fresh + " now. Ask again to decide on that."
	case decisions.KindAdopt:
		return "Expired: " + d.Target.Change + "'s files changed since this was raised (files " + short(old) + ", now " + short(fresh) + "). Ask again to decide on what they say now."
	case decisions.KindFinish:
		return "Expired: the beads under " + d.Target.Bead + ", its branch or where it lands changed since this was raised. Ask again to decide on finishing it as it is now."
	case decisions.KindSignoff:
		return "Expired: " + d.Target.Bead + "'s check changed since this was raised. Ask again to sign off what it asks now."
	}
	return "Expired: what it was for changed. Ask again to decide on it as it is now."
}

// hashOf is a short digest of parts, for binding a decision to content.
func hashOf(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:10])
}

// changeFilesDigest binds an adoption to the change's files as they stand:
// every file under openspec/changes/<name>/, by path and content. A missing
// folder digests as empty.
func changeFilesDigest(repoPath, name string) (string, error) {
	if name == "" || strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
		return "", badDecision("invalid change name")
	}
	root := filepath.Join(repoPath, "openspec", "changes", name)
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !e.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", filepath.ToSlash(rel), len(data))
		h.Write(data)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)[:10]), nil
}

// beadErr is a bead that couldn't be read: 404 when bd says it doesn't
// exist, the read's own error otherwise (a failed read isn't a missing bead).
func beadErr(id string, err error) error {
	var be *bd.Error
	if (errors.As(err, &be) && be.NotFound) || strings.Contains(err.Error(), "no issue") {
		return noTarget("%s: %v", id, err)
	}
	return err
}
