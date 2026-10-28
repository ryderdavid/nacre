// Package daemonupdate installs only published stable Nacre releases.
// Status lives outside the service so a separate LaunchAgent can verify restart.
package daemonupdate

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const Window = 25 * time.Minute

type Status struct {
	Available      bool      `json:"available"`
	ID             string    `json:"id,omitempty"`
	HostID         string    `json:"host_id,omitempty"`
	Phase          string    `json:"phase"`
	Message        string    `json:"message"`
	CurrentVersion string    `json:"current_version"`
	CurrentCommit  string    `json:"current_commit,omitempty"`
	TargetVersion  string    `json:"target_version,omitempty"`
	TargetCommit   string    `json:"target_commit,omitempty"`
	StartedAt      time.Time `json:"started_at,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
	Active         bool      `json:"active"`
}

type Options struct {
	Dir, Version, Commit, HostID string
	Unavailable                  string
	Launch                       func(Status) error
}
type Manager struct {
	mu   sync.Mutex
	opts Options
}

func New(opts Options) *Manager { return &Manager{opts: opts} }
func (m *Manager) Get() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.get()
}
func (m *Manager) get() Status {
	s, err := read(m.opts.Dir)
	if err != nil {
		s = Status{Phase: "idle", Message: "Ready to check for a stable release."}
		if !errors.Is(err, os.ErrNotExist) {
			s.Phase = "interrupted"
			s.Message = "Update status could not be read. Check this computer before retrying."
			s.Active = true
			s.StartedAt = time.Now()
		}
	}
	s.Available = m.opts.Unavailable == ""
	s.CurrentVersion = m.opts.Version
	s.CurrentCommit = m.opts.Commit
	s.HostID = m.opts.HostID
	if !s.Available {
		s.Message = m.opts.Unavailable
		if s.Phase == "idle" {
			s.Phase = "unavailable"
		}
	}
	if s.Active && time.Since(s.StartedAt) > Window {
		s.Phase = "interrupted"
		s.Active = false
		s.Message = "Update was interrupted. Check nacred on this computer and its saved rollback before retrying."
	}
	return s
}
func (m *Manager) Start() (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.get()
	if !s.Available {
		return s, errors.New(s.Message)
	}
	if s.Active {
		if s.Phase == "interrupted" {
			return s, errors.New(s.Message)
		}
		return s, nil
	}
	if s.Phase == "interrupted" {
		return s, errors.New(s.Message)
	}
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return s, err
	}
	s = Status{Available: true, ID: hex.EncodeToString(b), HostID: m.opts.HostID, Phase: "checking", Message: "Checking the latest stable release…", CurrentVersion: m.opts.Version, CurrentCommit: m.opts.Commit, StartedAt: time.Now().UTC(), Active: true}
	if err := save(m.opts.Dir, &s); err != nil {
		return s, err
	}
	if m.opts.Launch == nil {
		s.Phase = "failed"
		s.Active = false
		s.Message = "Update helper unavailable."
		_ = save(m.opts.Dir, &s)
		return s, fmt.Errorf("update helper unavailable")
	}
	if err := m.opts.Launch(s); err != nil {
		s.Phase = "failed"
		s.Active = false
		s.Message = err.Error()
		_ = save(m.opts.Dir, &s)
		return s, err
	}
	return s, nil
}
func read(dir string) (Status, error) {
	var s Status
	b, e := os.ReadFile(filepath.Join(dir, "status.json"))
	if e == nil {
		e = json.Unmarshal(b, &s)
	}
	return s, e
}
func save(dir string, s *Status) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	s.UpdatedAt = time.Now().UTC()
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".status-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0o600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, "status.json"))
}
