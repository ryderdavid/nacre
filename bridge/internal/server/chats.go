package server

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/chat"
	"github.com/ryderdavid/nacre/bridge/internal/sse"
)

// chatTail is how many events GET /v1/chats/{id} returns.
const chatTail = 500

// NacredURL is the NACRED_URL given to chat turns: the address the server's
// listener is bound to, which is the address from config.ResolveListen that
// the `nacred` command also uses.
func NacredURL(ln net.Listener) string { return "http://" + ln.Addr().String() }

// chatsOn writes 503 and returns nil when Chat isn't set up here.
func (s *Server) chatsOn(w http.ResponseWriter) *chat.Manager {
	if s.Chats == nil {
		writeError(w, http.StatusServiceUnavailable, "Chat isn't available on this computer.", "")
		return nil
	}
	return s.Chats
}

// chatOf maps a chat error to a response.
func (s *Server) chatFail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, chat.ErrNotFound):
		writeError(w, http.StatusNotFound, "unknown chat", "")
	case errors.Is(err, chat.ErrUnknownRepo):
		writeError(w, http.StatusBadRequest, "unknown repo", "")
	case errors.Is(err, chat.ErrBusy), errors.Is(err, chat.ErrLimit), errors.Is(err, chat.ErrImageLimit):
		writeError(w, http.StatusConflict, err.Error(), "")
	default:
		writeError(w, http.StatusBadRequest, err.Error(), "")
	}
}

func (s *Server) listChats(w http.ResponseWriter, r *http.Request) {
	m := s.chatsOn(w)
	if m == nil {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chats": m.List()})
}

func (s *Server) createChat(w http.ResponseWriter, r *http.Request) {
	m := s.chatsOn(w)
	if m == nil {
		return
	}
	var req chat.CreateRequest
	if err := decode(r, &req, true); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
		return
	}
	switch req.Agent {
	case "", chat.AgentClaude:
		if s.Assist == nil || !s.Assist.Available() {
			writeError(w, http.StatusConflict, "Claude Code isn't installed on this computer.", "")
			return
		}
		if cs := s.chatSupport(r.Context()); !cs.OK {
			writeError(w, http.StatusConflict, cs.Reason, "")
			return
		}
	case chat.AgentCodex:
		if reason := s.codexChatStatus(r.Context()); reason != "" {
			writeError(w, http.StatusConflict, reason, "")
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "unknown agent", "")
		return
	}
	req.PeerAddr = r.RemoteAddr
	rec, err := m.Create(req)
	if err != nil {
		s.chatFail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"chat": rec})
}

func (s *Server) getChat(w http.ResponseWriter, r *http.Request) {
	m := s.chatsOn(w)
	if m == nil {
		return
	}
	rec, events, err := m.Get(r.PathValue("chat"), chatTail)
	if err != nil {
		s.chatFail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chat": rec, "events": events})
}

func (s *Server) deleteChat(w http.ResponseWriter, r *http.Request) {
	m := s.chatsOn(w)
	if m == nil {
		return
	}
	if err := m.Delete(r.PathValue("chat")); err != nil {
		s.chatFail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// chatMessage is the body of POST /v1/chats/{id}/messages.
type chatMessage struct {
	TurnID  string        `json:"turn_id"`
	Text    string        `json:"text"`
	Context *chat.Context `json:"context"`
	// Images is the message's ordered images, at most maxChatImages.
	Images []chatImage `json:"images,omitempty"`
}

// sendChat starts a turn with a message. Its body may carry images, so it is
// read with maxTurnBody (413 above it), and every image is validated (400
// naming the image or the limit) before anything is written or started.
func (s *Server) sendChat(w http.ResponseWriter, r *http.Request) {
	m := s.chatsOn(w)
	if m == nil {
		return
	}
	var body chatMessage
	if !decodeTurn(w, r, &body, maxTurnBody) {
		return
	}
	images, bad := decodeChatImages(body.Images)
	if bad != "" {
		writeError(w, http.StatusBadRequest, bad, "")
		return
	}
	req := chat.SendRequest{TurnID: body.TurnID, Text: body.Text, Context: body.Context, Images: images}
	res, err := m.Send(r.PathValue("chat"), req)
	if err != nil {
		s.chatFail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, res)
}

// chatImage serves one of a chat's stored images, for thumbnails.
func (s *Server) chatImage(w http.ResponseWriter, r *http.Request) {
	m := s.chatsOn(w)
	if m == nil {
		return
	}
	path, mediaType, err := m.Image(r.PathValue("chat"), r.PathValue("image"))
	switch {
	case errors.Is(err, chat.ErrNoImage):
		writeError(w, http.StatusNotFound, "unknown image", "")
		return
	case err != nil:
		s.chatFail(w, err)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "unknown image", "")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusNotFound, "unknown image", "")
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", st.ModTime(), f)
}

func (s *Server) cancelChat(w http.ResponseWriter, r *http.Request) {
	m := s.chatsOn(w)
	if m == nil {
		return
	}
	if err := m.Cancel(r.PathValue("chat")); err != nil {
		s.chatFail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// streamChat sends a chat's events after ?after=seq (or Last-Event-ID), each
// with its seq as the SSE id, and follows them while a turn runs. Once the
// chat is not running and every event has been sent, one "state" message
// carries the record and the stream ends.
func (s *Server) streamChat(w http.ResponseWriter, r *http.Request) {
	m := s.chatsOn(w)
	if m == nil {
		return
	}
	id := r.PathValue("chat")
	after := int64(0)
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		after, _ = strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	} else if v := r.URL.Query().Get("after"); v != "" {
		after, _ = strconv.ParseInt(v, 10, 64)
	}
	snap, err := m.Since(id, after)
	if err != nil {
		s.chatFail(w, err)
		return
	}
	sw, err := sse.NewWriter(w)
	if err != nil {
		return
	}
	ping := time.NewTicker(sse.KeepAlive)
	defer ping.Stop()
	for {
		for _, ev := range snap.Events {
			if sw.Send(strconv.FormatInt(ev.Seq, 10), "event", ev) != nil {
				return
			}
			after = ev.Seq
		}
		if snap.Gone {
			_ = sw.Send("", "error", map[string]string{"error": "This chat was deleted."})
			return
		}
		st := snap.Record.State
		if st != chat.StateRunning && st != chat.StateWaitingPermission {
			_ = sw.Send("", "state", snap.Record)
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if sw.Ping() != nil {
				return
			}
		case <-snap.Changed:
		}
		snap, err = m.Since(id, after)
		if err != nil {
			_ = sw.Send("", "error", map[string]string{"error": "This chat was deleted."})
			return
		}
	}
}

// permissionAnswer is POST /v1/chats/{id}/permissions/{req}.
type permissionAnswer struct {
	Allow *bool  `json:"allow"`
	Note  string `json:"note,omitempty"`
}

// answerChatPermission answers a chat's host permission request. Only a
// paired device may answer: a request from this computer itself (loopback or
// one of its own addresses, such as the Tailscale address in NACRED_URL) is
// refused, so a session on the Mac can't approve its own step.
func (s *Server) answerChatPermission(w http.ResponseWriter, r *http.Request) {
	m := s.chatsOn(w)
	if m == nil {
		return
	}
	if s.fromThisComputer(r) {
		writeError(w, http.StatusForbidden, chat.PairedDeviceMsg,
			"Permission requests are answered from the phone, not from this computer.")
		return
	}
	var req permissionAnswer
	if err := decode(r, &req, false); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
		return
	}
	if req.Allow == nil {
		writeError(w, http.StatusBadRequest, "allow required", "")
		return
	}
	ev, err := m.Answer(r.PathValue("chat"), r.PathValue("req"), *req.Allow, req.Note)
	switch {
	case err == nil:
		rec, _, _ := m.Get(r.PathValue("chat"), 0)
		writeJSON(w, http.StatusOK, map[string]any{"permission": ev, "chat": rec})
	case errors.Is(err, chat.ErrDecided):
		body := map[string]any{"error": err.Error()}
		if ev.Kind != "" {
			body["permission"] = ev
		}
		writeJSON(w, http.StatusConflict, body)
	case errors.Is(err, chat.ErrUnknownRequest):
		writeError(w, http.StatusNotFound, "unknown permission request", "")
	default:
		s.chatFail(w, err)
	}
}

// fromThisComputer reports whether r came from this computer: loopback, the
// address in NACRED_URL, or any address of its own interfaces.
func (s *Server) fromThisComputer(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return true // not a peer address we can place; refuse
	}
	if ip.IsLoopback() || ip.IsUnspecified() {
		return true
	}
	if s.Chats != nil {
		if u, err := url.Parse(s.Chats.NacredURL()); err == nil {
			if own := net.ParseIP(u.Hostname()); own != nil && own.Equal(ip) {
				return true
			}
		}
	}
	for _, own := range s.localAddrs() {
		if own.Equal(ip) {
			return true
		}
	}
	return false
}

func (s *Server) localAddrs() []net.IP {
	if s.LocalAddrs != nil {
		return s.LocalAddrs()
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	out := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok {
			out = append(out, ipn.IP)
		}
	}
	return out
}
