package claude

// On-disk session transcripts are a second, different encoding of a Claude Code
// session from the `--output-format stream-json` stream that ParseLine reads.
// Claude Code appends one to ~/.claude/projects/<escaped-cwd>/<session-id>.jsonl
// for every session on the machine, including sessions nacred did not start, so
// tailing one is how an agent-owned session becomes a monitorable run.
//
// The differences that matter:
//
//   - Entries are {type, message, cwd, sessionId, timestamp, uuid, parentUuid,
//     isSidechain, toolUseResult, ...}. There is no system/init entry and no
//     result entry, so neither Init nor Result can be recovered.
//   - Cost and duration come from a periodically rewritten cost-state entry
//     holding running totals, not from a per-turn result.
//   - Roughly twenty further entry types (attachment, api-request*, last-prompt,
//     file-history-*, artifact-*, ...) carry Claude Code's own bookkeeping and
//     must be ignored rather than rendered.
//   - "message" is the same Anthropic message shape, so blocks(), ToolSummary(),
//     toolResultText(), OneLine() and Truncate() are reused as-is.
//
// Nothing here talks to the run manager: a TranscriptTail is a pull-based source
// of api.RunEvents that the session-registry watcher drives.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

// Limits applied when translating a transcript. They exist because a transcript
// is an unbounded append-only log — several MB and tens of thousands of entries
// for a long session — while a run's events are held in memory and replayed over
// SSE on every reconnect.
const (
	// MaxTranscriptEvents is the default number of events a TranscriptTail
	// retains. Older events are evicted and counted in TranscriptState.Dropped.
	MaxTranscriptEvents = 1000
	// MaxTranscriptText is the rune cap on user, text and thinking events.
	MaxTranscriptText = 8000
	// MaxTranscriptToolInput is the byte cap on a retained tool_use input.
	// A larger input is dropped to "{}"; the one-line summary survives.
	MaxTranscriptToolInput = 16 << 10
	// MaxTranscriptLine is the longest line parsed. api-request-shape entries
	// embed the whole system prompt and tool schema and can be megabytes;
	// skipping them unparsed keeps the read cheap.
	MaxTranscriptLine = 4 << 20
	// TranscriptIdleAfter is how long a live session may go without writing to
	// its transcript before DeriveSessionStatus calls it idle.
	TranscriptIdleAfter = 2 * time.Minute
)

// Activity is the finer-grained state of an agent-owned session. It is separate
// from the five api run statuses because the app keys its own handling (cancel,
// notifications, the foreground service) on those.
const (
	ActivityWorking = "working" // alive and writing to its transcript
	ActivityIdle    = "idle"    // alive but parked, or quiet for a while
	ActivityEnded   = "ended"   // the owning process is gone
)

// TranscriptCost is the running total from a cost-state entry. The totals are
// absolute, not deltas: a later cost-state entry replaces an earlier one.
type TranscriptCost struct {
	CostUSD        float64
	DurationMS     int64
	ToolDurationMS int64
	LinesAdded     int
	LinesRemoved   int
	Models         []string // model names seen in modelUsage
}

// TranscriptParsed is what one transcript entry contributes to a run. Events
// carry no Seq or TS; TranscriptTail assigns those.
type TranscriptParsed struct {
	Events []api.RunEvent
	// Type is the raw entry type, for diagnostics.
	Type string
	// Ignored reports that the entry type is Claude Code bookkeeping we
	// deliberately do not translate.
	Ignored bool
	// Sidechain marks a subagent turn (isSidechain).
	Sidechain bool
	// Assistant marks a model turn, whether or not it produced events.
	Assistant bool

	SessionID string
	CWD       string
	Model     string
	Version   string
	GitBranch string
	Title     string // custom-title
	AgentName string // agent-name
	TS        string // entry timestamp, as written
	Cost      *TranscriptCost
}

// transcriptEntry is one line of a session transcript. Only the fields the
// adapter uses are declared; everything else is ignored by encoding/json.
type transcriptEntry struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype"`
	SessionID string          `json:"sessionId"`
	Timestamp string          `json:"timestamp"`
	CWD       string          `json:"cwd"`
	Version   string          `json:"version"`
	GitBranch string          `json:"gitBranch"`
	Message   json.RawMessage `json:"message"`

	IsSidechain      bool `json:"isSidechain"`
	IsMeta           bool `json:"isMeta"`
	IsCompactSummary bool `json:"isCompactSummary"`
	IsAPIError       bool `json:"isApiErrorMessage"`

	// assistant error entries
	Error string `json:"error"`

	// system entries
	Content    json.RawMessage `json:"content"`
	Level      string          `json:"level"`
	HookErrors json.RawMessage `json:"hookErrors"`

	// custom-title / agent-name
	CustomTitle string `json:"customTitle"`
	AgentName   string `json:"agentName"`

	// cost-state
	TotalCostUSD      float64                    `json:"totalCostUSD"`
	TotalDuration     int64                      `json:"totalDuration"`
	TotalToolDuration int64                      `json:"totalToolDuration"`
	TotalLinesAdded   int                        `json:"totalLinesAdded"`
	TotalLinesRemoved int                        `json:"totalLinesRemoved"`
	ModelUsage        map[string]json.RawMessage `json:"modelUsage"`
}

// ParseTranscriptEntry parses one line of an on-disk session transcript. Blank
// lines and entry types we do not translate yield an empty (or Ignored) result
// and no error; malformed JSON is an error.
func ParseTranscriptEntry(line []byte) (TranscriptParsed, error) {
	var p TranscriptParsed
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return p, nil
	}
	var e transcriptEntry
	if err := json.Unmarshal(line, &e); err != nil {
		return p, fmt.Errorf("transcript: %w", err)
	}
	p.Type = e.Type
	p.Sidechain = e.IsSidechain
	p.TS = e.Timestamp
	p.SessionID = e.SessionID
	p.CWD = e.CWD
	p.Version = e.Version
	p.GitBranch = e.GitBranch

	switch e.Type {
	case "user":
		p.Events = userEvents(e)
	case "assistant":
		p.Assistant = true
		p.Model = e.messageModel()
		p.Events = assistantEvents(e)
	case "system":
		p.Events = systemEvents(e)
	case "cost-state":
		p.Cost = &TranscriptCost{
			CostUSD: e.TotalCostUSD, DurationMS: e.TotalDuration,
			ToolDurationMS: e.TotalToolDuration,
			LinesAdded:     e.TotalLinesAdded, LinesRemoved: e.TotalLinesRemoved,
			Models: modelNames(e.ModelUsage),
		}
	case "custom-title":
		p.Title = e.CustomTitle
	case "agent-name":
		p.AgentName = e.AgentName
	default:
		p.Ignored = true
	}
	return p, nil
}

// userEvents translates a user entry: tool results, or the human's own prompt.
// Entries Claude Code injected itself (isMeta) and the rehydrated history after
// a compaction (isCompactSummary) are not the human talking, so they are
// dropped rather than shown as prompts.
func userEvents(e transcriptEntry) []api.RunEvent {
	if e.IsMeta || e.IsCompactSummary {
		return nil
	}
	var evs []api.RunEvent
	for _, b := range blocks(rawMessage{Message: e.Message}) {
		if b.Type != "tool_result" {
			continue
		}
		evs = append(evs, api.RunEvent{
			Kind: api.KindToolResult, ToolID: b.ToolUseID,
			Text: Truncate(toolResultText(b.Content), MaxToolResult), IsError: api.Bool(b.IsError),
		})
	}
	if len(evs) > 0 {
		return evs
	}
	// No tool results, so this is a prompt. Content is either a plain string or
	// a block list; toolResultText flattens both, turning images into "[image]".
	if text := strings.TrimSpace(toolResultText(messageContent(e.Message))); text != "" {
		evs = append(evs, api.RunEvent{Kind: api.KindUser, Text: Truncate(text, MaxTranscriptText)})
	}
	return evs
}

// assistantEvents translates a model turn the same way ParseLine does, except
// that a synthetic API-error turn becomes a system notice rather than text.
func assistantEvents(e transcriptEntry) []api.RunEvent {
	if e.IsAPIError {
		text := "API error"
		if e.Error != "" {
			text += " (" + e.Error + ")"
		}
		if msg := strings.TrimSpace(toolResultText(messageContent(e.Message))); msg != "" {
			text += ": " + msg
		}
		return []api.RunEvent{{Kind: api.KindSystem, Text: OneLine(text, maxSummary), IsError: api.Bool(true)}}
	}
	var evs []api.RunEvent
	for _, b := range blocks(rawMessage{Message: e.Message}) {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				evs = append(evs, api.RunEvent{Kind: api.KindText, Text: Truncate(b.Text, MaxTranscriptText)})
			}
		case "thinking":
			if strings.TrimSpace(b.Thinking) != "" {
				evs = append(evs, api.RunEvent{Kind: api.KindThinking, Text: Truncate(b.Thinking, MaxTranscriptText)})
			}
		case "tool_use":
			input := b.Input
			if len(bytes.TrimSpace(input)) == 0 || bytes.Equal(bytes.TrimSpace(input), []byte("null")) {
				input = json.RawMessage("{}")
			}
			// The input is kept whole here so an Observe hook sees the full
			// command; TranscriptTail caps it before retaining the event.
			evs = append(evs, api.RunEvent{
				Kind: api.KindToolUse, Tool: b.Name, ToolID: b.ID,
				Input: input, Text: ToolSummary(b.Name, input),
			})
		}
	}
	return evs
}

// systemEvents translates the few system entries worth showing. Hook summaries
// are routine (one per stop) and only interesting when a hook failed.
func systemEvents(e transcriptEntry) []api.RunEvent {
	text := strings.TrimSpace(jsonString(e.Content))
	switch e.Subtype {
	case "compact_boundary":
		if text == "" {
			text = "Context compacted"
		}
	case "stop_hook_summary":
		if !nonEmptyArray(e.HookErrors) {
			return nil
		}
		text = "Stop hook error: " + OneLine(string(e.HookErrors), maxSummary)
	}
	if text == "" {
		return nil
	}
	isErr := e.Level == "error"
	return []api.RunEvent{{Kind: api.KindSystem, Text: Truncate(text, MaxTranscriptText), IsError: api.Bool(isErr)}}
}

// messageContent returns message.content as raw JSON, or nil.
func messageContent(raw json.RawMessage) json.RawMessage {
	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	if len(raw) == 0 || raw[0] != '{' || json.Unmarshal(raw, &msg) != nil {
		return nil
	}
	return msg.Content
}

// messageModel returns message.model, ignoring the "<synthetic>" placeholder
// Claude Code uses for locally generated turns.
func (e transcriptEntry) messageModel() string {
	var msg struct {
		Model string `json:"model"`
	}
	if len(e.Message) == 0 || e.Message[0] != '{' || json.Unmarshal(e.Message, &msg) != nil {
		return ""
	}
	if strings.HasPrefix(msg.Model, "<") {
		return ""
	}
	return msg.Model
}

// jsonString returns raw decoded as a JSON string, or "" if it is not one.
func jsonString(raw json.RawMessage) string {
	var s string
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

// nonEmptyArray reports whether raw is a JSON array with at least one element.
func nonEmptyArray(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '[' {
		return false
	}
	var items []json.RawMessage
	return json.Unmarshal(raw, &items) == nil && len(items) > 0
}

// modelNames returns the model keys of a cost-state modelUsage map, sorted so
// the result is stable.
func modelNames(usage map[string]json.RawMessage) []string {
	if len(usage) == 0 {
		return nil
	}
	names := make([]string, 0, len(usage))
	for name := range usage {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// TranscriptState is the run metadata derived from everything read so far.
// Counts cover the whole transcript, not just the retained events.
type TranscriptState struct {
	SessionID string
	CWD       string
	Model     string // most recent model that produced a turn
	Version   string
	GitBranch string
	Title     string // custom-title, when the session has one
	AgentName string // agent-name, when the session has one

	UserPrompts    int // human turns
	AssistantTurns int
	ToolCalls      int
	LastText       string // last assistant text, one line

	CostUSD        float64
	DurationMS     int64
	ToolDurationMS int64
	LinesAdded     int
	LinesRemoved   int

	FirstActivity time.Time // earliest entry timestamp
	LastActivity  time.Time // latest entry timestamp
	LastEntryTS   string    // that timestamp, as written

	Entries   int // entries read
	Ignored   int // entries whose type we do not translate
	Malformed int // lines that did not parse, or were too long
	Sidechain int // subagent entries dropped by the sidechain filter
	Dropped   int // events evicted by the retention cap
}

// TranscriptOptions configures a TranscriptTail. The zero value is usable.
type TranscriptOptions struct {
	// MaxEvents is the retained-event cap; MaxTranscriptEvents if unset.
	MaxEvents int
	// IncludeSidechain keeps subagent (isSidechain) turns, which are
	// interleaved into the same file as the main conversation. They are
	// dropped by default: api.RunEvent has no field to label them with, so
	// including them would mix subagent output into the main transcript.
	IncludeSidechain bool
	// Observe, if set, is called for every parsed entry before the sidechain
	// filter, the tool-input cap and the retention cap. It lets a caller scan
	// the full transcript
	// — for instance for the `bd` command that links a session to a bead —
	// without reading the file a second time.
	Observe func(TranscriptParsed)
}

// TranscriptDelta is the outcome of one TranscriptTail.Read.
type TranscriptDelta struct {
	// Events are the events this read produced that are still retained,
	// oldest first.
	Events []api.RunEvent
	// Dropped counts events this read produced that the cap already evicted.
	Dropped int
	// Consumed is how many bytes of the file this read turned into entries.
	Consumed int64
	// Restarted reports that the file shrank or was replaced, so the tail
	// reset and re-read it from the beginning.
	Restarted bool
	// Size and ModTime are the file's stat at the time of the read.
	Size    int64
	ModTime time.Time
}

// Grew reports whether the transcript advanced during this read.
func (d TranscriptDelta) Grew() bool { return d.Consumed > 0 }

// TranscriptTail incrementally translates one on-disk session transcript into
// api.RunEvents. Reads are pull-based, so the tail needs no goroutine, no file
// watcher and no lock; the caller polls at whatever cadence it likes. A tail is
// not safe for concurrent use.
//
// Retained events are kept in a ring of at most MaxEvents, so a session that
// runs for hours costs a bounded amount of memory. Seq numbers keep increasing
// across evictions, so EventsAfter stays usable for SSE replay; it simply
// cannot return events that have already been evicted.
type TranscriptTail struct {
	path string
	opts TranscriptOptions

	offset  int64 // bytes consumed; always at an entry boundary
	nextSeq int64
	ring    []api.RunEvent
	start   int // index of the oldest retained event
	n       int // number of retained events
	state   TranscriptState
}

// NewTranscriptTail returns a tail of the transcript at path. Nothing is read
// until Read is called, and a new tail always starts from the beginning of the
// file.
func NewTranscriptTail(path string, opts TranscriptOptions) *TranscriptTail {
	if opts.MaxEvents <= 0 {
		opts.MaxEvents = MaxTranscriptEvents
	}
	return &TranscriptTail{path: path, opts: opts, nextSeq: 1}
}

// FindTranscript returns the transcript file for sessionID under projectsDir
// (normally ~/.claude/projects). Claude Code derives each project directory
// name by escaping the session's working directory, which is lossy and
// ambiguous, so the session id in the file name is the only reliable key. If
// more than one directory holds a file for the session, the most recently
// modified one wins.
func FindTranscript(projectsDir, sessionID string) (string, error) {
	if sessionID == "" {
		return "", errors.New("transcript: empty session id")
	}
	// A session id is a UUID; refuse anything that could escape the directory.
	if strings.ContainsAny(sessionID, `/\*?[]`) || strings.Contains(sessionID, "..") {
		return "", fmt.Errorf("transcript: bad session id %q", sessionID)
	}
	matches, err := filepath.Glob(filepath.Join(projectsDir, "*", sessionID+".jsonl"))
	if err != nil {
		return "", err
	}
	var best string
	var bestMod time.Time
	for _, m := range matches {
		fi, err := os.Stat(m)
		if err != nil {
			continue
		}
		if best == "" || fi.ModTime().After(bestMod) {
			best, bestMod = m, fi.ModTime()
		}
	}
	if best == "" {
		return "", fmt.Errorf("transcript: no transcript for session %s under %s: %w", sessionID, projectsDir, fs.ErrNotExist)
	}
	return best, nil
}

// Path is the transcript this tail follows.
func (t *TranscriptTail) Path() string { return t.path }

// Offset is how many bytes of the transcript have been consumed.
func (t *TranscriptTail) Offset() int64 { return t.offset }

// State is the metadata derived so far.
func (t *TranscriptTail) State() TranscriptState { return t.state }

// Events returns the retained events, oldest first.
func (t *TranscriptTail) Events() []api.RunEvent { return t.EventsAfter(0) }

// EventsAfter returns the retained events with Seq greater than seq, oldest
// first. Events the cap already evicted are not returned.
func (t *TranscriptTail) EventsAfter(seq int64) []api.RunEvent {
	if t.n == 0 {
		return nil
	}
	skip := 0
	if oldest := t.nextSeq - int64(t.n); seq >= oldest {
		skip = int(seq - oldest + 1)
	}
	if skip >= t.n {
		return nil
	}
	out := make([]api.RunEvent, 0, t.n-skip)
	for i := skip; i < t.n; i++ {
		out = append(out, t.ring[(t.start+i)%len(t.ring)])
	}
	return out
}

// Read parses everything appended since the last call. A missing transcript is
// reported as an error wrapping fs.ErrNotExist; callers polling a session whose
// transcript has not appeared yet should treat that as "nothing yet".
//
// A transcript that was replaced rather than appended to is detected by the
// file shrinking. Claude Code names the file after the session id and only ever
// appends, so a replacement that happens to be at least as long as what we have
// already read is not a case worth guarding against.
func (t *TranscriptTail) Read() (TranscriptDelta, error) {
	var d TranscriptDelta
	f, err := os.Open(t.path)
	if err != nil {
		return d, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return d, err
	}
	d.Size, d.ModTime = fi.Size(), fi.ModTime()
	if fi.Size() < t.offset {
		// Truncated, or a different file at the same path.
		t.reset()
		d.Restarted = true
	}
	if fi.Size() == t.offset {
		return d, nil
	}
	if _, err := f.Seek(t.offset, io.SeekStart); err != nil {
		return d, err
	}

	firstNew, droppedBefore, startOffset := t.nextSeq, t.state.Dropped, t.offset
	br := bufio.NewReaderSize(f, 64<<10)
	var readErr error
	for {
		line, err := br.ReadBytes('\n')
		if err != nil {
			// No trailing newline: the writer is mid-entry. Leave those bytes
			// unconsumed so the next Read sees the whole line.
			if !errors.Is(err, io.EOF) {
				readErr = err
			}
			break
		}
		t.offset += int64(len(line))
		t.consume(line)
	}
	d.Consumed = t.offset - startOffset
	d.Events = t.EventsAfter(firstNew - 1)
	d.Dropped = t.state.Dropped - droppedBefore
	return d, readErr
}

// reset forgets everything read so far, for a transcript that was replaced.
// Seq numbering continues so that subscribers never see a seq go backwards.
func (t *TranscriptTail) reset() {
	t.offset = 0
	t.start, t.n = 0, 0
	t.ring = nil
	seq := t.nextSeq
	t.state = TranscriptState{}
	t.nextSeq = seq
}

// consume folds one transcript line into the tail.
func (t *TranscriptTail) consume(line []byte) {
	t.state.Entries++
	if len(line) > MaxTranscriptLine {
		t.state.Malformed++
		return
	}
	p, err := ParseTranscriptEntry(line)
	if err != nil {
		t.state.Malformed++
		return
	}
	if p.Type == "" { // blank line
		t.state.Entries--
		return
	}
	if t.opts.Observe != nil {
		t.opts.Observe(p)
	}
	if p.Ignored {
		t.state.Ignored++
		return
	}
	if p.Sidechain && !t.opts.IncludeSidechain {
		t.state.Sidechain++
		return
	}
	t.applyMeta(p)
	for _, ev := range p.Events {
		switch ev.Kind {
		case api.KindToolUse:
			t.state.ToolCalls++
			if len(ev.Input) > MaxTranscriptToolInput {
				ev.Input = json.RawMessage("{}")
			}
		case api.KindText:
			t.state.LastText = OneLine(ev.Text, maxTranscriptLastText)
		case api.KindUser:
			t.state.UserPrompts++
		}
		ev.Seq = t.nextSeq
		t.nextSeq++
		ev.TS = p.TS
		t.push(ev)
	}
}

// applyMeta folds an entry's session metadata into the derived state.
func (t *TranscriptTail) applyMeta(p TranscriptParsed) {
	set := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	set(&t.state.SessionID, p.SessionID)
	set(&t.state.CWD, p.CWD)
	set(&t.state.Model, p.Model)
	set(&t.state.Version, p.Version)
	set(&t.state.GitBranch, p.GitBranch)
	set(&t.state.Title, p.Title)
	set(&t.state.AgentName, p.AgentName)
	if p.Assistant {
		t.state.AssistantTurns++
	}
	if ts, err := time.Parse(time.RFC3339, p.TS); err == nil {
		if t.state.FirstActivity.IsZero() || ts.Before(t.state.FirstActivity) {
			t.state.FirstActivity = ts
		}
		if ts.After(t.state.LastActivity) {
			t.state.LastActivity = ts
			t.state.LastEntryTS = p.TS
		}
	}
	if c := p.Cost; c != nil {
		// cost-state holds running totals, so the newest entry wins.
		t.state.CostUSD = c.CostUSD
		t.state.DurationMS = c.DurationMS
		t.state.ToolDurationMS = c.ToolDurationMS
		t.state.LinesAdded = c.LinesAdded
		t.state.LinesRemoved = c.LinesRemoved
		if t.state.Model == "" && len(c.Models) > 0 {
			t.state.Model = c.Models[len(c.Models)-1]
		}
	}
}

// maxTranscriptLastText matches the run manager's cap on Run.LastText.
const maxTranscriptLastText = 280

// push appends an event to the retention ring, evicting the oldest if full.
func (t *TranscriptTail) push(ev api.RunEvent) {
	if t.ring == nil {
		t.ring = make([]api.RunEvent, t.opts.MaxEvents)
	}
	if t.n < len(t.ring) {
		t.ring[(t.start+t.n)%len(t.ring)] = ev
		t.n++
		return
	}
	t.ring[t.start] = ev
	t.start = (t.start + 1) % len(t.ring)
	t.state.Dropped++
}

// SessionLiveness is what the session-registry watcher knows about the process
// that owns a transcript.
type SessionLiveness struct {
	// Alive reports that the pid in the session record is still the same live
	// process: kill(pid, 0) succeeded and procStart still matches.
	Alive bool
	// RecordStatus is the session record's own "status" field, "idle" when the
	// session is parked waiting for its human.
	RecordStatus string
}

// DeriveSessionStatus maps what is known about an agent-owned session onto an
// api run status plus a finer-grained activity. A transcript cannot be read for
// status: there is no result entry, so liveness of the owning process plus how
// recently the transcript grew is all there is.
//
// The transcript also never says how a session ended, so a dead process yields
// StatusSucceeded — the run is over and nothing failed on nacred's side. A
// caller that needs the difference must use the activity, not the status.
//
// idleAfter is how long a live session may stay quiet before it counts as idle;
// TranscriptIdleAfter is used when it is zero.
func DeriveSessionStatus(l SessionLiveness, lastActivity, now time.Time, idleAfter time.Duration) (status, activity string) {
	if !l.Alive {
		return api.StatusSucceeded, ActivityEnded
	}
	if idleAfter <= 0 {
		idleAfter = TranscriptIdleAfter
	}
	switch {
	case l.RecordStatus == ActivityIdle:
		return api.StatusRunning, ActivityIdle
	case lastActivity.IsZero(), now.Sub(lastActivity) > idleAfter:
		return api.StatusRunning, ActivityIdle
	default:
		return api.StatusRunning, ActivityWorking
	}
}
