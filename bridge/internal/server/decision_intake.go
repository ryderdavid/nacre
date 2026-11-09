package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"unicode"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/chat"
	"github.com/ryderdavid/nacre/bridge/internal/decisions"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
	"github.com/ryderdavid/nacre/bridge/internal/rounds"
	"github.com/ryderdavid/nacre/bridge/internal/specflow"
)

// maxIntakeDraft bounds an intake draft, as POST /v1/repos/{repo}/intake does.
const maxIntakeDraft = 24000

var draftTypes = map[string]bool{"": true, "task": true, "bug": true, "feature": true, "chore": true, "epic": true}

// specMessage is a specflow error without its sentinel's prefix.
func specMessage(err error) string {
	msg := err.Error()
	for _, sentinel := range []error{specflow.ErrConflict, specflow.ErrUnavailable} {
		msg = strings.TrimPrefix(msg, sentinel.Error()+": ")
	}
	return msg
}

// verdictLabel is how a card names an intake route.
func verdictLabel(verdict, change string) string {
	switch verdict {
	case specflow.VerdictBead:
		return "Just a bead"
	case specflow.VerdictAmend:
		return "Amends " + change
	case specflow.VerdictCovered:
		return "Already covered"
	}
	return "New change"
}

// intakeFeedback is the verdict as feedback for developing the proposal.
func intakeFeedback(v specflow.IntakeVerdict) string {
	var b strings.Builder
	b.WriteString("Intake verdict: " + verdictLabel(v.Verdict, v.ChangeName) + ". " + strings.TrimSpace(v.Rationale))
	if len(v.References) > 0 {
		b.WriteString("\nCited:")
		for _, r := range v.References {
			b.WriteString("\n- " + r.Path)
			if r.Requirement != "" {
				b.WriteString(" (Requirement: " + r.Requirement + ")")
			}
		}
	}
	if v.Fallback != "" {
		b.WriteString("\n" + v.Fallback)
	}
	out := b.String()
	if len(out) > 11000 {
		out = out[:11000] + "…"
	}
	return out
}

// buildIntake runs intake on a chat's New effort draft and builds the card
// Ryder answers: Amends <change> for each active change, New change, Just a
// bead and Already covered, the verdict's route recommended. Choosing a
// change creates the bead, then develops it with the verdict as feedback;
// nothing develops before Ryder chooses. t.Bead names an existing bead to
// use instead of creating one.
func (s *Server) buildIntake(ctx context.Context, repoID, chatID string, t decisions.Target, draft *specflow.IntakeDraft) (decisions.Record, error) {
	ref, err := s.repoRef(repoID)
	if err != nil {
		return decisions.Record{}, err
	}
	if draft == nil || strings.TrimSpace(draft.Title) == "" {
		return decisions.Record{}, badDecision("intake needs a draft with a title: {\"draft\": {\"title\", \"description\"?, \"design\"?, \"acceptance\"?, \"type\"?, \"parent\"?}}")
	}
	d := *draft
	d.Title = strings.TrimSpace(d.Title)
	switch {
	case len(d.Digest) > chat.MaxDigest:
		return decisions.Record{}, badDecision("draft.digest is longer than %d bytes; keep it to the main points", chat.MaxDigest)
	case len(d.Title)+len(d.Description)+len(d.Design)+len(d.Acceptance)+len(d.Digest) > maxIntakeDraft:
		return decisions.Record{}, badDecision("The draft is too long for intake; shorten it.")
	case !draftTypes[d.Type]:
		return decisions.Record{}, badDecision("draft.type is task, bug, feature, chore or epic")
	case d.Parent != "" && !validID(d.Parent):
		return decisions.Record{}, badDecision("invalid parent id")
	}
	existing := t.Bead
	if existing != "" {
		if !validID(existing) {
			return decisions.Record{}, badDecision("invalid bead id")
		}
		if _, err := s.facts().Bead(ctx, ref.Path, existing); err != nil {
			return decisions.Record{}, beadErr(existing, err)
		}
	}
	v, err := s.facts().Intake(ctx, ref.Path, d)
	failed := ""
	if err != nil {
		switch {
		case errors.Is(err, specflow.ErrConflict):
			return decisions.Record{}, badDecision("%s", specMessage(err))
		case errors.Is(err, specflow.ErrUnavailable):
			return decisions.Record{}, &buildError{http.StatusServiceUnavailable, specMessage(err)}
		}
		// Intake timed out, hit its cap or answered nonsense: the card still
		// offers Just a bead and New change, with nothing recommended.
		failed = "Intake couldn't decide where this goes: " + intakeFailure(err) + ". Pick one yourself."
		v = specflow.IntakeVerdict{CostUSD: v.CostUSD}
	}
	summaries, _ := s.facts().Changes(ref.Path)
	byName := map[string]api.ChangeSummary{}
	for _, c := range summaries {
		byName[c.ChangeName] = c
	}
	// active names the round or run using a change, without a closing stop.
	active := func(name string) string {
		work := ""
		if c, ok := byName[name]; ok && c.Active != "" {
			work = c.Active
		} else if name == v.ChangeName {
			work = v.TargetActive
		}
		return strings.TrimRight(strings.TrimSpace(work), ".")
	}

	limit := usd(specflow.MaxActionCostUSD)
	issues := "/v1/repos/" + ref.ID + "/issues"
	feedback := intakeFeedback(v)
	if failed != "" {
		feedback = "Intake couldn't run; Ryder chose the route himself."
	}
	if existing != "" && strings.TrimSpace(d.Digest) != "" {
		// The bead already exists, so its notes aren't ours to fill: the
		// brainstorm travels with the develop request instead.
		feedback = "From the brainstorm:\n" + strings.TrimSpace(d.Digest) + "\n\n" + feedback
	}
	create := func(extra string) decisions.Action {
		body := map[string]any{"title": d.Title}
		desc := d.Description
		if extra != "" {
			desc = strings.TrimSpace(desc + "\n\n" + extra)
		}
		for k, val := range map[string]string{"description": desc, "design": d.Design, "acceptance_criteria": d.Acceptance, "issue_type": d.Type, "parent": d.Parent, "notes": strings.TrimSpace(d.Digest)} {
			if val != "" {
				body[k] = val
			}
		}
		// Ryder's note on the card is added to the description.
		return decisions.Action{Method: "POST", Path: issues, Body: body, Done: "Created {id}", NoteField: "owner_note"}
	}
	develop := func(route, change, done string) decisions.Action {
		hint := map[string]any{"verdict": route}
		if change != "" {
			hint["change_name"] = change
		}
		bead := "{id}"
		if existing != "" {
			bead = existing
		}
		return decisions.Action{Method: "POST", Path: issues + "/" + bead + "/change",
			Body:  map[string]any{"action": "develop", "feedback": feedback, "intake": hint},
			Label: "development", Done: done,
			// Ryder's note on the card is added to the feedback.
			NoteField: "owner_note"}
	}
	// developing either creates the bead and then develops it as what, or
	// develops the existing one.
	developing := func(route, change, what string) decisions.Action {
		if existing != "" {
			return develop(route, change, "Developing "+existing+" as "+what)
		}
		act := create("")
		then := develop(route, change, "Created {id}; developing it as "+what)
		act.Then = &then
		return act
	}

	var opts []decisions.Option
	actions := map[string]decisions.Action{}
	newDetail := "Files it, then writes and reviews a proposal for you to approve."
	if existing != "" {
		newDetail = "Writes and reviews a proposal from " + existing + " for you to approve."
	}
	opts = append(opts, decisions.Option{ID: "new", Label: "New change", Detail: newDetail, Recommended: v.Verdict == specflow.VerdictNew})
	actions["new"] = developing(specflow.VerdictNew, "", "a new change")
	for _, name := range v.ActiveChanges {
		id := "amend:" + name
		detail := "Files it, then adds it to the " + name + " proposal for you to approve again."
		if existing != "" {
			detail = "Adds " + existing + " to the " + name + " proposal for you to approve again."
		}
		opt := decisions.Option{ID: id, Label: "Amends " + name, Detail: detail,
			Recommended: v.Verdict == specflow.VerdictAmend && name == v.ChangeName}
		actions[id] = developing(specflow.VerdictAmend, name, "an amendment to "+name)
		if work := active(name); work != "" {
			opt.Disabled = name + " has active work (" + work + "); it can't be amended until that ends."
			if opt.Recommended && existing == "" {
				opt.Recommended = false
				opts = append(opts, decisions.Option{ID: "later", Label: "Create the bead, develop later",
					Detail:      "Files it now, noting it amends " + name + ". Add it to the proposal once " + work + " ends.",
					Recommended: true})
				later := create("Intake: this amends " + name + ". Develop it once its active work (" + work + ") ends.")
				later.Done = "Created {id}; develop it as an amendment to " + name + " once " + work + " ends"
				actions["later"] = later
			}
		}
		opts = append(opts, opt)
	}
	beadOpt := decisions.Option{ID: "bead", Label: "Just a bead", Detail: "Files it as a bead. Nothing else happens.",
		Recommended: v.Verdict == specflow.VerdictBead}
	if v.Verdict == specflow.VerdictCovered {
		beadOpt.Detail = "Files it as a bead anyway. Nothing else happens."
	}
	if existing != "" {
		beadOpt.Label, beadOpt.Detail = "Keep it as a bead", "Leaves "+existing+" as ordinary work. Nothing else happens."
	} else {
		actions["bead"] = create("")
	}
	opts = append(opts, beadOpt)
	if failed == "" {
		opts = append(opts, decisions.Option{ID: "covered", Label: "Already covered", Detail: "Nothing is filed. Its links show what already covers it.",
			Recommended: v.Verdict == specflow.VerdictCovered})
	}
	sort.SliceStable(opts, func(i, j int) bool { return opts[i].Recommended && !opts[j].Recommended })

	var cons []string
	if failed == "" {
		cons = append(cons, "Suggested: "+verdictLabel(v.Verdict, v.ChangeName)+", because "+lowerFirst(firstSentence(v.Rationale)))
	}
	cons = append(cons, "Nothing happens until you pick one",
		"Intake cost "+usd(v.CostUSD)+". Writing a proposal is capped at "+limit)
	if v.Fallback != "" {
		cons = append(cons, v.Fallback)
	}
	if existing != "" {
		cons = append(cons, "Uses "+existing+"; no new bead is created")
	}
	var ev []decisions.Evidence
	if c, ok := byName[v.ChangeName]; ok && v.ChangeName != "" {
		ev = append(ev, decisions.Evidence{Label: "Open " + v.ChangeName, Kind: "review", Ref: c.ID, Repo: ref.ID})
	}
	for i, r := range v.References {
		if i == 6 {
			break
		}
		label := r.Path
		if r.Requirement != "" {
			label = r.Requirement
		}
		ev = append(ev, decisions.Evidence{Label: label, Kind: "spec", Ref: r.Path, Repo: ref.ID})
	}
	if existing != "" {
		ev = append(ev, decisions.Evidence{Label: existing, Kind: "bead", Ref: existing, Repo: ref.ID})
	}
	return decisions.Record{
		Decision: decisions.Decision{
			Repo: ref.ID, RepoName: ref.Name, Kind: decisions.KindIntake,
			Title:        "Where does “" + firstLine(d.Title, 60) + "” belong?",
			Subtitle:     s.subtitle(ref.Name, "intake"),
			Target:       decisions.Target{Bead: existing, Change: v.ChangeName},
			Consequences: cons, Evidence: ev,
			Body:       strings.TrimSpace(or(failed, v.Rationale)),
			Options:    opts,
			AllowDefer: true, AllowReject: true, RejectLabel: "Drop it",
		},
		Actions: actions,
		Key:     "intake:" + ref.ID + ":" + hashOf(d.Title, existing),
	}, nil
}

// foundChange finds the change found work belongs to: a spec round's, the
// change named, or the change of a bead that is its epic, a section, or a
// bead under the epic.
func (s *Server) foundChange(ctx context.Context, ref repos.Ref, t decisions.Target, round *api.Round) (*api.ChangeSummary, error) {
	list, err := s.facts().Changes(ref.Path)
	if err != nil {
		return nil, err
	}
	var change *api.ChangeSummary
	pick := func(match func(c api.ChangeSummary) bool) {
		for i := range list {
			if change == nil && match(list[i]) {
				change = &list[i]
			}
		}
	}
	if round != nil {
		ids := map[string]bool{}
		for _, b := range round.Beads {
			if b.SpecChangeID != "" {
				ids[b.SpecChangeID] = true
			}
		}
		pick(func(c api.ChangeSummary) bool { return ids[c.ID] || (round.Epic != "" && c.EpicID == round.Epic) })
	}
	if t.Change != "" {
		pick(func(c api.ChangeSummary) bool { return c.ChangeName == t.Change || c.ID == t.Change })
	}
	if change == nil && t.Bead != "" {
		pick(func(c api.ChangeSummary) bool {
			if c.EpicID == t.Bead {
				return true
			}
			for _, sec := range c.Sections {
				if sec.BeadID == t.Bead {
					return true
				}
			}
			return false
		})
		if change == nil {
			b, err := s.facts().Bead(ctx, ref.Path, t.Bead)
			if err != nil {
				return nil, beadErr(t.Bead, err)
			}
			for _, l := range b.Labels {
				if id, ok := strings.CutPrefix(l, specflow.FoundLabelPrefix); ok {
					pick(func(c api.ChangeSummary) bool { return c.ID == id })
				}
			}
			// Walk up through section beads and beads under them to the epic.
			parent := b.Parent
			for depth := 0; change == nil && parent != "" && depth < 6; depth++ {
				p := parent
				pick(func(c api.ChangeSummary) bool {
					if c.EpicID == p {
						return true
					}
					for _, sec := range c.Sections {
						if sec.BeadID == p {
							return true
						}
					}
					return false
				})
				if change != nil {
					break
				}
				pb, err := s.facts().Bead(ctx, ref.Path, p)
				if err != nil {
					break
				}
				parent = pb.Parent
			}
		}
	}
	return change, nil
}

// buildFound builds a found-work card from what a chat was told: where the
// work was found and a note. Its answers are Only how or in what order (a
// bead under the epic, optionally before an open section), Changes what
// done means (a pending amendment, which Needs you offers to revise the
// change with) and Out of scope (a bead related to the epic, not under it,
// and a new effort for it in the chat). From a round, the card asks which
// round bead it was found in, defaulting to the one given.
func (s *Server) buildFound(ctx context.Context, repoID, chatID string, t decisions.Target, note string) (decisions.Record, error) {
	ref, err := s.repoRef(repoID)
	if err != nil {
		return decisions.Record{}, err
	}
	note = strings.TrimSpace(note)
	switch {
	case note == "":
		return decisions.Record{}, badDecision("found needs a note: what was found")
	case len(note) > specflow.MaxFoundNote:
		return decisions.Record{}, badDecision("keep the note under %d characters", specflow.MaxFoundNote)
	case t.Bead != "" && !validID(t.Bead):
		return decisions.Record{}, badDecision("invalid bead id")
	case t.Bead == "" && t.Round == "" && t.Change == "":
		return decisions.Record{}, badDecision("found needs where it was found: target bead (the epic, a section or a bead under them), round or change")
	}
	var round *api.Round
	if t.Round != "" {
		r, err := s.round(t.Round)
		if err != nil {
			if errors.Is(err, rounds.ErrNotFound) {
				return decisions.Record{}, noTarget("unknown round")
			}
			return decisions.Record{}, err
		}
		if r.RepoID != "" && r.RepoID != ref.ID {
			return decisions.Record{}, badDecision("round %s isn't in %s", r.ID, ref.Name)
		}
		if t.Bead != "" && !roundHas(r, t.Bead) {
			return decisions.Record{}, badDecision("%s isn't one of round %s's beads", t.Bead, r.ID)
		}
		round = &r
	}
	change, err := s.foundChange(ctx, ref, t, round)
	if err != nil {
		return decisions.Record{}, err
	}
	if change == nil {
		return decisions.Record{}, notNow("Found work is recorded for a spec epic, its sections, beads under them or a spec round; %s isn't one. File it as a bead, or raise intake for it.",
			firstOf(t.Round, t.Bead, t.Change))
	}
	name := change.ChangeName
	if change.EpicID == "" {
		return decisions.Record{}, notNow("%s has no epic yet: give feedback on its proposal instead of recording found work", name)
	}
	var ask *decisions.Input
	if round != nil {
		var choices []decisions.Choice
		for _, b := range round.Beads {
			label := b.ID
			if b.Title != "" {
				label += " · " + b.Title
			}
			choices = append(choices, decisions.Choice{ID: b.ID, Label: label})
		}
		if len(choices) > 0 {
			ask = &decisions.Input{Kind: decisions.InputChoice, Label: "Found in which round bead?", Choices: choices, Default: t.Bead, Required: true}
		}
	}
	path := "/v1/repos/" + ref.ID + "/changes/" + change.ID + "/found"
	askField := ""
	if ask != nil {
		askField = "found_in"
	}
	body := func(answer string) map[string]any {
		b := map[string]any{"answer": answer, "note": note}
		if ask == nil && t.Bead != "" {
			b["found_in"] = t.Bead
		}
		return b
	}
	how := decisions.Option{ID: "how_order", Label: "Changes how or order",
		Detail: "A new bead under " + change.EpicID + "; the approved plan stays as it is, and the bead runs as ordinary work"}
	if choices := openSections(*change); len(choices) > 0 {
		how.Input = &decisions.Input{Kind: decisions.InputChoice, Label: "Required before which section?", Choices: choices}
	}
	oos := body(specflow.FoundOutOfScope)
	oos["create"] = true
	effort := effortAction(ref.ID, chatID, name, change.EpicID, note)
	actions := map[string]decisions.Action{
		"how_order": {Method: "POST", Path: path, Body: body(specflow.FoundHowOrder), InputField: "before_section", AskField: askField,
			Done: "Filed {id} under " + change.EpicID + "; the approved plan stays as it is"},
		"amend": {Method: "POST", Path: path, Body: body(specflow.FoundAmend), AskField: askField,
			Done: "Filed {id} as a pending amendment to " + name + "; Needs you offers to revise the change with it"},
		"out_of_scope": {Method: "POST", Path: path, Body: oos, AskField: askField, Then: &effort},
	}
	ev := []decisions.Evidence{}
	if t.Bead != "" {
		ev = append(ev, decisions.Evidence{Label: "Found in " + t.Bead, Kind: "bead", Ref: t.Bead, Repo: ref.ID})
	}
	if round != nil {
		ev = append(ev, decisions.Evidence{Label: "Round " + round.ID, Kind: "round", Ref: round.ID})
	}
	ev = append(ev, decisions.Evidence{Label: "Open " + name, Kind: "review", Ref: change.ID, Repo: ref.ID})
	return decisions.Record{
		Decision: decisions.Decision{
			Repo: ref.ID, RepoName: ref.Name, Kind: decisions.KindFound,
			Title:    "Found while working on " + name + ": " + firstLine(note, 70),
			Subtitle: s.subtitle(ref.Name, firstOf(t.Round, t.Bead, change.EpicID)),
			Target:   decisions.Target{Change: name, Bead: t.Bead, Round: t.Round},
			Consequences: []string{"Nothing here spends anything",
				"Changing how or order leaves the approved revision current; changing what done means needs a revision before " + name + " runs again",
				"Out of scope starts a new effort for it in this chat"},
			Evidence: ev,
			Body:     note,
			Ask:      ask,
			Options: []decisions.Option{how,
				{ID: "amend", Label: "Changes what done means", Detail: "A pending amendment to " + name + ": the change needs a revision (a new review and your approval) before it runs again"},
				{ID: "out_of_scope", Label: "Out of scope", Detail: "A new bead related to " + change.EpicID + ", not under it, and a new effort for it starts in this chat, prefilled"},
			},
			AllowDefer: true, AllowReject: true, RejectLabel: "Not worth recording",
		},
		Actions: actions,
		Key:     "found:" + ref.ID + ":" + hashOf(change.ID, t.Round, t.Bead, note),
	}, nil
}

func roundHas(r api.Round, bead string) bool {
	for _, b := range r.Beads {
		if b.ID == bead {
			return true
		}
	}
	return false
}

// openSections are a change's sections not yet landed, as choices.
func openSections(c api.ChangeSummary) []decisions.Choice {
	var out []decisions.Choice
	for i, sec := range c.Sections {
		if sec.Evidence == api.EvidenceLanded {
			continue
		}
		out = append(out, decisions.Choice{ID: sec.ID, Label: itoa(i+1) + ". " + sec.Title})
	}
	return out
}

// effortAction starts a new effort for out-of-scope work in chat (or a new
// chat in repo), prefilled with what was found and the bead filed for it.
func effortAction(repo, chatID, change, epic, note string) decisions.Action {
	text := "New effort, found out of scope of " + change + ": " + note +
		"\n\nIt's filed as {id}, related to epic " + epic + " but not part of it. Talk it through with me and draft it, then raise intake for {id} with that draft."
	return decisions.Action{Method: decisions.MethodEffort, Body: map[string]any{"repo": repo, "chat": chatID, "text": text, "bead": "{id}"},
		Label: "the new effort", Done: "Filed {id}, related to " + epic + " and not under it"}
}

// startEffort carries out an effort action: the text goes to its chat as a
// message, or to a new chat in its repository when that chat is busy, gone
// or not given.
func (s *Server) startEffort(act decisions.Action) (int, []byte) {
	fail := func(status int, msg string) (int, []byte) {
		b, _ := json.Marshal(map[string]string{"error": msg})
		return status, b
	}
	if s.Chats == nil {
		return fail(http.StatusServiceUnavailable, "Chat isn't available on this computer.")
	}
	text, _ := act.Body["text"].(string)
	chatID, _ := act.Body["chat"].(string)
	repo, _ := act.Body["repo"].(string)
	bead, _ := act.Body["bead"].(string)
	ok := func(id, where string) (int, []byte) {
		if validID(bead) {
			// Intake from this chat's drafts is for the bead just filed.
			s.Chats.SetEffortBead(id, bead)
		}
		b, _ := json.Marshal(map[string]string{"chat_id": id, "message": "a new effort for it started in " + where})
		return http.StatusOK, b
	}
	if chatID != "" {
		if _, err := s.Chats.Send(chatID, chat.SendRequest{Text: text}); err == nil {
			return ok(chatID, "its chat")
		}
	}
	var r *string
	if repo != "" {
		r = &repo
	}
	rec, err := s.Chats.Create(chat.CreateRequest{Agent: chat.AgentClaude, Repo: r})
	if err != nil {
		return fail(http.StatusConflict, "A chat for it couldn't be made: "+err.Error())
	}
	if _, err := s.Chats.Send(rec.ID, chat.SendRequest{Text: text}); err != nil {
		return fail(http.StatusConflict, "Its new chat couldn't start: "+err.Error())
	}
	return ok(rec.ID, "a new chat")
}

// execChain executes an action and, once it succeeded, its Then with the id
// the first response names. It returns the last response and, when the
// chain says one, the decision's message: what it did, or what was done
// before a later step failed.
func (s *Server) execChain(ctx context.Context, rec decisions.Record, act decisions.Action) (int, []byte, string) {
	status, resp := s.execAction(ctx, act)
	if status/100 != 2 {
		return status, resp, ""
	}
	id := createdID(resp)
	s.linkCreated(rec, id, resp)
	last := act
	if act.Then != nil {
		then := withID(*act.Then, id)
		tstatus, tresp := s.execAction(ctx, then)
		if tstatus/100 != 2 {
			done := strings.ReplaceAll(or(act.Done, "{id} was done"), "{id}", or(id, "the first step"))
			return tstatus, tresp, done + ", but " + or(then.Label, "the next step") + " didn't start: " + errorMessage(tresp, tstatus)
		}
		s.linkCreated(rec, "", tresp)
		status, resp, last = tstatus, tresp, then
	}
	done := or(last.Done, act.Done)
	if done == "" {
		return status, resp, ""
	}
	msg := strings.ReplaceAll(done, "{id}", id)
	var said struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(resp, &said) == nil && said.Message != "" {
		msg += "; " + said.Message
	}
	return status, resp, msg
}

// createdID is the bead a response names: a created issue's id
// ({"issue": {"id"}}), or a found answer's bead_id.
func createdID(resp []byte) string {
	var r struct {
		Issue struct {
			ID string `json:"id"`
		} `json:"issue"`
		BeadID string `json:"bead_id"`
	}
	_ = json.Unmarshal(resp, &r)
	return or(r.BeadID, r.Issue.ID)
}

// withID replaces "{id}" in an action's path and string body values.
func withID(a decisions.Action, id string) decisions.Action {
	a.Path = strings.ReplaceAll(a.Path, "{id}", id)
	body := map[string]any{}
	for k, v := range a.Body {
		if str, ok := v.(string); ok {
			v = strings.ReplaceAll(str, "{id}", id)
		}
		body[k] = v
	}
	a.Body = body
	return a
}

// linkCreated ties what a chat's decision created (a bead, a change) to the
// chat, so later decisions about it show in that chat.
func (s *Server) linkCreated(rec decisions.Record, id string, resp []byte) {
	if rec.Decision.ChatID == nil || *rec.Decision.ChatID == "" {
		return
	}
	chatID, repo := *rec.Decision.ChatID, rec.Decision.Repo
	if id != "" {
		s.Decisions.LinkWork(beadLink(repo, id), chatID)
	}
	var env struct {
		Change *struct {
			ID     string `json:"id"`
			SeedID string `json:"seed_id"`
		} `json:"change"`
	}
	if json.Unmarshal(resp, &env) == nil && env.Change != nil && env.Change.ID != "" {
		s.Decisions.LinkWork(changeLink(repo, env.Change.ID), chatID)
		if env.Change.SeedID != "" {
			s.Decisions.LinkWork(beadLink(repo, env.Change.SeedID), chatID)
		}
	}
}

// Work-to-chat link keys for beads and changes; rounds and runs are linked
// by their own ids.
func beadLink(repo, id string) string   { return "bead:" + repo + ":" + id }
func changeLink(repo, id string) string { return "change:" + repo + ":" + id }

// itemChats names the chat a Needs you item belongs to: the chat that
// started its round, the round that carried its bead, or created its bead
// or change. It reads the rounds once per listing.
func (s *Server) itemChats() func(api.AttentionItem) *string {
	if s.Decisions == nil {
		return func(api.AttentionItem) *string { return nil }
	}
	byRepo := map[string][]api.Round{}
	roundsOf := func(repo string) []api.Round {
		list, ok := byRepo[repo]
		if !ok {
			list = append([]api.Round{}, s.facts().RoundsOf(repo)...)
			sort.SliceStable(list, func(i, j int) bool { return list[i].CreatedAt > list[j].CreatedAt })
			byRepo[repo] = list
		}
		return list
	}
	return func(it api.AttentionItem) *string {
		if it.RoundID != "" {
			if c := s.Decisions.ChatOf(it.RoundID); c != "" {
				return &c
			}
		}
		if it.BeadID != "" {
			if c := s.Decisions.ChatOf(beadLink(it.Repo, it.BeadID)); c != "" {
				return &c
			}
			for _, r := range roundsOf(it.Repo) {
				if r.Epic == it.BeadID || r.FinishEpic == it.BeadID || roundHas(r, it.BeadID) {
					if c := s.Decisions.ChatOf(r.ID); c != "" {
						return &c
					}
				}
			}
		}
		if it.ChangeID != "" {
			if c := s.Decisions.ChatOf(changeLink(it.Repo, it.ChangeID)); c != "" {
				return &c
			}
		}
		return nil
	}
}

// firstSentence is text up to the end of its first sentence.
func firstSentence(text string) string {
	t := strings.TrimSpace(text)
	for i, r := range t {
		if (r == '.' || r == '!' || r == '?') && (i+1 == len(t) || t[i+1] == ' ' || t[i+1] == '\n') {
			return t[:i]
		}
	}
	return strings.TrimRight(t, ".")
}

// lowerFirst lowers a sentence's first letter, unless it starts a name such
// as an acronym or an identifier.
func lowerFirst(s string) string {
	if len(s) < 2 || !unicode.IsUpper(rune(s[0])) || unicode.IsUpper(rune(s[1])) {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// intakeFailure says in plain words why an intake call gave no verdict.
func intakeFailure(err error) string {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "longer than") || strings.Contains(msg, "deadline") || strings.Contains(msg, "timed out"):
		return "it took too long"
	case strings.Contains(msg, "budget") || strings.Contains(msg, "cost") || strings.Contains(msg, "limit"):
		return "it reached its spending limit"
	case strings.Contains(msg, "malformed") || strings.Contains(msg, "verdict") || strings.Contains(msg, "json"):
		return "its answer couldn't be read"
	}
	return "it stopped with an error"
}
