package chat

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// ReadOnlyTools is what a light or drafting turn may use without asking. It
// runs restricted ([RestrictedTools]) in default permission mode, so the
// owner's own allow rules don't apply and the CLI asks nacred about every
// other step. Each Bash rule is a command with no option that writes a file:
// commands like `git diff --output=f` or `rg --pre=cmd` are left out, since a
// prefix rule approves any flag. A shell redirect is checked as a write
// (testdata/default_allowlist_drafting.jsonl), so `ls > f` still reaches
// nacred.
var ReadOnlyTools = []string{
	"Read", "Grep", "Glob", "WebFetch", "WebSearch",
	"Bash(bd show:*)", "Bash(bd list:*)", "Bash(bd ready:*)", "Bash(bd search:*)", "Bash(bd dep tree:*)",
	"Bash(openspec list:*)", "Bash(openspec show:*)", "Bash(openspec validate:*)", "Bash(openspec status:*)",
	"Bash(git status:*)", "Bash(ls:*)",
}

// RestrictedTools are the built-in tools a restricted light or drafting turn
// keeps: reading, Bash for ReadOnlyTools, and the file tools nacred allows
// inside the mockups directory only.
var RestrictedTools = []string{"Bash", "Read", "Grep", "Glob", "Write", "Edit", "MultiEdit", "WebFetch", "WebSearch"}

// DraftingReminder is what a drafting turn is told when nacred refuses a
// step: drafting changes nothing.
const DraftingReminder = "Still drafting: nothing changes until Ryder raises intake on the draft. " +
	"Revise the draft instead, or write a mockup into $" + EnvMockups + "."

// mockupTools are the tools whose file_path may be allowed inside the chat's
// mockups directory.
var mockupTools = map[string]bool{"Write": true, "Edit": true, "MultiEdit": true}

// mockupWrite reports whether req writes a file inside dir, the chat's
// mockups directory: a Write, Edit or MultiEdit whose file_path, with
// symlinks resolved, lies below dir. A path with ".." in it, a relative path
// or a symlink leading out is refused.
func mockupWrite(req claude.ControlRequest, dir string) bool {
	if !mockupTools[req.ToolName] || dir == "" {
		return false
	}
	var in struct {
		FilePath string `json:"file_path"`
	}
	if json.Unmarshal(req.Input, &in) != nil {
		return false
	}
	return insideDir(dir, in.FilePath)
}

// insideDir reports whether path names a file below dir once symlinks are
// resolved. The file itself may not exist yet; its parent must.
func insideDir(dir, path string) bool {
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return false
		}
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(filepath.Clean(path)))
	if err != nil {
		return false
	}
	full := filepath.Join(parent, filepath.Base(path))
	if fi, err := os.Lstat(full); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return false
	}
	return strings.HasPrefix(full, root+string(filepath.Separator))
}

// EndDrafting ends a New effort chat's drafting: its later messages are
// routed like any other chat's. Called when Ryder answers the chat's intake
// card with an option that creates a bead or develops a change.
func (m *Manager) EndDrafting(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.chats[id]
	if c == nil || !c.rec.Drafting {
		return
	}
	c.rec.Drafting = false
	m.appendLocked(c, Event{Kind: KindSystem, Text: "Filed. This chat is an ordinary chat again: what you ask here can change things"})
	m.touchLocked(c)
}

// SetIntakeRaiser sets how a drafting turn's go-ahead raises intake on the
// chat's newest draft: the server's own path for the Draft card's button.
func (m *Manager) SetIntakeRaiser(fn func(chatID string, seq int64) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.intakeRaiser = fn
}

// raiseOnGoAhead raises intake on the chat's newest draft after a drafting
// turn ended with [IntakeMarker], or says in the chat why it couldn't.
func (m *Manager) raiseOnGoAhead(c *chatState, turnID string) {
	m.mu.Lock()
	raise := m.intakeRaiser
	var seq int64
	for _, ev := range c.events {
		if ev.Kind == KindDraft && ev.Draft != nil {
			seq = ev.Seq
		}
	}
	id := c.rec.ID
	m.mu.Unlock()
	var err error
	switch {
	case seq == 0:
		err = errors.New("there's no draft yet")
	case raise == nil:
		err = errors.New("intake isn't available here")
	default:
		err = raise(id, seq)
	}
	// A raise already running on that draft (a tap on the card) brings its own card.
	if err != nil && !strings.Contains(err.Error(), "already running") {
		m.emit(c, Event{Kind: KindError, TurnID: turnID, Text: "Couldn't ask where it belongs: " + err.Error()})
	}
}

// ErrDraftGone is returned for a draft a chat doesn't hold.
var ErrDraftGone = errors.New("that draft is gone; ask the chat for a new one")

// DraftAt is the draft the chat recorded as event seq, the one a Draft card
// showed, or ErrDraftGone when the chat has no such draft event.
func (m *Manager) DraftAt(id string, seq int64) (Draft, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.chats[id]
	if c == nil {
		return Draft{}, ErrNotFound
	}
	m.loadLocked(c)
	for _, ev := range c.events {
		if ev.Seq == seq && ev.Kind == KindDraft && ev.Draft != nil {
			return *ev.Draft, nil
		}
	}
	return Draft{}, ErrDraftGone
}

// splitIntakeMarker removes a final [IntakeMarker] line from a drafting
// reply, reporting whether it was there.
func splitIntakeMarker(text string) (string, bool) {
	trimmed := strings.TrimRight(text, " \t\r\n")
	i := strings.LastIndexByte(trimmed, '\n')
	if strings.TrimSpace(trimmed[i+1:]) != IntakeMarker {
		return text, false
	}
	if i < 0 {
		return "", true
	}
	return strings.TrimRight(trimmed[:i], " \t\r\n"), true
}
