package server

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/chat"
	"github.com/ryderdavid/nacre/bridge/internal/chat/fakeclaude"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
	"github.com/ryderdavid/nacre/bridge/internal/codex"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
)

func TestMain(m *testing.M) {
	fakeclaude.RunIfFake()
	os.Exit(m.Run())
}

const (
	phoneAddr = "100.101.102.103:51234"
	macURL    = "http://100.64.0.7:7420"
)

// promptServer is a server whose chat is waiting on the recorded host
// permission request. NACRED_URL is the Mac's own Tailscale address.
func promptServer(t *testing.T) (s *Server, h http.Handler, id, reqID, fdir string) {
	t.Helper()
	bin, fdir := fakeclaude.Install(t, "../chat/testdata")
	s, _ = chatServer(t, bin)
	s.Chats.SetNacredURL(macURL)
	h = s.Handler()
	code, body := serve(t, h, "POST", "/v1/chats", `{"agent":"claude"}`, phoneAddr)
	if code != http.StatusCreated {
		t.Fatalf("create = %d %v", code, body)
	}
	id = body["chat"].(map[string]any)["id"].(string)
	if code, body := serve(t, h, "POST", "/v1/chats/"+id+"/messages", `{"turn_id":"c_perm0001","text":"push it"}`, phoneAddr); code != http.StatusAccepted {
		t.Fatalf("send = %d %v", code, body)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		rec, evs, _ := s.Chats.Get(id, 0)
		if rec.State == chat.StateWaitingPermission {
			for _, ev := range evs {
				if ev.Kind == chat.KindPermission {
					return s, h, id, ev.RequestID, fdir
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no permission request")
	return
}

// serve runs one request through h as if it came from remote.
func serve(t *testing.T, h http.Handler, method, path, body, remote string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	req.RemoteAddr = remote
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	var out map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	return rr.Code, out
}

func chatState(t *testing.T, s *Server, id string) string {
	t.Helper()
	rec, _, err := s.Chats.Get(id, 0)
	if err != nil {
		t.Fatal(err)
	}
	return rec.State
}

func waitChatDone(t *testing.T, s *Server, id string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		st := chatState(t, s, id)
		if st != chat.StateRunning && st != chat.StateWaitingPermission {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("turn did not finish")
}

// noChatTurnLeft checks no chat turn is counted and the CLI has exited.
func noChatTurnLeft(t *testing.T, s *Server, fdir string) {
	t.Helper()
	if s.Chats.Busy() {
		t.Error("a chat turn is still running")
	}
	data, err := os.ReadFile(filepath.Join(fdir, "pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(string(data))
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("claude process %d is still running", pid)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestPhoneAllowsAndASecondAnswerGets409(t *testing.T) {
	s, h, id, reqID, fdir := promptServer(t)
	path := "/v1/chats/" + id + "/permissions/" + reqID
	code, body := serve(t, h, "POST", path, `{"allow":true}`, phoneAddr)
	if code != http.StatusOK {
		t.Fatalf("allow = %d %v", code, body)
	}
	perm := body["permission"].(map[string]any)
	if perm["kind"] != chat.KindPermissionAnswer || perm["allowed"] != true || perm["by"] != "phone" {
		t.Fatalf("answer = %v", perm)
	}
	code, body = serve(t, h, "POST", path, `{"allow":false,"note":"wait"}`, "100.101.102.104:1000")
	if code != http.StatusConflict || body["permission"].(map[string]any)["allowed"] != true {
		t.Fatalf("second answer = %d %v", code, body)
	}
	waitChatDone(t, s, id)
	if _, err := os.Stat(filepath.Join(fdir, "ran")); err != nil {
		t.Fatal("the allowed step did not run")
	}
	if code, _ := serve(t, h, "POST", path, `{"allow":true}`, phoneAddr); code != http.StatusConflict {
		t.Fatalf("answer after the turn = %d", code)
	}
	if code, _ := serve(t, h, "POST", "/v1/chats/"+id+"/permissions/nope", `{"allow":true}`, phoneAddr); code != http.StatusNotFound {
		t.Fatalf("unknown request = %d", code)
	}
	if code, _ := serve(t, h, "POST", path, `{"note":"x"}`, phoneAddr); code != http.StatusBadRequest && code != http.StatusConflict {
		t.Fatalf("missing allow = %d", code)
	}
	noChatTurnLeft(t, s, fdir)
}

func TestPhoneDeniesWithANote(t *testing.T) {
	s, h, id, reqID, fdir := promptServer(t)
	code, body := serve(t, h, "POST", "/v1/chats/"+id+"/permissions/"+reqID, `{"allow":false,"note":"use the debug build"}`, phoneAddr)
	if code != http.StatusOK {
		t.Fatalf("deny = %d %v", code, body)
	}
	waitChatDone(t, s, id)
	if chatState(t, s, id) != chat.StateIdle {
		t.Fatalf("state = %s", chatState(t, s, id))
	}
	data, _ := os.ReadFile(filepath.Join(fdir, "answer"))
	if !strings.Contains(string(data), `"behavior":"deny"`) || !strings.Contains(string(data), `"message":"use the debug build"`) {
		t.Fatalf("answer written = %s", data)
	}
	noChatTurnLeft(t, s, fdir)
}

func TestAnswersFromThisComputerAreRefused(t *testing.T) {
	s, h, id, reqID, fdir := promptServer(t)
	path := "/v1/chats/" + id + "/permissions/" + reqID
	for _, from := range []string{"127.0.0.1:40000", "[::1]:40000", "100.64.0.7:40000"} {
		code, body := serve(t, h, "POST", path, `{"allow":true}`, from)
		if code != http.StatusForbidden || body["error"] != "Answer from a paired device" {
			t.Fatalf("answer from %s = %d %v", from, code, body)
		}
		if st := chatState(t, s, id); st != chat.StateWaitingPermission {
			t.Fatalf("after an answer from %s the chat is %s", from, st)
		}
	}
	// One of the computer's own interface addresses is refused too.
	s.LocalAddrs = func() []net.IP { return []net.IP{net.ParseIP("192.168.1.20")} }
	if code, _ := serve(t, h, "POST", path, `{"allow":true}`, "192.168.1.20:40000"); code != http.StatusForbidden {
		t.Fatalf("answer from an own interface = %d", code)
	}
	if _, err := os.Stat(filepath.Join(fdir, "answer")); err == nil {
		t.Fatal("a refused answer reached the CLI")
	}
	if code, _ := serve(t, h, "POST", path, `{"allow":true}`, phoneAddr); code != http.StatusOK {
		t.Fatalf("the phone's answer = %d", code)
	}
	waitChatDone(t, s, id)
	noChatTurnLeft(t, s, fdir)
}

// The agent itself, inside the chat's turn, posts to NACRED_URL: that is the
// computer's own address, so it is refused.
func TestAgentCantAnswerItsOwnRequestThroughNacredURL(t *testing.T) {
	ip := nonLoopbackIPv4(t)
	s, h, id, reqID, fdir := promptServer(t)
	ln, err := net.Listen("tcp", ip+":0")
	if err != nil {
		t.Skip(err)
	}
	srv := &http.Server{Handler: h}
	go srv.Serve(ln)
	defer srv.Close()
	s.Chats.SetNacredURL(NacredURL(ln))
	s.LocalAddrs = func() []net.IP { return nil } // only NACRED_URL names the computer
	req, _ := http.NewRequest("POST", NacredURL(ln)+"/v1/chats/"+id+"/permissions/"+reqID, strings.NewReader(`{"allow":true}`))
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := (&http.Client{Transport: &http.Transport{
		DialContext: (&net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(ip)}}).DialContext,
	}}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("answer through NACRED_URL = %d", resp.StatusCode)
	}
	if st := chatState(t, s, id); st != chat.StateWaitingPermission {
		t.Fatalf("state = %s", st)
	}
	serve(t, h, "POST", "/v1/chats/"+id+"/cancel", "", phoneAddr)
	waitChatDone(t, s, id)
	noChatTurnLeft(t, s, fdir)
}

func TestCancelWhilePendingEndsTheTurn(t *testing.T) {
	s, h, id, reqID, fdir := promptServer(t)
	if code, _ := serve(t, h, "POST", "/v1/chats/"+id+"/cancel", "", phoneAddr); code != http.StatusNoContent {
		t.Fatalf("cancel = %d", code)
	}
	waitChatDone(t, s, id)
	if st := chatState(t, s, id); st != chat.StateIdle {
		t.Fatalf("state = %s", st)
	}
	code, body := serve(t, h, "POST", "/v1/chats/"+id+"/permissions/"+reqID, `{"allow":true}`, phoneAddr)
	if code != http.StatusConflict || body["permission"].(map[string]any)["by"] != "stop" {
		t.Fatalf("answer after cancel = %d %v", code, body)
	}
	noChatTurnLeft(t, s, fdir)
}

func TestAgentChatFollowsCLISupport(t *testing.T) {
	bin, _ := chatClaude(t, "")
	s, _ := chatServer(t, bin)
	has := func() bool {
		for _, f := range s.features(codex.Status{}, s.chatSupport(context.Background())) {
			if f == "agent_chat" {
				return true
			}
		}
		return false
	}
	if !has() {
		t.Fatal("agent_chat missing although the CLI supports host prompts")
	}

	old := claude.ChatSupportFromHelp("2.0.1", "  --permission-mode <mode>  ...\n")
	s2, _ := chatServer(t, bin)
	s2.ChatCheck = func(context.Context) claude.ChatSupport { return old }
	for _, f := range s2.features(codex.Status{}, s2.chatSupport(context.Background())) {
		if f == "agent_chat" {
			t.Fatal("agent_chat advertised although the CLI lacks host prompts")
		}
	}
	if !strings.Contains(old.Reason, "Claude Code "+claude.ChatMinVersion+" or later") || !strings.Contains(old.Reason, "2.0.1") {
		t.Fatalf("reason = %q", old.Reason)
	}
	mgr, err := runs.New(runs.Options{Dir: t.TempDir(), ClaudePath: bin})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mgr.Shutdown(time.Second) })
	s2.Runs = mgr
	s2.CodexCheck = func(context.Context) codex.Status { return codex.Status{Reason: "not installed"} }
	ts := httptest.NewServer(s2.Handler())
	defer ts.Close()
	_, health := call(t, "GET", ts.URL+"/v1/health", "")
	if health["agent_chat_unavailable"] != old.Reason {
		t.Fatalf("health = %v", health)
	}
	code, body := call(t, "POST", ts.URL+"/v1/chats", `{"agent":"claude"}`)
	if code != http.StatusConflict || body["error"] != old.Reason {
		t.Fatalf("create without support = %d %v", code, body)
	}

	// The real --help of a CLI with host prompts.
	ok := claude.ChatSupportFromHelp("2.1.318", `  --permission-prompts <target>         Who answers permission prompts with
                                        --print: "host" (the SDK host or
                                        --permission-prompt-tool) or "none"`)
	if !ok.OK {
		t.Fatalf("support from help = %+v", ok)
	}
	// Without Chat set up nothing is advertised.
	s3, _ := chatServer(t, bin)
	s3.Chats = nil
	for _, f := range s3.features(codex.Status{}, claude.ChatSupport{OK: true}) {
		if f == "agent_chat" {
			t.Fatal("agent_chat advertised without Chat")
		}
	}
}
