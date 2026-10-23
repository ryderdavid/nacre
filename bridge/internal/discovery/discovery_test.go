package discovery

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func mkRepo(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(d, ".beads"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestScan(t *testing.T) {
	home := t.TempDir()
	code := filepath.Join(home, "Code")
	docs := filepath.Join(home, "Documents")
	mkRepo(t,
		home, // home-level .beads: excluded by default
		filepath.Join(code, "GanttGen"),
		filepath.Join(code, "HomeAgentic", "gemini-voice-mcp"),
		filepath.Join(code, "web", "node_modules", "pkg"),           // excluded by name
		filepath.Join(code, "Nacre", ".claude", "worktrees", "wt1"), // hidden dir
		filepath.Join(code, "a", "b", "c", "d", "tooDeep"),          // depth 5
		filepath.Join(code, "a", "b", "c", "okDepth4"),              // depth 4
		filepath.Join(docs, "Projects", "Harness"),
		filepath.Join(docs, "Projects", "Secret"),
		filepath.Join(docs, "Skip", "Me"),
		filepath.Join(home, "Elsewhere", "Extra"),
		filepath.Join(home, ".traycer", "x"),
	)
	// A symlinked repo must not be followed.
	if err := os.Symlink(filepath.Join(code, "GanttGen"), filepath.Join(code, "link")); err != nil {
		t.Fatal(err)
	}

	got := Scan(Options{
		Roots:   []string{code, docs, home + "/does-not-exist"},
		Depth:   4,
		Home:    home,
		Exclude: []string{"node_modules", "~/Documents/Skip", "~/.traycer"},
		Extra:   []string{filepath.Join(home, "Elsewhere", "Extra"), filepath.Join(home, "NotARepo")},
		Hidden:  []string{filepath.Join(docs, "Projects", "Secret")},
	})
	want := []string{
		filepath.Join(code, "GanttGen"),
		filepath.Join(code, "HomeAgentic", "gemini-voice-mcp"),
		filepath.Join(code, "a", "b", "c", "okDepth4"),
		filepath.Join(docs, "Projects", "Harness"),
		filepath.Join(home, "Elsewhere", "Extra"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Scan:\n got  %v\n want %v", got, want)
	}
}

func TestScanHomeRoot(t *testing.T) {
	// Even when the home dir itself is a scan root, its .beads is not a repo,
	// but repos below it are found and hidden dirs are skipped.
	home := t.TempDir()
	mkRepo(t, home, filepath.Join(home, "Beads"), filepath.Join(home, ".hidden", "r"))
	got := Scan(Options{Roots: []string{home}, Depth: 2, Home: home})
	want := []string{filepath.Join(home, "Beads")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestMatcher(t *testing.T) {
	m := newMatcher("/Users/x", []string{"build", "~/.traycer", "/abs/path", " "})
	cases := []struct {
		path, name string
		skip       bool
	}{
		{"/Users/x/Code/app/build", "build", true},
		{"/Users/x/Code/app/builder", "builder", false},
		{"/Users/x/.traycer", ".traycer", true},
		{"/Users/x/.traycer/sub", "sub", true},
		{"/Users/x/.traycerx", ".traycerx", false},
		{"/abs/path/deeper", "deeper", true},
	}
	for _, c := range cases {
		if got := m.skip(c.path, c.name); got != c.skip {
			t.Errorf("skip(%q)=%v want %v", c.path, got, c.skip)
		}
	}
}

func TestRepoID(t *testing.T) {
	id := RepoID("/Users/ryder/Code/GanttGen")
	if len(id) != 12 {
		t.Fatalf("len %d", len(id))
	}
	if id != RepoID("/Users/ryder/Code/GanttGen") || id == RepoID("/Users/ryder/Code/Nacre") {
		t.Fatal("RepoID not stable/unique")
	}
}
