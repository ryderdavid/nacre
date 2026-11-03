package assist

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

var pngImage = claude.Image{MediaType: "image/png", Name: "shot.png", Data: []byte("\x89PNG\r\n\x1a\nimage-bytes")}

// imageClaude writes a fake claude that saves its stdin and argv to out, and
// the staged directories under base it can see, then runs tail (a shell
// snippet that prints a transcript, sleeps or fails).
func imageClaude(t *testing.T, base, tail string) (path, out string) {
	t.Helper()
	dir := t.TempDir()
	path, out = filepath.Join(dir, "claude"), t.TempDir()
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do echo \"$a\"; done > " + out + "/argv\n" +
		"cat > " + out + "/stdin\n" +
		"ls " + base + " > " + out + "/staged\n" +
		tail + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path, out
}

func stagedDirs(t *testing.T, base string) []string {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(base, StagePrefix+"*"))
	return m
}

func TestStageIsPrivate(t *testing.T) {
	base := t.TempDir()
	r := &Runner{TempDir: base}
	dir, staged, err := r.stage([]claude.Image{pngImage, {MediaType: "image/jpeg", Name: "b.jpg", Data: []byte{0xFF, 0xD8, 0xFF, 1}}})
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if !strings.HasPrefix(filepath.Base(dir), StagePrefix) || filepath.Dir(dir) != base {
		t.Fatalf("dir = %s", dir)
	}
	if st, _ := os.Stat(dir); st.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %v", st.Mode().Perm())
	}
	if len(staged) != 2 || staged[0].Name != "shot.png" || staged[1].MediaType != "image/jpeg" {
		t.Fatalf("staged = %+v", staged)
	}
	for _, s := range staged {
		st, err := os.Stat(s.Path)
		if err != nil || st.Mode().Perm() != 0o600 {
			t.Errorf("%s: %v %v", s.Path, st.Mode().Perm(), err)
		}
	}
	if b, _ := os.ReadFile(staged[0].Path); string(b) != string(pngImage.Data) {
		t.Errorf("file = %q", b)
	}
}

func TestImageTurnSendsStdinAndRemovesItsDirectory(t *testing.T) {
	base := t.TempDir()
	path, out := imageClaude(t, base, "cat <<'EOF'\n"+successTranscript+"\nEOF")
	r := &Runner{ClaudePath: path, TempDir: base, Timeout: 20 * time.Second}
	h := claude.Headless{Prompt: "what is this? [image 1: shot.png]", Images: []claude.Image{pngImage}, Persist: true, Resume: "s-1"}
	if _, err := r.Run(context.Background(), t.TempDir(), h, nil); err != nil {
		t.Fatal(err)
	}
	argv, _ := os.ReadFile(filepath.Join(out, "argv"))
	stdin, _ := os.ReadFile(filepath.Join(out, "stdin"))
	staged, _ := os.ReadFile(filepath.Join(out, "staged"))
	if want := strings.Join(h.Args(), "\n") + "\n"; string(argv) != want {
		t.Errorf("argv = %q, want %q", argv, want)
	}
	if strings.Contains(string(argv), "what is this?") || strings.Count(string(stdin), "what is this?") != 1 {
		t.Errorf("the prompt must be sent once, on stdin: argv %q stdin %q", argv, stdin)
	}
	if !strings.Contains(string(stdin), `"Image 1: shot.png"`) || !strings.Contains(string(stdin), `"media_type":"image/png"`) {
		t.Errorf("stdin = %s", stdin)
	}
	if !strings.HasPrefix(string(staged), StagePrefix) {
		t.Errorf("the images were not staged during the turn: %q", staged)
	}
	if d := stagedDirs(t, base); len(d) != 0 {
		t.Fatalf("staged directory left after success: %v", d)
	}
}

func TestTextTurnHasEmptyStdinAndStagesNothing(t *testing.T) {
	base := t.TempDir()
	path, out := imageClaude(t, base, "cat <<'EOF'\n"+successTranscript+"\nEOF")
	r := &Runner{ClaudePath: path, TempDir: base}
	h := claude.Headless{Prompt: "what is this?"}
	if _, err := r.Run(context.Background(), t.TempDir(), h, nil); err != nil {
		t.Fatal(err)
	}
	argv, _ := os.ReadFile(filepath.Join(out, "argv"))
	stdin, _ := os.ReadFile(filepath.Join(out, "stdin"))
	staged, _ := os.ReadFile(filepath.Join(out, "staged"))
	if want := strings.Join(h.Args(), "\n") + "\n"; string(argv) != want || !strings.Contains(string(argv), "what is this?\n") {
		t.Errorf("argv = %q", argv)
	}
	if len(stdin) != 0 || len(staged) != 0 {
		t.Errorf("text turn stdin %q staged %q", stdin, staged)
	}
}

func TestImageDirectoryRemovedOnEveryEnding(t *testing.T) {
	const failed = `{"type":"result","subtype":"error_during_execution","is_error":true,"result":"boom"}`
	cases := []struct {
		name    string
		tail    string
		timeout time.Duration
		cancel  bool
		wantErr string
	}{
		{name: "error", tail: "cat <<'EOF'\n" + failed + "\nEOF", wantErr: "boom"},
		{name: "exit", tail: "exit 3", wantErr: "status 3"},
		{name: "timeout", tail: "sleep 30", timeout: 300 * time.Millisecond, wantErr: "too long"},
		{name: "cancel", tail: "sleep 30", cancel: true, wantErr: "canceled"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			base := t.TempDir()
			path, out := imageClaude(t, base, c.tail)
			r := &Runner{ClaudePath: path, TempDir: base, Timeout: c.timeout}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if c.cancel {
				go func() {
					// Cancel once claude is running with the images staged.
					for i := 0; i < 200; i++ {
						if b, _ := os.ReadFile(filepath.Join(out, "staged")); len(b) > 0 {
							break
						}
						time.Sleep(20 * time.Millisecond)
					}
					cancel()
				}()
			}
			_, err := r.Run(ctx, t.TempDir(), claude.Headless{Prompt: "x", Images: []claude.Image{pngImage}}, nil)
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("err = %v, want %q", err, c.wantErr)
			}
			if b, _ := os.ReadFile(filepath.Join(out, "staged")); !strings.HasPrefix(string(b), StagePrefix) {
				t.Fatalf("not staged during the turn: %q", b)
			}
			if d := stagedDirs(t, base); len(d) != 0 {
				t.Fatalf("staged directory left after %s: %v", c.name, d)
			}
		})
	}
}

func TestDetachedImageTurnCleansUpWhenCancelled(t *testing.T) {
	base := t.TempDir()
	path, out := imageClaude(t, base, "sleep 30")
	r := &Runner{ClaudePath: path, TempDir: base}
	turn := r.Start(t.TempDir(), claude.Headless{Prompt: "x", Images: []claude.Image{pngImage}})
	waitFor(t, func() bool { b, _ := os.ReadFile(filepath.Join(out, "staged")); return len(b) > 0 })
	turn.Cancel()
	waitFor(t, func() bool { return turn.State(0).Done })
	if d := stagedDirs(t, base); len(d) != 0 {
		t.Fatalf("staged directory left after cancel: %v", d)
	}
}

func TestSweepStagedRemovesLeftovers(t *testing.T) {
	base := t.TempDir()
	for _, d := range []string{StagePrefix + "123", StagePrefix + "456/inner"} {
		if err := os.MkdirAll(filepath.Join(base, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(base, StagePrefix+"123", "1.png"), []byte("x"), 0o600)
	os.Mkdir(filepath.Join(base, "someone-else"), 0o700)
	os.WriteFile(filepath.Join(base, StagePrefix+"file"), []byte("not ours to judge"), 0o600)

	n, err := (&Runner{TempDir: base}).SweepStaged()
	if err != nil || n != 2 {
		t.Fatalf("swept %d, %v", n, err)
	}
	if d := stagedDirs(t, base); len(d) != 1 || filepath.Base(d[0]) != StagePrefix+"file" {
		t.Fatalf("left %v", d)
	}
	if _, err := os.Stat(filepath.Join(base, "someone-else")); err != nil {
		t.Fatal("swept a directory that isn't nacred's")
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for i := 0; i < 500; i++ {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting")
}
