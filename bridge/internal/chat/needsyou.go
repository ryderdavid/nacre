package chat

import (
	"fmt"
	"regexp"
	"strings"
)

// Needs you in Chat: the /needs-you walk-through, the nacre-ask block that
// names the decision being asked, and a new chat's heads-up. See
// openspec/changes/needs-you-in-chat.

// Event kinds of the walk-through.
const (
	// KindNeedsYou is a new chat's heads-up: what waits on Ryder in its
	// repository, written by nacred, not a model.
	KindNeedsYou = "needs_you"
	// KindAsk marks where the agent asked about a decision; its text is the
	// decision id.
	KindAsk = "ask"
)

// askFence is the language of the block that names the decision asked.
const askFence = "nacre-ask"

// NeedsYouCommand starts the walk-through.
const NeedsYouCommand = "/needs-you"

var (
	needsYouRe   = regexp.MustCompile(`^/needs-you(?:\s+([A-Za-z0-9_.:-]{1,128}))?$`)
	decisionIDRe = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)
)

// ParseNeedsYou reports whether text is the walk-through command, and the
// decision it starts with, if any.
func ParseNeedsYou(text string) (start string, ok bool) {
	m := needsYouRe.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return "", false
	}
	return m[1], true
}

// NeedsYouPrompt is what the agent receives in place of /needs-you. The
// transcript keeps Ryder's short message.
func NeedsYouPrompt(start string) string {
	first := "Take them in that order."
	if start != "" {
		first = "Start with decision `" + start + "`, then take the rest in that order."
	}
	return strings.Join([]string{
		"Go through what needs me, one item at a time.",
		"",
		"1. Run `nacred decisions` in this chat's repository (or `nacred decisions <repo>`): it lists the decisions waiting on me, in nacred's order, with their options and consequences. `nacred decision <id>` shows one. " + first,
		"2. Open with one short line: how many wait and of what kinds (for example \"9 wait: 3 approvals, 2 rounds to land, 4 questions.\"). Then go straight to the first; I already said yes.",
		"3. For each item, in two to five short lines: what it is, why it waits now, what the recommended option does, and what the others do. Quote nacred's own consequences for anything that lands, pushes, merges or spends; don't paraphrase them. Then end the message with exactly one block naming it:",
		"```nacre-ask",
		"{\"decision\": \"<its id>\"}",
		"```",
		"My answers appear as buttons on my phone, with nacred's exact options. Never answer a decision yourself, never call a protected route, and never claim something happened before nacred says it did.",
		"4. If I ask about the item, answer briefly and end with the same nacre-ask block again. If I type an answer such as \"yes, approve it\", tell me to tap the option instead: only a tap on my phone answers.",
		"5. When my message says \"Answered: …\", nacred's current state of that decision is in the note (or run `nacred decision <id>`). Report it in one line (still executing means underway), then move on to the next item. A Skip moves on too.",
		"6. A failed or unknown outcome: say what nacred reports went wrong and what can be done about it; my Got it chip clears it. An expired or superseded item: say why, and offer to raise it again. When none are left, say so in one line and stop.",
	}, "\n")
}

// AnsweredNote is the line an "Answered: …" message's turn carries before
// its On screen note: nacred's current state of the decision.
func AnsweredNote(id, state string) string {
	if state == "" {
		state = "unknown to nacred"
	}
	return "Ryder answered decision `" + id + "` on his phone. nacred now reports: " + state
}

// ExtractAsks removes every fenced nacre-ask block from text and returns
// the rest and the decision ids the blocks name, in order. A block that
// doesn't hold {"decision": "<id>"} is dropped with nothing found.
func ExtractAsks(text string) (rest string, ids []string) {
	if !strings.Contains(text, askFence) {
		return text, nil
	}
	lines := strings.Split(text, "\n")
	var kept []string
	for i := 0; i < len(lines); i++ {
		fence, ok := openFenceFor(lines[i], askFence)
		if !ok {
			kept = append(kept, lines[i])
			continue
		}
		var body []string
		j := i + 1
		for ; j < len(lines) && !closesFence(lines[j], fence); j++ {
			body = append(body, lines[j])
		}
		if id := askID(strings.Join(body, "\n")); id != "" {
			ids = append(ids, id)
		}
		i = j
	}
	return tidy(strings.Join(kept, "\n")), ids
}

var askBodyRe = regexp.MustCompile(`"decision"\s*:\s*"([^"]+)"`)

func askID(body string) string {
	m := askBodyRe.FindStringSubmatch(body)
	if m == nil || !decisionIDRe.MatchString(m[1]) {
		return ""
	}
	return m[1]
}

// kindGroups name the heads-up's groups, in the order the queue takes them.
var kindGroups = []struct {
	kinds          []string
	one, many      string
}{
	{[]string{"question", "recovery"}, "question", "questions"},
	{[]string{"found"}, "found-work note", "found-work notes"},
	{[]string{"intake"}, "new effort to place", "new efforts to place"},
	{[]string{"approve", "adopt"}, "approval", "approvals"},
	{[]string{"signoff"}, "sign-off", "sign-offs"},
	{[]string{"land"}, "round to land", "rounds to land"},
	{[]string{"finish"}, "epic to finish", "epics to finish"},
	{[]string{"limit"}, "limit to raise", "limits to raise"},
}

// HeadsUpText is a new chat's heads-up for decisions of these kinds, or ""
// when none wait.
func HeadsUpText(kinds []string) string {
	if len(kinds) == 0 {
		return ""
	}
	count := map[string]int{}
	for _, k := range kinds {
		count[k]++
	}
	var parts []string
	seen := 0
	for _, g := range kindGroups {
		n := 0
		for _, k := range g.kinds {
			n += count[k]
		}
		if n == 0 {
			continue
		}
		seen += n
		label := g.many
		if n == 1 {
			label = g.one
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, label))
	}
	if other := len(kinds) - seen; other > 0 {
		label := "others"
		if other == 1 {
			label = "other"
		}
		parts = append(parts, fmt.Sprintf("%d %s", other, label))
	}
	head := fmt.Sprintf("%d things need you here", len(kinds))
	if len(kinds) == 1 {
		head = "1 thing needs you here"
	}
	lead := "Go through them one by one?"
	if len(kinds) == 1 {
		lead = "Go through it now?"
	}
	return head + ": " + joinList(parts) + ". " + lead
}

func joinList(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

// DecisionHooks are what the chat manager asks nacred's decisions about.
type DecisionHooks struct {
	// PendingKinds is the kinds of the decisions waiting on Ryder in a
	// repository, for a new chat's heads-up.
	PendingKinds func(repo string) []string
	// Asked reports whether id names a pending decision of repo.
	Asked func(repo, id string) bool
	// State is a one-line account of a decision's current state.
	State func(id string) string
}

// SetDecisionHooks sets how chats look up decisions.
func (m *Manager) SetDecisionHooks(h DecisionHooks) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.decisionHooks = h
}

func (m *Manager) hooks() DecisionHooks {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.decisionHooks
}

// headsUp adds a new chat's heads-up when decisions wait in its
// repository. It runs after Create returns, so a slow queue never delays
// the chat.
func (m *Manager) headsUp(id, repo string) {
	h := m.hooks()
	if h.PendingKinds == nil || repo == "" {
		return
	}
	kinds := h.PendingKinds(repo)
	text := HeadsUpText(kinds)
	if text == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.chats[id]
	if c == nil || c.deleted {
		return
	}
	m.loadLocked(c)
	for _, ev := range c.events {
		if ev.Kind == KindNeedsYou || ev.Kind == KindUser {
			return // once per chat, and never after the conversation started
		}
	}
	m.appendLocked(c, Event{Kind: KindNeedsYou, Text: text, Total: len(kinds)})
	if err := m.saveLocked(c); err != nil {
		m.opts.Logger.Printf("chat %s: save: %v", id, err)
	}
}

// recordAsk records that the agent asked about decision id in a turn, if
// it is pending in the chat's repository; otherwise the block is dropped.
func (m *Manager) recordAsk(c *chatState, id, turnID, route string) {
	h := m.hooks()
	m.mu.Lock()
	repo := ""
	if c.rec.Repo != nil {
		repo = *c.rec.Repo
	}
	m.mu.Unlock()
	if h.Asked == nil || !h.Asked(repo, id) {
		m.opts.Logger.Printf("chat %s: dropped nacre-ask for %q: not a pending decision of %q", c.rec.ID, id, repo)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if c.deleted {
		return
	}
	c.rec.Asking = id
	m.appendLocked(c, Event{Kind: KindAsk, TurnID: turnID, Route: route, Text: id})
	m.touchLocked(c)
}

// SetAsking makes decision id the one a chat is asking, as when the chat
// raised it. A deleted or unknown chat is skipped.
func (m *Manager) SetAsking(chatID, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.chats[chatID]
	if c == nil || c.deleted || c.rec.Asking == id {
		return
	}
	c.rec.Asking = id
	m.touchLocked(c)
}
