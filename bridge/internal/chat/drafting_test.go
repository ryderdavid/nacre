package chat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// answers are the control responses nacred wrote to one fake call, in order:
// behavior, message and the updated input's file_path.
type answer struct{ behavior, message, path string }

func answersOf(t *testing.T, c fakeCall) []answer {
	t.Helper()
	var out []answer
	for _, line := range c.stdin {
		var m struct {
			Type     string
			Response struct {
				Response struct {
					Behavior     string
					Message      string
					UpdatedInput struct {
						FilePath string `json:"file_path"`
					} `json:"updatedInput"`
				}
			}
		}
		if json.Unmarshal([]byte(line), &m) != nil || m.Type != "control_response" {
			continue
		}
		r := m.Response.Response
		out = append(out, answer{r.Behavior, r.Message, r.UpdatedInput.FilePath})
	}
	return out
}

// draftingChat is a New effort chat behind the fake CLI.
func draftingChat(t *testing.T, files map[string]string) (*Manager, Record, string) {
	t.Helper()
	m, _, fdir := routedChat(t, files)
	rec, err := m.Create(CreateRequest{Agent: "claude", NewEffort: true})
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Drafting {
		t.Fatalf("a new Claude New effort chat isn't drafting: %+v", rec)
	}
	return m, rec, fdir
}

func TestDraftingTurnIsNotRoutedAndOnlyWritesMockups(t *testing.T) {
	m, rec, fdir := draftingChat(t, map[string]string{"router": "light"})
	sendAndWait(t, m, rec.ID, "c_draft001", "Do these steps")

	cs := calls(t, fdir)
	if kinds(cs) != "drafting" {
		t.Fatalf("calls %s, want one drafting turn and no routing call", kinds(cs))
	}
	argv := cs[0].argv
	if v, _ := flag(argv, "--permission-mode"); v != "default" {
		t.Errorf("drafting permission mode: %v", argv)
	}
	if _, ok := flag(argv, "--model"); ok {
		t.Errorf("a drafting turn runs on the owner's model, not --model: %v", argv)
	}
	if !containsAll(argv, "--restricted", "--allowed-tools", "Read", "Bash(openspec show:*)") {
		t.Errorf("drafting allowlist: %v", argv)
	}
	if v, _ := flag(argv, "--tools"); v != strings.Join(RestrictedTools, ",") {
		t.Errorf("drafting tools %q", v)
	}

	ans := answersOf(t, cs[0])
	if len(ans) != 8 {
		t.Fatalf("answers %+v, want one per recorded request", ans)
	}
	mock := m.MockupsDir(rec.ID)
	allowed := 0
	for i, a := range ans {
		switch {
		case a.behavior == "allow":
			allowed++
			if a.path != filepath.Join(mock, "d.html") {
				t.Errorf("answer %d allowed %q, outside the mockups directory", i, a.path)
			}
		case a.behavior != "deny" || a.message != DraftingReminder:
			t.Errorf("answer %d = %+v, want a deny with the drafting reminder", i, a)
		}
	}
	if allowed != 1 {
		t.Errorf("%d writes allowed, want only the mockup", allowed)
	}
}

func TestDraftingEndsAndThenRoutes(t *testing.T) {
	m, rec, fdir := draftingChat(t, map[string]string{"router": "light"})
	m.EndDrafting(rec.ID)
	got, evs, _ := m.Get(rec.ID, 0)
	if got.Drafting {
		t.Fatal("still drafting after EndDrafting")
	}
	if len(evs) == 0 || evs[len(evs)-1].Kind != KindSystem || !strings.Contains(evs[len(evs)-1].Text, "Filed. This chat is an ordinary chat again") {
		t.Errorf("no note that drafting finished: %+v", evs)
	}
	sendAndWait(t, m, rec.ID, "c_after001", "What's the difference between a merge and a rebase?")
	if k := kinds(calls(t, fdir)); k != "router,light" {
		t.Errorf("calls %s after drafting ended, want routing as usual", k)
	}
	// Ending twice is harmless and says nothing more.
	m.EndDrafting(rec.ID)
	_, evs2, _ := m.Get(rec.ID, 0)
	n := 0
	for _, e := range evs2 {
		if strings.Contains(e.Text, "Filed. This chat is an ordinary chat again") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("drafting finished %d times", n)
	}
}

func TestCodexNewEffortChatDoesNotDraft(t *testing.T) {
	m, _, _ := routedChat(t, nil)
	rec, err := m.Create(CreateRequest{Agent: AgentCodex, NewEffort: true})
	if err != nil {
		t.Skipf("codex chats unavailable here: %v", err)
	}
	if rec.Drafting {
		t.Error("a Codex New effort chat is drafting")
	}
}

func TestLightTurnDrawsAMockupWithoutHandingOff(t *testing.T) {
	m, rec, fdir := routedChat(t, map[string]string{"router": "light", "light": "light_mockup"})
	sendAndWait(t, m, rec.ID, "c_mock0001", "How does intake reach the author?")
	cs := calls(t, fdir)
	if kinds(cs) != "router,light" {
		t.Fatalf("calls %s, want no handoff", kinds(cs))
	}
	ans := answersOf(t, cs[1])
	if len(ans) != 1 || ans[0].behavior != "allow" || ans[0].path != filepath.Join(m.MockupsDir(rec.ID), "flow.html") {
		t.Fatalf("answers %+v", ans)
	}
	_, evs, _ := m.Get(rec.ID, 0)
	for _, e := range evs {
		if e.Kind == KindHandoff {
			t.Errorf("handed off: %+v", e)
		}
	}
}

func TestLightTurnWritingElsewhereHandsOffInPlainWords(t *testing.T) {
	m, rec, fdir := routedChat(t, map[string]string{"router": "light", "light": "default_allowlist_drafting", "full": "light_plain"})
	sendAndWait(t, m, rec.ID, "c_else0001", "Copy the README")
	cs := calls(t, fdir)
	if kinds(cs) != "router,light,full" {
		t.Fatalf("calls %s, want a handoff to the full turn", kinds(cs))
	}
	_, evs, _ := m.Get(rec.ID, 0)
	var h *Event
	for i := range evs {
		if evs[i].Kind == KindHandoff {
			h = &evs[i]
		}
	}
	if h == nil || !strings.HasPrefix(h.Text, "Switching to the full agent: it needs a step the quick answer can't take (Bash") ||
		strings.Contains(h.Text, "harness") {
		t.Errorf("handoff %+v", h)
	}
}

func TestMockupWriteStaysInsideTheDirectory(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "f.html"), filepath.Join(dir, "link.html")); err != nil {
		t.Fatal(err)
	}
	req := func(tool, path string) claude.ControlRequest {
		in, _ := json.Marshal(map[string]string{"file_path": path})
		return claude.ControlRequest{ToolName: tool, Input: in}
	}
	for _, c := range []struct {
		name string
		req  claude.ControlRequest
		want bool
	}{
		{"write inside", req("Write", filepath.Join(dir, "a.html")), true},
		{"edit inside", req("Edit", filepath.Join(dir, "a.html")), true},
		{"multiedit inside", req("MultiEdit", filepath.Join(dir, "a.html")), true},
		{"bash", req("Bash", filepath.Join(dir, "a.html")), false},
		{"outside", req("Write", filepath.Join(outside, "a.html")), false},
		{"dot dot", req("Write", dir+"/../"+filepath.Base(outside)+"/a.html"), false},
		{"relative", req("Write", "a.html"), false},
		{"through a symlinked directory", req("Write", filepath.Join(dir, "escape", "a.html")), false},
		{"onto a symlink", req("Write", filepath.Join(dir, "link.html")), false},
		{"the directory itself", req("Write", dir), false},
		{"empty", req("Write", ""), false},
	} {
		if got := mockupWrite(c.req, dir); got != c.want {
			t.Errorf("%s: mockupWrite = %v, want %v", c.name, got, c.want)
		}
	}
	if mockupWrite(req("Write", filepath.Join(dir, "a.html")), "") {
		t.Error("allowed with no mockups directory")
	}
}

func TestDraftAtFindsTheRecordedDraft(t *testing.T) {
	m, rec, _ := draftingChat(t, nil)
	m.mu.Lock()
	c := m.chats[rec.ID]
	m.loadLocked(c)
	m.appendLocked(c, Event{Kind: KindDraft, Text: "Keep drafts", Draft: &Draft{Title: "Keep drafts", Digest: "Decided: per chat"}})
	seq := c.events[len(c.events)-1].Seq
	m.appendLocked(c, Event{Kind: KindSystem, Text: "not a draft"})
	other := c.events[len(c.events)-1].Seq
	m.mu.Unlock()

	d, err := m.DraftAt(rec.ID, seq)
	if err != nil || d.Title != "Keep drafts" || d.Digest != "Decided: per chat" {
		t.Errorf("DraftAt = %+v, %v", d, err)
	}
	if _, err := m.DraftAt(rec.ID, other); err != ErrDraftGone {
		t.Errorf("a non-draft event: %v", err)
	}
	if _, err := m.DraftAt(rec.ID, 9999); err != ErrDraftGone {
		t.Errorf("a missing event: %v", err)
	}
	if _, err := m.DraftAt("ch_nope", seq); err != ErrNotFound {
		t.Errorf("a missing chat: %v", err)
	}
}

func TestDraftDigestIsReadAndBounded(t *testing.T) {
	d, err := parseDraft(`{"title":"Keep drafts","digest":["Decided: per chat","Ruled out: a new tab"]}`)
	if err != nil || d.Digest != "- Decided: per chat\n- Ruled out: a new tab" {
		t.Errorf("digest list = %q, %v", d.Digest, err)
	}
	d, err = parseDraft(`{"title":"Keep drafts","digest":"Decided: per chat"}`)
	if err != nil || d.Digest != "Decided: per chat" {
		t.Errorf("digest text = %q, %v", d.Digest, err)
	}
	long, _ := json.Marshal(map[string]string{"title": "x", "digest": strings.Repeat("a", MaxDigest+1)})
	if _, err := parseDraft(string(long)); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Errorf("an oversize digest: %v", err)
	}
}

func TestIntakeMarkerEndsADraftingReply(t *testing.T) {
	rest, ok := splitIntakeMarker("Here it is.\n\nNACRE_INTAKE\n")
	if !ok || rest != "Here it is." {
		t.Errorf("marker line: %q %v", rest, ok)
	}
	if _, ok := splitIntakeMarker("Say NACRE_INTAKE when you're ready."); ok {
		t.Error("a marker inside a sentence counted")
	}
	if rest, ok := splitIntakeMarker("NACRE_INTAKE"); !ok || rest != "" {
		t.Errorf("marker alone: %q %v", rest, ok)
	}
}

func TestGoAheadRaisesIntakeOnTheNewestDraft(t *testing.T) {
	m, rec, fdir := draftingChat(t, map[string]string{"drafting": "drafting_go_ahead"})
	var raised []int64
	m.SetIntakeRaiser(func(id string, seq int64) error {
		if id != rec.ID {
			t.Errorf("raised in %s", id)
		}
		raised = append(raised, seq)
		return nil
	})
	sendAndWait(t, m, rec.ID, "c_goahead1", "go ahead")
	_, evs, _ := m.Get(rec.ID, 0)
	var draftSeq int64
	for _, e := range evs {
		if e.Kind == KindDraft {
			draftSeq = e.Seq
		}
		if strings.Contains(e.Text, IntakeMarker) {
			t.Errorf("the marker was shown: %+v", e)
		}
	}
	if draftSeq == 0 || len(raised) != 1 || raised[0] != draftSeq {
		t.Fatalf("raised %v, newest draft %d (calls %s)", raised, draftSeq, kinds(calls(t, fdir)))
	}

	// Without a raiser the chat says why nothing happened.
	m.SetIntakeRaiser(nil)
	sendAndWait(t, m, rec.ID, "c_goahead2", "go ahead")
	_, evs, _ = m.Get(rec.ID, 0)
	if last := evs[len(evs)-2]; last.Kind != KindError || !strings.Contains(last.Text, "Couldn't ask where it belongs") {
		t.Errorf("no error note: %+v", evs[len(evs)-3:])
	}
}

func TestDraftNamesItsBead(t *testing.T) {
	d, err := parseDraft(`{"title":"Migrate the store","bead":"NAC-9"}`)
	if err != nil || d.Bead != "NAC-9" {
		t.Errorf("bead = %q, %v", d.Bead, err)
	}
	if _, err := parseDraft(`{"title":"x","bead":"not a bead; rm -rf"}`); err == nil {
		t.Error("a bad bead id was accepted")
	}
}

func TestLaterDraftingMessagesRepeatTheReminder(t *testing.T) {
	m, rec, fdir := draftingChat(t, map[string]string{"drafting": "light_plain", "router": "full", "full": "light_plain"})
	sendAndWait(t, m, rec.ID, "c_remind01", "first")
	sendAndWait(t, m, rec.ID, "c_remind02", "second")
	cs := calls(t, fdir)
	if kinds(cs) != "drafting,drafting" {
		t.Fatalf("calls %s", kinds(cs))
	}
	first, second := userTexts(t, cs[0].stdin[0]), userTexts(t, cs[1].stdin[0])
	if !strings.HasPrefix(first[0], "New effort on") {
		t.Errorf("first message lacks the drafting prompt: %q", first[0])
	}
	if !strings.HasPrefix(second[0], DraftingTurnNote) || !strings.Contains(second[0], IntakeMarker) {
		t.Errorf("second message lacks the reminder: %q", second[0])
	}
	m.EndDrafting(rec.ID)
	sendAndWait(t, m, rec.ID, "c_remind03", "third")
	cs = calls(t, fdir)
	if third := userTexts(t, cs[len(cs)-1].stdin[0]); strings.Contains(third[0], "Still drafting") {
		t.Errorf("reminder after drafting ended: %q", third[0])
	}
}
