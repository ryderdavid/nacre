package sessions

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// procStartSkew is how far a record's procStart may differ from the start time
// the operating system reports. ps(1) prints whole seconds and the two clocks
// are read at slightly different moments, so an exact match is not available.
const procStartSkew = 2 * time.Second

// procTimeout bounds the ps(1) call made once per poll.
const procTimeout = 5 * time.Second

// ProcTable reports the start times of live processes. Comparing a start time
// against the record's procStart is what makes a stale record safe: session
// records are not reliably removed when a session ends, and a pid is reused
// soon enough that liveness alone would adopt an unrelated process.
type ProcTable interface {
	// StartTimes returns the start time of each pid that is alive now. Pids
	// that are dead, or whose start time cannot be read, are absent.
	StartTimes(pids []int) map[int]time.Time
}

// SystemProcs returns a ProcTable backed by the operating system.
func SystemProcs() ProcTable { return psTable{} }

// psTable reads process start times from ps(1), which reports them in local
// time with one-second resolution.
type psTable struct{}

func (psTable) StartTimes(pids []int) map[int]time.Time {
	out := map[int]time.Time{}
	args := make([]string, 0, len(pids))
	for _, pid := range pids {
		if alive(pid) {
			args = append(args, strconv.Itoa(pid))
		}
	}
	if len(args) == 0 {
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), procTimeout)
	defer cancel()
	// Exits non-zero when no pid matched, which is not an error here: the
	// output is authoritative and an empty one means nothing is alive.
	raw, _ := exec.CommandContext(ctx, "ps", "-o", "pid=,lstart=", "-p", strings.Join(args, ",")).Output()
	for _, line := range strings.Split(string(raw), "\n") {
		pid, start, ok := parsePSLine(line)
		if ok {
			out[pid] = start
		}
	}
	return out
}

// parsePSLine reads one "<pid> Mon Jan _2 15:04:05 2006" line of `ps -o
// pid=,lstart=` output.
func parsePSLine(line string) (int, time.Time, bool) {
	line = strings.TrimSpace(line)
	pidStr, rest, found := strings.Cut(line, " ")
	if !found {
		return 0, time.Time{}, false
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		return 0, time.Time{}, false
	}
	start, err := time.ParseInLocation(procStartLayout, strings.TrimSpace(rest), time.Local)
	if err != nil {
		return 0, time.Time{}, false
	}
	return pid, start, true
}

// alive reports whether a process with pid exists. EPERM means it exists but
// belongs to another user.
func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// live reports whether rec's process is still the process the record was
// written for. A record with no usable procStart is never live, so an
// ambiguous record is dropped rather than adopted against the wrong process.
func live(rec Record, starts map[int]time.Time) bool {
	want := rec.ProcStarted()
	got, ok := starts[rec.PID]
	if !ok || want.IsZero() {
		return false
	}
	d := want.Sub(got)
	if d < 0 {
		d = -d
	}
	return d <= procStartSkew
}

// Alive reports whether pid is alive now and is the process that started at
// procStart, a start time as a session record writes it. It is the liveness
// half of adoption, for checking one record again on demand.
func Alive(procs ProcTable, pid int, procStart string) bool {
	if procs == nil {
		procs = SystemProcs()
	}
	rec := Record{PID: pid, ProcStart: procStart}
	return live(rec, procs.StartTimes([]int{pid}))
}
