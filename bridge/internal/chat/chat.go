// Package chat keeps the app's Chat conversations on this computer: one record
// and one event log per chat under nacred's state directory, and the full
// Claude Code turns that run in them, detached from the requests that start
// them.
package chat

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// States a chat can be in.
const (
	StateIdle              = "idle"
	StateRunning           = "running"
	StateWaitingPermission = "waiting_permission"
	StateInterrupted       = "interrupted"
	StateLimitReached      = "limit_reached"
)

// Agents a chat can run on.
const (
	AgentClaude = "claude"
	AgentCodex  = "codex"
)

// Event kinds besides the transcript kinds of api.RunEvent (text, thinking,
// tool_use, tool_result, system, result).
const (
	KindUser       = "user"
	KindPermission = "permission"
	KindError      = "error"
	KindTurnEnd    = "turn_end"
	KindTruncated  = "truncated"
	KindSystem     = "system"
)

// Routes an assistant event came from: a light turn (the router's quick
// answer) or the full harness.
const (
	RouteFull  = "full"
	RouteLight = "light"
)

const (
	// MaxEvents is how many events a chat's log keeps; older ones are
	// dropped behind one truncated marker.
	MaxEvents = 5000
	// MaxChats is how many chats are kept, by most recent update.
	MaxChats = 200
	// maxTurnIDs is how many turn ids a chat remembers for resends.
	maxTurnIDs = 200
	// titleLen bounds a chat's title.
	titleLen = 80
)

// LimitReason is the refusal once a chat has spent its runaway ceiling.
const LimitReason = "This chat reached its runaway limit. Start a new chat to keep going."

var (
	// ErrNotFound is an unknown chat.
	ErrNotFound = errors.New("unknown chat")
	// ErrBusy is a message sent while a turn runs.
	ErrBusy = errors.New("A turn is already running in this chat. Wait for it to finish or stop it.")
	// ErrLimit is a message sent to a chat that has reached its ceiling.
	ErrLimit = errors.New(LimitReason)
	// ErrCodexUnavailable is a message to a Codex chat on a computer
	// without the Codex CLI.
	ErrCodexUnavailable = errors.New("Codex chats can't run on this computer")
)

// Context is what was on the phone's screen when a message was sent.
type Context struct {
	// Screen is boards, inbox, activity, board, bead, change, round or run.
	Screen string `json:"screen"`
	// Repo is the repository key on this computer, or the key on another
	// computer when Elsewhere names it.
	Repo string `json:"repo,omitempty"`
	// Elsewhere names the computer holding Repo when it has no checkout on
	// the chat's computer.
	Elsewhere string `json:"elsewhere,omitempty"`
	BeadID    string `json:"bead_id,omitempty"`
	ChangeID  string `json:"change_id,omitempty"`
	RoundID   string `json:"round_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Selection string `json:"selection,omitempty"`
	Pane      string `json:"pane,omitempty"`
	// Summary is up to 4 KB of what the screen shows.
	Summary string `json:"summary,omitempty"`
}

// Record is one chat as stored in chats/<id>.json.
type Record struct {
	ID    string `json:"id"`
	Agent string `json:"agent"`
	// Repo is the repository the chat started in; nil for all boards.
	Repo     *string `json:"repo"`
	HostID   string  `json:"host_id,omitempty"`
	HostName string  `json:"host_name,omitempty"`
	// Cwd is fixed for the life of the chat.
	Cwd       string   `json:"cwd"`
	AddDirs   []string `json:"add_dirs"`
	Title     string   `json:"title"`
	NewEffort bool     `json:"new_effort,omitempty"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
	// SessionID is the agent's session: the Claude session UUID, or the
	// Codex thread id once thread/start has given one.
	SessionID string `json:"session_id,omitempty"`
	// SessionStarted is set once the agent has the session, so later turns
	// resume it rather than start it again.
	SessionStarted bool `json:"session_started,omitempty"`
	// LastContext is the context of the last message sent.
	LastContext *Context `json:"last_context,omitempty"`
	CostUSD     float64  `json:"cost_usd,omitempty"`
	ImageCount  int      `json:"image_count,omitempty"`
	ImageBytes  int64    `json:"image_bytes,omitempty"`
	State       string   `json:"state"`
	// Reason says why the chat is interrupted or limit_reached.
	Reason string `json:"reason,omitempty"`
	// TurnID is the running turn, or the last one.
	TurnID string `json:"turn_id,omitempty"`
	// LastSeq is the newest event's seq.
	LastSeq int64 `json:"last_seq"`
	// Phone is the Tailscale device of the phone that created the chat, for
	// Taildrop; empty when it couldn't be resolved.
	Phone string `json:"phone,omitempty"`
	// TurnIDs are the turns this chat has started, newest last, so a resend
	// is recognised.
	TurnIDs []string `json:"turn_ids,omitempty"`
}

// Event is one entry in a chat's log.
type Event struct {
	Seq    int64  `json:"seq"`
	TS     string `json:"ts"`
	Kind   string `json:"kind"`
	TurnID string `json:"turn_id,omitempty"`
	Text   string `json:"text,omitempty"`
	// Route is light or full on assistant events.
	Route   string          `json:"route,omitempty"`
	Tool    string          `json:"tool,omitempty"`
	ToolID  string          `json:"tool_id,omitempty"`
	Input   json.RawMessage `json:"input,omitempty"`
	IsError *bool           `json:"is_error,omitempty"`
	// Context is the screen a user message was sent from.
	Context *Context `json:"context,omitempty"`
	// RequestID, Allowed and Reason describe a permission request.
	RequestID string `json:"request_id,omitempty"`
	Allowed   *bool  `json:"allowed,omitempty"`
	Reason    string `json:"reason,omitempty"`
	// By says who decided a permission_answer: phone, timeout, stop,
	// restart or turn_end.
	By      string  `json:"by,omitempty"`
	CostUSD float64 `json:"cost_usd,omitempty"`
}

// chatState is a loaded chat.
type chatState struct {
	rec     Record
	events  []Event // nil until loaded
	loaded  bool
	changed chan struct{}
	deleted bool
	turn    *turn
	// pending are host permission requests waiting for the phone.
	pending map[string]*pending
}

// Manager holds every chat on this computer.
type Manager struct {
	opts Options

	mu    sync.Mutex
	chats map[string]*chatState
	// active counts running turns, for the update maintenance reservation.
	active int
	// rejected are chat models the CLI refused; routing stops using them.
	rejected map[string]bool
}

// Options configure a Manager.
type Options struct {
	// Dir is the chats directory under nacred's state directory.
	Dir        string
	ClaudePath string
	// CodexPath is the Codex CLI, whose app-server runs Codex chats.
	CodexPath string
	// NacredURL is http://<address nacred listens on>, given to every turn.
	NacredURL string
	// HostID and HostName name this computer.
	HostID, HostName string
	// RepoPath resolves a repository key to its checkout.
	RepoPath func(key string) (string, error)
	// Boards lists the checkouts of every board on this computer.
	Boards func() []string
	// Whois names the Tailscale device at an address; nil uses tailscale.
	Whois  func(addr string) (string, error)
	Logger *log.Logger
	// PermissionTimeout is how long a permission request waits for the
	// phone; zero is PermissionTimeout (15 minutes).
	PermissionTimeout time.Duration
	// RouterModel and LightModel are chat.router_model and
	// chat.light_model. Claude chat messages are routed only when both are
	// set.
	RouterModel, LightModel string
	// RouterTimeout bounds a routing call; zero is RouterTimeout (15s).
	RouterTimeout time.Duration
	// Now is the clock; nil is time.Now.
	Now func() time.Time
}

var idRe = regexp.MustCompile(`^ch_[a-f0-9]{24}$`)

// ValidID reports whether id can name a chat.
func ValidID(id string) bool { return idRe.MatchString(id) }

var turnIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{4,80}$`)

// ValidTurnID reports whether a client-chosen turn id is acceptable.
func ValidTurnID(id string) bool { return turnIDRe.MatchString(id) }

// New loads the chats in opts.Dir. Chats a previous nacred left running or
// waiting for permission are marked interrupted.
func New(opts Options) (*Manager, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Logger == nil {
		opts.Logger = log.New(os.Stderr, "", log.LstdFlags)
	}
	if err := os.MkdirAll(opts.Dir, 0o700); err != nil {
		return nil, err
	}
	m := &Manager{opts: opts, chats: map[string]*chatState{}}
	entries, err := os.ReadDir(opts.Dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if !ValidID(id) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(opts.Dir, name))
		if err != nil {
			continue
		}
		var rec Record
		if json.Unmarshal(data, &rec) != nil || rec.ID != id {
			opts.Logger.Printf("chat %s: unreadable record, skipped", id)
			continue
		}
		m.chats[id] = &chatState{rec: rec, changed: make(chan struct{})}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.chats {
		if c.rec.State == StateRunning || c.rec.State == StateWaitingPermission {
			m.closeUnansweredLocked(c, RestartReason, ByRestart)
			c.rec.State = StateInterrupted
			c.rec.Reason = "nacred restarted during this turn. Send a message to carry on."
			m.appendLocked(c, Event{Kind: KindSystem, TurnID: c.rec.TurnID, Text: "Interrupted: nacred restarted during this turn. The next message resumes the same session."})
			m.saveLocked(c)
		}
	}
	m.trimLocked()
	return m, nil
}

// SetNacredURL sets the address turns are given as NACRED_URL, once nacred
// knows where it is listening.
func (m *Manager) SetNacredURL(u string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.opts.NacredURL = u
}

// NacredURL is the address turns are given.
func (m *Manager) NacredURL() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.opts.NacredURL
}

// Busy reports whether any chat turn is running, which holds off updates.
func (m *Manager) Busy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active > 0
}

// CreateRequest is POST /v1/chats.
type CreateRequest struct {
	Agent     string   `json:"agent"`
	Repo      *string  `json:"repo"`
	Context   *Context `json:"context"`
	NewEffort bool     `json:"new_effort,omitempty"`
	// PeerAddr is the address the request came from, for the phone's
	// Tailscale device.
	PeerAddr string `json:"-"`
}

// ErrUnknownRepo is a create request for a repository not on this computer.
var ErrUnknownRepo = errors.New("unknown repo")

// Create makes a new chat. Its working directory is fixed here: the
// repository's checkout for a repo key, otherwise chats/work.
func (m *Manager) Create(req CreateRequest) (Record, error) {
	if req.Agent == "" {
		req.Agent = AgentClaude
	}
	cwd := m.workDir()
	title := "New chat · all boards"
	var repo *string
	if req.Repo != nil && *req.Repo != "" {
		if m.opts.RepoPath == nil {
			return Record{}, ErrUnknownRepo
		}
		p, err := m.opts.RepoPath(*req.Repo)
		if err != nil || p == "" {
			return Record{}, ErrUnknownRepo
		}
		cwd = p
		key := *req.Repo
		repo = &key
		title = "New chat · " + filepath.Base(p)
		if req.NewEffort {
			title = "New effort · " + filepath.Base(p)
		}
	} else if err := os.MkdirAll(cwd, 0o700); err != nil {
		return Record{}, err
	}
	var dirs []string
	if m.opts.Boards != nil {
		seen := map[string]bool{filepath.Clean(cwd): true}
		for _, b := range m.opts.Boards() {
			b = filepath.Clean(b)
			if b == "" || seen[b] {
				continue
			}
			seen[b] = true
			dirs = append(dirs, b)
		}
		sort.Strings(dirs)
	}
	phone := ""
	if req.PeerAddr != "" {
		whois := m.opts.Whois
		if whois == nil {
			whois = TailscaleWhois
		}
		if name, err := whois(req.PeerAddr); err == nil {
			phone = name
		}
	}
	now := m.opts.Now().UTC().Format(time.RFC3339Nano)
	rec := Record{
		ID: newID("ch_"), Agent: req.Agent, Repo: repo, HostID: m.opts.HostID, HostName: m.opts.HostName,
		Cwd: cwd, AddDirs: dirs, Title: title, NewEffort: req.NewEffort,
		CreatedAt: now, UpdatedAt: now, SessionID: newUUID(), State: StateIdle, Phone: phone,
	}
	if rec.Agent == AgentCodex {
		// The thread id comes from thread/start on the first message.
		rec.SessionID = ""
	}
	if rec.AddDirs == nil {
		rec.AddDirs = []string{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	c := &chatState{rec: rec, loaded: true, events: []Event{}, changed: make(chan struct{})}
	m.chats[rec.ID] = c
	if err := m.saveLocked(c); err != nil {
		delete(m.chats, rec.ID)
		return Record{}, err
	}
	m.trimLocked()
	return rec, nil
}

// List returns every chat, most recently updated first.
func (m *Manager) List() []Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Record, 0, len(m.chats))
	for _, c := range m.chats {
		out = append(out, c.rec)
	}
	sortNewest(out)
	return out
}

// Get returns a chat and its newest events (at most tail).
func (m *Manager) Get(id string, tail int) (Record, []Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.chats[id]
	if c == nil {
		return Record{}, nil, ErrNotFound
	}
	m.loadLocked(c)
	evs := c.events
	if tail > 0 && len(evs) > tail {
		evs = evs[len(evs)-tail:]
	}
	return c.rec, append([]Event{}, evs...), nil
}

// Snapshot is a chat's state for one read of its stream.
type Snapshot struct {
	Record Record
	// Events are those after the seq asked for.
	Events []Event
	// Changed is closed when there is something new.
	Changed <-chan struct{}
	Gone    bool
}

// Since returns the events after seq.
func (m *Manager) Since(id string, after int64) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.chats[id]
	if c == nil {
		return Snapshot{}, ErrNotFound
	}
	m.loadLocked(c)
	snap := Snapshot{Record: c.rec, Changed: c.changed, Gone: c.deleted}
	i := sort.Search(len(c.events), func(i int) bool { return c.events[i].Seq > after })
	snap.Events = append([]Event{}, c.events[i:]...)
	return snap, nil
}

// Delete stops a chat's turn and removes its record, log and directory.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	c := m.chats[id]
	if c == nil {
		m.mu.Unlock()
		return ErrNotFound
	}
	t := c.turn
	m.removeLocked(c)
	m.mu.Unlock()
	if t != nil {
		t.stop()
	}
	return nil
}

// removeLocked forgets a chat and deletes its files.
func (m *Manager) removeLocked(c *chatState) {
	id := c.rec.ID
	c.deleted = true
	for _, p := range c.pending {
		if p.timer != nil {
			p.timer.Stop()
		}
	}
	c.pending = nil
	delete(m.chats, id)
	_ = os.Remove(m.recordPath(id))
	_ = os.Remove(m.logPath(id))
	_ = os.RemoveAll(filepath.Join(m.opts.Dir, id))
	m.signalLocked(c)
}

// trimLocked keeps the MaxChats most recently updated chats. A chat with a
// running turn is never trimmed.
func (m *Manager) trimLocked() {
	if len(m.chats) <= MaxChats {
		return
	}
	all := make([]Record, 0, len(m.chats))
	for _, c := range m.chats {
		all = append(all, c.rec)
	}
	sortNewest(all)
	for _, r := range all[MaxChats:] {
		c := m.chats[r.ID]
		if c == nil || c.turn != nil {
			continue
		}
		m.removeLocked(c)
	}
}

func sortNewest(rs []Record) {
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].UpdatedAt != rs[j].UpdatedAt {
			return rs[i].UpdatedAt > rs[j].UpdatedAt
		}
		return rs[i].ID > rs[j].ID
	})
}

func (m *Manager) recordPath(id string) string { return filepath.Join(m.opts.Dir, id+".json") }
func (m *Manager) logPath(id string) string    { return filepath.Join(m.opts.Dir, id+".events.jsonl") }

// saveLocked writes the record atomically with mode 0600.
func (m *Manager) saveLocked(c *chatState) error {
	if c.deleted {
		return nil
	}
	data, err := json.MarshalIndent(c.rec, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(m.opts.Dir, ".chat-*")
	if err != nil {
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), m.recordPath(c.rec.ID)); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// loadLocked reads a chat's log the first time it is needed.
func (m *Manager) loadLocked(c *chatState) {
	if c.loaded {
		return
	}
	c.loaded = true
	c.events = []Event{}
	f, err := os.Open(m.logPath(c.rec.ID))
	if err != nil {
		return
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, 64*1024)
	for {
		line, err := br.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			var ev Event
			if json.Unmarshal(line, &ev) == nil {
				c.events = append(c.events, ev)
			}
		}
		if err != nil {
			break
		}
	}
	if n := len(c.events); n > 0 && c.events[n-1].Seq > c.rec.LastSeq {
		c.rec.LastSeq = c.events[n-1].Seq
	}
}

// appendLocked numbers ev, writes it to the log and only then makes it
// visible to readers. The log is trimmed to MaxEvents behind a truncated
// marker.
func (m *Manager) appendLocked(c *chatState, ev Event) Event {
	if c.deleted {
		return ev
	}
	m.loadLocked(c)
	c.rec.LastSeq++
	ev.Seq = c.rec.LastSeq
	ev.TS = m.opts.Now().UTC().Format(time.RFC3339Nano)
	line, err := json.Marshal(ev)
	if err == nil {
		f, ferr := os.OpenFile(m.logPath(c.rec.ID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if ferr == nil {
			_, err = f.Write(append(line, '\n'))
			f.Close()
		} else {
			err = ferr
		}
	}
	if err != nil {
		m.opts.Logger.Printf("chat %s: append event: %v", c.rec.ID, err)
	}
	c.events = append(c.events, ev)
	if m.countLocked(c) > MaxEvents {
		m.truncateLocked(c)
	}
	m.signalLocked(c)
	return ev
}

// countLocked is the number of real events (the marker doesn't count).
func (m *Manager) countLocked(c *chatState) int {
	n := len(c.events)
	if n > 0 && c.events[0].Kind == KindTruncated {
		n--
	}
	return n
}

// truncateLocked keeps the newest MaxEvents events behind one truncated
// marker, whose seq is the newest dropped event's, and rewrites the log.
func (m *Manager) truncateLocked(c *chatState) {
	evs := c.events
	dropped := 0
	if len(evs) > 0 && evs[0].Kind == KindTruncated {
		fmt.Sscanf(evs[0].Text, "%d", &dropped)
		evs = evs[1:]
	}
	cut := len(evs) - MaxEvents
	if cut <= 0 {
		return
	}
	dropped += cut
	kept := evs[cut:]
	marker := Event{
		Seq: kept[0].Seq - 1, TS: kept[0].TS, Kind: KindTruncated,
		Text: fmt.Sprintf("%d earlier events were dropped", dropped),
	}
	c.events = append([]Event{marker}, kept...)
	var buf bytes.Buffer
	for _, ev := range c.events {
		line, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	tmp := m.logPath(c.rec.ID) + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err == nil {
		if err := os.Rename(tmp, m.logPath(c.rec.ID)); err != nil {
			m.opts.Logger.Printf("chat %s: trim log: %v", c.rec.ID, err)
		}
	}
}

func (m *Manager) signalLocked(c *chatState) {
	close(c.changed)
	c.changed = make(chan struct{})
}

// touchLocked marks the chat updated and saves it.
func (m *Manager) touchLocked(c *chatState) {
	c.rec.UpdatedAt = m.opts.Now().UTC().Format(time.RFC3339Nano)
	if err := m.saveLocked(c); err != nil {
		m.opts.Logger.Printf("chat %s: save: %v", c.rec.ID, err)
	}
	m.signalLocked(c)
}

func newID(prefix string) string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:])
}

// newUUID is a random (version 4) UUID, as --session-id requires.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// trimTitle is a message's first line, up to titleLen runes.
func trimTitle(text string) string {
	line := strings.TrimSpace(text)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	r := []rune(line)
	if len(r) > titleLen {
		return string(r[:titleLen-1]) + "…"
	}
	return line
}
