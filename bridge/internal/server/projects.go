package server

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/decisions"
	"github.com/ryderdavid/nacre/bridge/internal/repoconfig"
	"github.com/ryderdavid/nacre/bridge/internal/runs"
)

// endedWorkWindow is how long ended rounds and runs stay in a work list.
const endedWorkWindow = 24 * time.Hour

// maxWork bounds a work list.
const maxWork = 50

// WorkSummary is a round or run as its work card shows it.
type WorkSummary struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Title  string `json:"title"`
	Change string `json:"change"`
	Branch string `json:"branch"`
	// State is running, stopped, ready_to_land, held, done, failed or
	// cancelled.
	State  string     `json:"state"`
	Landed int        `json:"landed"`
	Total  int        `json:"total"`
	Gates  []WorkGate `json:"gates"`
	Spend  float64    `json:"spend_usd"`
	Budget float64    `json:"budget_usd"`
	// Reason says why it stopped, failed or is held.
	Reason       string  `json:"reason,omitempty"`
	StartedAt    string  `json:"started_at"`
	LastOutputAt string  `json:"last_output_at"`
	Stalled      bool    `json:"stalled"`
	ChatID       *string `json:"chat_id"`
	DecisionID   *string `json:"decision_id"`
	// RoundState and RunStatus are the underlying states.
	RoundState string `json:"round_state,omitempty"`
	RunStatus  string `json:"run_status,omitempty"`
}

// WorkGate is one gate's state on a work card: pass, fail or pending.
type WorkGate struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

func strp(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// repoWork lists a repository's rounds and runs, newest first: the ones
// still going, and those that ended in the last day.
func (s *Server) repoWork(w http.ResponseWriter, r *http.Request) {
	path, ok := s.repoPath(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"work": s.workFor(r.Context(), r.PathValue("repo"), path, s.openDecisions(r.Context()))})
}

// openDecisions are the queue's open decisions, for work cards and counts.
func (s *Server) openDecisions(ctx context.Context) []decisions.Decision {
	if s.Decisions == nil {
		return nil
	}
	var out []decisions.Decision
	for _, rec := range s.queue(ctx) {
		if rec.Decision.Open() {
			out = append(out, rec.Decision)
		}
	}
	return out
}

func (s *Server) workFor(ctx context.Context, repoID, path string, open []decisions.Decision) []WorkSummary {
	out := []WorkSummary{}
	now := time.Now()
	recent := func(ended string) bool {
		t, err := time.Parse(time.RFC3339, ended)
		return err != nil || now.Sub(t) < endedWorkWindow
	}
	var runsByID map[string]api.Run
	var runList []api.Run
	if s.Runs != nil {
		runList = s.Runs.List(runs.Filter{RepoID: repoID})
		runsByID = map[string]api.Run{}
		for _, run := range runList {
			runsByID[run.ID] = run
		}
	}
	var gateNames []string
	if cfg, err := repoconfig.Load(path); err == nil && cfg != nil {
		for _, g := range cfg.Gates {
			gateNames = append(gateNames, g.Name)
		}
	}
	if s.Rounds != nil {
		for _, round := range s.Rounds.List(repoID, false) {
			if !round.Active() && !recent(round.EndedAt) {
				continue
			}
			out = append(out, s.roundSummary(round, runsByID, gateNames, open))
		}
	}
	for _, run := range runList {
		if run.RoundID != "" || run.Adopted() {
			continue
		}
		active := run.Status == api.StatusQueued || run.Status == api.StatusRunning
		if !active && !recent(run.EndedAt) {
			continue
		}
		out = append(out, s.runSummary(run))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartedAt > out[j].StartedAt })
	if len(out) > maxWork {
		out = out[:maxWork]
	}
	return out
}

func (s *Server) chatOf(id string) *string {
	if s.Decisions == nil {
		return nil
	}
	return strp(s.Decisions.ChatOf(id))
}

func (s *Server) roundSummary(round api.Round, runsByID map[string]api.Run, gateNames []string, open []decisions.Decision) WorkSummary {
	ws := WorkSummary{Kind: "round", ID: round.ID, Title: "Round " + round.ID, Branch: round.Branch,
		Total: len(round.Beads), Spend: round.SpentUSD, Budget: round.BudgetUSD, Gates: []WorkGate{},
		StartedAt: or(round.StartedAt, round.CreatedAt), LastOutputAt: round.UpdatedAt,
		ChatID: s.chatOf(round.ID), RoundState: round.State}
	if round.Epic != "" {
		ws.Title += " · " + round.Epic
	}
	if round.FinishEpic != "" {
		ws.Title = "Finish " + round.FinishEpic
	}
	for _, c := range round.SpecContracts {
		ws.Change = c.ChangeName
		break
	}
	for _, b := range round.Beads {
		if b.State == api.BeadIntegrated {
			ws.Landed++
		}
	}
	switch round.State {
	case api.RoundPlanning, api.RoundImplementing, api.RoundIntegrating:
		ws.State = "running"
	case api.RoundNeedsDecision:
		ws.State, ws.Reason = "stopped", "Waiting for your answers"
	case api.RoundPaused:
		ws.State = "stopped"
		if round.Stop != nil {
			ws.Reason = round.Stop.Reason
		}
	case api.RoundReadyToLand:
		ws.State = "ready_to_land"
	case api.RoundLanded:
		ws.State = "done"
	case api.RoundCancelled:
		ws.State = "cancelled"
	default:
		ws.State = round.State
	}
	for _, d := range open {
		if d.Target.Round != round.ID {
			continue
		}
		id := d.ID
		ws.DecisionID = &id
		if d.State == decisions.StateHeld && ws.State == "ready_to_land" {
			ws.State, ws.Reason = "held", d.Reason
		}
	}
	// Gates: failing when a bead's latest verification failed one, passing
	// once every verified bead passed it, pending otherwise.
	state := map[string]string{}
	seen := append([]string{}, gateNames...)
	for _, b := range round.Beads {
		if b.Verification == nil {
			continue
		}
		for _, g := range b.Verification.Gates {
			if _, ok := state[g.Name]; !ok && !contains(seen, g.Name) {
				seen = append(seen, g.Name)
			}
			switch {
			case !g.OK:
				state[g.Name] = "fail"
			case state[g.Name] != "fail":
				state[g.Name] = "pass"
			}
		}
	}
	for _, name := range seen {
		st := state[name]
		if st == "" {
			st = "pending"
		}
		ws.Gates = append(ws.Gates, WorkGate{Name: name, State: st})
	}
	for _, id := range round.RunIDs {
		run, ok := runsByID[id]
		if !ok {
			continue
		}
		if run.Status == api.StatusRunning && run.Stalled {
			ws.Stalled = true
		}
		if run.LastActivityAt > ws.LastOutputAt {
			ws.LastOutputAt = run.LastActivityAt
		}
	}
	return ws
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (s *Server) runSummary(run api.Run) WorkSummary {
	ws := WorkSummary{Kind: "run", ID: run.ID, Title: or(run.IssueTitle, "Run "+run.ID), Change: run.SpecChangeID,
		Total: 1, Spend: run.CostUSD, Gates: []WorkGate{}, StartedAt: or(run.StartedAt, run.CreatedAt),
		LastOutputAt: or(run.LastActivityAt, or(run.StalledSince, run.StartedAt)),
		Stalled:      run.Status == api.StatusRunning && run.Stalled, ChatID: s.chatOf(run.ID), RunStatus: run.Status}
	switch run.Status {
	case api.StatusQueued, api.StatusRunning:
		ws.State = "running"
	case api.StatusSucceeded:
		ws.State, ws.Landed = "done", 1
	case api.StatusFailed:
		ws.State, ws.Reason = "failed", run.Error
	case api.StatusCancelled:
		ws.State = "cancelled"
	default:
		ws.State = run.Status
	}
	return ws
}

// Project is one repository on this computer as Projects shows it.
type Project struct {
	Repo         string       `json:"repo"`
	Name         string       `json:"name"`
	Remote       string       `json:"remote"`
	HostID       string       `json:"host_id"`
	HostName     string       `json:"host_name"`
	Status       string       `json:"status"`
	StatusText   string       `json:"status_text"`
	NeedsYou     int          `json:"needs_you"`
	ActiveChange string       `json:"active_change"`
	LastLine     string       `json:"last_line"`
	LastAt       string       `json:"last_at"`
	Work         *WorkSummary `json:"work"`
}

// listProjects is this computer's repositories with their live status, what
// waits on Ryder, the change under way and the last line of their newest
// chat. The app groups them by remote across computers.
func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	open := s.openDecisions(r.Context())
	needs := map[string]int{}
	for _, d := range open {
		if d.Waiting() {
			needs[d.Repo]++
		}
	}
	host := api.Host{}
	if s.Config != nil {
		host = s.Config.Host()
	}
	out := []Project{}
	for _, repo := range s.Repos.List(false) {
		p := Project{Repo: repo.ID, Name: repo.Name, Remote: repo.GitRemote, HostID: host.ID, HostName: host.Name,
			NeedsYou: needs[repo.ID], LastAt: repo.LastUpdated}
		work := s.workFor(r.Context(), repo.ID, repo.Path, open)
		for i := range work {
			if work[i].State == "running" || work[i].State == "stopped" || work[i].State == "ready_to_land" || work[i].State == "held" {
				p.Work = &work[i]
				break
			}
		}
		stalled, running := false, false
		for _, wk := range work {
			stalled = stalled || wk.Stalled
			running = running || wk.State == "running"
		}
		switch {
		case stalled:
			p.Status = "stalled"
		case running:
			p.Status = "working"
		case p.NeedsYou > 0:
			p.Status = "waiting"
		default:
			p.Status = "idle"
		}
		p.StatusText = statusText(p, stalled)
		if list, err := s.facts().Changes(repo.Path); err == nil {
			for _, c := range list {
				if c.Active != "" {
					p.ActiveChange = c.ChangeName
					break
				}
			}
		}
		if p.ActiveChange == "" && p.Work != nil {
			p.ActiveChange = p.Work.Change
		}
		if s.Chats != nil {
			if chats := s.Chats.Listings(repo.ID); len(chats) > 0 {
				p.LastLine = chats[0].LastLine
				p.LastAt = chats[0].UpdatedAt
			}
		}
		out = append(out, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": out})
}

// statusText says in a line what a project is doing.
func statusText(p Project, stalled bool) string {
	var parts []string
	if wk := p.Work; wk != nil {
		noun := "Round"
		if wk.Kind == "run" {
			noun = "Run"
		}
		switch wk.State {
		case "running":
			line := noun + " running"
			if wk.Kind == "round" && wk.Total > 0 {
				line += fmt.Sprintf(" · %d of %d landed", wk.Landed, wk.Total)
			}
			if stalled {
				line += " · may be stalled"
			}
			parts = append(parts, line)
		case "stopped":
			parts = append(parts, noun+" stopped")
		case "ready_to_land":
			parts = append(parts, noun+" ready to land")
		case "held":
			parts = append(parts, noun+" held by you")
		}
	}
	if p.NeedsYou > 0 {
		parts = append(parts, plural(p.NeedsYou, "needs you", "need you"))
	}
	if len(parts) == 0 {
		return "Idle"
	}
	return strings.Join(parts, " · ")
}
