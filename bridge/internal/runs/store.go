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

// appendEvent appends evs to a run's event log in one write.
func appendEvent(dir string, evs ...api.RunEvent) error {
	var data []byte
	for _, ev := range evs {
		line, err := json.Marshal(ev)
		if err != nil {
			return err
		}
		data = append(append(data, line...), '\n')
	}
	if len(data) == 0 {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(dir, eventsFile), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// writeEvents replaces a run's event log with evs atomically: the new log is
// written and synced to a temporary file, then renamed over the old one, so a
// crash leaves either the old log or the new one, never a half-written mix.
func writeEvents(dir string, evs []api.RunEvent) error {
	var data []byte
	for _, ev := range evs {
		line, err := json.Marshal(ev)
		if err != nil {
			return err
		}
		data = append(append(data, line...), '\n')
	}
	tmp := filepath.Join(dir, eventsFile+".tmp")
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, eventsFile))
}

// eventsSize is the size in bytes of a run's event log, 0 if it has none.
func eventsSize(dir string) int64 {
	fi, err := os.Stat(filepath.Join(dir, eventsFile))
	if err != nil {
		return 0
	}
	return fi.Size()
}

// readEvents loads a run's events, skipping corrupt lines (e.g. a torn final
// write after a crash). keep > 0 holds only the newest keep events while
// reading, so an oversized log costs bounded memory.
func readEvents(dir string, keep int) ([]api.RunEvent, error) {
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
				if keep > 0 && len(out) >= 2*keep {
					out = append(out[:0], out[len(out)-keep:]...)
				}
			}
		}
		if err != nil {
			break
		}
	}
	if keep > 0 && len(out) > keep {
		out = append([]api.RunEvent(nil), out[len(out)-keep:]...)
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
