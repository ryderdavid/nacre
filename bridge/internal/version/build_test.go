package version

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the real Makefile in an isolated repository so release tags, later
// commits and edits cannot accidentally collapse to the same installed version.
func TestMakeBuildProvenance(t *testing.T) {
	dir := t.TempDir()
	write := func(name, contents string) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for dest, source := range map[string]string{"Makefile": "../../Makefile", "internal/version/version.go": "version.go"} {
		b, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		write(dest, string(b))
	}
	write("go.mod", "module github.com/ryderdavid/nacre/bridge\n\ngo 1.25.0\n")
	write(".gitignore", "/bin/\n")
	write("cmd/nacred/main.go", `package main
import (
 "encoding/json"
 "os"
 "github.com/ryderdavid/nacre/bridge/internal/version"
)
func main() { json.NewEncoder(os.Stdout).Encode(map[string]string{"version": version.Version, "commit": version.Commit}) }
`)
	run := func(name string, args ...string) string {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off")
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, b)
		}
		return strings.TrimSpace(string(b))
	}
	run("git", "init", "-q")
	run("git", "config", "user.name", "Test")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "commit.gpgsign", "false")
	run("git", "add", ".")
	run("git", "commit", "-qm", "initial")
	check := func(wantVersion, wantCommit string) {
		t.Helper()
		run("make", "build")
		var got map[string]string
		if err := json.Unmarshal([]byte(run(filepath.Join(dir, "bin/nacred"), "version", "--json")), &got); err != nil {
			t.Fatal(err)
		}
		if got["version"] != wantVersion || got["commit"] != wantCommit {
			t.Fatalf("build = %v, want version %s, commit %s", got, wantVersion, wantCommit)
		}
	}
	commit := run("git", "rev-parse", "HEAD")
	check("dev-"+run("git", "describe", "--always"), commit)
	run("git", "tag", "v1.2.3")
	check("1.2.3", commit)
	write("change.txt", "next release\n")
	run("git", "add", ".")
	run("git", "commit", "-qm", "next")
	commit = run("git", "rev-parse", "HEAD")
	described := strings.TrimPrefix(run("git", "describe", "--tags"), "v")
	check(described, commit)
	write("change.txt", "uncommitted\n")
	check(described+"-dirty", commit)
	run("git", "add", ".")
	check(described+"-dirty", commit)

	// Source archives have no VCS stamp. They must say dev unless their
	// release builder explicitly supplies the resolved tag and commit.
	if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
		t.Fatal(err)
	}
	check("dev", "")
	run("make", "build", "VERSION=1.2.4", "COMMIT="+commit)
	var got map[string]string
	if err := json.Unmarshal([]byte(run(filepath.Join(dir, "bin/nacred"))), &got); err != nil {
		t.Fatal(err)
	}
	if got["version"] != "1.2.4" || got["commit"] != commit {
		t.Fatalf("archive build = %v", got)
	}
}
