package chat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// KindDraft is a New effort draft the agent wrote as a fenced nacre-draft
// block. The app shows the newest as a Draft card with Continue.
const KindDraft = "draft"

// DraftUnreadable starts the error event for a block that can't be used.
const DraftUnreadable = "The draft couldn't be read: "

// draftFence is the info string of a draft block.
const draftFence = "nacre-draft"

// beadID is the shape of a bead id a draft may name.
var beadID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*-[A-Za-z0-9.]+$`)

// IntakeMarker is the line a drafting turn ends its reply with when Ryder
// says to go ahead: drafting can't call nacred, so nacred raises intake on
// the chat's newest draft itself.
const IntakeMarker = "NACRE_INTAKE"

// DraftingTurnNote goes before every drafting message after the first.
const DraftingTurnNote = "Still drafting: you can read and write mockups, and nacred refuses anything else, including calling nacred or raising decisions. " +
	"Keep the nacre-draft block current. When Ryder says to go ahead, end your reply with the line `" + IntakeMarker + "` alone and nacred raises intake on your newest draft."

// MaxDigest bounds a draft's brainstorm digest.
const MaxDigest = 2048

// Draft holds the fields of a New effort draft. Digest is the brainstorm so
// far (decided, ruled out, open questions, agreed mockups), one point per
// line; it goes into the notes of the bead intake creates.
type Draft struct {
	Title       string `json:"title"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	Design      string `json:"design,omitempty"`
	Acceptance  string `json:"acceptance,omitempty"`
	Digest      string `json:"digest,omitempty"`
	// Bead is the bead the effort already has (found work filed out of
	// scope); intake raised from the draft is for that bead.
	Bead string `json:"bead,omitempty"`
}

// DraftingPrompt goes before a New effort chat's first message.
func DraftingPrompt(board string) string {
	if board == "" {
		board = "this board"
	}
	return "New effort on " + board + ". You are drafting: you can read the repository, its beads and its OpenSpec changes, and write mockups into $" + EnvMockups + ", and nacred refuses anything else until Ryder raises intake. " +
		"Before your first draft, say in two or three short lines what you think Ryder wants, name the assumptions you're making, and ask at most three numbered questions so he can answer \"2: per chat\". Write no draft until he has answered or asks for one. " +
		"When you have a draft, present it as one fenced code block whose language is `nacre-draft`, holding JSON with \"title\" (required), \"type\" (task, bug, feature, chore or epic), \"description\", \"design\" and \"acceptance\" when there is something to say, " +
		"and \"digest\": a short list of what the brainstorm settled, one point per line starting \"Decided:\", \"Ruled out:\", \"Open:\" or \"Mockup:\". The digest goes into the bead's notes, so the proposal's author sees it. " +
		"Write a new block whenever the draft changes, and start that reply with one line, \"Changed: …\", naming what changed. " +
		"Nacre shows the newest block as a Draft card. Its \"Where does this belong?\" button raises intake on that exact draft, which decides whether it is a bead, a new change or an amendment. " +
		"When Ryder says to go ahead, make sure your newest draft is up to date and end that reply with the line `" + IntakeMarker + "` alone: nacred raises intake on that draft (you can't call nacred while drafting). Otherwise tell him the button takes it on. " +
		"If the effort already has a bead (found work filed out of scope), put its id in the draft's \"bead\" field."
}

// draftText is a JSON value that should be text: a string, or a list of
// strings (joined one per line), or null.
type draftText string

func (d *draftText) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		*d = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*d = draftText(s)
		return nil
	}
	var list []string
	if err := json.Unmarshal(b, &list); err == nil {
		lines := make([]string, 0, len(list))
		for _, l := range list {
			if l = strings.TrimSpace(l); l != "" {
				lines = append(lines, "- "+l)
			}
		}
		*d = draftText(strings.Join(lines, "\n"))
		return nil
	}
	return errors.New("expected text")
}

// parseDraft reads one block's JSON.
func parseDraft(body string) (Draft, error) {
	var raw struct {
		Title       draftText `json:"title"`
		Type        draftText `json:"type"`
		Description draftText `json:"description"`
		Design      draftText `json:"design"`
		Acceptance  draftText `json:"acceptance"`
		Digest      draftText `json:"digest"`
		Bead        draftText `json:"bead"`
	}
	if strings.TrimSpace(body) == "" {
		return Draft{}, errors.New("the block is empty")
	}
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return Draft{}, fmt.Errorf("it isn't valid JSON (%v)", err)
	}
	d := Draft{
		Title:       strings.TrimSpace(string(raw.Title)),
		Type:        strings.ToLower(strings.TrimSpace(string(raw.Type))),
		Description: strings.TrimSpace(string(raw.Description)),
		Design:      strings.TrimSpace(string(raw.Design)),
		Acceptance:  strings.TrimSpace(string(raw.Acceptance)),
		Digest:      strings.TrimSpace(string(raw.Digest)),
		Bead:        strings.TrimSpace(string(raw.Bead)),
	}
	if d.Title == "" {
		return Draft{}, errors.New("it has no title")
	}
	if d.Bead != "" && !beadID.MatchString(d.Bead) {
		return Draft{}, fmt.Errorf("its bead %q isn't a bead id", d.Bead)
	}
	if len(d.Digest) > MaxDigest {
		return Draft{}, fmt.Errorf("its digest is longer than %d bytes; keep it to the main points", MaxDigest)
	}
	return d, nil
}

// DraftResult is one nacre-draft block found in assistant text: a draft,
// or why it couldn't be read.
type DraftResult struct {
	Draft *Draft
	Err   error
}

// ExtractDrafts removes every fenced nacre-draft block from text and
// returns the text left to show and what each block held, in order. A block
// without a closing fence runs to the end of the text.
func ExtractDrafts(text string) (rest string, found []DraftResult) {
	if !strings.Contains(text, draftFence) {
		return text, nil
	}
	lines := strings.SplitAfter(text, "\n")
	var out strings.Builder
	for i := 0; i < len(lines); i++ {
		fence, ok := openFence(lines[i])
		if !ok {
			out.WriteString(lines[i])
			continue
		}
		var body strings.Builder
		closed := false
		j := i + 1
		for ; j < len(lines); j++ {
			if closesFence(lines[j], fence) {
				closed = true
				break
			}
			body.WriteString(lines[j])
		}
		if !closed {
			found = append(found, DraftResult{Err: errors.New("the block isn't closed")})
			break
		}
		d, err := parseDraft(body.String())
		if err != nil {
			found = append(found, DraftResult{Err: err})
		} else {
			found = append(found, DraftResult{Draft: &d})
		}
		i = j
	}
	if len(found) == 0 {
		return text, nil
	}
	return tidy(out.String()), found
}

// openFence reports whether line opens a nacre-draft block, and its fence.
func openFence(line string) (string, bool) { return openFenceFor(line, draftFence) }

// openFenceFor reports whether line opens a fenced block whose language is
// lang, and its fence.
func openFenceFor(line, lang string) (string, bool) {
	s := strings.TrimSpace(line)
	for _, ch := range []string{"`", "~"} {
		n := 0
		for n < len(s) && s[n:n+1] == ch {
			n++
		}
		if n < 3 {
			continue
		}
		info := strings.TrimSpace(s[n:])
		if f := strings.Fields(info); len(f) > 0 && strings.EqualFold(f[0], lang) {
			return s[:n], true
		}
	}
	return "", false
}

// closesFence reports whether line closes a block opened with fence.
func closesFence(line, fence string) bool {
	s := strings.TrimSpace(line)
	if len(s) < len(fence) || !strings.HasPrefix(s, fence) {
		return false
	}
	return strings.Trim(s, fence[:1]) == ""
}

// tidy collapses the blank lines a removed block leaves behind.
func tidy(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(s)
}

// draftEvents are the events for the blocks found in one assistant text.
func draftEvents(found []DraftResult, turnID, route string) []Event {
	var evs []Event
	for _, f := range found {
		if f.Err != nil {
			evs = append(evs, Event{Kind: KindError, TurnID: turnID, Text: DraftUnreadable + f.Err.Error()})
			continue
		}
		d := *f.Draft
		evs = append(evs, Event{Kind: KindDraft, TurnID: turnID, Route: route, Text: d.Title, Draft: &d})
	}
	return evs
}
