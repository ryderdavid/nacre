package server

import (
	"errors"
	"net"
	"net/http"
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
	case errors.Is(err, chat.ErrBusy), errors.Is(err, chat.ErrLimit):
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
	case chat.AgentCodex:
		writeError(w, http.StatusConflict, "Codex chats aren't available on this computer yet.", "")
		return
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

func (s *Server) sendChat(w http.ResponseWriter, r *http.Request) {
	m := s.chatsOn(w)
	if m == nil {
		return
	}
	var req chat.SendRequest
	if err := decode(r, &req, false); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
		return
	}
	res, err := m.Send(r.PathValue("chat"), req)
	if err != nil {
		s.chatFail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, res)
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
