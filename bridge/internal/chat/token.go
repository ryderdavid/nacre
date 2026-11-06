package chat

import "strings"

// Environment variables a chat turn gets besides NACRED_URL.
const (
	// EnvChatToken is the chat-scoped bearer token: accepted on every
	// nacred route except the protected ones (docs/API.md, Token scopes).
	EnvChatToken = "NACRE_CHAT_TOKEN"
	// EnvChatID names the chat the turn runs in, for raising decisions
	// with POST /v1/chats/$NACRE_CHAT_ID/decisions.
	EnvChatID = "NACRE_CHAT_ID"
)

// scrubbedEnv are variables a turn never inherits from nacred's own
// environment: a stale chat's token or id, or a device token.
var scrubbedEnv = map[string]bool{EnvChatToken: true, EnvChatID: true, "NACRED_TOKEN": true}

// withChatToken drops inherited token variables from env and adds the
// chat's id and, when nacred can mint one, its chat-scoped token. The device
// token is never put in a turn's environment.
func (m *Manager) withChatToken(env []string, chatID string) []string {
	out := env[:0:0]
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if scrubbedEnv[k] {
			continue
		}
		out = append(out, kv)
	}
	out = append(out, EnvChatID+"="+chatID)
	if m.opts.ChatToken != nil {
		if tok := m.opts.ChatToken(chatID); tok != "" {
			out = append(out, EnvChatToken+"="+tok)
		}
	}
	return out
}

// Exists reports whether id names a chat on this computer, for checking a
// chat-scoped token.
func (m *Manager) Exists(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.chats[id]
	return c != nil && !c.deleted
}
