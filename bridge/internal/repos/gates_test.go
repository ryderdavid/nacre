package repos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/bd"
)

// A repo's entry says whether it has usable gates, so the app can offer rounds
// only where they can run.
func TestRepoEntryReportsGates(t *testing.T) {
	good, bad, none := t.TempDir(), t.TempDir(), t.TempDir()
	write := func(dir, s string) {
		if err := os.WriteFile(filepath.Join(dir, "nacre.yaml"), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(good, "gates:\n  - name: test\n    run: go test ./...\n  - name: vet\n    run: go vet ./...\n")
	write(bad, "gates:\n  - name: test\n")
	r := New(func() []string { return []string{good, bad, none} }, bd.New("/nonexistent/bd", ""))

	byPath := map[string][]string{}
	errs := map[string]string{}
	for _, rp := range r.List(true) {
		byPath[rp.Path], errs[rp.Path] = rp.Gates, rp.GatesError
	}
	if got := strings.Join(byPath[good], ","); got != "test,vet" || errs[good] != "" {
		t.Errorf("good: gates %q err %q", got, errs[good])
	}
	if len(byPath[bad]) != 0 || !strings.Contains(errs[bad], "line 2") || !strings.Contains(errs[bad], "run is required") {
		t.Errorf("bad: gates %v err %q", byPath[bad], errs[bad])
	}
	if len(byPath[none]) != 0 || errs[none] != "" {
		t.Errorf("none: gates %v err %q", byPath[none], errs[none])
	}
}
