package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/chat"
	"github.com/ryderdavid/nacre/bridge/internal/chat/fakecodex"
	"github.com/ryderdavid/nacre/bridge/internal/codex"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
)

// codexChatServer is a chat server whose Codex chats run on the fake
// app-server, with Codex reported as status.
func codexChatServer(t *testing.T, status codex.Status) (s *Server, fdir string) {
	t.Helper()
	claudeBin, _ := chatClaude(t, "")
	codexBin, fdir := fakecodex.Install(t)
	s, _ = chatServer(t, claudeBin, func(o *chat.Options) { o.CodexPath = codexBin })
	s.Config.CodexPath = codexBin
	s.CodexCheck = func(context.Context) codex.Status { return status }
	mgr, err := runs.New(runs.Options{Dir: t.TempDir(), ClaudePath: claudeBin})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mgr.Shutdown(time.Second) })
	s.Runs = mgr
	return s, fdir
}

func hasFeature(body map[string]any, name string) bool {
	feats, _ := body["features"].([]any)
	for _, f := range feats {
		if f == name {
			return true
		}
	}
	return false
}

func TestAgentChatCodexAdvertisedWhenSignedInAndAppServerStarts(t *testing.T) {
	s, fdir := codexChatServer(t, codex.Status{Version: "0.159.2"})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	_, health := call(t, "GET", ts.URL+"/v1/health", "")
	if !hasFeature(health, "agent_chat_codex") || health["agent_chat_codex_unavailable"] != nil {
		t.Fatalf("health = %v", health)
	}
	// The check started the (fake) app-server and completed its handshake.
	argv, err := os.ReadFile(filepath.Join(fdir, "call0.argv"))
	if err != nil || strings.TrimSpace(string(argv)) != "app-server" {
		t.Fatalf("app-server wasn't started by the check: %q %v", argv, err)
	}
	code, body := call(t, "POST", ts.URL+"/v1/chats", `{"agent":"codex"}`)
	if code != http.StatusCreated || body["chat"].(map[string]any)["agent"] != "codex" {
		t.Fatalf("create codex chat = %d %v", code, body)
	}
}

func TestAgentChatCodexAbsentWhenCodexIsNotSignedIn(t *testing.T) {
	reason := "Codex isn't signed in on this computer; run `codex login` there"
	s, fdir := codexChatServer(t, codex.Status{Version: "0.159.2", Reason: reason})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	_, health := call(t, "GET", ts.URL+"/v1/health", "")
	if hasFeature(health, "agent_chat_codex") || health["agent_chat_codex_unavailable"] != reason {
		t.Fatalf("health = %v", health)
	}
	if !hasFeature(health, "agent_chat") {
		t.Fatalf("Claude chat should stay available: %v", health)
	}
	code, body := call(t, "POST", ts.URL+"/v1/chats", `{"agent":"codex"}`)
	if code != http.StatusConflict || body["error"] != reason {
		t.Fatalf("create codex chat = %d %v", code, body)
	}
	if code, _ := call(t, "POST", ts.URL+"/v1/chats", `{"agent":"claude"}`); code != http.StatusCreated {
		t.Fatalf("create claude chat = %d", code)
	}
	if _, err := os.Stat(filepath.Join(fdir, "call0.argv")); err == nil {
		t.Error("app-server was started although Codex isn't signed in")
	}
}

func TestAgentChatCodexAbsentWhenAppServerDoesNotStart(t *testing.T) {
	s, _ := codexChatServer(t, codex.Status{Version: "0.159.2"})
	s.CodexAppServerCheck = func(context.Context) error { return errors.New("unknown command app-server") }
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	_, health := call(t, "GET", ts.URL+"/v1/health", "")
	why, _ := health["agent_chat_codex_unavailable"].(string)
	if hasFeature(health, "agent_chat_codex") || !strings.Contains(why, "app-server") || !strings.Contains(why, "unknown command") {
		t.Fatalf("health = %v", health)
	}
	if code, body := call(t, "POST", ts.URL+"/v1/chats", `{"agent":"codex"}`); code != http.StatusConflict || body["error"] != why {
		t.Fatalf("create codex chat = %d %v", code, body)
	}
}

func TestCodexChildNacredURLIsTheBoundAddress(t *testing.T) {
	ip := nonLoopbackIPv4(t)
	ln, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if err != nil {
		t.Skip(err)
	}
	s, fdir := codexChatServer(t, codex.Status{Version: "0.159.2"})
	s.Chats.SetNacredURL(NacredURL(ln))
	srv := &http.Server{Handler: s.Handler()}
	go srv.Serve(ln)
	defer srv.Close()

	want := "http://" + ln.Addr().String()
	code, body := call(t, "POST", want+"/v1/chats", `{"agent":"codex"}`)
	if code != http.StatusCreated {
		t.Fatalf("create = %d %v", code, body)
	}
	id := body["chat"].(map[string]any)["id"].(string)
	if code, b := call(t, "POST", want+"/v1/chats/"+id+"/messages", `{"turn_id":"c_codexurl","text":"ping nacred"}`); code != 202 {
		t.Fatalf("send: %d %v", code, b)
	}
	waitChatIdle(t, want, id)

	// The turn's app-server is the call that got turn/start (the feature
	// check started one too).
	matches, _ := filepath.Glob(filepath.Join(fdir, "call*.rpc"))
	found := false
	for _, rpc := range matches {
		data, _ := os.ReadFile(rpc)
		if !strings.Contains(string(data), `"turn/start"`) {
			continue
		}
		found = true
		env, _ := os.ReadFile(strings.TrimSuffix(rpc, ".rpc") + ".env")
		var urls []string
		for _, kv := range strings.Split(string(env), "\n") {
			if strings.HasPrefix(kv, "NACRED_URL=") {
				urls = append(urls, strings.TrimPrefix(kv, "NACRED_URL="))
			}
		}
		if len(urls) != 1 || urls[0] != want {
			t.Fatalf("child NACRED_URL = %v, want the bound address %q", urls, want)
		}
	}
	if !found {
		t.Fatal("no Codex turn ran")
	}
}
