package chat

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

func pngImage(name string, n int) claude.Image {
	b := make([]byte, n)
	copy(b, "\x89PNG\r\n\x1a\n")
	b[n-1] = byte(len(name)) // tell images apart
	return claude.Image{MediaType: "image/png", Name: name, Data: b}
}

func jpegImage(name string, n int) claude.Image {
	b := make([]byte, n)
	copy(b, []byte{0xFF, 0xD8, 0xFF, 0xE0})
	return claude.Image{MediaType: "image/jpeg", Name: name, Data: b}
}

// block is one content block of a stream-json user message.
type block struct {
	Type   string `json:"type"`
	Text   string `json:"text"`
	Source struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
	} `json:"source"`
}

func userBlocks(t *testing.T, line string) []block {
	t.Helper()
	var m struct {
		Type    string
		Message struct {
			Role    string
			Content []block
		}
	}
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("stdin %.200q: %v", line, err)
	}
	if m.Type != "user" || m.Message.Role != "user" {
		t.Fatalf("not a user message: %.200s", line)
	}
	return m.Message.Content
}

func userEvent(t *testing.T, m *Manager, id, turnID string) Event {
	t.Helper()
	for _, ev := range allEvents(t, m, id) {
		if ev.Kind == KindUser && ev.TurnID == turnID {
			return ev
		}
	}
	t.Fatalf("no user event for %s", turnID)
	return Event{}
}

func TestImagesAreStoredAndSentBeforeTheText(t *testing.T) {
	bin, fdir := fakeClaude(t, "0.5", "")
	_, _, opt := repoSetup(t)
	dir := t.TempDir()
	m := newManager(t, dir, bin, opt)
	rec, err := m.Create(CreateRequest{Agent: "claude", Repo: strp("r-a")})
	if err != nil {
		t.Fatal(err)
	}
	a, b := pngImage("shot.png", 300), jpegImage("photo.jpg", 200)
	ctx := &Context{Screen: "bead", Repo: "r-a", BeadID: "NAC-3"}
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_img00001", Text: "what's wrong here?", Context: ctx, Images: []claude.Image{a, b}}); err != nil {
		t.Fatal(err)
	}
	got := waitIdle(t, m, rec.ID)
	if got.ImageCount != 2 || got.ImageBytes != 500 {
		t.Errorf("record counts %d images, %d bytes", got.ImageCount, got.ImageBytes)
	}

	// The note, then each image's label and block in order, then the text.
	blocks := userBlocks(t, readLines(t, filepath.Join(fdir, "stdin0"))[0])
	if len(blocks) != 6 {
		t.Fatalf("%d blocks: %+v", len(blocks), blocks)
	}
	if blocks[0].Type != "text" || !strings.HasPrefix(blocks[0].Text, "On screen:") {
		t.Errorf("first block %+v, want the note", blocks[0])
	}
	if blocks[1].Text != "Image 1: shot.png" || blocks[3].Text != "Image 2: photo.jpg" {
		t.Errorf("labels %q, %q", blocks[1].Text, blocks[3].Text)
	}
	for i, want := range []claude.Image{a, b} {
		bl := blocks[2+2*i]
		data, _ := base64.StdEncoding.DecodeString(bl.Source.Data)
		if bl.Type != "image" || bl.Source.Type != "base64" || bl.Source.MediaType != want.MediaType || !bytes.Equal(data, want.Data) {
			t.Errorf("image block %d: %s %s %s, %d bytes", i+1, bl.Type, bl.Source.Type, bl.Source.MediaType, len(data))
		}
	}
	if blocks[5].Type != "text" || blocks[5].Text != "what's wrong here?" {
		t.Errorf("last block %+v, want the text", blocks[5])
	}

	// Stored under chats/<id>/images, directory 0700 and files 0600, and
	// named by the user event.
	ev := userEvent(t, m, rec.ID, "c_img00001")
	if len(ev.Images) != 2 || ev.Images[0].Name != "shot.png" || ev.Images[1].MediaType != "image/jpeg" || ev.Images[0].Bytes != 300 {
		t.Fatalf("user event images %+v", ev.Images)
	}
	imgDir := filepath.Join(dir, rec.ID, "images")
	for _, d := range []string{filepath.Join(dir, rec.ID), imgDir} {
		st, err := os.Stat(d)
		if err != nil || st.Mode().Perm() != 0o700 {
			t.Errorf("%s: %v %v, want 0700", d, err, st)
		}
	}
	for i, ref := range ev.Images {
		if !imageIDRe.MatchString(ref.ID) {
			t.Errorf("image id %q", ref.ID)
		}
		p := filepath.Join(imgDir, ref.ID)
		st, err := os.Stat(p)
		if err != nil || st.Mode().Perm() != 0o600 {
			t.Errorf("%s: %v %v, want 0600", p, err, st)
		}
		data, _ := os.ReadFile(p)
		if !bytes.Equal(data, []claude.Image{a, b}[i].Data) {
			t.Errorf("stored image %d differs", i+1)
		}
		path, mt, err := m.Image(rec.ID, ref.ID)
		if err != nil || path != p || mt != ref.MediaType {
			t.Errorf("Image(%s) = %s %s %v", ref.ID, path, mt, err)
		}
	}
	if !strings.HasSuffix(ev.Images[1].ID, ".jpg") || !strings.HasSuffix(ev.Images[0].ID, ".png") {
		t.Errorf("extensions: %+v", ev.Images)
	}

	// A follow-up carries no images; the resumed session holds them.
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_img00002", Text: "and now?", Context: ctx}); err != nil {
		t.Fatal(err)
	}
	got = waitIdle(t, m, rec.ID)
	argv := readLines(t, filepath.Join(fdir, "argv1"))
	if v, _ := flag(argv, "--resume"); v != rec.SessionID {
		t.Errorf("follow-up didn't resume: %q", argv)
	}
	for _, bl := range userBlocks(t, readLines(t, filepath.Join(fdir, "stdin1"))[0]) {
		if bl.Type != "text" || strings.HasPrefix(bl.Text, "Image ") {
			t.Errorf("follow-up block %+v", bl)
		}
	}
	if got.ImageCount != 2 || len(userEvent(t, m, rec.ID, "c_img00002").Images) != 0 {
		t.Errorf("follow-up changed the images: %d", got.ImageCount)
	}
	if entries, _ := os.ReadDir(imgDir); len(entries) != 2 {
		t.Errorf("%d stored images after the follow-up", len(entries))
	}

	// Unknown, malformed or traversing names are not served.
	for _, bad := range []string{"im_000000000000000000000000.png", "../" + rec.ID + ".json", ev.Images[0].ID + "x", ""} {
		if _, _, err := m.Image(rec.ID, bad); !errors.Is(err, ErrNoImage) {
			t.Errorf("Image(%q) = %v", bad, err)
		}
	}
	if _, _, err := m.Image("ch_000000000000000000000000", ev.Images[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown chat: %v", err)
	}

	// Deleting the chat deletes its images.
	if err := m.Delete(rec.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, rec.ID)); !os.IsNotExist(err) {
		t.Errorf("chat directory left after delete: %v", err)
	}
}

func TestImagesOnlyMessage(t *testing.T) {
	bin, fdir := fakeClaude(t, "0.5", "")
	m := newManager(t, t.TempDir(), bin)
	rec, _ := m.Create(CreateRequest{Agent: "claude"})
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_imgonly1"}); err == nil {
		t.Fatal("an empty message was accepted")
	}
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_imgonly1", Images: []claude.Image{pngImage("a.png", 50)}}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m, rec.ID)
	blocks := userBlocks(t, readLines(t, filepath.Join(fdir, "stdin0"))[0])
	if n := len(blocks); n != 3 || blocks[1].Text != "Image 1: a.png" || blocks[2].Type != "image" {
		t.Fatalf("blocks %+v", blocks)
	}
}

func TestChatImageBudget(t *testing.T) {
	bin, fdir := fakeClaude(t, "0.1", "")
	dir := t.TempDir()
	m := newManager(t, dir, bin)
	rec, _ := m.Create(CreateRequest{Agent: "claude"})
	// 18 images sent over four messages.
	n := 0
	for _, k := range []int{5, 5, 5, 3} {
		var imgs []claude.Image
		for i := 0; i < k; i++ {
			imgs = append(imgs, pngImage("s.png", 100))
		}
		n++
		if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_budget0" + itoa(n), Text: "look", Images: imgs}); err != nil {
			t.Fatal(err)
		}
		waitIdle(t, m, rec.ID)
	}
	before := len(allEvents(t, m, rec.ID))
	three := []claude.Image{pngImage("a.png", 100), pngImage("b.png", 100), pngImage("c.png", 100)}
	_, err := m.Send(rec.ID, SendRequest{TurnID: "c_budget09", Text: "three more", Images: three})
	if !errors.Is(err, ErrImageLimit) || err.Error() != ImageLimitReason {
		t.Fatalf("21 images: %v", err)
	}
	if len(allEvents(t, m, rec.ID)) != before {
		t.Error("a refused message was logged")
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, rec.ID, "images")); len(entries) != 18 {
		t.Errorf("%d images stored, want 18", len(entries))
	}
	if _, err := os.Stat(filepath.Join(fdir, "stdin4")); err == nil {
		t.Error("a turn ran for a refused message")
	}
	// Two more fit; text alone always does.
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_budget10", Text: "two", Images: three[:2]}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m, rec.ID)
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_budget11", Text: "one more", Images: three[:1]}); !errors.Is(err, ErrImageLimit) {
		t.Fatalf("21st image: %v", err)
	}
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_budget12", Text: "text is fine"}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m, rec.ID)

	// The bytes count too: 15 MiB decoded.
	rec2, _ := m.Create(CreateRequest{Agent: "claude"})
	big := []claude.Image{pngImage("1.png", 3<<20), pngImage("2.png", 3<<20), pngImage("3.png", 3<<20), pngImage("4.png", 3<<20), pngImage("5.png", 3<<20)}
	if _, err := m.Send(rec2.ID, SendRequest{TurnID: "c_bytes001", Text: "five", Images: big}); err != nil {
		t.Fatal(err)
	}
	if got := waitIdle(t, m, rec2.ID); got.ImageBytes != 15<<20 || got.ImageCount != 5 {
		t.Fatalf("record %d images %d bytes", got.ImageCount, got.ImageBytes)
	}
	if _, err := m.Send(rec2.ID, SendRequest{TurnID: "c_bytes002", Text: "tiny", Images: []claude.Image{pngImage("t.png", 20)}}); !errors.Is(err, ErrImageLimit) {
		t.Fatalf("over 15 MiB: %v", err)
	}
}

func TestTrimDeletesAChatsImages(t *testing.T) {
	bin, _ := fakeClaude(t, "0.1", "")
	dir := t.TempDir()
	clock := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	m := newManager(t, dir, bin, func(o *Options) {
		o.Now = func() time.Time { clock = clock.Add(time.Second); return clock }
	})
	old, _ := m.Create(CreateRequest{Agent: "claude"})
	if _, err := m.Send(old.ID, SendRequest{TurnID: "c_trim0001", Text: "x", Images: []claude.Image{pngImage("a.png", 50)}}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m, old.ID)
	if _, err := os.Stat(filepath.Join(dir, old.ID, "images")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxChats; i++ {
		if _, err := m.Create(CreateRequest{Agent: "claude"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := m.Get(old.ID, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("oldest chat kept: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, old.ID)); !os.IsNotExist(err) {
		t.Errorf("trimmed chat's directory left: %v", err)
	}
}

func TestRouterSeesOnlyTheImageCount(t *testing.T) {
	m, rec, fdir := routedChat(t, map[string]string{"router": "light", "light": "light_handoff_line", "full": "light_plain"})
	imgs := []claude.Image{pngImage("secret-name.png", 64), pngImage("other.png", 64)}
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_route001", Text: "fix what this shows", Context: &Context{Screen: "boards"}, Images: imgs}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m, rec.ID)
	cs := calls(t, fdir)
	if kinds(cs) != "router,light,full" {
		t.Fatalf("calls %s", kinds(cs))
	}
	prompt := cs[0].argv[indexOf(cs[0].argv, "-p")+1]
	if !strings.Contains(prompt, "Images attached to the new message: 2") {
		t.Errorf("router prompt lacks the count:\n%s", prompt)
	}
	enc := base64.StdEncoding.EncodeToString(imgs[0].Data)
	for _, c := range cs {
		if c.kind == "router" && (strings.Contains(strings.Join(c.argv, " ")+strings.Join(c.stdin, " "), "secret-name") ||
			strings.Contains(strings.Join(c.stdin, " "), enc)) {
			t.Error("the router saw an image or its name")
		}
	}
	// The light turn gets the images; the full turn that resumes the same
	// session after the handoff gets none again.
	light := strings.Join(cs[1].stdin, "\n")
	if !strings.Contains(light, "Image 1: secret-name.png") || !strings.Contains(light, enc) {
		t.Errorf("light turn lacks the images")
	}
	if full := strings.Join(cs[2].stdin, "\n"); strings.Contains(full, `"type":"image"`) {
		t.Errorf("the continuation resent the images")
	}
}

func TestCodexTurnStartCarriesTheStoredImagePaths(t *testing.T) {
	m, rec, fdir, _ := codexChat(t, "")
	imgs := []claude.Image{pngImage("photo.png", 80), jpegImage("b.jpg", 40)}
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_cximg001", Text: "what is this?", Context: &Context{Screen: "boards"}, Images: imgs}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m, rec.ID)
	ev := userEvent(t, m, rec.ID, "c_cximg001")
	if len(ev.Images) != 2 {
		t.Fatalf("user event images %+v", ev.Images)
	}
	input := rpcCalls(t, fdir, 0)["turn/start"]["input"].([]any)
	if len(input) != 4 {
		t.Fatalf("turn/start input = %v", input)
	}
	if text := input[1].(map[string]any)["text"]; text != "what is this?" {
		t.Errorf("text input = %v", text)
	}
	for i, ref := range ev.Images {
		in := input[2+i].(map[string]any)
		path, _, err := m.Image(rec.ID, ref.ID)
		if err != nil {
			t.Fatal(err)
		}
		if in["type"] != "localImage" || in["path"] != path {
			t.Errorf("image input %d = %v, want localImage %s", i+1, in, path)
		}
		data, _ := os.ReadFile(path)
		if !bytes.Equal(data, imgs[i].Data) {
			t.Errorf("stored copy %d differs", i+1)
		}
	}
	// The follow-up sends none again.
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_cximg002", Text: "and?", Context: &Context{Screen: "boards"}}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m, rec.ID)
	for _, in := range rpcCalls(t, fdir, 1)["turn/start"]["input"].([]any) {
		if in.(map[string]any)["type"] != "text" {
			t.Errorf("follow-up input %v", in)
		}
	}
}
