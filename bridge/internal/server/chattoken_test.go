package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/chat"
)

func TestChatTokenMintAndCheck(t *testing.T) {
	mint := ChatTokenFor("device")
	id := "ch_0123456789abcdef01234567"
	tok := mint(id)
	if !strings.HasPrefix(tok, "nct_"+id+".") || strings.Contains(tok, "device") {
		t.Fatalf("token %q", tok)
	}
	if got := chatTokenChat("device", tok); got != id {
		t.Fatalf("check = %q", got)
	}
	other := mint("ch_ffffffffffffffffffffffff")
	for _, bad := range []string{"", "device", tok + "x", strings.Replace(tok, id, "ch_ffffffffffffffffffffffff", 1), other[:len(other)-2] + "AA", "nct_nope.x"} {
		if got := chatTokenChat("device", bad); got != "" {
			t.Errorf("check(%q) = %q", bad, got)
		}
	}
	if chatTokenChat("rotated", tok) != "" {
		t.Error("a token survived rotating the device token")
	}
	if mint("not-a-chat") != "" || ChatTokenFor("")(id) != "" {
		t.Error("minted a token for an invalid chat or without a device token")
	}
}

// A chat turn reaches nacred with $NACRE_CHAT_TOKEN, and its environment
// holds no device token. The token stops working once the chat is deleted.
func TestChatTurnCallsNacredWithItsChatToken(t *testing.T) {
	bin, dir := chatClaude(t, `env > "$d/env"; curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $NACRE_CHAT_TOKEN" "$NACRED_URL/v1/chats" > "$d/code"`)
	s, _ := chatServer(t, bin, func(o *chat.Options) { o.ChatToken = ChatTokenFor("tok") })
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	s.Chats.SetNacredURL(ts.URL)

	_, body := call(t, "POST", ts.URL+"/v1/chats", `{"agent":"claude"}`)
	id := body["chat"].(map[string]any)["id"].(string)
	if code, b := call(t, "POST", ts.URL+"/v1/chats/"+id+"/messages", `{"turn_id":"c_chattoken","text":"hi"}`); code != 202 {
		t.Fatalf("send: %d %v", code, b)
	}
	waitChatIdle(t, ts.URL, id)
	code, _ := os.ReadFile(filepath.Join(dir, "code"))
	if string(code) != "200" {
		t.Fatalf("turn's call with its chat token: %q", code)
	}
	env, _ := os.ReadFile(filepath.Join(dir, "env"))
	for _, line := range strings.Split(string(env), "\n") {
		k, v, _ := strings.Cut(line, "=")
		if v == "tok" {
			t.Errorf("the device token reached the turn as %s", k)
		}
	}
	if !strings.Contains(string(env), "NACRE_CHAT_TOKEN="+ChatTokenFor("tok")(id)+"\n") {
		t.Error("turn lacks its chat token")
	}

	chatTok := ChatTokenFor("tok")(id)
	if resp, _ := do(t, "GET", ts.URL+"/v1/chats", chatTok); resp.StatusCode != http.StatusOK {
		t.Fatalf("chat token: %d", resp.StatusCode)
	}
	if code, _ := call(t, "DELETE", ts.URL+"/v1/chats/"+id, ""); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if resp, _ := do(t, "GET", ts.URL+"/v1/chats", chatTok); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a deleted chat's token: %d", resp.StatusCode)
	}
}
