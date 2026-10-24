package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
)

func (s *Server) listRepos(w http.ResponseWriter, r *http.Request) {
	refresh := r.URL.Query().Get("refresh") == "1"
	writeJSON(w, http.StatusOK, map[string]any{"repos": s.Repos.List(refresh)})
}

func (s *Server) listIssues(w http.ResponseWriter, r *http.Request) {
	repo, issues, err := s.Repos.Issues(r.PathValue("repo"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"repo": repo, "issues": s.withElsewhere(repo.ID, issues)})
}

func (s *Server) getIssue(w http.ResponseWriter, r *http.Request) {
	path, ok := s.repoPath(w, r)
	if !ok {
		return
	}
	id, ok := issueID(w, r)
	if !ok {
		return
	}
	var (
		wg       sync.WaitGroup
		issue    api.Issue
		comments []api.Comment
		ierr     error
		cerr     error
	)
	wg.Add(2)
	go func() { defer wg.Done(); issue, ierr = s.BD.Get(r.Context(), path, id) }()
	go func() { defer wg.Done(); comments, cerr = s.BD.Comments(r.Context(), path, id) }()
	wg.Wait()
	if ierr != nil {
		s.fail(w, ierr)
		return
	}
	if cerr != nil {
		s.fail(w, cerr)
		return
	}
	issue = s.withElsewhere(r.PathValue("repo"), []api.Issue{issue})[0]
	writeJSON(w, http.StatusOK, map[string]any{"issue": issue, "comments": comments})
}

// createIssueRequest is the CreateIssue body.
type createIssueRequest struct {
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	IssueType          string   `json:"issue_type"`
	Priority           *int     `json:"priority"`
	Parent             string   `json:"parent"`
	Labels             []string `json:"labels"`
	Design             string   `json:"design"`
	AcceptanceCriteria string   `json:"acceptance_criteria"`
	Notes              string   `json:"notes"`
	Assignee           string   `json:"assignee"`
}

func validPriority(p *int) bool { return p == nil || (*p >= 0 && *p <= 4) }

func (s *Server) createIssue(w http.ResponseWriter, r *http.Request) {
	path, ok := s.repoPath(w, r)
	if !ok {
		return
	}
	var req createIssueRequest
	if err := decode(r, &req, false); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	switch {
	case req.Title == "":
		writeError(w, http.StatusBadRequest, "title is required", "")
		return
	case !validPriority(req.Priority):
		writeError(w, http.StatusBadRequest, "priority must be 0-4", "")
		return
	case req.Parent != "" && !validID(req.Parent):
		writeError(w, http.StatusBadRequest, "invalid parent id", "")
		return
	}
	id, err := s.BD.Create(r.Context(), path, bd.CreateParams{
		Title: req.Title, Description: req.Description, IssueType: req.IssueType,
		Priority: req.Priority, Parent: req.Parent, Labels: cleanLabels(req.Labels),
		Design: req.Design, AcceptanceCriteria: req.AcceptanceCriteria, Notes: req.Notes,
		Assignee: req.Assignee,
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	repoID := r.PathValue("repo")
	s.changed(repoID)
	s.respondIssue(w, r, path, id, http.StatusCreated)
}

// updateIssueRequest is the UpdateIssue body; nil fields are left unchanged.
type updateIssueRequest struct {
	Title              *string   `json:"title"`
	Description        *string   `json:"description"`
	Design             *string   `json:"design"`
	AcceptanceCriteria *string   `json:"acceptance_criteria"`
	Notes              *string   `json:"notes"`
	Status             *string   `json:"status"`
	Priority           *int      `json:"priority"`
	IssueType          *string   `json:"issue_type"`
	Assignee           *string   `json:"assignee"`
	Labels             *[]string `json:"labels"`
	Parent             *string   `json:"parent"`
}

func (s *Server) updateIssue(w http.ResponseWriter, r *http.Request) {
	path, ok := s.repoPath(w, r)
	if !ok {
		return
	}
	id, ok := issueID(w, r)
	if !ok {
		return
	}
	var req updateIssueRequest
	if err := decode(r, &req, false); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
		return
	}
	switch {
	case !validPriority(req.Priority):
		writeError(w, http.StatusBadRequest, "priority must be 0-4", "")
		return
	case req.Status != nil && strings.TrimSpace(*req.Status) == "":
		writeError(w, http.StatusBadRequest, "status must not be empty", "")
		return
	case req.Title != nil && strings.TrimSpace(*req.Title) == "":
		writeError(w, http.StatusBadRequest, "title must not be empty", "")
		return
	case req.Parent != nil && *req.Parent != "" && !validID(*req.Parent):
		writeError(w, http.StatusBadRequest, "invalid parent id", "")
		return
	}
	p := bd.UpdateParams{
		Title: req.Title, Description: req.Description, Design: req.Design,
		AcceptanceCriteria: req.AcceptanceCriteria, Notes: req.Notes, Priority: req.Priority,
		IssueType: req.IssueType, Assignee: req.Assignee, Parent: req.Parent,
	}
	repoID := r.PathValue("repo")
	ctx := r.Context()

	// The current issue is needed to clear labels (bd ignores an empty
	// --set-labels) and to decide between close/reopen for status changes.
	var cur struct {
		Status string   `json:"status"`
		Labels []string `json:"labels"`
	}
	if req.Status != nil || req.Labels != nil {
		raw, err := s.BD.Get(ctx, path, id)
		if err != nil {
			s.fail(w, err)
			return
		}
		_ = json.Unmarshal(raw, &cur)
	}
	if req.Labels != nil {
		labels := cleanLabels(*req.Labels)
		if len(labels) == 0 {
			p.RemoveLabels = cur.Labels
		} else {
			p.Labels = &labels
		}
	}

	closeAfter := false
	if req.Status != nil {
		want := strings.TrimSpace(*req.Status)
		switch {
		case want == "closed":
			closeAfter = cur.Status != "closed"
		case cur.Status == "closed":
			// Reopen first (bd reopen clears closed_at), then apply the target status.
			if err := s.BD.Reopen(ctx, path, id); err != nil {
				s.changed(repoID)
				s.fail(w, err)
				return
			}
			if want != "open" {
				p.Status = &want
			}
		default:
			p.Status = &want
		}
	}
	if err := s.BD.Update(ctx, path, id, p); err != nil {
		s.changed(repoID)
		s.fail(w, err)
		return
	}
	if closeAfter {
		if err := s.BD.Close(ctx, path, id, ""); err != nil {
			s.changed(repoID)
			s.fail(w, err)
			return
		}
	}
	s.changed(repoID)
	s.respondIssue(w, r, path, id, http.StatusOK)
}

func (s *Server) closeIssue(w http.ResponseWriter, r *http.Request) {
	path, ok := s.repoPath(w, r)
	if !ok {
		return
	}
	id, ok := issueID(w, r)
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := decode(r, &req, true); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
		return
	}
	if err := s.BD.Close(r.Context(), path, id, strings.TrimSpace(req.Reason)); err != nil {
		s.fail(w, err)
		return
	}
	s.changed(r.PathValue("repo"))
	s.respondIssue(w, r, path, id, http.StatusOK)
}

func (s *Server) reopenIssue(w http.ResponseWriter, r *http.Request) {
	path, ok := s.repoPath(w, r)
	if !ok {
		return
	}
	id, ok := issueID(w, r)
	if !ok {
		return
	}
	if err := s.BD.Reopen(r.Context(), path, id); err != nil {
		s.fail(w, err)
		return
	}
	s.changed(r.PathValue("repo"))
	s.respondIssue(w, r, path, id, http.StatusOK)
}

func (s *Server) addComment(w http.ResponseWriter, r *http.Request) {
	path, ok := s.repoPath(w, r)
	if !ok {
		return
	}
	id, ok := issueID(w, r)
	if !ok {
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := decode(r, &req, false); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeError(w, http.StatusBadRequest, "text is required", "")
		return
	}
	if err := s.BD.AddComment(r.Context(), path, id, req.Text); err != nil {
		s.fail(w, err)
		return
	}
	s.changed(r.PathValue("repo"))
	comments, err := s.BD.Comments(r.Context(), path, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": comments})
}

// depTypes are the dependency types `bd dep add --type` documents. bd itself
// accepts arbitrary strings, so the bridge validates.
var depTypes = []string{"blocks", "tracks", "related", "parent-child", "discovered-from", "until", "caused-by", "validates", "relates-to", "supersedes"}

var validDepType = func() map[string]bool {
	m := map[string]bool{}
	for _, t := range depTypes {
		m[t] = true
	}
	return m
}()

func (s *Server) addDep(w http.ResponseWriter, r *http.Request) {
	path, ok := s.repoPath(w, r)
	if !ok {
		return
	}
	var req struct {
		IssueID     string `json:"issue_id"`
		DependsOnID string `json:"depends_on_id"`
		Type        string `json:"type"`
	}
	if err := decode(r, &req, false); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
		return
	}
	if !validID(req.IssueID) || !validID(req.DependsOnID) {
		writeError(w, http.StatusBadRequest, "issue_id and depends_on_id are required", "")
		return
	}
	if req.Type == "" {
		req.Type = "blocks"
	}
	if !validDepType[req.Type] {
		writeError(w, http.StatusBadRequest, "invalid dependency type", "one of: "+strings.Join(depTypes, ", "))
		return
	}
	if err := s.BD.DepAdd(r.Context(), path, req.IssueID, req.DependsOnID, req.Type); err != nil {
		s.fail(w, err)
		return
	}
	s.changed(r.PathValue("repo"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) removeDep(w http.ResponseWriter, r *http.Request) {
	path, ok := s.repoPath(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	issue, dep := q.Get("issue_id"), q.Get("depends_on_id")
	if !validID(issue) || !validID(dep) {
		writeError(w, http.StatusBadRequest, "issue_id and depends_on_id query parameters are required", "")
		return
	}
	if err := s.BD.DepRemove(r.Context(), path, issue, dep); err != nil {
		s.fail(w, err)
		return
	}
	s.changed(r.PathValue("repo"))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// respondIssue writes {"issue": Issue} fetched fresh from bd.
func (s *Server) respondIssue(w http.ResponseWriter, r *http.Request, path, id string, status int) {
	issue, err := s.BD.Get(r.Context(), path, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, status, map[string]any{"issue": s.withElsewhere(r.PathValue("repo"), []api.Issue{issue})[0]})
}

// cleanLabels trims labels and drops empties.
func cleanLabels(in []string) []string {
	out := make([]string, 0, len(in))
	for _, l := range in {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
