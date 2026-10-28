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
