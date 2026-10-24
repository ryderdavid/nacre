package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func gatesRepo(t *testing.T, yaml string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nacre.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const gatesYAML = `gates:
  - name: unit
    run: echo unit ok
  - name: slow lint
    run: echo linted
    final_only: true
build:
  - name: package
    run: echo built > out.txt
    artifact: out.txt
env:
  GREETING: hi
`

func TestGatesCommandPasses(t *testing.T) {
	dir := gatesRepo(t, gatesYAML)
	var out bytes.Buffer
	if err := runGates([]string{dir}, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "▶ unit") || !strings.Contains(s, "▶ slow lint") || strings.Contains(s, "▶ package") || !strings.Contains(s, "All 2 passed") {
		t.Errorf("output:\n%s", s)
	}
}

func TestGatesCommandFlagsAfterDirAndBuild(t *testing.T) {
	dir := gatesRepo(t, gatesYAML)
	sub := filepath.Join(dir, "deep", "inside")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	// From a subdirectory, with flags after the directory.
	if err := runGates([]string{sub, "--build", "--skip-final"}, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	s := out.String()
	if strings.Contains(s, "slow lint") || !strings.Contains(s, "▶ package") {
		t.Errorf("output:\n%s", s)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "out.txt")); strings.TrimSpace(string(b)) != "built" {
		t.Errorf("build did not run in the repo root: %q", b)
	}
}

func TestGatesCommandFailsWithTail(t *testing.T) {
	dir := gatesRepo(t, "gates:\n  - name: ok\n    run: \"true\"\n  - name: broken\n    run: echo \"$GREETING, it broke\" >&2; exit 4\nenv:\n  GREETING: hey\n")
	var out bytes.Buffer
	err := runGates([]string{dir}, &out)
	if !errors.Is(err, errGatesFailed) {
		t.Fatalf("err = %v", err)
	}
	if s := out.String(); !strings.Contains(s, `gate "broken" failed with exit status 4`) || !strings.Contains(s, "hey, it broke") {
		t.Errorf("output:\n%s", s)
	}
}

func TestGatesCommandOnlyAndJSON(t *testing.T) {
	dir := gatesRepo(t, gatesYAML)
	var out bytes.Buffer
	if err := runGates([]string{"--only", "package", "--json", dir}, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	var res []struct {
		Name string `json:"name"`
		OK   bool   `json:"ok"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if len(res) != 1 || res[0].Name != "package" || !res[0].OK {
		t.Errorf("results = %+v", res)
	}
	if err := runGates([]string{"--only", "nope", dir}, &out); err == nil || !strings.Contains(err.Error(), `no gate or build step named "nope"`) {
		t.Errorf("unknown --only: %v", err)
	}
}

func TestGatesCommandWithoutFile(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".git"), 0o755)
	var out bytes.Buffer
	if err := runGates([]string{dir}, &out); err == nil || !strings.Contains(err.Error(), "has no nacre.yaml") {
		t.Errorf("err = %v", err)
	}
}
