// Package prompt renders the run prompt template.
//
// Templates use simple placeholders: {{id}}, {{title}}, {{repo}} and
// {{instructions}}. Substitution is a single pass, so placeholder-like text
// inside a bead title or the instructions is never expanded again.
package prompt

import "strings"

// InstructionsSuffix is appended when the template does not itself reference
// {{instructions}} and the instructions are non-empty.
const InstructionsSuffix = "\n\nAdditional instructions from Ryder:\n{{instructions}}"

// Vars are the values substituted into a template.
type Vars struct {
	ID           string
	Title        string
	Repo         string
	Instructions string
}

// Render expands tmpl with v.
func Render(tmpl string, v Vars) string {
	instr := strings.TrimSpace(v.Instructions)
	if instr != "" && !strings.Contains(tmpl, "{{instructions}}") {
		tmpl += InstructionsSuffix
	}
	r := strings.NewReplacer(
		"{{id}}", v.ID,
		"{{title}}", v.Title,
		"{{repo}}", v.Repo,
		"{{instructions}}", instr,
	)
	return r.Replace(tmpl)
}
