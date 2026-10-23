package prompt

import (
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/config"
)

func TestRenderDefaultNoInstructions(t *testing.T) {
	got := Render(config.DefaultPromptTemplate, Vars{ID: "NAC-1", Title: "Fix it", Instructions: "  \n"})
	if !strings.HasPrefix(got, `You are working on bead NAC-1 ("Fix it") from`) {
		t.Fatalf("unexpected prefix: %q", got[:80])
	}
	if strings.Contains(got, "{{") || strings.Contains(got, "Additional instructions") {
		t.Fatalf("unexpanded or unexpected suffix: %q", got)
	}
	if !strings.Contains(got, "`bd update NAC-1 --claim`") || !strings.Contains(got, `bd close NAC-1 --reason`) {
		t.Fatal("id not substituted everywhere")
	}
	if !strings.HasSuffix(got, "how it was verified.") {
		t.Fatalf("unexpected suffix: %q", got[len(got)-40:])
	}
}

func TestRenderAppendsInstructions(t *testing.T) {
	got := Render("Work on {{id}}.", Vars{ID: "X-1", Instructions: " only plan \n"})
	want := "Work on X-1.\n\nAdditional instructions from Ryder:\nonly plan"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRenderExplicitInstructionsPlaceholder(t *testing.T) {
	got := Render("[{{instructions}}] {{id}} in {{repo}}", Vars{ID: "X-1", Repo: "Nacre", Instructions: "go"})
	if got != "[go] X-1 in Nacre" {
		t.Fatalf("got %q", got)
	}
	// Empty instructions with an explicit placeholder render empty, no suffix.
	if got := Render("a{{instructions}}b", Vars{}); got != "ab" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderNoRecursiveExpansion(t *testing.T) {
	got := Render("{{title}} / {{id}}", Vars{ID: "X-1", Title: "uses {{id}} literally"})
	if got != "uses {{id}} literally / X-1" {
		t.Fatalf("got %q", got)
	}
}
