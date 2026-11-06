package chat

import "strings"

// lastLineLen bounds a chat's last line.
const lastLineLen = 160

// lastLineOf is the line a chat list shows for an event: the first line of
// a message from Ryder or the agent; "" for anything else.
func lastLineOf(ev Event) string {
	if ev.Kind != KindUser && ev.Kind != "text" {
		return ""
	}
	line := strings.TrimSpace(ev.Text)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	if r := []rune(line); len(r) > lastLineLen {
		line = string(r[:lastLineLen-1]) + "…"
	}
	return line
}

// Listing is a chat as chat lists show it: its record, how many permission
// requests wait on Ryder, and (in LastLine) its newest message's first line.
type Listing struct {
	Record
	// Waiting counts its pending permission requests; nacred adds the
	// chat's waiting decisions.
	Waiting int `json:"waiting"`
}

// Listings returns the chats, most recently updated first, those in repo
// only when repo is set.
func (m *Manager) Listings(repo string) []Listing {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Listing{}
	for _, c := range m.chats {
		if repo != "" && (c.rec.Repo == nil || *c.rec.Repo != repo) {
			continue
		}
		if c.rec.LastLine == "" && c.rec.LastSeq > 0 {
			// A chat from before last lines were kept.
			m.loadLocked(c)
			for i := len(c.events) - 1; i >= 0; i-- {
				if line := lastLineOf(c.events[i]); line != "" {
					c.rec.LastLine = line
					break
				}
			}
		}
		out = append(out, Listing{Record: c.rec, Waiting: len(c.pending)})
	}
	recs := make([]Record, len(out))
	for i := range out {
		recs[i] = out[i].Record
	}
	sortNewest(recs)
	byID := map[string]Listing{}
	for _, l := range out {
		byID[l.ID] = l
	}
	for i, r := range recs {
		out[i] = byID[r.ID]
	}
	return out
}
