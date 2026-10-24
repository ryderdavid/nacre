package server

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/discovery"
	"github.com/ryderdavid/nacre/bridge/internal/gates"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
	"github.com/ryderdavid/nacre/bridge/internal/rounds"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
	"github.com/ryderdavid/nacre/bridge/internal/sse"
	"github.com/ryderdavid/nacre/bridge/internal/workspace"
)

// The rounds API: refusals, listing and unknown rounds.
func TestRoundsAPI(t *testing.T) {
	repoDir := t.TempDir()
	client := bd.New("/nonexistent/bd", "")
	registry := repos.New(func() []string { return []string{repoDir} }, client)
	mgr, err := runs.New(runs.Options{Dir: t.TempDir(), ClaudePath: "/nonexistent"})
	if err != nil {
		t.Fatal(err)
	}
	rm, err := rounds.New(rounds.Options{
		Dir: t.TempDir(), Runs: mgr, Beads: client, Gates: gates.NewRunner(1),
		Workspaces: &workspace.Manager{Root: filepath.Join(t.TempDir(), "wt")},
		Repo: func(id string) (string, string, error) {
			rp, err := registry.Summary(id)
			return rp.Name, rp.Path, err
		},
		Logger: log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Config: &config.Config{Token: "tok"}, Repos: registry, BD: client, Runs: mgr, Rounds: rm, Broker: sse.NewBroker(), Log: log.New(io.Discard, "", 0)}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	repoID := discovery.RepoID(repoDir)

	resp, body := post(t, ts.URL+"/v1/repos/"+repoID+"/rounds", `{"beads":["X-1"]}`)
	if resp.StatusCode != http.StatusConflict || !strings.Contains(body["error"].(string), "nacre.yaml") {
		t.Errorf("round without nacre.yaml: %d %v", resp.StatusCode, body)
	}
	if resp, _ := post(t, ts.URL+"/v1/repos/"+repoID+"/rounds", `{"beads":["X-1"],"budget_usd":-1}`); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("negative budget: %d", resp.StatusCode)
	}
	if resp, _ := post(t, ts.URL+"/v1/repos/nope/rounds", `{"beads":["X-1"]}`); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown repo: %d", resp.StatusCode)
	}
	resp, body = do(t, "GET", ts.URL+"/v1/rounds", "tok")
	if resp.StatusCode != 200 || body["rounds"] == nil {
		t.Errorf("list: %d %v", resp.StatusCode, body)
	}
	for _, path := range []string{"/v1/rounds/rdnope", "/v1/rounds/rdnope/stream"} {
		if resp, _ := do(t, "GET", ts.URL+path, "tok"); resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: %d", path, resp.StatusCode)
		}
	}
	for _, path := range []string{"land", "cancel", "resume", "beads/X-1/retry"} {
		if resp, _ := post(t, ts.URL+"/v1/rounds/rdnope/"+path, `{}`); resp.StatusCode != http.StatusNotFound {
			t.Errorf("POST %s: %d", path, resp.StatusCode)
		}
	}
	if resp, _ := post(t, ts.URL+"/v1/rounds/rdnope/answers", `{"question_id":"q1","answer":"yes"}`); resp.StatusCode != http.StatusNotFound {
		t.Errorf("answers: %d", resp.StatusCode)
	}
}
