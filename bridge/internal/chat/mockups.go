package chat

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Mockups: a chat's agent shows Ryder a mockup by writing a self-contained
// HTML page or an image into the chat's mockups directory and putting
// "MEDIA:<path>" alone on a line of its reply. The app fetches it with
// GET /v1/chats/{id}/mockups/{name} and renders it inline: HTML in a
// sandboxed web view, images as pictures, each with a pinch-zoom full
// screen. Nothing outside that directory is ever served.

// EnvMockups names the chat's mockups directory in a turn's environment.
const EnvMockups = "NACRE_MOCKUPS_DIR"

// MaxMockupBytes bounds one mockup file.
const MaxMockupBytes = 5 << 20

// ErrNoMockup is an unknown, unservable or oversized mockup.
var ErrNoMockup = errors.New("unknown mockup")

var mockupNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,120}$`)

var mockupMedia = map[string]string{
	"html": "text/html; charset=utf-8", "htm": "text/html; charset=utf-8",
	"png": "image/png", "jpg": "image/jpeg", "jpeg": "image/jpeg", "webp": "image/webp", "gif": "image/gif",
	"svg": "image/svg+xml",
}

// MockupsDir is chats/<id>/mockups.
func (m *Manager) MockupsDir(id string) string {
	return filepath.Join(m.opts.Dir, id, "mockups")
}

// ensureMockupsDir makes the chat's mockups directory, private to the owner.
func (m *Manager) ensureMockupsDir(id string) string {
	dir := m.MockupsDir(id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		m.opts.Logger.Printf("chat %s: mockups dir: %v", id, err)
	}
	return dir
}

// Mockup returns the path and media type of one of a chat's mockups.
func (m *Manager) Mockup(id, name string) (path, mediaType string, err error) {
	if !mockupNameRe.MatchString(name) || strings.Contains(name, "..") {
		return "", "", ErrNoMockup
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	mediaType, ok := mockupMedia[ext]
	if !ok {
		return "", "", ErrNoMockup
	}
	m.mu.Lock()
	c := m.chats[id]
	m.mu.Unlock()
	if c == nil {
		return "", "", ErrNotFound
	}
	path = filepath.Join(m.MockupsDir(id), name)
	// Lstat: a symlink planted in the directory doesn't reach outside it.
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Size() > MaxMockupBytes {
		return "", "", ErrNoMockup
	}
	return path, mediaType, nil
}
