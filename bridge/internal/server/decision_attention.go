package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/decisions"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
)

// questionCheck prefixes the AttentionID of a round question's decision: it
// still applies while the round asks that question.
const questionCheck = "question:"

// attentionDecisions are the Needs you items as decisions, each with a
// stable id derived from its item (or, for a round question, from the round
// and the question). Items for an action a chat can raise (an approval, a
// land, a finish, a sign-off) are built like the chat's, so one decision
// serves the queue and the chat.
func (s *Server) attentionDecisions(ctx context.Context) []decisions.Record {
	if s.Attention == nil {
		return nil
	}
	refs := map[string]repos.Ref{}
	for _, ref := range s.Repos.Refs() {
		refs[ref.ID] = ref
	}
	var out []decisions.Record
	for _, it := range s.Attention.Items(ctx) {
		out = append(out, s.fromAttention(ctx, refs[it.Repo], it)...)
	}
	return out
}

// attentionStill reports whether the Needs you item a decision came from
// still waits on Ryder.
func (s *Server) attentionStill(ctx context.Context, id string) bool {
	if rest, ok := strings.CutPrefix(id, questionCheck); ok {
		roundID, qid, _ := strings.Cut(rest, ":")
		round, err := s.round(roundID)
		if err != nil || round.State != api.RoundNeedsDecision {
			return false
		}
		for _, q := range round.Questions {
			if q.ID == qid && q.Answer == "" {
				return true
			}
		}
		return false
	}
	if s.Attention == nil {
		return false
	}
	for _, it := range s.Attention.Items(ctx) {
		if it.ID == id {
			return true
		}
	}
	return false
}

// attentionKind maps a Needs you kind to a decision kind.
func attentionKind(kind string) string {
	switch kind {
	case api.AttentionProposalReady:
		return decisions.KindApprove
	case api.AttentionRoundReady:
		return decisions.KindLand
	case api.AttentionChangeArchivable, api.AttentionEpicFinishable:
		return decisions.KindFinish
	case api.AttentionBeadSignoff:
		return decisions.KindSignoff
	case api.AttentionFoundWork:
		return decisions.KindFound
	case api.AttentionApprovalStale, api.AttentionWorkflowInterrupted, api.AttentionBeadParked,
		api.AttentionBeadUnclosed, api.AttentionBeadStuck:
		return decisions.KindRecovery
	}
	return decisions.KindQuestion
}

func digestID(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "dc_" + hex.EncodeToString(h[:12])
}

// fromAttention turns one Needs you item into decisions.
func (s *Server) fromAttention(ctx context.Context, ref repos.Ref, it api.AttentionItem) []decisions.Record {
	if it.Kind == api.AttentionRoundQuestion {
		return s.questionDecisions(ref, it)
	}
	kind := attentionKind(it.Kind)
	var rec decisions.Record
	built := false
	if ref.ID != "" {
		var err error
		switch kind {
		case decisions.KindApprove:
			rec, err = s.buildApprove(ctx, ref, decisions.Target{Bead: it.BeadID})
		case decisions.KindLand:
			rec, err = s.buildLand(decisions.Target{Round: it.RoundID})
		case decisions.KindFinish:
			rec, err = s.buildFinish(ctx, ref, decisions.Target{Bead: it.BeadID})
		case decisions.KindSignoff:
			rec, err = s.buildSignoff(ctx, ref, decisions.Target{Bead: it.BeadID})
		default:
			err = errNotBuilt
		}
		built = err == nil
	}
	if !built {
		rec = s.genericDecision(ref, it, kind)
		if it.Kind == api.AttentionFoundWork {
			s.foundOptions(ref, it, &rec)
		}
	}
	d := &rec.Decision
	d.ID, d.Source, d.ChatID = "dc_"+it.ID, decisions.SourceAttention, nil
	d.Repo, d.RepoName = it.Repo, it.RepoName
	d.State, d.CreatedAt, d.ExpiresAt = decisions.StatePending, it.CreatedAt, nil
	d.AllowDefer = true
	if d.Body == "" {
		d.Body = it.Why
	}
	if d.Consequences == nil {
		d.Consequences = []string{}
	}
	if d.Evidence == nil {
		d.Evidence = []decisions.Evidence{}
	}
	if d.Options == nil {
		d.Options = []decisions.Option{}
	}
	rec.AttentionID = it.ID
	return []decisions.Record{rec}
}

var errNotBuilt = &buildError{0, "not built"}

// genericDecision maps an item's server-defined options: actions become
// options, navigations evidence.
func (s *Server) genericDecision(ref repos.Ref, it api.AttentionItem, kind string) decisions.Record {
	rec := decisions.Record{
		Decision: decisions.Decision{
			Kind: kind, Title: it.Title, Subtitle: s.subtitle(it.RepoName, firstOf(it.BeadID, it.RoundID)),
			Target: decisions.Target{Bead: it.BeadID, Round: it.RoundID, Change: it.ChangeID},
			Body:   it.Why,
		},
		Actions: map[string]decisions.Action{},
	}
	used := map[string]bool{}
	for _, o := range it.Options {
		switch o.Type {
		case "action":
			id := optionID(o.Label, used)
			opt := decisions.Option{ID: id, Label: o.Label, Recommended: o.Recommended, Disabled: o.Disabled}
			opt.Protected = len(s.protectedAction(o.Method, o.Path, o.Body)) > 0
			rec.Decision.Options = append(rec.Decision.Options, opt)
			rec.Actions[id] = decisions.Action{Method: o.Method, Path: o.Path, Body: o.Body}
		case "open":
			if ev, ok := evidenceOf(o); ok {
				rec.Decision.Evidence = append(rec.Decision.Evidence, ev)
			}
		}
	}
	if it.Kind == api.AttentionBeadDecision && it.BeadID != "" && ref.ID != "" {
		// A bead decision is answered in Ryder's own words, as a comment.
		id := optionID("answer", used)
		rec.Decision.Options = append(rec.Decision.Options, decisions.Option{ID: id, Label: "Answer",
			Detail: "Adds your answer to " + it.BeadID + " as a comment",
			Input:  &decisions.Input{Kind: decisions.InputText, Label: "Your answer", Required: true}})
		rec.Actions[id] = decisions.Action{Method: "POST", Path: "/v1/repos/" + ref.ID + "/issues/" + it.BeadID + "/comments", InputField: "text"}
	}
	return rec
}

func firstOf(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// optionID is a stable option id from a label, unique within used.
func optionID(label string, used map[string]bool) string {
	id := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(label), "_"), "_")
	if len(id) > 40 {
		id = strings.TrimRight(id[:40], "_")
	}
	if id == "" {
		id = "option"
	}
	base := id
	for n := 2; used[id]; n++ {
		id = base + "_" + string(rune('0'+n%10))
	}
	used[id] = true
	return id
}

// evidenceOf turns a navigation into evidence, when it names a bead, a
// change or a round.
func evidenceOf(o api.AttentionOption) (decisions.Evidence, bool) {
	t := o.Target
	if t == nil || t.ID == "" {
		return decisions.Evidence{}, false
	}
	switch t.Kind {
	case "bead", "round":
		return decisions.Evidence{Label: o.Label, Kind: t.Kind, Ref: t.ID, Repo: t.Repo}, true
	case "change":
		return decisions.Evidence{Label: o.Label, Kind: "review", Ref: t.ID, Repo: t.Repo}, true
	}
	return decisions.Evidence{}, false
}

// foundOptions answers a Found: comment on the card itself: how or order
// (optionally before a section), what done means, out of scope (a new bead
// related to the epic), or dismiss.
func (s *Server) foundOptions(ref repos.Ref, it api.AttentionItem, rec *decisions.Record) {
	var t *api.AttentionTarget
	for _, o := range it.Options {
		if o.Type == "open" && o.Target != nil && o.Target.Kind == "found_something" {
			t = o.Target
		}
	}
	if t == nil || ref.ID == "" {
		return
	}
	epic := ""
	var choices []decisions.Choice
	if list, err := s.facts().Changes(ref.Path); err == nil {
		for _, c := range list {
			if c.ID != t.Change {
				continue
			}
			epic = c.EpicID
			for i, sec := range c.Sections {
				choices = append(choices, decisions.Choice{ID: sec.ID, Label: itoa(i+1) + ". " + sec.Title})
			}
		}
	}
	found := "/v1/repos/" + ref.ID + "/changes/" + t.Change + "/found"
	body := func(answer string) map[string]any {
		return map[string]any{"answer": answer, "found_in": t.FoundIn, "comment_id": t.CommentID, "note": t.Note}
	}
	d := &rec.Decision
	d.Target = decisions.Target{Bead: t.FoundIn, Change: t.Change, Comment: t.CommentID}
	d.Body = t.Note
	how := decisions.Option{ID: "how_order", Label: "Changes how or order",
		Detail: "A new bead under the epic; the approved plan stays as it is"}
	if len(choices) > 0 {
		how.Input = &decisions.Input{Kind: decisions.InputChoice, Label: "Required before which section?", Choices: choices}
	}
	opts := []decisions.Option{how,
		{ID: "amend", Label: "Changes what done means", Detail: "A pending amendment: the change needs a revision before it runs again"},
		{ID: "out_of_scope", Label: "Out of scope", Detail: "A new bead related to the epic, outside this change"},
	}
	actions := map[string]decisions.Action{
		"how_order": {Method: "POST", Path: found, Body: body("how_order"), InputField: "before_section"},
		"amend":     {Method: "POST", Path: found, Body: body("amend")},
		"out_of_scope": {Method: "POST", Path: "/v1/repos/" + ref.ID + "/issues", Body: map[string]any{
			"title": firstLine(t.Note, 80), "description": t.Note, "issue_type": "task", "related": nonEmpty(epic),
			"found_comment": map[string]any{"bead": t.FoundIn, "comment_id": t.CommentID},
		}},
	}
	for _, o := range d.Options {
		if a, ok := rec.Actions[o.ID]; ok && o.ID == "dismiss" {
			opts = append(opts, o)
			actions["dismiss"] = a
		}
	}
	d.Options, rec.Actions = opts, actions
}

func nonEmpty(xs ...string) []string {
	out := []string{}
	for _, x := range xs {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

func firstLine(s string, n int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// questionDecisions makes one question decision per unanswered question of
// a round: its suggested answers, recommended first, and In my own words.
func (s *Server) questionDecisions(ref repos.Ref, it api.AttentionItem) []decisions.Record {
	var out []decisions.Record
	for _, q := range it.Questions {
		used := map[string]bool{}
		rec := decisions.Record{
			Decision: decisions.Decision{
				ID: digestID("question", it.RoundID, q.ID), Repo: it.Repo, RepoName: it.RepoName,
				Source: decisions.SourceAttention, Kind: decisions.KindQuestion,
				Title: it.Title, Subtitle: s.subtitle(it.RepoName, it.RoundID),
				Target:       decisions.Target{Round: it.RoundID, Question: q.ID},
				Consequences: []string{},
				Evidence:     []decisions.Evidence{{Label: "Round " + it.RoundID, Kind: "round", Ref: it.RoundID}},
				Body:         q.Text, AllowDefer: true,
				State: decisions.StatePending, CreatedAt: it.CreatedAt,
			},
			Actions:     map[string]decisions.Action{},
			AttentionID: questionCheck + it.RoundID + ":" + q.ID,
		}
		if q.Reason != "" {
			rec.Decision.Consequences = append(rec.Decision.Consequences, "Why the recommendation: "+q.Reason)
		}
		for _, o := range q.Options {
			if o.Type != "action" {
				continue
			}
			id := optionID(o.Label, used)
			rec.Decision.Options = append(rec.Decision.Options, decisions.Option{ID: id, Label: o.Label, Recommended: o.Recommended})
			rec.Actions[id] = decisions.Action{Method: o.Method, Path: o.Path, Body: o.Body}
		}
		id := optionID("own_words", used)
		rec.Decision.Options = append(rec.Decision.Options, decisions.Option{ID: id, Label: "In my own words",
			Input: &decisions.Input{Kind: decisions.InputText, Label: "Your answer", Required: true}})
		rec.Actions[id] = decisions.Action{Method: "POST", Path: "/v1/rounds/" + it.RoundID + "/answers",
			Body: map[string]any{"question_id": q.ID}, InputField: "answer"}
		out = append(out, rec)
	}
	return out
}
