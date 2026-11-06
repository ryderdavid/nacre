package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/chat"
	"github.com/ryderdavid/nacre/bridge/internal/decisions"
	"github.com/ryderdavid/nacre/bridge/internal/specflow"
)

// Timing of decision execution; tests shorten them.
var (
	// followEvery is how often a decision whose action finishes later (a
	// land, a finish, an approval) checks on it.
	followEvery = 3 * time.Second
	// followFor is how long it keeps checking before the outcome is unknown.
	followFor = 8 * time.Hour
	// execTimeout bounds one executed action.
	execTimeout = 2 * time.Minute
)

// decisionsOn writes 503 and returns nil when decisions aren't set up here.
func (s *Server) decisionsOn(w http.ResponseWriter) *decisions.Store {
	if s.Decisions == nil {
		writeError(w, http.StatusServiceUnavailable, "Decisions aren't available on this computer.", "")
		return nil
	}
	return s.Decisions
}

// raiseRequest is POST /v1/chats/{id}/decisions.
type raiseRequest struct {
	Kind   string           `json:"kind"`
	Target decisions.Target `json:"target"`
	// Repo is the repository key, when the chat isn't in it.
	Repo string `json:"repo"`
	// Draft is an intake decision's New effort draft.
	Draft *specflow.IntakeDraft `json:"draft"`
	// Note is what found work is.
	Note string `json:"note"`
}

// raiseDecision builds a decision an agent asks for and stores it as
// pending in its chat. A chat token may raise decisions in its own chat only.
func (s *Server) raiseDecision(w http.ResponseWriter, r *http.Request) {
	store := s.decisionsOn(w)
	m := s.chatsOn(w)
	if store == nil || m == nil {
		return
	}
	chatID := r.PathValue("chat")
	if sc := scopeOf(r.Context()); sc.Chat() && sc.ChatID != chatID {
		writeError(w, http.StatusForbidden, "A chat raises decisions in its own chat only.", "")
		return
	}
	rec, _, err := m.Get(chatID, 0)
	if err != nil {
		s.chatFail(w, err)
		return
	}
	var req raiseRequest
	if !decodeOK(w, r, &req) {
		return
	}
	repo := req.Repo
	if repo == "" && rec.Repo != nil {
		repo = *rec.Repo
	}
	var built decisions.Record
	switch req.Kind {
	case decisions.KindIntake:
		built, err = s.buildIntake(r.Context(), repo, chatID, req.Target, req.Draft)
	case decisions.KindFound:
		built, err = s.buildFound(r.Context(), repo, chatID, req.Target, req.Note)
	default:
		built, err = s.buildDecision(r.Context(), repo, req.Kind, req.Target)
	}
	if err != nil {
		s.decisionFail(w, err)
		return
	}
	built.Decision.Source = decisions.SourceAgent
	built.Decision.ChatID = &chatID
	created, err := store.Create(built)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created.Decision)
}

func (s *Server) decisionFail(w http.ResponseWriter, err error) {
	var be *buildError
	if errors.As(err, &be) {
		writeError(w, be.status, be.msg, "")
		return
	}
	s.fail(w, err)
}

// listDecisions is the queue: stored decisions and Needs you items as
// decisions, for one repository or all, open ones (the default) or all.
func (s *Server) listDecisions(w http.ResponseWriter, r *http.Request) {
	if s.decisionsOn(w) == nil {
		return
	}
	q := r.URL.Query()
	all := q.Get("state") == "all"
	if st := q.Get("state"); st != "" && st != "open" && st != "all" {
		writeError(w, http.StatusBadRequest, "state is open or all", "")
		return
	}
	out := []decisions.Decision{}
	for _, rec := range s.queue(r.Context()) {
		d := rec.Decision
		if (q.Get("repo") != "" && d.Repo != q.Get("repo")) || (!all && !d.Open()) {
			continue
		}
		out = append(out, d)
	}
	writeJSON(w, http.StatusOK, map[string]any{"decisions": out})
}

// queue is every decision: the stored ones, then Needs you items not yet
// stored and not covered by an open stored decision for the same target,
// questions first, then approvals, then landing, newest first in each.
func (s *Server) queue(ctx context.Context) []decisions.Record {
	s.revalidateOpen(ctx)
	stored := s.Decisions.List()
	byID := map[string]bool{}
	openKeys := map[string]bool{}
	for _, rec := range stored {
		byID[rec.Decision.ID] = true
		if rec.Key != "" && rec.Decision.Open() {
			openKeys[rec.Key] = true
		}
	}
	virtual := s.attentionDecisions(ctx)
	s.announce(virtual)
	live := map[string]bool{}
	for _, rec := range virtual {
		live[rec.AttentionID] = true
	}
	out := stored[:0:0]
	for _, rec := range stored {
		if rec.AttentionID != "" && rec.Decision.Answerable() && s.Attention != nil && !live[rec.AttentionID] {
			// Its item no longer waits: a deferred or held card goes with it.
			if upd, err := s.Decisions.Update(rec.Decision.ID, func(r *decisions.Record) bool {
				if !r.Decision.Answerable() {
					return false
				}
				r.Decision.State, r.Decision.Reason, r.Decision.DeferredUntil = decisions.StateExpired, "This no longer waits on you.", nil
				r.Decision.Acknowledged = true
				return true
			}); err == nil {
				rec = upd
			}
		}
		out = append(out, rec)
	}
	for _, rec := range virtual {
		if byID[rec.Decision.ID] || (rec.Key != "" && openKeys[rec.Key]) {
			continue
		}
		out = append(out, rec)
	}
	sort.SliceStable(out, func(i, j int) bool {
		gi, gj := kindGroup(out[i].Decision.Kind), kindGroup(out[j].Decision.Kind)
		if gi != gj {
			return gi < gj
		}
		return out[i].Decision.CreatedAt > out[j].Decision.CreatedAt
	})
	return out
}

// kindGroup orders the queue: questions and found work, then approvals and
// checks, then landing, finishing and limits.
func kindGroup(kind string) int {
	switch kind {
	case decisions.KindQuestion, decisions.KindFound, decisions.KindRecovery, decisions.KindIntake:
		return 0
	case decisions.KindApprove, decisions.KindAdopt, decisions.KindSignoff:
		return 1
	}
	return 2
}

// lookup finds a decision: stored, or a Needs you item by its stable id.
func (s *Server) lookup(ctx context.Context, id string) (decisions.Record, bool) {
	if rec, ok := s.Decisions.Get(id); ok {
		return rec, true
	}
	for _, rec := range s.attentionDecisions(ctx) {
		if rec.Decision.ID == id {
			return rec, true
		}
	}
	return decisions.Record{}, false
}

func (s *Server) getDecision(w http.ResponseWriter, r *http.Request) {
	if s.decisionsOn(w) == nil {
		return
	}
	rec, ok := s.lookup(r.Context(), r.PathValue("decision"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown decision", "")
		return
	}
	writeJSON(w, http.StatusOK, rec.Decision)
}

// answerBody is POST /v1/decisions/{id}/answer: an option (with a note and
// an input), a defer, a reject (with a note), or acknowledging a failed or
// unknown outcome.
type answerBody struct {
	Option      string          `json:"option"`
	Note        string          `json:"note"`
	Input       json.RawMessage `json:"input"`
	Ask         string          `json:"ask"`
	Defer       bool            `json:"defer"`
	Reject      bool            `json:"reject"`
	Acknowledge bool            `json:"acknowledge"`
}

// inputText is the answer's input as text: a JSON string as is, a number
// in its shortest form.
func (b answerBody) inputText() string {
	raw := bytes.TrimSpace(b.Input)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return string(raw)
}

// answerDecision is Ryder's answer, from a paired device only. An option is
// revalidated against the bound revision and executed once: the same answer
// again gets the same result.
func (s *Server) answerDecision(w http.ResponseWriter, r *http.Request) {
	store := s.decisionsOn(w)
	if store == nil {
		return
	}
	if scopeOf(r.Context()).Chat() || s.fromThisComputer(r) {
		writeError(w, http.StatusForbidden, chat.PairedDeviceMsg, "Decisions are answered from the phone, not from this computer.")
		return
	}
	var body answerBody
	if !decodeOK(w, r, &body) {
		return
	}
	body.Note = strings.TrimSpace(body.Note)
	if len(body.Note) > 4000 {
		writeError(w, http.StatusBadRequest, "The note is over 4000 bytes.", "")
		return
	}
	n := 0
	for _, set := range []bool{body.Option != "", body.Defer, body.Reject, body.Acknowledge} {
		if set {
			n++
		}
	}
	if n != 1 {
		writeError(w, http.StatusBadRequest, "Answer with one of option, defer, reject or acknowledge.", "")
		return
	}
	id := r.PathValue("decision")
	rec, ok := s.lookup(r.Context(), id)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown decision", "")
		return
	}
	if _, stored := store.Get(id); !stored {
		// A Needs you item is stored once it is answered, keeping its id.
		var err error
		if rec, err = store.Put(rec); err != nil {
			s.fail(w, err)
			return
		}
	}
	ctx := context.WithoutCancel(r.Context())
	var (
		out decisions.Record
		err error
	)
	switch {
	case body.Acknowledge:
		out, err = store.Update(id, func(rec *decisions.Record) bool {
			if st := rec.Decision.State; st != decisions.StateFailed && st != decisions.StateUnknown && st != decisions.StateExpired {
				return false
			}
			rec.Decision.Acknowledged = true
			return true
		})
	case body.Defer:
		out, err = s.deferDecision(rec)
	case body.Reject:
		out, err = s.rejectDecision(ctx, rec, body)
	default:
		out, err = s.chooseOption(ctx, rec, body)
	}
	if err != nil {
		var ae *answerError
		if errors.As(err, &ae) {
			writeJSON(w, ae.status, map[string]any{"error": ae.msg, "decision": out.Decision})
			return
		}
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out.Decision)
}

// answerError refuses an answer: 400 for a bad one, 409 for a decision no
// longer answerable that way.
type answerError struct {
	status int
	msg    string
}

func (e *answerError) Error() string { return e.msg }

func refuse(status int, format string, a ...any) error {
	return &answerError{status, fmt.Sprintf(format, a...)}
}

func (s *Server) deferDecision(rec decisions.Record) (decisions.Record, error) {
	if !rec.Decision.AllowDefer {
		return rec, refuse(http.StatusBadRequest, "This decision can't be deferred.")
	}
	var refused error
	out, err := s.Decisions.Update(rec.Decision.ID, func(rec *decisions.Record) bool {
		d := &rec.Decision
		if d.State == decisions.StateDeferred {
			return false
		}
		if d.State != decisions.StatePending && d.State != decisions.StateHeld {
			refused = refuse(http.StatusConflict, "This decision is %s.", d.State)
			return false
		}
		until := s.Decisions.Clock().Add(decisions.DeferFor).UTC().Format(time.RFC3339)
		d.State, d.DeferredUntil = decisions.StateDeferred, &until
		return true
	})
	if err == nil {
		err = refused
	}
	return out, err
}

func (s *Server) rejectDecision(ctx context.Context, rec decisions.Record, body answerBody) (decisions.Record, error) {
	d := rec.Decision
	if !d.AllowReject {
		return rec, refuse(http.StatusBadRequest, "This decision can't be rejected.")
	}
	if d.RejectNeedsNote && body.Note == "" {
		return rec, refuse(http.StatusBadRequest, "Say what isn't right: rejecting this needs a note.")
	}
	ans := decisions.Answer{Reject: true, Note: body.Note}
	if !rec.RejectHolds && rec.Reject != nil {
		// Rejecting runs an action (a revision, a send-back): bound like an
		// option.
		return s.run(ctx, rec, ans, *rec.Reject, "")
	}
	var refused error
	out, err := s.Decisions.Update(d.ID, func(rec *decisions.Record) bool {
		d := &rec.Decision
		if d.Answer != nil && d.Answer.Same(ans) && (d.State == decisions.StateHeld || d.State == decisions.StateDeclined) {
			return false
		}
		if !d.Answerable() {
			refused = refuse(http.StatusConflict, "This decision is %s.", d.State)
			return false
		}
		now := s.Decisions.Now()
		d.Answer, d.AnsweredAt, d.DeferredUntil = &ans, &now, nil
		if rec.RejectHolds {
			d.State, d.Reason = decisions.StateHeld, "Held by you"
			if ans.Note != "" {
				d.Reason += ": " + ans.Note
			}
			// Held work keeps its expiry off: it waits for Ryder.
			d.ExpiresAt = nil
		} else {
			d.State, d.Reason = decisions.StateDeclined, ans.Note
		}
		return true
	})
	if err == nil {
		err = refused
	}
	return out, err
}

// chooseOption validates an option and its input, then runs its action.
func (s *Server) chooseOption(ctx context.Context, rec decisions.Record, body answerBody) (decisions.Record, error) {
	var opt *decisions.Option
	for i := range rec.Decision.Options {
		if rec.Decision.Options[i].ID == body.Option {
			opt = &rec.Decision.Options[i]
		}
	}
	if opt == nil {
		return rec, refuse(http.StatusBadRequest, "unknown option %q", body.Option)
	}
	input := body.inputText()
	ask := strings.TrimSpace(body.Ask)
	if in := rec.Decision.Ask; in != nil && ask == "" {
		ask = in.Default
	}
	ans := decisions.Answer{Option: opt.ID, Note: body.Note, Input: input, Ask: ask}
	if d := rec.Decision; d.Answer != nil && d.Answer.Same(ans) && !d.Answerable() {
		// The same answer again: its result, not a second execution.
		return rec, nil
	}
	if opt.Disabled != "" {
		return rec, refuse(http.StatusConflict, "%s", opt.Disabled)
	}
	if msg := checkInput(opt.Input, input); msg != "" {
		return rec, refuse(http.StatusBadRequest, "%s", msg)
	}
	if msg := checkInput(rec.Decision.Ask, ask); msg != "" {
		return rec, refuse(http.StatusBadRequest, "%s", msg)
	}
	act, ok := rec.Actions[opt.ID]
	if !ok {
		return s.record(rec, ans, opt.Label)
	}
	return s.run(ctx, rec, ans, act, input)
}

// checkInput validates an option's input; "" when it is fine.
func checkInput(in *decisions.Input, v string) string {
	if in == nil {
		return ""
	}
	if v == "" {
		if in.Required {
			return "This option needs " + strings.ToLower(or(in.Label, "an answer")) + "."
		}
		return ""
	}
	switch in.Kind {
	case decisions.InputAmount:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return "Enter an amount."
		}
		if in.Min != nil && f <= *in.Min {
			return fmt.Sprintf("Enter more than %s (what's already spent).", usd(*in.Min))
		}
		if in.Max != nil && f > *in.Max {
			return fmt.Sprintf("Enter at most %s (the runaway ceiling).", usd(*in.Max))
		}
	case decisions.InputChoice:
		for _, c := range in.Choices {
			if c.ID == v {
				return ""
			}
		}
		return "Pick one of the choices."
	}
	return ""
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// record answers an option that executes nothing.
func (s *Server) record(rec decisions.Record, ans decisions.Answer, label string) (decisions.Record, error) {
	var refused error
	out, err := s.Decisions.Update(rec.Decision.ID, func(rec *decisions.Record) bool {
		d := &rec.Decision
		if !d.Answerable() {
			refused = refuse(http.StatusConflict, "This decision was already answered.")
			return false
		}
		now := s.Decisions.Now()
		d.Answer, d.AnsweredAt, d.DeferredUntil = &ans, &now, nil
		d.State, d.Result, d.Reason = decisions.StateDone, &decisions.Result{OK: true, Message: "Recorded: " + label}, ""
		return true
	})
	if err == nil {
		err = refused
	}
	return out, err
}

// run revalidates a decision, claims it for execution (so the same decision
// never runs twice), executes the action and records the outcome. An action
// that finishes later leaves it executing for follow.
func (s *Server) run(ctx context.Context, rec decisions.Record, ans decisions.Answer, act decisions.Action, input string) (decisions.Record, error) {
	if !rec.Decision.Answerable() {
		if rec.Decision.Answer != nil && rec.Decision.Answer.Same(ans) {
			return rec, nil
		}
		return rec, refuse(http.StatusConflict, "This decision is %s.", rec.Decision.State)
	}
	if reason := s.revalidate(ctx, rec); reason != "" {
		out, err := s.Decisions.Update(rec.Decision.ID, func(rec *decisions.Record) bool {
			if !rec.Decision.Answerable() {
				return false
			}
			rec.Decision.State, rec.Decision.Reason, rec.Decision.DeferredUntil = decisions.StateExpired, reason, nil
			return true
		})
		return out, err
	}
	claimed := false
	out, err := s.Decisions.Update(rec.Decision.ID, func(r *decisions.Record) bool {
		d := &r.Decision
		if !d.Answerable() {
			return false
		}
		now := s.Decisions.Now()
		d.Answer, d.AnsweredAt, d.DeferredUntil = &ans, &now, nil
		d.State, d.Reason, d.Result = decisions.StateExecuting, "", nil
		claimed = true
		return true
	})
	if err != nil || !claimed {
		if err == nil && (out.Decision.Answer == nil || !out.Decision.Answer.Same(ans)) {
			err = refuse(http.StatusConflict, "This decision was already answered.")
		}
		return out, err
	}
	act = fill(act, input, ans.Note, ans.Ask)
	ectx, cancel := context.WithTimeout(ctx, execTimeout)
	status, resp, said := s.execChain(ectx, rec, act)
	cancel()
	follow := rec.Follow && !ans.Reject && status/100 == 2 && ans.Option != "cancel"
	out, err = s.Decisions.Update(rec.Decision.ID, func(r *decisions.Record) bool {
		d := &r.Decision
		switch {
		case status/100 == 2 && follow:
			if r.Decision.Kind == decisions.KindFinish {
				var fin struct {
					Round *api.Round `json:"round"`
				}
				if json.Unmarshal(resp, &fin) == nil && fin.Round != nil {
					d.Target.Round = fin.Round.ID
				}
			}
			r.Follow = true
		case status/100 == 2:
			d.State = decisions.StateDone
			if ans.Reject {
				d.State = decisions.StateDeclined
			}
			msg := said
			if msg == "" {
				msg = doneMessage(*r, ans, resp)
			}
			d.Result = &decisions.Result{OK: true, Message: msg}
		case status/100 == 4 || status == http.StatusServiceUnavailable || said != "":
			// A chain that stopped part way says what it did and what didn't happen.
			msg := said
			if msg == "" {
				msg = errorMessage(resp, status)
			}
			d.State, d.Reason, d.Result = decisions.StateFailed, msg, &decisions.Result{OK: false, Message: msg}
		default:
			msg := "nacred couldn't tell whether it ran: " + errorMessage(resp, status) + ". Check before answering again."
			d.State, d.Reason, d.Result = decisions.StateUnknown, msg, &decisions.Result{OK: false, Message: msg}
		}
		return true
	})
	if err == nil && follow {
		if out.Decision.ChatID != nil && out.Decision.Kind == decisions.KindFinish && out.Decision.Target.Round != "" {
			s.Decisions.LinkWork(out.Decision.Target.Round, *out.Decision.ChatID)
		}
		// A finish that closed the epic at once is already done.
		if done, ok := s.followCheck(context.Background(), out, resp); ok {
			out, err = s.settle(out.Decision.ID, done)
		} else {
			s.goFollow(out.Decision.ID)
		}
	}
	return out, err
}

// fill puts an answer's input, note and ask into an action's body.
func fill(act decisions.Action, input, note, ask string) decisions.Action {
	body := map[string]any{}
	for k, v := range act.Body {
		body[k] = v
	}
	if act.InputField != "" && input != "" {
		if f, err := strconv.ParseFloat(input, 64); err == nil && (act.InputField == "budget_usd" || act.InputField == "time_limit_minutes") {
			body[act.InputField] = f
		} else {
			body[act.InputField] = input
		}
	}
	if act.NoteField != "" && note != "" {
		body[act.NoteField] = note
	}
	if act.AskField != "" && ask != "" {
		body[act.AskField] = ask
	}
	act.Body = body
	return act
}

// execAction carries out an action as a paired device would, through
// nacred's own routes (maintenance included, protection not).
func (s *Server) execAction(ctx context.Context, act decisions.Action) (int, []byte) {
	if s.DecisionExec != nil {
		return s.DecisionExec(ctx, act)
	}
	if act.Method == decisions.MethodEffort {
		return s.startEffort(act)
	}
	var rd *bytes.Reader
	if len(act.Body) > 0 {
		b, _ := json.Marshal(act.Body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(withScope(ctx, Scope{}), act.Method, act.Path, rd)
	if err != nil {
		return http.StatusInternalServerError, []byte(`{"error":"bad action"}`)
	}
	req.RemoteAddr = "nacred"
	w := &recorder{header: http.Header{}}
	s.inner().ServeHTTP(w, req)
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.status, w.body.Bytes()
}

// recorder is a minimal in-process ResponseWriter.
type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *recorder) Header() http.Header { return r.header }
func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
}
func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(b)
}

// inner is the routed handler without auth or protection, for executing
// approved decisions.
func (s *Server) inner() http.Handler {
	s.innerOnce.Do(func() {
		mux := http.NewServeMux()
		for _, rt := range s.routes() {
			mux.HandleFunc(rt.method+" "+rt.pattern, rt.handler)
		}
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusNotFound, "no such route: "+r.Method+" "+r.URL.Path, "")
		})
		s.innerHandler = s.maintenance(mux)
	})
	return s.innerHandler
}

// errorMessage is an error response's message.
func errorMessage(body []byte, status int) string {
	var e struct {
		Error  string `json:"error"`
		Detail string `json:"detail"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error != "" {
		return e.Error
	}
	return "nacred answered " + strconv.Itoa(status)
}

// doneMessage says what an action that finished at once did.
func doneMessage(rec decisions.Record, ans decisions.Answer, resp []byte) string {
	d := rec.Decision
	switch {
	case d.Kind == decisions.KindSignoff && ans.Reject:
		return "Sent back with your note; " + d.Target.Bead + " stays open"
	case d.Kind == decisions.KindSignoff:
		return "Signed off; " + d.Target.Bead + " is closed"
	case d.Kind == decisions.KindApprove && ans.Reject:
		return "Sent back for revision with your note"
	case d.Kind == decisions.KindLimit:
		var round api.Round
		if json.Unmarshal(resp, &round) == nil && round.BudgetUSD > 0 {
			return "Budget raised to " + usd(round.BudgetUSD)
		}
		return "Budget raised"
	case d.Kind == decisions.KindAdopt:
		return "Adopting " + d.Target.Change + ": its review is under way"
	case ans.Option == "cancel":
		return "Round cancelled; its branches are kept"
	}
	for _, o := range d.Options {
		if o.ID == ans.Option {
			return "Done: " + o.Label
		}
	}
	return "Done"
}

// revalidate checks a decision still applies before it runs: the revision it
// was bound to is current and its action still applies. It returns why not.
func (s *Server) revalidate(ctx context.Context, rec decisions.Record) string {
	reason, _ := s.stale(ctx, rec)
	return reason
}

// stale says why a decision no longer applies, and whether that is sure: a
// moved revision or an action that no longer applies is; a read that failed
// (bd timing out) isn't, and a sweep leaves the decision alone for it.
func (s *Server) stale(ctx context.Context, rec decisions.Record) (string, bool) {
	d := rec.Decision
	if rec.AttentionID != "" {
		if !s.attentionStill(ctx, rec.AttentionID) {
			return "This is no longer waiting on you: its state changed. Refresh to see what is.", true
		}
		if rec.Bound == "" {
			return "", false
		}
	}
	if !decisions.Protected(d.Kind) {
		return "", false
	}
	t := d.Target
	t.Revision, t.Commit = "", ""
	if d.Kind == decisions.KindFinish {
		t.Round = ""
	}
	fresh, err := s.buildDecision(ctx, d.Repo, d.Kind, t)
	if err != nil {
		var be *buildError
		return "It no longer applies: " + err.Error(), errors.As(err, &be)
	}
	if fresh.Bound != rec.Bound {
		return expiryReason(d, rec.Bound, fresh.Bound), true
	}
	return "", false
}

// revalidateEvery is how often one open decision is checked against the
// current state when the queue is listed or swept; tests shorten it.
var revalidateEvery = 20 * time.Second

// revalidateOpen expires every open, unanswered decision whose revision
// moved or whose action no longer applies, so a card for an obsolete
// revision doesn't stay pending. Each decision is checked at most every
// revalidateEvery; an expiry is published like any change.
func (s *Server) revalidateOpen(ctx context.Context) {
	if s.Decisions == nil {
		return
	}
	now := time.Now()
	for _, rec := range s.Decisions.List() {
		d := rec.Decision
		if !d.Answerable() || rec.Bound == "" || !decisions.Protected(d.Kind) {
			s.checkedMu.Lock()
			delete(s.checked, d.ID)
			s.checkedMu.Unlock()
			continue
		}
		s.checkedMu.Lock()
		if s.checked == nil {
			s.checked = map[string]time.Time{}
		}
		last, seen := s.checked[d.ID]
		due := !seen || now.Sub(last) >= revalidateEvery
		if due {
			s.checked[d.ID] = now
		}
		s.checkedMu.Unlock()
		if !due {
			continue
		}
		reason, sure := s.stale(ctx, rec)
		if reason == "" || !sure {
			continue
		}
		_, _ = s.Decisions.Update(d.ID, func(r *decisions.Record) bool {
			if !r.Decision.Answerable() || r.Bound != rec.Bound {
				return false
			}
			r.Decision.State, r.Decision.Reason, r.Decision.DeferredUntil = decisions.StateExpired, reason, nil
			return true
		})
		s.checkedMu.Lock()
		delete(s.checked, d.ID)
		s.checkedMu.Unlock()
	}
}

// settled is a followed decision's outcome.
type settled struct {
	state   string
	message string
}

// settle records a followed decision's outcome.
func (s *Server) settle(id string, out settled) (decisions.Record, error) {
	return s.Decisions.Update(id, func(r *decisions.Record) bool {
		d := &r.Decision
		if d.State != decisions.StateExecuting {
			return false
		}
		d.State = out.state
		d.Result = &decisions.Result{OK: out.state == decisions.StateDone, Message: out.message}
		d.Reason = ""
		if out.state != decisions.StateDone {
			d.Reason = out.message
		}
		return true
	})
}

// follow checks on an action that finishes later until it ends, or gives up
// with the outcome unknown.
// goFollow follows a decision to its outcome in the background.
func (s *Server) goFollow(id string) {
	s.followInit.Do(func() { s.stopFollow = make(chan struct{}) })
	s.following.Add(1)
	go func() {
		defer s.following.Done()
		s.follow(id)
	}()
}

// StopFollowing stops following decisions in the background and waits for
// the follows to return.
func (s *Server) StopFollowing() {
	s.followInit.Do(func() { s.stopFollow = make(chan struct{}) })
	s.stopOnce.Do(func() { close(s.stopFollow) })
	s.following.Wait()
}

func (s *Server) follow(id string) {
	deadline := time.Now().Add(followFor)
	tick := time.NewTicker(followEvery)
	defer tick.Stop()
	for {
		select {
		case <-s.stopFollow:
			return
		case <-tick.C:
		}
		rec, ok := s.Decisions.Get(id)
		if !ok || rec.Decision.State != decisions.StateExecuting {
			return
		}
		if out, done := s.followCheck(context.Background(), rec, nil); done {
			_, _ = s.settle(id, out)
			return
		}
		if time.Now().After(deadline) {
			_, _ = s.settle(id, settled{decisions.StateUnknown, "nacred stopped watching before it finished. Check the work before answering again."})
			return
		}
	}
}

// followCheck reads how a followed action stands. resp is the action's
// response when called right after it ran.
func (s *Server) followCheck(ctx context.Context, rec decisions.Record, resp []byte) (settled, bool) {
	d := rec.Decision
	switch d.Kind {
	case decisions.KindLand, decisions.KindFinish:
		if d.Kind == decisions.KindFinish && resp != nil {
			var fin struct {
				Closed bool `json:"closed"`
			}
			if json.Unmarshal(resp, &fin) == nil && fin.Closed {
				return settled{decisions.StateDone, d.Target.Bead + " closed: its work was already merged"}, true
			}
		}
		if d.Target.Round == "" {
			return settled{}, false
		}
		round, err := s.round(d.Target.Round)
		if err != nil {
			return settled{}, false
		}
		switch round.State {
		case api.RoundLanded:
			msg := "Landed at " + short(round.LandedCommit)
			if round.Pushed {
				msg += " and pushed"
			} else {
				msg += "; not pushed"
			}
			if round.Tag != "" {
				msg += " (tagged " + round.Tag + ")"
			}
			return settled{decisions.StateDone, msg}, true
		case api.RoundPaused:
			why := "The round stopped"
			if round.Stop != nil && round.Stop.Reason != "" {
				why = round.Stop.Reason
			}
			return settled{decisions.StateFailed, why}, true
		case api.RoundCancelled:
			return settled{decisions.StateFailed, "The round was cancelled"}, true
		}
	case decisions.KindApprove:
		ref, err := s.repoRef(d.Repo)
		if err != nil {
			return settled{}, false
		}
		env, err := s.facts().Change(ctx, ref.Path, d.Target.Bead)
		if err != nil || env.Change == nil {
			return settled{}, false
		}
		c := env.Change
		switch c.Phase {
		case "ready":
			if c.ApprovedRevision != rec.Bound {
				return settled{decisions.StateFailed, "Revision " + short(c.ApprovedRevision) + " was approved instead"}, true
			}
			if d.Answer == nil || d.Answer.Option == "approve_only" {
				return settled{decisions.StateDone, "Approved revision " + short(rec.Bound) + "; its beads are ready"}, true
			}
			for i := len(c.History) - 1; i >= 0; i-- {
				h := c.History[i]
				if h.Kind == "round" && h.RoundID != "" {
					if d.ChatID != nil {
						s.Decisions.LinkWork(h.RoundID, *d.ChatID)
						if c.EpicID != "" {
							s.Decisions.LinkWork(beadLink(d.Repo, c.EpicID), *d.ChatID)
						}
					}
					return settled{decisions.StateDone, "Approved revision " + short(rec.Bound) + "; round " + h.RoundID + " started"}, true
				}
				if h.Kind == "run_error" {
					return settled{decisions.StateFailed, h.Text}, true
				}
				if h.Kind == "approval" {
					break
				}
			}
		case "needs_attention", "interrupted", "stale":
			return settled{decisions.StateFailed, or(c.Error, "The approval stopped: "+strings.ReplaceAll(c.Phase, "_", " "))}, true
		}
	}
	return settled{}, false
}

// renewDecision is Ask again: a fresh decision from the current state for
// the same kind and target, which supersedes the old one.
func (s *Server) renewDecision(w http.ResponseWriter, r *http.Request) {
	store := s.decisionsOn(w)
	if store == nil {
		return
	}
	old, ok := s.lookup(r.Context(), r.PathValue("decision"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown decision", "")
		return
	}
	d := old.Decision
	if d.State == decisions.StateExecuting {
		writeError(w, http.StatusConflict, "This decision is being carried out.", "")
		return
	}
	switch {
	case decisions.Protected(d.Kind):
	case d.Source == decisions.SourceAgent:
		writeError(w, http.StatusConflict, "Ask the chat to raise this again: it is built from what the chat sent.", "")
		return
	default:
		writeError(w, http.StatusConflict, "Needs you raises this again while it still waits on you.", "")
		return
	}
	t := d.Target
	t.Revision, t.Commit = "", ""
	if d.Kind == decisions.KindFinish {
		t.Round = ""
	}
	fresh, err := s.buildDecision(r.Context(), d.Repo, d.Kind, t)
	if err != nil {
		s.decisionFail(w, err)
		return
	}
	fresh.Decision.Source, fresh.Decision.ChatID = d.Source, d.ChatID
	if _, stored := store.Get(d.ID); !stored {
		if _, err := store.Put(old); err != nil {
			s.fail(w, err)
			return
		}
	}
	created, err := store.Create(fresh)
	if err != nil {
		s.fail(w, err)
		return
	}
	// The old one links to its replacement whatever state it ended in.
	_, _ = store.Update(d.ID, func(r *decisions.Record) bool {
		if r.Decision.SupersededBy != "" {
			return false
		}
		r.Decision.SupersededBy = created.Decision.ID
		return true
	})
	writeJSON(w, http.StatusCreated, created.Decision)
}

// StartDecisions resumes following decisions a previous nacred left
// executing, marks unfollowable ones unknown, and sweeps expiry every minute
// until ctx ends.
func (s *Server) StartDecisions(ctx context.Context) {
	if s.Decisions == nil {
		return
	}
	s.announceNeedsYou(ctx)
	for _, rec := range s.Decisions.List() {
		if rec.Decision.State != decisions.StateExecuting {
			continue
		}
		if rec.Follow {
			s.goFollow(rec.Decision.ID)
			continue
		}
		_, _ = s.settle(rec.Decision.ID, settled{decisions.StateUnknown,
			"nacred restarted while carrying this out. Check the result before answering again."})
	}
	go func() {
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				s.Decisions.Sweep()
				s.announceNeedsYou(ctx)
				s.revalidateOpen(ctx)
			}
		}
	}()
}

// announceNeedsYou publishes Needs you decisions that appeared since the
// last look, so a phone hears about them without listing the queue.
func (s *Server) announceNeedsYou(ctx context.Context) {
	if s.Attention == nil {
		return
	}
	s.announce(s.attentionDecisions(ctx))
}

// announce publishes each derived decision the first time it is seen. The
// first call only records what already waits: a restart announces nothing.
func (s *Server) announce(virtual []decisions.Record) {
	s.announceMu.Lock()
	first := s.announced == nil
	if first {
		s.announced = map[string]bool{}
	}
	var fresh []decisions.Decision
	for _, rec := range virtual {
		id := rec.Decision.ID
		if s.announced[id] {
			continue
		}
		s.announced[id] = true
		if _, stored := s.Decisions.Get(id); !first && !stored {
			fresh = append(fresh, rec.Decision)
		}
	}
	s.announceMu.Unlock()
	for _, d := range fresh {
		s.decisionChanged(d)
	}
}
