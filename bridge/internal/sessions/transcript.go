package sessions

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Limits on transcript scanning. A transcript grows for as long as its session
// lives, so each pass reads only the bytes added since the previous one, and
// stops after maxScanBytes to keep one poll bounded; the rest is read next
// poll.
const (
	maxScanBytes = 8 << 20 // per scan pass
	maxScanLine  = 1 << 20 // a longer entry cannot be a Bash tool call we care about
	scanBufSize  = 256 << 10
)

// Weights of the bd commands that link a session to a bead, strongest last. A
// later command of at least the current weight replaces the previous one, so
// the most recent strong signal wins.
const (
	weightRead  = 1 // `bd show`/`bd comments`: the agent read that bead
	weightClose = 2 // `bd close`: the agent finished that bead
	weightClaim = 3 // `bd update … --claim`: the agent took that bead
)

// sessionIDRe matches the uuid Claude Code uses for a session id. The id is
// interpolated into a glob, so anything else is refused.
var sessionIDRe = regexp.MustCompile(`^[0-9a-fA-F-]{8,64}$`)

// beadIDRe matches the shape of a bead id ("NAC-j5z", "NAC-j5z.1"). It is
// deliberately loose: every candidate is validated against the repo's own
// issue list before it is trusted.
var beadIDRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*-[A-Za-z0-9]+(?:\.[A-Za-z0-9]+)*$`)

// activeStatuses are the bd statuses that mean somebody is working on the
// bead: both the `bd update --status` values that take it on and the states a
// claimed bead sits in afterwards.
var activeStatuses = map[string]bool{"in_progress": true, "hooked": true}

// transcriptPath locates a session's transcript under projects (normally
// ~/.claude/projects). The directory name is an escaped form of the session's
// working directory and that escaping is lossy, so the session id is globbed
// for instead of recomputed from the cwd.
func transcriptPath(projects, sessionID string) string {
	if projects == "" || !sessionIDRe.MatchString(sessionID) {
		return ""
	}
	matches, err := filepath.Glob(filepath.Join(projects, "*", sessionID+".jsonl"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	// A session that moved between worktrees can leave more than one file;
	// the largest is the live one.
	best, bestSize := "", int64(-1)
	for _, m := range matches {
		fi, err := os.Stat(m)
		if err != nil {
			continue
		}
		if fi.Size() > bestSize {
			best, bestSize = m, fi.Size()
		}
	}
	return best
}

// mention is a bead id found in a bd command, with the weight of the command
// that mentioned it.
type mention struct {
	id     string
	weight int
}

// scanner remembers how far each session's transcript has been read, so a
// growing transcript is scanned once rather than once per poll.
type scanner struct {
	state map[string]*scanState // by session id
}

type scanState struct {
	path   string
	offset int64 // bytes consumed, always at an entry boundary
	best   mention
}

func newScanner() *scanner { return &scanner{state: map[string]*scanState{}} }

// forget drops a session's scan position, freeing it once the session is gone.
func (s *scanner) forget(sessionID string) { delete(s.state, sessionID) }

// scan reads the part of path that has appeared since the last call and
// returns the strongest bd mention seen in the transcript so far. valid
// rejects ids that are not beads of the session's repo; when it is nil every
// syntactically valid id is accepted.
func (s *scanner) scan(sessionID, path string, valid func(string) bool) (string, bool) {
	if path == "" {
		return "", false
	}
	st := s.state[sessionID]
	if st == nil || st.path != path {
		st = &scanState{path: path}
		s.state[sessionID] = st
	}
	f, err := os.Open(path)
	if err != nil {
		return st.best.id, st.best.id != ""
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil && fi.Size() < st.offset {
		st.offset, st.best = 0, mention{} // truncated or replaced
	}
	if _, err := f.Seek(st.offset, io.SeekStart); err != nil {
		return st.best.id, st.best.id != ""
	}
	rd := bufio.NewReaderSize(f, scanBufSize)
	read := int64(0)
	for read < maxScanBytes {
		line, err := rd.ReadBytes('\n')
		if err != nil {
			break // partial final line: leave the offset before it
		}
		read += int64(len(line))
		st.offset += int64(len(line))
		if len(line) > maxScanLine {
			continue
		}
		for _, m := range bdMentions(line) {
			if m.weight < st.best.weight {
				continue
			}
			if valid != nil && !valid(m.id) {
				continue
			}
			st.best = m
		}
	}
	return st.best.id, st.best.id != ""
}

// transcriptEntry is the fragment of a transcript entry the scan needs. The
// on-disk format carries many entry types; the only interesting ones are
// assistant turns whose message content holds a Bash tool call.
type transcriptEntry struct {
	Type        string `json:"type"`
	IsSidechain bool   `json:"isSidechain"`
	Message     struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type contentBlock struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Input struct {
		Command string `json:"command"`
	} `json:"input"`
}

// bdMentions returns the bead ids mentioned by bd commands in one transcript
// entry. Subagent turns (isSidechain) are ignored: they are a different agent
// working on something else in the same file.
func bdMentions(line []byte) []mention {
	if !bytes.Contains(line, []byte("bd ")) {
		return nil
	}
	var ent transcriptEntry
	if json.Unmarshal(line, &ent) != nil || ent.IsSidechain || ent.Type != "assistant" {
		return nil
	}
	var blocks []contentBlock
	if json.Unmarshal(ent.Message.Content, &blocks) != nil {
		return nil // a plain-string content block carries no tool call
	}
	var out []mention
	for _, b := range blocks {
		if b.Type != "tool_use" || b.Name != "Bash" || b.Input.Command == "" {
			continue
		}
		out = append(out, commandMentions(b.Input.Command)...)
	}
	return out
}

// commandMentions reads the bd invocations out of one shell command line.
func commandMentions(cmd string) []mention {
	var out []mention
	for _, seg := range splitShell(cmd) {
		if m, ok := mentionFrom(strings.Fields(seg)); ok {
			out = append(out, m)
		}
	}
	return out
}

// splitShell cuts a command line at the separators that start a new command,
// so `cd x && bd update Y --claim` yields the bd invocation on its own. It is
// not a shell parser: a separator inside quotes splits too, which can only
// lose a mention, never invent one.
func splitShell(cmd string) []string {
	return strings.FieldsFunc(cmd, func(r rune) bool {
		return r == ';' || r == '&' || r == '|' || r == '\n' || r == '(' || r == ')'
	})
}

// mentionFrom classifies one bd invocation. Only the commands that say
// something about which bead the agent is working on produce a mention.
func mentionFrom(fields []string) (mention, bool) {
	if len(fields) < 2 || filepath.Base(fields[0]) != "bd" {
		return mention{}, false
	}
	args := fields[1:]
	// Global flags precede the subcommand; -C takes a separate value.
	sub := ""
	for len(args) > 0 {
		switch {
		case args[0] == "-C" || args[0] == "--dir":
			args = args[min(2, len(args)):]
		case strings.HasPrefix(args[0], "-"):
			args = args[1:]
		default:
			sub, args = args[0], args[1:]
		}
		if sub != "" {
			break
		}
	}

	weight := 0
	switch sub {
	case "show", "comments":
		weight = weightRead
	case "close":
		weight = weightClose
	case "update":
		// Only a write that takes the bead on counts; `bd update --notes` says
		// nothing about who is working on what.
		for i, a := range args {
			switch {
			case a == "--claim":
				weight = weightClaim
			case strings.HasPrefix(a, "--status="):
				if activeStatuses[strings.TrimPrefix(a, "--status=")] {
					weight = weightClaim
				}
			case a == "--status" && i+1 < len(args):
				if activeStatuses[args[i+1]] {
					weight = weightClaim
				}
			}
		}
	}
	if weight == 0 {
		return mention{}, false
	}
	for _, a := range args {
		if !strings.HasPrefix(a, "-") && beadIDRe.MatchString(a) {
			return mention{id: a, weight: weight}, true
		}
	}
	return mention{}, false
}
