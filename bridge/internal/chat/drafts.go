package chat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// KindDraft is a New effort draft the agent wrote as a fenced nacre-draft
// block. The app shows the newest as a Draft card with Continue.
const KindDraft = "draft"

// DraftUnreadable starts the error event for a block that can't be used.
const DraftUnreadable = "The draft couldn't be read: "

// draftFence is the info string of a draft block.
const draftFence = "nacre-draft"

// Draft holds the fields of the New effort sheet's draft.
type Draft struct {
	Title       string `json:"title"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	Design      string `json:"design,omitempty"`
	Acceptance  string `json:"acceptance,omitempty"`
}

// DraftingPrompt goes before a New effort chat's first message.
func DraftingPrompt(board string) string {
	if board == "" {
		board = "this board"
	}
	return "New effort on " + board + ". Talk the effort through with Ryder: ask what you need to understand it, and read the repository, its beads and its OpenSpec changes if that helps. Change nothing while drafting: no edits, no beads, no proposals, no rounds. " +
		"When you have a draft, or Ryder asks for one, present it as one fenced code block whose language is `nacre-draft`, holding JSON with \"title\" (required), \"type\" (task, bug, feature, chore or epic), \"description\", and \"design\" and \"acceptance\" when there is something to say. " +
		"Nacre shows it as a Draft card; Continue takes it into the New effort sheet, where intake decides whether it is a bead, a new change or an amendment. Write a new block whenever the draft changes."
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
	}
	if d.Title == "" {
		return Draft{}, errors.New("it has no title")
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
func openFence(line string) (string, bool) {
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
		if f := strings.Fields(info); len(f) > 0 && strings.EqualFold(f[0], draftFence) {
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
