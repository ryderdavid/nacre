// Package repoconfig reads nacre.yaml, the file in which a repository says
// what "done" means: the gates every piece of work must pass, the build that
// ends a round, the environment both need, how to bump the version, which
// resources are exclusive, and how a finished round lands.
//
// The file is checked in, so the agents doing the work and nacred checking it
// read the same commands. See openspec/changes/semi-attended-rounds.
package repoconfig

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileName is the name of the file at a repository's root.
const FileName = "nacre.yaml"

// Defaults for fields the file leaves out.
const (
	DefaultTarget             = "main"
	DefaultGateTimeoutMinutes = 20
	DefaultFixAttempts        = 2
)

// LevelPlaceholder is replaced in Version.Bump by "patch" or "minor".
const LevelPlaceholder = "{level}"

// VersionPlaceholder is replaced in Version.Tag by the version the bump made.
const VersionPlaceholder = "{version}"

// DefaultTag is the release tag when a bump is set and version.tag isn't;
// "none" turns tagging off.
const DefaultTag = "v" + VersionPlaceholder

// Config is a parsed and validated nacre.yaml.
type Config struct {
	// Gates must all exit 0 for work to count as done.
	Gates []Step
	// Build runs at the end of a round, after the gates.
	Build []Step
	// Env is added to the environment of every gate and build command.
	Env map[string]string
	// Version says how to bump the version once per round.
	Version Version
	// Resources are exclusive: one run at a time may use each.
	Resources []Resource
	Land      Land
	Limits    Limits
	Merge     Merge
}

// Merge says how a round settles conflicts it can resolve without Claude.
type Merge struct {
	// Regenerate lists files that are rebuilt rather than merged, such as
	// lockfiles: on a conflict the target's side is taken and Run rebuilds it.
	Regenerate []Regenerate
}

// Regenerate is one rebuilt file.
type Regenerate struct {
	// Path is a pattern matched against repository-relative paths with
	// path.Match ("*" doesn't cross "/").
	Path string
	Run  string
	Line int
}

// Regenerates returns the entry whose pattern matches file, if any.
func (m Merge) Regenerates(file string) (Regenerate, bool) {
	for _, r := range m.Regenerate {
		if ok, _ := path.Match(r.Path, file); ok {
			return r, true
		}
	}
	return Regenerate{}, false
}

// Step is one named command.
type Step struct {
	Name string
	Run  string
	// Artifact is the file a build step produces, relative to the repository.
	Artifact string
	// FinalOnly keeps an expensive gate out of per-bead verification; it runs
	// only on the integration branch at the end of a round.
	FinalOnly bool
	// Line is where the step starts in the file.
	Line int
}

// Version holds the version bump command and the release tag.
type Version struct {
	// Bump is run on the integration branch once per round. LevelPlaceholder
	// in it becomes "patch" or "minor".
	Bump string
	// Tag names the tag put on the landed commit, with VersionPlaceholder
	// replaced by the version the bump made, so builds can take their version
	// from `git describe --tags`. It is DefaultTag when there is a bump, and
	// "" when tagging is off.
	Tag string
}

// Resource is an exclusive resource such as an emulator.
type Resource struct {
	Name string
	// ID identifies the instance, e.g. an emulator serial, and is passed to
	// the run holding it.
	ID   string
	Line int
}

// Land says how a finished round lands.
type Land struct {
	// Target is the branch a round starts from and lands on.
	Target string
	// Auto lands without waiting for Ryder.
	Auto bool
	// Push pushes Target after landing.
	Push bool
}

// Limits bound a round's work.
type Limits struct {
	GateTimeoutMinutes int
	// FixAttempts is how many follow-up turns a failure gets before the bead
	// is parked.
	FixAttempts int
}

// Error is a problem in the file, located by line where possible.
type Error struct {
	Line int
	Msg  string
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s line %d: %s", FileName, e.Line, e.Msg)
	}
	return FileName + ": " + e.Msg
}

// ErrNotFound means the repository has no nacre.yaml.
var ErrNotFound = errors.New(FileName + " not found")

// Load reads dir/nacre.yaml. A missing file is ErrNotFound; anything wrong
// with the file is an *Error.
func Load(dir string) (*Config, error) {
	b, err := os.ReadFile(filepath.Join(dir, FileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// file mirrors the YAML. Steps and resources keep their nodes so problems can
// name a line.
type file struct {
	Gates   []yaml.Node       `yaml:"gates"`
	Build   []yaml.Node       `yaml:"build"`
	Env     map[string]string `yaml:"env"`
	Version struct {
		Bump string `yaml:"bump"`
		Tag  string `yaml:"tag"`
	} `yaml:"version"`
	Resources []yaml.Node `yaml:"resources"`
	Land      struct {
		Target string `yaml:"target"`
		Auto   bool   `yaml:"auto"`
		Push   *bool  `yaml:"push"`
	} `yaml:"land"`
	Limits struct {
		GateTimeoutMinutes *int `yaml:"gate_timeout_minutes"`
		FixAttempts        *int `yaml:"fix_attempts"`
	} `yaml:"limits"`
	Merge struct {
		Regenerate []yaml.Node `yaml:"regenerate"`
	} `yaml:"merge"`
}

var (
	envName        = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	releaseVersion = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/+-]*$`)
	resourceName   = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	yamlLine       = regexp.MustCompile(`^yaml: (?:unmarshal errors:\n\s*)?line (\d+): (.*)$`)
)

// Parse validates the contents of a nacre.yaml.
func Parse(b []byte) (*Config, error) {
	var f file
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, yamlError(err)
	}

	c := &Config{Env: f.Env, Version: Version{Bump: strings.TrimSpace(f.Version.Bump), Tag: strings.TrimSpace(f.Version.Tag)}}
	switch {
	case c.Version.Tag == "none":
		c.Version.Tag = ""
	case c.Version.Tag == "" && c.Version.Bump != "":
		c.Version.Tag = DefaultTag
	}
	if t := c.Version.Tag; t != "" {
		if c.Version.Bump == "" {
			return nil, &Error{Msg: "version: tag needs a bump: the tag is named after the version the bump makes"}
		}
		if !strings.Contains(t, VersionPlaceholder) || !releaseVersion.MatchString(strings.ReplaceAll(t, VersionPlaceholder, "1.0.0")) {
			return nil, &Error{Msg: fmt.Sprintf("version: tag %q must contain %s and otherwise only letters, digits, '.', '-', '_' or '/'", t, VersionPlaceholder)}
		}
	}
	seen := map[string]int{}
	var err error
	if c.Gates, err = steps("gate", f.Gates, false, seen); err != nil {
		return nil, err
	}
	if len(c.Gates) == 0 {
		return nil, &Error{Msg: "at least one gate is required: a gate is the command that says work is done"}
	}
	if c.Build, err = steps("build step", f.Build, true, seen); err != nil {
		return nil, err
	}
	for k := range c.Env {
		if !envName.MatchString(k) {
			return nil, &Error{Msg: fmt.Sprintf("env: %q is not a valid environment variable name", k)}
		}
	}
	if c.Resources, err = resources(f.Resources); err != nil {
		return nil, err
	}

	c.Land = Land{Target: strings.TrimSpace(f.Land.Target), Auto: f.Land.Auto, Push: true}
	if c.Land.Target == "" {
		c.Land.Target = DefaultTarget
	}
	if strings.HasPrefix(c.Land.Target, "-") || strings.ContainsAny(c.Land.Target, " \t\n~^:?*[\\") {
		return nil, &Error{Msg: fmt.Sprintf("land: %q is not a valid branch name", c.Land.Target)}
	}
	if f.Land.Push != nil {
		c.Land.Push = *f.Land.Push
	}

	c.Limits = Limits{GateTimeoutMinutes: DefaultGateTimeoutMinutes, FixAttempts: DefaultFixAttempts}
	if v := f.Limits.GateTimeoutMinutes; v != nil {
		if *v <= 0 {
			return nil, &Error{Msg: "limits: gate_timeout_minutes must be more than 0"}
		}
		c.Limits.GateTimeoutMinutes = *v
	}
	if v := f.Limits.FixAttempts; v != nil {
		if *v < 0 {
			return nil, &Error{Msg: "limits: fix_attempts can't be negative"}
		}
		c.Limits.FixAttempts = *v
	}
	if c.Merge.Regenerate, err = regenerates(f.Merge.Regenerate); err != nil {
		return nil, err
	}
	return c, nil
}

// regenerates validates merge.regenerate.
func regenerates(nodes []yaml.Node) ([]Regenerate, error) {
	var out []Regenerate
	for i := range nodes {
		n := &nodes[i]
		where := fmt.Sprintf("merge.regenerate %d", i+1)
		if n.Kind != yaml.MappingNode {
			return nil, &Error{Line: n.Line, Msg: where + ": expected path and run"}
		}
		for k := 0; k+1 < len(n.Content); k += 2 {
			switch key := n.Content[k].Value; key {
			case "path", "run":
			default:
				return nil, &Error{Line: n.Content[k].Line, Msg: fmt.Sprintf("%s: unknown field %q", where, key)}
			}
		}
		var r struct {
			Path string `yaml:"path"`
			Run  string `yaml:"run"`
		}
		if err := n.Decode(&r); err != nil {
			return nil, yamlError(err)
		}
		r.Path, r.Run = strings.TrimSpace(r.Path), strings.TrimSpace(r.Run)
		switch {
		case r.Path == "":
			return nil, &Error{Line: n.Line, Msg: where + ": path is required"}
		case r.Run == "":
			return nil, &Error{Line: n.Line, Msg: where + ": run is required: the command that rebuilds " + r.Path}
		case strings.HasPrefix(r.Path, "/") || strings.Contains(r.Path, ".."):
			return nil, &Error{Line: n.Line, Msg: fmt.Sprintf("%s: %q must be a path inside the repository", where, r.Path)}
		}
		if _, err := path.Match(r.Path, ""); err != nil {
			return nil, &Error{Line: n.Line, Msg: fmt.Sprintf("%s: %q is not a valid pattern", where, r.Path)}
		}
		out = append(out, Regenerate{Path: r.Path, Run: r.Run, Line: n.Line})
	}
	return out, nil
}

// steps validates a list of gates or build steps. Names are unique across both
// lists, since results are reported by name.
func steps(kind string, nodes []yaml.Node, build bool, seen map[string]int) ([]Step, error) {
	out := make([]Step, 0, len(nodes))
	for i := range nodes {
		n := &nodes[i]
		where := fmt.Sprintf("%s %d", kind, i+1)
		if n.Kind != yaml.MappingNode {
			return nil, &Error{Line: n.Line, Msg: where + ": expected name and run"}
		}
		var s struct {
			Name      string `yaml:"name"`
			Run       string `yaml:"run"`
			Artifact  string `yaml:"artifact"`
			FinalOnly bool   `yaml:"final_only"`
		}
		if err := n.Decode(&s); err != nil {
			return nil, yamlError(err)
		}
		s.Name, s.Run = strings.TrimSpace(s.Name), strings.TrimSpace(s.Run)
		if s.Name != "" {
			where = fmt.Sprintf("%s %d (%q)", kind, i+1, s.Name)
		}
		// Decoding a node ignores unknown keys, so check them here.
		for k := 0; k+1 < len(n.Content); k += 2 {
			switch key := n.Content[k].Value; key {
			case "name", "run", "artifact", "final_only":
			default:
				return nil, &Error{Line: n.Content[k].Line, Msg: fmt.Sprintf("%s: unknown field %q", where, key)}
			}
		}
		switch {
		case s.Name == "":
			return nil, &Error{Line: n.Line, Msg: where + ": name is required"}
		case s.Run == "":
			return nil, &Error{Line: n.Line, Msg: where + ": run is required"}
		case s.Artifact != "" && !build:
			return nil, &Error{Line: n.Line, Msg: where + ": artifact belongs on a build step, not a gate"}
		case s.FinalOnly && build:
			return nil, &Error{Line: n.Line, Msg: where + ": final_only is for gates; build steps always run at the end"}
		case filepath.IsAbs(s.Artifact) || strings.HasPrefix(filepath.Clean(s.Artifact), ".."):
			return nil, &Error{Line: n.Line, Msg: where + ": artifact must be a path inside the repository"}
		}
		if prev, dup := seen[s.Name]; dup {
			return nil, &Error{Line: n.Line, Msg: fmt.Sprintf("%s: the name is already used on line %d", where, prev)}
		}
		seen[s.Name] = n.Line
		out = append(out, Step{Name: s.Name, Run: s.Run, Artifact: s.Artifact, FinalOnly: s.FinalOnly, Line: n.Line})
	}
	return out, nil
}

// resources accepts either a bare name ("emulator") or {name, id}.
func resources(nodes []yaml.Node) ([]Resource, error) {
	out := make([]Resource, 0, len(nodes))
	seen := map[string]bool{}
	for i := range nodes {
		n := &nodes[i]
		var r Resource
		switch n.Kind {
		case yaml.ScalarNode:
			r.Name = n.Value
		case yaml.MappingNode:
			var m struct {
				Name string `yaml:"name"`
				ID   string `yaml:"id"`
			}
			for k := 0; k+1 < len(n.Content); k += 2 {
				if key := n.Content[k].Value; key != "name" && key != "id" {
					return nil, &Error{Line: n.Content[k].Line, Msg: fmt.Sprintf("resource %d: unknown field %q", i+1, key)}
				}
			}
			if err := n.Decode(&m); err != nil {
				return nil, yamlError(err)
			}
			r.Name, r.ID = m.Name, m.ID
		default:
			return nil, &Error{Line: n.Line, Msg: fmt.Sprintf("resource %d: expected a name, or name and id", i+1)}
		}
		r.Name, r.ID, r.Line = strings.TrimSpace(r.Name), strings.TrimSpace(r.ID), n.Line
		if !resourceName.MatchString(r.Name) {
			return nil, &Error{Line: n.Line, Msg: fmt.Sprintf("resource %d: %q must be lowercase letters, digits and dashes", i+1, r.Name)}
		}
		if seen[r.Name] {
			return nil, &Error{Line: n.Line, Msg: fmt.Sprintf("resource %q is listed twice", r.Name)}
		}
		seen[r.Name] = true
		out = append(out, r)
	}
	return out, nil
}

// yamlError turns a yaml.v3 error into an *Error with its line.
func yamlError(err error) error {
	var te *yaml.TypeError
	if errors.As(err, &te) && len(te.Errors) > 0 {
		err = errors.New("yaml: " + te.Errors[0])
	}
	if m := yamlLine.FindStringSubmatch(err.Error()); m != nil {
		var line int
		fmt.Sscan(m[1], &line)
		return &Error{Line: line, Msg: m[2]}
	}
	return &Error{Msg: strings.TrimPrefix(err.Error(), "yaml: ")}
}

// BumpCommand is the version bump with its level filled in.
func (c *Config) BumpCommand(minor bool) string {
	level := "patch"
	if minor {
		level = "minor"
	}
	return strings.ReplaceAll(c.Version.Bump, LevelPlaceholder, level)
}

// TagName is the release tag for a bump whose output ends in bumped, such as
// "0.5.1 (11) -> 0.6.0 (12)" or just "0.6.0": the first word after the last
// "->", or of the line. It is "" when no tag is configured or the bump didn't
// print a usable version.
func (c *Config) TagName(bumped string) string {
	if c.Version.Tag == "" {
		return ""
	}
	if i := strings.LastIndex(bumped, "->"); i >= 0 {
		bumped = bumped[i+2:]
	}
	f := strings.Fields(bumped)
	if len(f) == 0 || f[0][0] < '0' || f[0][0] > '9' || !releaseVersion.MatchString(f[0]) || strings.Contains(f[0], "..") {
		return ""
	}
	return strings.ReplaceAll(c.Version.Tag, VersionPlaceholder, f[0])
}

// GateNames lists the gates in order.
func (c *Config) GateNames() []string {
	names := make([]string, len(c.Gates))
	for i, g := range c.Gates {
		names[i] = g.Name
	}
	return names
}
