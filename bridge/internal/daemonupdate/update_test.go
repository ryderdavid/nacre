package daemonupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const sha = "1234567890123456789012345678901234567890"

func pending(t *testing.T) (string, Status) {
	t.Helper()
	dir := t.TempDir()
	s := Status{ID: "test", HostID: "host", Phase: "checking", Active: true, CurrentVersion: "0.8.0", StartedAt: time.Now()}
	if e := save(dir, &s); e != nil {
		t.Fatal(e)
	}
	return dir, s
}

func TestWorkerOutcomes(t *testing.T) {
	for _, scenario := range []struct {
		name, fail, phase    string
		installed, recovered bool
	}{
		{"success", "", "updated", true, false}, {"no release", "latest", "failed", false, false}, {"build failure", "build", "failed", false, false},
		{"backup failure", "backup", "failed", false, false}, {"install failure", "install", "failed", false, false},
		{"health failure", "health", "rolled_back", true, true}, {"restart failure", "restart", "rolled_back", true, true},
		{"rollback failure", "restore", "recovery_failed", true, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			dir, s := pending(t)
			installed, recovered := false, false
			restarts := 0
			healthCalls := 0
			broken := errors.New("fixture failure")
			e := Environment{
				Latest: func(context.Context) (Release, error) {
					if scenario.fail == "latest" {
						return Release{}, broken
					}
					return Release{"v0.8.1", sha}, nil
				},
				Build: func(context.Context, Release) (string, error) {
					if scenario.fail == "build" {
						return "", broken
					}
					return "candidate", nil
				},
				Backup: func() error {
					if scenario.fail == "backup" {
						return broken
					}
					return nil
				},
				Install: func(string) error {
					if scenario.fail == "install" {
						return broken
					}
					installed = true
					return nil
				},
				Restart: func(context.Context) error {
					restarts++
					if scenario.fail == "restart" && restarts == 1 {
						return broken
					}
					return nil
				},
				Healthy: func(_ context.Context, v, c, h string) error {
					healthCalls++
					if h != "host" {
						t.Fatal("host identity omitted")
					}
					if healthCalls == 1 && (scenario.fail == "health" || scenario.fail == "restore") {
						return broken
					}
					if recovered && v != "0.8.0" {
						t.Fatal("wrong rollback identity")
					}
					return nil
				},
				Restore: func() error {
					recovered = true
					if scenario.fail == "restore" {
						return broken
					}
					return nil
				},
			}
			_ = Work(context.Background(), dir, s.ID, e)
			got, err := read(dir)
			if err != nil {
				t.Fatal(err)
			}
			if got.Phase != scenario.phase || got.Active || installed != scenario.installed || recovered != scenario.recovered {
				t.Fatalf("status=%+v installed=%v recovered=%v", got, installed, recovered)
			}
		})
	}
}
func TestDuplicateStartAndRecovery(t *testing.T) {
	var launches atomic.Int32
	m := New(Options{Dir: t.TempDir(), Version: "1.0.0", Launch: func(Status) error { launches.Add(1); return nil }})
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := m.Start(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if launches.Load() != 1 {
		t.Fatal("duplicate helper", launches.Load())
	}
	restarted := New(m.opts)
	if !restarted.Get().Active || restarted.Get().ID != m.Get().ID {
		t.Fatal("operation lost on restart")
	}
	s := m.Get()
	s.StartedAt = time.Now().Add(-Window - time.Second)
	if err := save(m.opts.Dir, &s); err != nil {
		t.Fatal(err)
	}
	if m.Get().Phase != "interrupted" {
		t.Fatal(m.Get())
	}
	if _, err := m.Start(); err == nil {
		t.Fatal("interrupted operation silently replaced")
	}
}
func TestUnsupportedAndLaunchFailure(t *testing.T) {
	for _, o := range []Options{{Dir: t.TempDir(), Unavailable: "manual install needed"}, {Dir: t.TempDir(), Launch: func(Status) error { return errors.New("could not launch") }}} {
		m := New(o)
		if _, err := m.Start(); err == nil {
			t.Fatal("start should fail")
		}
		if m.Get().Active {
			t.Fatal("failed launch left maintenance active")
		}
	}
}
func TestWorkerNoDowngradeAndProvenance(t *testing.T) {
	for _, tc := range []struct{ version, commit, want string }{{"0.7.9", sha, "current"}, {"v0.8.0", sha, "current"}, {"main", sha, "failed"}, {"1.0.0", "bad", "failed"}} {
		dir, s := pending(t)
		s.CurrentCommit = sha
		_ = save(dir, &s)
		_ = Work(context.Background(), dir, s.ID, Environment{Latest: func(context.Context) (Release, error) { return Release{tc.version, tc.commit}, nil }, Build: func(context.Context, Release) (string, error) { t.Fatal("unexpected build"); return "", nil }})
		got, _ := read(dir)
		if got.Phase != tc.want {
			t.Fatal(got)
		}
	}
}
func TestCancelledBuildNeverInstalls(t *testing.T) {
	dir, s := pending(t)
	ctx, cancel := context.WithCancel(context.Background())
	_ = Work(ctx, dir, s.ID, Environment{Latest: func(context.Context) (Release, error) { return Release{"1.0.0", sha}, nil }, Build: func(context.Context, Release) (string, error) { cancel(); return "candidate", nil }, Backup: func() error { t.Fatal("cancelled work reached backup"); return nil }})
	got, _ := read(dir)
	if got.Phase != "failed" {
		t.Fatal(got)
	}
}
func archive(t *testing.T, name string, kind byte, size int64) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "root/bridge/go.mod", Mode: 0600, Size: 0, Typeflag: tar.TypeReg})
	_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: size, Typeflag: kind, Linkname: "../../escape"})
	if size > 0 && size < 1024 {
		_, _ = tw.Write(bytes.Repeat([]byte("x"), int(size)))
	}
	_ = tw.Close()
	_ = gz.Close()
	return b.Bytes()
}
func TestArchiveConfinement(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind byte
		size int64
		ok   bool
	}{
		{"root/bridge/main.go", tar.TypeReg, 5, true}, {"root/bridge/../../escape", tar.TypeReg, 0, false},
		{"/absolute", tar.TypeReg, 0, false}, {"root/bridge/link", tar.TypeSymlink, 0, false}, {"root/bridge/hard", tar.TypeLink, 0, false},
		{"root/bridge/big", tar.TypeReg, 17 << 20, false}, {"second/bridge/file", tar.TypeReg, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := extract(bytes.NewReader(archive(t, tc.name, tc.kind, tc.size)), t.TempDir())
			if (e == nil) != tc.ok {
				t.Fatal(e)
			}
		})
	}
}
func TestAtomicBackupPreservesBytes(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "old")
	dst := filepath.Join(dir, "backup")
	_ = os.WriteFile(src, []byte("known good"), 0700)
	if e := copyAtomic(src, dst); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(dst)
	if string(b) != "known good" {
		t.Fatal(string(b))
	}
	if e := copyAtomic("missing", dst); e == nil {
		t.Fatal("missing copy succeeded")
	}
	b, _ = os.ReadFile(dst)
	if string(b) != "known good" {
		t.Fatal("failed copy destroyed backup")
	}
}
func TestLatestUsesFixedOriginAndRejectsPrerelease(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$4\" in\n repos/ryderdavid/nacre/releases/latest) printf '%s' '{\"tag_name\":\"v1.2.3\",\"prerelease\":false}' ;;\n repos/ryderdavid/nacre/commits/v1.2.3) printf '%s' '{\"sha\":\"" + sha + "\"}' ;;\n *) exit 1 ;;\nesac\n"
	// gh arguments are api --hostname github.com <fixed endpoint>.
	_ = os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0700)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	r, e := latest(context.Background())
	if e != nil || r.Commit != sha || r.Version != "v1.2.3" {
		t.Fatalf("%+v %v", r, e)
	}
	_ = os.WriteFile(filepath.Join(dir, "gh"), []byte(strings.Replace(script, `"prerelease":false`, `"prerelease":true`, 1)), 0700)
	if _, e = latest(context.Background()); e == nil {
		t.Fatal("accepted prerelease")
	}
}

func TestIsolatedSourceBuildChecksCandidateProtocol(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "tools")
	_ = os.Mkdir(bin, 0700)
	source := filepath.Join(dir, "archive.gz")
	_ = os.WriteFile(source, archive(t, "root/bridge/main.go", tar.TypeReg, 5), 0600)
	gh := "#!/bin/sh\ncase \"$4\" in repos/ryderdavid/nacre/tarball/" + sha + ") cat \"$UPDATE_TEST_ARCHIVE\" ;; *) exit 1 ;; esac\n"
	_ = os.WriteFile(filepath.Join(bin, "gh"), []byte(gh), 0700)
	goScript := `#!/bin/sh
printf '%s\n' "$@" > "$UPDATE_TEST_ARGS"
out=''
while [ "$#" -gt 0 ]; do
  if [ "$1" = '-o' ]; then shift; out="$1"; fi
  shift
done
printf '#!/bin/sh\nprintf '\''%%s'\'' '\''{"version":"1.2.3","commit":"1234567890123456789012345678901234567890","update_protocol":1}'\''\n' > "$out"
chmod 700 "$out"
`
	_ = os.WriteFile(filepath.Join(bin, "go"), []byte(goScript), 0700)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("UPDATE_TEST_ARCHIVE", source)
	t.Setenv("UPDATE_TEST_ARGS", filepath.Join(dir, "args"))
	candidate, err := build(context.Background(), dir, Release{"1.2.3", sha})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(candidate); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	if !strings.Contains(string(args), "-buildvcs=false") || !strings.Contains(string(args), "version.Commit="+sha) {
		t.Fatal(string(args))
	}
	_ = os.WriteFile(filepath.Join(bin, "go"), []byte(strings.Replace(goScript, `"update_protocol":1`, `"update_protocol":0`, 1)), 0700)
	if _, err = build(context.Background(), dir, Release{"1.2.3", sha}); err == nil {
		t.Fatal("accepted release without supported update protocol")
	}
}

func TestHealthRequiresAuthenticatedExactHostAndBuild(t *testing.T) {
	var wrong atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-only" {
			w.WriteHeader(401)
			return
		}
		commit := sha
		if wrong.Load() {
			commit = "old"
		}
		_, _ = io.WriteString(w, `{"version":"1.2.3","commit":"`+commit+`","host_id":"host"}`)
	}))
	defer ts.Close()
	addr := strings.TrimPrefix(ts.URL, "http://")
	if err := healthy(context.Background(), addr, "fixture-only", "1.2.3", sha, "host"); err != nil {
		t.Fatal(err)
	}
	wrong.Store(true)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := healthy(ctx, addr, "fixture-only", "1.2.3", sha, "host"); err == nil {
		t.Fatal("accepted wrong commit")
	}
}

func TestExtractSkipsPaxGlobalHeader(t *testing.T) {
	// GitHub source tarballs begin with a pax_global_header entry that
	// archive/tar yields as an ordinary header; it is not the archive root.
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	if e := tw.WriteHeader(&tar.Header{Name: "pax_global_header", Typeflag: tar.TypeXGlobalHeader, Format: tar.FormatPAX, Xattrs: map[string]string{"comment": "git"}}); e != nil {
		t.Fatal(e)
	}
	_ = tw.WriteHeader(&tar.Header{Name: "root/", Typeflag: tar.TypeDir, Mode: 0700})
	_ = tw.WriteHeader(&tar.Header{Name: "root/bridge/go.mod", Typeflag: tar.TypeReg, Mode: 0600})
	_ = tw.Close()
	_ = gz.Close()
	dir := t.TempDir()
	if e := extract(bytes.NewReader(b.Bytes()), dir); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(dir, "bridge", "go.mod")); e != nil {
		t.Fatal(e)
	}
}

// A daemon replaced outside the updater (a direct install or a local build)
// reports idle, not an update result that no longer matches what runs.
func TestAnOldUpdateResultDoesNotOutliveAnotherInstall(t *testing.T) {
	dir := t.TempDir()
	if err := save(dir, &Status{ID: "u1", Phase: "updated", TargetVersion: "0.38.0", TargetCommit: "aaa"}); err != nil {
		t.Fatal(err)
	}
	s := New(Options{Dir: dir, Version: "0.38.6", Commit: "bbb"}).Get()
	if s.Phase != "idle" || s.CurrentVersion != "0.38.6" {
		t.Fatalf("%+v", s)
	}
	same := New(Options{Dir: dir, Version: "0.38.0", Commit: "aaa"}).Get()
	if same.Phase != "updated" {
		t.Fatalf("a matching update result changed: %+v", same)
	}
}
