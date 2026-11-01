package api

// SpecContract is the immutable reviewed input carried into a round. Evidence
// must match all three identities: change, revision and section.
type SpecContract struct {
	ChangeID     string         `json:"change_id"`
	Revision     string         `json:"revision"`
	SectionID    string         `json:"section_id"`
	BeadID       string         `json:"bead_id"`
	ChangeName   string         `json:"change_name"`
	Summary      string         `json:"summary"`
	Rationale    string         `json:"rationale"`
	References   []string       `json:"references"`
	SectionTitle string         `json:"section_title"`
	SectionBody  string         `json:"section_body"`
	Artifacts    []SpecArtifact `json:"artifacts"`
}

type SpecArtifact struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// RequirementRef names one requirement in an accepted spec or an active
// change: its capability, title, and where it is written.
type RequirementRef struct {
	Capability string `json:"capability"`
	Title      string `json:"title"`
	Path       string `json:"path"`
	Line       int    `json:"line"`
}

// RequirementIndex is the bounded list of requirements round preflight checks
// beads against. Omitted names capabilities left out to stay within the bound.
type RequirementIndex struct {
	Requirements []RequirementRef `json:"requirements"`
	Omitted      []string         `json:"omitted,omitempty"`
}

// Find returns the requirement written at path with the given title.
func (ix *RequirementIndex) Find(path, title string) (RequirementRef, bool) {
	if ix == nil {
		return RequirementRef{}, false
	}
	for _, r := range ix.Requirements {
		if r.Path == path && r.Title == title {
			return r, true
		}
	}
	return RequirementRef{}, false
}

// Text renders the index for a prompt, one capability at a time.
func (ix *RequirementIndex) Text() string {
	if ix == nil {
		return ""
	}
	var b []byte
	last := ""
	for _, r := range ix.Requirements {
		if r.Capability != last {
			b = append(b, "\n"+r.Capability+"\n"...)
			last = r.Capability
		}
		b = append(b, CapabilityLine(r)...)
	}
	return string(b)
}

// CapabilityLine is how one requirement is listed in Text.
func CapabilityLine(r RequirementRef) string {
	return "- " + r.Path + ":" + itoa(r.Line) + " Requirement: " + r.Title + "\n"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}
