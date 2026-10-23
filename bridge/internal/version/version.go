// Package version holds the nacred build version.
package version

// Version is the nacred version. It is overridden at build time with
// -ldflags "-X github.com/ryderdavid/nacre/bridge/internal/version.Version=...".
var Version = "0.1.0"

// Name is the service name reported by /v1/ping and /v1/health.
const Name = "nacred"
