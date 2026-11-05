package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHostIDIsGeneratedOnceAndKept(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	first, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first.HostID, "h_") || len(first.HostID) != 14 {
		t.Fatalf("host id %q", first.HostID)
	}
	again, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if again.HostID != first.HostID {
		t.Errorf("host id changed across loads: %q then %q", first.HostID, again.HostID)
	}
	if again.Host().Name == "" {
		t.Error("host name falls back to the hostname")
	}
	again.HostName = "Work laptop"
	if got := again.Host().Name; got != "Work laptop" {
		t.Errorf("explicit host_name ignored: %q", got)
	}
}

func TestAdoptIdleMinutesDefaultsToFifteen(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.AdoptIdleMinutes != 15 || c.AdoptIdleAfter() != 15*time.Minute {
		t.Fatalf("default idle window = %d min (%v)", c.AdoptIdleMinutes, c.AdoptIdleAfter())
	}
	c.AdoptIdleMinutes = 40
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	again, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if again.AdoptIdleAfter() != 40*time.Minute {
		t.Fatalf("configured idle window = %v, want 40m", again.AdoptIdleAfter())
	}
	if (&Config{}).AdoptIdleAfter() != 15*time.Minute {
		t.Fatal("an unset window falls back to the default")
	}
}

func TestStallAfter(t *testing.T) {
	for minutes, want := range map[int]time.Duration{0: 15 * time.Minute, 20: 20 * time.Minute, -1: 0} {
		if got := (&Config{StallMinutes: minutes}).StallAfter(); got != want {
			t.Errorf("stall_minutes %d: %v, want %v", minutes, got, want)
		}
	}
}

func TestChatModelsDefaultAndKeepTheirSetting(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Chat.RouterModel != "haiku" || c.Chat.LightModel != "sonnet" {
		t.Fatalf("defaults: router %q light %q", c.Chat.RouterModel, c.Chat.LightModel)
	}
	c.Chat.RouterModel, c.Chat.LightModel = "claude-haiku-5-5", "claude-sonnet-5-5"
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"router_model": "claude-haiku-5-5"`) || !strings.Contains(string(data), `"light_model": "claude-sonnet-5-5"`) {
		t.Fatalf("saved config lacks chat.router_model / chat.light_model:\n%s", data)
	}
	again, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if again.Chat.RouterModel != "claude-haiku-5-5" || again.Chat.LightModel != "claude-sonnet-5-5" {
		t.Fatalf("reloaded: %+v", again.Chat)
	}
}
