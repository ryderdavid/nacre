package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
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

func get(t *testing.T, h http.Handler, path string, out any) int {
	t.Helper()
	r := httptest.NewRequest("GET", path, nil)
	r.Header.Set("Authorization", "Bearer fixture")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if out != nil && w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			t.Fatalf("%s: %v %s", path, err, w.Body.String())
		}
	}
	return w.Code
}

func TestAttentionAndChangesRoutes(t *testing.T) {
	repo := t.TempDir()
	bin := filepath.Join(t.TempDir(), "bd-fixture")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho '[{\"id\":\"N-1\",\"title\":\"Decide the palette\",\"status\":\"open\",\"issue_type\":\"decision\"}]'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	client := bd.New(bin, "")
	registry := repos.New(func() []string { return []string{repo} }, client)
	s := &Server{Config: &config.Config{Token: "fixture", DefaultMode: "auto"}, Repos: registry, BD: client, Log: log.New(io.Discard, "", 0)}
	h := s.Handler()

	var empty struct{ Items []api.AttentionItem }
	if code := get(t, h, "/v1/attention", &empty); code != http.StatusOK || empty.Items == nil || len(empty.Items) != 0 {
		t.Fatalf("without a builder: %d %+v", code, empty)
	}
	var none struct {
		Changes  []api.ChangeSummary
		Total    int
		OpenSpec bool
	}
	changesPath := "/v1/repos/" + discovery.RepoID(repo) + "/changes"
	if code := get(t, h, changesPath, &none); code != http.StatusOK || none.Changes == nil || none.Total != 0 {
		t.Fatalf("without specflow: %d %+v", code, none)
	}

	mgr, err := specflow.New(specflow.Options{Dir: t.TempDir(), Beads: client})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Shutdown(time.Second)
	s.Changes = mgr
	s.Attention = &attention.Builder{Repos: registry.Refs, Issues: client.List, Changes: func(p string) ([]api.ChangeSummary, error) {
		l, _, e := mgr.Summaries(p)
		return l, e
	}}
	h = s.Handler()
	var got struct{ Items []api.AttentionItem }
	if code := get(t, h, "/v1/attention", &got); code != http.StatusOK || len(got.Items) != 1 || got.Items[0].Kind != api.AttentionBeadDecision || got.Items[0].BeadID != "N-1" {
		t.Fatalf("attention: %d %+v", code, got)
	}
	if code := get(t, h, changesPath, &none); code != http.StatusOK || len(none.Changes) != 0 || none.OpenSpec {
		t.Fatalf("changes: %d %+v", code, none)
	}
	if code := get(t, h, "/v1/repos/nope/changes", nil); code != http.StatusNotFound {
		t.Fatalf("unknown repo: %d", code)
	}
}
