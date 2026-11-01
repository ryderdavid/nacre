package runs

import (
	"errors"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/peer"
	"github.com/ryderdavid/nacre/bridge/internal/sessions"
)

// fakeLive records sends instead of dialling a session.
type fakeLive struct {
	mu    sync.Mutex
	sends []string
	err   error
	n     int
}

func (f *fakeLive) Send(t peer.Target, content string) (peer.Sent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return peer.Sent{}, f.err
	}
	f.n++
	f.sends = append(f.sends, content)
	id := "msg-" + string(rune('0'+f.n))
	return peer.Sent{MsgID: id, IdleMsgID: id + "-idle", From: "uds:/tmp/cc-socks/1.sock"}, nil
}

// liveSocket binds a socket so the record's messagingSocketPath exists.
func liveSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "naclive")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "4242.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return path
}

// liveSession is an adopted terminal session that passes the record guard.
func liveSession(t *testing.T, sid string) sessions.Session {
	s := agentSession(sid, "NAC-7")
	s.Entrypoint = "cli"
	s.Kind = peer.KindInteractive
	s.ProcStart = "Sat Oct 24 00:25:54 2026"
	s.PIDDomain = "darwin"
	s.MessagingSocketPath = liveSocket(t)
	s.PeerProtocol = 1
	s.PeerFeatures = []string{peer.FeatureNotifyIdle}
	return s
}

func liveManager(t *testing.T, live LiveSender) *Manager {
	t.Helper()
	m, err := New(Options{Dir: t.TempDir(), ClaudePath: "/nonexistent", MaxConcurrent: 1,
		Live: live, LiveHost: "darwin", LiveConfirmAfter: time.Minute,
		Logger: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func adoptedRun(t *testing.T, m *Manager) api.Run {
	t.Helper()
	list := m.List(Filter{IncludeAdopted: true, IncludeStale: true})
	if len(list) != 1 {
		t.Fatalf("adopted runs = %+v", list)
	}
	return list[0]
}

func TestLiveSendOfferedOnlyWhenTheGuardPasses(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*sessions.Session)
		want   string // "" = offered
	}{
		{"tested shape", func(*sessions.Session) {}, ""},
		{"unknown peer protocol", func(s *sessions.Session) { s.PeerProtocol = 2 }, "unsupported peer protocol"},
		{"desktop entrypoint", func(s *sessions.Session) { s.Entrypoint = peer.EntrypointDesktop }, "desktop app"},
		{"no socket in the record", func(s *sessions.Session) { s.MessagingSocketPath = "" }, "no messaging socket"},
		{"socket file gone", func(s *sessions.Session) { s.MessagingSocketPath = "/tmp/nacre-no-such.sock" }, "socket is missing"},
		{"another host", func(s *sessions.Session) { s.PIDDomain = "linux" }, "not on this computer"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			live := &fakeLive{}
			m := liveManager(t, live)
			s := liveSession(t, "sess-live")
			c.mutate(&s)
			m.SyncAdopted([]sessions.Session{s})
			r := adoptedRun(t, m)
			if c.want == "" {
				if !r.CanSendLive || r.LiveSendUnavailable != "" {
					t.Fatalf("run = %+v, want a live send offered", r)
				}
				return
			}
			if r.CanSendLive || !strings.Contains(r.LiveSendUnavailable, c.want) {
				t.Fatalf("can_send_live=%v unavailable=%q, want refused with %q", r.CanSendLive, r.LiveSendUnavailable, c.want)
			}
			if _, err := m.SendLive(r.ID, "hi", "Ryder"); !errors.Is(err, ErrConflict) {
				t.Fatalf("SendLive = %v, want a conflict", err)
			}
			if len(live.sends) != 0 {
				t.Fatalf("sent %q to an ineligible session", live.sends)
			}
			// The follow-up 409 is unchanged.
			if _, err := m.Followup(r.ID, "hi", ""); !errors.Is(err, ErrConflict) {
				t.Fatalf("Followup = %v, want a conflict", err)
			}
		})
	}
}

func TestLiveSendDisabledWithoutASender(t *testing.T) {
	m := quietManager(t, t.TempDir())
	m.SyncAdopted([]sessions.Session{liveSession(t, "sess-live")})
	r := adoptedRun(t, m)
	if r.CanSendLive {
		t.Fatalf("run = %+v: no sender, nothing to offer", r)
	}
	if _, err := m.SendLive(r.ID, "hi", ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("SendLive = %v, want a conflict", err)
	}
}

func TestLiveSendReceiptsAndTranscriptConfirmation(t *testing.T) {
	live := &fakeLive{}
	m := liveManager(t, live)
	s := liveSession(t, "sess-live")
	m.SyncAdopted([]sessions.Session{s})
	r := adoptedRun(t, m)

	got, err := m.SendLive(r.ID, "please also run the tests", "Ryder Cobean")
	if err != nil {
		t.Fatal(err)
	}
	if len(live.sends) != 1 || live.sends[0] != "Ryder Cobean (via Nacre): please also run the tests" {
		t.Fatalf("sent %q", live.sends)
	}
	if len(got.PeerMessages) != 1 || got.PeerMessages[0].Status != api.PeerSent || got.PeerMessages[0].MsgID != "msg-1" ||
		got.PeerMessages[0].Text != "please also run the tests" {
		t.Fatalf("peer messages = %+v", got.PeerMessages)
	}
	if got.CanFollowup == nil || *got.CanFollowup {
		t.Fatal("a live send must not turn on the ordinary follow-up")
	}

	// Held, then the user's approval arrives as delivered.
	m.PeerFrame(peer.Frame{Type: "control", Action: peer.ActionMessageStatus, Status: peer.StatusHeld, Reason: "held for approval", OrigMsgID: "msg-1"})
	_, evs, _ := m.Get(r.ID)
	if pm := mustRun(t, m, r.ID).PeerMessages[0]; pm.Status != peer.StatusHeld || pm.Reason != "held for approval" {
		t.Fatalf("after held: %+v", pm)
	}
	if last := evs[len(evs)-1]; last.Kind != api.KindSystem || !strings.HasPrefix(last.Text, "held:") {
		t.Fatalf("last event = %+v", last)
	}
	m.PeerFrame(peer.Frame{Type: "control", Action: peer.ActionMessageStatus, Status: peer.StatusDelivered, OrigMsgID: "msg-1"})
	if pm := mustRun(t, m, r.ID).PeerMessages[0]; pm.Status != peer.StatusDelivered || pm.Confirmed {
		t.Fatalf("after delivered receipt: %+v (a receipt is not transcript confirmation)", pm)
	}

	// The transcript shows our msg_id: confirmed.
	s.TranscriptState.PeerMsgIDs = []string{"someone-else", "msg-1"}
	m.SyncAdopted([]sessions.Session{s})
	if pm := mustRun(t, m, r.ID).PeerMessages[0]; !pm.Confirmed || pm.ConfirmedAt == "" {
		t.Fatalf("after transcript: %+v", pm)
	}

	// The idle notice is a hint, recorded on the message.
	m.PeerFrame(peer.Frame{Type: "control", Action: peer.ActionIdleNotice, State: "idle", FinishedAt: time.Now().UnixMilli(), OrigMsgID: "msg-1-idle"})
	if pm := mustRun(t, m, r.ID).PeerMessages[0]; pm.IdleAt == "" || pm.Status != peer.StatusDelivered {
		t.Fatalf("after idle notice: %+v", pm)
	}
	// Receipts for msg_ids nacred never sent change nothing.
	before := mustRun(t, m, r.ID)
	m.PeerFrame(peer.Frame{Type: "control", Action: peer.ActionMessageStatus, Status: peer.StatusRefused, OrigMsgID: "nope"})
	if after := mustRun(t, m, r.ID); after.PeerMessages[0].Status != before.PeerMessages[0].Status {
		t.Fatal("an unknown receipt changed the run")
	}
}

func TestLiveSendRefusedAndDroppedAreShown(t *testing.T) {
	live := &fakeLive{}
	m := liveManager(t, live)
	m.SyncAdopted([]sessions.Session{liveSession(t, "sess-live")})
	r := adoptedRun(t, m)
	m.SendLive(r.ID, "one", "")
	m.SendLive(r.ID, "two", "")
	m.PeerFrame(peer.Frame{Type: "control", Action: peer.ActionMessageStatus, Status: peer.StatusRefused, Reason: "crossSessionInbound is refuse", OrigMsgID: "msg-1"})
	m.PeerFrame(peer.Frame{Type: "control", Action: peer.ActionMessageStatus, Status: peer.StatusDropped, DropReason: "shutdown", OrigMsgID: "msg-2", DroppedMsgIDs: []string{"msg-2"}})
	pms := mustRun(t, m, r.ID).PeerMessages
	if pms[0].Status != peer.StatusRefused || pms[1].Status != peer.StatusDropped || pms[1].Reason != "shutdown" {
		t.Fatalf("peer messages = %+v", pms)
	}
	_, evs, _ := m.Get(r.ID)
	var errs []string
	for _, ev := range evs {
		if ev.Kind == api.KindSystem && ev.IsError != nil && *ev.IsError {
			errs = append(errs, ev.Text)
		}
	}
	if len(errs) != 2 || !strings.HasPrefix(errs[0], "refused:") || !strings.HasPrefix(errs[1], "dropped:") {
		t.Fatalf("error events = %q", errs)
	}
}

func TestLiveSendUnconfirmedAfterTheWindow(t *testing.T) {
	live := &fakeLive{}
	m := liveManager(t, live)
	s := liveSession(t, "sess-live")
	m.SyncAdopted([]sessions.Session{s})
	r := adoptedRun(t, m)
	m.SendLive(r.ID, "hello", "")

	// Backdate the send past LiveConfirmAfter, then poll.
	rr := m.get(r.ID)
	rr.mu.Lock()
	rr.meta.PeerMessages[0].SentAt = time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339)
	rr.mu.Unlock()
	m.SyncAdopted([]sessions.Session{s})
	if pm := mustRun(t, m, r.ID).PeerMessages[0]; pm.Status != api.PeerUnconfirmed || pm.Confirmed {
		t.Fatalf("after the window: %+v", pm)
	}
	if len(live.sends) != 1 {
		t.Fatal("nacred must never resend on its own")
	}
	// A late transcript entry still confirms it.
	s.TranscriptState.PeerMsgIDs = []string{"msg-1"}
	m.SyncAdopted([]sessions.Session{s})
	if pm := mustRun(t, m, r.ID).PeerMessages[0]; !pm.Confirmed {
		t.Fatalf("late confirmation: %+v", pm)
	}
}

func TestLiveSendErrorKeepsTheRunReadOnly(t *testing.T) {
	live := &fakeLive{err: errors.New("connection refused")}
	m := liveManager(t, live)
	m.SyncAdopted([]sessions.Session{liveSession(t, "sess-live")})
	r := adoptedRun(t, m)
	if _, err := m.SendLive(r.ID, "hi", ""); !errors.Is(err, ErrConflict) || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("SendLive = %v, want a conflict carrying the error", err)
	}
	if after := mustRun(t, m, r.ID); len(after.PeerMessages) != 0 {
		t.Fatalf("a failed send was recorded: %+v", after.PeerMessages)
	}
	// So does the guard refusing at send time (the pid died since the poll).
	live.err = &peer.Ineligible{Reason: "session process 4242 is not running"}
	if _, err := m.SendLive(r.ID, "hi", ""); !errors.Is(err, ErrConflict) || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("SendLive = %v", err)
	}
}

func TestLiveSendEndsWithTheSession(t *testing.T) {
	m := liveManager(t, &fakeLive{})
	m.SyncAdopted([]sessions.Session{liveSession(t, "sess-live")})
	r := adoptedRun(t, m)
	m.SyncAdopted(nil)
	after := mustRun(t, m, r.ID)
	if after.CanSendLive || after.Active() {
		t.Fatalf("ended run = %+v", after)
	}
	if _, err := m.SendLive(r.ID, "hi", ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("SendLive = %v", err)
	}
}

func TestSendLiveRefusesDispatchedRuns(t *testing.T) {
	m := liveManager(t, &fakeLive{})
	run, err := m.Start(StartRequest{RepoID: "rid", RepoPath: t.TempDir(), IssueID: "NAC-1", Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.SendLive(run.ID, "hi", ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("SendLive = %v", err)
	}
	if _, err := m.SendLive("r_nope", "hi", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SendLive unknown = %v", err)
	}
	m.Shutdown(time.Second)
}

func mustRun(t *testing.T, m *Manager, id string) api.Run {
	t.Helper()
	r, _, err := m.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
