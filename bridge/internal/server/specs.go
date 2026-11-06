package server

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SpecSummary is one current OpenSpec capability of a repository.
type SpecSummary struct {
	Name string `json:"name"`
	// Requirements are its requirement headings, in order.
	Requirements []string `json:"requirements"`
	// Path is spec.md's path in the repository.
	Path string `json:"path"`
}

// listSpecs lists a repository's current specifications: each capability
// folder under openspec/specs with a spec.md, and its requirement headings.
// A repository without OpenSpec has none.
func (s *Server) listSpecs(w http.ResponseWriter, r *http.Request) {
	path, ok := s.repoPath(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"specs": readSpecs(path)})
}

func readSpecs(repoPath string) []SpecSummary {
	out := []SpecSummary{}
	root := filepath.Join(repoPath, "openspec", "specs")
	entries, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		rel := filepath.ToSlash(filepath.Join("openspec", "specs", e.Name(), "spec.md"))
		f, err := os.Open(filepath.Join(repoPath, rel))
		if err != nil {
			continue
		}
		spec := SpecSummary{Name: e.Name(), Requirements: []string{}, Path: rel}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			if req, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "### Requirement:"); ok {
				if req = strings.TrimSpace(req); req != "" {
					spec.Requirements = append(spec.Requirements, req)
				}
			}
		}
		f.Close()
		out = append(out, spec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
