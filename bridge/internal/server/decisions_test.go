package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/chat"
	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/decisions"
	"github.com/ryderdavid/nacre/bridge/internal/rounds"
	"github.com/ryderdavid/nacre/bridge/internal/specflow"
)

// fakeFacts is the state decisions read, set by each test.
type fakeFacts struct {
	mu        sync.Mutex
	rounds    map[string]api.Round
	changes   map[string]specflow.Envelope
	summaries []api.ChangeSummary
	beads     map[string]BeadFacts
	target    string
	push      *bool
	// verdict and intakeErr answer Intake; intakes counts its calls.
	verdict   specflow.IntakeVerdict
	intakeErr error
	intakes   int
	// heads answers Head by branch.
	heads map[string]string
}

func newFacts() *fakeFacts {
	return &fakeFacts{rounds: map[string]api.Round{}, changes: map[string]specflow.Envelope{}, beads: map[string]BeadFacts{}, target: "main"}
}

func (f *fakeFacts) Round(id string) (api.Round, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rounds[id]
	if !ok {
		return api.Round{}, rounds.ErrNotFound
	}
	return r, nil
}
func (f *fakeFacts) Change(_ context.Context, _, bead string) (specflow.Envelope, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.changes[bead], nil
}
func (f *fakeFacts) Changes(string) ([]api.ChangeSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.summaries, nil
}
func (f *fakeFacts) Bead(_ context.Context, _, id string) (BeadFacts, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.beads[id]
	if !ok {
		return BeadFacts{}, fmt.Errorf("no issue %s", id)
	}
	return b, nil
}
func (f *fakeFacts) Children(_ context.Context, _, id string) ([]BeadFacts, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []BeadFacts
	for _, b := range f.beads {
		if b.Parent == id {
			out = append(out, b)
		}
	}
	return out, nil
}
func (f *fakeFacts) RoundsOf(repo string) []api.Round {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []api.Round
	for _, r := range f.rounds {
		if repo == "" || r.RepoID == repo {
			out = append(out, r)
		}
	}
	return out
}
func (f *fakeFacts) Intake(context.Context, string, specflow.IntakeDraft) (specflow.IntakeVerdict, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.intakes++
	return f.verdict, f.intakeErr
}
func (f *fakeFacts) setBead(b BeadFacts) { f.mu.Lock(); f.beads[b.ID] = b; f.mu.Unlock() }
func (f *fakeFacts) Head(_ context.Context, _, branch string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.heads[branch]
}
func (f *fakeFacts) setHead(branch, sha string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.heads == nil {
		f.heads = map[string]string{}
	}
	f.heads[branch] = sha
}
func (f *fakeFacts) Land(string) (string, *bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.target, f.push
}
func (f *fakeFacts) setRound(r api.Round) { f.mu.Lock(); f.rounds[r.ID] = r; f.mu.Unlock() }
func (f *fakeFacts) setChange(bead string, c *specflow.Change) {
	f.mu.Lock()
	f.changes[bead] = specflow.Envelope{Available: true, Change: c}
	f.mu.Unlock()
}

// execCall is one executed action.
type execCall struct {
	method, path string
	body         map[string]any
}

// decisionRig is a server with a chat in a repository, decisions, fake facts
// and a recorded executor.
type decisionRig struct {
	s      *Server
	h      http.Handler
	facts  *fakeFacts
	repo   string
	chatID string
	mu     sync.Mutex
	calls  []execCall
	// reply answers executed actions; nil answers 200 {}.
	reply func(execCall) (int, string)
}

func newDecisionRig(t *testing.T) *decisionRig {
	t.Helper()
	bin, _ := chatClaude(t, "")
	s, repo := chatServer(t, bin)
	s.Config = &config.Config{Token: "tok", HostName: "Mac-Studio", HostID: "h_1"}
	store, err := decisions.Open(filepath.Join(t.TempDir(), "decisions"), nil, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	s.Decisions = store
	rg := &decisionRig{s: s, facts: newFacts(), repo: repo}
	s.Facts = rg.facts
	s.DecisionExec = func(_ context.Context, act decisions.Action) (int, []byte) {
		c := execCall{act.Method, act.Path, act.Body}
		rg.mu.Lock()
		rg.calls = append(rg.calls, c)
		reply := rg.reply
		rg.mu.Unlock()
		if reply != nil {
			code, body := reply(c)
			return code, []byte(body)
		}
		return 200, []byte(`{}`)
	}
	store.OnChange = s.decisionChanged
	rec, err := s.Chats.Create(chat.CreateRequest{Agent: chat.AgentClaude, Repo: &repo})
	if err != nil {
		t.Fatal(err)
	}
	rg.chatID = rec.ID
	rg.h = s.Handler()
	// Registered last so it runs first: background follows finish before the temporary files go.
	t.Cleanup(s.StopFollowing)
	return rg
}

func (rg *decisionRig) executed() []execCall {
	rg.mu.Lock()
	defer rg.mu.Unlock()
	return append([]execCall{}, rg.calls...)
}

// do sends a request with the device token from the phone.
func (rg *decisionRig) do(t *testing.T, method, path, body string) (int, map[string]any) {
	t.Helper()
	return serve(t, rg.h, method, path, body, phoneAddr)
}

// raise raises a decision in the rig's chat with its chat token.
func (rg *decisionRig) raise(t *testing.T, body string) (int, decisions.Decision, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/chats/"+rg.chatID+"/decisions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+ChatTokenFor("tok")(rg.chatID))
	req.RemoteAddr = "127.0.0.1:5000"
	w := httptest.NewRecorder()
	rg.h.ServeHTTP(w, req)
	var d decisions.Decision
	var raw map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &d)
	_ = json.Unmarshal(w.Body.Bytes(), &raw)
	return w.Code, d, raw
}

func (rg *decisionRig) answer(t *testing.T, id, body string) (int, decisions.Decision) {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/decisions/"+id+"/answer", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	req.RemoteAddr = phoneAddr
	w := httptest.NewRecorder()
	rg.h.ServeHTTP(w, req)
	var d decisions.Decision
	if w.Code == 200 {
		_ = json.Unmarshal(w.Body.Bytes(), &d)
	} else {
		var wrap struct {
			Decision decisions.Decision `json:"decision"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &wrap)
		d = wrap.Decision
	}
	return w.Code, d
}

func (rg *decisionRig) get(t *testing.T, id string) decisions.Decision {
	t.Helper()
	req := httptest.NewRequest("GET", "/v1/decisions/"+id, nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	rg.h.ServeHTTP(w, req)
	var d decisions.Decision
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil || w.Code != 200 {
		t.Fatalf("get %s: %d %s", id, w.Code, w.Body.String())
	}
	return d
}

// list GETs path with the device token into out.
func (rg *decisionRig) list(t *testing.T, path string, out any) int {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	rg.h.ServeHTTP(w, req)
	if w.Code == 200 {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	return w.Code
}

func waitState(t *testing.T, rg *decisionRig, id, state string) decisions.Decision {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if d := rg.get(t, id); d.State == state {
			return d
		}
		time.Sleep(10 * time.Millisecond)
	}
	d := rg.get(t, id)
	t.Fatalf("decision %s is %s (%s), want %s", id, d.State, d.Reason, state)
	return d
}

func has(list []string, sub string) bool {
	for _, s := range list {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func awaitingChange(rev string) *specflow.Change {
	return &specflow.Change{ID: "chg1", SeedID: "N-1", ChangeName: "offline-chat-resume", Phase: "awaiting_approval", Revision: rev,
		Summary: "Resume chats offline.",
		Sections: []specflow.Section{
			{ID: "s1", Title: "Store drafts", Body: "- [ ] 1.1 a\n- [ ] 1.2 b"},
			{ID: "s2", Title: "Reconnect", Body: "- [ ] 2.1 c"},
		}}
}

func boolp(b bool) *bool { return &b }

// ---- 2.2: building decisions per kind ----

func TestApproveDecisionOffersTheThreeOptions(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.push = boolp(true)
	rg.facts.setChange("N-1", awaitingChange("4c0ffee1234"))
	rg.facts.summaries = []api.ChangeSummary{{ID: "chg1", ChangeName: "offline-chat-resume", SeedIDs: []string{"N-1"}}}

	code, d, raw := rg.raise(t, `{"kind":"approve","target":{"change":"offline-chat-resume"}}`)
	if code != http.StatusCreated {
		t.Fatalf("raise: %d %v", code, raw)
	}
	if d.Kind != "approve" || d.Source != "agent" || d.State != "pending" || d.ChatID == nil || *d.ChatID != rg.chatID ||
		d.Target.Bead != "N-1" || d.Target.Revision != "4c0ffee1234" || d.Title != "Approve offline-chat-resume and run it" ||
		!strings.Contains(d.Subtitle, "Mac-Studio") || !strings.Contains(d.Subtitle, "revision 4c0ffee") || d.ExpiresAt == nil {
		t.Fatalf("decision %+v", d)
	}
	var ids, labels []string
	for _, o := range d.Options {
		ids, labels = append(ids, o.ID), append(labels, o.Label)
		if !o.Protected {
			t.Errorf("option %s isn't protected", o.ID)
		}
	}
	if strings.Join(ids, ",") != "run_hold,run_land,approve_only" || !d.Options[0].Recommended || d.Options[1].Recommended ||
		strings.Join(labels, "|") != "Approve, run, I'll land|Approve, run and land to main|Approve only" {
		t.Fatalf("options %+v", d.Options)
	}
	if !has(d.Consequences, "Freezes the plan at revision 4c0ffee: 2 sections, 3 tasks") || !has(d.Consequences, "Up to $25") ||
		!has(d.Consequences, "Pushes main to origin") {
		t.Fatalf("consequences %q", d.Consequences)
	}
	if !d.AllowReject || !d.RejectNeedsNote || d.RejectLabel != "Not right" || len(d.Evidence) == 0 || d.Evidence[0].Kind != "review" {
		t.Fatalf("reject/evidence %+v", d)
	}
	if strings.Contains(fmt.Sprint(raw), "/v1/repos") {
		t.Fatalf("the bound action reached the client: %v", raw)
	}
	// By bead too; a change not awaiting approval can't be approved.
	if code, d2, _ := rg.raise(t, `{"kind":"approve","target":{"bead":"N-1"}}`); code != 201 || d2.Target.Change != "offline-chat-resume" {
		t.Fatalf("by bead: %d %+v", code, d2)
	}
	c := awaitingChange("4c0ffee1234")
	c.Phase = "drafting"
	rg.facts.setChange("N-1", c)
	if code, _, raw := rg.raise(t, `{"kind":"approve","target":{"bead":"N-1"}}`); code != http.StatusConflict || !strings.Contains(raw["error"].(string), "isn't awaiting approval") {
		t.Fatalf("drafting: %d %v", code, raw)
	}
}

func landRound() api.Round {
	return api.Round{ID: "rdto3k", RepoID: "r1", RepoName: "Nacre", RepoPath: "/x", State: api.RoundReadyToLand, Target: "main",
		FinalCommit: "a1b2c3d4e5", Artifacts: []string{"/x/app.apk"},
		Beads: []api.RoundBead{
			{ID: "N-2", State: api.BeadIntegrated},
			{ID: "N-3", State: api.BeadIntegrated, HumanCheck: "Try it on the Fold"},
			{ID: "N-4", State: api.BeadParked, Reason: "gates failed twice"},
		}}
}

func TestLandDecisionResolvesPushFromNacreYAML(t *testing.T) {
	rg := newDecisionRig(t)
	r := landRound()
	r.LandPush = boolp(false)
	rg.facts.setRound(r)
	code, d, raw := rg.raise(t, `{"kind":"land","target":{"round":"rdto3k"}}`)
	if code != 201 {
		t.Fatalf("raise: %d %v", code, raw)
	}
	if d.Options[0].ID != "land" || d.Options[0].Label != "Merge, don't push" || d.Target.Commit != "a1b2c3d4e5" ||
		d.Options[1].ID != "cancel" || d.Options[1].Protected || d.RejectLabel != "Don't land" {
		t.Fatalf("options %+v", d)
	}
	for _, want := range []string{"Merges commit a1b2c3d into main", "Moves main on Mac-Studio only; doesn't push",
		"Closes 1 bead: N-2", "Leaves open for your check: N-3 (Try it on the Fold)", "Leaves open: N-4 (gates failed twice)"} {
		if !has(d.Consequences, want) {
			t.Errorf("consequences lack %q: %q", want, d.Consequences)
		}
	}
	if !strings.Contains(d.Body, "/x/app.apk") {
		t.Errorf("body %q", d.Body)
	}
	// An older round that doesn't say: nacre.yaml decides; unreadable means "depends".
	r.LandPush = nil
	r.ID = "rdold"
	rg.facts.setRound(r)
	rg.facts.push = boolp(true)
	if _, d, _ := rg.raise(t, `{"kind":"land","target":{"round":"rdold"}}`); d.Options[0].Label != "Merge into main and push" {
		t.Fatalf("nacre.yaml push: %+v", d.Options[0])
	}
	rg.facts.push = nil
	if _, d, _ := rg.raise(t, `{"kind":"land","target":{"round":"rdold"}}`); !has(d.Consequences, "depends on nacre.yaml") {
		t.Fatalf("unknown push: %q", d.Consequences)
	}
	r.State = api.RoundImplementing
	rg.facts.setRound(r)
	if code, _, _ := rg.raise(t, `{"kind":"land","target":{"round":"rdold"}}`); code != http.StatusConflict {
		t.Fatalf("not ready: %d", code)
	}
	if code, _, _ := rg.raise(t, `{"kind":"land","target":{"round":"rdnope"}}`); code != http.StatusNotFound {
		t.Fatalf("unknown round: %d", code)
	}
}

func TestFinishSignoffLimitAndAdoptDecisions(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.push = boolp(true)
	rg.facts.beads["N-9"] = BeadFacts{ID: "N-9", Title: "Offline chat", Status: "open", Type: "epic"}
	rg.facts.summaries = []api.ChangeSummary{{ID: "chg1", ChangeName: "offline-chat-resume", EpicID: "N-9", SeedIDs: []string{"N-1"}}}
	code, d, raw := rg.raise(t, `{"kind":"finish","target":{"bead":"N-9"}}`)
	if code != 201 || d.Title != "Archive offline-chat-resume and merge into main" ||
		!has(d.Consequences, "Merges epic/n-9 into main") || !has(d.Consequences, "Archives offline-chat-resume") ||
		!has(d.Consequences, "Pushes main") || d.Options[0].Label != "Merge into main and push" {
		t.Fatalf("finish: %d %+v %v", code, d, raw)
	}

	rg.facts.beads["N-5"] = BeadFacts{ID: "N-5", Title: "Drafts survive", Status: "open", Labels: []string{"signoff"},
		Metadata: map[string]any{"signoff": "Kill the app mid-draft and reopen it"}}
	code, d, _ = rg.raise(t, `{"kind":"signoff","target":{"bead":"N-5"}}`)
	if code != 201 || d.Body != "Kill the app mid-draft and reopen it" || d.RejectLabel != "Not right" || !d.RejectNeedsNote ||
		d.Options[0].ID != "signoff" || !has(d.Consequences, "Closes N-5") {
		t.Fatalf("signoff: %d %+v", code, d)
	}
	rg.facts.beads["N-6"] = BeadFacts{ID: "N-6", Status: "open"}
	if code, _, _ := rg.raise(t, `{"kind":"signoff","target":{"bead":"N-6"}}`); code != http.StatusConflict {
		t.Fatalf("no signoff label: %d", code)
	}

	rg.facts.setRound(api.Round{ID: "rdlim", RepoID: "r1", RepoName: "Nacre", State: api.RoundPaused, BudgetUSD: 25, SpentUSD: 6.4,
		Stop: &api.Stop{Kind: api.StopLimits, Reason: "Budget reached", EstimateUSD: 8}})
	code, d, _ = rg.raise(t, `{"kind":"limit","target":{"round":"rdlim"}}`)
	in := d.Options[0].Input
	if code != 201 || in == nil || in.Kind != "amount" || *in.Min != 6.4 || *in.Max != 100 || in.Unit != "usd" ||
		!has(d.Consequences, "Spent $6.40 of $25") || !has(d.Consequences, "resumes the round") {
		t.Fatalf("limit: %d %+v %+v", code, d, in)
	}

	rg.facts.changes["N-7"] = specflow.Envelope{Available: true, Adoptable: []specflow.Adoptable{
		{ChangeName: "hand-written", TasksDone: 3, TasksTotal: 8, HasReview: true, Why: "Written in a terminal session"}}}
	code, d, _ = rg.raise(t, `{"kind":"adopt","target":{"bead":"N-7","change":"hand-written"}}`)
	if code != 201 || len(d.Options) != 2 || d.Options[0].ID != "adopt" || d.Options[1].ID != "adopt_recorded_review" ||
		!has(d.Consequences, "3 of 8 tasks already checked") {
		t.Fatalf("adopt: %d %+v", code, d)
	}
	if code, _, _ := rg.raise(t, `{"kind":"adopt","target":{"bead":"N-7","change":"nope"}}`); code != http.StatusConflict {
		t.Fatalf("unknown adoptable: %d", code)
	}
	if code, _, _ := rg.raise(t, `{"kind":"question","target":{"bead":"N-7"}}`); code != http.StatusBadRequest {
		t.Fatalf("a chat raising a question: %d", code)
	}
}

func TestAChatRaisesDecisionsInItsOwnChatOnly(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.setRound(landRound())
	other, _ := rg.s.Chats.Create(chat.CreateRequest{Agent: chat.AgentClaude})
	req := httptest.NewRequest("POST", "/v1/chats/"+other.ID+"/decisions", strings.NewReader(`{"kind":"land","target":{"round":"rdto3k"}}`))
	req.Header.Set("Authorization", "Bearer "+ChatTokenFor("tok")(rg.chatID))
	w := httptest.NewRecorder()
	rg.h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("another chat's decisions: %d %s", w.Code, w.Body.String())
	}
	// The app (device token) may raise one in any chat.
	if code, _ := rg.do(t, "POST", "/v1/chats/"+other.ID+"/decisions", `{"kind":"land","target":{"round":"rdto3k"}}`); code != 201 {
		t.Fatalf("device token: %d", code)
	}
}

// ---- 2.3: answering ----

func TestAnswerFromTheMacOrTheChatTokenIsRefused(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.setRound(landRound())
	_, d, _ := rg.raise(t, `{"kind":"land","target":{"round":"rdto3k"}}`)
	if code, body := serve(t, rg.h, "POST", "/v1/decisions/"+d.ID+"/answer", `{"option":"land"}`, "127.0.0.1:5555"); code != http.StatusForbidden || body["error"] != "Answer from a paired device" {
		t.Fatalf("from the Mac: %d %v", code, body)
	}
	req := httptest.NewRequest("POST", "/v1/decisions/"+d.ID+"/answer", strings.NewReader(`{"option":"land"}`))
	req.Header.Set("Authorization", "Bearer "+ChatTokenFor("tok")(rg.chatID))
	req.RemoteAddr = phoneAddr
	w := httptest.NewRecorder()
	rg.h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "Answer from a paired device") {
		t.Fatalf("chat token: %d %s", w.Code, w.Body.String())
	}
	if len(rg.executed()) != 0 || rg.get(t, d.ID).State != "pending" {
		t.Fatal("a refused answer executed or changed the decision")
	}
}

func TestStaleRevisionExpiresNamingTheNewOne(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.setChange("N-1", awaitingChange("rev4aaaaaaa"))
	_, d, _ := rg.raise(t, `{"kind":"approve","target":{"bead":"N-1"}}`)
	rg.facts.setChange("N-1", awaitingChange("rev5bbbbbbb"))
	code, got := rg.answer(t, d.ID, `{"option":"run_hold"}`)
	if code != 200 || got.State != "expired" || !strings.Contains(got.Reason, "revision rev5bbb") || !strings.Contains(got.Reason, "revision rev4aaa") {
		t.Fatalf("stale: %d %+v", code, got)
	}
	if len(rg.executed()) != 0 {
		t.Fatalf("a stale decision executed: %+v", rg.executed())
	}
	// Ask again builds a fresh decision for revision 5 that replaces it.
	code, body := rg.do(t, "POST", "/v1/decisions/"+d.ID+"/renew", ``)
	if code != 201 || body["target"].(map[string]any)["revision"] != "rev5bbbbbbb" || body["state"] != "pending" {
		t.Fatalf("renew: %d %v", code, body)
	}
	if old := rg.get(t, d.ID); old.SupersededBy != body["id"] || old.State != "expired" {
		t.Fatalf("old after renew: %+v", old)
	}
}

func TestTheSameAnswerTwiceRunsOnce(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.beads["N-5"] = BeadFacts{ID: "N-5", Title: "Drafts", Status: "open", Labels: []string{"signoff"}}
	_, d, _ := rg.raise(t, `{"kind":"signoff","target":{"bead":"N-5"}}`)
	var wg sync.WaitGroup
	results := make([]decisions.Decision, 4)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, results[i] = rg.answer(t, d.ID, `{"option":"signoff","note":"works"}`)
		}(i)
	}
	wg.Wait()
	calls := rg.executed()
	if len(calls) != 1 || calls[0].path != "/v1/repos/"+rg.repo+"/issues/N-5/signoff" || calls[0].body["approve"] != true || calls[0].body["note"] != "works" {
		t.Fatalf("executed %+v", calls)
	}
	final := rg.get(t, d.ID)
	if final.State != "done" || final.Result == nil || !final.Result.OK || final.Result.Message != "Signed off; N-5 is closed" || final.AnsweredAt == nil {
		t.Fatalf("final %+v", final)
	}
	if code, again := rg.answer(t, d.ID, `{"option":"signoff","note":"works"}`); code != 200 || again.Result == nil || again.Result.Message != final.Result.Message {
		t.Fatalf("again: %d %+v", code, again)
	}
	if len(rg.executed()) != 1 {
		t.Fatal("ran twice")
	}
	// A different answer to a decided decision is refused with its state.
	if code, got := rg.answer(t, d.ID, `{"reject":true,"note":"no"}`); code != http.StatusConflict || got.State != "done" {
		t.Fatalf("different answer: %d %+v", code, got)
	}
}

func TestFailedExecutionKeepsNacredsReason(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.setRound(landRound())
	_, d, _ := rg.raise(t, `{"kind":"land","target":{"round":"rdto3k"}}`)
	rg.reply = func(execCall) (int, string) { return 409, `{"error":"the round isn't ready to merge"}` }
	code, got := rg.answer(t, d.ID, `{"option":"land"}`)
	if code != 200 || got.State != "failed" || got.Result == nil || got.Result.OK || got.Result.Message != "the round isn't ready to merge" {
		t.Fatalf("failed: %d %+v", code, got)
	}
	// Failed stays in the queue until acknowledged.
	if !got.Open() {
		t.Fatal("failed isn't open")
	}
	if _, acked := rg.answer(t, d.ID, `{"acknowledge":true}`); acked.Open() {
		t.Fatalf("acknowledged %+v", acked)
	}
}

// Without a stand-in, the action runs through nacred's own route as a paired
// device would: here the land route answers that rounds aren't available.
func TestExecutionGoesThroughNacredsOwnRoute(t *testing.T) {
	rg := newDecisionRig(t)
	rg.s.DecisionExec = nil
	rg.facts.setRound(landRound())
	_, d, _ := rg.raise(t, `{"kind":"land","target":{"round":"rdto3k"}}`)
	code, got := rg.answer(t, d.ID, `{"option":"land"}`)
	if code != 200 || got.State != "failed" || got.Reason != "rounds aren't available on this computer" {
		t.Fatalf("in-process land: %d %+v", code, got)
	}
}

// Landing finishes later: the decision stays executing until the round
// lands, and a push failure shows as failed with the round still ready.
func TestLandIsFollowedToItsOutcome(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.setRound(landRound())
	_, d, _ := rg.raise(t, `{"kind":"land","target":{"round":"rdto3k"}}`)
	code, got := rg.answer(t, d.ID, `{"option":"land"}`)
	if code != 200 || got.State != "executing" || got.Result != nil {
		t.Fatalf("answer: %d %+v", code, got)
	}
	if calls := rg.executed(); len(calls) != 1 || calls[0].path != "/v1/rounds/rdto3k/land" {
		t.Fatalf("executed %+v", calls)
	}
	landed := landRound()
	landed.State, landed.LandedCommit, landed.Pushed = api.RoundLanded, "3f2c1aa9", true
	rg.facts.setRound(landed)
	done := waitState(t, rg, d.ID, "done")
	if done.Result.Message != "Landed at 3f2c1aa and pushed" {
		t.Fatalf("done %+v", done.Result)
	}

	r := landRound()
	r.ID = "rdpush"
	rg.facts.setRound(r)
	_, d2, _ := rg.raise(t, `{"kind":"land","target":{"round":"rdpush"}}`)
	rg.answer(t, d2.ID, `{"option":"land"}`)
	r.State, r.Stop = api.RoundPaused, &api.Stop{Kind: api.StopGitError, Phase: "land", Reason: "git push failed: rejected"}
	rg.facts.setRound(r)
	failed := waitState(t, rg, d2.ID, "failed")
	if failed.Reason != "git push failed: rejected" || failed.Result.OK {
		t.Fatalf("failed %+v", failed)
	}
}

func TestApproveRunHoldExecutesTheBoundRevision(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.setChange("N-1", awaitingChange("rev4aaaaaaa"))
	_, d, _ := rg.raise(t, `{"kind":"approve","target":{"bead":"N-1"}}`)
	if code, got := rg.answer(t, d.ID, `{"option":"run_hold"}`); code != 200 || got.State != "executing" {
		t.Fatalf("answer %d %+v", code, got)
	}
	call := rg.executed()[0]
	if call.body["action"] != "approve" || call.body["revision"] != "rev4aaaaaaa" || call.body["run"] != true || call.body["hold"] != true {
		t.Fatalf("executed %+v", call)
	}
	c := awaitingChange("rev4aaaaaaa")
	c.Phase, c.ApprovedRevision = "ready", "rev4aaaaaaa"
	c.History = []specflow.History{{Kind: "approval"}, {Kind: "round", RoundID: "rdnew"}}
	rg.facts.setChange("N-1", c)
	done := waitState(t, rg, d.ID, "done")
	if done.Result.Message != "Approved revision rev4aaa; round rdnew started" || rg.s.Decisions.ChatOf("rdnew") != rg.chatID {
		t.Fatalf("done %+v link=%q", done.Result, rg.s.Decisions.ChatOf("rdnew"))
	}
}

func TestLimitAmountIsValidated(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.setRound(api.Round{ID: "rdlim", RepoID: "r1", State: api.RoundPaused, BudgetUSD: 25, SpentUSD: 6.4})
	_, d, _ := rg.raise(t, `{"kind":"limit","target":{"round":"rdlim"}}`)
	for _, bad := range []string{`{"option":"raise"}`, `{"option":"raise","input":5}`, `{"option":"raise","input":6.4}`, `{"option":"raise","input":"500"}`} {
		if code, _ := rg.answer(t, d.ID, bad); code != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, code)
		}
	}
	rg.reply = func(execCall) (int, string) { return 200, `{"id":"rdlim","budget_usd":50}` }
	code, got := rg.answer(t, d.ID, `{"option":"raise","input":"50"}`)
	if code != 200 || got.State != "done" || got.Result.Message != "Budget raised to $50" {
		t.Fatalf("raise: %d %+v", code, got)
	}
	if call := rg.executed()[0]; call.path != "/v1/rounds/rdlim/limits" || call.body["budget_usd"] != 50.0 {
		t.Fatalf("executed %+v", call)
	}
}

// ---- 2.4: the queue lifecycle ----

func TestDeferKeepsItQueuedOutsideTheCount(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.setRound(landRound())
	_, d, _ := rg.raise(t, `{"kind":"land","target":{"round":"rdto3k"}}`)
	code, got := rg.answer(t, d.ID, `{"defer":true}`)
	if code != 200 || got.State != "deferred" || got.DeferredUntil == nil || !got.Open() || got.Waiting() {
		t.Fatalf("defer: %d %+v", code, got)
	}
	var list struct{ Decisions []decisions.Decision }
	if code := rg.list(t, "/v1/decisions?repo=r1", &list); code != 200 || len(list.Decisions) != 1 || list.Decisions[0].State != "deferred" {
		t.Fatalf("queue: %d %+v", code, list)
	}
	// It can still be answered.
	if code, got := rg.answer(t, d.ID, `{"option":"land"}`); code != 200 || got.State != "executing" {
		t.Fatalf("answered after defer: %d %+v", code, got)
	}
}

func TestRejectActsByKind(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.setChange("N-1", awaitingChange("rev4aaaaaaa"))
	_, ap, _ := rg.raise(t, `{"kind":"approve","target":{"bead":"N-1"}}`)
	if code, _ := rg.answer(t, ap.ID, `{"reject":true}`); code != http.StatusBadRequest {
		t.Fatalf("reject an approval without a note: %d", code)
	}
	code, got := rg.answer(t, ap.ID, `{"reject":true,"note":"keep it per chat"}`)
	call := rg.executed()
	if code != 200 || got.State != "declined" || len(call) != 1 || call[0].body["action"] != "revise" || call[0].body["feedback"] != "keep it per chat" {
		t.Fatalf("approval sent back: %d %+v %+v", code, got, call)
	}

	// Landing held: listed under Held with the note, Land and Cancel still offered.
	rg.facts.setRound(landRound())
	_, ld, _ := rg.raise(t, `{"kind":"land","target":{"round":"rdto3k"}}`)
	code, got = rg.answer(t, ld.ID, `{"reject":true,"note":"wait for the device check"}`)
	if code != 200 || got.State != "held" || got.Reason != "Held by you: wait for the device check" || !got.Open() || len(rg.executed()) != 1 {
		t.Fatalf("held: %d %+v", code, got)
	}
	if code, got := rg.answer(t, ld.ID, `{"option":"cancel"}`); code != 200 || got.State != "done" || got.Result.Message != "Round cancelled; its branches are kept" {
		t.Fatalf("cancel from held: %d %+v", code, got)
	}

	// A human check sent back with its note.
	rg.facts.beads["N-5"] = BeadFacts{ID: "N-5", Status: "open", Labels: []string{"signoff"}}
	_, so, _ := rg.raise(t, `{"kind":"signoff","target":{"bead":"N-5"}}`)
	code, got = rg.answer(t, so.ID, `{"reject":true,"note":"crashes on resume"}`)
	calls := rg.executed()
	last := calls[len(calls)-1]
	if code != 200 || got.State != "declined" || last.body["approve"] != false || last.body["note"] != "crashes on resume" {
		t.Fatalf("send back: %d %+v %+v", code, got, last)
	}
}

func TestSupersessionAndExpiry(t *testing.T) {
	rg := newDecisionRig(t)
	rg.facts.setRound(landRound())
	_, first, _ := rg.raise(t, `{"kind":"land","target":{"round":"rdto3k"}}`)
	_, second, _ := rg.raise(t, `{"kind":"land","target":{"round":"rdto3k"}}`)
	old := rg.get(t, first.ID)
	if old.State != "superseded" || old.SupersededBy != second.ID {
		t.Fatalf("old %+v", old)
	}
	if code, got := rg.answer(t, first.ID, `{"option":"land"}`); code != http.StatusConflict || got.State != "superseded" {
		t.Fatalf("answering a superseded one: %d %+v", code, got)
	}
	var open struct{ Decisions []decisions.Decision }
	rg.list(t, "/v1/decisions", &open)
	var all struct{ Decisions []decisions.Decision }
	rg.list(t, "/v1/decisions?state=all", &all)
	if len(open.Decisions) != 1 || open.Decisions[0].ID != second.ID || len(all.Decisions) != 2 {
		t.Fatalf("open %+v all %d", open.Decisions, len(all.Decisions))
	}
	// A round that moved on: the answer expires, naming the new commit.
	r := landRound()
	r.FinalCommit = "ffff0000ee"
	rg.facts.setRound(r)
	if _, got := rg.answer(t, second.ID, `{"option":"land"}`); got.State != "expired" || !strings.Contains(got.Reason, "commit ffff000") {
		t.Fatalf("moved on: %+v", got)
	}
}
