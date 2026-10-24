package rounds

import (
	"fmt"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/gates"
	"github.com/ryderdavid/nacre/bridge/internal/repoconfig"
)

// Markers the round's prompts start with. Tests use them to tell the kinds
// of turn apart; they also tell a reader of the transcript what the turn is.
const (
	markBead        = "You are working on bead"
	markConflict    = "Resolve the merge conflicts"
	markGates       = "Fix the integration"
	markFinal       = "Fix the round's final check"
	markPreflight   = "Review these beads together before anyone writes code"
	markFollowupFix = "nacred checked your work"
)

// tailLines is how much gate output goes into a prompt.
const tailLines = 60

type beadPromptVars struct {
	Round      api.Round
	Issue      issue
	Comments   []api.Comment
	Branch     string
	Base       string
	Decisions  []string
	Shared     []string
	Integrated []string
	Cfg        *repoconfig.Config
	Env        []string
}

// beadPrompt is the contract a round run works under.
func beadPrompt(v beadPromptVars) string {
	var s strings.Builder
	is := v.Issue
	fmt.Fprintf(&s, "%s %s (%q) in the %s repository, as part of round %s: nacred is finishing several beads together and will merge your branch with the others.\n\n",
		markBead, is.ID, is.Title, v.Round.RepoName, v.Round.ID)

	s.WriteString("## The bead\n")
	fmt.Fprintf(&s, "Type: %s. Priority: P%d.\n", is.Type, is.Priority)
	section(&s, "Description", is.Description)
	section(&s, "Design", is.Design)
	section(&s, "Acceptance criteria", is.AcceptanceCriteria)
	section(&s, "Notes", is.Notes)
	if n := len(v.Comments); n > 0 {
		s.WriteString("\nRecent comments:\n")
		start := 0
		if n > 6 {
			start = n - 6
		}
		for _, c := range v.Comments[start:] {
			fmt.Fprintf(&s, "- %s: %s\n", c.Author, oneLine(c.Text, 600))
		}
	}

	if len(v.Decisions) > 0 {
		s.WriteString("\n## Decisions already made for this round\n")
		for _, d := range v.Decisions {
			fmt.Fprintf(&s, "- %s\n", d)
		}
	}

	s.WriteString("\n## Where you are working\n")
	fmt.Fprintf(&s, "Your working directory is a git worktree on branch %s, started from %s. Work only in this directory.\n", v.Branch, short(v.Base))
	if len(v.Integrated) > 0 {
		fmt.Fprintf(&s, "It already contains this round's finished work on: %s.\n", strings.Join(v.Integrated, "; "))
	}

	s.WriteString("\n## How your work will be checked\n")
	fmt.Fprintf(&s, "When your turn ends, nacred checks this worktree itself:\n1. nothing is left uncommitted;\n2. %s has at least one commit;\n3. these gates pass:\n", v.Branch)
	for _, g := range beadGates(v.Cfg) {
		fmt.Fprintf(&s, "   - %s: `%s`\n", g.Name, g.Run)
	}
	s.WriteString("Run `nacred gates --skip-final` in this worktree before you finish; it runs exactly these, with the same environment.\n")

	s.WriteString("\n## Rules for this round\n")
	fmt.Fprintf(&s, "- Commit your work on %s. Don't merge, rebase onto other branches, push, or switch branches: nacred merges the beads.\n", v.Branch)
	fmt.Fprintf(&s, "- Don't close %s or change its status; nacred closes it when the round is verified. You may add bd comments.\n", is.ID)
	for _, sh := range v.Shared {
		fmt.Fprintf(&s, "- Don't change %s.\n", sh)
	}
	for _, kv := range v.Env {
		name, val, _ := strings.Cut(kv, "=")
		fmt.Fprintf(&s, "- You have the %s %q (in $%s). Use only that one; another run may be using any other.\n",
			strings.ToLower(strings.TrimPrefix(name, "NACRE_RESOURCE_")), val, name)
	}
	s.WriteString("- If an acceptance criterion can only be checked by a person (a physical device, an account, a judgment), say so in your final message and leave it; don't fake it.\n")
	s.WriteString("\nFinish with a short summary of what you changed and how you checked it.\n")
	return s.String()
}

func section(s *strings.Builder, name, text string) {
	if text = strings.TrimSpace(text); text != "" {
		fmt.Fprintf(s, "\n%s:\n%s\n", name, text)
	}
}

// stallPrompt resumes a turn the round stopped because it went quiet.
const stallPrompt = "You stopped responding for a long time, so nacred stopped your turn. Carry on from where you were. Commit your work on your branch, and run `nacred gates --skip-final` before you finish."

func errorPrompt(err string) string {
	return fmt.Sprintf("Your last turn ended with an error: %s\n\nCarry on from where you were. Commit your work on your branch, and run `nacred gates --skip-final` before you finish.", oneLine(err, 600))
}

// verifyPrompt sends a bead back with exactly what failed.
func verifyPrompt(v *api.Verification) string {
	var s strings.Builder
	fmt.Fprintf(&s, "%s and it isn't done yet: %s.\n", markFollowupFix, v.Reason)
	if len(v.Uncommitted) > 0 {
		fmt.Fprintf(&s, "\nUncommitted files:\n")
		for _, f := range v.Uncommitted {
			fmt.Fprintf(&s, "- %s\n", f)
		}
		s.WriteString("Commit them if they are part of the work; otherwise delete them or add them to .gitignore.\n")
	}
	for _, g := range v.Gates {
		if !g.OK {
			fmt.Fprintf(&s, "\nGate %q output (last lines):\n```\n%s\n```\n", g.Name, lastN(g.Tail, tailLines))
		}
	}
	s.WriteString("\nFix this in the same worktree, commit, and run `nacred gates --skip-final` before you finish.\n")
	return s.String()
}

// integrationVars describe the integration for its prompts.
type integrationVars struct {
	Round     api.Round
	Bead      *api.RoundBead
	Issue     issue
	Conflicts []string
	Results   []gates.Result
	Done      []string
	Decisions []string
	Cfg       *repoconfig.Config
}

// integrationVars gathers what an integration turn needs. It takes r.mu.
func (r *round) integrationVars(bead string, conflicts []string, results []gates.Result) integrationVars {
	r.mu.Lock()
	defer r.mu.Unlock()
	v := integrationVars{Round: r.meta, Conflicts: conflicts, Results: results, Done: r.integratedSummaries(), Decisions: r.decisionsFor(bead), Cfg: r.cfg}
	if b := r.bead(bead); b != nil {
		c := *b
		v.Bead = &c
	}
	return v
}

func (v integrationVars) header(mark string) string {
	var s strings.Builder
	fmt.Fprintf(&s, "%s for round %s in the %s repository. Your working directory is the round's integration worktree, on branch %s.\n",
		mark, v.Round.ID, v.Round.RepoName, v.Round.Branch)
	if v.Bead != nil {
		fmt.Fprintf(&s, "\n## The bead being merged: %s (%q)\n", v.Bead.ID, v.Bead.Title)
	}
	if len(v.Done) > 0 {
		fmt.Fprintf(&s, "\n## Already integrated\n%s\n", strings.Join(v.Done, "\n"))
	}
	if len(v.Decisions) > 0 {
		s.WriteString("\n## Decisions made for this round\n")
		for _, d := range v.Decisions {
			fmt.Fprintf(&s, "- %s\n", d)
		}
	}
	return s.String()
}

func (v integrationVars) rules(gatesCmd string) string {
	return fmt.Sprintf("\n## Rules\n- Work only in this worktree, on %s. Don't push, reset, rebase or switch branches.\n- Keep what each bead set out to do; when they truly can't both hold, follow the decisions above, and explain any trade-off in your final message.\n- Commit when done, then run `%s` and fix anything it reports.\n", v.Round.Branch, gatesCmd)
}

func conflictPrompt(v integrationVars) string {
	var s strings.Builder
	s.WriteString(v.header(markConflict))
	fmt.Fprintf(&s, "\nnacred merged the bead's branch and git reported conflicts in:\n")
	for _, f := range v.Conflicts {
		fmt.Fprintf(&s, "- %s\n", f)
	}
	s.WriteString("\nResolve every conflict so both the bead and the work already integrated keep doing what they were meant to, `git add` the files and run `git commit --no-edit` to finish the merge.\n")
	s.WriteString(v.rules("nacred gates --skip-final"))
	return s.String()
}

func gatesPrompt(v integrationVars) string {
	var s strings.Builder
	s.WriteString(v.header(markGates))
	s.WriteString("\nThe bead merged cleanly, but with it these gates fail:\n")
	writeFailures(&s, v.Results)
	s.WriteString("\nFind what breaks when the bead meets the work already integrated, fix it, and commit.\n")
	s.WriteString(v.rules("nacred gates --skip-final"))
	return s.String()
}

func finalPrompt(v integrationVars) string {
	var s strings.Builder
	s.WriteString(v.header(markFinal))
	s.WriteString("\nEvery bead is merged and the version is bumped, but the final check fails:\n")
	writeFailures(&s, v.Results)
	s.WriteString("\nFix it and commit. Don't change the version again.\n")
	s.WriteString(v.rules("nacred gates --build"))
	return s.String()
}

// integrationFollowup sends an integration turn back with what still fails.
func integrationFollowup(problem string, results []gates.Result, final bool) string {
	var s strings.Builder
	fmt.Fprintf(&s, "It isn't fixed yet: %s.\n", problem)
	writeFailures(&s, results)
	cmd := "nacred gates --skip-final"
	if final {
		cmd = "nacred gates --build"
	}
	fmt.Fprintf(&s, "\nFix it in the same worktree, commit, and run `%s` before you finish.\n", cmd)
	return s.String()
}

func writeFailures(s *strings.Builder, results []gates.Result) {
	for _, r := range results {
		if !r.OK {
			fmt.Fprintf(s, "\n%s. Last lines of its output:\n```\n%s\n```\n", r.Summary(), lastN(r.Tail, tailLines))
		}
	}
}

// ---- pre-flight ----

const preflightSystem = `You review a set of beads (issues) that will be implemented in parallel by separate agents, each on its own branch, and then merged. Your job is to find where their intentions collide before anyone writes code, so a person can settle real disagreements up front and everything else can be sequenced automatically. You can read the repository and run read-only bd and git commands; you cannot change anything. Be concrete: name files and areas from the code, not guesses. Only raise a contradiction or an ambiguity when it would genuinely change what gets built; overlaps and orderings are handled without asking anyone.`

// preflightPrompt lists the beads, their dependencies and the unmerged
// branches for the review.
func preflightPrompt(meta api.Round, in preflightInput, cfg *repoconfig.Config) string {
	var s strings.Builder
	fmt.Fprintf(&s, "%s. Round %s in the %s repository will implement these %d beads, each on its own branch from %s, and merge them one at a time.\n",
		markPreflight, meta.ID, meta.RepoName, len(in.Beads), meta.Target)
	for _, b := range in.Beads {
		is := b.Issue
		fmt.Fprintf(&s, "\n## %s: %s\nType %s, P%d. Labels: %s.\n", is.ID, is.Title, is.Type, is.Priority, strings.Join(is.Labels, ", "))
		if len(b.After) > 0 {
			fmt.Fprintf(&s, "Depends in bd on: %s.\n", strings.Join(b.After, ", "))
		}
		section(&s, "Description", is.Description)
		section(&s, "Design", is.Design)
		section(&s, "Acceptance criteria", is.AcceptanceCriteria)
		for _, c := range b.Comments {
			fmt.Fprintf(&s, "Comment (%s): %s\n", c.Author, oneLine(c.Text, 400))
		}
	}
	if len(in.Unmerged) > 0 {
		fmt.Fprintf(&s, "\n## Branches not merged into %s\nWork that exists outside the round. Report an unmerged-work finding for each branch that changes files or areas a bead above will touch.\n", meta.Target)
		for _, u := range in.Unmerged {
			fmt.Fprintf(&s, "- %s: %s\n", u.Name, list(u.Files, maxUnmergedFiles))
		}
	}
	s.WriteString("\n## What the round owns\nThe round, not any bead, bumps the version once at the end")
	if cfg != nil && cfg.Version.Bump != "" {
		fmt.Fprintf(&s, " (`%s`)", cfg.Version.Bump)
	}
	s.WriteString(". Report a shared-artifact finding for anything else several beads would each change that only one place should (generated files, migrations numbered in sequence, changelogs).\n")
	if cfg != nil && len(cfg.Resources) > 0 {
		var names []string
		for _, r := range cfg.Resources {
			names = append(names, r.Name)
		}
		fmt.Fprintf(&s, "\nExclusive resources a bead's work may need (list them under needs): %s.\n", strings.Join(names, ", "))
	}
	s.WriteString(`
## Answer
For each bead: the areas and files it will likely touch, any round bead it must build on that bd doesn't already say (depends_on), the resources it needs, and anything in its acceptance criteria only a person can check (human_check; empty if none).
Then the findings, each with a kind:
- contradiction: two beads want incompatible things. Give a question for the person and 2-4 options.
- ambiguity: a bead is unclear in a way that changes how it meets another. Give a question and options.
- overlap: beads change the same files or areas with compatible intent. List the beads.
- ordering: one bead needs another's result. List the beads in the order they must happen.
- shared-artifact: something several beads would each change. List the beads; say what it is in text.
- unmerged-work: a branch above changes what a bead will touch. Give the branch, the beads and the shared files.
Keep text to one or two sentences. An empty findings list is a fine answer.
`)
	return s.String()
}

var preflightSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []string{"summary", "beads", "findings"},
	"properties": map[string]any{
		"summary": map[string]any{"type": "string"},
		"beads": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"id", "areas", "files", "depends_on", "needs", "human_check"},
				"properties": map[string]any{
					"id":          map[string]any{"type": "string"},
					"areas":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"files":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"depends_on":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"needs":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"human_check": map[string]any{"type": "string"},
				},
			},
		},
		"findings": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"kind", "beads", "branch", "files", "text", "question", "options"},
				"properties": map[string]any{
					"kind":     map[string]any{"type": "string", "enum": []string{api.FindingContradiction, api.FindingAmbiguity, api.FindingOverlap, api.FindingOrdering, api.FindingSharedArtifact, api.FindingUnmergedWork}},
					"beads":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"branch":   map[string]any{"type": "string"},
					"files":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"text":     map[string]any{"type": "string"},
					"question": map[string]any{"type": "string"},
					"options":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
			},
		},
	},
}

// ---- what prompts are told about the round (callers hold r.mu) ----

// decisionsFor lists the answered questions that concern bead ("" = all).
func (r *round) decisionsFor(bead string) []string {
	var out []string
	for _, q := range r.meta.Questions {
		if q.Answer == "" || (bead != "" && !contains(q.Beads, bead)) {
			continue
		}
		out = append(out, fmt.Sprintf("%s Answer: %s", q.Text, q.Answer))
	}
	return out
}

// sharedArtifacts lists what round runs must leave alone.
func (r *round) sharedArtifacts() []string {
	out := []string{"the version number (the round bumps it once at the end)"}
	if r.meta.Plan != nil {
		for _, f := range r.meta.Plan.Findings {
			if f.Kind == api.FindingSharedArtifact && f.Text != "" {
				out = append(out, f.Text)
			}
		}
	}
	return out
}

// integratedSummaries lists the beads merged so far.
func (r *round) integratedSummaries() []string {
	var out []string
	for _, b := range r.meta.Beads {
		if b.State == api.BeadIntegrated {
			out = append(out, fmt.Sprintf("%s (%s)", b.ID, b.Title))
		}
	}
	return out
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

func lastN(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
