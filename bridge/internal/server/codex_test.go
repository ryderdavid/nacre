package server

import (
	"context"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/codex"
	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/discovery"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
	"github.com/ryderdavid/nacre/bridge/internal/sse"
)

// codexServer serves one repo whose bd knows bead X-1, with fake claude and
// codex binaries that each finish a turn at once. status is what the Codex
// check reports; checks counts how often it ran.
func codexServer(t *testing.T, status *codex.Status, checks *int) (ts *httptest.Server, repoID string, mgr *runs.Manager) {
	t.Helper()
	repoDir, bin := t.TempDir(), t.TempDir()
	write := func(name, script string) string {
		p := filepath.Join(bin, name)
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	bdPath := write("bd", `echo '[{"id":"X-1","title":"Do the thing","description":"Full detail","acceptance_criteria":"- exact criterion"}]'`+"\n")
	claudePath := write("claude", `echo '{"type":"result","subtype":"success","is_error":false,"result":"ok","session_id":"s"}'`+"\n")
	codexPath := write("codex", `echo '{"type":"thread.started","thread_id":"th"}'; echo '{"type":"turn.completed"}'`+"\n")
	mgr, err := runs.New(runs.Options{Dir: t.TempDir(), ClaudePath: claudePath, CodexPath: codexPath})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mgr.Shutdown(time.Second) })
	client := bd.New(bdPath, "")
	s := &Server{
		Config: &config.Config{Token: "tok", DefaultMode: "auto", DefaultModel: "opus", PromptTemplate: "work on {{id}}"},
		Repos:  repos.New(func() []string { return []string{repoDir} }, client),
		BD:     client, Runs: mgr, Broker: sse.NewBroker(), Log: log.New(io.Discard, "", 0),
		CodexCheck: func(context.Context) codex.Status { *checks++; return *status },
	}
	ts = httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts, discovery.RepoID(repoDir), mgr
}

func TestHealthReportsCodex(t *testing.T) {
	st, n := codex.Status{Version: "0.159.2"}, 0
	ts, _, _ := codexServer(t, &st, &n)
	_, body := do(t, "GET", ts.URL+"/v1/health", "tok")
	feats, _ := body["features"].([]any)
	if len(feats) != 1 || feats[0] != "codex" || body["codex_version"] != "0.159.2" || body["codex_unavailable"] != nil {
		t.Fatalf("health = %v", body)
	}

	st = codex.Status{Reason: "Codex isn't signed in on this computer; run `codex login` there"}
	ts, _, _ = codexServer(t, &st, &n)
	_, body = do(t, "GET", ts.URL+"/v1/health", "tok")
	if feats, _ := body["features"].([]any); len(feats) != 0 || body["codex_unavailable"] != st.Reason {
		t.Fatalf("health = %v", body)
	}
}

func TestStartRunWithCodex(t *testing.T) {
	st, checks := codex.Status{Reason: "Codex CLI isn't installed on this computer"}, 0
	ts, repoID, mgr := codexServer(t, &st, &checks)
	url := ts.URL + "/v1/repos/" + repoID + "/issues/X-1/runs"

	resp, body := postJSON(t, url, `{"agent":"codex"}`)
	if resp.StatusCode != 409 || body["detail"] != st.Reason {
		t.Fatalf("unavailable: %d %v", resp.StatusCode, body)
	}
	if resp, body := postJSON(t, url, `{"agent":"gemini"}`); resp.StatusCode != 400 {
		t.Fatalf("unknown agent: %d %v", resp.StatusCode, body)
	}

	// Signing in on the computer counts at once: an unavailable answer is not cached.
	st = codex.Status{Version: "1"}
	resp, body = postJSON(t, url, `{"agent":"codex","mode":"plan"}`)
	if resp.StatusCode != 201 || body["agent"] != "codex" || body["model"] != "" || body["mode"] != "plan" {
		t.Fatalf("codex run: %d %v", resp.StatusCode, body)
	}
	id := body["id"].(string)
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if r, _ := mgr.Run(id); !r.Active() {
			if r.Status != api.StatusSucceeded || r.SessionID != "th" {
				t.Fatalf("run = %+v", r)
			}
			break
		}
	}

	// The Claude path is unchanged: no agent on the wire, the default model.
	resp, body = postJSON(t, url, `{}`)
	if _, has := body["agent"]; resp.StatusCode != 201 || has || body["model"] != "opus" {
		t.Fatalf("claude run: %d %v", resp.StatusCode, body)
	}
	resp, body = postJSON(t, url, `{"agent":"claude-code"}`)
	if _, has := body["agent"]; resp.StatusCode != 201 || has {
		t.Fatalf("claude-code run: %d %v", resp.StatusCode, body)
	}
}
