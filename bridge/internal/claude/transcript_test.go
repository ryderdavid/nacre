package claude

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

const fixture = "testdata/transcript.jsonl"

// fixtureLines returns the captured transcript lines, one per entry.
func fixtureLines(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

// writeTranscript writes lines (newline-terminated) to a temp file.
func writeTranscript(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l)
		b.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func kinds(evs []api.RunEvent) []string {
	if len(evs) == 0 {
		return nil
	}
	out := make([]string, 0, len(evs))
	for _, e := range evs {
		out = append(out, e.Kind)
	}
	return out
}

// TestParseTranscriptFixture pins what every captured entry type contributes:
// the handled ones their event kinds, the bookkeeping ones nothing at all.
func TestParseTranscriptFixture(t *testing.T) {
	want := []struct {
		typ     string
		ignored bool
		kinds   []string
	}{
		0:  {typ: "user", kinds: []string{api.KindUser}}, // prompt as a plain string
		1:  {typ: "user", kinds: []string{api.KindUser}}, // prompt as a text block
		2:  {typ: "user"},                                // isMeta: injected, not the human
		3:  {typ: "user"},                                // isCompactSummary: rehydrated history
		4:  {typ: "user", kinds: []string{api.KindUser}}, // image + text
		5:  {typ: "assistant", kinds: []string{api.KindThinking}},
		6:  {typ: "assistant", kinds: []string{api.KindText}},
		7:  {typ: "assistant", kinds: []string{api.KindSystem}},  // synthetic API-error turn
		8:  {typ: "assistant", kinds: []string{api.KindToolUse}}, // Bash
		9:  {typ: "assistant", kinds: []string{api.KindToolUse}}, // Read
		10: {typ: "assistant", kinds: []string{api.KindToolUse}}, // Edit
		11: {typ: "assistant", kinds: []string{api.KindToolUse}}, // WebFetch
		12: {typ: "user", kinds: []string{api.KindToolResult}},
		13: {typ: "user", kinds: []string{api.KindToolResult}}, // is_error
		14: {typ: "system"},                                    // stop_hook_summary, no hook errors
		15: {typ: "system", kinds: []string{api.KindSystem}},   // compact_boundary
		16: {typ: "cost-state"},
		17: {typ: "custom-title"},
		18: {typ: "agent-name"},
		19: {typ: "attachment", ignored: true},
		20: {typ: "api-request", ignored: true},
		21: {typ: "api-request-blob", ignored: true},
		22: {typ: "api-request-shape", ignored: true},
		23: {typ: "last-prompt", ignored: true},
		24: {typ: "atis-latch", ignored: true},
		25: {typ: "queue-operation", ignored: true},
		26: {typ: "frame-link", ignored: true},
		27: {typ: "worktree-state", ignored: true},
		28: {typ: "file-history-delta", ignored: true},
		29: {typ: "file-history-snapshot", ignored: true},
		30: {typ: "mode", ignored: true},
		31: {typ: "bridge-session", ignored: true},
		32: {typ: "artifact-autoreact-ledger", ignored: true},
		33: {typ: "artifact-comment-monitor", ignored: true},
	}
	lines := fixtureLines(t, fixture)
	if len(lines) != len(want) {
		t.Fatalf("fixture has %d entries, expectations cover %d", len(lines), len(want))
	}
	for i, line := range lines {
		p, err := ParseTranscriptEntry([]byte(line))
		if err != nil {
			t.Fatalf("entry %d (%s): %v", i, want[i].typ, err)
		}
		if p.Type != want[i].typ {
			t.Errorf("entry %d: type = %q, want %q", i, p.Type, want[i].typ)
		}
		if p.Ignored != want[i].ignored {
			t.Errorf("entry %d (%s): ignored = %v", i, p.Type, p.Ignored)
		}
		if got := kinds(p.Events); !reflect.DeepEqual(got, want[i].kinds) {
			t.Errorf("entry %d (%s): kinds = %v, want %v", i, p.Type, got, want[i].kinds)
		}
	}
}

// TestParseTranscriptDetails checks the content of the translated events, not
// just their kinds.
func TestParseTranscriptDetails(t *testing.T) {
	lines := fixtureLines(t, fixture)
	parse := func(i int) TranscriptParsed {
		t.Helper()
		p, err := ParseTranscriptEntry([]byte(lines[i]))
		if err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
		return p
	}

	// An image block in a human turn is flattened to a placeholder.
	if text := parse(4).Events[0].Text; !strings.HasPrefix(text, "[image]\n") {
		t.Errorf("image prompt = %q", text)
	}
	// A synthetic API-error turn carries the error code and is marked as one.
	apiErr := parse(7).Events[0]
	if apiErr.Text != "API error (authentication_failed): Not logged in · Please run /login" {
		t.Errorf("api error text = %q", apiErr.Text)
	}
	if apiErr.IsError == nil || !*apiErr.IsError {
		t.Error("api error event not marked is_error")
	}
	// "<synthetic>" is not a model name.
	if m := parse(7).Model; m != "" {
		t.Errorf("synthetic model = %q", m)
	}
	if m := parse(8).Model; m != "claude-opus-5-5" {
		t.Errorf("model = %q", m)
	}
	// Tool calls keep the raw input so the app can render it, plus a summary.
	bash := parse(8).Events[0]
	if bash.Tool != "Bash" || bash.ToolID == "" {
		t.Errorf("bash event = %+v", bash)
	}
	var in struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(bash.Input, &in); err != nil {
		t.Fatalf("bash input: %v", err)
	}
	if !strings.Contains(in.Command, "bd update NAC-013.7 --claim") {
		t.Errorf("bash input lost the bd command: %q", in.Command)
	}
	if !strings.HasPrefix(bash.Text, "cd ~/Code/Nacre && bd close NAC-013.1") {
		t.Errorf("bash summary = %q", bash.Text)
	}
	if strings.ContainsAny(bash.Text, "\n") {
		t.Errorf("bash summary is not one line: %q", bash.Text)
	}
	// Tool results keep the tool_use id so the app can pair them up.
	res := parse(12).Events[0]
	if res.ToolID != "toolu_01XJ4JYGxrUdetbU53JLUpE7" || res.IsError == nil || *res.IsError {
		t.Errorf("tool result = %+v", res)
	}
	if e := parse(13).Events[0]; e.IsError == nil || !*e.IsError {
		t.Errorf("error tool result not marked: %+v", e)
	}
	// cost-state carries the running totals.
	cost := parse(16).Cost
	if cost == nil || cost.CostUSD != 46.2684787 || cost.DurationMS != 28098236 ||
		cost.ToolDurationMS != 3930613 || cost.LinesAdded != 10951 {
		t.Fatalf("cost = %+v", cost)
	}
	if !reflect.DeepEqual(cost.Models, []string{"claude-haiku-5-5", "claude-opus-5-5"}) {
		t.Errorf("cost models = %v", cost.Models)
	}
	if got := parse(17).Title; got != "Nacre · voicebar-8lo" {
		t.Errorf("custom title = %q", got)
	}
	if got := parse(18).AgentName; got != "Nacre · voicebar-8lo" {
		t.Errorf("agent name = %q", got)
	}
}

func TestParseTranscriptBlankAndMalformed(t *testing.T) {
	for _, line := range []string{"", "   ", "\n"} {
		p, err := ParseTranscriptEntry([]byte(line))
		if err != nil || p.Type != "" || len(p.Events) != 0 {
			t.Errorf("blank line %q: %+v, %v", line, p, err)
		}
	}
	if _, err := ParseTranscriptEntry([]byte("{not json")); err == nil {
		t.Error("malformed line: want error")
	}
	// A line that is valid JSON but not an object we know is simply ignored.
	p, err := ParseTranscriptEntry([]byte(`{"type":"something-new-in-2.2","sessionId":"s"}`))
	if err != nil || !p.Ignored || len(p.Events) != 0 {
		t.Errorf("unknown type: %+v, %v", p, err)
	}
}

// TestTranscriptTailFixture tails the whole captured fixture and checks the
// events and the derived state.
func TestTranscriptTailFixture(t *testing.T) {
	lines := fixtureLines(t, fixture)
	tail := NewTranscriptTail(writeTranscript(t, lines), TranscriptOptions{})
	d, err := tail.Read()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		api.KindUser, api.KindUser, api.KindUser,
		api.KindThinking, api.KindText, api.KindSystem,
		api.KindToolUse, api.KindToolUse, api.KindToolUse, api.KindToolUse,
		api.KindToolResult, api.KindToolResult,
		api.KindSystem,
	}
	if got := kinds(d.Events); !reflect.DeepEqual(got, want) {
		t.Fatalf("delta kinds = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(kinds(tail.Events()), want) {
		t.Error("Events() disagrees with the delta")
	}
	// Every event kind is one the app already renders.
	rendered := map[string]bool{
		api.KindUser: true, api.KindSystem: true, api.KindText: true,
		api.KindThinking: true, api.KindToolUse: true, api.KindToolResult: true,
	}
	for _, e := range d.Events {
		if !rendered[e.Kind] {
			t.Errorf("unrenderable event kind %q", e.Kind)
		}
	}
	// Seqs start at 1 and are contiguous; timestamps come from the entries.
	for i, e := range d.Events {
		if e.Seq != int64(i+1) {
			t.Fatalf("event %d seq = %d", i, e.Seq)
		}
		if _, err := time.Parse(time.RFC3339, e.TS); err != nil {
			t.Errorf("event %d ts = %q: %v", i, e.TS, err)
		}
	}

	st := tail.State()
	if st.Entries != len(lines) {
		t.Errorf("entries = %d, want %d", st.Entries, len(lines))
	}
	if st.Ignored != 15 {
		t.Errorf("ignored = %d, want 15", st.Ignored)
	}
	if st.Malformed != 0 || st.Dropped != 0 || st.Sidechain != 0 {
		t.Errorf("malformed/dropped/sidechain = %d/%d/%d", st.Malformed, st.Dropped, st.Sidechain)
	}
	if st.SessionID != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("session id = %q", st.SessionID)
	}
	if st.CWD == "" || !strings.HasPrefix(st.CWD, "/Users/dev") {
		t.Errorf("cwd = %q", st.CWD)
	}
	if st.Model != "claude-opus-5-5" {
		t.Errorf("model = %q", st.Model)
	}
	if st.Version == "" || st.GitBranch == "" {
		t.Errorf("version/branch = %q/%q", st.Version, st.GitBranch)
	}
	if st.Title != "Nacre · voicebar-8lo" || st.AgentName != "Nacre · voicebar-8lo" {
		t.Errorf("title/agent = %q/%q", st.Title, st.AgentName)
	}
	if st.UserPrompts != 3 || st.AssistantTurns != 7 || st.ToolCalls != 4 {
		t.Errorf("prompts/turns/tools = %d/%d/%d", st.UserPrompts, st.AssistantTurns, st.ToolCalls)
	}
	if !strings.HasPrefix(st.LastText, "I'll start by surveying the Mac") {
		t.Errorf("last text = %q", st.LastText)
	}
	if st.CostUSD != 46.2684787 || st.DurationMS != 28098236 || st.ToolDurationMS != 3930613 {
		t.Errorf("cost/duration = %v/%v/%v", st.CostUSD, st.DurationMS, st.ToolDurationMS)
	}
	if st.LinesAdded != 10951 || st.LinesRemoved != 5 {
		t.Errorf("lines +%d-%d", st.LinesAdded, st.LinesRemoved)
	}
	// Timestamps span the captured entries, which come from several sessions.
	if st.FirstActivity.After(st.LastActivity) || st.LastEntryTS == "" {
		t.Errorf("activity window %v..%v (%q)", st.FirstActivity, st.LastActivity, st.LastEntryTS)
	}
	if d.Size == 0 || d.Consumed != d.Size || !d.Grew() {
		t.Errorf("delta = %+v", TranscriptDelta{Size: d.Size, Consumed: d.Consumed})
	}

	// A second read of an unchanged file yields nothing.
	d2, err := tail.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(d2.Events) != 0 || d2.Grew() {
		t.Errorf("second read = %+v", d2)
	}
}

// TestTranscriptTailIncremental is the live-session case: the file grows while
// we read it, and a read can land in the middle of a line being appended.
func TestTranscriptTailIncremental(t *testing.T) {
	lines := fixtureLines(t, fixture)
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	tail := NewTranscriptTail(path, TranscriptOptions{})

	// Nothing written yet.
	if d, err := tail.Read(); err != nil || len(d.Events) != 0 {
		t.Fatalf("empty file: %+v, %v", d, err)
	}

	appendLine := func(i int) {
		t.Helper()
		if _, err := f.WriteString(lines[i] + "\n"); err != nil {
			t.Fatal(err)
		}
	}

	appendLine(0) // user prompt
	appendLine(5) // thinking
	d, err := tail.Read()
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(d.Events); !reflect.DeepEqual(got, []string{api.KindUser, api.KindThinking}) {
		t.Fatalf("first read kinds = %v", got)
	}

	// A half-written entry must not be parsed, and must not be skipped either.
	half := lines[8][:len(lines[8])/2]
	if _, err := f.WriteString(half); err != nil {
		t.Fatal(err)
	}
	d, err = tail.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Events) != 0 || d.Grew() {
		t.Fatalf("partial line produced %+v", d)
	}
	if st := tail.State(); st.Malformed != 0 || st.Entries != 2 {
		t.Fatalf("partial line counted: %+v", st)
	}
	if _, err := f.WriteString(lines[8][len(half):] + "\n"); err != nil {
		t.Fatal(err)
	}
	d, err = tail.Read()
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(d.Events); !reflect.DeepEqual(got, []string{api.KindToolUse}) {
		t.Fatalf("completed line kinds = %v", got)
	}
	if d.Events[0].Seq != 3 {
		t.Errorf("seq = %d, want 3", d.Events[0].Seq)
	}
	if got := kinds(tail.Events()); len(got) != 3 {
		t.Errorf("retained = %v", got)
	}
	// EventsAfter is the SSE replay path.
	if got := kinds(tail.EventsAfter(2)); !reflect.DeepEqual(got, []string{api.KindToolUse}) {
		t.Errorf("EventsAfter(2) = %v", got)
	}
	if got := tail.EventsAfter(3); len(got) != 0 {
		t.Errorf("EventsAfter(3) = %v", kinds(got))
	}
}

func TestTranscriptTailMissingFile(t *testing.T) {
	tail := NewTranscriptTail(filepath.Join(t.TempDir(), "nope.jsonl"), TranscriptOptions{})
	if _, err := tail.Read(); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v, want not-exist", err)
	}
}

// TestTranscriptTailRestart covers a transcript that is replaced or truncated
// under us: the tail re-reads it without ever reusing a seq number.
func TestTranscriptTailRestart(t *testing.T) {
	lines := fixtureLines(t, fixture)
	path := writeTranscript(t, []string{lines[0], lines[6]})
	tail := NewTranscriptTail(path, TranscriptOptions{})
	d, err := tail.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Events) != 2 || d.Restarted {
		t.Fatalf("first read = %+v", d)
	}

	if err := os.WriteFile(path, []byte(lines[0]+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	d, err = tail.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !d.Restarted {
		t.Fatal("shrunken file not reported as restarted")
	}
	if got := kinds(d.Events); !reflect.DeepEqual(got, []string{api.KindUser}) {
		t.Fatalf("after restart kinds = %v", got)
	}
	if d.Events[0].Seq != 3 {
		t.Errorf("seq after restart = %d, want 3 (seqs must not go backwards)", d.Events[0].Seq)
	}
	if st := tail.State(); st.Entries != 1 || st.UserPrompts != 1 {
		t.Errorf("state not reset: %+v", st)
	}
}

// TestTranscriptTailSidechain covers subagent turns, which are interleaved into
// the same file as the main conversation.
func TestTranscriptTailSidechain(t *testing.T) {
	main := fixtureLines(t, fixture)
	side := fixtureLines(t, "testdata/transcript_sidechain.jsonl")
	lines := append([]string{main[0]}, side...)

	tail := NewTranscriptTail(writeTranscript(t, lines), TranscriptOptions{})
	d, err := tail.Read()
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(d.Events); !reflect.DeepEqual(got, []string{api.KindUser}) {
		t.Fatalf("sidechain entries leaked: %v", got)
	}
	if st := tail.State(); st.Sidechain != len(side) {
		t.Errorf("sidechain count = %d, want %d", st.Sidechain, len(side))
	}

	tail = NewTranscriptTail(writeTranscript(t, lines), TranscriptOptions{IncludeSidechain: true})
	d, err = tail.Read()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{api.KindUser, api.KindText, api.KindToolUse, api.KindToolResult}
	if got := kinds(d.Events); !reflect.DeepEqual(got, want) {
		t.Fatalf("IncludeSidechain kinds = %v, want %v", got, want)
	}
	if st := tail.State(); st.Sidechain != 0 {
		t.Errorf("sidechain count = %d, want 0", st.Sidechain)
	}
}

// TestTranscriptObserve checks the hook the session watcher uses to find the bd
// command that links a session to a bead, including entries the tail itself
// filters out.
func TestTranscriptObserve(t *testing.T) {
	lines := fixtureLines(t, fixture)
	side := fixtureLines(t, "testdata/transcript_sidechain.jsonl")
	all := append(append([]string{}, lines...), side...)

	var seen int
	var commands []string
	tail := NewTranscriptTail(writeTranscript(t, all), TranscriptOptions{
		MaxEvents: 1, // the observer must not be affected by the cap
		Observe: func(p TranscriptParsed) {
			seen++
			for _, e := range p.Events {
				if e.Kind == api.KindToolUse && e.Tool == "Bash" {
					var in struct {
						Command string `json:"command"`
					}
					if json.Unmarshal(e.Input, &in) == nil {
						commands = append(commands, in.Command)
					}
				}
			}
		},
	})
	if _, err := tail.Read(); err != nil {
		t.Fatal(err)
	}
	if seen != len(all) {
		t.Errorf("observed %d entries, want %d", seen, len(all))
	}
	var claimed bool
	for _, c := range commands {
		if strings.Contains(c, "bd update NAC-013.7 --claim") {
			claimed = true
		}
	}
	if !claimed {
		t.Errorf("observer did not see the bd claim; commands = %q", commands)
	}
	if n := len(tail.Events()); n != 1 {
		t.Errorf("retained %d events, want 1", n)
	}
}

// TestTranscriptTailLargeTranscript is the memory bound: a multi-megabyte
// transcript must leave a bounded event list behind, while the derived state
// still covers every entry.
func TestTranscriptTailLargeTranscript(t *testing.T) {
	lines := fixtureLines(t, fixture)
	const repeats = 200 // 34 entries each, ~6.5 MB
	big := make([]string, 0, len(lines)*repeats)
	for i := 0; i < repeats; i++ {
		big = append(big, lines...)
	}
	path := writeTranscript(t, big)
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() < 5<<20 {
		t.Fatalf("fixture too small to be a useful test: %d bytes", fi.Size())
	}

	tail := NewTranscriptTail(path, TranscriptOptions{MaxEvents: 100})
	d, err := tail.Read()
	if err != nil {
		t.Fatal(err)
	}
	st := tail.State()
	if st.Entries != len(big) {
		t.Errorf("entries = %d, want %d", st.Entries, len(big))
	}
	if st.ToolCalls != 4*repeats || st.UserPrompts != 3*repeats {
		t.Errorf("tools/prompts = %d/%d", st.ToolCalls, st.UserPrompts)
	}
	if n := len(tail.Events()); n != 100 {
		t.Fatalf("retained %d events, want the cap of 100", n)
	}
	if len(d.Events) != 100 {
		t.Errorf("delta carried %d events; it must be capped too", len(d.Events))
	}
	if st.Dropped != 13*repeats-100 {
		t.Errorf("dropped = %d, want %d", st.Dropped, 13*repeats-100)
	}
	if d.Dropped != st.Dropped {
		t.Errorf("delta dropped = %d, state dropped = %d", d.Dropped, st.Dropped)
	}
	// The retained window is the newest events, and replay from an evicted seq
	// returns what is left rather than failing.
	evs := tail.Events()
	last := evs[len(evs)-1]
	if last.Seq != int64(13*repeats) {
		t.Errorf("last seq = %d, want %d", last.Seq, 13*repeats)
	}
	if n := len(tail.EventsAfter(0)); n != 100 {
		t.Errorf("EventsAfter(0) = %d events, want 100", n)
	}
	if n := len(tail.EventsAfter(last.Seq - 10)); n != 10 {
		t.Errorf("EventsAfter(last-10) = %d events, want 10", n)
	}
	// The retained window is a small fraction of the transcript, which is what
	// keeps nacred memory and the SSE replay bounded.
	var held int
	for _, e := range evs {
		held += len(e.Text) + len(e.Input)
	}
	if int64(held) > fi.Size()/4 {
		t.Errorf("retained %d bytes of a %d byte transcript", held, fi.Size())
	}
}

// TestTranscriptTailCaps covers the per-event caps that keep one pathological
// entry from dominating memory.
func TestTranscriptTailCaps(t *testing.T) {
	huge := strings.Repeat("x", MaxTranscriptText+500)
	bigInput, err := json.Marshal(map[string]string{
		"file_path": "/Users/dev/Code/example/main.go",
		"content":   strings.Repeat("y", MaxTranscriptToolInput+100),
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	lines := []string{
		entry(map[string]any{"type": "user", "sessionId": "s", "message": map[string]any{"role": "user", "content": huge}}),
		entry(map[string]any{"type": "assistant", "sessionId": "s", "message": map[string]any{"role": "assistant", "content": []any{
			map[string]any{"type": "text", "text": huge},
			map[string]any{"type": "thinking", "thinking": huge},
			map[string]any{"type": "tool_use", "id": "tu1", "name": "Write", "input": json.RawMessage(bigInput)},
		}}}),
		// An over-long line is not parsed at all.
		entry(map[string]any{"type": "user", "sessionId": "s", "message": map[string]any{
			"role": "user", "content": strings.Repeat("z", MaxTranscriptLine),
		}}),
	}
	tail := NewTranscriptTail(writeTranscript(t, lines), TranscriptOptions{})
	d, err := tail.Read()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{api.KindUser, api.KindText, api.KindThinking, api.KindToolUse}
	if got := kinds(d.Events); !reflect.DeepEqual(got, want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
	for _, e := range d.Events[:3] {
		if n := len([]rune(e.Text)); n != MaxTranscriptText {
			t.Errorf("%s text is %d runes, want %d", e.Kind, n, MaxTranscriptText)
		}
	}
	tool := d.Events[3]
	if string(tool.Input) != "{}" {
		t.Errorf("oversized tool input retained (%d bytes)", len(tool.Input))
	}
	if tool.Text != "/Users/dev/Code/example/main.go" {
		t.Errorf("summary lost with the input: %q", tool.Text)
	}
	if st := tail.State(); st.Malformed != 1 {
		t.Errorf("over-long line: malformed = %d, want 1", st.Malformed)
	}
}

func TestDeriveSessionStatus(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	fresh := now.Add(-10 * time.Second)
	stale := now.Add(-10 * time.Minute)

	cases := []struct {
		name         string
		live         SessionLiveness
		last         time.Time
		idleAfter    time.Duration
		wantStatus   string
		wantActivity string
	}{
		{"dead process", SessionLiveness{}, fresh, 0, api.StatusSucceeded, ActivityEnded},
		{"dead and stale", SessionLiveness{}, stale, 0, api.StatusSucceeded, ActivityEnded},
		{"alive and writing", SessionLiveness{Alive: true}, fresh, 0, api.StatusRunning, ActivityWorking},
		{"alive but quiet", SessionLiveness{Alive: true}, stale, 0, api.StatusRunning, ActivityIdle},
		{"parked by the record", SessionLiveness{Alive: true, RecordStatus: "idle"}, fresh, 0, api.StatusRunning, ActivityIdle},
		{"no transcript yet", SessionLiveness{Alive: true}, time.Time{}, 0, api.StatusRunning, ActivityIdle},
		{"custom idle window", SessionLiveness{Alive: true}, stale, time.Hour, api.StatusRunning, ActivityWorking},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, activity := DeriveSessionStatus(c.live, c.last, now, c.idleAfter)
			if status != c.wantStatus || activity != c.wantActivity {
				t.Errorf("= %q/%q, want %q/%q", status, activity, c.wantStatus, c.wantActivity)
			}
		})
	}
}

func TestFindTranscript(t *testing.T) {
	projects := t.TempDir()
	const session = "11111111-2222-3333-4444-555555555555"
	write := func(dir, name string, mod time.Time) string {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(projects, dir), 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(projects, dir, name)
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatal(err)
		}
		return path
	}

	if _, err := FindTranscript(projects, session); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("no transcript: err = %v, want not-exist", err)
	}

	old := write("-Users-dev-Code-example", session+".jsonl", time.Now().Add(-time.Hour))
	if got, err := FindTranscript(projects, session); err != nil || got != old {
		t.Fatalf("= %q, %v; want %q", got, err, old)
	}
	// A second directory for the same session (a worktree, say): newest wins.
	newer := write("-Users-dev-Code-example--claude-worktrees-wt", session+".jsonl", time.Now())
	if got, err := FindTranscript(projects, session); err != nil || got != newer {
		t.Fatalf("= %q, %v; want %q", got, err, newer)
	}
	// Another session's transcript in the same directory is not a match.
	write("-Users-dev-Code-example", "99999999-0000-0000-0000-000000000000.jsonl", time.Now())
	if got, err := FindTranscript(projects, session); err != nil || got != newer {
		t.Fatalf("= %q, %v; want %q", got, err, newer)
	}

	for _, bad := range []string{"", "../../etc/passwd", "*", "a/b"} {
		if _, err := FindTranscript(projects, bad); err == nil {
			t.Errorf("FindTranscript(%q): want error", bad)
		}
	}
}
