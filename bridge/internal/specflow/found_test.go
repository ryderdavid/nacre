package specflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

func hasDependency(is issue, target, typ string) bool {
	for _, d := range is.Dependencies {
		if d.DependsOnID == target && d.Type == typ {
			return true
		}
	}
	return false
}

func TestFoundHowOrderIsOrdinaryWorkUnderTheEpic(t *testing.T) {
	repo, b, _, opts := fixture(t)
	m := newManager(t, opts)
	c := approve(t, m, repo, develop(t, m, repo))
	section := c.Sections[1]
	res, err := m.Found(context.Background(), repo, c.ID, FoundRequest{Answer: FoundHowOrder, Note: "Migrate the drafts table first", FoundIn: c.Sections[0].BeadID, BeforeSection: section.ID})
	if err != nil {
		t.Fatal(err)
	}
	p := b.created[len(b.created)-1]
	if p.Parent != c.EpicID || !p.NoInheritLabels || len(p.Labels) != 1 || p.Labels[0] != "found:"+c.ID || p.Title != "Migrate the drafts table first" {
		t.Fatalf("created %+v", p)
	}
	b.edit(res.BeadID, func(is *issue) {
		if !hasDependency(*is, c.Sections[0].BeadID, "discovered-from") || !strings.Contains(is.Description, "Found in: "+c.Sections[0].BeadID) {
			t.Fatalf("found bead=%+v", is)
		}
	})
	b.edit(section.BeadID, func(is *issue) {
		if !hasDependency(*is, res.BeadID, "blocks") {
			t.Fatalf("section doesn't wait for the found bead: %+v", is.Dependencies)
		}
	})
	if contract, err := m.Contract(context.Background(), repo, section.BeadID); err != nil || contract == nil {
		t.Fatalf("an extra dependency staled the section: %v", err)
	}
	if contract, err := m.Contract(context.Background(), repo, res.BeadID); err != nil || contract != nil {
		t.Fatalf("found bead isn't ordinary work: %+v %v", contract, err)
	}
	if _, err := m.Found(context.Background(), repo, c.ID, FoundRequest{Answer: FoundHowOrder, Note: "x", BeforeSection: "nope"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("unknown section accepted: %v", err)
	}
}

func TestFoundCommentsAreAnsweredOnce(t *testing.T) {
	repo, b, _, opts := fixture(t)
	m := newManager(t, opts)
	c := approve(t, m, repo, develop(t, m, repo))
	s := c.Sections[0].BeadID
	b.mu.Lock()
	b.comments[s] = append(b.comments[s], api.Comment{ID: "c7", Text: "Found: export has no pagination"}, api.Comment{ID: "c8", Text: "Not a finding"})
	b.mu.Unlock()
	res, err := m.Found(context.Background(), repo, c.ID, FoundRequest{Answer: FoundAmend, FoundIn: s, CommentID: "c7"})
	if err != nil {
		t.Fatal(err)
	}
	p := b.created[len(b.created)-1]
	if p.Title != "export has no pagination" || !strings.Contains(p.Description, "source comment: c7") || len(p.Labels) != 2 || p.Labels[1] != FoundAmendLabel {
		t.Fatalf("created %+v", p)
	}
	comments, _ := b.Comments(context.Background(), repo, s)
	if last := comments[len(comments)-1].Text; !strings.HasPrefix(last, FoundMarker("c7")) || !strings.Contains(last, res.BeadID) {
		t.Fatalf("marker=%q", last)
	}
	if _, err := m.Found(context.Background(), repo, c.ID, FoundRequest{Answer: FoundDismiss, FoundIn: s, CommentID: "c8"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("a comment that isn't Found: was dismissed: %v", err)
	}
	pre, _ := b.Comments(context.Background(), repo, s)
	out, err := m.Found(context.Background(), repo, c.ID, FoundRequest{Answer: FoundOutOfScope, FoundIn: s, CommentID: "c7"})
	if err != nil || out.Prefill == nil || out.Prefill.Related[0] != c.EpicID || out.Prefill.FoundComment.CommentID != "c7" || out.BeadID != "" {
		t.Fatalf("out of scope=%+v err=%v", out, err)
	}
	if post, _ := b.Comments(context.Background(), repo, s); len(post) != len(pre) {
		t.Fatal("out of scope posted a marker before any bead exists")
	}
}

func TestReviseFromFoundClosesItsBeadsOnApproval(t *testing.T) {
	repo, b, _, opts := fixture(t)
	m := newManager(t, opts)
	c := approve(t, m, repo, develop(t, m, repo))
	one, _ := m.Found(context.Background(), repo, c.ID, FoundRequest{Answer: FoundAmend, Note: "Archive needs human checks closed", FoundIn: c.Sections[0].BeadID})
	two, _ := m.Found(context.Background(), repo, c.ID, FoundRequest{Answer: FoundAmend, Note: "Also export"})
	other, _ := m.Found(context.Background(), repo, c.ID, FoundRequest{Answer: FoundHowOrder, Note: "Unrelated order"})
	if _, err := m.Found(context.Background(), repo, c.ID, FoundRequest{Answer: FoundRevise, FoundBeads: []string{other.BeadID}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("how/order work revised the change: %v", err)
	}
	m.opts.Active = func(string, string) string { return "Round R1 is still active." }
	if _, err := m.Found(context.Background(), repo, c.ID, FoundRequest{Answer: FoundRevise, FoundBeads: []string{one.BeadID}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("revised during active work: %v", err)
	}
	m.opts.Active = nil
	if _, err := m.Found(context.Background(), repo, c.ID, FoundRequest{Answer: FoundRevise, FoundBeads: []string{one.BeadID, two.BeadID}}); err != nil {
		t.Fatal(err)
	}
	m.Wait()
	list, _, _ := m.Summaries(repo)
	if len(list[0].PendingFound) != 2 {
		t.Fatalf("pending found=%v", list[0].PendingFound)
	}
	env, _ := m.Get(context.Background(), repo, c.SeedID)
	if !strings.Contains(m.records[c.ID].Feedback, "Archive needs human checks closed") || !strings.Contains(m.records[c.ID].Feedback, "Also export") {
		t.Fatalf("feedback=%q", m.records[c.ID].Feedback)
	}
	approve(t, m, repo, *env.Change)
	for _, id := range []string{one.BeadID, two.BeadID} {
		b.edit(id, func(is *issue) {
			if is.Status != "closed" {
				t.Fatalf("%s still %s after its revision was approved", id, is.Status)
			}
		})
	}
	b.edit(other.BeadID, func(is *issue) {
		if is.Status == "closed" {
			t.Fatal("how/order work closed by the revision")
		}
	})
}

func TestFoundNeedsAPromotedChange(t *testing.T) {
	repo, _, _, opts := fixture(t)
	m := newManager(t, opts)
	c := develop(t, m, repo)
	if _, err := m.Found(context.Background(), repo, c.ID, FoundRequest{Answer: FoundHowOrder, Note: "x"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("found work before promotion: %v", err)
	}
	if _, err := m.Found(context.Background(), repo, "sfnope", FoundRequest{Answer: FoundHowOrder, Note: "x"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("unknown change: %v", err)
	}
}

func TestFoundOnTheEpicNeedsNoDiscoveredFromLink(t *testing.T) {
	repo, b, _, opts := fixture(t)
	m := newManager(t, opts)
	c := approve(t, m, repo, develop(t, m, repo))
	res, err := m.Found(context.Background(), repo, c.ID, FoundRequest{Answer: FoundHowOrder, Note: "Migrate first", FoundIn: c.EpicID, BeforeSection: c.Sections[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	b.edit(res.BeadID, func(is *issue) {
		if hasDependency(*is, c.EpicID, "discovered-from") {
			t.Fatal("a child was linked to its own parent")
		}
	})
	b.edit(c.Sections[1].BeadID, func(is *issue) {
		if !hasDependency(*is, res.BeadID, "blocks") {
			t.Fatal("the section doesn't wait for the found bead")
		}
	})
}
