package peer

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProbeTimeout bounds the connect probe the guard makes before a send.
const ProbeTimeout = 250 * time.Millisecond

// sendTimeout bounds one send: connect, write, half-close.
const sendTimeout = 2 * time.Second

// maxInboundLine caps one inbound frame. Receipts are a few hundred bytes.
const maxInboundLine = 64 << 10

// inboundDeadline is how long a receiver's connection to the inbox may stay
// open. Each receipt arrives on a connection of its own.
const inboundDeadline = 10 * time.Second

// Client sends follow-ups to live sessions and receives their receipts. It is
// safe for concurrent use.
type Client struct {
	// PID is this process's pid; it names the inbox. Default os.Getpid().
	PID int
	// KeyDir holds the sessions' <pid>.<hex>.key files (normally
	// ~/.claude/sessions); "" sends no auth line.
	KeyDir string
	// Host is the pidDomain a session on this machine records. Default
	// HostDomain().
	Host string
	// Alive reports whether pid is still the process that started at
	// procStart (a record's string). Required: without it Check refuses.
	Alive func(pid int, procStart string) bool
	// Handle receives every inbound frame. It runs on the inbox's goroutine.
	Handle func(Frame)
	Logger *log.Logger

	mu      sync.Mutex
	inboxes map[string]net.Listener // by socket directory
	closed  bool
}

func (c *Client) pid() int {
	if c.PID > 0 {
		return c.PID
	}
	return os.Getpid()
}

func (c *Client) host() string {
	if c.Host != "" {
		return c.Host
	}
	return HostDomain()
}

func (c *Client) logf(format string, args ...any) {
	if c.Logger != nil {
		c.Logger.Printf(format, args...)
	}
}

// Check is the whole version guard: CheckRecord, then the process is alive
// with the start time the record claims, then the socket accepts a connection.
func (c *Client) Check(t Target) error {
	if err := CheckRecord(t, c.host()); err != nil {
		return err
	}
	if c.Alive == nil || !c.Alive(t.PID, t.ProcStart) {
		return ineligible("session process %d is not running", t.PID)
	}
	if err := probe(t.SocketPath); err != nil {
		return ineligible("session socket does not answer: %v", err)
	}
	return nil
}

// probe connects to the socket and hangs up without writing.
func probe(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%s is not a socket", path)
	}
	conn, err := net.DialTimeout("unix", path, ProbeTimeout)
	if err != nil {
		return err
	}
	return conn.Close()
}

// SocketExists is the cheap part of the probe, for deciding whether to offer
// a live send without connecting to the session on every poll.
func SocketExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode()&os.ModeSocket != 0
}

// dialSend opens the connection a send writes to. Tests replace it to order
// the dial against the session going away.
var dialSend = func(path string) (net.Conn, error) {
	return net.DialTimeout("unix", path, sendTimeout)
}

// Sent is what a successful send leaves to track.
type Sent struct {
	MsgID     string
	IdleMsgID string // "" when no notify_when_idle was sent
	From      string
}

// PartialWrite is Send's error when the write to the session failed after
// some of the frames had gone out (NAC-jif). The session may already hold the
// message, so Sent carries its msg_ids: a later receipt or transcript entry
// can still be matched to it. A send that fails before any byte is written
// returns a plain error instead.
type PartialWrite struct {
	Sent    Sent
	Written int // bytes written before the failure
	Err     error
}

func (e *PartialWrite) Error() string {
	return fmt.Sprintf("write to the session failed after %d bytes: %v", e.Written, e.Err)
}

func (e *PartialWrite) Unwrap() error { return e.Err }

// Send runs the guard and delivers content to the session as one peer
// message, with a notify_when_idle subscription when the session offers it.
// A nil error means the frames were written, not that the session accepted
// them: that is what the receipts and the transcript say. A write that fails
// partway returns a *PartialWrite.
func (c *Client) Send(t Target, content string) (Sent, error) {
	if err := c.Check(t); err != nil {
		return Sent{}, err
	}
	inbox, err := c.inbox(filepath.Dir(t.SocketPath))
	if err != nil {
		return Sent{}, fmt.Errorf("bind inbox: %w", err)
	}
	s := Send{Token: c.token(t), From: "uds:" + inbox, MsgID: NewMsgID(), Content: content}
	if t.NotifyIdle() {
		s.IdleMsgID = NewMsgID()
	}
	data, err := EncodeSend(s)
	if err != nil {
		return Sent{}, err
	}
	conn, err := dialSend(t.SocketPath)
	if err != nil {
		return Sent{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(sendTimeout))
	sent := Sent{MsgID: s.MsgID, IdleMsgID: s.IdleMsgID, From: s.From}
	if n, err := conn.Write(data); err != nil {
		if n > 0 {
			return Sent{}, &PartialWrite{Sent: sent, Written: n, Err: err}
		}
		return Sent{}, err
	}
	if uc, ok := conn.(*net.UnixConn); ok {
		_ = uc.CloseWrite()
	}
	return sent, nil
}

// keyFile is the JSON in ~/.claude/sessions/<pid>.<hex>.key.
type keyFile struct {
	PeerToken string `json:"peerToken"`
	ProcStart string `json:"procStart"`
	PIDDomain string `json:"pidDomain"`
}

// token returns the session's peerToken, or "" when there is none to send.
// The auth line is optional on macOS and Linux, so a missing or mismatched
// key is not an error; it is sent for forward compatibility only. A key
// written for a different process start (a reused pid) is ignored.
func (c *Client) token(t Target) string {
	if c.KeyDir == "" {
		return ""
	}
	matches, _ := filepath.Glob(filepath.Join(c.KeyDir, strconv.Itoa(t.PID)+".*.key"))
	for _, m := range matches {
		data, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		var k keyFile
		if json.Unmarshal(data, &k) != nil || k.PeerToken == "" {
			continue
		}
		if k.ProcStart != "" && strings.TrimSpace(k.ProcStart) != strings.TrimSpace(t.ProcStart) {
			continue
		}
		return k.PeerToken
	}
	return ""
}

// InboxPath is where nacred's inbox lives for sessions whose sockets are in
// dir: the receiver only dials reply addresses in its own socket directory,
// named <pid>.sock, bound by the sending process.
func InboxPath(dir string, pid int) string {
	return filepath.Join(dir, strconv.Itoa(pid)+".sock")
}

// inbox returns the inbox for dir, binding it on first use. It stays bound
// until Close: a held message can be approved, and its receipt sent, long
// after the send.
func (c *Client) inbox(dir string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return "", errors.New("peer client closed")
	}
	path := InboxPath(dir, c.pid())
	if _, ok := c.inboxes[dir]; ok {
		return path, nil
	}
	if _, err := os.Lstat(path); err == nil {
		// A socket left by an earlier process with this pid. One that still
		// answers belongs to somebody else; leave it alone.
		if conn, err := net.DialTimeout("unix", path, ProbeTimeout); err == nil {
			conn.Close()
			return "", fmt.Errorf("%s is in use by another process", path)
		}
		if err := os.Remove(path); err != nil {
			return "", err
		}
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return "", err
	}
	if c.inboxes == nil {
		c.inboxes = map[string]net.Listener{}
	}
	c.inboxes[dir] = ln
	go c.serve(ln)
	return path, nil
}

func (c *Client) serve(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				c.logf("peer: inbox %s: %v", ln.Addr(), err)
			}
			return
		}
		go c.read(conn)
	}
}

// read handles one inbound connection: every complete line is a frame.
func (c *Client) read(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(inboundDeadline))
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 4096), maxInboundLine)
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		f, err := DecodeFrame(line)
		if err != nil {
			c.logf("peer: inbox: bad frame: %v", err)
			continue
		}
		if c.Handle != nil {
			c.Handle(f)
		}
	}
}

// Close unbinds every inbox and removes its socket file.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	for dir, ln := range c.inboxes {
		ln.Close() // a Go unix listener unlinks its socket on close
		delete(c.inboxes, dir)
	}
}
