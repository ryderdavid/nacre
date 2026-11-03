package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var testImage = Image{MediaType: "image/png", Name: "shot.png", Data: []byte("\x89PNG\r\n\x1a\nxx")}

// The argv of today's text-only turns, pinned, so an image turn can be
// compared with exactly what a text turn runs.
func TestHeadlessTextOnlyArgsUnchanged(t *testing.T) {
	ask := Headless{Prompt: "what is this?", System: "sys", Schema: json.RawMessage(`{"type":"object"}`)}
	want := []string{"-p", "what is this?", "--safe-mode", "--permission-prompts", "none",
		"--output-format", "stream-json", "--verbose", "--system-prompt", "sys",
		"--json-schema", `{"type":"object"}`, "--tools", ""}
	want = append(append(want, budgetArgs(0, RunawayUSD)...), "--no-session-persistence")
	if got := ask.Args(); !slices.Equal(got, want) {
		t.Fatalf("text Ask argv changed:\n got %q\nwant %q", got, want)
	}
}

func TestHeadlessImageAskArgs(t *testing.T) {
	ask := Headless{Prompt: "what is this? [image 1: shot.png]", System: "sys", Schema: json.RawMessage(`{"type":"object"}`)}
	text := ask.Args()
	ask.Images = []Image{testImage}
	got := ask.Args()
	want := []string{"-p", "--input-format", "stream-json", "--safe-mode", "--permission-prompts", "none",
		"--output-format", "stream-json", "--verbose", "--system-prompt", "sys",
		"--json-schema", `{"type":"object"}`, "--tools", ""}
	want = append(append(want, budgetArgs(0, RunawayUSD)...), "--no-session-persistence")
	if !slices.Equal(got, want) {
		t.Fatalf("image Ask argv:\n got %q\nwant %q", got, want)
	}
	// Exactly today's argv with the positional prompt swapped for
	// --input-format stream-json: every other flag is unchanged.
	if !slices.Equal(append([]string{"-p", "--input-format", "stream-json"}, text[2:]...), got) {
		t.Fatalf("flags differ from the text turn: %q vs %q", got, text)
	}
	if strings.Contains(strings.Join(got, "\x00"), "what is this?") {
		t.Fatalf("prompt leaked into argv: %q", got)
	}
}

func TestHeadlessImageAssistantArgs(t *testing.T) {
	as := Headless{Prompt: "and this? [image 1: shot.png]", System: "sys", AllowedTools: BdReadTools, Resume: "sess-1", Persist: true}
	text := as.Args()
	as.Images = []Image{testImage}
	got := as.Args()
	if !slices.Equal(append([]string{"-p", "--input-format", "stream-json"}, text[2:]...), got) {
		t.Fatalf("image assistant argv %q, text %q", got, text)
	}
	if i := index(got, "--resume"); i < 0 || got[i+1] != "sess-1" {
		t.Fatalf("resume lost: %q", got)
	}
	// A kept turn stays kept, as today; an unkept one stays unpersisted.
	if index(got, "--no-session-persistence") >= 0 {
		t.Fatalf("kept image turn got --no-session-persistence: %q", got)
	}
	as.Persist = false
	if index(as.Args(), "--no-session-persistence") < 0 {
		t.Fatalf("unkept image turn lost --no-session-persistence: %q", as.Args())
	}
	if strings.Contains(strings.Join(got, "\x00"), "and this?") {
		t.Fatalf("prompt leaked into argv: %q", got)
	}
}

func TestUserMessage(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "1.png"), filepath.Join(dir, "2.jpg")
	os.WriteFile(a, []byte("AAA"), 0o600)
	os.WriteFile(b, []byte("BB"), 0o600)
	got, err := UserMessage("look [image 1: a.png] [image 2: b.jpg]", []StagedImage{
		{MediaType: "image/png", Name: "a.png", Path: a},
		{MediaType: "image/jpeg", Name: "b.jpg", Path: b},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"message":{"content":[` +
		`{"text":"Image 1: a.png","type":"text"},` +
		`{"source":{"data":"QUFB","media_type":"image/png","type":"base64"},"type":"image"},` +
		`{"text":"Image 2: b.jpg","type":"text"},` +
		`{"source":{"data":"QkI=","media_type":"image/jpeg","type":"base64"},"type":"image"},` +
		`{"text":"look [image 1: a.png] [image 2: b.jpg]","type":"text"}` +
		`],"role":"user"},"type":"user"}` + "\n"
	if string(got) != want {
		t.Fatalf("stdin:\n got %s\nwant %s", got, want)
	}
	if strings.Count(string(got), "\n") != 1 {
		t.Fatal("stream-json input is one message on one line")
	}
}
