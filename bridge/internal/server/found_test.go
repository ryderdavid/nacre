package server

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/discovery"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
	"github.com/ryderdavid/nacre/bridge/internal/sse"
)

// A fake bd that logs every call and knows one spec epic and one plain bead.
const fakeBD = `#!/bin/sh
echo "$@" >> "$BD_LOG"
case "$3" in
list)
  case "$*" in
  *--id=E-1*) echo '[{"id":"E-1","title":"Epic","issue_type":"epic","labels":["specflow:sf1","frontend"]}]' ;;
  *--id=P-1*) echo '[{"id":"P-1","title":"Plain","issue_type":"epic","labels":["frontend"]}]' ;;
  *--id=N-*) echo '[{"id":"N-9","title":"New","labels":[]}]' ;;
  *) echo '[]' ;;
  esac ;;
create) echo N-9 ;;
*) echo '{}' ;;
esac
`

func TestCreatesUnderASpecEpicAreFoundWork(t *testing.T) {
	repo := t.TempDir()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bd")
	logPath := filepath.Join(dir, "calls")
	if err := os.WriteFile(bin, []byte(fakeBD), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BD_LOG", logPath)
	client := bd.New(bin, "")
	s := &Server{Config: &config.Config{Token: "fixture", DefaultMode: "auto"}, Repos: repos.New(func() []string { return []string{repo} }, client), BD: client, Broker: sse.NewBroker(), Log: log.New(io.Discard, "", 0)}
	h := s.Handler()
	post := func(body string) int {
		r := httptest.NewRequest("POST", "/v1/repos/"+discovery.RepoID(repo)+"/issues", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer fixture")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	calls := func() string {
		b, _ := os.ReadFile(logPath)
		os.Remove(logPath)
		return string(b)
	}
	if code := post(`{"title":"Migrate first","parent":"E-1"}`); code != http.StatusCreated {
		t.Fatalf("create under spec epic: %d", code)
	}
	got := calls()
	if !strings.Contains(got, "--no-inherit-labels") || !strings.Contains(got, "--labels=found:sf1") {
		t.Fatalf("spec epic child inherits labels or lacks found:\n%s", got)
	}
	if code := post(`{"title":"Ordinary child","parent":"P-1"}`); code != http.StatusCreated {
		t.Fatalf("create under plain epic: %d", code)
	}
	if got := calls(); strings.Contains(got, "--no-inherit-labels") || strings.Contains(got, "found:") {
		t.Fatalf("plain epic child changed:\n%s", got)
	}
	if code := post(`{"title":"Follow-on","related":["E-1"],"follow_on_of":"F-1"}`); code != http.StatusCreated {
		t.Fatalf("follow-on: %d", code)
	}
	got = calls()
	if !strings.Contains(got, "dep add --type=related -- N-9 E-1") || !strings.Contains(got, "close --reason=Made a follow-on change: N-9. -- F-1") {
		t.Fatalf("follow-on not linked:\n%s", got)
	}
	if code := post(`{"title":"x","related":["bad id"]}`); code != http.StatusBadRequest {
		t.Fatalf("invalid related id accepted: %d", code)
	}
	// A note from the intake card is added to the description.
	if code := post(`{"title":"Keep drafts","description":"Per thread.","owner_note":"keep it to the folded screen"}`); code != http.StatusCreated {
		t.Fatalf("create with a note: %d", code)
	}
	if got := calls(); !strings.Contains(got, "Per thread.\n\nRyder's note: keep it to the folded screen") {
		t.Fatalf("note not in the description:\n%s", got)
	}
}

func TestPendingAmendmentsNeverRunAlone(t *testing.T) {
	s := &Server{}
	if err := s.standaloneChangeGuard(t.Context(), "unused", "F-1", []byte(`{"labels":["found:sf1","found-amend"]}`)); err == nil || !strings.Contains(err.Error(), "pending amendment") {
		t.Fatalf("pending amendment ran: %v", err)
	}
	if err := s.standaloneChangeGuard(t.Context(), "unused", "F-2", []byte(`{"labels":["found:sf1"]}`)); err != nil {
		t.Fatalf("how/order found work refused: %v", err)
	}
}
