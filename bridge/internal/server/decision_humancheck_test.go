package server

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/attention"
)

const humanTasks = `# Tasks

## 11. Remove what chat replaced

- [x] 11.1 Remove the old shell

## 12. Device checks (label the bead ` + "`human`" + `)

- [ ] 12.1 On the Fold, cover and unfolded: open a project and answer each card kind.
- [ ] 12.2 Accessibility runs: TalkBack and 200% font.

## Workflow follow-up

- Finish the epic.
`

// NAC-pjq.4: a bead labelled human shows as a human-check card (owner-
// decisions: Human-check cards): the exact check, the build and how to try
// it, Sign off, and Not right with a required note. It is not a generic
// question answered as a comment.
func TestAHumanBeadIsAHumanCheckCard(t *testing.T) {
	rg := newDecisionRig(t)
	writeFile(t, filepath.Join(rg.repoDir(t), "openspec/changes/chat-first-harness/tasks.md"), humanTasks)
	desc := "Section 12 of openspec/changes/chat-first-harness/tasks.md; behaviour per the change's specs."
	rg.facts.setBead(BeadFacts{ID: "N-12", Title: "12. Device checks (human)", Description: desc, Status: "open", Type: "task", Labels: []string{"app", "human"}})
	beads := []api.Issue{
		json.RawMessage(`{"id":"N-12","title":"12. Device checks (human)","status":"open","issue_type":"task","labels":["app","human"],"updated_at":"2026-11-06T07:25:12Z"}`),
		json.RawMessage(`{"id":"N-13","title":"Which palette?","status":"open","issue_type":"decision","labels":["human"],"updated_at":"2026-11-06T07:00:00Z"}`),
	}
	rg.s.Attention = &attention.Builder{
		Repos:  rg.s.Repos.Refs,
		Issues: func(context.Context, string) ([]api.Issue, error) { return beads, nil },
	}

	k := byKind(rg.queueList(t, "/v1/decisions"))
	if len(k["signoff"]) != 1 || len(k["question"]) != 1 || k["question"][0].Target.Bead != "N-13" {
		t.Fatalf("queue %+v", k)
	}
	d := k["signoff"][0]
	for _, want := range []string{"**What to check**", "12.1 On the Fold", "12.2 Accessibility runs", "**The build:** no round built it",
		"**How to try it**", "Section 12 of openspec/changes/chat-first-harness/tasks.md"} {
		if !strings.Contains(d.Body, want) {
			t.Errorf("body lacks %q:\n%s", want, d.Body)
		}
	}
	if strings.Contains(d.Body, "11.1") || strings.Contains(d.Body, "Finish the epic") {
		t.Errorf("only section 12 is the check:\n%s", d.Body)
	}
	if d.Title != "Check N-12: 12. Device checks (human)" || len(d.Options) != 1 || d.Options[0].ID != "signoff" || d.Options[0].Label != "Sign off" ||
		!d.AllowReject || d.RejectLabel != "Not right" || !d.RejectNeedsNote || !has(d.Consequences, "leaves it open") {
		t.Fatalf("card %+v", d)
	}

	// Not right needs a note, then adds it to the bead and leaves it open.
	if code, _ := rg.answer(t, d.ID, `{"reject":true}`); code == http.StatusOK {
		t.Fatal("Not right without a note was accepted")
	}
	code, got := rg.answer(t, d.ID, `{"reject":true,"note":"The cover screen clips the queue"}`)
	calls := rg.executed()
	if code != http.StatusOK || len(calls) != 1 || calls[0].path != "/v1/repos/"+rg.repo+"/issues/N-12/signoff" ||
		calls[0].body["approve"] != false || calls[0].body["note"] != "The cover screen clips the queue" {
		t.Fatalf("not right: %d %+v %+v", code, got, calls)
	}

	// The build a round made it in, when one did; and a chat can raise it.
	rg.facts.setRound(api.Round{ID: "rdbuilt", RepoID: rg.repo, State: api.RoundLanded, Target: "main", LandedCommit: "abcdef1234567",
		CreatedAt: "2026-11-06T08:00:00Z", Beads: []api.RoundBead{{ID: "N-12", State: api.BeadIntegrated, MergeCommit: "1234567abcdef"}}})
	code, raised, _ := rg.raise(t, `{"kind":"signoff","target":{"bead":"N-12"}}`)
	if code != http.StatusOK && code != http.StatusCreated {
		t.Fatalf("raise: %d %+v", code, raised)
	}
	if !strings.Contains(raised.Body, "**The build:** round rdbuilt, merge 1234567, landed on main at abcdef1") {
		t.Fatalf("build line:\n%s", raised.Body)
	}
	// Sign off closes it.
	code, _ = rg.answer(t, raised.ID, `{"option":"signoff"}`)
	calls = rg.executed()
	if code != http.StatusOK || len(calls) != 2 || calls[1].body["approve"] != true {
		t.Fatalf("sign off: %d %+v", code, calls)
	}
}
