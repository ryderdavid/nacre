package config

import (
	"path/filepath"
	"strings"
	"testing"
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
