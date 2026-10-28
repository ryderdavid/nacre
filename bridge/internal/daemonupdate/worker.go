package daemonupdate

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Release struct{ Version, Commit string }

// Environment separates destructive service operations from deterministic tests.
type Environment struct {
	Latest  func(context.Context) (Release, error)
	Build   func(context.Context, Release) (string, error)
	Backup  func() error
	Install func(string) error
	Restart func(context.Context) error
	Healthy func(context.Context, string, string, string) error
	Restore func() error
}

var stableVersion = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var commitPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)

func versionParts(v string) ([3]uint64, bool) {
	var p [3]uint64
	m := stableVersion.FindStringSubmatch(v)
	if m == nil {
		return p, false
	}
	for i := range p {
		n, e := strconv.ParseUint(m[i+1], 10, 64)
		if e != nil {
			return p, false
		}
		p[i] = n
	}
	return p, true
}
func older(target, current string) bool {
	t, ok := versionParts(target)
	if !ok {
		return false
	}
	c, ok := versionParts(strings.SplitN(current, "-", 2)[0])
	if !ok {
		return false
	}
	for i := range t {
		if t[i] != c[i] {
			return t[i] < c[i]
		}
	}
	return false
}

// Work runs in the independent helper. The old service remains healthy until
// the complete candidate and rollback are ready. No error counts as success.
func Work(ctx context.Context, dir, id string, e Environment) error {
	s, err := read(dir)
	if err != nil {
		return err
	}
	if s.ID != id || !s.Active {
		return fmt.Errorf("update operation no longer active")
	}
	set := func(phase, msg string, active bool) error {
		s.Phase = phase
		s.Message = msg
		s.Active = active
		return save(dir, &s)
	}
	fail := func(err error) error { _ = set("failed", err.Error(), false); return err }
	release, err := e.Latest(ctx)
	if err != nil {
		return fail(err)
	}
	if !stableVersion.MatchString(release.Version) || !commitPattern.MatchString(release.Commit) {
		return fail(fmt.Errorf("release tag or commit is invalid"))
	}
	s.TargetVersion = strings.TrimPrefix(release.Version, "v")
	s.TargetCommit = release.Commit
	if older(s.TargetVersion, s.CurrentVersion) {
		return set("current", "This computer is newer than the latest published release.", false)
	}
	if s.CurrentCommit == s.TargetCommit && strings.TrimPrefix(s.CurrentVersion, "v") == s.TargetVersion {
		return set("current", "Already running the latest stable release.", false)
	}
	if err = set("building", "Downloading and building the release…", true); err != nil {
		return err
	}
	candidate, err := e.Build(ctx, Release{s.TargetVersion, s.TargetCommit})
	if err != nil {
		return fail(err)
	}
	if err = ctx.Err(); err != nil {
		return fail(err)
	}
	if err = e.Backup(); err != nil {
		return fail(fmt.Errorf("could not preserve rollback: %w", err))
	}
	if err = set("restarting", "Restarting and verifying the new daemon…", true); err != nil {
		return err
	}
	if err = e.Install(candidate); err != nil {
		return fail(fmt.Errorf("installation failed before replacement: %w", err))
	}
	err = e.Restart(ctx)
	if err == nil {
		err = e.Healthy(ctx, s.TargetVersion, s.TargetCommit, s.HostID)
	}
	if err == nil {
		return set("updated", "Updated to "+s.TargetVersion+" and verified the connection.", false)
	}
	cause := err
	// Status storage failure must never prevent restoring an unhealthy service.
	_ = set("rolling_back", "The new daemon did not verify. Restoring the previous version…", true)
	recovery, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err = e.Restore(); err == nil {
		err = e.Restart(recovery)
	}
	if err == nil {
		err = e.Healthy(recovery, s.CurrentVersion, s.CurrentCommit, s.HostID)
	}
	if err != nil {
		_ = set("recovery_failed", "Update failed and the previous daemon could not be verified. Check this computer. Rollback binary is saved in its nacred updates directory.", false)
		return fmt.Errorf("update: %v; recovery: %w", cause, err)
	}
	return set("rolled_back", "Update failed; restored and verified the previous daemon. "+cause.Error(), false)
}
