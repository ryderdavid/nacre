package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/rounds"
	"github.com/ryderdavid/nacre/bridge/internal/sse"
)

// roundRequest starts a round: bead ids, or an epic whose open children
// become the set.
type roundRequest struct {
	Beads            []string `json:"beads"`
	Epic             string   `json:"epic"`
	Target           string   `json:"target"`
	BudgetUSD        float64  `json:"budget_usd"`
	TimeLimitMinutes int      `json:"time_limit_minutes"`
}

func (s *Server) roundsReady(w http.ResponseWriter) bool {
	if s.Rounds == nil {
		writeError(w, http.StatusServiceUnavailable, "rounds aren't available on this computer", "")
		return false
	}
	return true
}

func (s *Server) createRound(w http.ResponseWriter, r *http.Request) {
	if !s.roundsReady(w) {
		return
	}
	repoID := r.PathValue("repo")
	if _, ok := s.repoPath(w, r); !ok {
		return
	}
	var req roundRequest
	if err := decode(r, &req, false); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
		return
	}
	if req.BudgetUSD < 0 || req.TimeLimitMinutes < 0 {
		writeError(w, http.StatusBadRequest, "budget and time limit can't be negative", "")
		return
	}
	round, err := s.Rounds.Create(rounds.CreateRequest{
		RepoID: repoID, Beads: req.Beads, Epic: req.Epic, Target: req.Target,
		BudgetUSD: req.BudgetUSD, TimeLimitMinutes: req.TimeLimitMinutes,
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, round)
}

func (s *Server) listRounds(w http.ResponseWriter, r *http.Request) {
	if !s.roundsReady(w) {
		return
	}
	q := r.URL.Query()
	writeJSON(w, http.StatusOK, map[string]any{"rounds": s.Rounds.List(q.Get("repo"), q.Get("active") == "1")})
}

func (s *Server) getRound(w http.ResponseWriter, r *http.Request) {
	if !s.roundsReady(w) {
		return
	}
	round, events, err := s.Rounds.Get(r.PathValue("round"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"round": round, "events": events})
}

// streamRound replays the round's log after ?after=seq (or Last-Event-ID),
// sends the round, then streams log entries ("event") and the round
// ("round") as they change.
func (s *Server) streamRound(w http.ResponseWriter, r *http.Request) {
	if !s.roundsReady(w) {
		return
	}
	sub, err := s.Rounds.Subscribe(r.PathValue("round"))
	if err != nil {
		s.fail(w, err)
		return
	}
	defer sub.Close()
	after := int64(0)
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		after, _ = strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	} else if v := r.URL.Query().Get("after"); v != "" {
		after, _ = strconv.ParseInt(v, 10, 64)
	}
	sw, err := sse.NewWriter(w)
	if err != nil {
		return
	}
	var ver uint64
	send := func(force bool) bool {
		for _, ev := range sub.EventsAfter(after) {
			if sw.Send(strconv.FormatInt(ev.Seq, 10), "event", ev) != nil {
				return false
			}
			after = ev.Seq
		}
		round, v := sub.Snapshot()
		if force || v != ver {
			ver = v
			return sw.Send("", "round", round) == nil
		}
		return true
	}
	if !send(true) {
		return
	}
	ping := time.NewTicker(sse.KeepAlive)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if sw.Ping() != nil {
				return
			}
		case <-sub.C:
			if !send(false) {
				return
			}
		}
	}
}

func (s *Server) answerRound(w http.ResponseWriter, r *http.Request) {
	var body struct {
		QuestionID string `json:"question_id"`
		Answer     string `json:"answer"`
	}
	if !s.roundsReady(w) || !decodeOK(w, r, &body) {
		return
	}
	s.roundResult(w, func() (api.Round, error) { return s.Rounds.Answer(r.PathValue("round"), body.QuestionID, body.Answer) })
}

func (s *Server) editRoundPlan(w http.ResponseWriter, r *http.Request) {
	var body rounds.PlanEdit
	if !s.roundsReady(w) || !decodeOK(w, r, &body) {
		return
	}
	s.roundResult(w, func() (api.Round, error) { return s.Rounds.EditPlan(r.PathValue("round"), body) })
}

func (s *Server) setRoundLimits(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BudgetUSD        float64 `json:"budget_usd"`
		TimeLimitMinutes int     `json:"time_limit_minutes"`
	}
	if !s.roundsReady(w) || !decodeOK(w, r, &body) {
		return
	}
	s.roundResult(w, func() (api.Round, error) {
		return s.Rounds.SetLimits(r.PathValue("round"), body.BudgetUSD, body.TimeLimitMinutes)
	})
}

func (s *Server) landRound(w http.ResponseWriter, r *http.Request) {
	if s.roundsReady(w) {
		s.roundResult(w, func() (api.Round, error) { return s.Rounds.Land(r.PathValue("round")) })
	}
}

func (s *Server) cancelRound(w http.ResponseWriter, r *http.Request) {
	if s.roundsReady(w) {
		s.roundResult(w, func() (api.Round, error) { return s.Rounds.Cancel(r.PathValue("round")) })
	}
}

func (s *Server) resumeRound(w http.ResponseWriter, r *http.Request) {
	if s.roundsReady(w) {
		s.roundResult(w, func() (api.Round, error) { return s.Rounds.Resume(r.PathValue("round")) })
	}
}

func (s *Server) retryRoundBead(w http.ResponseWriter, r *http.Request) {
	if s.roundsReady(w) {
		s.roundResult(w, func() (api.Round, error) { return s.Rounds.RetryBead(r.PathValue("round"), r.PathValue("id")) })
	}
}

func (s *Server) roundResult(w http.ResponseWriter, f func() (api.Round, error)) {
	round, err := f()
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, round)
}

func decodeOK(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := decode(r, v, false); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
		return false
	}
	return true
}
