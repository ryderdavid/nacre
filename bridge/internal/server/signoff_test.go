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

const signoffBD = `#!/bin/sh
echo "$@" >> "$BD_LOG"
case "$*" in
*" list "*) echo '[{"id":"B-1","title":"Placeholder","status":"open"}]' ;;
*) echo '[]' ;;
esac
`

func TestSigningOffClosesTheBeadAndNotRightSendsItBackToWork(t *testing.T) {
	repo, dir := t.TempDir(), t.TempDir()
	bin, logPath := filepath.Join(dir, "bd"), filepath.Join(dir, "calls")
	if err := os.WriteFile(bin, []byte(signoffBD), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BD_LOG", logPath)
	client := bd.New(bin, "ryder")
	s := &Server{Config: &config.Config{Token: "tok"}, Repos: repos.New(func() []string { return []string{repo} }, client), BD: client, Broker: sse.NewBroker(), Log: log.New(io.Discard, "", 0)}
	h := s.Handler()
	signOff := func(body string) (int, string) {
		os.Remove(logPath)
		r := httptest.NewRequest("POST", "/v1/repos/"+discovery.RepoID(repo)+"/issues/B-1/signoff", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer tok")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		b, _ := os.ReadFile(logPath)
		return w.Code, string(b)
	}

	code, calls := signOff(`{"approve":true}`)
	if code != http.StatusOK || !strings.Contains(calls, "close --force") || !strings.Contains(calls, "Signed off") ||
		!strings.Contains(calls, "--remove-label=signoff") || !strings.Contains(calls, "--unset-metadata=signoff") {
		t.Fatalf("sign off: %d\n%s", code, calls)
	}

	if code, _ := signOff(`{"approve":false}`); code != http.StatusBadRequest {
		t.Fatalf("not right without saying why: %d", code)
	}

	code, calls = signOff(`{"approve":false,"note":"The spinner overlaps the text"}`)
	if code != http.StatusOK || strings.Contains(calls, "close") || !strings.Contains(calls, "Not signed off: The spinner overlaps the text") ||
		!strings.Contains(calls, "--remove-label=signoff") {
		t.Fatalf("not right: %d\n%s", code, calls)
	}
}
