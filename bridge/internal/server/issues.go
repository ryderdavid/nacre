package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/specflow"
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
	// Related adds a related dependency on each listed bead (a follow-on
	// effort and the epic it came from).
	Related []string `json:"related"`
	// FollowOnOf is a found bead this effort replaces as a follow-on change;
	// it is closed with a link once the new bead exists.
	FollowOnOf string `json:"follow_on_of"`
	// FoundComment is a `Found:` comment this effort answers; its marker is
	// posted once the new bead exists.
	FoundComment *specflow.FoundComment `json:"found_comment"`
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
	case req.FollowOnOf != "" && !validID(req.FollowOnOf):
		writeError(w, http.StatusBadRequest, "invalid follow_on_of id", "")
		return
	}
	for _, id := range req.Related {
		if !validID(id) {
			writeError(w, http.StatusBadRequest, "invalid related id", "")
			return
		}
	}
	params := bd.CreateParams{
		Title: req.Title, Description: req.Description, IssueType: req.IssueType,
		Priority: req.Priority, Parent: req.Parent, Labels: cleanLabels(req.Labels),
		Design: req.Design, AcceptanceCriteria: req.AcceptanceCriteria, Notes: req.Notes,
		Assignee: req.Assignee,
	}
	if err := s.underSpecEpic(r.Context(), path, &params); err != nil {
		s.fail(w, err)
		return
	}
	id, err := s.BD.Create(r.Context(), path, params)
	if err != nil {
		s.fail(w, err)
		return
	}
	repoID := r.PathValue("repo")
	if err := s.afterCreate(r.Context(), path, id, req); err != nil {
		s.Log.Printf("created %s but couldn't link it: %v", id, err)
	}
	if sc := scopeOf(r.Context()); sc.Chat() && s.Decisions != nil {
		s.Decisions.LinkWork(beadLink(repoID, id), sc.ChatID)
	}
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
		// Force closes it even while beads it waits on are open.
		Force bool `json:"force"`
	}
	if err := decode(r, &req, true); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
		return
	}
	closeIt := s.BD.Close
	if req.Force {
		closeIt = s.BD.CloseForce
	}
	if err := closeIt(r.Context(), path, id, strings.TrimSpace(req.Reason)); err != nil {
		s.fail(w, err)
		return
	}
	s.changed(r.PathValue("repo"))
	s.respondIssue(w, r, path, id, http.StatusOK)
}

// signOff answers a bead left only for Ryder's check. Approving closes it;
// otherwise what isn't right goes on the bead as a comment and the bead is
// ordinary open work again. Either way the sign-off marker comes off.
func (s *Server) signOff(w http.ResponseWriter, r *http.Request) {
	path, ok := s.repoPath(w, r)
	if !ok {
		return
	}
	id, ok := issueID(w, r)
	if !ok {
		return
	}
	var req struct {
		Approve bool   `json:"approve"`
		Note    string `json:"note"`
	}
	if err := decode(r, &req, false); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
		return
	}
	note := strings.TrimSpace(req.Note)
	if !req.Approve && note == "" {
		writeError(w, http.StatusBadRequest, "say what isn't right", "")
		return
	}
	clear := bd.UpdateParams{RemoveLabels: []string{api.SignoffLabel}, UnsetMetadata: []string{api.SignoffKey}}
	if req.Approve {
		reason := "Signed off"
		if note != "" {
			reason += ": " + note
		}
		// The order beads were built in doesn't bind signing off: a bead
		// whose blocker is still waiting on its own check closes too.
		if err := s.BD.CloseForce(r.Context(), path, id, reason); err != nil {
			s.fail(w, err)
			return
		}
		_ = s.BD.Update(r.Context(), path, id, clear)
	} else {
		if err := s.BD.AddComment(r.Context(), path, id, "Not signed off: "+note); err != nil {
			s.fail(w, err)
			return
		}
		if err := s.BD.Update(r.Context(), path, id, clear); err != nil {
			s.fail(w, err)
			return
		}
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

// ClaudeAuthor is who an answer saved from a chat about a bead is from.
const ClaudeAuthor = "Claude"

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
		// FromClaude saves an answer from a chat about the bead as Claude's
		// comment, so the thread says who said it.
		FromClaude bool `json:"from_claude"`
	}
	if err := decode(r, &req, false); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeError(w, http.StatusBadRequest, "text is required", "")
		return
	}
	author := s.BD.Actor
	if req.FromClaude {
		author = ClaudeAuthor
	}
	if err := s.BD.AddCommentAs(r.Context(), path, id, author, req.Text); err != nil {
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
