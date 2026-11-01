package repoconfig

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const full = `gates:
  - name: bridge
    run: cd bridge && go test ./...
  - name: app tests
    run: cd android && ./gradlew -q testDebugUnitTest
  - name: lint
    run: ./lint.sh
    final_only: true
build:
  - name: release apk
    run: cd android && ./gradlew -q assembleRelease
    artifact: android/app/build/outputs/apk/release/app-release.apk
env:
  JAVA_HOME: /Applications/Android Studio.app/Contents/jbr/Contents/Home
version:
  bump: scripts/bump-version.sh {level}
resources:
  - emulator
  - name: fold
    id: emulator-5554
land:
  target: main
  push: false
limits:
  gate_timeout_minutes: 30
  fix_attempts: 3
`

func TestParseFull(t *testing.T) {
	c, err := Parse([]byte(full))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(c.GateNames(), ","); got != "bridge,app tests,lint" {
		t.Errorf("gates = %s", got)
	}
	if !c.Gates[2].FinalOnly || c.Gates[0].FinalOnly {
		t.Errorf("final_only = %v", c.Gates)
	}
	if c.Gates[1].Line != 4 {
		t.Errorf("app tests line = %d", c.Gates[1].Line)
	}
	if len(c.Build) != 1 || c.Build[0].Artifact != "android/app/build/outputs/apk/release/app-release.apk" {
		t.Errorf("build = %+v", c.Build)
	}
	if c.Env["JAVA_HOME"] == "" {
		t.Error("env lost")
	}
	if c.BumpCommand(true) != "scripts/bump-version.sh minor" || c.BumpCommand(false) != "scripts/bump-version.sh patch" {
		t.Errorf("bump = %q / %q", c.BumpCommand(true), c.BumpCommand(false))
	}
	for bumped, want := range map[string]string{"0.5.1 (11) -> 0.6.0 (12)": "v0.6.0", "0.6.0": "v0.6.0", "0.6.0 -> ": "", "oops": "", "": "", "1.2.3 -> 1.2.4~x": ""} {
		if got := c.TagName(bumped); got != want {
			t.Errorf("TagName(%q) = %q, want %q", bumped, got, want)
		}
	}
	if len(c.Resources) != 2 || c.Resources[0].Name != "emulator" || c.Resources[1].ID != "emulator-5554" {
		t.Errorf("resources = %+v", c.Resources)
	}
	if c.Land != (Land{Target: "main", Push: false}) {
		t.Errorf("land = %+v", c.Land)
	}
	if c.Limits != (Limits{GateTimeoutMinutes: 30, FixAttempts: 3}) {
		t.Errorf("limits = %+v", c.Limits)
	}
}

func TestDefaults(t *testing.T) {
	c, err := Parse([]byte("gates:\n  - name: test\n    run: make test\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Land != (Land{Target: "main", Push: true}) {
		t.Errorf("land = %+v", c.Land)
	}
	if c.Limits != (Limits{GateTimeoutMinutes: 20, FixAttempts: 2}) {
		t.Errorf("limits = %+v", c.Limits)
	}
	if c.Version.Bump != "" || c.BumpCommand(true) != "" || c.TagName("0.1.0 -> 0.2.0") != "" {
		t.Errorf("bump = %q", c.Version.Bump)
	}
}

func TestTag(t *testing.T) {
	for y, want := range map[string]string{
		"version:\n  bump: b\n":                       "v{version}",
		"version:\n  bump: b\n  tag: none\n":          "",
		"version:\n  bump: b\n  tag: rel-{version}\n": "rel-{version}",
		"": "",
	} {
		c, err := Parse([]byte("gates:\n  - name: t\n    run: a\n" + y))
		if err != nil {
			t.Fatal(err)
		}
		if c.Version.Tag != want {
			t.Errorf("%q: tag = %q, want %q", y, c.Version.Tag, want)
		}
	}
}

func TestErrorsNameTheProblemAndLine(t *testing.T) {
	cases := map[string]struct {
		yaml string
		line int
		msg  string
	}{
		"missing run":        {"gates:\n  - name: test\n    run: make\n  - name: app\n", 4, `gate 2 ("app"): run is required`},
		"missing name":       {"gates:\n  - run: make\n", 2, "gate 1: name is required"},
		"no gates":           {"build:\n  - name: b\n    run: make\n", 0, "at least one gate is required"},
		"unknown step field": {"gates:\n  - name: t\n    run: make\n    timeout: 5\n", 4, `gate 1 ("t"): unknown field "timeout"`},
		"unknown top field":  {"gates:\n  - name: t\n    run: make\nrunners: 3\n", 4, "field runners not found"},
		"duplicate name":     {"gates:\n  - name: t\n    run: a\nbuild:\n  - name: t\n    run: b\n", 5, "already used on line 2"},
		"artifact on gate":   {"gates:\n  - name: t\n    run: a\n    artifact: x.apk\n", 2, "artifact belongs on a build step"},
		"artifact outside":   {"gates:\n  - name: t\n    run: a\nbuild:\n  - name: b\n    run: b\n    artifact: ../x.apk\n", 5, "inside the repository"},
		"bad env":            {"gates:\n  - name: t\n    run: a\nenv:\n  BAD-NAME: x\n", 0, "not a valid environment variable"},
		"bad resource":       {"gates:\n  - name: t\n    run: a\nresources:\n  - Emulator\n", 5, "lowercase"},
		"bad timeout":        {"gates:\n  - name: t\n    run: a\nlimits:\n  gate_timeout_minutes: 0\n", 0, "more than 0"},
		"bad target":         {"gates:\n  - name: t\n    run: a\nland:\n  target: --force\n", 0, "not a valid branch"},
		"not yaml":           {"gates: [\n", 0, ""},
		"tag without bump":   {"gates:\n  - name: t\n    run: a\nversion:\n  tag: v{version}\n", 0, "tag needs a bump"},
		"tag no placeholder": {"gates:\n  - name: t\n    run: a\nversion:\n  bump: b\n  tag: release\n", 0, "must contain {version}"},
		"regenerate no run":  {"gates:\n  - name: t\n    run: a\nmerge:\n  regenerate:\n    - path: go.sum\n", 6, "run is required"},
		"regenerate outside": {"gates:\n  - name: t\n    run: a\nmerge:\n  regenerate:\n    - path: ../x\n      run: make\n", 6, "inside the repository"},
		"regenerate field":   {"gates:\n  - name: t\n    run: a\nmerge:\n  regenerate:\n    - path: go.sum\n      run: make\n      when: always\n", 8, `unknown field "when"`},
	}
	for name, tc := range cases {
		_, err := Parse([]byte(tc.yaml))
		var e *Error
		if !errors.As(err, &e) {
			t.Errorf("%s: err = %v, want *Error", name, err)
			continue
		}
		if tc.line != 0 && e.Line != tc.line {
			t.Errorf("%s: line %d, want %d (%v)", name, e.Line, tc.line, err)
		}
		if !strings.Contains(err.Error(), tc.msg) {
			t.Errorf("%s: %q does not mention %q", name, err, tc.msg)
		}
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(dir); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(full), 0o644); err != nil {
		t.Fatal(err)
	}
	if c, err := Load(dir); err != nil || len(c.Gates) != 3 {
		t.Fatalf("load: %v %+v", err, c)
	}
}

func TestMergeRegenerate(t *testing.T) {
	c, err := Parse([]byte("gates:\n  - name: t\n    run: a\nmerge:\n  regenerate:\n    - path: android/*.lockfile\n      run: cd android && ./gradlew -q dependencies --write-locks\n"))
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := c.Merge.Regenerates("android/gradle.lockfile"); !ok || !strings.Contains(r.Run, "--write-locks") {
		t.Errorf("lockfile not matched: %+v %v", r, ok)
	}
	if _, ok := c.Merge.Regenerates("android/app/gradle.lockfile"); ok {
		t.Error("* crossed a directory")
	}
}
