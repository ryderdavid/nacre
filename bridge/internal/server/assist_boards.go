package server

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/attention"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
	"github.com/ryderdavid/nacre/bridge/internal/repos"
)

// maxAssistBoards bounds one call across boards; each board adds a symlink
// and a handful of permission patterns.
const maxAssistBoards = 200

// assistBoard is one board a call across boards reads: the repo id on this
// computer, and the name the app knows it by (which proposals use).
type assistBoard struct {
	Repo string `json:"repo"`
	Name string `json:"name"`
}

// unreadableBoard is a board that was asked for and could not be opened.
type unreadableBoard struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// linkedBoard is a board the call can read, under Dir in the working directory.
type linkedBoard struct {
	Dir  string
	Name string
	// Path is the repository the link points at.
	Path string
}

// assistBoardsStream runs one board-assistant turn across several boards on
// this computer: the assistant in the app's "All boards" view.
//
// claude -p needs one working directory, so each board is linked into
// Config.AssistBoardsDir and read with `bd -C <link> …`. The allowlist names
// those links one by one, so the call reads exactly the boards it was given
// and, as on the per-repo route, can change none of them: writes come back as
// proposals that the app applies once the user approves them.
//
// Boards that can't be opened here are named in the first "turn" message and
// in the prompt, so neither the app nor Claude passes over them in silence.
func (s *Server) assistBoardsStream(w http.ResponseWriter, r *http.Request) {
	var req assistRequest
	if err := decode(r, &req, false); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
		return
	}
	h, bad := req.headless()
	switch {
	case bad != "":
	case len(req.Boards) == 0:
		bad = "boards required"
	case len(req.Boards) > maxAssistBoards:
		bad = "too many boards"
	case len(req.Images) > 0:
		bad = "images are not accepted by the assistant across boards"
	}
	if bad != "" {
		writeError(w, http.StatusBadRequest, bad, "")
		return
	}
	dir := s.Config.AssistBoardsDir()
	linked, unreadable, err := s.linkBoards(dir, req.Boards)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "couldn't prepare the boards for Claude", err.Error())
		return
	}
	if len(linked) == 0 {
		names := make([]string, 0, len(unreadable))
		for _, u := range unreadable {
			names = append(names, u.Name+" ("+u.Reason+")")
		}
		writeError(w, http.StatusUnprocessableEntity, "none of those boards can be read on this computer: "+strings.Join(names, "; "), "")
		return
	}
	dirs := make([]string, len(linked))
	repos := make([]string, len(linked))
	for i, b := range linked {
		dirs[i], repos[i] = b.Dir, b.Path
	}
	// Each board's beads and its repository, read-only; nothing else on this computer.
	h.AllowedTools, h.ReadRepo, h.ReadSource = claude.BoardReadTools(dirs), true, false
	h.AddDirs = repos
	h.System = strings.TrimRight(h.System, "\n") + "\n\n" + boardsSection(linked, unreadable)
	if req.NeedsYou && s.Attention != nil {
		h.Prompt += attention.Context(s.Attention.Items(r.Context()))
	}
	turn := s.Assist.Start(dir, h)
	s.streamTurn(w, r, turn, 0, &assistTurn{TurnID: turn.ID, Unreadable: unreadable})
}

// linkBoards makes sure each readable board has a link in dir and returns
// them, with the boards it couldn't open and why.
//
// A link, once made, keeps its name and target, so a conversation's later
// turns (and any other conversation running at the same time) see the same
// names. Links whose board has gone are cleared out.
func (s *Server) linkBoards(dir string, boards []assistBoard) ([]linkedBoard, []unreadableBoard, error) {
	s.boardsMu.Lock()
	defer s.boardsMu.Unlock()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, err
	}
	// What is already linked, by target and by name.
	byTarget, taken := map[string]string{}, map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if e.Type()&os.ModeSymlink == 0 {
			taken[e.Name()] = true // not ours; leave it alone, and don't reuse its name
			continue
		}
		target, err := os.Readlink(p)
		if err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(target, ".beads")); err != nil {
			_ = os.Remove(p)
			continue
		}
		byTarget[target], taken[e.Name()] = e.Name(), true
	}

	var linked []linkedBoard
	var unreadable []unreadableBoard
	seen := map[string]bool{}
	for _, b := range boards {
		name := boardLabel(b.Name, b.Repo)
		path, err := s.Repos.Path(b.Repo)
		if err != nil {
			reason := "not found on this computer"
			if !errors.Is(err, repos.ErrNotFound) {
				reason = err.Error()
			}
			unreadable = append(unreadable, unreadableBoard{name, reason})
			continue
		}
		if st, err := os.Stat(filepath.Join(path, ".beads")); err != nil || !st.IsDir() {
			unreadable = append(unreadable, unreadableBoard{name, "its .beads folder is missing at " + path})
			continue
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		link, ok := byTarget[path]
		if !ok {
			link = freeLinkName(slug(name), taken)
			if err := os.Symlink(path, filepath.Join(dir, link)); err != nil {
				unreadable = append(unreadable, unreadableBoard{name, "couldn't link it for Claude: " + err.Error()})
				continue
			}
			byTarget[path], taken[link] = link, true
		}
		linked = append(linked, linkedBoard{Dir: link, Name: name, Path: path})
	}
	return linked, unreadable, nil
}

var slugJunk = regexp.MustCompile(`[^a-z0-9._-]+`)

// slug makes a board name into a link name that needs no quoting in a shell
// command or a permission pattern.
func slug(name string) string {
	s := strings.Trim(slugJunk.ReplaceAllString(strings.ToLower(name), "-"), "-.")
	if len(s) > 40 {
		s = strings.TrimRight(s[:40], "-.")
	}
	if s == "" {
		s = "board"
	}
	return s
}

// freeLinkName is base, or base-2, base-3… if that name is taken.
func freeLinkName(base string, taken map[string]bool) string {
	if !taken[base] {
		return base
	}
	for i := 2; ; i++ {
		if n := base + "-" + strconv.Itoa(i); !taken[n] {
			return n
		}
	}
}

// boardLabel is a board's name made safe to put on one line of a prompt.
func boardLabel(name, repo string) string {
	n := strings.Join(strings.Fields(name), " ")
	if n == "" {
		n = repo
	}
	return claude.Truncate(n, 100)
}

// boardsSection tells Claude which boards it can read and how, and which it
// was asked about but cannot.
func boardsSection(linked []linkedBoard, unreadable []unreadableBoard) string {
	var b strings.Builder
	b.WriteString("## The boards on this computer\n\n")
	b.WriteString("Each board is a directory where you are running. Read one with `bd -C <directory> <command>`, for example `bd -C ")
	b.WriteString(linked[0].Dir)
	b.WriteString(" ready --json`. Plain `bd` without `-C` reads no board. Only these commands work: ")
	b.WriteString("`" + strings.Join(claude.BdReadCommandNames(), "`, `") + "`.")
	b.WriteString(" Each board's code is readable too: its repository is at the path listed, for the Read, Grep and Glob tools, and `git -C <directory> log|diff|show|status|branch` reads its history.")
	b.WriteString(" Name a board in a proposal by its board name, not its directory.\n\n")
	for _, l := range linked {
		fmt.Fprintf(&b, "- `%s`: %s, repository at %s\n", l.Dir, l.Name, l.Path)
	}
	if len(unreadable) > 0 {
		b.WriteString("\nThese boards were asked for but can't be read on this computer. Tell Ryder they weren't read; never answer as if you had covered them:\n\n")
		for _, u := range unreadable {
			fmt.Fprintf(&b, "- %s: %s\n", u.Name, u.Reason)
		}
	}
	return b.String()
}
