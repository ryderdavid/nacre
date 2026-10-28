// Package peer speaks Claude Code's session-to-session messaging protocol
// (peerProtocol 1), so nacred can hand a follow-up to a live interactive
// session it adopted instead of forking it with `claude -p --resume`.
//
// The protocol is undocumented and was read off Claude Code 2.1.309/2.1.310
// (spike NAC-j5z.12); everything here is guarded so that a session that does
// not look exactly like the ones tested keeps the adopted run read-only.
//
// Wire format: NDJSON over an AF_UNIX stream socket at the session record's
// messagingSocketPath. One connection per send: connect, write the lines,
// half-close. Nothing comes back on that connection; the receiver dials the
// sender's own inbox (a socket the sender binds in the same directory, named
// <sender pid>.sock) with peer_message_status receipts and peer_idle_notice.
// The receiver checks that the inbox is bound by the process that sent, so
// the inbox and the sends must both live in nacred's own process.
package peer

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"slices"
)

// Protocol is the only peerProtocol nacred speaks.
const Protocol = 1

// FeatureNotifyIdle is the peerFeatures entry that means the session honours
// notify_when_idle.
const FeatureNotifyIdle = "notify_idle"

// EntrypointDesktop marks a session hosted by the Claude desktop app. Those
// publish a socket too but were not tested, so they stay read-only.
const EntrypointDesktop = "claude-desktop"

// KindInteractive is the session-record kind of a terminal session.
const KindInteractive = "interactive"

// Receipt statuses carried by peer_message_status.
const (
	StatusHeld      = "held"
	StatusDenied    = "denied"
	StatusExpired   = "expired"
	StatusDelivered = "delivered"
	StatusRefused   = "refused"
	StatusDropped   = "dropped"
)

// Control actions.
const (
	ActionNotifyWhenIdle = "notify_when_idle"
	ActionIdleNotice     = "peer_idle_notice"
	ActionMessageStatus  = "peer_message_status"
)

// Target is what a session record says about its messaging socket. It is
// filled from sessions.Record; the guard decides from it alone, plus the
// liveness and connect checks Check runs.
type Target struct {
	PID        int
	ProcStart  string // as the record wrote it
	Kind       string
	Entrypoint string
	PIDDomain  string
	SocketPath string
	Protocol   int
	Features   []string
}

// NotifyIdle reports whether the session advertises notify_when_idle.
func (t Target) NotifyIdle() bool { return slices.Contains(t.Features, FeatureNotifyIdle) }

// HostDomain is the pidDomain a session on this machine records: Claude Code
// writes the platform name ("darwin", "linux").
func HostDomain() string { return runtime.GOOS }

// Ineligible is why a session cannot be sent a live follow-up. Its text is
// shown to the user as is.
type Ineligible struct{ Reason string }

func (e *Ineligible) Error() string { return "live send unavailable: " + e.Reason }

func ineligible(format string, args ...any) error {
	return &Ineligible{Reason: fmt.Sprintf(format, args...)}
}

// CheckRecord is the part of the version guard that the record answers on its
// own: an interactive terminal session on this host that speaks peerProtocol 1
// and published a socket. host is HostDomain() outside tests.
func CheckRecord(t Target, host string) error {
	switch {
	case t.Kind != KindInteractive:
		return ineligible("not an interactive session (kind %q)", t.Kind)
	case t.Entrypoint == EntrypointDesktop:
		return ineligible("sessions in the Claude desktop app are not supported yet")
	case t.PIDDomain != host:
		return ineligible("session is not on this computer (pidDomain %q)", t.PIDDomain)
	case t.SocketPath == "":
		return ineligible("session has no messaging socket")
	case t.Protocol != Protocol:
		return ineligible("unsupported peer protocol %d (nacred speaks %d)", t.Protocol, Protocol)
	case t.PID <= 0 || t.ProcStart == "":
		return ineligible("session record has no pid or process start time")
	}
	return nil
}

// Frame is one NDJSON line, in either direction. Only the fields nacred uses
// are declared; unknown ones are ignored.
type Frame struct {
	Type    string   `json:"type"`
	Token   string   `json:"token,omitempty"`
	Message *Message `json:"message,omitempty"`
	Action  string   `json:"action,omitempty"`
	From    string   `json:"from,omitempty"`
	MsgID   string   `json:"msg_id,omitempty"`

	// Inbound control frames.
	OrigMsgID     string   `json:"orig_msg_id,omitempty"`
	Status        string   `json:"status,omitempty"`
	Reason        string   `json:"reason,omitempty"`
	DropReason    string   `json:"drop_reason,omitempty"`
	DroppedMsgIDs []string `json:"dropped_msg_ids,omitempty"`
	State         string   `json:"state,omitempty"`
	FinishedAt    int64    `json:"finished_at,omitempty"`
	MsgV          int      `json:"msgV,omitempty"`
}

// Message is the body of a user frame.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Send describes one follow-up: the frames EncodeSend writes.
type Send struct {
	// Token is the session's peerToken; "" sends no auth line, which macOS
	// and Linux accept.
	Token string
	// From is the reply address, "uds:<inbox path>".
	From    string
	MsgID   string
	Content string
	// IdleMsgID, when set, adds a notify_when_idle subscription with this id.
	IdleMsgID string
}

// EncodeSend renders s as the NDJSON lines of one connection: an optional
// auth line, the user frame, and an optional notify_when_idle.
func EncodeSend(s Send) ([]byte, error) {
	if s.From == "" || s.MsgID == "" {
		return nil, errors.New("peer: send needs a reply address and a msg_id")
	}
	frames := make([]Frame, 0, 3)
	if s.Token != "" {
		frames = append(frames, Frame{Type: "auth", Token: s.Token})
	}
	frames = append(frames, Frame{Type: "user", Message: &Message{Role: "user", Content: s.Content}, From: s.From, MsgID: s.MsgID})
	if s.IdleMsgID != "" {
		frames = append(frames, Frame{Type: "control", Action: ActionNotifyWhenIdle, From: s.From, MsgID: s.IdleMsgID})
	}
	var out []byte
	for _, f := range frames {
		line, err := json.Marshal(f)
		if err != nil {
			return nil, err
		}
		out = append(out, line...)
		out = append(out, '\n')
	}
	return out, nil
}

// DecodeFrame parses one inbound line.
func DecodeFrame(line []byte) (Frame, error) {
	var f Frame
	if err := json.Unmarshal(line, &f); err != nil {
		return Frame{}, err
	}
	if f.Type == "" {
		return Frame{}, errors.New("peer: frame has no type")
	}
	return f, nil
}

// MsgIDs returns every msg_id a receipt is about: orig_msg_id and, for a
// dropped batch, dropped_msg_ids.
func (f Frame) MsgIDs() []string {
	var out []string
	if f.OrigMsgID != "" {
		out = append(out, f.OrigMsgID)
	}
	for _, id := range f.DroppedMsgIDs {
		if id != "" && id != f.OrigMsgID {
			out = append(out, id)
		}
	}
	return out
}

// NewMsgID returns a random UUID (version 4) for a frame's msg_id.
func NewMsgID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
