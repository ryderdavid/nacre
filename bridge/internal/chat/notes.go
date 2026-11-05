package chat

import (
	"path/filepath"
	"strings"
)

// maxSummary bounds a context's summary, as the app does.
const maxSummary = 4096

// allBoards are the screens a message about every board is sent from.
var allBoards = map[string]bool{"boards": true, "inbox": true, "activity": true, "": true}

// screenNames are how the note names each screen.
var screenNames = map[string]string{
	"boards": "Boards", "inbox": "Inbox", "activity": "Activity",
	"board": "board", "bead": "bead", "change": "change", "round": "round", "run": "run",
}

// sameContext reports whether two contexts name the same screen in the same
// state, so the second message needs only the one-line reminder.
func sameContext(a, b *Context) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// Label is the short name of what a context is about, as the composer's
// About chip shows it: a bead id, "round r-12 · NAC-3", or "all boards".
func Label(c *Context, repoName func(string) string) string {
	if c == nil || allBoards[c.Screen] {
		return "all boards"
	}
	var s string
	switch c.Screen {
	case "bead":
		s = c.BeadID
	case "change":
		s = "change " + c.ChangeID
	case "round":
		s = "round " + c.RoundID
	case "run":
		s = "run " + c.RunID
	}
	if s == "" || strings.HasSuffix(s, " ") {
		s = repoLabel(c, repoName)
	}
	if c.Selection != "" && c.Screen != "bead" {
		s += " · " + c.Selection
	}
	return s
}

func repoLabel(c *Context, repoName func(string) string) string {
	if repoName != nil && c.Elsewhere == "" {
		if n := repoName(c.Repo); n != "" {
			return n
		}
	}
	if c.Repo != "" {
		return c.Repo
	}
	return "this board"
}

// Note is the "On screen" note put before a user message. It is the full
// context when ctx differs from prev, and the one-line reminder otherwise.
// repoPath resolves a repository key to its checkout on this computer.
func Note(ctx, prev *Context, host string, repoPath func(string) (string, error)) string {
	name := func(key string) string {
		if repoPath == nil {
			return ""
		}
		if p, err := repoPath(key); err == nil && p != "" {
			return filepath.Base(p)
		}
		return ""
	}
	if prev != nil && sameContext(ctx, prev) {
		return "On screen: same as before (" + Label(ctx, name) + ")"
	}
	if host == "" {
		host = "this computer"
	}
	if ctx == nil || allBoards[ctx.Screen] {
		screen := "the app"
		if ctx != nil && ctx.Screen != "" {
			screen = screenNames[ctx.Screen]
		}
		var b strings.Builder
		b.WriteString("On screen: " + screen + ", across all boards on this computer (" + host + "). This message is about all boards on this computer, not one repository.")
		if ctx != nil {
			writeDetails(&b, ctx)
		}
		return b.String()
	}
	var b strings.Builder
	b.WriteString("On screen: " + screenNames[ctx.Screen])
	switch ctx.Screen {
	case "bead":
		b.WriteString(" " + ctx.BeadID)
	case "change":
		b.WriteString(" " + ctx.ChangeID)
	case "round":
		b.WriteString(" " + ctx.RoundID)
	case "run":
		b.WriteString(" " + ctx.RunID)
	}
	b.WriteString(". \"This\" means what is named here.")
	if ctx.Repo != "" {
		if ctx.Elsewhere != "" {
			b.WriteString("\nRepository: " + ctx.Repo + " is on " + ctx.Elsewhere + ", not on this computer (" + host + "). It has no checkout here; work in it needs a chat on " + ctx.Elsewhere + ".")
		} else {
			line := "\nRepository: " + ctx.Repo
			if repoPath != nil {
				if p, err := repoPath(ctx.Repo); err == nil && p != "" {
					line += " (checkout " + p + ")"
				}
			}
			b.WriteString(line)
		}
	}
	ids := []struct{ k, v string }{
		{"Bead", ctx.BeadID}, {"Change", ctx.ChangeID}, {"Round", ctx.RoundID}, {"Run", ctx.RunID},
	}
	for _, id := range ids {
		if id.v != "" {
			b.WriteString("\n" + id.k + ": " + id.v)
		}
	}
	writeDetails(&b, ctx)
	return b.String()
}

func writeDetails(b *strings.Builder, ctx *Context) {
	if ctx.Selection != "" {
		b.WriteString("\nSelected: " + ctx.Selection)
	}
	if ctx.Pane != "" {
		b.WriteString("\nOpen pane: " + ctx.Pane)
	}
	if s := strings.TrimSpace(ctx.Summary); s != "" {
		if len(s) > maxSummary {
			s = strings.ToValidUTF8(s[:maxSummary], "")
		}
		b.WriteString("\nWhat the screen shows:\n" + s)
	}
}
