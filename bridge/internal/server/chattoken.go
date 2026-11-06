package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/chat"
)

// chatTokenPrefix starts every chat-scoped token: "nct_<chat id>.<mac>".
const chatTokenPrefix = "nct_"

// ChatTokenFor mints chat-scoped tokens from the device token: each is the
// chat's id and an HMAC of it keyed by the device token, so nacred can check
// one without storing it, and every chat (and only that chat) has its own.
// Rotating the device token rotates every chat token with it.
func ChatTokenFor(deviceToken string) func(chatID string) string {
	return func(chatID string) string {
		if deviceToken == "" || !chat.ValidID(chatID) {
			return ""
		}
		return chatTokenPrefix + chatID + "." + chatMAC(deviceToken, chatID)
	}
}

func chatMAC(deviceToken, chatID string) string {
	h := hmac.New(sha256.New, []byte(deviceToken))
	h.Write([]byte("nacre-chat-token:" + chatID))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// chatTokenChat returns the chat a presented chat-scoped token belongs to,
// or "" when it isn't one nacred minted with deviceToken.
func chatTokenChat(deviceToken, presented string) string {
	rest, ok := strings.CutPrefix(presented, chatTokenPrefix)
	if !ok || deviceToken == "" {
		return ""
	}
	id, mac, ok := strings.Cut(rest, ".")
	if !ok || !chat.ValidID(id) {
		return ""
	}
	want := chatMAC(deviceToken, id)
	if subtle.ConstantTimeCompare([]byte(mac), []byte(want)) != 1 {
		return ""
	}
	return id
}

// bearer is the token in an Authorization header, or "".
func bearer(header string) string {
	const prefix = "bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

// scopeKey is the context key of a request's token scope.
type scopeKey struct{}

// Scope is who a request's token speaks for: a paired device (the device
// token, or nacred executing an approved decision) or one chat's turns.
type Scope struct {
	// ChatID is set for a chat-scoped token; empty for a device.
	ChatID string
}

// Chat reports whether the request carries a chat-scoped token.
func (sc Scope) Chat() bool { return sc.ChatID != "" }

func withScope(ctx context.Context, sc Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, sc)
}

// scopeOf is the request's scope; a request that never went through auth
// (nacred's own in-process calls) is a device.
func scopeOf(ctx context.Context) Scope {
	sc, _ := ctx.Value(scopeKey{}).(Scope)
	return sc
}
