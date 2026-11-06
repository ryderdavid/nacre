package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/chat"
	"github.com/ryderdavid/nacre/bridge/internal/decisions"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
	"github.com/ryderdavid/nacre/bridge/internal/specflow"
)

func (rg *decisionRig) repoDir(t *testing.T) string {
	t.Helper()
	p, err := rg.s.Repos.Path(rg.repo)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (rg *decisionRig) ref(t *testing.T) repos.Ref {
	t.Helper()
	for _, ref := range rg.s.Repos.Refs() {
		if ref.ID == rg.repo {
			return ref
		}
	}
	t.Fatal("no ref")
	return repos.Ref{}
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ---- binding real content ----

// Adopting is bound to the change's files: an edit to tasks.md after the
// card was raised expires it, naming both digests, and nothing runs.
func TestAdoptIsBoundToTheChangesFiles(t *testing.T) {
	rg := newDecisionRig(t)
	dir := filepath.Join(rg.repoDir(t), "openspec", "changes", "hand-written")
	writeFile(t, filepath.Join(dir, "proposal.md"), "## Why\nBecause.\n")
	writeFile(t, filepath.Join(dir, "tasks.md"), "## 1. Do\n- [x] 1.1 a\n- [ ] 1.2 b\n")
	rg.facts.changes["N-7"] = specflow.Envelope{Available: true, Adoptable: []specflow.Adoptable{{ChangeName: "hand-written", TasksDone: 1, TasksTotal: 2}}}
	code, d, raw := rg.raise(t, `{"kind":"adopt","target":{"bead":"N-7","change":"hand-written"}}`)
	if code != 201 || d.Target.Revision == "" || !strings.Contains(d.Subtitle, "files "+short(d.Target.Revision)) {
		t.Fatalf("raise: %d %+v %v", code, d, raw)
	}
	// Same files, same binding: raising again binds the same digest.
	if _, again, _ := rg.raise(t, `{"kind":"adopt","target":{"bead":"N-7","change":"hand-written"}}`); again.Target.Revision != d.Target.Revision {
		t.Fatalf("digest moved without an edit: %s vs %s", again.Target.Revision, d.Target.Revision)
	} else {
		d = again
	}
	writeFile(t, filepath.Join(dir, "tasks.md"), "## 1. Do\n- [x] 1.1 a\n- [ ] 1.2 b, done differently\n")
	code, got := rg.answer(t, d.ID, `{"option":"adopt"}`)
	if code != 200 || got.State != "expired" || !strings.Contains(got.Reason, "hand-written's files changed") ||
		!strings.Contains(got.Reason, short(d.Target.Revision)) || len(rg.executed()) != 0 {
		t.Fatalf("edited files: %d %+v %+v", code, got, rg.executed())
	}
}

// Finishing is bound to the epic's beads and their states: a bead added
// under the epic since the card was raised expires it.
func TestFinishIsBoundToTheEpicsBeads(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.setBead(BeadFacts{ID: "N-9", Title: "Offline chat", Status: "open", Type: "epic"})
	rg.facts.setBead(BeadFacts{ID: "N-9.1", Status: "closed", Parent: "N-9"})
	rg.facts.setBead(BeadFacts{ID: "N-9.2", Status: "closed", Parent: "N-9"})
	code, d, raw := rg.raise(t, `{"kind":"finish","target":{"bead":"N-9"}}`)
	if code != 201 || d.Target.Revision == "" || !has(d.Consequences, "Covers 2 beads under N-9") {
		t.Fatalf("raise: %d %+v %v", code, d, raw)
	}
	rg.facts.setBead(BeadFacts{ID: "N-9.3", Status: "open", Parent: "N-9"})
	code, got := rg.answer(t, d.ID, `{"option":"finish"}`)
	if code != 200 || got.State != "expired" || !strings.Contains(got.Reason, "the beads under N-9 changed") || len(rg.executed()) != 0 {
		t.Fatalf("new bead: %d %+v", code, got)
	}
	// Reopening one changes the binding as well.
	rg.facts.setBead(BeadFacts{ID: "N-9.3", Status: "closed", Parent: "N-9"})
	_, d2, _ := rg.raise(t, `{"kind":"finish","target":{"bead":"N-9"}}`)
	rg.facts.setBead(BeadFacts{ID: "N-9.1", Status: "open", Parent: "N-9"})
	if _, got := rg.answer(t, d2.ID, `{"option":"finish"}`); got.State != "expired" {
		t.Fatalf("reopened: %+v", got)
	}
}

// Signing off is bound to the check Ryder is asked to try: a changed check
// expires the card.
func TestSignoffIsBoundToTheCheck(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.setBead(BeadFacts{ID: "N-5", Title: "Drafts survive", Status: "open", Labels: []string{"signoff"},
		Metadata: map[string]any{"signoff": "Kill the app mid-draft"}})
	_, d, _ := rg.raise(t, `{"kind":"signoff","target":{"bead":"N-5"}}`)
	rg.facts.setBead(BeadFacts{ID: "N-5", Title: "Drafts survive", Status: "open", Labels: []string{"signoff"},
		Metadata: map[string]any{"signoff": "Kill the app mid-draft, then rotate it"}})
	code, got := rg.answer(t, d.ID, `{"option":"signoff"}`)
	if code != 200 || got.State != "expired" || !strings.Contains(got.Reason, "N-5's check changed") || len(rg.executed()) != 0 {
		t.Fatalf("changed check: %d %+v", code, got)
	}
}

// ---- expiry seen when listed ----

// A decision whose revision moved expires when the queue is listed, not
// only when someone answers it, and the expiry reaches its chat.
func TestListingExpiresAnObsoleteRevision(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.setChange("N-1", awaitingChange("rev4aaaaaaa"))
	_, d, _ := rg.raise(t, `{"kind":"approve","target":{"bead":"N-1"}}`)
	// Deferred cards are revalidated too.
	if code, _ := rg.answer(t, d.ID, `{"defer":true}`); code != 200 {
		t.Fatalf("defer %d", code)
	}
	if open := rg.queueList(t, "/v1/decisions"); len(open) != 1 || open[0].State != "deferred" {
		t.Fatalf("before %+v", open)
	}
	rg.facts.setChange("N-1", awaitingChange("rev5bbbbbbb"))
	// Within the revalidation interval nothing is read again.
	if open := rg.queueList(t, "/v1/decisions"); len(open) != 1 {
		t.Fatalf("checked again too soon: %+v", open)
	}
	old := revalidateEvery
	revalidateEvery = 0
	defer func() { revalidateEvery = old }()
	if open := rg.queueList(t, "/v1/decisions"); len(open) != 0 {
		t.Fatalf("an obsolete revision stays open: %+v", open)
	}
	got := rg.get(t, d.ID)
	if got.State != "expired" || !strings.Contains(got.Reason, "revision rev4aaa") || !strings.Contains(got.Reason, "revision rev5bbb") {
		t.Fatalf("expired %+v", got)
	}
	if !chatSawDecision(t, rg, d.ID, "expired") {
		t.Fatal("the chat didn't hear about the expiry")
	}
	if len(rg.executed()) != 0 {
		t.Fatal("expiring ran something")
	}
}

// The minute sweep expires one too, and a read that fails leaves it alone.
func TestSweepExpiresAndAFailedReadDoesnt(t *testing.T) {
	rg := newDecisionRig(t)
	old := revalidateEvery
	revalidateEvery = 0
	defer func() { revalidateEvery = old }()
	rg.facts.setRound(landRound())
	_, d, _ := rg.raise(t, `{"kind":"land","target":{"round":"rdto3k"}}`)
	r := landRound()
	r.FinalCommit = "ffff0000ee"
	rg.facts.setRound(r)
	rg.s.revalidateOpen(context.Background())
	if got := rg.get(t, d.ID); got.State != "expired" || !strings.Contains(got.Reason, "commit ffff000") {
		t.Fatalf("sweep: %+v", got)
	}

	rg.facts.setBead(BeadFacts{ID: "N-5", Status: "open", Labels: []string{"signoff"}})
	_, so, _ := rg.raise(t, `{"kind":"signoff","target":{"bead":"N-5"}}`)
	flaky := &flakyFacts{fakeFacts: rg.facts}
	rg.s.Facts = flaky
	rg.s.revalidateOpen(context.Background())
	if got := rg.get(t, so.ID); got.State != "pending" {
		t.Fatalf("a failed read expired it: %+v", got)
	}
}

// flakyFacts fails every bead read the way a killed bd does.
type flakyFacts struct{ *fakeFacts }

func (f *flakyFacts) Bead(context.Context, string, string) (BeadFacts, error) {
	return BeadFacts{}, &os.PathError{Op: "exec", Path: "bd", Err: os.ErrDeadlineExceeded}
}

func chatSawDecision(t *testing.T, rg *decisionRig, id, state string) bool {
	t.Helper()
	_, events, err := rg.s.Chats.Get(rg.chatID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		var d decisions.Decision
		if ev.Kind == "decision" && json.Unmarshal(ev.Decision, &d) == nil && d.ID == id && d.State == state {
			return true
		}
	}
	return false
}

// ---- intake ----

func intakeVerdict(verdict, change string) specflow.IntakeVerdict {
	return specflow.IntakeVerdict{Verdict: verdict, ChangeName: change, CostUSD: 0.04,
		Rationale:     "Drafts are part of resuming chats offline.",
		References:    []specflow.IntakeReference{{Path: "openspec/specs/agent-chat/spec.md", Requirement: "Drafts survive"}},
		ActiveChanges: []string{"offline-chat-resume", "palette-picker"}}
}

// replies answers executed actions by path: a created issue, a develop.
func (rg *decisionRig) replies(byPath map[string]func(execCall) (int, string)) {
	rg.mu.Lock()
	defer rg.mu.Unlock()
	rg.reply = func(c execCall) (int, string) {
		for suffix, f := range byPath {
			if strings.HasSuffix(c.path, suffix) || c.method == suffix {
				return f(c)
			}
		}
		return 200, `{}`
	}
}

func optionByID(d decisions.Decision, id string) *decisions.Option {
	for i := range d.Options {
		if d.Options[i].ID == id {
			return &d.Options[i]
		}
	}
	return nil
}

// A chat raises intake for a draft: nacred runs intake once and builds the
// card with every route, the verdict's first; nothing is created or
// developed until Ryder answers.
func TestIntakeDecisionOffersEveryRoute(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.verdict = intakeVerdict(specflow.VerdictAmend, "offline-chat-resume")
	rg.facts.summaries = []api.ChangeSummary{
		{ID: "chg1", ChangeName: "offline-chat-resume", Active: "Round rdx is running"},
		{ID: "chg2", ChangeName: "palette-picker"},
	}
	code, d, raw := rg.raise(t, `{"kind":"intake","draft":{"title":"Keep drafts per chat","description":"Each thread keeps its own draft.","type":"feature"}}`)
	if code != 201 {
		t.Fatalf("raise: %d %v", code, raw)
	}
	if d.Kind != "intake" || d.ChatID == nil || *d.ChatID != rg.chatID || d.Body != "Drafts are part of resuming chats offline." ||
		!has(d.Consequences, "Intake says: Amends offline-chat-resume") || !has(d.Consequences, "Intake cost $0.04") {
		t.Fatalf("decision %+v", d)
	}
	if rg.facts.intakes != 1 || len(rg.executed()) != 0 {
		t.Fatalf("intake calls %d, executed %+v", rg.facts.intakes, rg.executed())
	}
	// The amendment's develop option is disabled naming the round; creating
	// the bead to develop later is offered (and recommended) instead.
	if d.Options[0].ID != "later" || !d.Options[0].Recommended {
		t.Fatalf("first option %+v", d.Options[0])
	}
	amend := optionByID(d, "amend:offline-chat-resume")
	if amend == nil || !strings.Contains(amend.Disabled, "Round rdx is running") || amend.Recommended {
		t.Fatalf("active amendment %+v", amend)
	}
	other := optionByID(d, "amend:palette-picker")
	newOpt := optionByID(d, "new")
	if other == nil || other.Disabled != "" || !strings.Contains(other.Detail, "Spends up to $30 (the per-action limit)") ||
		newOpt == nil || !strings.Contains(newOpt.Detail, "$30") || optionByID(d, "bead") == nil || optionByID(d, "covered") == nil {
		t.Fatalf("options %+v", d.Options)
	}
	if code, _ := rg.answer(t, d.ID, `{"option":"amend:offline-chat-resume"}`); code != http.StatusConflict {
		t.Fatalf("disabled amendment chosen: %d", code)
	}
	var spec bool
	for _, e := range d.Evidence {
		spec = spec || (e.Kind == "spec" && e.Ref == "openspec/specs/agent-chat/spec.md" && e.Label == "Drafts survive")
	}
	if !spec || d.Evidence[0].Kind != "review" || d.Evidence[0].Ref != "chg1" {
		t.Fatalf("evidence %+v", d.Evidence)
	}

	// Develop later creates the bead only, noting the amendment.
	rg.replies(map[string]func(execCall) (int, string){"/issues": func(execCall) (int, string) { return 201, `{"issue":{"id":"N-30","title":"x"}}` }})
	code, got := rg.answer(t, d.ID, `{"option":"later"}`)
	calls := rg.executed()
	if code != 200 || got.State != "done" || len(calls) != 1 || calls[0].body["title"] != "Keep drafts per chat" ||
		calls[0].body["issue_type"] != "feature" || !strings.Contains(calls[0].body["description"].(string), "amends offline-chat-resume") ||
		!strings.Contains(got.Result.Message, "Created N-30") {
		t.Fatalf("later: %d %+v %+v", code, got, calls)
	}
}

// New change creates the bead, then develops it with the verdict as
// feedback; the bead and its change belong to the chat from then on.
func TestIntakeNewChangeCreatesThenDevelops(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.verdict = intakeVerdict(specflow.VerdictNew, "")
	_, d, _ := rg.raise(t, `{"kind":"intake","draft":{"title":"Export chats"}}`)
	if d.Options[0].ID != "new" || !d.Options[0].Recommended {
		t.Fatalf("options %+v", d.Options)
	}
	rg.replies(map[string]func(execCall) (int, string){
		"/change": func(execCall) (int, string) { return 200, `{"change":{"id":"chg9","seed_id":"N-31"}}` },
		"/issues": func(execCall) (int, string) { return 201, `{"issue":{"id":"N-31","title":"x"}}` },
	})
	code, got := rg.answer(t, d.ID, `{"option":"new"}`)
	calls := rg.executed()
	if code != 200 || got.State != "done" || len(calls) != 2 || calls[1].path != "/v1/repos/"+rg.repo+"/issues/N-31/change" {
		t.Fatalf("new: %d %+v %+v", code, got, calls)
	}
	dev := calls[1].body
	hint, _ := dev["intake"].(map[string]any)
	if dev["action"] != "develop" || !strings.Contains(dev["feedback"].(string), "Drafts are part of resuming chats offline.") ||
		!strings.Contains(dev["feedback"].(string), "openspec/specs/agent-chat/spec.md") || hint["verdict"] != "new" {
		t.Fatalf("develop body %+v", dev)
	}
	if got.Result.Message != "Created N-31; developing it as a new change" {
		t.Fatalf("result %+v", got.Result)
	}
	if rg.s.Decisions.ChatOf(beadLink(rg.repo, "N-31")) != rg.chatID || rg.s.Decisions.ChatOf(changeLink(rg.repo, "chg9")) != rg.chatID {
		t.Fatal("the created bead and change aren't the chat's")
	}
}

// Development refused: the bead stays, and the card says why development
// didn't start. Just a bead and Already covered spend nothing more.
func TestIntakeRefusedDevelopmentKeepsTheBead(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.verdict = intakeVerdict(specflow.VerdictNew, "")
	rg.replies(map[string]func(execCall) (int, string){
		"/change": func(execCall) (int, string) {
			return 409, `{"error":"another proposal is being developed in this repository"}`
		},
		"/issues": func(execCall) (int, string) { return 201, `{"issue":{"id":"N-32","title":"x"}}` },
	})
	_, d, _ := rg.raise(t, `{"kind":"intake","draft":{"title":"Export chats"}}`)
	code, got := rg.answer(t, d.ID, `{"option":"amend:palette-picker"}`)
	if code != 200 || got.State != "failed" || len(rg.executed()) != 2 ||
		got.Result.Message != "Created N-32, but development didn't start: another proposal is being developed in this repository" {
		t.Fatalf("refused: %d %+v", code, got)
	}
	if hint := rg.executed()[1].body["intake"].(map[string]any); hint["verdict"] != "amend" || hint["change_name"] != "palette-picker" {
		t.Fatalf("amend hint %+v", hint)
	}

	_, b, _ := rg.raise(t, `{"kind":"intake","draft":{"title":"Rename a label"}}`)
	before := len(rg.executed())
	if code, got := rg.answer(t, b.ID, `{"option":"bead"}`); code != 200 || got.State != "done" || len(rg.executed()) != before+1 ||
		got.Result.Message != "Created N-32" {
		t.Fatalf("just a bead: %d %+v", code, got)
	}
	_, c, _ := rg.raise(t, `{"kind":"intake","draft":{"title":"Already there"}}`)
	before = len(rg.executed())
	if code, got := rg.answer(t, c.ID, `{"option":"covered"}`); code != 200 || got.State != "done" || len(rg.executed()) != before {
		t.Fatalf("covered: %d %+v", code, got)
	}
}

// An intake for an existing bead develops that bead and creates nothing;
// bad drafts and an unavailable intake are refused.
func TestIntakeForAnExistingBeadAndRefusals(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.verdict = intakeVerdict(specflow.VerdictNew, "")
	rg.facts.setBead(BeadFacts{ID: "N-40", Title: "Out of scope work", Status: "open"})
	_, d, _ := rg.raise(t, `{"kind":"intake","target":{"bead":"N-40"},"draft":{"title":"Export chats"}}`)
	if code, got := rg.answer(t, d.ID, `{"option":"new"}`); code != 200 || got.State != "done" || len(rg.executed()) != 1 ||
		rg.executed()[0].path != "/v1/repos/"+rg.repo+"/issues/N-40/change" || got.Result.Message != "Developing N-40 as a new change" {
		t.Fatalf("existing: %d %+v %+v", code, got, rg.executed())
	}
	if code, _, _ := rg.raise(t, `{"kind":"intake"}`); code != http.StatusBadRequest {
		t.Fatalf("no draft: %d", code)
	}
	if code, _, _ := rg.raise(t, `{"kind":"intake","draft":{"title":"x","type":"saga"}}`); code != http.StatusBadRequest {
		t.Fatalf("bad type: %d", code)
	}
	rg.facts.intakeErr = specflow.ErrUnavailable
	if code, _, raw := rg.raise(t, `{"kind":"intake","draft":{"title":"x"}}`); code != http.StatusServiceUnavailable {
		t.Fatalf("unavailable: %d %v", code, raw)
	}
}

// ---- found work ----

func foundSetup(rg *decisionRig) {
	rg.facts.summaries = []api.ChangeSummary{{ID: "chg1", ChangeName: "offline-chat-resume", EpicID: "N-9", SeedIDs: []string{"N-1"},
		Sections: []api.SectionSummary{{ID: "s1", Title: "Store drafts", BeadID: "N-2", Evidence: api.EvidenceLanded}, {ID: "s2", Title: "Reconnect", BeadID: "N-3"}}}}
	rg.facts.setRound(api.Round{ID: "rdf", RepoID: rg.repo, State: api.RoundImplementing,
		Beads: []api.RoundBead{{ID: "N-2", Title: "Store drafts", SpecChangeID: "chg1"}, {ID: "N-3", Title: "Reconnect", SpecChangeID: "chg1"}}})
}

// Found work told to a chat during a round raises a found-work card that
// asks which round bead it was found in, defaulting to the one given, and
// offers the three answers.
func TestFoundWorkFromARound(t *testing.T) {
	rg := newDecisionRig(t)
	foundSetup(rg)
	code, d, raw := rg.raise(t, `{"kind":"found","target":{"round":"rdf","bead":"N-3"},"note":"Migrate the drafts table first"}`)
	if code != 201 {
		t.Fatalf("raise: %d %v", code, raw)
	}
	if d.Kind != "found" || d.Ask == nil || d.Ask.Default != "N-3" || len(d.Ask.Choices) != 2 || !d.Ask.Required ||
		d.Target.Change != "offline-chat-resume" || d.Body != "Migrate the drafts table first" {
		t.Fatalf("decision %+v ask %+v", d, d.Ask)
	}
	var ids []string
	for _, o := range d.Options {
		ids = append(ids, o.ID)
	}
	how := optionByID(d, "how_order")
	if strings.Join(ids, ",") != "how_order,amend,out_of_scope" || how.Input == nil || len(how.Input.Choices) != 1 || how.Input.Choices[0].ID != "s2" {
		t.Fatalf("options %v %+v", ids, how.Input)
	}
	// Only how or in what order, before section 2, found in the default bead.
	rg.replies(map[string]func(execCall) (int, string){"/found": func(execCall) (int, string) { return 200, `{"bead_id":"N-20"}` }})
	code, got := rg.answer(t, d.ID, `{"option":"how_order","input":"s2"}`)
	calls := rg.executed()
	if code != 200 || got.State != "done" || len(calls) != 1 || calls[0].path != "/v1/repos/"+rg.repo+"/changes/chg1/found" ||
		calls[0].body["answer"] != "how_order" || calls[0].body["found_in"] != "N-3" || calls[0].body["before_section"] != "s2" ||
		calls[0].body["note"] != "Migrate the drafts table first" || got.Result.Message != "Filed N-20 under N-9; the approved plan stays as it is" {
		t.Fatalf("how/order: %d %+v %+v", code, got, calls)
	}
	// Changes what done means, found in another round bead.
	_, d2, _ := rg.raise(t, `{"kind":"found","target":{"round":"rdf","bead":"N-3"},"note":"Drafts must sync across devices"}`)
	if code, _ := rg.answer(t, d2.ID, `{"option":"amend","ask":"N-99"}`); code != http.StatusBadRequest {
		t.Fatalf("a bead outside the round: %d", code)
	}
	if code, got := rg.answer(t, d2.ID, `{"option":"amend","ask":"N-2"}`); code != 200 || got.State != "done" || got.Answer.Ask != "N-2" {
		t.Fatalf("amend: %d %+v", code, got)
	}
	if last := rg.executed()[len(rg.executed())-1]; last.body["answer"] != "amend" || last.body["found_in"] != "N-2" {
		t.Fatalf("amend call %+v", last)
	}
}

// Out of scope files a bead related to the epic (not under it) and starts
// a new effort for it in the chat, prefilled with the bead.
func TestFoundOutOfScopeStartsANewEffortInTheChat(t *testing.T) {
	rg := newDecisionRig(t)
	foundSetup(rg)
	_, d, _ := rg.raise(t, `{"kind":"found","target":{"bead":"N-3"},"note":"Chats need an export"}`)
	if d.Ask != nil {
		t.Fatalf("asked outside a round: %+v", d.Ask)
	}
	rg.replies(map[string]func(execCall) (int, string){
		"/found": func(execCall) (int, string) { return 200, `{"bead_id":"N-21"}` },
		decisions.MethodEffort: func(execCall) (int, string) {
			return 200, `{"chat_id":"x","message":"a new effort for it started in its chat"}`
		},
	})
	code, got := rg.answer(t, d.ID, `{"option":"out_of_scope"}`)
	calls := rg.executed()
	if code != 200 || got.State != "done" || len(calls) != 2 {
		t.Fatalf("out of scope: %d %+v %+v", code, got, calls)
	}
	if calls[0].body["answer"] != "out_of_scope" || calls[0].body["create"] != true || calls[0].body["found_in"] != "N-3" {
		t.Fatalf("found call %+v", calls[0])
	}
	text, _ := calls[1].body["text"].(string)
	if calls[1].method != decisions.MethodEffort || calls[1].body["chat"] != rg.chatID || !strings.Contains(text, "Chats need an export") ||
		!strings.Contains(text, "filed as N-21") || !strings.Contains(text, "raise intake for N-21") {
		t.Fatalf("effort %+v", calls[1])
	}
	if got.Result.Message != "Filed N-21, related to N-9 and not under it; a new effort for it started in its chat" {
		t.Fatalf("result %+v", got.Result)
	}
	// Work outside any spec epic is no found work.
	rg.facts.setBead(BeadFacts{ID: "N-50", Status: "open"})
	if code, _, _ := rg.raise(t, `{"kind":"found","target":{"bead":"N-50"},"note":"x"}`); code != http.StatusConflict {
		t.Fatalf("not spec work: %d", code)
	}
	// A bead under the epic finds its change through its parent.
	rg.facts.setBead(BeadFacts{ID: "N-9.7", Status: "open", Parent: "N-9"})
	if code, d, raw := rg.raise(t, `{"kind":"found","target":{"bead":"N-9.7"},"note":"x"}`); code != 201 || d.Target.Change != "offline-chat-resume" {
		t.Fatalf("under the epic: %d %+v %v", code, d, raw)
	}
}

// A Found: comment's card answers Out of scope the same way: through the
// found route with a bead created now, then a new effort in the chat the
// work came from.
func TestFoundCommentCardMatchesTheChatsCard(t *testing.T) {
	rg := newDecisionRig(t)
	foundSetup(rg)
	rg.s.Decisions.LinkWork("rdf", rg.chatID)
	it := api.AttentionItem{ID: "fw1", Kind: api.AttentionFoundWork, Repo: rg.repo, RepoName: "repo", BeadID: "N-3", ChangeID: "chg1",
		Options: []api.AttentionOption{
			{Type: "open", Label: "Answer", Target: &api.AttentionTarget{Kind: "found_something", Repo: rg.repo, Change: "chg1", FoundIn: "N-3", CommentID: "c7", Note: "Chats need an export"}},
			{Type: "action", Label: "Dismiss", Method: "POST", Path: "/v1/repos/" + rg.repo + "/changes/chg1/found", Body: map[string]any{"answer": "dismiss"}},
		}}
	recs := rg.s.fromAttention(context.Background(), rg.ref(t), it, rg.s.itemChats()(it))
	rec := recs[0]
	if rec.Decision.ChatID == nil || *rec.Decision.ChatID != rg.chatID {
		t.Fatalf("chat %+v", rec.Decision.ChatID)
	}
	oos := rec.Actions["out_of_scope"]
	if !strings.HasSuffix(oos.Path, "/changes/chg1/found") || oos.Body["create"] != true || oos.Body["comment_id"] != "c7" ||
		oos.Then == nil || oos.Then.Method != decisions.MethodEffort || oos.Then.Body["chat"] != rg.chatID {
		t.Fatalf("out of scope %+v then %+v", oos, oos.Then)
	}
	if _, ok := rec.Actions["dismiss"]; !ok || optionByID(rec.Decision, "how_order").Input.Choices[0].ID != "s2" {
		t.Fatalf("options %+v", rec.Decision.Options)
	}
}

// startEffort sends the effort to its chat, or to a new chat in the
// repository when that chat is gone.
func TestStartEffortFallsBackToANewChat(t *testing.T) {
	rg := newDecisionRig(t)
	status, body := rg.s.startEffort(decisions.Action{Method: decisions.MethodEffort,
		Body: map[string]any{"chat": rg.chatID, "repo": rg.repo, "text": "New effort: export chats"}})
	var out struct{ ChatID, Message string }
	_ = json.Unmarshal(body, &struct {
		ChatID  *string `json:"chat_id"`
		Message *string `json:"message"`
	}{&out.ChatID, &out.Message})
	if status != 200 || out.ChatID != rg.chatID || out.Message != "a new effort for it started in its chat" {
		t.Fatalf("own chat: %d %s", status, body)
	}
	waitChatDone(t, rg.s, rg.chatID)
	_, events, _ := rg.s.Chats.Get(rg.chatID, 0)
	sent := false
	for _, ev := range events {
		sent = sent || (ev.Kind == chat.KindUser && ev.Text == "New effort: export chats")
	}
	if !sent {
		t.Fatal("the effort wasn't sent to the chat")
	}
	status, body = rg.s.startEffort(decisions.Action{Method: decisions.MethodEffort,
		Body: map[string]any{"chat": "ch_gone", "repo": rg.repo, "text": "New effort: export chats"}})
	_ = json.Unmarshal(body, &struct {
		ChatID  *string `json:"chat_id"`
		Message *string `json:"message"`
	}{&out.ChatID, &out.Message})
	if status != 200 || out.ChatID == rg.chatID || out.ChatID == "" || out.Message != "a new effort for it started in a new chat" {
		t.Fatalf("new chat: %d %s", status, body)
	}
	waitChatDone(t, rg.s, out.ChatID)
	if rec, _, err := rg.s.Chats.Get(out.ChatID, 0); err != nil || rec.Repo == nil || *rec.Repo != rg.repo {
		t.Fatalf("new chat %+v %v", rec, err)
	}
}

// ---- chat association ----

// Needs you decisions name the chat that started their work, keeping their
// stable ids: a round's questions and landing by the round, a bead by its
// link or the round that carried it.
func TestNeedsYouDecisionsNameTheirChat(t *testing.T) {
	rg, _ := attentionRig(t)
	before := rg.queueList(t, "/v1/decisions")
	for _, d := range before {
		if d.ChatID != nil {
			t.Fatalf("no links yet, but %s names a chat", d.ID)
		}
	}
	rg.s.Decisions.LinkWork("rdask", rg.chatID)
	rg.s.Decisions.LinkWork(beadLink(rg.repo, "N-8"), rg.chatID)
	after := rg.queueList(t, "/v1/decisions")
	if len(after) != len(before) {
		t.Fatalf("queue changed size: %d vs %d", len(after), len(before))
	}
	ids := map[string]bool{}
	for _, d := range before {
		ids[d.ID] = true
	}
	for _, d := range after {
		if !ids[d.ID] {
			t.Fatalf("id changed: %s", d.ID)
		}
		inChat := d.ChatID != nil && *d.ChatID == rg.chatID
		switch {
		case d.Kind == "question" && d.Target.Round == "rdask", d.Kind == "question" && d.Target.Bead == "N-8":
			if !inChat {
				t.Errorf("%s (%s) isn't in the chat", d.ID, d.Title)
			}
		case d.Kind == "land":
			if d.ChatID != nil {
				t.Errorf("an unlinked round's land names a chat: %+v", d)
			}
		}
	}
	// A bead the linked round carried belongs to its chat as well.
	rg.s.Decisions.LinkWork("rdto3k", rg.chatID)
	for _, d := range byKind(rg.queueList(t, "/v1/decisions"))["land"] {
		if d.ChatID == nil || *d.ChatID != rg.chatID {
			t.Fatalf("land %+v", d)
		}
	}
	if got := rg.s.itemChats()(api.AttentionItem{Repo: rg.repo, BeadID: "N-3"}); got == nil || *got != rg.chatID {
		t.Fatalf("a bead of the round: %v", got)
	}
}

// A chat that develops a change is the chat of that change's decisions.
func TestDevelopingLinksTheChangeToTheChat(t *testing.T) {
	rg := newDecisionRig(t)
	if got := rg.s.itemChats()(api.AttentionItem{Repo: rg.repo, ChangeID: "chg1"}); got != nil {
		t.Fatalf("unlinked: %v", *got)
	}
	rg.s.linkCreated(decisions.Record{Decision: decisions.Decision{Repo: rg.repo, ChatID: &rg.chatID}}, "", []byte(`{"change":{"id":"chg1","seed_id":"N-1"}}`))
	if got := rg.s.itemChats()(api.AttentionItem{Repo: rg.repo, ChangeID: "chg1"}); got == nil || *got != rg.chatID {
		t.Fatalf("by change: %v", got)
	}
	if got := rg.s.itemChats()(api.AttentionItem{Repo: rg.repo, BeadID: "N-1"}); got == nil || *got != rg.chatID {
		t.Fatalf("by seed: %v", got)
	}
}
