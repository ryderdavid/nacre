package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/assist"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/config"
	"github.com/ryderdavid/nacre/bridge/internal/discovery"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
	"github.com/ryderdavid/nacre/bridge/internal/sse"
)

// imageServer serves one repo with a fake claude that records its argv,
// stdin and the staged directories it saw into out, then runs tail. Images
// are staged under base.
type imageServer struct {
	ts       *httptest.Server
	repo     string
	base     string
	out      string
	runner   *assist.Runner
}

func newImageServer(t *testing.T, tail string) *imageServer {
	t.Helper()
	repoDir, base, out := t.TempDir(), t.TempDir(), t.TempDir()
	bin := filepath.Join(t.TempDir(), "claude")
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do echo \"$a\"; done > " + out + "/argv\n" +
		"cat > " + out + "/stdin\n" +
		"ls " + base + " > " + out + "/staged\n" +
		tail + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	mgr, err := runs.New(runs.Options{Dir: t.TempDir(), ClaudePath: bin})
	if err != nil {
		t.Fatal(err)
	}
	runner := &assist.Runner{ClaudePath: bin, TempDir: base, Timeout: 30 * time.Second}
	s := &Server{
		Config: &config.Config{Token: "tok"},
		Repos:  repos.New(func() []string { return []string{repoDir} }, bd.New("/nonexistent", "")),
		BD:     bd.New("/nonexistent", ""),
		Runs:   mgr,
		Assist: runner,
		Broker: sse.NewBroker(),
		Log:    log.New(io.Discard, "", 0),
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return &imageServer{ts: ts, repo: discovery.RepoID(repoDir), base: base, out: out, runner: runner}
}

func (is *imageServer) url(kind string) string {
	return is.ts.URL + "/v1/repos/" + is.repo + "/claude/" + kind
}

func (is *imageServer) read(name string) string {
	b, _ := os.ReadFile(filepath.Join(is.out, name))
	return string(b)
}

// staged lists nacred's turn directories under base.
func (is *imageServer) staged() []string {
	m, _ := filepath.Glob(filepath.Join(is.base, assist.StagePrefix+"*"))
	return m
}

func (is *imageServer) waitClean(t *testing.T) {
	t.Helper()
	for i := 0; i < 500; i++ {
		if len(is.staged()) == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("staged directories left: %v", is.staged())
}

// pngOf is a PNG signature padded to n bytes.
func pngOf(n int) []byte {
	b := make([]byte, n)
	copy(b, "\x89PNG\r\n\x1a\n")
	return b
}

func img(mediaType, name string, data []byte) map[string]any {
	return map[string]any{"media_type": mediaType, "name": name, "data": base64.StdEncoding.EncodeToString(data)}
}

func postImageJSON(t *testing.T, url string, body any) (*http.Response, []byte) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return postRaw(t, url, b)
}

func postRaw(t *testing.T, url string, body []byte) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp, out
}

const imageTranscript = `{"type":"system","subtype":"init","session_id":"s-1"}
{"type":"result","subtype":"success","is_error":false,"session_id":"s-1","result":"{\"reply\":\"a cat\"}","structured_output":{"reply":"a cat"}}`

func TestLargestImageRequestIsAccepted(t *testing.T) {
	for _, kind := range []string{"json", "stream"} {
		t.Run(kind, func(t *testing.T) {
			is := newImageServer(t, "cat <<'EOF'\n"+imageTranscript+"\nEOF")
			images := []map[string]any{}
			prompt := "describe these"
			for i := 1; i <= maxImages; i++ {
				images = append(images, img("image/png", fmt.Sprintf("shot%d.png", i), pngOf(maxImagesBytes/maxImages)))
				prompt += fmt.Sprintf(" [image %d: shot%d.png]", i, i)
			}
			body := map[string]any{"prompt": prompt, "schema": map[string]any{"type": "object"}, "images": images}
			resp, out := postImageJSON(t, is.url(kind), body)
			if resp.StatusCode != 200 {
				t.Fatalf("status %d: %s", resp.StatusCode, out)
			}
			if kind == "stream" && !strings.Contains(string(out), "event: done") {
				t.Fatalf("stream = %.500s", out)
			}
			if kind == "json" && !strings.Contains(string(out), "a cat") {
				t.Fatalf("body = %s", out)
			}
			stdin := is.read("stdin")
			if strings.Count(stdin, `"type":"image"`) != maxImages || !strings.Contains(stdin, `"Image 20: shot20.png"`) {
				t.Fatalf("stdin has %d images", strings.Count(stdin, `"type":"image"`))
			}
			if i1, i20, p := strings.Index(stdin, "Image 1: shot1.png"), strings.Index(stdin, "Image 20: shot20.png"), strings.Index(stdin, "describe these"); !(i1 < i20 && i20 < p) {
				t.Fatalf("images must come in order before the prompt: %d %d %d", i1, i20, p)
			}
			is.waitClean(t)
		})
	}
}

func TestTurnBodyLimit(t *testing.T) {
	is := newImageServer(t, "touch "+"/dev/null")
	tooBig := append([]byte(`{"prompt":"`), bytes.Repeat([]byte("a"), maxTurnBody)...)
	tooBig = append(tooBig, `"}`...)
	for _, kind := range []string{"json", "stream"} {
		resp, out := postRaw(t, is.url(kind), tooBig[:maxTurnBody+1])
		if resp.StatusCode != http.StatusRequestEntityTooLarge || !strings.Contains(string(out), "32 MiB") {
			t.Errorf("%s: status %d: %s", kind, resp.StatusCode, out)
		}
	}
	// Every other endpoint, including the assistant across boards, keeps 1 MiB.
	overOne := append([]byte(`{"prompt":"`), bytes.Repeat([]byte("a"), maxBody)...)
	overOne = append(overOne, `","boards":[{"repo":"x"}]}`...)
	for _, url := range []string{is.ts.URL + "/v1/claude/stream", is.ts.URL + "/v1/sessions"} {
		resp, out := postRaw(t, url, overOne)
		if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(out), "too large") {
			t.Errorf("%s: status %d: %.200s", url, resp.StatusCode, out)
		}
	}
	if is.read("argv") != "" {
		t.Fatal("claude ran for a rejected body")
	}
}

func TestInvalidImagesAreRejectedBeforeAnything(t *testing.T) {
	is := newImageServer(t, "cat <<'EOF'\n"+imageTranscript+"\nEOF")
	ok := img("image/png", "ok.png", pngOf(100))
	many := []map[string]any{}
	for i := 0; i <= maxImages; i++ {
		many = append(many, ok)
	}
	big := []map[string]any{}
	for i := 0; i < 6; i++ {
		big = append(big, img("image/png", fmt.Sprintf("big%d.png", i), pngOf(maxImageBytes)))
	}
	cases := map[string]struct {
		images []map[string]any
		want   string
	}{
		"media type":     {[]map[string]any{ok, img("image/bmp", "x.bmp", pngOf(10))}, `image 2 (x.bmp): media type \"image/bmp\"`},
		"base64":         {[]map[string]any{{"media_type": "image/png", "name": "bad.png", "data": "not base64!"}}, "image 1 (bad.png): data is not valid base64"},
		"signature":      {[]map[string]any{img("image/jpeg", "fake.jpg", pngOf(10))}, "image 1 (fake.jpg): bytes are not a jpeg image"},
		"gif signature":  {[]map[string]any{img("image/gif", "a.gif", []byte("GIF90a...."))}, "image 1 (a.gif): bytes are not a gif image"},
		"webp signature": {[]map[string]any{img("image/webp", "a.webp", []byte("RIFF0000WEBX"))}, "image 1 (a.webp): bytes are not a webp image"},
		"image size":     {[]map[string]any{img("image/png", "huge.png", pngOf(maxImageBytes+1))}, "image 1 (huge.png): over the 3 MiB limit"},
		"count":          {many, "too many images: 21, at most 20"},
		"total":          {big, "images total more than the 15 MiB limit"},
		"empty":          {[]map[string]any{img("image/png", "e.png", nil)}, "image 1 (e.png): data is empty"},
	}
	for name, c := range cases {
		for _, kind := range []string{"json", "stream"} {
			resp, out := postImageJSON(t, is.url(kind), map[string]any{"prompt": "look", "images": c.images})
			if resp.StatusCode != 400 || !strings.Contains(string(out), c.want) {
				t.Errorf("%s/%s: status %d: %.300s, want %q", name, kind, resp.StatusCode, out, c.want)
			}
		}
	}
	if is.read("argv") != "" {
		t.Fatal("claude ran for an invalid request")
	}
	if entries, _ := os.ReadDir(is.base); len(entries) != 0 {
		t.Fatalf("files written for an invalid request: %v", entries)
	}
	// Every accepted type passes with its own signature.
	for _, good := range []map[string]any{
		img("image/jpeg", "a.jpg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 1}),
		img("image/gif", "a.gif", []byte("GIF89a....")),
		img("image/webp", "a.webp", []byte("RIFF\x10\x00\x00\x00WEBPVP8 ")),
	} {
		if resp, out := postImageJSON(t, is.url("json"), map[string]any{"prompt": "look", "schema": map[string]any{"type": "object"}, "images": []map[string]any{good}}); resp.StatusCode != 200 {
			t.Errorf("%v: status %d: %s", good["name"], resp.StatusCode, out)
		}
	}
}

func TestAssistantAcrossBoardsRefusesImages(t *testing.T) {
	is := newImageServer(t, "true")
	resp, out := postImageJSON(t, is.ts.URL+"/v1/claude/stream", map[string]any{
		"prompt": "look", "boards": []map[string]any{{"repo": "x"}}, "images": []map[string]any{img("image/png", "a.png", pngOf(10))},
	})
	if resp.StatusCode != 400 || !strings.Contains(string(out), "images are not accepted") {
		t.Fatalf("status %d: %s", resp.StatusCode, out)
	}
}

// The argv and stdin of image turns through the API, for an Ask turn with a
// schema (not kept) and an assistant turn that resumes a kept session.
func TestImageTurnArgvAndStdin(t *testing.T) {
	is := newImageServer(t, "cat <<'EOF'\n"+imageTranscript+"\nEOF")
	shot := img("image/png", "shot.png", pngOf(64))

	ask := map[string]any{"prompt": "what is this? [image 1: shot.png]", "system": "sys", "schema": map[string]any{"type": "object"}, "images": []any{shot}}
	if resp, out := postImageJSON(t, is.url("json"), ask); resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, out)
	}
	argv := strings.Split(strings.TrimSuffix(is.read("argv"), "\n"), "\n")
	textAsk, _ := assistRequest{Prompt: "what is this? [image 1: shot.png]", System: "sys", Schema: json.RawMessage(`{"type":"object"}`)}.headless()
	if want := append([]string{"-p", "--input-format", "stream-json"}, textAsk.Args()[2:]...); !slices.Equal(argv, want) {
		t.Fatalf("Ask argv:\n got %q\nwant %q", argv, want)
	}
	if !slices.Contains(argv, "--no-session-persistence") || !slices.Contains(argv, "--json-schema") {
		t.Fatalf("an unkept Ask turn keeps its flags: %q", argv)
	}
	wantStdin := `{"message":{"content":[{"text":"Image 1: shot.png","type":"text"},` +
		`{"source":{"data":"` + shot["data"].(string) + `","media_type":"image/png","type":"base64"},"type":"image"},` +
		`{"text":"what is this? [image 1: shot.png]","type":"text"}],"role":"user"},"type":"user"}` + "\n"
	if got := is.read("stdin"); got != wantStdin {
		t.Fatalf("Ask stdin:\n got %s\nwant %s", got, wantStdin)
	}

	as := map[string]any{"prompt": "and now? [image 1: shot.png]", "system": "sys", "session_id": "sess-1", "keep": true, "bd_reads": true, "images": []any{shot}}
	resp, out := postImageJSON(t, is.url("stream"), as)
	if resp.StatusCode != 200 || !strings.Contains(string(out), "event: done") {
		t.Fatalf("status %d: %s", resp.StatusCode, out)
	}
	argv = strings.Split(strings.TrimSuffix(is.read("argv"), "\n"), "\n")
	textAs, _ := assistRequest{Prompt: "and now? [image 1: shot.png]", System: "sys", SessionID: "sess-1", Keep: true, BdReads: true}.headless()
	if want := append([]string{"-p", "--input-format", "stream-json"}, textAs.Args()[2:]...); !slices.Equal(argv, want) {
		t.Fatalf("assistant argv:\n got %q\nwant %q", argv, want)
	}
	if i := slices.Index(argv, "--resume"); i < 0 || argv[i+1] != "sess-1" || slices.Contains(argv, "--no-session-persistence") {
		t.Fatalf("a kept, resumed turn stays kept: %q", argv)
	}
	if stdin := is.read("stdin"); strings.Count(stdin, "and now?") != 1 || strings.Contains(strings.Join(argv, " "), "and now?") {
		t.Fatalf("prompt must appear once, on stdin: %q / %q", argv, stdin)
	}
	is.waitClean(t)
}

func TestTextTurnUnchanged(t *testing.T) {
	is := newImageServer(t, "cat <<'EOF'\n"+imageTranscript+"\nEOF")
	req := map[string]any{"prompt": "just text", "system": "sys", "schema": map[string]any{"type": "object"}}
	if resp, out := postImageJSON(t, is.url("json"), req); resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, out)
	}
	h, _ := assistRequest{Prompt: "just text", System: "sys", Schema: json.RawMessage(`{"type":"object"}`)}.headless()
	if got := is.read("argv"); got != strings.Join(h.Args(), "\n")+"\n" || !strings.HasPrefix(got, "-p\njust text\n") {
		t.Fatalf("argv = %q", got)
	}
	if is.read("stdin") != "" || is.read("staged") != "" {
		t.Fatalf("text turn stdin %q staged %q", is.read("stdin"), is.read("staged"))
	}
}

func TestImageTurnCleanupThroughTheAPI(t *testing.T) {
	shot := []any{img("image/png", "shot.png", pngOf(64))}
	body := map[string]any{"prompt": "look", "images": shot}
	t.Run("error", func(t *testing.T) {
		is := newImageServer(t, "exit 2")
		if resp, out := postImageJSON(t, is.url("json"), body); resp.StatusCode != http.StatusBadGateway {
			t.Fatalf("status %d: %s", resp.StatusCode, out)
		}
		if !strings.HasPrefix(is.read("staged"), assist.StagePrefix) {
			t.Fatal("images were not staged")
		}
		is.waitClean(t)
	})
	t.Run("timeout", func(t *testing.T) {
		is := newImageServer(t, "sleep 30")
		is.runner.Timeout = 300 * time.Millisecond
		if resp, out := postImageJSON(t, is.url("json"), body); resp.StatusCode != http.StatusBadGateway || !strings.Contains(string(out), "too long") {
			t.Fatalf("status %d: %s", resp.StatusCode, out)
		}
		is.waitClean(t)
	})
	t.Run("client disconnect", func(t *testing.T) {
		is := newImageServer(t, "sleep 30")
		ctx, cancel := context.WithCancel(context.Background())
		b, _ := json.Marshal(body)
		req, _ := http.NewRequestWithContext(ctx, "POST", is.url("json"), bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer tok")
		done := make(chan struct{})
		go func() {
			defer close(done)
			if resp, err := http.DefaultClient.Do(req); err == nil {
				resp.Body.Close()
			}
		}()
		waitUntil(t, func() bool { return strings.HasPrefix(is.read("staged"), assist.StagePrefix) })
		cancel()
		<-done
		is.waitClean(t)
	})
	t.Run("cancel", func(t *testing.T) {
		is := newImageServer(t, "sleep 30")
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", is.url("stream"), bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer tok")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		turnID := readTurnID(t, resp.Body)
		waitUntil(t, func() bool { return strings.HasPrefix(is.read("staged"), assist.StagePrefix) })
		if r, _ := postRaw(t, is.ts.URL+"/v1/assist/"+turnID+"/cancel", nil); r.StatusCode != http.StatusNoContent {
			t.Fatalf("cancel status %d", r.StatusCode)
		}
		is.waitClean(t)
	})
}

// readTurnID reads the first "turn" message of an assist stream.
func readTurnID(t *testing.T, body io.Reader) string {
	t.Helper()
	buf := make([]byte, 0, 1024)
	chunk := make([]byte, 256)
	for !strings.Contains(string(buf), "\n\n") {
		n, err := body.Read(chunk)
		buf = append(buf, chunk[:n]...)
		if err != nil {
			break
		}
	}
	for _, line := range strings.Split(string(buf), "\n") {
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			var tt assistTurn
			if json.Unmarshal([]byte(data), &tt) == nil && tt.TurnID != "" {
				return tt.TurnID
			}
		}
	}
	t.Fatalf("no turn message in %q", buf)
	return ""
}

func waitUntil(t *testing.T, ok func() bool) {
	t.Helper()
	for i := 0; i < 500; i++ {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting")
}

func TestFeaturesListChatImages(t *testing.T) {
	is := newImageServer(t, "true")
	_, body := do(t, "GET", is.ts.URL+"/v1/health", "tok")
	feats, _ := body["features"].([]any)
	if !slices.Contains(feats, any("chat_images")) {
		t.Fatalf("features = %v", feats)
	}
}
