package main

import (
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/config"
)

// Inside a chat turn the nacred command calls the API with the chat's scoped
// token; elsewhere with the device token.
func TestClientTokenPrefersTheChatToken(t *testing.T) {
	cfg := &config.Config{Token: "device"}
	t.Setenv("NACRE_CHAT_TOKEN", "")
	if got := clientToken(cfg); got != "device" {
		t.Fatalf("outside a chat: %q", got)
	}
	t.Setenv("NACRE_CHAT_TOKEN", "nct_ch_x.mac")
	if got := clientToken(cfg); got != "nct_ch_x.mac" {
		t.Fatalf("inside a chat: %q", got)
	}
}
