package server

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/discovery"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
	"github.com/ryderdavid/nacre/bridge/internal/specflow"
)

func TestSpecLabelsNeverFallBackToGenericRun(t *testing.T) {
	s := &Server{}
	for _, raw := range []string{
		`{"labels":["specflow:missing"]}`,
		`{"labels":["spec-section:missing"]}`,
	} {
		if err := s.standaloneChangeGuard(context.Background(), "unused", "N-1", api.Issue(raw)); !errors.Is(err, specflow.ErrConflict) {
			t.Fatalf("linked work bypassed unavailable state: %v", err)
		}
	}
	if err := s.standaloneChangeGuard(context.Background(), "unused", "N-1", api.Issue(`{"labels":["frontend"]}`)); err != nil {
		t.Fatal(err)
	}
}

func TestSpecRoutesAuthenticateAndKeepOrphanLabelsClosed(t *testing.T) {
	repo := t.TempDir()
	bin := filepath.Join(t.TempDir(), "bd-fixture")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho '[{\"id\":\"N-1\",\"title\":\"Draft\",\"labels\":[\"specflow:missing\"]}]'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	client := bd.New(bin, "")
	mgr, err := specflow.New(specflow.Options{Dir: t.TempDir(), Beads: client})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Shutdown(time.Second)
	s := &Server{Config: &config.Config{Token: "fixture", DefaultMode: "auto"},
		Repos: repos.New(func() []string { return []string{repo} }, client), BD: client, Changes: mgr, Log: log.New(io.Discard, "", 0)}
	h := s.Handler()
	base := "/v1/repos/" + discovery.RepoID(repo) + "/issues/N-1"
	for _, tc := range []struct {
		method, path, body, token string
		status                    int
		contains                  string
	}{
		{"GET", base + "/change", "", "", http.StatusUnauthorized, "missing or invalid bearer token"},
		{"GET", base + "/change", "", "fixture", http.StatusOK, "saved workflow is missing"},
		{"POST", base + "/change", `{"action":"delete"}`, "fixture", http.StatusBadRequest, "supported specification action"},
		{"POST", base + "/change", `{"action":"develop"}`, "fixture", http.StatusConflict, "restore the workflow record"},
		{"POST", base + "/change", `{"action":"approve","revision":"reviewed","run":true}`, "fixture", http.StatusConflict, "develop a proposal first"},
		{"POST", base + "/runs", `{}`, "fixture", http.StatusConflict, "saved workflow is missing"},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.contains) {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}
