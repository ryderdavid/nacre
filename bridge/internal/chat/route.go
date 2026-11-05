package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// KindHandoff records that a light turn handed the message to the full
// harness. Its text is "Handed to the full harness: <reason>" and its reason
// the reason alone.
const KindHandoff = "handoff"

const (
	// RouterBudgetUSD caps one routing call.
	RouterBudgetUSD = 0.10
	// RouterTimeout bounds one routing call; past it the message goes full.
	RouterTimeout = 15 * time.Second
	// routerHistoryMessages is how many earlier messages the router sees.
	routerHistoryMessages = 6
	// routerHistoryBytes bounds the history the router sees.
	routerHistoryBytes = 2048
	// interruptGrace is how long a stopped light turn may take to print its
	// result (and its cost) before it is killed.
	interruptGrace = 10 * time.Second
)

// HandoffMessage is what the light turn is told when nacred refuses its
// permission request or its ExitPlanMode call.
const HandoffMessage = "Handing this to the full harness"

// handoffPrefix starts the last line of a light answer that needs more.
const handoffPrefix = "NACRE_HANDOFF:"

// LightAddendum is added to a light turn's "On screen" note.
const LightAddendum = "You are answering in read-only plan mode with a lighter model. If reading is enough, answer. " +
	"If the message needs anything more (editing files, changing beads, changes or rounds, running builds or tests, sending things, or leaving plan mode), " +
	"don't attempt it: end your reply with exactly one last line `" + handoffPrefix + " <one-line reason>` and the full harness carries on in this same session."

// ContinuationNote is the whole input of the full turn a handoff starts,
// before its one-line "On screen" reminder.
const ContinuationNote = "Continue with the full harness. Carry out Ryder's last message above."

// RouterSchema is the routing call's --json-schema.
var RouterSchema = json.RawMessage(`{"type":"object","properties":{"route":{"type":"string","enum":["light","full"]},"reason":{"type":"string"}},"required":["route","reason"],"additionalProperties":false}`)

// RouterSystem is the routing call's system prompt.
const RouterSystem = "You route messages Ryder sends to Nacre's Chat, a Claude Code session on his computer. You have no tools and see no files. " +
	"Answer with route \"light\" or \"full\" and a one-line reason.\n\n" +
	"- light: a plain question that only needs reading and answering: explaining, summarising, looking something up in the repository or in bd, or talking an idea through (including a New effort draft).\n" +
	"- full: anything that changes files, beads, OpenSpec changes or rounds; runs builds, tests or installs; sends, pushes, lands or deploys anything; or asks to carry on with work (\"do it\", \"go ahead\", \"fix it\").\n\n" +
	"When unsure, choose full."

// RouterInput is everything the routing model sees. It never sees files,
// tools, images or the session.
type RouterInput struct {
	Text string
	// Note is the message's "On screen" note.
	Note string
	// Images is how many images are attached; the router never sees them.
	Images    int
	NewEffort bool
	// History is the last six user and assistant messages, oldest first,
	// trimmed to 2 KB.
	History []HistoryMessage
}

// HistoryMessage is one earlier message for the router.
type HistoryMessage struct {
	Role string // "Ryder" or "Assistant"
	Text string
}

// Prompt is the routing call's prompt.
func (in RouterInput) Prompt() string {
	var b strings.Builder
	kind := "an ordinary chat"
	if in.NewEffort {
		kind = "a New effort chat (drafting a new piece of work with Ryder before intake)"
	}
	b.WriteString("This is " + kind + ".\n")
	fmt.Fprintf(&b, "Images attached to the new message: %d\n", in.Images)
	b.WriteString("\nRecent messages, oldest first:\n")
	if len(in.History) == 0 {
		b.WriteString("(none)\n")
	}
	for _, h := range in.History {
		b.WriteString(h.Role + ": " + h.Text + "\n")
	}
	b.WriteString("\n" + in.Note + "\n\nNew message:\n" + in.Text + "\n")
	return b.String()
}

// routerHistory is the last six user and assistant messages before the one
// being sent, trimmed to 2 KB, oldest first. The assistant texts of one turn
// make one message.
func routerHistory(events []Event) []HistoryMessage {
	var msgs []HistoryMessage
	lastTurn := ""
	for _, ev := range events {
		switch {
		case ev.Kind == KindUser:
			msgs = append(msgs, HistoryMessage{Role: "Ryder", Text: ev.Text})
			lastTurn = ""
		case ev.Kind == "text" && strings.TrimSpace(ev.Text) != "":
			if n := len(msgs); n > 0 && msgs[n-1].Role == "Assistant" && lastTurn == ev.TurnID {
				msgs[n-1].Text += "\n" + ev.Text
			} else {
				msgs = append(msgs, HistoryMessage{Role: "Assistant", Text: ev.Text})
			}
			lastTurn = ev.TurnID
		}
	}
	if len(msgs) > routerHistoryMessages {
		msgs = msgs[len(msgs)-routerHistoryMessages:]
	}
	return trimHistory(msgs, routerHistoryBytes)
}

// trimHistory keeps the newest messages within limit bytes of text; the
// oldest one kept is cut from its start to fit.
func trimHistory(msgs []HistoryMessage, limit int) []HistoryMessage {
	var out []HistoryMessage
	left := limit
	for i := len(msgs) - 1; i >= 0 && left > 0; i-- {
		m := msgs[i]
		if len(m.Text) > left {
			cut := m.Text[len(m.Text)-left:]
			m.Text = strings.ToValidUTF8(cut, "")
		}
		left -= len(m.Text)
		out = append([]HistoryMessage{m}, out...)
	}
	return out
}

// routeDecision is what the router chose.
type routeDecision struct {
	Route  string
	Reason string
	Cost   float64
	// Rejected is set when the CLI rejected the router model.
	Rejected bool
}

// routerHeadless is the routing call.
func (m *Manager) routerHeadless(in RouterInput, remaining float64) claude.Headless {
	budget := RouterBudgetUSD
	if remaining < budget {
		budget = remaining
	}
	return claude.Headless{
		Prompt: in.Prompt(), System: RouterSystem, Schema: RouterSchema,
		Model: m.opts.RouterModel, MaxBudgetUSD: budget, Timeout: m.routerTimeout(),
	}
}

func (m *Manager) routerTimeout() time.Duration {
	if m.opts.RouterTimeout > 0 {
		return m.opts.RouterTimeout
	}
	return RouterTimeout
}

// route asks the routing model. Any error, timeout, cost cap or malformed
// answer is full.
func (m *Manager) route(ctx context.Context, in RouterInput, remaining float64) routeDecision {
	h := m.routerHeadless(in, remaining)
	ctx, cancel := context.WithTimeout(ctx, h.Timeout)
	defer cancel()
	dir := filepath.Join(m.opts.Dir, "router")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return routeDecision{Route: RouteFull, Reason: err.Error()}
	}
	cmd := exec.CommandContext(ctx, m.opts.ClaudePath, h.Args()...)
	cmd.Dir = dir
	cmd.Env = claude.RunEnv()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.Output()
	var result *claude.Result
	for _, line := range strings.Split(string(out), "\n") {
		if p, perr := claude.ParseLine([]byte(line)); perr == nil && p.Result != nil {
			result = p.Result
		}
	}
	d := routeDecision{Route: RouteFull}
	if result != nil {
		d.Cost = result.CostUSD
	}
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		d.Reason = "the router took longer than " + h.Timeout.String()
	case result == nil:
		d.Reason = "the router gave no answer"
		if err != nil {
			d.Reason += ": " + err.Error()
		}
	case result.IsError:
		d.Reason = "the router failed: " + claude.OneLine(result.Text, 200)
		d.Rejected = claude.ModelRejected(result.Text)
	default:
		var ans struct {
			Route  string `json:"route"`
			Reason string `json:"reason"`
		}
		if json.Unmarshal(result.Structured, &ans) != nil || (ans.Route != RouteLight && ans.Route != RouteFull) {
			d.Reason = "the router's answer was malformed"
			break
		}
		d.Route, d.Reason = ans.Route, ans.Reason
	}
	return d
}

// splitHandoff removes a final `NACRE_HANDOFF: <reason>` line from a light
// answer's text. found is set when the text's last non-empty line is one.
func splitHandoff(text string) (rest, reason string, found bool) {
	trimmed := strings.TrimRight(text, " \t\r\n")
	i := strings.LastIndexByte(trimmed, '\n')
	last := strings.TrimSpace(trimmed[i+1:])
	if !strings.HasPrefix(last, handoffPrefix) {
		return text, "", false
	}
	reason = strings.TrimSpace(strings.TrimPrefix(last, handoffPrefix))
	if reason == "" {
		reason = "the quick answer needs more"
	}
	if i < 0 {
		return "", reason, true
	}
	return strings.TrimRight(trimmed[:i], " \t\r\n"), reason, true
}

// planText is ExitPlanMode's plan, from its input.
func planText(input json.RawMessage) string {
	var in struct {
		Plan string `json:"plan"`
	}
	_ = json.Unmarshal(input, &in)
	return strings.TrimSpace(in.Plan)
}

// light is a light turn's handoff state, shared by its stdout reader and
// control handler.
type light struct {
	// reason is set once the turn hands off.
	reason string
	// stopped is set once nacred has stopped the light process.
	stopped bool
	// plan is the last ExitPlanMode plan seen, for a handoff when the call
	// never reached the host.
	plan string
}

// modelRejected remembers, once, that the CLI rejected a chat model; later
// messages then go straight to full.
func (m *Manager) modelRejected(model, what string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rejected == nil {
		m.rejected = map[string]bool{}
	}
	if m.rejected[model] {
		return
	}
	m.rejected[model] = true
	m.opts.Logger.Printf("chat: Claude Code rejected the %s model %q; Claude chat messages now go straight to the full harness", what, model)
}

// routingOn reports whether Claude chat messages are routed: both models
// are set and the CLI has rejected neither.
func (m *Manager) routingOnLocked() bool {
	r, l := m.opts.RouterModel, m.opts.LightModel
	return r != "" && l != "" && !m.rejected[r] && !m.rejected[l]
}
