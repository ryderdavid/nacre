package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const hookSID = "c704d257-8d1e-4bf5-9821-9cc7b84c5131"

func hookConfig(t *testing.T, listen string, adopt bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	data, _ := json.Marshal(map[string]any{"listen": listen, "token": "tok", "adopt_sessions": adopt})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

type hookRecorder struct {
	mu     sync.Mutex
	bodies []map[string]any
	auth   []string
}

func (h *hookRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	data, _ := io.ReadAll(r.Body)
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	h.mu.Lock()
	h.bodies = append(h.bodies, m)
	h.auth = append(h.auth, r.Method+" "+r.URL.Path+" "+r.Header.Get("Authorization"))
	h.mu.Unlock()
	w.WriteHeader(http.StatusAccepted)
}

func TestHookPostsTheSessionAndStaysSilent(t *testing.T) {
	rec := &hookRecorder{}
	ts := httptest.NewServer(rec)
	defer ts.Close()
	cfg := hookConfig(t, strings.TrimPrefix(ts.URL, "http://"), true)

	run := func(payload string) string {
		var out bytes.Buffer
		if code := runHook([]string{"--config", cfg}, strings.NewReader(payload), &out); code != 0 {
			t.Fatalf("exit %d", code)
		}
		return out.String()
	}
	start := `{"session_id":"` + hookSID + `","transcript_path":"/t/` + hookSID + `.jsonl","cwd":"/r","hook_event_name":"SessionStart","source":"startup"}`
	if out := run(start); out != "" {
		t.Fatalf("hook wrote to stdout: %q", out)
	}
	// Tool calls other than a bd claim or close are not sent, and the tool's
	// output never is.
	run(`{"session_id":"` + hookSID + `","hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"ls"}}`)
	run(`{"session_id":"` + hookSID + `","hook_event_name":"PostToolUse","tool_name":"Read","tool_input":{"file_path":"/x"}}`)
	run(`{"session_id":"` + hookSID + `","hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"bd update NAC-1 --claim"},"tool_response":{"stdout":"secret"}}`)
	run(`not json`)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.bodies) != 2 {
		t.Fatalf("sent %d hints: %+v", len(rec.bodies), rec.bodies)
	}
	if rec.auth[0] != "POST /v1/sessions Bearer tok" {
		t.Fatalf("request = %q", rec.auth[0])
	}
	if b := rec.bodies[0]; b["transcript_path"] != "/t/"+hookSID+".jsonl" || b["cwd"] != "/r" || b["session_id"] != hookSID {
		t.Fatalf("start body = %+v", b)
	}
	if b := rec.bodies[1]; b["tool_response"] != nil || b["tool_input"].(map[string]any)["command"] != "bd update NAC-1 --claim" {
		t.Fatalf("claim body = %+v", b)
	}
}

// With nacred down, adoption off or no config at all, the hook still exits 0
// at once and says nothing.
func TestHookNeverFailsTheSession(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	down := ln.Addr().String()
	ln.Close()
	payload := `{"session_id":"` + hookSID + `","hook_event_name":"Stop"}`
	for name, cfg := range map[string]string{
		"nacred down": hookConfig(t, down, true),
		"adopt off":   hookConfig(t, down, false),
		"no config":   filepath.Join(t.TempDir(), "missing.json"),
	} {
		var out bytes.Buffer
		began := time.Now()
		if code := runHook([]string{"--config", cfg}, strings.NewReader(payload), &out); code != 0 || out.Len() != 0 {
			t.Fatalf("%s: exit %d, stdout %q", name, code, out.String())
		}
		if d := time.Since(began); d > 2*hookTimeout {
			t.Fatalf("%s: took %s", name, d)
		}
		if _, err := os.Stat(cfg); name == "no config" && err == nil {
			t.Fatal("the hook created a config file")
		}
	}
	if err := postHook(context.Background(), hookConfig(t, down, false), strings.NewReader(payload), http.DefaultClient); err == nil || !strings.Contains(err.Error(), "adopt_sessions is off") {
		t.Fatalf("adopt off: %v", err)
	}
}

func TestHookSettingsSnippet(t *testing.T) {
	var out bytes.Buffer
	runHook([]string{"--settings", "--config", "/tmp/my config.json"}, strings.NewReader(""), &out)
	var got struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type, Command string
				Timeout       int
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	for _, ev := range []string{"SessionStart", "PostToolUse", "Stop"} {
		e := got.Hooks[ev]
		if len(e) != 1 || len(e[0].Hooks) != 1 || !strings.HasSuffix(e[0].Hooks[0].Command, " hook --config '/tmp/my config.json'") {
			t.Fatalf("%s = %+v", ev, e)
		}
	}
	if got.Hooks["PostToolUse"][0].Matcher != "Bash" {
		t.Fatalf("PostToolUse matcher = %q", got.Hooks["PostToolUse"][0].Matcher)
	}
}
