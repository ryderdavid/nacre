// Package sse implements Server-Sent Events: a per-response Writer and a
// fan-out Broker for global notifications.
package sse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// KeepAlive is the interval between ": ping" comments.
const KeepAlive = 15 * time.Second

// Writer writes SSE messages to an HTTP response, flushing after each one.
type Writer struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

// NewWriter sets SSE headers, sends them, and returns a Writer.
func NewWriter(w http.ResponseWriter) (*Writer, error) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	sw := &Writer{w: w, rc: http.NewResponseController(w)}
	// Disable any server write deadline for this long-lived response.
	_ = sw.rc.SetWriteDeadline(time.Time{})
	return sw, sw.rc.Flush()
}

// Send writes one message. id may be empty. data is JSON-encoded unless it is
// a json.RawMessage or []byte.
func (s *Writer) Send(id, event string, data any) error {
	var payload []byte
	switch d := data.(type) {
	case json.RawMessage:
		payload = d
	case []byte:
		payload = d
	default:
		b, err := Marshal(d)
		if err != nil {
			return err
		}
		payload = b
	}
	var b strings.Builder
	if id != "" {
		fmt.Fprintf(&b, "id: %s\n", id)
	}
	if event != "" {
		fmt.Fprintf(&b, "event: %s\n", event)
	}
	// JSON has no raw newlines, but be safe for arbitrary payloads.
	for _, line := range strings.Split(string(payload), "\n") {
		fmt.Fprintf(&b, "data: %s\n", line)
	}
	b.WriteString("\n")
	if _, err := s.w.Write([]byte(b.String())); err != nil {
		return err
	}
	return s.rc.Flush()
}

// Ping writes a keep-alive comment.
func (s *Writer) Ping() error {
	if _, err := s.w.Write([]byte(": ping\n\n")); err != nil {
		return err
	}
	return s.rc.Flush()
}

// Marshal encodes v as compact JSON without HTML escaping (so "&&" in a
// resume command stays readable), matching the REST responses.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Message is one broker notification.
type Message struct {
	Event string
	Data  []byte // JSON
}

// Broker fans messages out to subscribers. Slow subscribers drop messages
// rather than blocking publishers.
type Broker struct {
	mu   sync.Mutex
	subs map[chan Message]struct{}
}

// NewBroker returns an empty broker.
func NewBroker() *Broker { return &Broker{subs: map[chan Message]struct{}{}} }

// Subscribe registers a subscriber. Call the returned func to unsubscribe.
func (b *Broker) Subscribe() (<-chan Message, func()) {
	ch := make(chan Message, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}

// Publish JSON-encodes data and delivers it to every subscriber.
func (b *Broker) Publish(event string, data any) {
	payload, err := Marshal(data)
	if err != nil {
		return
	}
	msg := Message{Event: event, Data: payload}
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- msg:
		default: // subscriber is behind; drop
		}
	}
}

// Subscribers returns the current subscriber count.
func (b *Broker) Subscribers() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}
