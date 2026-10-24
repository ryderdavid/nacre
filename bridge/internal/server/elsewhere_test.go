package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/discovery"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
	"github.com/ryderdavid/nacre/bridge/internal/sessions"
	"github.com/ryderdavid/nacre/bridge/internal/sse"
)

func elsewhereOf(t *testing.T, raw api.Issue) *api.ActiveElsewhere {
	t.Helper()
	var m struct {
		Elsewhere *api.ActiveElsewhere `json:"active_elsewhere"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("issue no longer decodes: %v: %s", err, raw)
	}
	return m.Elsewhere
}

func TestMarkElsewhere(t *testing.T) {
	const actor = "Ryder Cobean via Nacre"
	issue := func(status, assignee string) api.Issue {
		b, _ := json.Marshal(map[string]any{"id": "NAC-1", "title": "t", "status": status, "assignee": assignee,
			"started_at": "2026-10-24T10:00:00Z", "updated_at": "2026-10-24T11:00:00Z"})
		return b
	}
	cases := []struct {
		name, status, assignee string
		local                  bool
		want                   *api.ActiveElsewhere
	}{
		{"claimed by someone else", "in_progress", "codex", false, &api.ActiveElsewhere{Assignee: "codex", Since: "2026-10-24T10:00:00Z"}},
		{"hooked", "hooked", "codex", false, &api.ActiveElsewhere{Assignee: "codex", Since: "2026-10-24T10:00:00Z"}},
		{"the person nacred acts for", "in_progress", "Ryder Cobean", false, &api.ActiveElsewhere{Assignee: "Ryder Cobean", Since: "2026-10-24T10:00:00Z", Self: true}},
		{"a local run wins", "in_progress", "codex", true, nil},
		{"nacred's own actor", "in_progress", actor, false, nil},
		{"unassigned", "in_progress", "", false, nil},
		{"open", "open", "codex", false, nil},
		{"blocked", "blocked", "codex", false, nil},
		{"closed", "closed", "codex", false, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := issue(c.status, c.assignee)
			got := elsewhereOf(t, markElsewhere(raw, actor, localRuns{live: map[string]bool{"NAC-1": c.local}}))
			if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
				t.Fatalf("active_elsewhere = %+v, want %+v", got, c.want)
			}
		})
	}

	// Since falls back to updated_at, and the rest of the issue is untouched.
	raw := []byte(`{"id":"NAC-2","status":"in_progress","assignee":"codex","updated_at":"2026-10-24T11:00:00Z","labels":["a"]}`)
	out := markElsewhere(raw, actor, localRuns{})
	if got := elsewhereOf(t, out); got == nil || got.Since != "2026-10-24T11:00:00Z" {
		t.Fatalf("since = %+v, want updated_at", got)
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	if m["id"] != "NAC-2" || len(m["labels"].([]any)) != 1 {
		t.Fatalf("issue fields lost: %s", out)
	}
	// A Nacre run claims its bead under the git user. Once it has ended, the
	// claim it made before ending is its own; a claim made after is not.
	claimed := []byte(`{"id":"NAC-3","status":"in_progress","assignee":"Ryder Cobean","started_at":"2026-10-24T10:00:00Z"}`)
	ended := func(at string) localRuns {
		end, _ := time.Parse(time.RFC3339, at)
		return localRuns{ended: map[string]time.Time{"NAC-3": end}}
	}
	if got := elsewhereOf(t, markElsewhere(claimed, actor, ended("2026-10-24T10:30:00Z"))); got != nil {
		t.Fatalf("claim made during a Nacre run that has ended: %+v", got)
	}
	if got := elsewhereOf(t, markElsewhere(claimed, actor, ended("2026-10-24T10:00:00Z"))); got != nil {
		t.Fatalf("claim in the same second the run ended: %+v", got)
	}
	if got := elsewhereOf(t, markElsewhere(claimed, actor, ended("2026-10-24T09:00:00Z"))); got == nil || !got.Self {
		t.Fatalf("claim made after the last Nacre run ended: %+v", got)
	}
	// A custom actor has no person behind it to call "you".
	if got := elsewhereOf(t, markElsewhere(raw, "nacred", localRuns{})); got == nil || got.Self {
		t.Fatalf("custom actor: %+v", got)
	}
}

// The marker reaches clients on both issue routes, gives way to a live run
// linked to the bead, and comes back once that run is a stale adopted session.
func TestIssuesCarryActiveElsewhere(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	repoID := discovery.RepoID(root)
	list := filepath.Join(t.TempDir(), "issues.json")
	if err := os.WriteFile(list, []byte(`[
		{"id":"NAC-7","status":"in_progress","assignee":"codex","started_at":"2026-10-24T10:00:00Z"},
		{"id":"NAC-8","status":"open","assignee":"codex"},
		{"id":"NAC-9","status":"in_progress","assignee":"Ryder Cobean","started_at":"2026-10-24T10:00:00Z"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	bdPath := filepath.Join(t.TempDir(), "bd")
	if err := os.WriteFile(bdPath, []byte("#!/bin/sh\ncat "+list+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	mgr, err := runs.New(runs.Options{Dir: t.TempDir(), ClaudePath: "/nonexistent/claude",
		Logger: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	client := bd.New(bdPath, "")
	s := &Server{Config: &config.Config{Token: "tok", Actor: "Ryder Cobean via Nacre"},
		Repos: repos.New(func() []string { return []string{root} }, client), BD: client, Runs: mgr,
		Broker: sse.NewBroker(), Log: log.New(io.Discard, "", 0)}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	marked := func() map[string]bool {
		t.Helper()
		resp, body := do(t, "GET", ts.URL+"/v1/repos/"+repoID+"/issues", "tok")
		if resp.StatusCode != 200 {
			t.Fatalf("list issues: %d %v", resp.StatusCode, body)
		}
		out := map[string]bool{}
		for _, is := range body["issues"].([]any) {
			m := is.(map[string]any)
			_, out[m["id"].(string)] = m["active_elsewhere"]
		}
		return out
	}
	if got := marked(); !got["NAC-7"] || got["NAC-8"] || !got["NAC-9"] {
		t.Fatalf("marked = %v, want NAC-7 and NAC-9", got)
	}
	resp, body := do(t, "GET", ts.URL+"/v1/repos/"+repoID+"/issues/NAC-7", "tok")
	if resp.StatusCode != 200 {
		t.Fatalf("get issue: %d %v", resp.StatusCode, body)
	}
	if mark, _ := body["issue"].(map[string]any)["active_elsewhere"].(map[string]any); mark["assignee"] != "codex" || mark["since"] != "2026-10-24T10:00:00Z" {
		t.Fatalf("GET issue active_elsewhere = %v", mark)
	}

	// A Nacre run on NAC-9 that has since ended (claude is missing, so it
	// fails at once) accounts for the claim it left behind.
	own, err := mgr.Start(runs.StartRequest{RepoID: repoID, RepoPath: root, IssueID: "NAC-9", Prompt: "do it"})
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if r, _ := mgr.Run(own.ID); !r.Active() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("run did not end")
		}
	}
	if got := marked(); got["NAC-9"] {
		t.Fatalf("marked after a Nacre run on it ended: %v", got)
	}

	// An agent session working on the bead is the run marker's job.
	session := func(lastWrite time.Time) []sessions.Session {
		return []sessions.Session{{Record: sessions.Record{PID: os.Getpid(), SessionID: "sess-1", CWD: root,
			StartedAt: time.Now().Add(-time.Hour).UnixMilli()},
			Repo: sessions.Repo{ID: repoID, Name: "r", Path: root}, IssueID: "NAC-7", LastWrite: lastWrite}}
	}
	mgr.SyncAdopted(session(time.Now()))
	if got := marked(); got["NAC-7"] {
		t.Fatalf("marked with a working adopted run: %v", got)
	}
	// Left open past the idle window, it is out of the Runs list, and the
	// claim in bd shows again.
	mgr.SyncAdopted(session(time.Now().Add(-time.Hour)))
	if got := marked(); !got["NAC-7"] {
		t.Fatalf("not marked with only a stale adopted run: %v", got)
	}
}
