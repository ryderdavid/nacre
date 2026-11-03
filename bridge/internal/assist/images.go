package assist

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// StagePrefix starts the name of every per-turn image directory, so the
// startup sweep can find the ones a crash left behind.
const StagePrefix = "nacred-turn-"

// imageExt maps a validated media type to a file extension.
var imageExt = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp"}

// tempDir is where turn directories are made: TempDir when set, else the OS's.
func (r *Runner) tempDir() string {
	if r.TempDir != "" {
		return r.TempDir
	}
	return os.TempDir()
}

// stage writes a turn's images to a new private directory (0700) as files
// only this user can read (0600). The caller removes dir when the turn ends,
// whatever way it ends; dir is set even when staging fails part way.
func (r *Runner) stage(images []claude.Image) (dir string, staged []claude.StagedImage, err error) {
	dir, err = os.MkdirTemp(r.tempDir(), StagePrefix)
	if err != nil {
		return "", nil, fmt.Errorf("stage images: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return dir, nil, fmt.Errorf("stage images: %w", err)
	}
	for i, img := range images {
		p := filepath.Join(dir, strconv.Itoa(i+1)+imageExt[img.MediaType])
		if err := writePrivate(p, img.Data); err != nil {
			return dir, nil, fmt.Errorf("stage image %d: %w", i+1, err)
		}
		staged = append(staged, claude.StagedImage{MediaType: img.MediaType, Name: img.Name, Path: p})
	}
	return dir, staged, nil
}

func writePrivate(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	return errors.Join(werr, f.Close())
}

// SweepStaged removes turn directories left behind by an earlier nacred that
// stopped mid-turn. Call it once at startup, before any turn runs. It
// returns how many it removed.
func (r *Runner) SweepStaged() (int, error) {
	base := r.tempDir()
	entries, err := os.ReadDir(base)
	if err != nil {
		return 0, err
	}
	n := 0
	var errs []error
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), StagePrefix) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(base, e.Name())); err != nil {
			errs = append(errs, err)
			continue
		}
		n++
	}
	return n, errors.Join(errs...)
}
