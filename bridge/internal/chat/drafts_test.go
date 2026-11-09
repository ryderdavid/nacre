package chat

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractDrafts(t *testing.T) {
	text := "Here is the draft.\n\n```nacre-draft\n{\"title\": \"Attach PDFs to chat\", \"type\": \"Feature\", \"description\": \"Let Ryder send PDFs.\", \"acceptance\": [\"a PDF reaches the agent\", \"over 10 MB is refused\"]}\n```\n\nShall I change anything?"
	rest, found := ExtractDrafts(text)
	if rest != "Here is the draft.\n\nShall I change anything?" {
		t.Errorf("rest = %q", rest)
	}
	if len(found) != 1 || found[0].Err != nil || found[0].Draft == nil {
		t.Fatalf("found = %+v", found)
	}
	d := *found[0].Draft
	want := Draft{
		Title: "Attach PDFs to chat", Type: "feature", Description: "Let Ryder send PDFs.",
		Acceptance: "- a PDF reaches the agent\n- over 10 MB is refused",
	}
	if d != want {
		t.Errorf("draft = %+v, want %+v", d, want)
	}

	// Ordinary code blocks and prose naming the block are left alone.
	plain := "Use a `nacre-draft` block.\n```json\n{\"title\": \"x\"}\n```"
	if rest, found := ExtractDrafts(plain); rest != plain || found != nil {
		t.Errorf("plain text changed: %q %+v", rest, found)
	}

	// Two blocks, tilde fences and a design field.
	two := "~~~nacre-draft\n{\"title\":\"One\",\"design\":\"Do it simply.\"}\n~~~\nthen\n````nacre-draft\n{\"title\":\"Two\"}\n````"
	rest, found = ExtractDrafts(two)
	if rest != "then" || len(found) != 2 || found[0].Draft.Title != "One" || found[0].Draft.Design != "Do it simply." || found[1].Draft.Title != "Two" {
		t.Errorf("two blocks: %q %+v", rest, found)
	}
}

func TestMalformedDraftBlocks(t *testing.T) {
	for _, tc := range []struct{ name, text, reason string }{
		{"bad json", "```nacre-draft\n{\"title\": \"x\",}\n```", "it isn't valid JSON"},
		{"no title", "```nacre-draft\n{\"description\": \"only this\"}\n```", "it has no title"},
		{"blank title", "```nacre-draft\n{\"title\": \"  \"}\n```", "it has no title"},
		{"empty", "```nacre-draft\n```", "the block is empty"},
		{"unclosed", "ok\n```nacre-draft\n{\"title\": \"x\"}", "the block isn't closed"},
	} {
		rest, found := ExtractDrafts(tc.text)
		if strings.Contains(rest, "nacre-draft") {
			t.Errorf("%s: block left in the shown text: %q", tc.name, rest)
		}
		if len(found) != 1 || found[0].Draft != nil || found[0].Err == nil || !strings.HasPrefix(found[0].Err.Error(), tc.reason) {
			t.Errorf("%s: found = %+v", tc.name, found)
			continue
		}
		evs := draftEvents(found, "t_1", RouteFull)
		if len(evs) != 1 || evs[0].Kind != KindError || evs[0].Text != DraftUnreadable+found[0].Err.Error() {
			t.Errorf("%s: events = %+v", tc.name, evs)
		}
		if !strings.HasPrefix(evs[0].Text, "The draft couldn't be read: ") {
			t.Errorf("%s: note = %q", tc.name, evs[0].Text)
		}
	}
}

// draftClaude prints one assistant text holding a draft block and one with a
// malformed block.
const draftClaude = `printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"Draft below.\n\n` + "```" + `nacre-draft\n{\"title\":\"Fold support\",\"type\":\"task\",\"description\":\"Make it fold.\"}\n` + "```" + `"}]}}'
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"` + "```" + `nacre-draft\n{\"type\":\"task\"}\n` + "```" + `"}]}}'
`

func TestDraftEventsInAChat(t *testing.T) {
	bin, _ := fakeClaude(t, "0.1", draftClaude)
	_, _, opt := repoSetup(t)
	m := newManager(t, t.TempDir(), bin, opt)
	rec, err := m.Create(CreateRequest{Agent: "claude", Repo: strp("r-a")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_draft001", Text: "draft it", Context: &Context{Screen: "board", Repo: "r-a"}}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m, rec.ID)
	var texts []string
	var drafts []Draft
	var errs []string
	for _, ev := range allEvents(t, m, rec.ID) {
		switch ev.Kind {
		case "text":
			texts = append(texts, ev.Text)
		case KindDraft:
			if ev.Draft == nil || ev.Route != RouteFull {
				t.Errorf("draft event = %+v", ev)
				continue
			}
			drafts = append(drafts, *ev.Draft)
		case KindError:
			errs = append(errs, ev.Text)
		}
	}
	for _, s := range texts {
		if strings.Contains(s, "nacre-draft") {
			t.Errorf("block shown in text %q", s)
		}
	}
	if strings.Join(texts, "|") != "working on it|Draft below." {
		t.Errorf("texts = %q", texts)
	}
	if len(drafts) != 1 || drafts[0] != (Draft{Title: "Fold support", Type: "task", Description: "Make it fold."}) {
		t.Errorf("drafts = %+v", drafts)
	}
	if len(errs) != 1 || errs[0] != "The draft couldn't be read: it has no title" {
		t.Errorf("errors = %q", errs)
	}
	if got := waitIdle(t, m, rec.ID); got.State != StateIdle {
		t.Errorf("state = %s, want idle (the conversation continues)", got.State)
	}
}

func TestNewEffortDraftingPromptOnTheFirstMessageOnly(t *testing.T) {
	bin, fdir := fakeClaude(t, "0", "")
	repoA, _, opt := repoSetup(t)
	m := newManager(t, t.TempDir(), bin, opt)
	rec, err := m.Create(CreateRequest{Agent: "claude", Repo: strp("r-a"), NewEffort: true})
	if err != nil {
		t.Fatal(err)
	}
	if want := "New effort · " + filepath.Base(repoA); rec.Title != want || !rec.NewEffort {
		t.Fatalf("title = %q new_effort=%v, want %q", rec.Title, rec.NewEffort, want)
	}
	ctx := &Context{Screen: "board", Repo: "r-a"}
	for _, id := range []string{"c_effort01", "c_effort02"} {
		if _, err := m.Send(rec.ID, SendRequest{TurnID: id, Text: "pdfs in chat", Context: ctx}); err != nil {
			t.Fatal(err)
		}
		waitIdle(t, m, rec.ID)
	}
	first := userTexts(t, readLines(t, filepath.Join(fdir, "stdin0"))[0])
	second := userTexts(t, readLines(t, filepath.Join(fdir, "stdin1"))[0])
	prompt := DraftingPrompt(filepath.Base(repoA))
	if len(first) != 2 || !strings.HasPrefix(first[0], prompt+"\n\nOn screen:") || first[1] != "pdfs in chat" {
		t.Errorf("first message = %q", first)
	}
	// Later drafting messages carry the short drafting reminder, not the whole prompt.
	if len(second) != 2 || strings.Contains(second[0], "New effort on") || !strings.HasPrefix(second[0], DraftingTurnNote+"\n\nOn screen: same as before") {
		t.Errorf("second message = %q", second)
	}
	for _, want := range []string{"nacred refuses anything else", "`nacre-draft`", "\"title\"", "\"type\"", "\"description\"", "\"design\"", "\"acceptance\"",
		"\"digest\"", "Ruled out:", "at most three numbered questions", "name the assumptions", "\"Changed: …\"", "Where does this belong?", "When Ryder says to go ahead", "`NACRE_INTAKE`", "\"bead\" field"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("drafting prompt lacks %q", want)
		}
	}
	if strings.Contains(prompt, "sheet") {
		t.Error("drafting prompt still mentions the removed New effort sheet")
	}
	got, _, _ := m.Get(rec.ID, 0)
	if got.Title != "New effort · "+filepath.Base(repoA) {
		t.Errorf("title changed to %q by the first message", got.Title)
	}

	// An ordinary chat gets no drafting prompt.
	plain, _ := m.Create(CreateRequest{Agent: "claude", Repo: strp("r-a")})
	m.Send(plain.ID, SendRequest{TurnID: "c_plain001", Text: "hi", Context: ctx})
	waitIdle(t, m, plain.ID)
	if txt := userTexts(t, readLines(t, filepath.Join(fdir, "stdin2"))[0]); strings.Contains(txt[0], "New effort on") {
		t.Errorf("plain chat got the drafting prompt: %q", txt)
	}
}
