package daemonupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"
)

const repository = "repos/ryderdavid/nacre"
const maxArchive = 64 << 20
const maxSource = 256 << 20

// boundedBuffer fails a subprocess instead of allowing unbounded output memory.
type boundedBuffer struct {
	bytes.Buffer
	Limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.Limit {
		return 0, fmt.Errorf("command output exceeds limit")
	}
	return b.Buffer.Write(p)
}
func command(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error { return syscall.Kill(-c.Process.Pid, syscall.SIGKILL) }
	c.Dir = dir
	out := &boundedBuffer{Limit: 1 << 20}
	c.Stdout = out
	c.Stderr = &boundedBuffer{Limit: 64 << 10}
	if err := c.Run(); err != nil {
		return nil, fmt.Errorf("%s command failed; check installation and authentication on this computer", name)
	}
	return out.Bytes(), nil
}
func latest(ctx context.Context) (Release, error) {
	b, err := command(ctx, "", "gh", "api", "--hostname", "github.com", repository+"/releases/latest")
	if err != nil {
		return Release{}, fmt.Errorf("No stable release could be found. Publish a Nacre GitHub release and check gh authentication on this computer.")
	}
	var r struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if json.Unmarshal(b, &r) != nil || r.Draft || r.Prerelease || !stableVersion.MatchString(r.Tag) {
		return Release{}, fmt.Errorf("latest release must have a stable vMAJOR.MINOR.PATCH tag")
	}
	b, err = command(ctx, "", "gh", "api", "--hostname", "github.com", repository+"/commits/"+r.Tag)
	if err != nil {
		return Release{}, err
	}
	var c struct {
		SHA string `json:"sha"`
	}
	if json.Unmarshal(b, &c) != nil || !commitPattern.MatchString(c.SHA) {
		return Release{}, fmt.Errorf("release commit could not be resolved")
	}
	return Release{r.Tag, c.SHA}, nil
}
func build(ctx context.Context, dir string, r Release) (string, error) {
	stage, err := os.MkdirTemp(dir, "source-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	archive, err := os.Create(filepath.Join(stage, "source.tar.gz"))
	if err != nil {
		return "", err
	}
	c := exec.CommandContext(ctx, "gh", "api", "--hostname", "github.com", repository+"/tarball/"+r.Commit)
	c.Stdout = &limitedWriter{W: archive, N: maxArchive}
	c.Stderr = &boundedBuffer{Limit: 64 << 10}
	err = c.Run()
	ce := archive.Close()
	if err != nil || ce != nil {
		return "", fmt.Errorf("release source download failed or exceeded 64 MiB")
	}
	f, err := os.Open(archive.Name())
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err = extract(f, stage); err != nil {
		return "", err
	}
	candidate := filepath.Join(dir, "candidate")
	flags := "-X github.com/ryderdavid/nacre/bridge/internal/version.Version=" + r.Version + " -X github.com/ryderdavid/nacre/bridge/internal/version.Commit=" + r.Commit
	if _, err = command(ctx, filepath.Join(stage, "bridge"), "go", "build", "-buildvcs=false", "-trimpath", "-ldflags", flags, "-o", candidate, "./cmd/nacred"); err != nil {
		return "", fmt.Errorf("release build failed; check Go on this computer")
	}
	b, err := command(ctx, "", candidate, "version", "--json")
	if err != nil {
		return "", fmt.Errorf("release does not support verified updates; install it manually")
	}
	var v struct {
		Version  string `json:"version"`
		Commit   string `json:"commit"`
		Protocol int    `json:"update_protocol"`
	}
	if json.Unmarshal(b, &v) != nil || v.Version != r.Version || v.Commit != r.Commit || v.Protocol != 1 {
		return "", fmt.Errorf("built release provenance did not match the selected commit")
	}
	return candidate, nil
}

type limitedWriter struct {
	W io.Writer
	N int64
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.N {
		return 0, fmt.Errorf("download too large")
	}
	n, e := w.W.Write(p)
	w.N -= int64(n)
	return n, e
}
func extract(in io.Reader, dir string) error {
	gz, err := gzip.NewReader(in)
	if err != nil {
		return fmt.Errorf("invalid release archive")
	}
	defer gz.Close()
	limited := &io.LimitedReader{R: gz, N: maxSource + 1}
	tr := tar.NewReader(limited)
	count := 0
	root := ""
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return fmt.Errorf("invalid release archive: %w", e)
		}
		count++
		if count > 20000 || h.Size > 16<<20 {
			return fmt.Errorf("release source exceeds limits")
		}
		name := strings.TrimSuffix(h.Name, "/")
		if path.IsAbs(name) || strings.Contains(name, "\\") || path.Clean(name) != name {
			return fmt.Errorf("unsafe release path")
		}
		parts := strings.Split(name, "/")
		for _, p := range parts {
			if p == ".." || p == "." || p == "" {
				return fmt.Errorf("unsafe release path")
			}
		}
		if root == "" {
			root = parts[0]
		}
		if root != parts[0] {
			return fmt.Errorf("multiple release roots")
		}
		if len(parts) < 2 || parts[1] != "bridge" {
			continue
		}
		rel := filepath.Join(parts[1:]...)
		dst := filepath.Join(dir, rel)
		switch h.Typeflag {
		case tar.TypeDir:
			if e = os.MkdirAll(dst, 0o700); e != nil {
				return e
			}
		case tar.TypeReg, tar.TypeRegA:
			if e = os.MkdirAll(filepath.Dir(dst), 0o700); e != nil {
				return e
			}
			f, e := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if e != nil {
				return e
			}
			_, e = io.CopyN(f, tr, h.Size)
			ce := f.Close()
			if e != nil {
				return e
			}
			if ce != nil {
				return ce
			}
		default:
			return fmt.Errorf("release source contains unsupported links or special files")
		}
	}
	if limited.N <= 0 {
		return fmt.Errorf("release source exceeds 256 MiB")
	}
	if _, err = os.Stat(filepath.Join(dir, "bridge", "go.mod")); err != nil {
		return fmt.Errorf("release has no bridge source")
	}
	return nil
}
