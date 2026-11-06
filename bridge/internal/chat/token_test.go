package chat

import (
	"path/filepath"
	"strings"
	"testing"
)

// envOf maps a child's KEY=value lines.
func envOf(lines []string) map[string][]string {
	out := map[string][]string{}
	for _, kv := range lines {
		k, v, _ := strings.Cut(kv, "=")
		out[k] = append(out[k], v)
	}
	return out
}

func mintTest(id string) string { return "nct_" + id + ".mac" }

// A turn's environment holds its chat's scoped token and id, never an
// inherited token, whichever agent runs it.
func TestTurnEnvironmentHoldsTheChatTokenOnly(t *testing.T) {
	t.Setenv(EnvChatToken, "nct_stale")
	t.Setenv(EnvChatID, "ch_stale")
	t.Setenv("NACRED_TOKEN", "device-token")

	bin, fdir := fakeClaude(t, "0", "")
	m := newManager(t, t.TempDir(), bin, func(o *Options) { o.ChatToken = mintTest })
	rec, _ := m.Create(CreateRequest{Agent: AgentClaude})
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_tokenenv", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m, rec.ID)
	checkTokenEnv(t, envOf(readLines(t, filepath.Join(fdir, "env0"))), rec.ID)

	cm, crec, cdir, _ := codexChat(t, "", func(o *Options) { o.ChatToken = mintTest })
	if _, err := cm.Send(crec.ID, SendRequest{TurnID: "c_codextoken", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, cm, crec.ID)
	checkTokenEnv(t, envOf(readLines(t, filepath.Join(cdir, "call0.env"))), crec.ID)
}

func checkTokenEnv(t *testing.T, env map[string][]string, id string) {
	t.Helper()
	if got := env[EnvChatToken]; len(got) != 1 || got[0] != mintTest(id) {
		t.Errorf("%s = %q, want this chat's token", EnvChatToken, got)
	}
	if got := env[EnvChatID]; len(got) != 1 || got[0] != id {
		t.Errorf("%s = %q, want %s", EnvChatID, got, id)
	}
	if got, ok := env["NACRED_TOKEN"]; ok {
		t.Errorf("a device token reached the turn: NACRED_TOKEN=%q", got)
	}
	for k, vs := range env {
		for _, v := range vs {
			if v == "device-token" {
				t.Errorf("%s carries the device token", k)
			}
		}
	}
}

// Without a minter (an older wiring) the turn gets no token at all, and
// still nothing inherited.
func TestTurnEnvironmentWithoutAMinter(t *testing.T) {
	t.Setenv(EnvChatToken, "nct_stale")
	bin, fdir := fakeClaude(t, "0", "")
	m := newManager(t, t.TempDir(), bin)
	rec, _ := m.Create(CreateRequest{Agent: AgentClaude})
	if _, err := m.Send(rec.ID, SendRequest{TurnID: "c_notoken", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m, rec.ID)
	env := envOf(readLines(t, filepath.Join(fdir, "env0")))
	if got, ok := env[EnvChatToken]; ok {
		t.Errorf("%s = %q without a minter", EnvChatToken, got)
	}
	if !m.Exists(rec.ID) || m.Exists("ch_000000000000000000000000") {
		t.Error("Exists")
	}
}
