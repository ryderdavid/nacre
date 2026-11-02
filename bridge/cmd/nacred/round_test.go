package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/config"
)

// A stand-in nacred for the round command.
func fakeNacred(t *testing.T, repoPath string, got *map[string]any) *config.Config {
	t.Helper()
	round := api.Round{ID: "rd1", RepoName: "Nacre", State: api.RoundNeedsDecision, Target: "main", Base: "abcdef123456", BudgetUSD: 25,
		Beads:     []api.RoundBead{{ID: "NAC-1", State: api.BeadWaiting}, {ID: "NAC-2", State: api.BeadWaiting, After: []string{"NAC-1"}}},
		Questions: []api.Question{{ID: "q1", Beads: []string{"NAC-1", "NAC-2"}, Text: "Which way?", Options: []string{"Left", "Right"}}}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/repos", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"repos": []api.Repo{{ID: "repo1", Name: "Nacre", Path: repoPath}}})
	})
	mux.HandleFunc("POST /v1/repos/repo1/rounds", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		json.NewDecoder(r.Body).Decode(got)
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(round)
	})
	mux.HandleFunc("GET /v1/rounds/rd1", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"round": round, "events": []api.RoundEvent{{Seq: 1, Kind: "state", Text: "Round started"}}})
	})
	mux.HandleFunc("POST /v1/rounds/rd1/answers", func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(got)
		json.NewEncoder(w).Encode(round)
	})
	mux.HandleFunc("POST /v1/rounds/rdx/land", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(api.Error{Error: "the round isn't ready to merge"})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return &config.Config{Listen: strings.TrimPrefix(ts.URL, "http://"), Token: "tok", DataDir: t.TempDir()}
}

func TestRoundCommand(t *testing.T) {
	repo := t.TempDir()
	os.Mkdir(filepath.Join(repo, ".git"), 0o755)
	var got map[string]any
	cfg := fakeNacred(t, repo, &got)
	var out bytes.Buffer

	if err := runRound(cfg, []string{"start", "--repo", "Nacre", "NAC-1", "--budget", "12", "NAC-2"}, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if beads, _ := got["beads"].([]any); len(beads) != 2 || got["budget_usd"] != 12.0 {
		t.Errorf("request = %v", got)
	}
	if !strings.Contains(out.String(), "Round rd1 started in Nacre with 2 bead(s)") {
		t.Errorf("start output: %s", out.String())
	}

	out.Reset()
	if err := runRound(cfg, []string{"status", "rd1"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"needs_decision", "after NAC-1", "Question q1", "- Right", "nacred round answer rd1 q1"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("status lacks %q:\n%s", want, out.String())
		}
	}

	got = nil
	if err := runRound(cfg, []string{"answer", "rd1", "q1", "Go", "right"}, &out); err != nil || got["answer"] != "Go right" {
		t.Errorf("answer: %v %v", err, got)
	}
	if err := runRound(cfg, []string{"land", "rdx"}, &out); err == nil || !strings.Contains(err.Error(), "isn't ready to merge") {
		t.Errorf("land refusal: %v", err)
	}
	if err := runRound(cfg, []string{"start", "--repo", "Elsewhere", "X-1"}, &out); err == nil || !strings.Contains(err.Error(), "doesn't know") {
		t.Errorf("unknown repo: %v", err)
	}
	if err := runRound(cfg, []string{"gc"}, &out); err != nil {
		t.Errorf("gc: %v", err)
	}
}
