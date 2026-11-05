package server

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/chat"
)

// imageChat is a server with one Claude chat whose turns run a stand-in
// that saves each turn's stdin (stdin0, stdin1, ...) next to itself. It
// reads with head rather than the shell's read, which would take bytes one
// at a time from a 20 MiB line.
type imageChat struct {
	ts      *httptest.Server
	id      string
	chatDir string // chats/
	fake    string
	repoID  string
}

func newImageChat(t *testing.T) *imageChat {
	t.Helper()
	fake := t.TempDir()
	bin := filepath.Join(fake, "claude")
	script := `#!/bin/sh
d="$(dirname "$0")"
n=$(ls "$d" | grep -c '^stdin')
head -n 1 > "$d/stdin$n"
printf '{"type":"system","subtype":"init","session_id":"s"}\n'
printf '{"type":"result","subtype":"success","is_error":false,"result":"ok","total_cost_usd":0.01}\n'
cat > /dev/null
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	s, repoID := chatServer(t, bin, func(o *chat.Options) { o.Dir = dir })
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	code, body := call(t, "POST", ts.URL+"/v1/chats", `{"agent":"claude","repo":"`+repoID+`"}`)
	if code != 201 {
		t.Fatalf("create: %d %v", code, body)
	}
	return &imageChat{ts: ts, id: body["chat"].(map[string]any)["id"].(string), chatDir: dir, fake: fake, repoID: repoID}
}

func (ic *imageChat) send(t *testing.T, turnID, text string, images []map[string]any) (int, []byte) {
	t.Helper()
	body := map[string]any{"turn_id": turnID, "text": text, "context": map[string]any{"screen": "board", "repo": ic.repoID}}
	if images != nil {
		body["images"] = images
	}
	resp, out := postImageJSON(t, ic.ts.URL+"/v1/chats/"+ic.id+"/messages", body)
	return resp.StatusCode, out
}

func (ic *imageChat) turns() int {
	m, _ := filepath.Glob(filepath.Join(ic.fake, "stdin*"))
	return len(m)
}

func (ic *imageChat) stored() []string {
	entries, _ := os.ReadDir(filepath.Join(ic.chatDir, ic.id, "images"))
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func (ic *imageChat) events(t *testing.T) []any {
	t.Helper()
	_, body := call(t, "GET", ic.ts.URL+"/v1/chats/"+ic.id, "")
	evs, _ := body["events"].([]any)
	return evs
}

func TestChatMessageWithFiveLargestImages(t *testing.T) {
	ic := newImageChat(t)
	var images []map[string]any
	for i := 1; i <= maxChatImages; i++ {
		images = append(images, img("image/png", fmt.Sprintf("shot%d.png", i), pngOf(maxImageBytes)))
	}
	code, out := ic.send(t, "c_five0001", "what do these show?", images)
	if code != http.StatusAccepted {
		t.Fatalf("5 images of 3 MiB: %d %.300s", code, out)
	}
	rec := waitChatIdle(t, ic.ts.URL, ic.id)
	if rec["image_count"] != float64(5) || rec["image_bytes"] != float64(5*maxImageBytes) {
		t.Fatalf("record: %v images, %v bytes", rec["image_count"], rec["image_bytes"])
	}
	stdin, _ := os.ReadFile(filepath.Join(ic.fake, "stdin0"))
	if n := bytes.Count(stdin, []byte(`"type":"image"`)); n != 5 {
		t.Fatalf("stdin has %d image blocks", n)
	}
	if len(ic.stored()) != 5 {
		t.Fatalf("stored %v", ic.stored())
	}

	// The chat is now at its 15 MiB budget: another image is refused with
	// 409 and the reason, and nothing is written or started.
	code, out = ic.send(t, "c_five0002", "one more", []map[string]any{img("image/png", "x.png", pngOf(64))})
	if code != http.StatusConflict || !strings.Contains(string(out), chat.ImageLimitReason) {
		t.Fatalf("over the chat budget: %d %s", code, out)
	}
	if ic.turns() != 1 || len(ic.stored()) != 5 {
		t.Fatalf("refused message ran %d turns, stored %d images", ic.turns(), len(ic.stored()))
	}
	// The text alone is still sent, with no images.
	if code, out := ic.send(t, "c_five0003", "one more", nil); code != http.StatusAccepted {
		t.Fatalf("text-only follow-up: %d %s", code, out)
	}
	waitChatIdle(t, ic.ts.URL, ic.id)
	if stdin, _ := os.ReadFile(filepath.Join(ic.fake, "stdin1")); bytes.Contains(stdin, []byte(`"type":"image"`)) {
		t.Fatal("the follow-up sent images again")
	}
}

func TestChatMessageImageLimitsAndValidation(t *testing.T) {
	ic := newImageChat(t)
	ok := img("image/png", "ok.png", pngOf(100))
	six := []map[string]any{ok, ok, ok, ok, ok, ok}
	cases := map[string]struct {
		images []map[string]any
		want   string
	}{
		"six images":  {six, "too many images: 6, at most 5 per chat message"},
		"media type":  {[]map[string]any{ok, img("image/bmp", "x.bmp", pngOf(10))}, `image 2 (x.bmp): media type \"image/bmp\"`},
		"base64":      {[]map[string]any{{"media_type": "image/png", "name": "bad.png", "data": "not base64!"}}, "image 1 (bad.png): data is not valid base64"},
		"signature":   {[]map[string]any{img("image/jpeg", "fake.jpg", pngOf(10))}, "image 1 (fake.jpg): bytes are not a jpeg image"},
		"image size":  {[]map[string]any{img("image/png", "huge.png", pngOf(maxImageBytes+1))}, "image 1 (huge.png): over the 3 MiB limit"},
		"empty image": {[]map[string]any{img("image/png", "e.png", nil)}, "image 1 (e.png): data is empty"},
	}
	for name, c := range cases {
		code, out := ic.send(t, "c_bad00001", "look", c.images)
		if code != http.StatusBadRequest || !strings.Contains(string(out), c.want) {
			t.Errorf("%s: %d %.300s, want 400 %q", name, code, out, c.want)
		}
	}
	// Nothing was written or started: no image directory, no event, no
	// turn, and the turn id is still free.
	if _, err := os.Stat(filepath.Join(ic.chatDir, ic.id)); !os.IsNotExist(err) {
		t.Errorf("chat directory written for a refused message: %v", err)
	}
	if evs := ic.events(t); len(evs) != 0 {
		t.Errorf("events logged for refused messages: %v", evs)
	}
	if ic.turns() != 0 {
		t.Errorf("%d turns ran for refused messages", ic.turns())
	}
	if code, out := ic.send(t, "c_bad00001", "look", []map[string]any{ok}); code != http.StatusAccepted {
		t.Fatalf("valid message after refusals: %d %s", code, out)
	}
	waitChatIdle(t, ic.ts.URL, ic.id)

	// The per-repo turn endpoints still take 20 images for older apps.
	is := newImageServer(t, "cat <<'EOF'\n"+imageTranscript+"\nEOF")
	var twenty []map[string]any
	for i := 0; i < maxImages; i++ {
		twenty = append(twenty, img("image/png", fmt.Sprintf("s%d.png", i), pngOf(100)))
	}
	resp, out := postImageJSON(t, is.url("json"), map[string]any{"prompt": "look", "schema": map[string]any{"type": "object"}, "images": twenty})
	if resp.StatusCode != 200 {
		t.Fatalf("per-repo turn with 20 images: %d %s", resp.StatusCode, out)
	}
}

func TestChatMessageBodyLimit(t *testing.T) {
	ic := newImageChat(t)
	tooBig := append([]byte(`{"turn_id":"c_big00001","text":"`), bytes.Repeat([]byte("a"), maxTurnBody)...)
	tooBig = append(tooBig, `"}`...)
	resp, out := postRaw(t, ic.ts.URL+"/v1/chats/"+ic.id+"/messages", tooBig)
	if resp.StatusCode != http.StatusRequestEntityTooLarge || !strings.Contains(string(out), "32 MiB") {
		t.Fatalf("over 32 MiB: %d %.200s", resp.StatusCode, out)
	}
	if ic.turns() != 0 || len(ic.events(t)) != 0 {
		t.Fatal("an oversized message started a turn")
	}
	// A body over 1 MiB without images is read (and refused only as text
	// too long), while the other chat endpoints keep 1 MiB.
	long := append([]byte(`{"turn_id":"c_big00002","text":"`), bytes.Repeat([]byte("a"), maxBody)...)
	long = append(long, `"}`...)
	if resp, out := postRaw(t, ic.ts.URL+"/v1/chats/"+ic.id+"/messages", long); resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(out), "text too long") {
		t.Errorf("long text: %d %.200s", resp.StatusCode, out)
	}
	overOne := append([]byte(`{"agent":"claude","context":{"screen":"boards","summary":"`), bytes.Repeat([]byte("a"), maxBody)...)
	overOne = append(overOne, `"}}`...)
	if resp, out := postRaw(t, ic.ts.URL+"/v1/chats", overOne); resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(out), "too large") {
		t.Errorf("create over 1 MiB: %d %.200s", resp.StatusCode, out)
	}
}

func TestChatImageEndpointServesTheStoredFile(t *testing.T) {
	ic := newImageChat(t)
	data := pngOf(2048)
	data[2047] = 7
	if code, out := ic.send(t, "c_serve001", "see", []map[string]any{img("image/png", "shot.png", data)}); code != http.StatusAccepted {
		t.Fatalf("send: %d %s", code, out)
	}
	waitChatIdle(t, ic.ts.URL, ic.id)
	var ref map[string]any
	for _, e := range ic.events(t) {
		ev := e.(map[string]any)
		if ev["kind"] == "user" {
			imgs, _ := ev["images"].([]any)
			if len(imgs) != 1 {
				t.Fatalf("user event %v", ev)
			}
			ref = imgs[0].(map[string]any)
		}
	}
	if ref["name"] != "shot.png" || ref["media_type"] != "image/png" || ref["bytes"] != float64(2048) {
		t.Fatalf("image reference %v", ref)
	}
	get := func(path string) (*http.Response, []byte) {
		req, _ := http.NewRequest("GET", ic.ts.URL+path, nil)
		req.Header.Set("Authorization", "Bearer tok")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, b
	}
	url := "/v1/chats/" + ic.id + "/images/" + ref["id"].(string)
	resp, body := get(url)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" || !bytes.Equal(body, data) {
		t.Fatalf("GET image: %d %s, %d bytes", resp.StatusCode, resp.Header.Get("Content-Type"), len(body))
	}
	// It needs the owner's token like every other endpoint.
	if r, err := http.Get(ic.ts.URL + url); err != nil || r.StatusCode != http.StatusUnauthorized {
		t.Errorf("without a token: %v %v", r.StatusCode, err)
	}
	for _, bad := range []string{"im_000000000000000000000000.png", "..%2F" + ic.id + ".json", "shot.png"} {
		if resp, _ := get("/v1/chats/" + ic.id + "/images/" + bad); resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: %d", bad, resp.StatusCode)
		}
	}
	// Deleted with the chat.
	if code, _ := call(t, "DELETE", ic.ts.URL+"/v1/chats/"+ic.id, ""); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if resp, _ := get(url); resp.StatusCode != http.StatusNotFound {
		t.Errorf("image after delete: %d", resp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(ic.chatDir, ic.id)); !os.IsNotExist(err) {
		t.Errorf("images left after delete: %v", err)
	}
}
