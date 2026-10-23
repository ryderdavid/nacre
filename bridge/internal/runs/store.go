package runs

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

// On-disk layout: <dir>/<run id>/meta.json (the api.Run) and
// <dir>/<run id>/events.jsonl (one api.RunEvent per line).
const (
	metaFile   = "meta.json"
	eventsFile = "events.jsonl"
)

// newID returns a sortable unique run id: "r_" + a ULID (48-bit ms timestamp
// + 80 random bits, Crockford base32).
func newID(now time.Time) string {
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	var b [16]byte
	ms := uint64(now.UnixMilli())
	for i := 5; i >= 0; i-- {
		b[i] = byte(ms)
		ms >>= 8
	}
	_, _ = rand.Read(b[6:])
	// Encode 128 bits as 26 base32 chars (first char carries 3 bits).
	var out [26]byte
	var acc uint64
	bits := 0
	idx := 25
	for i := 15; i >= 0; i-- {
		acc |= uint64(b[i]) << bits
		bits += 8
		for bits >= 5 && idx >= 0 {
			out[idx] = alphabet[acc&31]
			acc >>= 5
			bits -= 5
			idx--
		}
	}
	for idx >= 0 {
		out[idx] = alphabet[acc&31]
		acc >>= 5
		idx--
	}
	return "r_" + string(out[:])
}

func writeMeta(dir string, meta api.Run) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, metaFile+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, metaFile))
}

func readMeta(dir string) (api.Run, error) {
	var meta api.Run
	data, err := os.ReadFile(filepath.Join(dir, metaFile))
	if err != nil {
		return meta, err
	}
	err = json.Unmarshal(data, &meta)
	return meta, err
}

func appendEvent(dir string, ev api.RunEvent) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, eventsFile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// readEvents loads a run's events, skipping corrupt lines (e.g. a torn final
// write after a crash).
func readEvents(dir string) ([]api.RunEvent, error) {
	f, err := os.Open(filepath.Join(dir, eventsFile))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []api.RunEvent
	rd := bufio.NewReader(f)
	for {
		line, err := rd.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			var ev api.RunEvent
			if json.Unmarshal(line, &ev) == nil && ev.Seq > 0 {
				out = append(out, ev)
			}
		}
		if err != nil {
			break
		}
	}
	return out, nil
}

// listRunDirs returns run ids found under dir.
func listRunDirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "r_") {
			ids = append(ids, e.Name())
		}
	}
	return ids, nil
}
