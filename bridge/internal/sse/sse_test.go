package sse

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriterFormat(t *testing.T) {
	rec := httptest.NewRecorder()
	w, err := NewWriter(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Send("7", "event", map[string]string{"cmd": "a && b"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Send("", "run", []byte("{\"x\":1}\n{\"y\":2}")); err != nil {
		t.Fatal(err)
	}
	_ = w.Ping()
	want := "id: 7\nevent: event\ndata: {\"cmd\":\"a && b\"}\n\nevent: run\ndata: {\"x\":1}\ndata: {\"y\":2}\n\n: ping\n\n"
	if got := rec.Body.String(); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type %q", ct)
	}
}

func TestBrokerDropsWhenFull(t *testing.T) {
	b := NewBroker()
	ch, unsub := b.Subscribe()
	for i := 0; i < 100; i++ {
		b.Publish("repo", map[string]int{"i": i}) // must never block
	}
	if len(ch) != cap(ch) {
		t.Fatalf("len %d cap %d", len(ch), cap(ch))
	}
	unsub()
	if b.Subscribers() != 0 {
		t.Fatal("unsubscribe failed")
	}
}
