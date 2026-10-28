package specflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const maxFileBytes = 128 << 10
const maxInventoryBytes = 1 << 20
const maxInventoryFiles = 256
const maxArtifacts = 40

type inventory struct {
	Files   []Artifact        `json:"files"`
	Digests map[string]string `json:"digests"`
}

func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func canonicalRepo(repo string) (string, error) {
	p, e := filepath.Abs(repo)
	if e != nil {
		return "", e
	}
	return filepath.EvalSymlinks(p)
}
func safeRelative(p string) bool {
	return p != "" && p == path.Clean(p) && !path.IsAbs(p) && p != "." && p != ".." && !strings.HasPrefix(p, "../") && !strings.ContainsAny(p, "\\\x00")
}
func noLinks(root *os.Root, p string) error {
	if !safeRelative(p) {
		return fmt.Errorf("unsafe repository path %q", p)
	}
	parts := strings.Split(p, "/")
	for i := range parts {
		s := strings.Join(parts[:i+1], "/")
		info, e := root.Lstat(s)
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not permitted in specification path %s", s)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("specification ancestor is not a directory: %s", s)
		}
	}
	return nil
}
func readFile(root *os.Root, p string) (string, error) {
	if e := noLinks(root, p); e != nil {
		return "", e
	}
	f, e := root.Open(p)
	if e != nil {
		return "", e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return "", e
	}
	if !st.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular specification file: %s", p)
	}
	b, e := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if e != nil {
		return "", e
	}
	if len(b) > maxFileBytes {
		return "", fmt.Errorf("%s exceeds the %d-byte context limit", p, maxFileBytes)
	}
	return string(b), nil
}
func collect(repo string) (inventory, error) {
	inv := inventory{Files: []Artifact{}, Digests: map[string]string{}}
	root, e := os.OpenRoot(repo)
	if e != nil {
		return inv, e
	}
	defer root.Close()
	add := func(p string) error {
		s, e := readFile(root, p)
		if e != nil {
			return fmt.Errorf("cannot read %s: %w", p, e)
		}
		inv.Files = append(inv.Files, Artifact{p, s})
		inv.Digests[p] = digest(s)
		return nil
	}
	if e = add("openspec/config.yaml"); e != nil {
		return inv, e
	}
	var cfg struct {
		Schema  string `yaml:"schema"`
		Context string `yaml:"context"`
	}
	if e = yaml.Unmarshal([]byte(inv.Files[0].Content), &cfg); e != nil || strings.TrimSpace(cfg.Schema) == "" || strings.TrimSpace(cfg.Context) == "" {
		return inv, fmt.Errorf("openspec/config.yaml needs valid schema and project context before a proposal can be developed")
	}
	if e = add("AGENTS.md"); e != nil {
		return inv, e
	}
	for _, base := range []string{"openspec/specs", "openspec/changes"} {
		if e = noLinks(root, base); e != nil {
			return inv, e
		}
		if _, e = root.Stat(base); os.IsNotExist(e) {
			continue
		} else if e != nil {
			return inv, e
		}
		e = fs.WalkDir(root.FS(), base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if p == "openspec/changes/archive" {
				return fs.SkipDir
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink in specification inventory: %s", p)
			}
			if d.IsDir() {
				return nil
			}
			if e := add(p); e != nil {
				return e
			}
			if len(inv.Files) > maxInventoryFiles {
				return fmt.Errorf("specification inventory exceeds %d files", maxInventoryFiles)
			}
			return nil
		})
		if e != nil {
			return inv, e
		}
	}
	total := 0
	for _, f := range inv.Files {
		total += len(f.Content)
	}
	if total > maxInventoryBytes {
		return inv, fmt.Errorf("specification inventory exceeds %d bytes; narrow it before developing a proposal", maxInventoryBytes)
	}
	sort.Slice(inv.Files, func(i, j int) bool { return inv.Files[i].Path < inv.Files[j].Path })
	return inv, nil
}
func revision(inv inventory, artifacts []Artifact) string {
	b, _ := json.Marshal(struct {
		Context   []Artifact
		Artifacts []Artifact
	}{inv.Files, artifacts})
	return digest(string(b))
}
func validateArtifactPaths(name string, artifacts []Artifact) error {
	if !slugPattern.MatchString(name) || name == "archive" {
		return fmt.Errorf("change name must be a lowercase hyphenated slug")
	}
	if len(artifacts) > maxArtifacts {
		return fmt.Errorf("proposal has too many artifacts")
	}
	seen := map[string]bool{}
	prefix := "openspec/changes/" + name + "/"
	hasSpec := false
	total := 0
	for _, a := range artifacts {
		if !safeRelative(a.Path) || !strings.HasPrefix(a.Path, prefix) {
			return fmt.Errorf("artifact is outside the active change: %s", a.Path)
		}
		rel := strings.TrimPrefix(a.Path, prefix)
		valid := rel == "proposal.md" || rel == "design.md" || rel == "tasks.md"
		parts := strings.Split(rel, "/")
		if len(parts) == 3 && parts[0] == "specs" && slugPattern.MatchString(parts[1]) && parts[2] == "spec.md" {
			valid = true
			hasSpec = true
		}
		if !valid {
			return fmt.Errorf("unsupported proposal artifact: %s", a.Path)
		}
		if seen[a.Path] {
			return fmt.Errorf("duplicate artifact: %s", a.Path)
		}
		seen[a.Path] = true
		if len(a.Content) > maxFileBytes || strings.TrimSpace(a.Content) == "" {
			return fmt.Errorf("empty or oversized artifact: %s", a.Path)
		}
		total += len(a.Content)
	}
	if total > maxInventoryBytes/2 {
		return fmt.Errorf("generated artifacts exceed output limit")
	}
	for _, required := range []string{"proposal.md", "design.md", "tasks.md"} {
		if !seen[prefix+required] {
			return fmt.Errorf("proposal is missing %s", required)
		}
	}
	if !hasSpec {
		return fmt.Errorf("proposal needs at least one behavior specification")
	}
	return nil
}

// writeArtifacts performs compare-and-swap against every original artifact. An
// already-written desired value is accepted for recovery after a lost response.
func writeArtifacts(repo string, artifacts []Artifact, baseline map[string]string) error {
	root, e := os.OpenRoot(repo)
	if e != nil {
		return e
	}
	defer root.Close()
	for _, a := range artifacts {
		current, e := readFile(root, a.Path)
		exists := e == nil
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		old, was := baseline[a.Path]
		if exists && digest(current) == digest(a.Content) {
			continue
		}
		if exists != was || (exists && digest(current) != old) {
			return fmt.Errorf("concurrent edit in %s; your files have been preserved", a.Path)
		}
	}
	for _, a := range artifacts {
		if e = noLinks(root, a.Path); e != nil {
			return e
		}
		if e = root.MkdirAll(path.Dir(a.Path), 0o755); e != nil {
			return e
		}
		current, er := readFile(root, a.Path)
		if er == nil && current == a.Content {
			continue
		}
		// Check again immediately before replacing this file. os.Root confines every
		// operation even if another process swaps an ancestor during this check.
		old, was := baseline[a.Path]
		if er != nil && !os.IsNotExist(er) {
			return er
		}
		if (er == nil) != was || (er == nil && digest(current) != old) {
			return fmt.Errorf("concurrent edit in %s", a.Path)
		}
		tmp := a.Path + ".nacre-" + newID()
		f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, err = f.WriteString(a.Content)
		if err == nil {
			err = f.Sync()
		}
		ce := f.Close()
		if err == nil {
			err = ce
		}
		if err == nil {
			err = root.Rename(tmp, a.Path)
		}
		if err != nil {
			_ = root.Remove(tmp)
			return err
		}
	}
	return nil
}
func strictValidator(ctx context.Context, repo, name string) error {
	binary, e := exec.LookPath("openspec")
	if e != nil {
		return fmt.Errorf("install the OpenSpec CLI on this computer to validate proposals")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "validate", name, "--strict", "--no-interactive")
	cmd.Dir = repo
	out, e := cmd.CombinedOutput()
	if e != nil {
		s := string(out)
		if len(s) > 6000 {
			s = s[:6000]
		}
		return fmt.Errorf("OpenSpec validation failed: %s", strings.TrimSpace(s))
	}
	return nil
}
func atomicJSON(file string, value any) error {
	b, e := json.MarshalIndent(value, "", "  ")
	if e != nil {
		return e
	}
	dir := filepath.Dir(file)
	if e = os.MkdirAll(dir, 0o700); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".workflow-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0o600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(tmp, file)
}
