package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSetupToken behaves like `claude setup-token`: it offers a link, then
// trades the code "good" for a token.
const fakeSetupToken = `#!/bin/sh
printf 'Browser did not open? Use the url below\n\033]8;id=1;https://claude.com/cai/oauth/authorize?code=true&state=abc\007link\033]8;;\007\n'
printf 'Paste code here if prompted > '
read code
code=$(printf '%s' "$code" | tr -d '\r')
if [ "$code" = good ]; then
  printf '\n\033[32mYour OAuth token:\033[0m\n\nsk-ant-oat01-AbCdEf_123-xyzXYZ0987654321abcdef\n\nStore this token securely.\n'
else
  printf '\nOAuth error: Invalid code. Press Enter to retry.\n'
fi
sleep 5
`

func TestSigningInFromThePhoneKeepsALongLivedToken(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	if err := os.WriteFile(bin, []byte(fakeSetupToken), 0o700); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(dir, "conf", "claude-token")
	if err := UseTokenFile(tokenPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = UseTokenFile(filepath.Join(t.TempDir(), "none")) })
	if HasToken() || AuthEnv() != nil {
		t.Fatal("a token before signing in")
	}

	in, link, err := StartSignIn(bin)
	if err != nil {
		t.Fatal(err)
	}
	if link != "https://claude.com/cai/oauth/authorize?code=true&state=abc" {
		t.Fatalf("link %q", link)
	}
	if err := in.Finish("nope"); err == nil || !strings.Contains(err.Error(), "didn't work") {
		t.Fatalf("a bad code: %v", err)
	}
	if HasToken() {
		t.Fatal("a bad code kept a token")
	}

	in, _, err = StartSignIn(bin)
	if err != nil {
		t.Fatal(err)
	}
	if err := in.Finish(" good "); err != nil {
		t.Fatal(err)
	}
	want := "sk-ant-oat01-AbCdEf_123-xyzXYZ0987654321abcdef"
	if env := AuthEnv(); len(env) != 1 || env[0] != "CLAUDE_CODE_OAUTH_TOKEN="+want {
		t.Fatalf("env %v", env)
	}
	b, _ := os.ReadFile(tokenPath)
	if strings.TrimSpace(string(b)) != want {
		t.Fatalf("saved %q", b)
	}
	if st, _ := os.Stat(tokenPath); st.Mode().Perm() != 0o600 {
		t.Fatalf("token file mode %v", st.Mode().Perm())
	}
	// A restart reads it back.
	if err := UseTokenFile(tokenPath); err != nil || !HasToken() {
		t.Fatalf("reload: %v", err)
	}
}
