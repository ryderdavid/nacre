package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/assist"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/chat"
	"github.com/ryderdavid/nacre/bridge/internal/codex"
	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/daemonupdate"
	"github.com/ryderdavid/nacre/bridge/internal/discovery"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
	"github.com/ryderdavid/nacre/bridge/internal/sse"
)

// chatClaude is a claude stand-in for chat turns: it reads the user message,
// runs body, answers, and waits for stdin to close.
func chatClaude(t *testing.T, body string) (bin, dir string) {
	t.Helper()
	dir = t.TempDir()
	bin = filepath.Join(dir, "claude")
	script := `#!/bin/sh
d="$(dirname "$0")"
IFS= read -r first
printf '{"type":"system","subtype":"init","session_id":"s"}\n'
` + body + `
printf '{"type":"assistant","message":{"content":[{"type":"text","text":"done here"}]}}\n'
printf '{"type":"result","subtype":"success","is_error":false,"result":"ok","total_cost_usd":0.5}\n'
cat > /dev/null
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, dir
}

// chatServer serves one repo with chats whose turns run bin.
func chatServer(t *testing.T, bin string) (s *Server, repoID string) {
	t.Helper()
	repoDir := t.TempDir()
	registry := repos.New(func() []string { return []string{repoDir} }, bd.New("/nonexistent", ""))
	chats, err := chat.New(chat.Options{
		Dir: t.TempDir(), ClaudePath: bin, NacredURL: "http://127.0.0.1:1", HostName: "Mac-Studio",
		RepoPath: registry.Path,
		Boards:   func() []string { return []string{repoDir} },
		Whois:    func(string) (string, error) { return "", fmt.Errorf("no tailscale") },
		Logger:   log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	s = &Server{
		Config: &config.Config{Token: "tok"},
		Repos:  registry,
		BD:     bd.New("/nonexistent", ""),
		Assist: &assist.Runner{ClaudePath: bin},
		Chats:  chats,
		Broker: sse.NewBroker(),
		Log:    log.New(io.Discard, "", 0),
	}
	return s, discovery.RepoID(repoDir)
}

func call(t *testing.T, method, url, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// sseEvents reads a chat stream to its end: the seqs of its events and the
// final message type.
func sseEvents(t *testing.T, url string) (seqs []int64, last string) {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	event := ""
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
			last = event
		case strings.HasPrefix(line, "data: ") && event == "event":
			var ev chat.Event
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
				t.Fatal(err)
			}
			seqs = append(seqs, ev.Seq)
		}
	}
	return seqs, last
}

func waitChatIdle(t *testing.T, base, id string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		_, body := call(t, "GET", base+"/v1/chats/"+id, "")
		rec, _ := body["chat"].(map[string]any)
		if st := rec["state"]; st != chat.StateRunning && st != chat.StateWaitingPermission {
			return rec
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("chat turn did not finish")
	return nil
}

func TestChatEndpoints(t *testing.T) {
	bin, _ := chatClaude(t, "sleep 0.3")
	s, repoID := chatServer(t, bin)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	if code, _ := call(t, "POST", ts.URL+"/v1/chats", `{"agent":"claude","repo":"nope"}`); code != 400 {
		t.Errorf("unknown repo: %d", code)
	}
	if code, body := call(t, "POST", ts.URL+"/v1/chats", `{"agent":"codex"}`); code != 409 || body["error"] == "" {
		t.Errorf("codex: %d %v", code, body)
	}
	code, body := call(t, "POST", ts.URL+"/v1/chats", `{"agent":"claude","repo":"`+repoID+`","context":{"screen":"board","repo":"`+repoID+`"}}`)
	if code != 201 {
		t.Fatalf("create: %d %v", code, body)
	}
	id := body["chat"].(map[string]any)["id"].(string)

	msg := `{"turn_id":"c_endpoint1","text":"what's open?","context":{"screen":"board","repo":"` + repoID + `"}}`
	code, body = call(t, "POST", ts.URL+"/v1/chats/"+id+"/messages", msg)
	if code != 202 || body["turn_id"] != "c_endpoint1" || body["started"] != true {
		t.Fatalf("send: %d %v", code, body)
	}
	// The same turn_id again is the same turn; another message is refused while it runs.
	code, body = call(t, "POST", ts.URL+"/v1/chats/"+id+"/messages", msg)
	if code != 202 || body["turn_id"] != "c_endpoint1" || body["started"] != false {
		t.Fatalf("resend: %d %v", code, body)
	}
	if code, _ := call(t, "POST", ts.URL+"/v1/chats/"+id+"/messages", `{"turn_id":"c_endpoint2","text":"more"}`); code != 409 {
		t.Fatalf("second message while running: %d", code)
	}
	// The stream follows the turn to its end.
	seqs, last := sseEvents(t, ts.URL+"/v1/chats/"+id+"/stream")
	if last != "state" || len(seqs) < 4 {
		t.Fatalf("stream: %v ending %q", seqs, last)
	}
	// Reattaching after a seq replays exactly the rest, by query or Last-Event-ID.
	cut := seqs[1]
	rest, _ := sseEvents(t, ts.URL+"/v1/chats/"+id+"/stream?after="+strconv.FormatInt(cut, 10))
	if fmt.Sprint(rest) != fmt.Sprint(seqs[2:]) {
		t.Fatalf("after %d: %v, want %v", cut, rest, seqs[2:])
	}
	req, _ := http.NewRequest("GET", ts.URL+"/v1/chats/"+id+"/stream", nil)
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Last-Event-ID", strconv.FormatInt(cut, 10))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if n := strings.Count(string(data), "event: event"); n != len(seqs)-2 {
		t.Fatalf("Last-Event-ID replayed %d events, want %d", n, len(seqs)-2)
	}

	rec := waitChatIdle(t, ts.URL, id)
	if rec["state"] != "idle" || rec["title"] != "what's open?" {
		t.Fatalf("record: %v", rec)
	}
	code, body = call(t, "GET", ts.URL+"/v1/chats", "")
	if list, _ := body["chats"].([]any); code != 200 || len(list) != 1 {
		t.Fatalf("list: %d %v", code, body)
	}
	if code, _ := call(t, "POST", ts.URL+"/v1/chats/"+id+"/cancel", ""); code != 204 {
		t.Fatalf("cancel: %d", code)
	}
	if code, _ := call(t, "DELETE", ts.URL+"/v1/chats/"+id, ""); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := call(t, "GET", ts.URL+"/v1/chats/"+id, ""); code != 404 {
		t.Fatalf("deleted chat: %d", code)
	}
}

func TestChatLimitReachedRefusesWithTheReason(t *testing.T) {
	bin, _ := chatClaude(t, "")
	s, _ := chatServer(t, bin)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	_, body := call(t, "POST", ts.URL+"/v1/chats", `{"agent":"claude"}`)
	id := body["chat"].(map[string]any)["id"].(string)
	// Spend $49.75 in earlier turns; the next $0.50 turn passes the ceiling.
	recPath := filepath.Join(s.Chats.List()[0].Cwd, "..", id+".json")
	data, _ := os.ReadFile(recPath)
	var rec map[string]any
	json.Unmarshal(data, &rec)
	rec["cost_usd"] = 49.75
	data, _ = json.Marshal(rec)
	os.WriteFile(recPath, data, 0o600)
	chats, err := chat.New(chat.Options{Dir: filepath.Dir(recPath), ClaudePath: bin, Logger: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	s.Chats = chats
	ts2 := httptest.NewServer(s.Handler())
	defer ts2.Close()
	if code, b := call(t, "POST", ts2.URL+"/v1/chats/"+id+"/messages", `{"turn_id":"c_limit0001","text":"go"}`); code != 202 {
		t.Fatalf("send: %d %v", code, b)
	}
	got := waitChatIdle(t, ts2.URL, id)
	if got["state"] != chat.StateLimitReached {
		t.Fatalf("state %v", got["state"])
	}
	code, b := call(t, "POST", ts2.URL+"/v1/chats/"+id+"/messages", `{"turn_id":"c_limit0002","text":"more"}`)
	if code != 409 || b["error"] != "This chat reached its runaway limit. Start a new chat to keep going." {
		t.Fatalf("at the limit: %d %v", code, b)
	}
}

func TestUpdateRefusedWhileAChatTurnRuns(t *testing.T) {
	bin, _ := chatClaude(t, "sleep 30")
	s, _ := chatServer(t, bin)
	s.Updates = daemonupdate.New(daemonupdate.Options{Dir: t.TempDir(), Launch: func(daemonupdate.Status) error { return nil }})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	_, body := call(t, "POST", ts.URL+"/v1/chats", `{"agent":"claude"}`)
	id := body["chat"].(map[string]any)["id"].(string)
	if code, _ := call(t, "POST", ts.URL+"/v1/chats/"+id+"/messages", `{"turn_id":"c_update001","text":"long job"}`); code != 202 {
		t.Fatal(code)
	}
	code, b := call(t, "POST", ts.URL+"/v1/daemon/update", `{}`)
	if code != 409 || !strings.Contains(fmt.Sprint(b["error"]), "assistant turn is running") || s.Updates.Get().Active {
		t.Fatalf("update during a chat turn: %d %v", code, b)
	}
	// A chat turn holds no run slot.
	if s.Runs != nil && s.Runs.ActiveCount("") != 0 {
		t.Fatal("chat turn took a run slot")
	}
	call(t, "POST", ts.URL+"/v1/chats/"+id+"/cancel", "")
	waitChatIdle(t, ts.URL, id)
	if code, b := call(t, "POST", ts.URL+"/v1/daemon/update", `{}`); code != 202 {
		t.Fatalf("update after the turn: %d %v", code, b)
	}
}

func TestChatsAreNotAdvertisedYet(t *testing.T) {
	bin, _ := chatClaude(t, "")
	s, _ := chatServer(t, bin)
	for _, f := range s.features(codex.Status{}) {
		if strings.HasPrefix(f, "agent_chat") {
			t.Fatalf("features advertise %s before host prompts are handled", f)
		}
	}
}

// nonLoopbackIPv4 is an address of this machine other than loopback.
func nonLoopbackIPv4(t *testing.T) string {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Skip(err)
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() && ipn.IP.To4() != nil && !ipn.IP.IsLinkLocalUnicast() {
			return ipn.IP.String()
		}
	}
	t.Skip("no non-loopback IPv4 address on this machine")
	return ""
}

func TestNacredURLIsTheBoundAddressAndReachesTheServer(t *testing.T) {
	ip := nonLoopbackIPv4(t)
	ln, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if err != nil {
		t.Skip(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	// Nothing listens on 127.0.0.1 at that port.
	if c, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", port), time.Second); err == nil {
		c.Close()
		ln.Close()
		t.Skip("something else listens on 127.0.0.1:" + port)
	}

	// The child asks nacred for its health at NACRED_URL with the owner's token.
	bin, dir := chatClaude(t, `curl -s -o "$d/health" -w '%{http_code}' -H "Authorization: Bearer tok" "$NACRED_URL/v1/chats" > "$d/code"; printf '%s' "$NACRED_URL" > "$d/url"`)
	s, _ := chatServer(t, bin)
	s.Chats.SetNacredURL(NacredURL(ln))
	srv := &http.Server{Handler: s.Handler()}
	go srv.Serve(ln)
	defer srv.Close()

	want := "http://" + net.JoinHostPort(ip, port)
	if got := s.Chats.NacredURL(); got != want {
		t.Fatalf("NACRED_URL = %q, want the bound address %q", got, want)
	}
	_, body := call(t, "POST", want+"/v1/chats", `{"agent":"claude"}`)
	id := body["chat"].(map[string]any)["id"].(string)
	if code, b := call(t, "POST", want+"/v1/chats/"+id+"/messages", `{"turn_id":"c_nacredurl","text":"ping nacred"}`); code != 202 {
		t.Fatalf("send: %d %v", code, b)
	}
	waitChatIdle(t, want, id)
	url, _ := os.ReadFile(filepath.Join(dir, "url"))
	if string(url) != want {
		t.Fatalf("child NACRED_URL = %q, want %q", url, want)
	}
	code, _ := os.ReadFile(filepath.Join(dir, "code"))
	health, _ := os.ReadFile(filepath.Join(dir, "health"))
	if string(code) != "200" || !strings.Contains(string(health), `"chats"`) {
		t.Fatalf("child's request: %q %q", code, health)
	}
}
