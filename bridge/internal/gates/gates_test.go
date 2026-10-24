package gates

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/repoconfig"
)

func step(name, run string) repoconfig.Step { return repoconfig.Step{Name: name, Run: run} }

func TestPassAndFail(t *testing.T) {
	r := NewRunner(1)
	dir := t.TempDir()
	ok := r.Run(context.Background(), dir, nil, step("ok", "echo fine; pwd"), time.Minute)
	if !ok.OK || ok.ExitCode != 0 || !strings.Contains(ok.Tail, "fine") {
		t.Fatalf("ok = %+v", ok)
	}
	// It ran in dir.
	real, _ := filepath.EvalSymlinks(dir)
	if !strings.Contains(ok.Tail, real) && !strings.Contains(ok.Tail, dir) {
		t.Errorf("not run in %s: %q", dir, ok.Tail)
	}

	bad := r.Run(context.Background(), dir, nil, step("bad", "echo broken >&2; exit 3"), time.Minute)
	if bad.OK || bad.ExitCode != 3 || !strings.Contains(bad.Tail, "broken") {
		t.Fatalf("bad = %+v", bad)
	}
	if !strings.Contains(bad.Summary(), "exit status 3") {
		t.Errorf("summary = %q", bad.Summary())
	}
}

func TestTimeoutKillsTheWholeGroup(t *testing.T) {
	r := NewRunner(1)
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	// A child that would outlive its shell if only the shell were killed.
	res := r.Run(context.Background(), dir, nil, step("slow", fmt.Sprintf("sleep 60 & echo $! > %s; wait", pidFile)), 300*time.Millisecond)
	if res.OK || !res.TimedOut {
		t.Fatalf("res = %+v", res)
	}
	if !strings.Contains(res.Summary(), "timed out") {
		t.Errorf("summary = %q", res.Summary())
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	fmt.Sscan(string(b), &pid)
	// The orphaned child is reparented and reaped by launchd, so once killed
	// it stops existing. Give it a moment.
	deadline := time.Now().Add(2 * time.Second)
	for syscall.Kill(pid, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if syscall.Kill(pid, 0) == nil {
		t.Errorf("child %d survived the timeout", pid)
	}
}

func TestEnvAndTail(t *testing.T) {
	r := NewRunner(1)
	r.BaseEnv = []string{"PATH=/usr/bin:/bin", "KEEP=yes", "OVERRIDE=old"}
	res := r.Run(context.Background(), t.TempDir(), map[string]string{"OVERRIDE": "new", "EXTRA": "x"},
		step("env", `echo "$KEEP $OVERRIDE $EXTRA"; i=0; while [ $i -lt 500 ]; do echo line$i; i=$((i+1)); done`), time.Minute)
	if !res.OK {
		t.Fatalf("res = %+v", res)
	}
	lines := strings.Split(res.Tail, "\n")
	if len(lines) != TailLines || lines[len(lines)-1] != "line499" || lines[0] != "line300" {
		t.Errorf("tail has %d lines, first %q last %q", len(lines), lines[0], lines[len(lines)-1])
	}
	// The env line scrolled out of the tail; check it separately.
	res = r.Run(context.Background(), t.TempDir(), map[string]string{"OVERRIDE": "new", "EXTRA": "x"}, step("env", `echo "$KEEP $OVERRIDE $EXTRA"`), time.Minute)
	if strings.TrimSpace(res.Tail) != "yes new x" {
		t.Errorf("env = %q", res.Tail)
	}
}

func TestComputerWideLimit(t *testing.T) {
	r := NewRunner(2)
	var running, peak atomic.Int32
	var wg sync.WaitGroup
	dir := t.TempDir()
	marker := filepath.Join(dir, "m")
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Each gate notes itself running for a moment.
			res := r.Run(context.Background(), dir, nil, step(fmt.Sprint(i), fmt.Sprintf("touch %s.%d; sleep 0.2; rm %s.%d", marker, i, marker, i)), time.Minute)
			if !res.OK {
				t.Errorf("gate %d: %+v", i, res)
			}
		}(i)
	}
	// Sample how many markers exist at once while the gates run.
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			m, _ := filepath.Glob(marker + ".*")
			n := int32(len(m))
			running.Store(n)
			if n > peak.Load() {
				peak.Store(n)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	wg.Wait()
	close(stop)
	if p := peak.Load(); p > 2 || p == 0 {
		t.Errorf("peak concurrent gates = %d, want 1..2", p)
	}
}

func TestCancelWhileWaiting(t *testing.T) {
	r := NewRunner(1)
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	go func() {
		close(started)
		r.Run(context.Background(), t.TempDir(), nil, step("hog", "sleep 1"), time.Minute)
	}()
	<-started
	time.Sleep(50 * time.Millisecond)
	cancel()
	res := r.Run(ctx, t.TempDir(), nil, step("waiting", "true"), time.Minute)
	if res.OK || !strings.Contains(res.Error, "waiting for a free gate slot") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRunAllStopsOnFirstFailure(t *testing.T) {
	cfg := &repoconfig.Config{Limits: repoconfig.Limits{GateTimeoutMinutes: 1}}
	steps := []repoconfig.Step{step("a", "true"), step("b", "false"), step("c", "true")}
	r := NewRunner(1)
	if res := r.RunAll(context.Background(), t.TempDir(), cfg, steps, true); len(res) != 2 || AllOK(res) {
		t.Errorf("stopOnFail: %+v", res)
	}
	if res := r.RunAll(context.Background(), t.TempDir(), cfg, steps, false); len(res) != 3 || AllOK(res) {
		t.Errorf("all: %+v", res)
	}
	if AllOK(nil) {
		t.Error("no results is not a pass")
	}
}
