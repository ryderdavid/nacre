package chat

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseNeedsYou(t *testing.T) {
	for _, c := range []struct {
		in, start string
		ok        bool
	}{
		{"/needs-you", "", true},
		{"  /needs-you  ", "", true},
		{"/needs-you dc_4c442ae0499a1a23", "dc_4c442ae0499a1a23", true},
		{"/needs-you a b", "", false},
		{"/needs-you $(rm -rf)", "", false},
		{"go through /needs-you", "", false},
		{"/needs-yous", "", false},
	} {
		start, ok := ParseNeedsYou(c.in)
		if ok != c.ok || start != c.start {
			t.Errorf("ParseNeedsYou(%q) = %q, %v; want %q, %v", c.in, start, ok, c.start, c.ok)
		}
	}
}

func TestNeedsYouPromptStartsAtTheGivenDecision(t *testing.T) {
	p := NeedsYouPrompt("dc_1")
	for _, want := range []string{"nacred decisions", "Start with decision `dc_1`", "```nacre-ask", "Never answer a decision yourself", "only a tap on my phone answers"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	if strings.Contains(NeedsYouPrompt(""), "Start with decision") {
		t.Error("prompt without a start names one")
	}
}

func TestExtractAsks(t *testing.T) {
	text := "1 · Approve export-chats.\n\n```nacre-ask\n{\"decision\": \"dc_abc\"}\n```\n"
	rest, ids := ExtractAsks(text)
	if rest != "1 · Approve export-chats." || len(ids) != 1 || ids[0] != "dc_abc" {
		t.Fatalf("got %q %v", rest, ids)
	}
	rest, ids = ExtractAsks("Hi\n```nacre-ask\nnot json\n```")
	if rest != "Hi" || len(ids) != 0 {
		t.Fatalf("a bad block: got %q %v, want it dropped", rest, ids)
	}
	if rest, ids := ExtractAsks("plain text"); rest != "plain text" || ids != nil {
		t.Fatalf("plain: %q %v", rest, ids)
	}
}

func TestHeadsUpText(t *testing.T) {
	got := HeadsUpText([]string{"approve", "approve", "land", "question", "approve", "found"})
	want := "6 things need you here: 1 question, 1 found-work note, 3 approvals and 1 round to land. Go through them one by one?"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if got := HeadsUpText([]string{"signoff"}); got != "1 thing needs you here: 1 sign-off. Go through it now?" {
		t.Errorf("one: %q", got)
	}
	if HeadsUpText(nil) != "" {
		t.Error("nothing waits, but there is a heads-up")
	}
}

func TestNewChatGetsOneHeadsUp(t *testing.T) {
	bin, _ := fakeClaude(t, "0.1", "")
	_, _, opt := repoSetup(t)
	m := newManager(t, t.TempDir(), bin, opt)
	m.SetDecisionHooks(DecisionHooks{PendingKinds: func(repo string) []string {
		if repo != "r-a" {
			t.Errorf("repo = %q", repo)
		}
		return []string{"approve", "land"}
	}})
	rec, err := m.Create(CreateRequest{Agent: "claude", Repo: strp("r-a")})
	if err != nil {
		t.Fatal(err)
	}
	var hu []Event
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if hu = eventsOf(t, m, rec.ID, KindNeedsYou); len(hu) > 0 {
			break
		}
	}
	if len(hu) != 1 || hu[0].Total != 2 || !strings.HasPrefix(hu[0].Text, "2 things need you here") {
		t.Fatalf("heads-up = %+v", hu)
	}
	m.headsUp(rec.ID, "r-a")
	if n := len(eventsOf(t, m, rec.ID, KindNeedsYou)); n != 1 {
		t.Fatalf("%d heads-ups, want one", n)
	}

	// A chat with no repository, or nothing waiting, gets none.
	none := newManager(t, t.TempDir(), bin, opt)
	none.SetDecisionHooks(DecisionHooks{PendingKinds: func(string) []string { return nil }})
	r2, _ := none.Create(CreateRequest{Agent: "claude", Repo: strp("r-a")})
	none.headsUp(r2.ID, "r-a")
	if len(eventsOf(t, none, r2.ID, KindNeedsYou)) != 0 {
		t.Fatal("heads-up with nothing waiting")
	}
}

const askReply = `printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"1 · Approve export-chats.\n\n` + "```nacre-ask\\n{\\\"decision\\\": \\\"dc_abc\\\"}\\n```" + `"}]}}'`

func TestWalkThroughRunsLightAndRecordsTheAsk(t *testing.T) {
	bin, fdir := fakeClaude(t, "0.1", askReply)
	_, _, opt := repoSetup(t)
	m := newManager(t, t.TempDir(), bin, opt, func(o *Options) { o.RouterModel, o.LightModel = "haiku", "sonnet" })
	var mu sync.Mutex
	var asked []string
	m.SetDecisionHooks(DecisionHooks{Asked: func(repo, id string) bool {
		mu.Lock()
		defer mu.Unlock()
		asked = append(asked, repo+"/"+id)
		return id == "dc_abc"
	}})
	rec, err := m.Create(CreateRequest{Agent: "claude", Repo: strp("r-a")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Send(rec.ID, SendRequest{Text: "/needs-you"}); err != nil {
		t.Fatal(err)
	}
	got := waitIdle(t, m, rec.ID)
	if got.Title != "Needs you" {
		t.Errorf("title = %q", got.Title)
	}
	if got.Asking != "dc_abc" {
		t.Errorf("asking = %q, want dc_abc", got.Asking)
	}
	// No router call: the first process is the light turn itself.
	argv := readLines(t, filepath.Join(fdir, "argv0"))
	if v, _ := flag(argv, "--model"); v != "sonnet" {
		t.Errorf("first process model = %q, want the light model", v)
	}
	sent := strings.Join(userTexts(t, readLines(t, filepath.Join(fdir, "stdin0"))[0]), "\n")
	if !strings.Contains(sent, "Go through what needs me, one item at a time.") {
		t.Errorf("agent did not get the walk-through prompt: %q", sent)
	}
	users := eventsOf(t, m, rec.ID, KindUser)
	if len(users) != 1 || users[0].Text != "/needs-you" {
		t.Errorf("transcript user events = %+v", users)
	}
	asks := eventsOf(t, m, rec.ID, KindAsk)
	if len(asks) != 1 || asks[0].Text != "dc_abc" {
		t.Errorf("ask events = %+v", asks)
	}
	for _, ev := range eventsOf(t, m, rec.ID, "text") {
		if strings.Contains(ev.Text, "nacre-ask") {
			t.Errorf("the block is shown as text: %q", ev.Text)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(asked) != 1 || asked[0] != "r-a/dc_abc" {
		t.Errorf("asked = %v", asked)
	}
}

func TestAForgedAskIsDropped(t *testing.T) {
	bin, _ := fakeClaude(t, "0.1", askReply)
	_, _, opt := repoSetup(t)
	m := newManager(t, t.TempDir(), bin, opt)
	m.SetDecisionHooks(DecisionHooks{Asked: func(repo, id string) bool { return false }})
	rec, _ := m.Create(CreateRequest{Agent: "claude", Repo: strp("r-a")})
	if _, err := m.Send(rec.ID, SendRequest{Text: "what waits?"}); err != nil {
		t.Fatal(err)
	}
	if got := waitIdle(t, m, rec.ID); got.Asking != "" {
		t.Fatalf("asking = %q, want the block dropped", got.Asking)
	}
	if n := len(eventsOf(t, m, rec.ID, KindAsk)); n != 0 {
		t.Fatalf("%d ask events", n)
	}
}

func TestAnsweredMessageCarriesTheDecisionState(t *testing.T) {
	bin, fdir := fakeClaude(t, "0.1", "")
	_, _, opt := repoSetup(t)
	m := newManager(t, t.TempDir(), bin, opt)
	m.SetDecisionHooks(DecisionHooks{State: func(id string) string { return "Approve export-chats: executing · answered \"Approve only\"" }})
	rec, _ := m.Create(CreateRequest{Agent: "claude", Repo: strp("r-a")})
	if _, err := m.Send(rec.ID, SendRequest{Text: "Answered: Approve only", Answered: "dc_abc"}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m, rec.ID)
	sent := strings.Join(userTexts(t, readLines(t, filepath.Join(fdir, "stdin0"))[0]), "\n")
	if !strings.Contains(sent, "Ryder answered decision `dc_abc` on his phone. nacred now reports: Approve export-chats: executing") {
		t.Errorf("turn lacks the decision state: %q", sent)
	}
	if u := eventsOf(t, m, rec.ID, KindUser); len(u) != 1 || u[0].Answered != "dc_abc" {
		t.Errorf("user event = %+v", u)
	}
	if _, err := m.Send(rec.ID, SendRequest{Text: "x", Answered: "dc abc"}); err == nil {
		t.Error("a bad answered id was accepted")
	}
}

func TestSetAsking(t *testing.T) {
	bin, _ := fakeClaude(t, "0.1", "")
	m := newManager(t, t.TempDir(), bin)
	rec, _ := m.Create(CreateRequest{Agent: "claude"})
	m.SetAsking(rec.ID, "dc_9")
	m.SetAsking("ch_000000000000000000000000", "dc_9")
	if r, _, _ := m.Get(rec.ID, 0); r.Asking != "dc_9" {
		t.Fatalf("asking = %q", r.Asking)
	}
}

func TestDecisionCommandsAreReadOnly(t *testing.T) {
	for _, c := range []string{"nacred decisions", "nacred decisions r-a", "nacred decision dc_abc"} {
		if !readOnlyCommand(c) {
			t.Errorf("%q refused", c)
		}
	}
	for _, c := range []string{"nacred decisions --answer", "nacred round land r-1", "nacred decision dc_1; rm x"} {
		if readOnlyCommand(c) {
			t.Errorf("%q allowed", c)
		}
	}
}
