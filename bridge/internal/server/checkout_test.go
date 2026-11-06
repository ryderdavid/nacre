package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/attention"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/discovery"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
	"github.com/ryderdavid/nacre/bridge/internal/specflow"
)

const checkoutBD = `#!/bin/sh
old='{"id":"N-48","title":"Chat images","status":"closed","issue_type":"epic"}'
live='{"id":"N-50","title":"Live","status":"open","issue_type":"feature"}'
case "$*" in
*--id=N-48*) echo "[$old]" ;;
*--id=N-50*) echo "[$live]" ;;
*--id=*) echo '[]' ;;
*) echo "[$old,$live]" ;;
esac
`

// NAC-pjq.3 and NAC-niv: the changes route lists the checkout's untracked
// active changes beside the recorded ones, and leaves out a recorded change
// proven given up (folder gone, owning beads closed) while keeping one whose
// folder is gone but whose seed is still open. Needs you does the same.
func TestChangesListTheCheckoutAndDropWithdrawnProposals(t *testing.T) {
	repo := t.TempDir()
	canon, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "bd-fixture")
	if err := os.WriteFile(bin, []byte(checkoutBD), 0o700); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for id, change := range map[string]map[string]any{
		"sfold":  {"id": "sfold", "seed_id": "N-48", "epic_id": "N-48", "change_name": "chat-image-attachments", "phase": "stale", "revision": "r1", "approved_revision": "r1", "updated_at": "2026-11-04T00:00:00Z"},
		"sflive": {"id": "sflive", "seed_id": "N-50", "change_name": "still-live", "phase": "stale", "revision": "r2", "approved_revision": "r2", "updated_at": "2026-11-05T00:00:00Z"},
	} {
		b, _ := json.Marshal(map[string]any{"repo": canon, "change": change})
		if err := os.WriteFile(filepath.Join(dir, id+".json"), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for path, body := range map[string]string{
		"openspec/changes/set-up-gates/proposal.md": "## Why\n\nGates by hand.\n",
		"openspec/changes/set-up-gates/tasks.md":    "## 1. Draft\n\n- [ ] 1.1 Draft nacre.yaml\n",
	} {
		full := filepath.Join(repo, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	client := bd.New(bin, "")
	registry := repos.New(func() []string { return []string{repo} }, client)
	mgr, err := specflow.New(specflow.Options{Dir: dir, Beads: client})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Shutdown(time.Second)
	s := &Server{Config: &config.Config{Token: "fixture", DefaultMode: "auto"}, Repos: registry, BD: client, Changes: mgr, Log: log.New(io.Discard, "", 0)}
	s.Attention = &attention.Builder{Repos: registry.Refs, Issues: client.List, Changes: func(p string) ([]api.ChangeSummary, error) {
		l, _, e := mgr.Summaries(p)
		return l, e
	}}
	h := s.Handler()
	base := "/v1/repos/" + discovery.RepoID(repo) + "/changes"

	var list struct {
		Changes   []api.ChangeSummary
		Total     int
		Untracked []api.ChangeSummary
	}
	if code := get(t, h, base, &list); code != http.StatusOK {
		t.Fatalf("changes: %d", code)
	}
	if len(list.Changes) != 1 || list.Changes[0].ID != "sflive" || !list.Changes[0].Missing || list.Total != 1 {
		t.Fatalf("recorded: %+v total %d", list.Changes, list.Total)
	}
	if len(list.Untracked) != 1 || list.Untracked[0].ChangeName != "set-up-gates" || !list.Untracked[0].Untracked || list.Untracked[0].Phase != "awaiting_approval" {
		t.Fatalf("untracked: %+v", list.Untracked)
	}

	var env specflow.Envelope
	if code := get(t, h, base+"/set-up-gates", &env); code != http.StatusOK || env.Change == nil || !env.Change.Untracked || len(env.Change.Artifacts) != 2 {
		t.Fatalf("untracked change: %d %+v", code, env)
	}
	for _, name := range []string{"nope", "chat-image-attachments", "still-live"} {
		if code := get(t, h, base+"/"+name, nil); code != http.StatusNotFound {
			t.Fatalf("%s: %d", name, code)
		}
	}

	var att struct{ Items []api.AttentionItem }
	if code := get(t, h, "/v1/attention", &att); code != http.StatusOK {
		t.Fatalf("attention: %d", code)
	}
	if len(att.Items) != 1 || att.Items[0].ChangeID != "sflive" || att.Items[0].Kind != api.AttentionApprovalStale {
		t.Fatalf("attention: %+v", att.Items)
	}
}
