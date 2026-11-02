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

const commentBD = `#!/bin/sh
echo "$@" >> "$BD_LOG"
echo '[]'
`

func TestAnAnswerSavedFromChatIsClaudesComment(t *testing.T) {
	repo, dir := t.TempDir(), t.TempDir()
	bin, logPath := filepath.Join(dir, "bd"), filepath.Join(dir, "calls")
	if err := os.WriteFile(bin, []byte(commentBD), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BD_LOG", logPath)
	client := bd.New(bin, "ryder")
	s := &Server{Config: &config.Config{Token: "tok"}, Repos: repos.New(func() []string { return []string{repo} }, client), BD: client, Broker: sse.NewBroker(), Log: log.New(io.Discard, "", 0)}
	h := s.Handler()
	comment := func(body string) string {
		os.Remove(logPath)
		r := httptest.NewRequest("POST", "/v1/repos/"+discovery.RepoID(repo)+"/issues/B-1/comments", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer tok")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("comment: %d %s", w.Code, w.Body)
		}
		b, _ := os.ReadFile(logPath)
		return string(b)
	}
	if got := comment(`{"text":"Sort by start date."}`); !strings.Contains(got, "--author=ryder -- B-1 Sort by start date.") {
		t.Errorf("owner's comment:\n%s", got)
	}
	if got := comment(`{"text":"Ready keeps its order.","from_claude":true}`); !strings.Contains(got, "--author=Claude -- B-1 Ready keeps its order.") {
		t.Errorf("saved answer:\n%s", got)
	}
}
