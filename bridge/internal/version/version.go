// Package version holds the nacred build version.
package version

import "runtime/debug"

// Version and Commit are set at build time from git (see the Makefile):
//
//	-ldflags "-X github.com/ryderdavid/nacre/bridge/internal/version.Version=0.6.0
//	          -X github.com/ryderdavid/nacre/bridge/internal/version.Commit=<sha>"
//
// Version is `git describe --tags --dirty` without the leading "v": "0.6.0"
// for a build of a release tag, "0.6.0-3-gabc1234" for one three commits
// past it, with "-dirty" when the tree had uncommitted changes. A build that
// didn't set it (a plain `go build`) reports Dev, or Dev+"-dirty".
var (
	Version = ""
	Commit  = ""
)

// Dev is the version of a build that wasn't given one from git.
const Dev = "dev"

// Name is the service name reported by /v1/ping and /v1/health.
const Name = "nacred"

func init() {
	Version, Commit = resolve(Version, Commit, vcsInfo())
}

// vcs is what the Go toolchain stamped into the binary about its checkout.
type vcs struct {
	revision string
	modified bool
}

func vcsInfo() vcs {
	var v vcs
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return v
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			v.revision = s.Value
		case "vcs.modified":
			v.modified = s.Value == "true"
		}
	}
	return v
}

// resolve fills in what the build didn't set: the commit from the toolchain's
// VCS stamp, and a version that says it is a development build.
func resolve(version, commit string, stamp vcs) (string, string) {
	if commit == "" {
		commit = stamp.revision
	}
	if version == "" {
		version = Dev
		if stamp.modified {
			version += "-dirty"
		}
	}
	return version, commit
}
