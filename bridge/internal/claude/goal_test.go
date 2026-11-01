package claude

import (
	"strings"
	"testing"
)

func TestNativeGoalLaunch(t *testing.T) {
	tn := Turn{Prompt: "work on X-1", Goal: "criteria + commit"}
	if got := tn.Args()[1]; !strings.HasPrefix(got, "/goal criteria + commit\n") || !strings.Contains(got, tn.Prompt) {
		t.Fatal(got)
	}
	tn.Resume = "session"
	if got := tn.Args()[1]; got != tn.Prompt {
		t.Fatal("followup reset the native goal", got)
	}
	if got := (Turn{Prompt: "ordinary"}).Args()[1]; got != "ordinary" {
		t.Fatal(got)
	}
}
