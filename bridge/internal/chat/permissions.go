package chat

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// PermissionTimeout is how long a host permission request waits for the
// phone before it is denied.
const PermissionTimeout = 15 * time.Minute

// Deny messages the agent sees when nacred, not Ryder, decides a request.
const (
	NoAnswerReason  = "No answer from the phone within 15 minutes"
	StoppedReason   = "Ryder stopped the turn before answering"
	RestartReason   = "nacred restarted before this was answered"
	TurnOverReason  = "The turn ended before this was answered"
	DeniedNoNote    = "Ryder denied this step from the phone"
	PairedDeviceMsg = "Answer from a paired device"
)

// Who decided a permission request, on its permission_answer event.
const (
	ByPhone   = "phone"
	ByTimeout = "timeout"
	ByStop    = "stop"
	ByRestart = "restart"
	ByTurnEnd = "turn_end"
)

// KindPermissionAnswer records how a permission request was decided. Its
// request_id names the permission event, allowed the outcome, by who decided
// and reason the deny message the agent saw.
const KindPermissionAnswer = "permission_answer"

var (
	// ErrDecided is an answer to a request that was already decided.
	ErrDecided = errors.New("This request was already answered.")
	// ErrUnknownRequest names no permission request of the chat.
	ErrUnknownRequest = errors.New("unknown permission request")
)

// pending is a host permission request waiting for the phone.
type pending struct {
	id    string
	turn  *turn
	input json.RawMessage
	write func([]byte)
	timer *time.Timer
}

func (m *Manager) permissionTimeout() time.Duration {
	if m.opts.PermissionTimeout > 0 {
		return m.opts.PermissionTimeout
	}
	return PermissionTimeout
}

// PermissionSummary is the one-line name of a step: the command, the file
// path, or the MCP tool and its server.
func PermissionSummary(req claude.ControlRequest) string {
	tool := req.ToolName
	if rest, ok := strings.CutPrefix(tool, "mcp__"); ok {
		server, name, found := strings.Cut(rest, "__")
		if found {
			return "MCP tool " + name + " on " + server
		}
	}
	summary := claude.ToolSummary(tool, req.Input)
	if summary == "" {
		summary = claude.OneLine(req.Description, 200)
	}
	if summary == "" {
		return tool
	}
	return tool + ": " + summary
}

// control answers a control request from the CLI. A permission request is
// recorded, the chat waits for the phone, and the answer goes back on stdin.
func (m *Manager) control(c *chatState, t *turn, req claude.ControlRequest, write func([]byte)) {
	if req.Subtype != "can_use_tool" {
		write(claude.UnsupportedControl(req.RequestID, req.Subtype))
		return
	}
	m.mu.Lock()
	if t.wasStopped() || c.turn != t || c.deleted {
		m.mu.Unlock()
		write(claude.DenyControl(req.RequestID, StoppedReason))
		return
	}
	if c.pending == nil {
		c.pending = map[string]*pending{}
	}
	if _, dup := c.pending[req.RequestID]; dup {
		m.mu.Unlock()
		return
	}
	m.appendLocked(c, Event{
		Kind: KindPermission, TurnID: t.id, RequestID: req.RequestID, Tool: req.ToolName,
		Text: PermissionSummary(req), Reason: req.Reason, Input: req.Input,
	})
	p := &pending{id: req.RequestID, turn: t, input: req.Input, write: write}
	c.pending[req.RequestID] = p
	c.rec.State = StateWaitingPermission
	m.touchLocked(c)
	id := req.RequestID
	p.timer = time.AfterFunc(m.permissionTimeout(), func() {
		m.decide(c, id, false, NoAnswerReason, ByTimeout)
	})
	m.mu.Unlock()
}

// decide settles a pending request and writes the answer to the CLI.
func (m *Manager) decide(c *chatState, reqID string, allow bool, message, by string) (Event, error) {
	m.mu.Lock()
	p := c.pending[reqID]
	if p == nil {
		ev, err := m.decidedLocked(c, reqID)
		m.mu.Unlock()
		return ev, err
	}
	ev := m.settleLocked(c, p, allow, message, by)
	m.mu.Unlock()
	if allow {
		p.write(claude.AllowControl(p.id, p.input))
	} else {
		p.write(claude.DenyControl(p.id, message))
	}
	return ev, nil
}

// settleLocked records the outcome of p and drops it from the chat.
func (m *Manager) settleLocked(c *chatState, p *pending, allow bool, message, by string) Event {
	delete(c.pending, p.id)
	if p.timer != nil {
		p.timer.Stop()
	}
	text := "Allowed"
	if !allow {
		text = "Denied: " + message
	}
	ev := Event{Kind: KindPermissionAnswer, TurnID: p.turn.id, RequestID: p.id, Allowed: api.Bool(allow), Text: text, By: by}
	if !allow {
		ev.Reason = message
	}
	ev = m.appendLocked(c, ev)
	if len(c.pending) == 0 && c.rec.State == StateWaitingPermission && c.turn == p.turn {
		c.rec.State = StateRunning
	}
	m.touchLocked(c)
	return ev
}

// decidedLocked finds how reqID was decided: ErrDecided with its answer
// event, or ErrUnknownRequest.
func (m *Manager) decidedLocked(c *chatState, reqID string) (Event, error) {
	m.loadLocked(c)
	asked := false
	for i := len(c.events) - 1; i >= 0; i-- {
		ev := c.events[i]
		if ev.RequestID != reqID {
			continue
		}
		switch ev.Kind {
		case KindPermissionAnswer:
			return ev, ErrDecided
		case KindPermission:
			asked = true
			if ev.Allowed != nil { // decided before answers were their own events
				return ev, ErrDecided
			}
		}
	}
	if asked {
		return Event{}, ErrDecided
	}
	return Event{}, ErrUnknownRequest
}

// Answer is the phone's answer to a chat's permission request. A request
// that is no longer pending gives ErrDecided with the recorded outcome.
func (m *Manager) Answer(id, reqID string, allow bool, note string) (Event, error) {
	m.mu.Lock()
	c := m.chats[id]
	m.mu.Unlock()
	if c == nil {
		return Event{}, ErrNotFound
	}
	message := ""
	if !allow {
		message = strings.TrimSpace(note)
		if len(message) > 2000 {
			message = message[:2000]
		}
		if message == "" {
			message = DeniedNoNote
		}
	}
	return m.decide(c, reqID, allow, message, ByPhone)
}

// denyAll denies every pending request of the chat, writing the denial to
// the CLI while it still listens.
func (m *Manager) denyAll(c *chatState, message, by string) {
	m.mu.Lock()
	ids := make([]string, 0, len(c.pending))
	for id := range c.pending {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		_, _ = m.decide(c, id, false, message, by)
	}
}

// closePendingLocked records every pending request as denied without
// writing to the CLI, which has gone.
func (m *Manager) closePendingLocked(c *chatState, message, by string) {
	for _, p := range c.pending {
		m.settleLocked(c, p, false, message, by)
	}
}

// closeUnansweredLocked records permission requests in the log that never
// got an answer as denied, for a chat a previous nacred left running.
func (m *Manager) closeUnansweredLocked(c *chatState, message, by string) {
	m.loadLocked(c)
	open := map[string]Event{}
	var order []string
	for _, ev := range c.events {
		switch {
		case ev.Kind == KindPermission && ev.Allowed == nil && ev.RequestID != "":
			if _, seen := open[ev.RequestID]; !seen {
				order = append(order, ev.RequestID)
			}
			open[ev.RequestID] = ev
		case ev.Kind == KindPermissionAnswer:
			delete(open, ev.RequestID)
		}
	}
	for _, id := range order {
		ev, ok := open[id]
		if !ok {
			continue
		}
		m.appendLocked(c, Event{
			Kind: KindPermissionAnswer, TurnID: ev.TurnID, RequestID: id, Allowed: api.Bool(false),
			Text: "Denied: " + message, Reason: message, By: by,
		})
	}
}
