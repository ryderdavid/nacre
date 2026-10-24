package sessions

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/claude"
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

// scanner folds the bd commands seen in each session's transcript into the
// strongest mention so far. It reads no files: the session's
// claude.TranscriptTail hands it every parsed entry through
// TranscriptOptions.Observe, so each transcript is read once per poll.
type scanner struct {
	state map[string]*scanState // by session id
}

type scanState struct {
	best mention
	// pending are the mentions observed since the last call to best, oldest
	// first. They are validated only then, against that poll's bd list, and
	// are kept while no bd list is available to validate them.
	pending []mention
}

func newScanner() *scanner { return &scanner{state: map[string]*scanState{}} }

// forget drops a session's scan state, freeing it once the session is gone.
func (s *scanner) forget(sessionID string) { delete(s.state, sessionID) }

func (s *scanner) get(sessionID string) *scanState {
	st := s.state[sessionID]
	if st == nil {
		st = &scanState{}
		s.state[sessionID] = st
	}
	return st
}

// observe records the bd mentions of one transcript entry.
func (s *scanner) observe(sessionID string, p claude.TranscriptParsed) {
	if ms := bdMentions(p); len(ms) > 0 {
		st := s.get(sessionID)
		st.pending = append(st.pending, ms...)
	}
}

// mark returns a position in sessionID's pending mentions, for restart.
func (s *scanner) mark(sessionID string) int {
	if st := s.state[sessionID]; st != nil {
		return len(st.pending)
	}
	return 0
}

// restart forgets everything observed before mark: the transcript was
// truncated or replaced and has been read again from its beginning.
func (s *scanner) restart(sessionID string, mark int) {
	if st := s.state[sessionID]; st != nil {
		st.best = mention{}
		st.pending = st.pending[min(mark, len(st.pending)):]
	}
}

// best folds the pending mentions into the session's strongest one and
// returns it. A later mention of at least the current weight replaces it.
// valid rejects ids that are not beads of the session's repo; when it is nil
// every syntactically valid id is accepted.
func (s *scanner) best(sessionID string, valid func(string) bool) (string, bool) {
	st := s.state[sessionID]
	if st == nil {
		return "", false
	}
	for _, m := range st.pending {
		if m.weight < st.best.weight {
			continue
		}
		if valid != nil && !valid(m.id) {
			continue
		}
		st.best = m
	}
	st.pending = nil
	return st.best.id, st.best.id != ""
}

// bdMentions returns the bead ids mentioned by bd commands in one parsed
// transcript entry: the Bash tool calls of an assistant turn. Subagent turns
// (sidechain) are ignored: they are a different agent working on something
// else in the same file.
func bdMentions(p claude.TranscriptParsed) []mention {
	if !p.Assistant || p.Sidechain {
		return nil
	}
	var out []mention
	for _, ev := range p.Events {
		if ev.Kind != api.KindToolUse || ev.Tool != "Bash" || !bytes.Contains(ev.Input, []byte("bd")) {
			continue
		}
		var in struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(ev.Input, &in) != nil || in.Command == "" {
			continue
		}
		out = append(out, commandMentions(in.Command)...)
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
