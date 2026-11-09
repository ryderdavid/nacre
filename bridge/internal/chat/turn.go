package chat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// TurnTimeout stops a turn that has run for this long. Chat turns can build
// and test for a long time; this only ends one that is wedged.
const TurnTimeout = 6 * time.Hour

// maxText bounds one message's text.
const maxText = 100_000

// SendRequest is POST /v1/chats/{id}/messages.
type SendRequest struct {
	TurnID  string   `json:"turn_id"`
	Text    string   `json:"text"`
	Context *Context `json:"context"`
	// Answered is the decision the phone just answered, for a message such
	// as "Answered: Approve only" that continues the walk-through.
	Answered string `json:"answered,omitempty"`
	// Images are the message's images, validated and decoded by the
	// server; they arrive as the body's images array.
	Images []claude.Image `json:"-"`
}

// SendResult is the turn a message started, or the one a resend names.
type SendResult struct {
	TurnID string `json:"turn_id"`
	// Started is false when the turn id was already known.
	Started bool   `json:"started"`
	Chat    Record `json:"chat"`
}

// turn is a running agent process.
type turn struct {
	id     string
	cancel context.CancelFunc
	mu     sync.Mutex
	// stopped is set when the turn was cancelled on purpose.
	stopped bool
	// cost is what the turn's router, light and full parts spent (guarded
	// by the manager's lock).
	cost float64
}

func (t *turn) stop() {
	t.markStopped()
	t.cancel()
}

// markStopped records that the turn is being stopped on purpose, so a
// permission request arriving now is denied at once.
func (t *turn) markStopped() {
	t.mu.Lock()
	t.stopped = true
	t.mu.Unlock()
}

func (t *turn) wasStopped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stopped
}

// Send starts a turn with the message, detached from the caller. A turn id
// the chat already knows returns that turn and starts nothing.
func (m *Manager) Send(id string, req SendRequest) (SendResult, error) {
	if strings.TrimSpace(req.Text) == "" && len(req.Images) == 0 {
		return SendResult{}, errors.New("text or images required")
	}
	if len(req.Text) > maxText {
		return SendResult{}, errors.New("text too long")
	}
	// nacred's state of an answered decision is read before the chat is
	// locked: the lookup can be slow.
	answeredState := ""
	if req.Answered != "" && decisionIDRe.MatchString(req.Answered) {
		if h := m.hooks(); h.State != nil {
			answeredState = h.State(req.Answered)
		}
	}
	if req.TurnID == "" {
		req.TurnID = newID("t_")
	} else if !ValidTurnID(req.TurnID) {
		return SendResult{}, errors.New("invalid turn id")
	}
	m.mu.Lock()
	c := m.chats[id]
	if c == nil {
		m.mu.Unlock()
		return SendResult{}, ErrNotFound
	}
	for _, known := range c.rec.TurnIDs {
		if known == req.TurnID {
			res := SendResult{TurnID: known, Chat: c.rec}
			m.mu.Unlock()
			return res, nil
		}
	}
	switch {
	case c.rec.State == StateRunning || c.rec.State == StateWaitingPermission:
		m.mu.Unlock()
		return SendResult{}, ErrBusy
	case c.rec.State == StateLimitReached || c.rec.CostUSD >= claude.ChatRunawayUSD-0.005:
		if c.rec.State != StateLimitReached {
			c.rec.State, c.rec.Reason = StateLimitReached, LimitReason
			m.touchLocked(c)
		}
		m.mu.Unlock()
		return SendResult{}, ErrLimit
	case c.rec.Agent == AgentCodex && m.opts.CodexPath == "":
		m.mu.Unlock()
		return SendResult{}, ErrCodexUnavailable
	case c.rec.Agent != AgentClaude && c.rec.Agent != AgentCodex:
		m.mu.Unlock()
		return SendResult{}, fmt.Errorf("%s chats can't run on this computer", c.rec.Agent)
	case overImageBudget(c.rec, req.Images):
		m.mu.Unlock()
		return SendResult{}, ErrImageLimit
	}
	// The images are stored before the turn starts, and the user event
	// names them.
	refs, err := m.storeImagesLocked(id, req.Images)
	if err != nil {
		m.mu.Unlock()
		return SendResult{}, err
	}

	note := Note(req.Context, c.rec.LastContext, m.opts.HostName, m.opts.RepoPath)
	m.loadLocked(c)
	firstMessage := true
	for _, ev := range c.events {
		if ev.Kind == KindUser {
			firstMessage = false
			break
		}
	}
	if _, isWalk := ParseNeedsYou(req.Text); firstMessage && !c.rec.NewEffort && isWalk {
		c.rec.Title = "Needs you"
	} else if firstMessage && !c.rec.NewEffort {
		if t := trimTitle(req.Text); t != "" {
			c.rec.Title = t
		}
	}
	// A New effort chat's first message carries the drafting prompt before
	// its note; later messages don't.
	turnNote := note
	if firstMessage && c.rec.NewEffort {
		turnNote = DraftingPrompt(strings.TrimPrefix(c.rec.Title, "New effort · ")) + "\n\n" + note
	} else if c.rec.Drafting && c.rec.Agent == AgentClaude {
		// Every later drafting message says again what drafting allows, so the
		// go-ahead marker works whatever the session remembers.
		turnNote = DraftingTurnNote + "\n\n" + note
	}
	// The walk-through: /needs-you is expanded into its prompt, and an
	// answered message carries nacred's state of the decision. Both run
	// as light turns.
	agentText := req.Text
	walk := false
	if start, ok := ParseNeedsYou(req.Text); ok {
		agentText, walk = NeedsYouPrompt(start), true
	}
	if req.Answered != "" {
		if !decisionIDRe.MatchString(req.Answered) {
			m.mu.Unlock()
			return SendResult{}, errors.New("invalid answered decision id")
		}
		turnNote = AnsweredNote(req.Answered, answeredState) + "\n\n" + turnNote
		walk = true
	}
	var ctxCopy *Context
	if req.Context != nil {
		cc := *req.Context
		ctxCopy = &cc
	}
	plan := turnPlan{note: turnNote, text: agentText, reminder: Reminder(req.Context, m.opts.RepoPath)}
	// Only this message's images are sent: the resumed session holds the
	// earlier ones.
	plan.images = m.stagedImages(id, refs)
	plan.codex = c.rec.Agent == AgentCodex
	plan.drafting = c.rec.Drafting && c.rec.Agent == AgentClaude
	if walk && c.rec.Agent == AgentClaude && !plan.drafting && m.opts.LightModel != "" && !m.rejected[m.opts.LightModel] {
		plan.forceLight, plan.lightModel = true, m.opts.LightModel
	} else if c.rec.Agent == AgentClaude && !plan.drafting && m.routingOnLocked() {
		// The router sees the history before this message, never files.
		plan.routed = true
		plan.router = RouterInput{
			Text: req.Text, Note: note, Images: len(refs), NewEffort: c.rec.NewEffort,
			History: routerHistory(c.events),
		}
		plan.lightModel = m.opts.LightModel
	}
	m.appendLocked(c, Event{Kind: KindUser, TurnID: req.TurnID, Text: req.Text, Context: ctxCopy, Images: refs, Answered: req.Answered})
	for _, r := range refs {
		c.rec.ImageCount++
		c.rec.ImageBytes += int64(r.Bytes)
	}
	c.rec.LastContext = ctxCopy
	c.rec.TurnIDs = append(c.rec.TurnIDs, req.TurnID)
	if len(c.rec.TurnIDs) > maxTurnIDs {
		c.rec.TurnIDs = c.rec.TurnIDs[len(c.rec.TurnIDs)-maxTurnIDs:]
	}
	c.rec.TurnID = req.TurnID
	c.rec.State, c.rec.Reason = StateRunning, ""

	plan.spec = claude.Chat{
		Briefing: Briefing(BriefingInput{Host: m.opts.HostName, NacredURL: m.opts.NacredURL, Phone: c.rec.Phone}),
		AddDirs:  append(append([]string{}, c.rec.AddDirs...), m.ensureMockupsDir(c.rec.ID)),
	}
	ctx, cancel := context.WithTimeout(context.Background(), TurnTimeout)
	t := &turn{id: req.TurnID, cancel: cancel}
	c.turn = t
	// Counted before the turn starts, router included, so an update can't
	// slip in between.
	m.active++
	m.touchLocked(c)
	res := SendResult{TurnID: req.TurnID, Started: true, Chat: c.rec}
	cwd, env := c.rec.Cwd, claude.ChatEnv(m.opts.NacredURL)
	if c.rec.Agent == AgentCodex {
		env = CodexEnv(m.opts.NacredURL)
	}
	env = append(m.withChatToken(env, c.rec.ID), EnvMockups+"="+m.MockupsDir(c.rec.ID))
	m.mu.Unlock()

	go m.run(ctx, c, t, cwd, env, plan)
	return res, nil
}

// turnPlan is what one message's turn runs.
type turnPlan struct {
	// spec is the full turn's argv without its session or budget, which
	// are set as each part starts.
	spec claude.Chat
	note string
	text string
	// images are this message's stored images, in order.
	images []claude.StagedImage
	// reminder is the one-line "On screen: same as before" note.
	reminder string
	// codex runs the message through codex app-server.
	codex bool
	// drafting runs a New effort chat's drafting turn: not routed, the
	// owner's model, read-only with mockup writes.
	drafting bool
	// routed asks the router first.
	routed     bool
	router     RouterInput
	// forceLight runs the light turn without asking the router: the
	// Needs you walk-through only reads.
	forceLight bool
	lightModel string
}

// Remaining is what is left of a chat's runaway ceiling after spent.
func Remaining(spent float64) float64 {
	r := claude.ChatRunawayUSD - spent
	if r < 0 {
		return 0
	}
	return r
}

// Cancel stops the chat's running turn, if any.
func (m *Manager) Cancel(id string) error {
	m.mu.Lock()
	c := m.chats[id]
	if c == nil {
		m.mu.Unlock()
		return ErrNotFound
	}
	t := c.turn
	m.mu.Unlock()
	if t != nil {
		// Pending requests are denied while the CLI still listens, then
		// the turn ends.
		t.markStopped()
		m.denyAll(c, StoppedReason, ByStop)
		t.stop()
	}
	return nil
}

// emit appends an event for the running turn.
func (m *Manager) emit(c *chatState, ev Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.appendLocked(c, ev)
}

// run is one turn, from the router call to the turn_end event: the router,
// then a light turn that may hand off to a full one, or a full turn alone.
// Stop cancels ctx, and with it whichever part is running.
func (m *Manager) run(ctx context.Context, c *chatState, t *turn, cwd string, env []string, p turnPlan) {
	defer t.cancel()
	result, runErr := m.parts(ctx, c, t, cwd, env, p)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.active--
	c.turn = nil
	if t.wasStopped() {
		m.closePendingLocked(c, StoppedReason, ByStop)
	} else {
		m.closePendingLocked(c, TurnOverReason, ByTurnEnd)
	}
	endText := "Turn finished"
	isErr := false
	switch {
	case t.wasStopped():
		endText = "Stopped"
	case runErr != nil:
		m.appendLocked(c, Event{Kind: KindError, TurnID: t.id, Text: runErr.Error()})
		endText, isErr = "Turn ended with an error", true
	case result != nil && result.IsError:
		m.appendLocked(c, Event{Kind: KindError, TurnID: t.id, Text: claude.OneLine(result.Text, 400)})
		endText, isErr = "Turn ended with an error", true
	}
	end := Event{Kind: KindTurnEnd, TurnID: t.id, Text: endText}
	if isErr {
		end.IsError = api.Bool(true)
	}
	end.CostUSD = t.cost
	m.appendLocked(c, end)
	if c.rec.CostUSD >= claude.ChatRunawayUSD-0.005 {
		c.rec.State, c.rec.Reason = StateLimitReached, LimitReason
	} else {
		c.rec.State, c.rec.Reason = StateIdle, ""
	}
	m.touchLocked(c)
}

// errLimitMidTurn ends a turn whose earlier parts spent the chat's ceiling.
var errLimitMidTurn = errors.New(LimitReason)

// parts runs the router, the light turn and the full turn as the plan and
// the router's answer say, and returns the last part's result.
func (m *Manager) parts(ctx context.Context, c *chatState, t *turn, cwd string, env []string, p turnPlan) (*claude.Result, error) {
	if p.codex {
		// Codex chats are never routed, and report no cost.
		return m.codexTurn(ctx, c, t, cwd, env, p)
	}
	if p.drafting {
		spec := m.sessionSpec(c, p.spec)
		if spec.MaxBudgetUSD < 0.005 {
			return nil, errLimitMidTurn
		}
		spec.PermissionMode, spec.AllowedTools = "default", ReadOnlyTools
		spec.Restricted, spec.Tools = true, RestrictedTools
		dt := &drafted{}
		res, err := m.exec(ctx, c, t, cwd, env, spec, p.note, p.images, p.text, RouteFull, turnMode{drafting: true, drafted: dt})
		if res != nil {
			m.addCost(c, t, res.CostUSD)
		}
		if dt.goAhead && !t.wasStopped() && (res == nil || !res.IsError) {
			m.raiseOnGoAhead(c, t.id)
		}
		return res, err
	}
	route := RouteFull
	if p.forceLight {
		route = RouteLight
	} else if p.routed {
		d := m.route(ctx, p.router, m.remaining(c))
		m.addCost(c, t, d.Cost)
		if d.Rejected {
			m.modelRejected(m.opts.RouterModel, "router")
		}
		if d.Route == RouteLight {
			route = RouteLight
		}
	}
	if t.wasStopped() {
		return nil, nil
	}
	if route == RouteLight {
		spec := m.sessionSpec(c, p.spec)
		if spec.MaxBudgetUSD < 0.005 {
			return nil, errLimitMidTurn
		}
		spec.Model, spec.PermissionMode, spec.AllowedTools = p.lightModel, "default", ReadOnlyTools
		spec.Restricted, spec.Tools = true, RestrictedTools
		lt := &light{}
		res, err := m.exec(ctx, c, t, cwd, env, spec, p.note+"\n\n"+LightAddendum, p.images, p.text, RouteLight, turnMode{light: lt})
		if res != nil {
			m.addCost(c, t, res.CostUSD)
		}
		if t.wasStopped() {
			return nil, nil
		}
		rejected := res != nil && res.IsError && claude.ModelRejected(res.Text)
		if rejected {
			m.modelRejected(p.lightModel, "light")
		}
		if lt.reason == "" && !rejected {
			return res, err
		}
		if lt.reason != "" {
			m.emit(c, Event{
				Kind: KindHandoff, TurnID: t.id, Reason: lt.reason,
				Text: HandoffText(lt.reason),
			})
		}
		// The full turn carries on in the same session: the light turn
		// already holds Ryder's message, so it gets the continuation note.
		if m.sessionHeld(c) {
			full := m.sessionSpec(c, p.spec)
			if full.MaxBudgetUSD < 0.005 {
				return nil, errLimitMidTurn
			}
			res, err := m.exec(ctx, c, t, cwd, env, full, ContinuationNote, nil, p.reminder, RouteFull, turnMode{})
			if res != nil {
				m.addCost(c, t, res.CostUSD)
			}
			return res, err
		}
	}
	full := m.sessionSpec(c, p.spec)
	if full.MaxBudgetUSD < 0.005 {
		return nil, errLimitMidTurn
	}
	res, err := m.exec(ctx, c, t, cwd, env, full, p.note, p.images, p.text, RouteFull, turnMode{})
	if res != nil {
		m.addCost(c, t, res.CostUSD)
	}
	return res, err
}

// turnMode says how one claude process of a turn answers host requests:
// a full turn (zero value) asks the phone; a light turn (light set) allows
// mockup writes and hands off anything else; a drafting turn allows mockup
// writes and refuses anything else.
type turnMode struct {
	light    *light
	drafting bool
	// drafted records what a drafting turn said: its go-ahead marker.
	drafted *drafted
}

// drafted is what a drafting turn's text asked of nacred.
type drafted struct {
	goAhead bool
}

// sessionSpec is spec for the next part of the chat's turn: the session
// started or resumed, and what is left of the ceiling as its budget.
func (m *Manager) sessionSpec(c *chatState, spec claude.Chat) claude.Chat {
	m.mu.Lock()
	defer m.mu.Unlock()
	spec.AddDirs = append([]string{}, spec.AddDirs...)
	if c.rec.SessionStarted {
		spec.Resume, spec.SessionID = c.rec.SessionID, ""
	} else {
		spec.SessionID, spec.Resume = c.rec.SessionID, ""
	}
	spec.MaxBudgetUSD = Remaining(c.rec.CostUSD)
	return spec
}

func (m *Manager) sessionHeld(c *chatState) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return c.rec.SessionStarted
}

func (m *Manager) remaining(c *chatState) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Remaining(c.rec.CostUSD)
}

// addCost adds a part's spending to the chat and the turn.
func (m *Manager) addCost(c *chatState, t *turn, usd float64) {
	if usd <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	c.rec.CostUSD += usd
	t.cost += usd
	m.touchLocked(c)
}

// exec runs one claude process of the turn and returns its result line, or
// an error when it ended without one. A light turn (lt set) hands off, by
// setting lt.reason, on a final handoff line, a host permission request or
// an ExitPlanMode call; the last two are refused and the process stopped.
func (m *Manager) exec(ctx context.Context, c *chatState, t *turn, cwd string, env []string, spec claude.Chat, note string, images []claude.StagedImage, text, route string, mode turnMode) (*claude.Result, error) {
	lt := mode.light
	mockups := m.MockupsDir(c.rec.ID)
	msg, err := claude.ChatUserMessage(note, images, text)
	if err != nil {
		return nil, err
	}
	// The light process can be stopped on its own, for a handoff.
	ctx, stopProcess := context.WithCancel(ctx)
	defer stopProcess()
	cmd := exec.CommandContext(ctx, m.opts.ClaudePath, spec.Args()...)
	cmd.Dir = cwd
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude: %w", err)
	}

	// Stdin stays open for the turn so nacred can answer host prompts on
	// the control channel; it is closed once the result arrives.
	var stdinMu sync.Mutex
	stdinOpen := true
	write := func(b []byte) {
		stdinMu.Lock()
		defer stdinMu.Unlock()
		if stdinOpen {
			_, _ = stdin.Write(b)
		}
	}
	closeStdin := func() {
		stdinMu.Lock()
		defer stdinMu.Unlock()
		if stdinOpen {
			stdinOpen = false
			_ = stdin.Close()
		}
	}
	defer closeStdin()
	write(msg)

	var (
		result *claude.Result
		tail   []string
		wg     sync.WaitGroup
		mu     sync.Mutex
	)
	// handOff refuses a light turn's request and stops the process: an
	// interrupt first, so the CLI still prints its result and cost, then a
	// kill if it doesn't.
	handOff := func(req claude.ControlRequest, reason string) {
		lt.reason, lt.stopped = reason, true
		write(claude.DenyControl(req.RequestID, HandoffMessage))
		write(claude.InterruptControl("nacre_handoff_" + req.RequestID))
		time.AfterFunc(interruptGrace, stopProcess)
	}
	wg.Add(2)
	go func() {
		defer wg.Done()
		scan(stdout, func(line []byte) {
			if req, ok := claude.ParseControlRequest(line); ok {
				switch {
				case mode.drafting:
					switch {
					case req.Subtype != "can_use_tool":
						write(claude.UnsupportedControl(req.RequestID, req.Subtype))
					case !t.wasStopped() && (mockupWrite(req, mockups) || readOnlyBash(req)):
						write(claude.AllowControl(req.RequestID, req.Input))
					default:
						write(claude.DenyControl(req.RequestID, DraftingReminder))
					}
				case lt == nil:
					m.control(c, t, req, write)
				case req.Subtype != "can_use_tool":
					write(claude.UnsupportedControl(req.RequestID, req.Subtype))
				case lt.stopped || t.wasStopped():
					write(claude.DenyControl(req.RequestID, HandoffMessage))
				case mockupWrite(req, mockups) || readOnlyBash(req):
					// A light turn may draw (a mockup is the one write it makes)
					// and run the read-only commands nacred checks itself.
					write(claude.AllowControl(req.RequestID, req.Input))
				case req.ToolName == "ExitPlanMode":
					reason := planText(req.Input)
					if reason == "" {
						reason = "the quick answer wants to leave plan mode and act"
					}
					handOff(req, reason)
				default:
					// No card: the full harness asks again if it needs to.
					handOff(req, "it needs a step the quick answer can't take ("+PermissionSummary(req)+")")
				}
				return
			}
			p, perr := claude.ParseLine(line)
			if perr != nil {
				return
			}
			if p.Init != nil && p.Init.SessionID != "" {
				m.sessionStarted(c, p.Init.SessionID)
			}
			if p.Result != nil {
				if p.Result.SessionID != "" {
					m.sessionStarted(c, p.Result.SessionID)
				}
				mu.Lock()
				result = p.Result
				mu.Unlock()
				closeStdin()
			}
			if lt != nil && lt.stopped {
				return // what the refused light turn says after the handoff
			}
			for _, ev := range p.Events {
				if ev.Kind == api.KindResult {
					continue // the turn_end event closes the turn
				}
				out := fromRunEvent(ev, t.id, route)
				if mode.drafted != nil && ev.Kind == api.KindText {
					if rest, ok := splitIntakeMarker(out.Text); ok {
						out.Text, ev.Text = rest, rest
						mode.drafted.goAhead = true
					}
				}
				var drafts []Event
				var asks []string
				if ev.Kind == api.KindText {
					rest, found := ExtractDrafts(out.Text)
					if len(found) > 0 {
						out.Text, ev.Text = rest, rest
						drafts = draftEvents(found, t.id, route)
					}
					if rest, ids := ExtractAsks(out.Text); len(ids) > 0 || rest != out.Text {
						out.Text, ev.Text = rest, rest
						asks = ids
					}
				}
				if lt != nil {
					if ev.Kind == api.KindToolUse && ev.Tool == "ExitPlanMode" {
						lt.plan = planText(ev.Input)
					}
					if ev.Kind == api.KindText {
						rest, reason, found := splitHandoff(ev.Text)
						if found {
							lt.reason = reason
							out.Text = rest
						} else if strings.TrimSpace(ev.Text) != "" {
							// Only a final line hands off; more text after
							// it means the answer went on.
							lt.reason = ""
						}
					}
				}
				if ev.Kind != api.KindText || strings.TrimSpace(out.Text) != "" {
					m.emit(c, out)
				}
				for _, d := range drafts {
					m.emit(c, d)
				}
				for _, id := range asks {
					m.recordAsk(c, id, t.id, route)
				}
			}
		})
	}()
	go func() {
		defer wg.Done()
		scan(stderr, func(line []byte) {
			s := strings.TrimSpace(string(line))
			if s == "" {
				return
			}
			mu.Lock()
			tail = append(tail, claude.Truncate(s, 400))
			if len(tail) > 10 {
				tail = tail[1:]
			}
			mu.Unlock()
		})
	}()
	wg.Wait()
	waitErr := cmd.Wait()
	mu.Lock()
	defer mu.Unlock()
	if lt != nil && lt.reason == "" && lt.plan != "" && (result == nil || !result.IsError) {
		// ExitPlanMode was called but never reached the host.
		lt.reason = lt.plan
	}
	if result != nil {
		return result, nil
	}
	if t.wasStopped() || (lt != nil && lt.stopped) {
		return nil, nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, errors.New("claude ran too long and was stopped")
	}
	msgText := "claude exited without an answer"
	var ee *exec.ExitError
	if errors.As(waitErr, &ee) {
		msgText = fmt.Sprintf("claude exited with status %d without an answer", ee.ExitCode())
	} else if waitErr != nil {
		msgText = waitErr.Error()
	}
	if len(tail) > 0 {
		msgText += ": " + tail[len(tail)-1]
	}
	return nil, errors.New(msgText)
}

// sessionStarted records that the agent has the chat's session, so the next
// turn resumes it even if this one never finishes.
func (m *Manager) sessionStarted(c *chatState, sid string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sid == c.rec.SessionID && !c.rec.SessionStarted {
		c.rec.SessionStarted = true
		m.touchLocked(c)
	}
}

// fromRunEvent turns a transcript event into a chat event of the route.
func fromRunEvent(ev api.RunEvent, turnID, route string) Event {
	out := Event{
		Kind: ev.Kind, TurnID: turnID, Text: ev.Text, Tool: ev.Tool, ToolID: ev.ToolID,
		Input: ev.Input, IsError: ev.IsError,
	}
	switch ev.Kind {
	case api.KindText, api.KindThinking, api.KindToolUse, api.KindToolResult:
		out.Route = route
	}
	return out
}

func scan(rd io.Reader, fn func([]byte)) {
	br := bufio.NewReaderSize(rd, 64*1024)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			fn(line)
		}
		if err != nil {
			return
		}
	}
}

// workDir is where an across-boards chat runs.
func (m *Manager) workDir() string { return filepath.Join(m.opts.Dir, "work") }
