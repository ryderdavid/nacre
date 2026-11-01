package runs

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/peer"
	"github.com/ryderdavid/nacre/bridge/internal/sessions"
)

// Live sends (NAC-j5z.15) hand a follow-up to a live adopted Claude Code
// session as a peer message over its messaging socket, instead of forking it
// with `claude -p --resume`. The session queues it as a turn of its own, shown
// to the person at the terminal, run under that session's permission mode;
// it cannot answer a pending permission prompt or run a slash command.
//
// Everything here is behind the version guard in internal/peer. A run whose
// session fails it, or a send that errors, keeps the adopted run read-only:
// POST .../live answers 409 and POST .../followup is untouched (always 409).

// DefaultLiveConfirmAfter is how long a live send waits for a receipt or its
// transcript entry before it is reported unconfirmed.
const DefaultLiveConfirmAfter = 2 * time.Minute

// maxPeerMessages bounds the live sends an adopted run remembers.
const maxPeerMessages = 20

// LiveSender delivers one follow-up into a live session, running the whole
// guard first. *peer.Client implements it.
type LiveSender interface {
	Send(t peer.Target, content string) (peer.Sent, error)
}

// peerRef is what a msg_id belongs to: a run, and the live send (by its own
// msg_id) in that run. idle marks a notify_when_idle subscription's id.
type peerRef struct {
	run  string
	msg  string
	idle bool
}

// LivePrefix is what a live send's text is prefixed with, so the person at
// the terminal and the model both see who sent it and from where.
func LivePrefix(author string) string {
	author = strings.TrimSpace(author)
	if author == "" {
		return "Sent via Nacre: "
	}
	return author + " (via Nacre): "
}

// foldPeer re-derives whether s can be sent a live follow-up, and settles the
// run's live sends against the transcript: a send is confirmed once a
// peer-origin user entry with its msg_id appears, and reported unconfirmed if
// nothing has been heard within LiveConfirmAfter. It reports whether anything
// changed. Caller holds r.mu.
func (m *Manager) foldPeer(r *run, s sessions.Session, now time.Time) bool {
	changed := false
	can, why := false, ""
	r.peer = nil
	if r.meta.Agent == "" || r.meta.Agent == api.AgentClaudeCode {
		t := s.Peer()
		switch err := peer.CheckRecord(t, m.opts.LiveHost); {
		case m.opts.Live == nil:
			why = "live sends are not enabled in nacred"
		case err != nil:
			why = reason(err)
		case !peer.SocketExists(t.SocketPath):
			why = "the session's messaging socket is missing"
		default:
			can, r.peer = true, &t
		}
	}
	if can != r.meta.CanSendLive || why != r.meta.LiveSendUnavailable {
		r.meta.CanSendLive, r.meta.LiveSendUnavailable = can, why
		changed = true
	}

	if len(r.meta.PeerMessages) == 0 {
		return changed
	}
	seen := s.TranscriptState.PeerMsgIDs
	msgs := slices.Clone(r.meta.PeerMessages)
	for i := range msgs {
		pm := &msgs[i]
		if pm.Confirmed {
			continue
		}
		if slices.Contains(seen, pm.MsgID) {
			pm.Confirmed, pm.ConfirmedAt = true, now.UTC().Format(time.RFC3339)
			r.addEvent(api.RunEvent{Kind: api.KindSystem, Text: "delivered: your message is in the session's transcript"})
			changed = true
			continue
		}
		if pm.Status == api.PeerSent {
			sent, err := time.Parse(time.RFC3339, pm.SentAt)
			if err != nil || now.Sub(sent) > m.opts.LiveConfirmAfter {
				pm.Status = api.PeerUnconfirmed
				r.addEvent(api.RunEvent{Kind: api.KindSystem, IsError: api.Bool(true),
					Text: "not confirmed: the session has not acknowledged your message or shown it in its transcript. nacred does not resend; check the terminal on the Mac."})
				changed = true
			}
		}
	}
	if changed {
		r.meta.PeerMessages = msgs
	}
	return changed
}

// reason is an Ineligible's own text, or the error's.
func reason(err error) string {
	var in *peer.Ineligible
	if errors.As(err, &in) {
		return in.Reason
	}
	return err.Error()
}

// SendLive hands message to the live session of adopted run id as a peer
// message, prefixed with LivePrefix(author). It is refused with a conflict —
// and the run left as it was — when the run is not a live adopted session
// that passes the guard, or when the send fails.
func (m *Manager) SendLive(id, message, author string) (api.Run, error) {
	r := m.get(id)
	if r == nil {
		return api.Run{}, ErrNotFound
	}
	r.mu.Lock()
	switch {
	case !r.meta.Adopted():
		r.mu.Unlock()
		return api.Run{}, conflict("run was started by nacred; send it a follow-up instead")
	case !r.meta.Active():
		r.mu.Unlock()
		return api.Run{}, conflict("the session has ended; it is read-only now")
	case m.opts.Live == nil || r.peer == nil || !r.meta.CanSendLive:
		why := r.meta.LiveSendUnavailable
		if why == "" {
			why = "the session cannot take live messages"
		}
		r.mu.Unlock()
		return api.Run{}, conflict("live send unavailable: " + why + "; the run stays read-only")
	}
	t := *r.peer
	r.mu.Unlock()

	// Connecting runs the guard again (liveness, connect probe), without locks.
	sent, err := m.opts.Live.Send(t, LivePrefix(author)+message)
	if err != nil {
		var in *peer.Ineligible
		if errors.As(err, &in) {
			return api.Run{}, conflict("live send unavailable: " + in.Reason + "; the run stays read-only")
		}
		m.opts.Logger.Printf("runs: live send to %s (pid %d): %v", id, t.PID, err)
		return api.Run{}, conflict("live send failed: " + err.Error() + "; the run stays read-only")
	}

	r.mu.Lock()
	msgs := append(slices.Clone(r.meta.PeerMessages), api.PeerMessage{
		MsgID: sent.MsgID, Text: message, SentAt: now(), Status: api.PeerSent,
	})
	if len(msgs) > maxPeerMessages {
		msgs = msgs[len(msgs)-maxPeerMessages:]
	}
	r.meta.PeerMessages = msgs
	r.addEvent(api.RunEvent{Kind: api.KindUser, Text: message})
	r.addEvent(api.RunEvent{Kind: api.KindSystem, Text: "sent to the live session as a peer message. It shows in the terminal on the Mac and runs as a turn there; it cannot approve prompts or run slash commands."})
	r.changed()
	_ = r.save()
	snap := r.meta
	r.mu.Unlock()

	m.mu.Lock()
	m.peerMsgs[sent.MsgID] = peerRef{run: id, msg: sent.MsgID}
	if sent.IdleMsgID != "" {
		m.peerMsgs[sent.IdleMsgID] = peerRef{run: id, msg: sent.MsgID, idle: true}
	}
	m.notify(snap)
	m.mu.Unlock()
	return snap, nil
}

// PeerFrame takes a frame a session sent to nacred's inbox: a receipt for a
// live send (peer_message_status) or the idle notice it subscribed to. Frames
// about msg_ids nacred does not know are ignored.
func (m *Manager) PeerFrame(f peer.Frame) {
	if f.Type != "control" || (f.Action != peer.ActionMessageStatus && f.Action != peer.ActionIdleNotice) {
		return
	}
	m.mu.Lock()
	var refs []peerRef
	for _, id := range f.MsgIDs() {
		if ref, ok := m.peerMsgs[id]; ok {
			refs = append(refs, ref)
			if f.Action == peer.ActionIdleNotice {
				delete(m.peerMsgs, id) // one-shot
			}
		}
	}
	runs := make([]*run, len(refs))
	for i, ref := range refs {
		runs[i] = m.runs[ref.run]
	}
	m.mu.Unlock()

	for i, r := range runs {
		if r == nil {
			continue
		}
		r.mu.Lock()
		if applyFrame(r, refs[i], f) {
			r.changed()
			_ = r.save()
			snap := r.meta
			r.mu.Unlock()
			m.notify(snap)
			continue
		}
		r.mu.Unlock()
	}
}

// applyFrame folds one inbound frame into the live send ref names. Caller
// holds r.mu.
func applyFrame(r *run, ref peerRef, f peer.Frame) bool {
	i := slices.IndexFunc(r.meta.PeerMessages, func(pm api.PeerMessage) bool { return pm.MsgID == ref.msg })
	if i < 0 {
		return false
	}
	msgs := slices.Clone(r.meta.PeerMessages)
	pm := &msgs[i]
	if f.Action == peer.ActionIdleNotice {
		if !ref.idle {
			return false
		}
		at := time.Now()
		if f.FinishedAt > 0 {
			at = time.UnixMilli(f.FinishedAt)
		}
		pm.IdleAt = at.UTC().Format(time.RFC3339)
		r.meta.PeerMessages = msgs
		r.addEvent(api.RunEvent{Kind: api.KindSystem, Text: "the session went idle after your message"})
		return true
	}
	if ref.idle || f.Status == "" {
		return false
	}
	pm.Status = f.Status
	pm.Reason = strings.TrimSpace(f.Reason)
	if pm.Reason == "" {
		pm.Reason = strings.TrimSpace(f.DropReason)
	}
	r.meta.PeerMessages = msgs
	text, bad := receiptText(f.Status, pm.Reason)
	ev := api.RunEvent{Kind: api.KindSystem, Text: text}
	if bad {
		ev.IsError = api.Bool(true)
	}
	r.addEvent(ev)
	return true
}

// receiptText says what a peer_message_status means for the person who sent
// the message, and whether it is bad news.
func receiptText(status, why string) (string, bool) {
	var text string
	bad := false
	switch status {
	case peer.StatusHeld:
		text = "held: the session is waiting for its user to approve your message on the Mac"
	case peer.StatusDelivered:
		text = "accepted: the session queued your message"
	case peer.StatusDenied:
		text, bad = "denied: the session's user declined your message", true
	case peer.StatusExpired:
		text, bad = "expired: your message was not approved in time", true
	case peer.StatusRefused:
		text, bad = "refused: the session does not accept messages from other sessions", true
	case peer.StatusDropped:
		text, bad = "dropped: the session discarded your message", true
	default:
		text = "the session reported " + status
	}
	if why != "" {
		text += " (" + why + ")"
	}
	return text, bad
}
