package peer

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// sockDir returns a short directory for unix sockets: t.TempDir() on macOS is
// too long for the 104-byte sun_path.
func sockDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "nacpeer")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// good is a record that passes CheckRecord on host "darwin".
func good(sock string) Target {
	return Target{
		PID: 4242, ProcStart: "Sat Oct 24 00:25:54 2026", Kind: KindInteractive, Entrypoint: "cli",
		PIDDomain: "darwin", SocketPath: sock, Protocol: 1,
		Features: []string{"notify_idle", "reply_across_default_dirs", "artifact_yield"},
	}
}

func TestCheckRecordGuard(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Target)
		want   string // "" = eligible; else a substring of the reason
	}{
		{"tested shape", func(*Target) {}, ""},
		{"unknown peer protocol", func(t *Target) { t.Protocol = 2 }, "unsupported peer protocol 2"},
		{"absent peer protocol", func(t *Target) { t.Protocol = 0 }, "unsupported peer protocol 0"},
		{"desktop entrypoint", func(t *Target) { t.Entrypoint = EntrypointDesktop }, "desktop app"},
		{"missing socket", func(t *Target) { t.SocketPath = "" }, "no messaging socket"},
		{"other host", func(t *Target) { t.PIDDomain = "linux" }, "not on this computer"},
		{"no pid domain", func(t *Target) { t.PIDDomain = "" }, "not on this computer"},
		{"not interactive", func(t *Target) { t.Kind = "bridge" }, "not an interactive session"},
		{"no proc start", func(t *Target) { t.ProcStart = "" }, "no pid or process start"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tg := good("/tmp/cc-socks/4242.sock")
			c.mutate(&tg)
			err := CheckRecord(tg, "darwin")
			if c.want == "" {
				if err != nil {
					t.Fatalf("CheckRecord = %v, want eligible", err)
				}
				return
			}
			var in *Ineligible
			if !errors.As(err, &in) || !strings.Contains(in.Reason, c.want) {
				t.Fatalf("CheckRecord = %v, want an Ineligible containing %q", err, c.want)
			}
		})
	}
}

// listen binds a unix socket that records each connection's lines.
func listen(t *testing.T, path string) (got func() [][]string) {
	t.Helper()
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	var mu sync.Mutex
	var conns [][]string
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			var lines []string
			sc := bufio.NewScanner(c)
			for sc.Scan() {
				lines = append(lines, sc.Text())
			}
			c.Close()
			mu.Lock()
			conns = append(conns, lines)
			mu.Unlock()
		}
	}()
	return func() [][]string {
		mu.Lock()
		defer mu.Unlock()
		return append([][]string(nil), conns...)
	}
}

func TestCheckLivenessAndProbe(t *testing.T) {
	dir := sockDir(t)
	sock := filepath.Join(dir, "4242.sock")
	alive := true
	c := &Client{PID: 777, Host: "darwin", Alive: func(pid int, start string) bool { return alive && pid == 4242 }}

	// Missing socket file: the record names one, but nothing is there.
	if err := c.Check(good(sock)); err == nil || !strings.Contains(err.Error(), "does not answer") {
		t.Fatalf("Check with no socket = %v", err)
	}
	listen(t, sock)
	if err := c.Check(good(sock)); err != nil {
		t.Fatalf("Check = %v, want eligible", err)
	}
	// Dead pid (or a reused one whose start time does not match).
	alive = false
	if err := c.Check(good(sock)); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("Check with dead pid = %v", err)
	}
	// No liveness check at all is a refusal, not a pass.
	if err := (&Client{Host: "darwin"}).Check(good(sock)); err == nil {
		t.Fatal("Check without Alive must refuse")
	}
}

func TestEncodeSend(t *testing.T) {
	got, err := EncodeSend(Send{Token: "abc123", From: "uds:/tmp/cc-socks/777.sock", MsgID: "m1", Content: "Ryder (via Nacre): hi", IdleMsgID: "m2"})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"type":"auth","token":"abc123"}` + "\n" +
		`{"type":"user","message":{"role":"user","content":"Ryder (via Nacre): hi"},"from":"uds:/tmp/cc-socks/777.sock","msg_id":"m1"}` + "\n" +
		`{"type":"control","action":"notify_when_idle","from":"uds:/tmp/cc-socks/777.sock","msg_id":"m2"}` + "\n"
	if string(got) != want {
		t.Fatalf("EncodeSend =\n%s\nwant\n%s", got, want)
	}

	// No token, no idle subscription: the user frame alone.
	got, err = EncodeSend(Send{From: "uds:/x/1.sock", MsgID: "m1", Content: "line one\nline two"})
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Split(strings.TrimSuffix(string(got), "\n"), "\n"); len(lines) != 1 {
		t.Fatalf("a newline in the content must stay inside one NDJSON line: %q", got)
	}

	if _, err := EncodeSend(Send{MsgID: "m1"}); err == nil {
		t.Fatal("a send without a reply address must be refused")
	}
}

func TestDecodeFrameAndMsgIDs(t *testing.T) {
	f, err := DecodeFrame([]byte(`{"type":"control","action":"peer_message_status","status":"dropped","drop_reason":"queue full","dropped_msg_ids":["a","b"],"from":"uds:/tmp/cc-socks/54913.sock","orig_msg_id":"a","msgV":1,"msg_id":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Action != ActionMessageStatus || f.Status != StatusDropped || f.DropReason != "queue full" || f.MsgV != 1 {
		t.Fatalf("frame = %+v", f)
	}
	if ids := f.MsgIDs(); strings.Join(ids, ",") != "a,b" {
		t.Fatalf("MsgIDs = %v", ids)
	}
	if _, err := DecodeFrame([]byte(`{"action":"x"}`)); err == nil {
		t.Fatal("a frame without a type must not decode")
	}
}

func TestNewMsgIDIsAUUID(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	a, b := NewMsgID(), NewMsgID()
	if !re.MatchString(a) || a == b {
		t.Fatalf("NewMsgID = %q, %q", a, b)
	}
}

// TestSendRoundTrip drives a fake session: it reads nacred's frames, then
// dials the reply address back with a receipt, as Claude Code does.
func TestSendRoundTrip(t *testing.T) {
	dir := sockDir(t)
	sock := filepath.Join(dir, "4242.sock")
	keys := t.TempDir()
	tg := good(sock)
	// A key for this process start, and a stale one for an earlier process
	// with the same pid.
	os.WriteFile(filepath.Join(keys, "4242.aaaa.key"), []byte(`{"peerToken":"stale","procStart":"Fri Oct 23 00:00:00 2026","pidDomain":"darwin"}`), 0o600)
	os.WriteFile(filepath.Join(keys, "4242.bbbb.key"), []byte(`{"peerToken":"tok","procStart":"Sat Oct 24 00:25:54 2026","pidDomain":"darwin"}`), 0o600)

	got := listen(t, sock)
	frames := make(chan Frame, 4)
	c := &Client{PID: 777, Host: "darwin", KeyDir: keys,
		Alive:  func(int, string) bool { return true },
		Handle: func(f Frame) { frames <- f }}
	defer c.Close()

	sent, err := c.Send(tg, "Ryder (via Nacre): hello")
	if err != nil {
		t.Fatal(err)
	}
	inbox := filepath.Join(dir, "777.sock")
	if sent.From != "uds:"+inbox || sent.MsgID == "" || sent.IdleMsgID == "" {
		t.Fatalf("sent = %+v", sent)
	}
	fi, err := os.Stat(inbox)
	if err != nil || fi.Mode()&os.ModeSocket == 0 || fi.Mode().Perm() != 0o600 {
		t.Fatalf("inbox %s: %v %v, want a 0600 socket", inbox, fi, err)
	}

	var lines []string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		// The probe is a connection with no lines; the send is the next one.
		for _, conn := range got() {
			if len(conn) > 0 {
				lines = conn
			}
		}
		if lines != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(lines) != 3 {
		t.Fatalf("send lines = %q", lines)
	}
	var auth, user, idle Frame
	json.Unmarshal([]byte(lines[0]), &auth)
	json.Unmarshal([]byte(lines[1]), &user)
	json.Unmarshal([]byte(lines[2]), &idle)
	if auth.Type != "auth" || auth.Token != "tok" {
		t.Fatalf("auth line = %s: want the key for this process start", lines[0])
	}
	if user.Type != "user" || user.Message == nil || user.Message.Content != "Ryder (via Nacre): hello" || user.MsgID != sent.MsgID || user.From != sent.From {
		t.Fatalf("user line = %s", lines[1])
	}
	if idle.Action != ActionNotifyWhenIdle || idle.MsgID != sent.IdleMsgID {
		t.Fatalf("idle line = %s", lines[2])
	}

	// The session answers on a connection of its own.
	conn, err := net.Dial("unix", inbox)
	if err != nil {
		t.Fatal(err)
	}
	conn.Write([]byte(`{"type":"control","action":"peer_message_status","status":"held","reason":"held for approval","orig_msg_id":"` + sent.MsgID + `","msgV":1,"msg_id":"r1"}` + "\n"))
	conn.Close()
	select {
	case f := <-frames:
		if f.Status != StatusHeld || f.OrigMsgID != sent.MsgID {
			t.Fatalf("receipt = %+v", f)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no receipt reached Handle")
	}

	c.Close()
	if _, err := os.Stat(inbox); !os.IsNotExist(err) {
		t.Fatalf("inbox left behind after Close: %v", err)
	}
}

func TestSendWithoutNotifyIdleOrKey(t *testing.T) {
	dir := sockDir(t)
	sock := filepath.Join(dir, "4242.sock")
	got := listen(t, sock)
	tg := good(sock)
	tg.Features = nil
	c := &Client{PID: 778, Host: "darwin", Alive: func(int, string) bool { return true }}
	defer c.Close()
	sent, err := c.Send(tg, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if sent.IdleMsgID != "" {
		t.Fatalf("idle subscription sent to a session without notify_idle: %+v", sent)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, conn := range got() {
			if len(conn) > 0 {
				if len(conn) != 1 || !strings.Contains(conn[0], `"type":"user"`) {
					t.Fatalf("lines = %q, want the user frame alone", conn)
				}
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("nothing was sent")
}

func TestSendRefusedByGuardWritesNothing(t *testing.T) {
	dir := sockDir(t)
	sock := filepath.Join(dir, "4242.sock")
	got := listen(t, sock)
	tg := good(sock)
	tg.Protocol = 2
	c := &Client{PID: 779, Host: "darwin", Alive: func(int, string) bool { return true }}
	defer c.Close()
	var in *Ineligible
	if _, err := c.Send(tg, "hi"); !errors.As(err, &in) {
		t.Fatalf("Send = %v, want Ineligible", err)
	}
	time.Sleep(50 * time.Millisecond)
	if n := len(got()); n != 0 {
		t.Fatalf("%d connections made to an ineligible session", n)
	}
	if _, err := os.Stat(filepath.Join(dir, "779.sock")); !os.IsNotExist(err) {
		t.Fatal("inbox bound for a refused send")
	}
}

func TestStaleInboxIsReplaced(t *testing.T) {
	dir := sockDir(t)
	sock := filepath.Join(dir, "4242.sock")
	listen(t, sock)
	// A dead socket file left by an earlier process with our pid.
	stale := filepath.Join(dir, "780.sock")
	ln, err := net.Listen("unix", stale)
	if err != nil {
		t.Fatal(err)
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	ln.Close()
	c := &Client{PID: 780, Host: "darwin", Alive: func(int, string) bool { return true }}
	defer c.Close()
	if _, err := c.Send(good(sock), "hi"); err != nil {
		t.Fatalf("Send over a stale inbox = %v", err)
	}
}
