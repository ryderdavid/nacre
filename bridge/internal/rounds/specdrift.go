package rounds

import (
	"context"
	"fmt"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
)

// Answers to a spec-drift question. They are fixed by nacred, not written by
// the review. See openspec/changes/surface-spec-workflow (spec-surfacing).
const (
	AnswerAmendSpec = "Amend the spec first"
	AnswerException = "Proceed as an exception"
	AnswerLeaveOut  = "Leave it out of this round"
)

// leftOutPrefix starts the reason of a bead left out of a round by a
// spec-drift answer; beads that build on it say it "was left out".
const leftOutPrefix = "Left out"

// specExempt reports whether a bead is left out of the spec-drift check: spec
// section beads (their contract is the pinned snapshot) and beads found while
// working a change (they already belong to it).
func specExempt(is issue) bool {
	for _, l := range is.Labels {
		if strings.HasPrefix(l, "specflow:") || strings.HasPrefix(l, "spec-section:") || strings.HasPrefix(l, "found:") {
			return true
		}
	}
	return false
}

// specDriftPrompt tells the review what behavior is specified.
func specDriftPrompt(s *strings.Builder, in preflightInput) {
	if in.Index == nil {
		return
	}
	s.WriteString("\n## Specified behavior\nRequirements of the repository's accepted specs and active OpenSpec changes, as path:line and title:\n")
	s.WriteString(in.Index.Text())
	var exempt []string
	for _, b := range in.Beads {
		if b.SpecExempt {
			exempt = append(exempt, b.Issue.ID)
		}
	}
	s.WriteString("\nReport a spec-drift finding when one bead would change behavior that a requirement above specifies, so building it as written would contradict the requirement. Give that one bead; requirement_path is the file path alone, without the :line, and requirement is the title after \"Requirement: \", both exactly as listed; in text say what conflicts. Report it as a spec-drift finding even when it is the only problem, never only in the summary. Don't report a bead that merely mentions or extends the area without contradicting it.")
	if len(exempt) > 0 {
		fmt.Fprintf(s, " Never report spec drift for %s: they already belong to a specification change.", strings.Join(exempt, ", "))
	}
	s.WriteString("\n")
}

// specDriftQuestion is the question a spec-drift finding asks.
func specDriftQuestion(f api.Finding) api.Question {
	req := f.Requirement
	text := fmt.Sprintf("%s would change behavior that %q specifies (%s, line %d). %s", f.Beads[0], req.Title, req.Path, req.Line, f.Text)
	return api.Question{ID: "q" + strings.TrimPrefix(f.ID, "f"), FindingID: f.ID, Kind: f.Kind, Beads: f.Beads,
		Text: strings.TrimSpace(text), Options: []string{AnswerAmendSpec, AnswerException, AnswerLeaveOut}, Requirement: req}
}

// specDriftPending reports whether an answered spec-drift question hasn't
// been carried out yet.
func (r *round) specDriftPending() bool {
	for _, q := range r.meta.Questions {
		if q.Kind == api.FindingSpecDrift && q.Answer != "" && !q.Applied {
			return true
		}
	}
	return false
}

// applySpecDrift carries out the spec-drift answers. Amending first starts
// proposal development for the bead; if that is refused the question comes
// back with the reason and the bead stays. Leaving out, or amending, takes
// the bead out of the round. An exception is recorded on the bead. Once they
// are done applyDecisions goes on to the other answers.
func (r *round) applySpecDrift() {
	type job struct {
		qid, bead, answer string
		req               api.RequirementRef
		text              string
	}
	var jobs []job
	for _, q := range r.meta.Questions {
		if q.Kind == api.FindingSpecDrift && q.Answer != "" && !q.Applied && len(q.Beads) > 0 && q.Requirement != nil {
			jobs = append(jobs, job{q.ID, q.Beads[0], q.Answer, *q.Requirement, q.Text})
		}
	}
	repo, round := r.meta.RepoPath, r.meta.ID
	r.goOp("decide", func(ctx context.Context) func() {
		refused := map[string]string{}
		for _, j := range jobs {
			cite := fmt.Sprintf("%q (%s, line %d)", j.req.Title, j.req.Path, j.req.Line)
			switch {
			case strings.HasPrefix(j.answer, "Amend"):
				err := fmt.Errorf("proposal development isn't available on this computer")
				if r.m.opts.Develop != nil {
					err = r.m.opts.Develop(ctx, repo, j.bead, fmt.Sprintf("Round %s's pre-flight found that this bead would change behavior the requirement %s specifies: %s Amend the specification first.", round, cite, j.text))
				}
				if err != nil {
					refused[j.qid] = fmt.Sprintf("Couldn't start a proposal for %s: %s. Choose another answer, or try again.", j.bead, firstLine(err.Error()))
				}
			case strings.HasPrefix(j.answer, "Proceed"):
				_ = r.m.opts.Beads.AddComment(ctx, repo, j.bead, fmt.Sprintf("Decision for round %s: proceeding as an exception to the requirement %s.", round, cite))
				_ = r.m.opts.Beads.Update(ctx, repo, j.bead, bd.UpdateParams{AddLabels: []string{"spec-touch:" + capabilityKey(j.req.Capability)}})
			}
		}
		return func() {
			reopened := false
			for _, j := range jobs {
				q := r.question(j.qid)
				if q == nil {
					continue
				}
				if problem, ok := refused[j.qid]; ok {
					q.Answer, q.AnsweredAt, q.Problem = "", "", problem
					q.Reopened++
					r.event("state", j.bead, problem)
					reopened = true
					continue
				}
				q.Applied = true
				reason := ""
				switch {
				case strings.HasPrefix(j.answer, "Amend"):
					reason = fmt.Sprintf("%s to amend the spec first: a proposal is being developed against %q", leftOutPrefix, j.req.Title)
				case strings.HasPrefix(j.answer, "Leave"):
					reason = fmt.Sprintf("%s: spec drift against %q", leftOutPrefix, j.req.Title)
				}
				if b := r.bead(j.bead); b != nil && reason != "" {
					b.State, b.Reason = api.BeadSkipped, reason
					r.event("bead", b.ID, "Skipped: "+reason)
					r.note(b.ID, fmt.Sprintf("Round %s left this bead out: %s.", r.meta.ID, reason))
				}
			}
			if reopened {
				r.meta.State = api.RoundNeedsDecision
				return
			}
			r.applyDecisions()
		}
	})
}

func (r *round) question(id string) *api.Question {
	for i := range r.meta.Questions {
		if r.meta.Questions[i].ID == id {
			return &r.meta.Questions[i]
		}
	}
	return nil
}

// capabilityKey is the capability's directory name, without the "(change …)"
// an active change's capability is listed with.
func capabilityKey(c string) string {
	if i := strings.Index(c, " ("); i > 0 {
		return c[:i]
	}
	return c
}

// allLeftOut reports whether every bead was left out by a spec-drift answer.
func (r *round) allLeftOut() bool {
	for _, b := range r.meta.Beads {
		if b.State != api.BeadSkipped || !strings.HasPrefix(b.Reason, leftOutPrefix) {
			return false
		}
	}
	return len(r.meta.Beads) > 0
}

// citation normalizes what the review cites the way the index lists it: a
// path without its ":line", a title without "Requirement:" or quotes.
func citation(path, title string) (string, string) {
	path = strings.Trim(strings.TrimSpace(path), "`\"'")
	if i := strings.LastIndexByte(path, ':'); i > 0 && allDigits(path[i+1:]) {
		path = path[:i]
	}
	title = strings.TrimSpace(title)
	title = strings.TrimSpace(strings.TrimPrefix(title, "Requirement:"))
	title = strings.Trim(title, "`\"'“”")
	return path, title
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
