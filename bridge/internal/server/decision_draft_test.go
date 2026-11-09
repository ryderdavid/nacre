package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/chat"
	"github.com/ryderdavid/nacre/bridge/internal/codex"
	"github.com/ryderdavid/nacre/bridge/internal/specflow"
)

// draftTurn prints one assistant message holding a nacre-draft block with a
// digest, as a New effort chat's agent writes it.
const draftTurn = `printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"Changed: the title.\n\n` +
	"```nacre-draft\\n" + `{\"title\":\"Keep drafts per chat\",\"type\":\"feature\",\"description\":\"Each thread keeps its own draft.\",\"digest\":[\"Decided: per chat\",\"Ruled out: a new tab\"]}` + "\\n```" +
	`"}]}}'`

// draftedChat is a New effort chat in the rig's repository whose one turn
// wrote a draft; it returns the chat id and the draft event's seq.
func draftedChat(t *testing.T, rg *decisionRig) (string, int64) {
	t.Helper()
	repo := rg.repo
	rec, err := rg.s.Chats.Create(chat.CreateRequest{Agent: chat.AgentClaude, Repo: &repo, NewEffort: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rg.s.Chats.Send(rec.ID, chat.SendRequest{TurnID: "c_draft0001", Text: "drafts per chat"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 400; i++ {
		got, evs, _ := rg.s.Chats.Get(rec.ID, 0)
		if got.State == chat.StateIdle && len(evs) > 1 {
			for _, ev := range evs {
				if ev.Kind == chat.KindDraft {
					return rec.ID, ev.Seq
				}
			}
			t.Fatalf("no draft event: %+v", evs)
		}
		sleepBriefly()
	}
	t.Fatal("the drafting turn never finished")
	return "", 0
}

func TestIntakeFromTheRecordedDraft(t *testing.T) {
	rg := newDecisionRigWith(t, draftTurn)
	rg.facts.verdict = intakeVerdict(specflow.VerdictNew, "")
	id, seq := draftedChat(t, rg)
	if got, _, _ := rg.s.Chats.Get(id, 0); !got.Drafting {
		t.Fatal("the New effort chat isn't drafting")
	}

	path := "/v1/chats/" + id + "/decisions"
	body := `{"kind":"intake","draft_seq":` + seqText(seq) + `}`
	code, raw := rg.do(t, "POST", path, body)
	if code != http.StatusCreated {
		t.Fatalf("raise from the draft: %d %v", code, raw)
	}
	if raw["title"] != "Where does “Keep drafts per chat” belong?" || raw["kind"] != "intake" {
		t.Fatalf("decision %v", raw)
	}
	decID, _ := raw["id"].(string)

	// A second tap on the same draft returns the same card.
	code, again := rg.do(t, "POST", path, body)
	if code != http.StatusOK || again["id"] != decID || rg.facts.intakes != 1 {
		t.Fatalf("repeat raise: %d %v (intakes %d)", code, again, rg.facts.intakes)
	}

	// New change with a note: the bead keeps the digest in its notes, and
	// the note reaches both the bead and the develop request.
	code, d := rg.answer(t, decID, `{"option":"new","note":"keep it to the folded screen"}`)
	if code/100 != 2 {
		t.Fatalf("answer: %d %+v", code, d)
	}
	calls := rg.executed()
	if len(calls) != 2 || !strings.HasSuffix(calls[0].path, "/issues") || !strings.HasSuffix(calls[1].path, "/change") {
		t.Fatalf("executed %+v", calls)
	}
	create, develop := calls[0].body, calls[1].body
	if create["notes"] != "- Decided: per chat\n- Ruled out: a new tab" || create["owner_note"] != "keep it to the folded screen" {
		t.Errorf("create body %v", create)
	}
	if develop["owner_note"] != "keep it to the folded screen" || !strings.HasPrefix(develop["feedback"].(string), "Intake verdict: New change.") {
		t.Errorf("develop body %v", develop)
	}
	if got, _, _ := rg.s.Chats.Get(id, 0); got.Drafting {
		t.Error("still drafting after the intake card created and developed")
	}
}

func TestIntakeFromAGoneDraft(t *testing.T) {
	rg := newDecisionRig(t)
	code, raw := rg.do(t, "POST", "/v1/chats/"+rg.chatID+"/decisions", `{"kind":"intake","draft_seq":9999}`)
	if code != http.StatusGone || raw["detail"] != "draft_gone" || !strings.Contains(raw["error"].(string), "That draft is gone") {
		t.Fatalf("gone draft: %d %v", code, raw)
	}
	if rg.facts.intakes != 0 {
		t.Errorf("intake ran for a gone draft")
	}
}

func TestDroppingIntakeKeepsDrafting(t *testing.T) {
	rg := newDecisionRigWith(t, draftTurn)
	rg.facts.verdict = intakeVerdict(specflow.VerdictNew, "")
	id, seq := draftedChat(t, rg)
	_, raw := rg.do(t, "POST", "/v1/chats/"+id+"/decisions", `{"kind":"intake","draft_seq":`+seqText(seq)+`}`)
	decID, _ := raw["id"].(string)
	if code, _ := rg.answer(t, decID, `{"reject":true,"note":"not yet"}`); code/100 != 2 {
		t.Fatalf("drop: %d", code)
	}
	if got, _, _ := rg.s.Chats.Get(id, 0); !got.Drafting {
		t.Error("dropping the intake card ended drafting")
	}
	_, raw = rg.do(t, "POST", "/v1/chats/"+id+"/decisions", `{"kind":"intake","draft_seq":`+seqText(seq)+`}`)
	decID, _ = raw["id"].(string)
	if code, _ := rg.answer(t, decID, `{"option":"covered"}`); code/100 != 2 {
		t.Fatalf("covered: %d", code)
	}
	if got, _, _ := rg.s.Chats.Get(id, 0); !got.Drafting {
		t.Error("Already covered ended drafting")
	}
}

func TestIntakeFailureStillOffersAWayForward(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.intakeErr = errors.New("intake took longer than 2m0s")
	code, d, raw := rg.raise(t, `{"kind":"intake","draft":{"title":"Keep drafts per chat","type":"feature"}}`)
	if code != http.StatusCreated {
		t.Fatalf("raise: %d %v", code, raw)
	}
	var ids []string
	for _, o := range d.Options {
		ids = append(ids, o.ID)
		if o.Recommended {
			t.Errorf("option %q recommended although intake failed", o.ID)
		}
	}
	if strings.Join(ids, ",") != "new,bead" {
		t.Errorf("options %v, want New change and Just a bead", ids)
	}
	if d.Body != "Intake couldn't decide where this goes: it took too long. Pick one yourself." {
		t.Errorf("body %q", d.Body)
	}
	for _, c := range d.Consequences {
		if strings.HasPrefix(c, "Suggested:") {
			t.Errorf("a suggestion although intake failed: %v", d.Consequences)
		}
	}
}

func TestIntakeOnAnExistingBeadCarriesTheDigest(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.verdict = intakeVerdict(specflow.VerdictNew, "")
	rg.facts.beads["N-4"] = BeadFacts{ID: "N-4", Status: "open"}
	code, d, raw := rg.raise(t, `{"kind":"intake","target":{"bead":"N-4"},"draft":{"title":"Keep drafts","digest":"Decided: per chat"}}`)
	if code != http.StatusCreated {
		t.Fatalf("raise: %d %v", code, raw)
	}
	if c, _ := rg.answer(t, d.ID, `{"option":"new"}`); c/100 != 2 {
		t.Fatalf("answer: %d", c)
	}
	calls := rg.executed()
	if len(calls) != 1 || !strings.HasPrefix(calls[0].body["feedback"].(string), "From the brainstorm:\nDecided: per chat\n\nIntake verdict:") {
		t.Fatalf("executed %+v", calls)
	}
}

func TestHealthListsDraftIntake(t *testing.T) {
	bin, _ := chatClaude(t, "")
	s, _ := chatServer(t, bin)
	for _, f := range s.features(codex.Status{}, s.chatSupport(context.Background()), "") {
		if f == "draft_intake" {
			return
		}
	}
	t.Fatal("draft_intake missing where Chat runs")
}

func seqText(n int64) string { return strconv.FormatInt(n, 10) }

func sleepBriefly() { time.Sleep(25 * time.Millisecond) }

func TestIntakeWhileItRunsOnTheSameDraft(t *testing.T) {
	rg := newDecisionRigWith(t, draftTurn)
	rg.facts.verdict = intakeVerdict(specflow.VerdictNew, "")
	id, seq := draftedChat(t, rg)
	// A first tap's intake call is still running.
	rg.s.draftMu.Lock()
	rg.s.draftRaising = map[string]bool{id + ":" + seqText(seq): true}
	rg.s.draftMu.Unlock()
	code, raw := rg.do(t, "POST", "/v1/chats/"+id+"/decisions", `{"kind":"intake","draft_seq":`+seqText(seq)+`}`)
	if code != http.StatusConflict || raw["detail"] != "intake_running" || rg.facts.intakes != 0 {
		t.Fatalf("second tap while running: %d %v (intakes %d)", code, raw, rg.facts.intakes)
	}
}

// draftTurnWithBead is a drafting reply for found work already filed as N-4.
const draftTurnWithBead = `printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"` +
	"```nacre-draft\\n" + `{\"title\":\"Migrate the store\",\"bead\":\"N-4\"}` + "\\n```" + `"}]}}'`

func TestIntakeFromADraftThatNamesItsBead(t *testing.T) {
	rg := newDecisionRigWith(t, draftTurnWithBead)
	rg.facts.verdict = intakeVerdict(specflow.VerdictNew, "")
	rg.facts.beads["N-4"] = BeadFacts{ID: "N-4", Status: "open"}
	id, seq := draftedChat(t, rg)
	code, raw := rg.do(t, "POST", "/v1/chats/"+id+"/decisions", `{"kind":"intake","draft_seq":`+seqText(seq)+`}`)
	if code != http.StatusCreated {
		t.Fatalf("raise: %d %v", code, raw)
	}
	decID, _ := raw["id"].(string)
	if c, _ := rg.answer(t, decID, `{"option":"new"}`); c/100 != 2 {
		t.Fatalf("answer: %d", c)
	}
	calls := rg.executed()
	if len(calls) != 1 || !strings.Contains(calls[0].path, "/issues/N-4/change") {
		t.Fatalf("a second bead was created instead of developing N-4: %+v", calls)
	}
}

func TestGoAheadRaisesThroughTheServer(t *testing.T) {
	rg := newDecisionRigWith(t, draftTurn)
	rg.facts.verdict = intakeVerdict(specflow.VerdictNew, "")
	rg.s.UseDecisions(rg.s.Decisions)
	id, seq := draftedChat(t, rg)
	if err := rg.s.raiseFromChat(id, seq); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, r := range rg.s.Decisions.List() {
		found = found || (r.DraftSeq == seq && r.Decision.ChatID != nil && *r.Decision.ChatID == id)
	}
	if !found || rg.facts.intakes != 1 {
		t.Fatalf("no card from the go-ahead (intakes %d)", rg.facts.intakes)
	}
	// Again on the same draft: the same card, no second intake.
	if err := rg.s.raiseFromChat(id, seq); err != nil || rg.facts.intakes != 1 {
		t.Fatalf("second go-ahead: %v (intakes %d)", err, rg.facts.intakes)
	}
}

func TestDirectIntakeDraftDigestIsBounded(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.verdict = intakeVerdict(specflow.VerdictNew, "")
	body := `{"kind":"intake","draft":{"title":"x","digest":"` + strings.Repeat("a", chat.MaxDigest+1) + `"}}`
	code, _, raw := rg.raise(t, body)
	if code != http.StatusBadRequest || !strings.Contains(raw["error"].(string), "digest") || rg.facts.intakes != 0 {
		t.Fatalf("oversize digest: %d %v", code, raw)
	}
}

func TestAFiledDraftIsNotFiledAgain(t *testing.T) {
	rg := newDecisionRigWith(t, draftTurn)
	rg.facts.verdict = intakeVerdict(specflow.VerdictNew, "")
	id, seq := draftedChat(t, rg)
	path, body := "/v1/chats/"+id+"/decisions", `{"kind":"intake","draft_seq":`+seqText(seq)+`}`
	_, raw := rg.do(t, "POST", path, body)
	decID, _ := raw["id"].(string)
	if c, _ := rg.answer(t, decID, `{"option":"bead"}`); c/100 != 2 {
		t.Fatalf("answer: %d", c)
	}
	code, again := rg.do(t, "POST", path, body)
	if code != http.StatusOK || again["id"] != decID || again["state"] != "done" || rg.facts.intakes != 1 || len(rg.executed()) != 1 {
		t.Fatalf("a filed draft raised again: %d %v (intakes %d, executed %d)", code, again, rg.facts.intakes, len(rg.executed()))
	}
}

func TestAPartlyFiledDraftIsNotFiledAgain(t *testing.T) {
	rg := newDecisionRigWith(t, draftTurn)
	rg.facts.verdict = intakeVerdict(specflow.VerdictNew, "")
	id, seq := draftedChat(t, rg)
	path, body := "/v1/chats/"+id+"/decisions", `{"kind":"intake","draft_seq":`+seqText(seq)+`}`
	_, raw := rg.do(t, "POST", path, body)
	decID, _ := raw["id"].(string)
	// The bead is created, then development is refused.
	rg.replies(map[string]func(execCall) (int, string){
		"/change": func(execCall) (int, string) { return http.StatusConflict, `{"error":"specflow is busy"}` },
	})
	rg.answer(t, decID, `{"option":"new"}`)
	if st := rg.get(t, decID).State; st != "failed" {
		t.Fatalf("state %s, want failed part way", st)
	}
	code, again := rg.do(t, "POST", path, body)
	if code != http.StatusOK || again["id"] != decID || rg.facts.intakes != 1 {
		t.Fatalf("a partly filed draft raised again: %d %v (intakes %d)", code, again, rg.facts.intakes)
	}
}

func TestEffortBeadBindsIntakeToTheFiledBead(t *testing.T) {
	rg := newDecisionRigWith(t, draftTurn)
	rg.facts.verdict = intakeVerdict(specflow.VerdictNew, "")
	rg.facts.beads["N-4"] = BeadFacts{ID: "N-4", Status: "open"}
	id, seq := draftedChat(t, rg)
	// Found work filed as N-4 started this effort in the chat.
	code, _ := rg.s.startEffort(withID(effortAction(rg.repo, id, "chg", "E-1", "migrate the store"), "N-4"))
	if code != http.StatusOK {
		t.Fatalf("startEffort: %d", code)
	}
	if rec, _, _ := rg.s.Chats.Get(id, 0); rec.EffortBead != "N-4" {
		t.Fatalf("effort bead %q", rec.EffortBead)
	}
	_, raw := rg.do(t, "POST", "/v1/chats/"+id+"/decisions", `{"kind":"intake","draft_seq":`+seqText(seq)+`}`)
	decID, _ := raw["id"].(string)
	if c, _ := rg.answer(t, decID, `{"option":"new"}`); c/100 != 2 {
		t.Fatalf("answer: %d", c)
	}
	calls := rg.executed()
	if len(calls) != 1 || !strings.Contains(calls[0].path, "/issues/N-4/change") {
		t.Fatalf("a second bead was filed: %+v", calls)
	}
	// The turn startEffort began finishes before the test's files go.
	for i := 0; i < 400; i++ {
		if rec, _, _ := rg.s.Chats.Get(id, 0); rec.State != chat.StateRunning {
			return
		}
		sleepBriefly()
	}
}
