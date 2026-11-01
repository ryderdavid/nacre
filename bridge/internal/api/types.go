// Package api defines the JSON wire types of the nacred HTTP API (see docs/API.md).
// It has no dependencies on the rest of the bridge so every layer can share it.
package api

import "encoding/json"

// Run statuses.
const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// RunEvent kinds.
const (
	KindUser       = "user"
	KindSystem     = "system"
	KindText       = "text"
	KindThinking   = "thinking"
	KindToolUse    = "tool_use"
	KindToolResult = "tool_result"
	KindResult     = "result"
	KindStderr     = "stderr"
	KindStatus     = "status"
)

// FeatureClaude is reported in /v1/health when this computer can run the
// app's Claude features (Refine, Draft, the board assistant) headless.
const FeatureClaude = "claude"

// FeatureCodex is reported in /v1/health when beads can be run with the
// OpenAI Codex CLI here: it is installed and signed in. When it is missing,
// health's codex_unavailable says why.
const FeatureCodex = "codex"

// Run origins.
const (
	// OriginNacre marks a run nacred dispatched and owns the process of. It
	// is the default, and is omitted from the wire.
	OriginNacre = "nacre"
	// OriginAdopted marks a Claude Code session an agent started on its own,
	// which nacred found in Claude Code's session registry and only observes.
	// nacred owns no process for it, so it cannot be cancelled and cannot
	// take a follow-up.
	OriginAdopted = "adopted"
)

// Agents that can own a run's session. Agent is omitted from the wire for a
// run nacred dispatched to Claude Code, so such a run is what it always was;
// it is AgentCodex on a run dispatched to the Codex CLI, and carried on an
// adopted run so clients can tell agents apart.
const (
	AgentClaudeCode = "claude-code"
	AgentCodex      = "codex"
)

// Activities are the finer-grained state of an adopted session, reported
// beside Status rather than inside it: clients key on the five statuses, so a
// session that is alive but parked is still StatusRunning and says so here.
// Omitted for a run nacred dispatched, whose status is exact.
const (
	ActivityWorking = "working" // alive and working
	ActivityIdle    = "idle"    // alive but parked waiting for input
	ActivityEnded   = "ended"   // the owning process is gone
)

// Modes is the set of permission modes accepted for runs (claude --permission-mode).
var Modes = []string{"auto", "acceptEdits", "plan", "bypassPermissions", "dontAsk"}

// ValidMode reports whether m is an accepted permission mode.
func ValidMode(m string) bool {
	for _, x := range Modes {
		if x == m {
			return true
		}
	}
	return false
}

// Repo is a discovered beads repository plus summary counts.
type Repo struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Path        string         `json:"path"`
	Prefix      string         `json:"prefix"`
	Counts      map[string]int `json:"counts"`
	LastUpdated string         `json:"last_updated"`
	ActiveRuns  int            `json:"active_runs"`
	// AdoptedRuns counts the live adopted sessions nacred is watching in this
	// repo — agent-owned work it does not run. ActiveRuns deliberately
	// excludes them so it keeps meaning "runs nacred is doing". Omitted when
	// zero, so a bridge with adoption off answers exactly as it did before.
	AdoptedRuns int    `json:"adopted_runs,omitempty"`
	Error       string `json:"error"`
	// GitRemote is the normalized origin URL ("github.com/owner/repo"), used by
	// clients to recognise the same repository on several computers.
	GitRemote string `json:"git_remote"`
	GitBranch string `json:"git_branch"`
	// Gates names the gates in the repository's nacre.yaml, in order; absent
	// when it has none. A repository needs them before rounds can run in it.
	Gates []string `json:"gates,omitempty"`
	// GatesError says why nacre.yaml can't be used, when it exists but is
	// invalid (with the line it is about).
	GatesError string `json:"gates_error,omitempty"`
}

// Host identifies the computer a nacred instance runs on.
type Host struct {
	ID   string `json:"host_id"`
	Name string `json:"host_name"`
}

// Issue is a bd issue object passed through verbatim from `bd list --json`,
// plus active_elsewhere when nacred adds it (see ActiveElsewhere).
type Issue = json.RawMessage

// ActiveElsewhere is the marker nacred adds to an issue, as active_elsewhere,
// when bd says someone is working on it but nothing on this computer is: the
// bead is in_progress or hooked, its assignee is not nacred's own actor, and
// no live run here (dispatched or adopted, stale adopted sessions aside) is
// linked to it. It is a claim seen in bd, not a run: there is no transcript.
type ActiveElsewhere struct {
	// Assignee is who holds the bead, as bd records it.
	Assignee string `json:"assignee"`
	// Since is the bead's started_at, or its updated_at when bd has no
	// started_at; "" when it has neither.
	Since string `json:"since,omitempty"`
	// Self is set when the assignee is the person nacred acts for (the actor
	// without its " via Nacre" suffix): their own work, outside Nacre.
	Self bool `json:"self,omitempty"`
}

// Comment is a bd comment.
type Comment struct {
	ID        string `json:"id"`
	IssueID   string `json:"issue_id"`
	Author    string `json:"author"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

// Run is a Claude Code run dispatched for one bead.
type Run struct {
	SpecChangeID  string  `json:"spec_change_id,omitempty"`
	SpecRevision  string  `json:"spec_revision,omitempty"`
	SpecSectionID string  `json:"spec_section_id,omitempty"`
	ID            string  `json:"id"`
	RepoID        string  `json:"repo_id"`
	RepoName      string  `json:"repo_name"`
	RepoPath      string  `json:"repo_path"`
	IssueID       string  `json:"issue_id"`
	IssueTitle    string  `json:"issue_title"`
	Status        string  `json:"status"`
	Mode          string  `json:"mode"`
	Model         string  `json:"model"`
	Worktree      bool    `json:"worktree"`
	SessionID     string  `json:"session_id"`
	CreatedAt     string  `json:"created_at"`
	StartedAt     string  `json:"started_at"`
	EndedAt       string  `json:"ended_at"`
	CostUSD       float64 `json:"cost_usd"`
	DurationMS    int64   `json:"duration_ms"`
	NumTurns      int     `json:"num_turns"`
	Turns         int     `json:"turns"`
	ToolCalls     int     `json:"tool_calls"`
	LastText      string  `json:"last_text"`
	ResultText    string  `json:"result_text"`
	Error         string  `json:"error"`
	ResumeCommand string  `json:"resume_command"`
	// SessionCWD is the working directory Claude reported at session init
	// (differs from RepoPath for worktree runs). Extension to API.md.
	SessionCWD string `json:"session_cwd,omitempty"`
	// Origin is OriginAdopted for an agent-owned session nacred only watches.
	// Empty (and absent from the wire) means OriginNacre.
	Origin string `json:"origin,omitempty"`
	// Agent names the tool that owns the session (AgentClaudeCode,
	// AgentCodex). Absent means Claude Code. For a Codex run SessionID is the
	// Codex thread id, and CostUSD stays 0: Codex reports no cost.
	Agent string `json:"agent,omitempty"`
	// SessionName is the name the owning agent gave the session, as Claude
	// Code recorded it. It is carried on adopted runs, where it is the only
	// human label for a session nacred did not name itself.
	SessionName string `json:"session_name,omitempty"`
	// OwnerPID is the process that owns the session. It is set only on an
	// adopted run, where that process is not nacred's child — it is exactly
	// the thing no client may be invited to signal.
	OwnerPID int `json:"owner_pid,omitempty"`
	// Activity is one of the Activity* values on an adopted run, "" on a run
	// nacred dispatched.
	Activity string `json:"activity,omitempty"`
	// Stale marks an adopted run whose session is alive but has not written
	// its transcript within the adoption idle window (adopt_idle_minutes): a
	// session left open rather than one doing work. A stale run is left out
	// of GET /v1/runs?include=adopted unless include also names "stale", and
	// is not counted by Repo.AdoptedRuns. Never set on a dispatched run.
	Stale bool `json:"stale,omitempty"`
	// LastActivityAt is the timestamp of the newest transcript entry nacred
	// has read for an adopted run, as Claude Code wrote it; absent until the
	// session has written one.
	LastActivityAt string `json:"last_activity_at,omitempty"`
	// Stalled marks a running run whose claude process has printed nothing
	// for longer than the stall window (config stall_minutes): most likely
	// stuck — on a dead API connection, say — rather than working. nacred
	// leaves the process alone; the person decides whether to cancel and
	// follow up. Cleared as soon as claude prints again or the turn ends.
	Stalled bool `json:"stalled,omitempty"`
	// StalledSince is when the last output arrived, set with Stalled.
	StalledSince string `json:"stalled_since,omitempty"`

	// RoundID is set on runs a round started; see openspec semi-attended-rounds.
	RoundID string `json:"round_id,omitempty"`
	// Kind is "bead" for a round's work on one bead and "integration" for a
	// round's merge or finishing turn; empty for a run started on its own.
	Kind string `json:"kind,omitempty"`
	// Branch and Workspace are the branch and worktree nacred made for the
	// run, and Base the commit the branch started from.
	Branch    string `json:"branch,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Base      string `json:"base,omitempty"`
	// Verification is nacred's own check of the run's work (clean tree,
	// commits, gates), absent until checked.
	Verification *Verification `json:"verification,omitempty"`
	// CanCancel and CanFollowup report whether POST /v1/runs/{run}/cancel and
	// .../followup are available for this run at all. See SetCapabilities for
	// why they are pointers.
	CanCancel   *bool `json:"can_cancel,omitempty"`
	CanFollowup *bool `json:"can_followup,omitempty"`
}

// Active reports whether the run is queued or running.
func (r *Run) Active() bool { return r.Status == StatusQueued || r.Status == StatusRunning }

// Adopted reports whether the run mirrors an agent-owned Claude Code session
// rather than a process nacred started.
func (r *Run) Adopted() bool { return r.Origin == OriginAdopted }

// SetCapabilities derives CanCancel and CanFollowup from Origin.
//
// For a run nacred dispatched both are left nil, which keeps them off the
// wire. Absence means "no restriction beyond the usual status rules": cancel
// on a finished run is still a no-op 200, a follow-up to an active run is
// still a 409. Leaving them out is what makes a Run that nacred dispatched
// byte-for-byte what it was before adopted runs existed, so a client built
// against the older API is not handed fields it would have to understand.
//
// For an adopted run both are false: nacred owns no process to signal, and
// resuming the session would fork the agent's work rather than continue it.
// Both endpoints answer 409 for such a run, and these flags are how a client
// knows that without having to try.
func (r *Run) SetCapabilities() {
	if !r.Adopted() {
		r.CanCancel, r.CanFollowup = nil, nil
		if r.SpecChangeID != "" {
			followup := false
			r.CanFollowup = &followup
		}
		return
	}
	cancel, followup := false, false
	r.CanCancel, r.CanFollowup = &cancel, &followup
}

// Verification is nacred's check that a round run's work is done: committed,
// on its branch, and passing the repository's gates.
type Verification struct {
	OK bool `json:"ok"`
	// Reason says what failed, in a sentence ("uncommitted changes: a.go").
	Reason string `json:"reason,omitempty"`
	// Uncommitted lists files left modified or untracked.
	Uncommitted []string `json:"uncommitted,omitempty"`
	// Commits is how many commits the branch has beyond its base.
	Commits int          `json:"commits"`
	Gates   []GateResult `json:"gates,omitempty"`
	At      string       `json:"at"`
}

// GateResult is the outcome of one nacre.yaml gate or build command.
type GateResult struct {
	Name       string `json:"name"`
	OK         bool   `json:"ok"`
	ExitCode   int    `json:"exit_code"`
	TimedOut   bool   `json:"timed_out,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	// Tail is the end of the command's output.
	Tail  string `json:"tail,omitempty"`
	Error string `json:"error,omitempty"`
}

// RunEvent is one entry in a run's transcript.
type RunEvent struct {
	Seq     int64           `json:"seq"`
	TS      string          `json:"ts"`
	Kind    string          `json:"kind"`
	Text    string          `json:"text,omitempty"`
	Tool    string          `json:"tool,omitempty"`
	ToolID  string          `json:"tool_id,omitempty"`
	Input   json.RawMessage `json:"input,omitempty"`
	IsError *bool           `json:"is_error,omitempty"`
	Status  string          `json:"status,omitempty"`
}

// Bool returns a pointer to b, for RunEvent.IsError.
func Bool(b bool) *bool { return &b }

// Error is the body of every non-2xx response.
type Error struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"`
}

// Stop is why a round stopped after trying to recover by itself, and the
// ways forward.
type Stop struct {
	// Kind is one of the Stop* constants.
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
	// Phase says which step to go back to: "preflight", "bead", "final" or
	// "land"; empty resumes the state the round paused from.
	Phase  string `json:"phase,omitempty"`
	Bead   string `json:"bead,omitempty"`
	Branch string `json:"branch,omitempty"`
	// Diagnosis is Claude's read of what is wrong, when it could be made.
	Diagnosis *Diagnosis   `json:"diagnosis,omitempty"`
	Choices   []StopChoice `json:"choices"`
	// Attempted lists what the round tried before stopping, e.g.
	// "fix:beads-jsonl", "turn:land-merge", "retry:2".
	Attempted []string `json:"attempted,omitempty"`
	// Fingerprint is the state the step failed in; "continue" is refused
	// while it is unchanged.
	Fingerprint string `json:"fingerprint,omitempty"`
	// Spend and EstimateUSD are set on a limits stop.
	Spend       []BeadSpend `json:"spend,omitempty"`
	EstimateUSD float64     `json:"estimate_usd,omitempty"`
}

// Diagnosis is a plain-language account of a stop.
type Diagnosis struct {
	Summary string  `json:"summary"`
	Cause   string  `json:"cause,omitempty"`
	CostUSD float64 `json:"cost_usd,omitempty"`
}

// StopChoice is one way forward from a stop.
type StopChoice struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Explanation string `json:"explanation,omitempty"`
}

// BeadSpend is what a bead's runs have cost so far.
type BeadSpend struct {
	Bead     string  `json:"bead"`
	SpentUSD float64 `json:"spent_usd"`
	Done     bool    `json:"done"`
}

// Stop kinds.
const (
	StopMergeConflict     = "merge-conflict"
	StopGatesFailed       = "gates-failed"
	StopGitError          = "git-error"
	StopBumpFailed        = "bump-failed"
	StopReviewFailed      = "review-failed"
	StopLimits            = "limits"
	StopNothingIntegrated = "nothing-integrated"
	StopConfig            = "config"
	StopLegacy            = "legacy"
)

// Stop choices.
const (
	ChoiceRetryWithBudget = "retry-with-budget"
	ChoiceSkipBranch      = "skip-branch"
	ChoiceSkipBead        = "skip-bead"
	ChoiceContinue        = "continue"
	ChoiceRaiseLimits     = "raise-limits"
	ChoiceCancel          = "cancel"
)

// Round states.
const (
	RoundPlanning      = "planning"
	RoundNeedsDecision = "needs_decision"
	RoundImplementing  = "implementing"
	RoundIntegrating   = "integrating"
	RoundReadyToLand   = "ready_to_land"
	RoundLanded        = "landed"
	RoundCancelled     = "cancelled"
	RoundPaused        = "paused"
)

// Bead states within a round.
const (
	BeadWaiting     = "waiting"
	BeadRunning     = "running"
	BeadVerifying   = "verifying"
	BeadIntegrating = "integrating"
	BeadIntegrated  = "integrated"
	BeadParked      = "parked"
	BeadSkipped     = "skipped"
)

// Pre-flight finding kinds.
const (
	FindingContradiction  = "contradiction"
	FindingAmbiguity      = "ambiguity"
	FindingOverlap        = "overlap"
	FindingOrdering       = "ordering"
	FindingSharedArtifact = "shared-artifact"
	FindingUnmergedWork   = "unmerged-work"
	// FindingSpecDrift: a bead would change behavior a cited requirement
	// specifies. Only raised in repositories with a valid OpenSpec setup.
	FindingSpecDrift = "spec-drift"
)

// Round is a set of beads in one repository that nacred carries from a
// pre-flight review to one commit that passes the repository's gates and
// builds. See openspec/changes/semi-attended-rounds.
type Round struct {
	// SpecContracts pins the reviewed artifacts and section for each linked bead.
	SpecContracts map[string]SpecContract `json:"spec_contracts,omitempty"`
	ID            string                  `json:"id"`
	RepoID        string                  `json:"repo_id"`
	RepoName      string                  `json:"repo_name"`
	RepoPath      string                  `json:"repo_path"`
	// Epic is set when the round was started from an epic.
	Epic  string `json:"epic,omitempty"`
	State string `json:"state"`
	// PausedFrom is the state a paused round resumes to; PauseReason says why
	// (the same text as Stop.Reason, kept for older clients).
	PausedFrom  string `json:"paused_from,omitempty"`
	PauseReason string `json:"pause_reason,omitempty"`
	// Stop says why a paused round stopped and what Ryder can do about it.
	Stop *Stop `json:"stop,omitempty"`
	// Target is the branch the round started from and lands on; Base is the
	// commit it started from.
	Target string `json:"target"`
	Base   string `json:"base"`
	// Branch and Workspace are the integration branch and its worktree.
	Branch    string      `json:"branch"`
	Workspace string      `json:"workspace"`
	Beads     []RoundBead `json:"beads"`
	Plan      *Plan       `json:"plan,omitempty"`
	Questions []Question  `json:"questions,omitempty"`
	// Integration is the merge in progress, if any.
	Integration *IntegrationStep `json:"integration,omitempty"`

	BudgetUSD float64 `json:"budget_usd"`
	SpentUSD  float64 `json:"spent_usd"`
	// OverheadUSD is what calls outside runs cost (diagnoses); it is part of
	// SpentUSD.
	OverheadUSD      float64 `json:"overhead_usd,omitempty"`
	TimeLimitMinutes int     `json:"time_limit_minutes"`
	Mode             string  `json:"mode"`
	Model            string  `json:"model,omitempty"`
	// RunIDs lists every run the round started, beads and integration alike.
	RunIDs []string `json:"run_ids,omitempty"`

	CreatedAt string `json:"created_at"`
	// StartedAt is when implementation began (after pre-flight and decisions).
	StartedAt string `json:"started_at,omitempty"`
	UpdatedAt string `json:"updated_at"`
	EndedAt   string `json:"ended_at,omitempty"`

	// FinalCommit, Version and Artifacts describe the verified result.
	FinalCommit string   `json:"final_commit,omitempty"`
	Version     string   `json:"version,omitempty"`
	Artifacts   []string `json:"artifacts,omitempty"`
	// LandedCommit is what the target was moved to; Pushed says whether it
	// was pushed. Tag is the release tag put on it (nacre.yaml version.tag).
	LandedCommit string `json:"landed_commit,omitempty"`
	Tag          string `json:"tag,omitempty"`
	Pushed       bool   `json:"pushed,omitempty"`
	// FinishEpic names the epic whose branch this round merges into Target;
	// such a round has no beads of its own.
	FinishEpic string `json:"finish_epic,omitempty"`
	AutoLand   bool   `json:"auto_land,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Active reports whether the round can still change.
func (r *Round) Active() bool {
	return r.State != RoundLanded && r.State != RoundCancelled
}

// RoundBead is one bead's progress in a round.
type RoundBead struct {
	SpecChangeID  string `json:"spec_change_id,omitempty"`
	SpecRevision  string `json:"spec_revision,omitempty"`
	SpecSectionID string `json:"spec_section_id,omitempty"`
	// CloseError says why closing the bead failed after its work was done;
	// the bead stays open and Needs you offers to close it.
	CloseError string `json:"close_error,omitempty"`
	ID         string `json:"id"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	Priority   int    `json:"priority"`
	State      string `json:"state"`
	// After lists the round beads this one waits for: bd dependencies and the
	// plan's orderings.
	After []string `json:"after,omitempty"`
	// Needs lists exclusive resources the bead's runs use.
	Needs     []string `json:"needs,omitempty"`
	Branch    string   `json:"branch,omitempty"`
	Workspace string   `json:"workspace,omitempty"`
	Base      string   `json:"base,omitempty"`
	RunID     string   `json:"run_id,omitempty"`
	// FixAttempts counts follow-ups sent for failed verification; Stalls
	// counts stall recoveries.
	FixAttempts  int           `json:"fix_attempts"`
	Stalls       int           `json:"stalls"`
	Verification *Verification `json:"verification,omitempty"`
	MergeCommit  string        `json:"merge_commit,omitempty"`
	// Reason says why a bead was parked or skipped.
	Reason string `json:"reason,omitempty"`
	// HumanCheck is what only Ryder can verify; such beads stay open.
	HumanCheck string `json:"human_check,omitempty"`
	// Closed is set once the round closed the bead.
	Closed bool `json:"closed,omitempty"`
	// CarriedFrom is the kept branch of an earlier round this bead's work
	// continues from; PickedUpBy is the later round that continued this
	// bead's kept branch.
	CarriedFrom string `json:"carried_from,omitempty"`
	PickedUpBy  string `json:"picked_up_by,omitempty"`
}

// Plan is the pre-flight review's result.
type Plan struct {
	Summary  string     `json:"summary"`
	Beads    []PlanBead `json:"beads"`
	Findings []Finding  `json:"findings"`
	CostUSD  float64    `json:"cost_usd"`
	// SpecUnchecked names capabilities the spec-drift check left out to stay
	// within its index bound.
	SpecUnchecked []string `json:"spec_unchecked,omitempty"`
	// SpecSetAside describes spec-drift findings dropped for not citing a
	// listed requirement, so the round says so instead of losing them.
	SpecSetAside []string `json:"spec_set_aside,omitempty"`
}

// PlanBead is what the review expects one bead to touch.
type PlanBead struct {
	ID         string   `json:"id"`
	Areas      []string `json:"areas"`
	Files      []string `json:"files"`
	DependsOn  []string `json:"depends_on,omitempty"`
	Needs      []string `json:"needs,omitempty"`
	HumanCheck string   `json:"human_check,omitempty"`
}

// Finding is one thing the review noticed about the beads together.
type Finding struct {
	ID    string   `json:"id"`
	Kind  string   `json:"kind"`
	Beads []string `json:"beads"`
	// Branch names the unmerged branch of an unmerged-work finding.
	Branch string   `json:"branch,omitempty"`
	Files  []string `json:"files,omitempty"`
	Text   string   `json:"text"`
	// Accepted is set when Ryder accepts an ordering as a bd dependency.
	Accepted bool `json:"accepted,omitempty"`
	// Unlinked is set when Ryder said an overlap needn't be sequenced.
	Unlinked bool `json:"unlinked,omitempty"`
	// Requirement is what a spec-drift finding cites.
	Requirement *RequirementRef `json:"requirement,omitempty"`
}

// Question is a finding that needs Ryder's decision.
type Question struct {
	ID         string   `json:"id"`
	FindingID  string   `json:"finding_id"`
	Kind       string   `json:"kind"`
	Beads      []string `json:"beads"`
	Branch     string   `json:"branch,omitempty"`
	Text       string   `json:"text"`
	Options    []string `json:"options"`
	Answer     string   `json:"answer,omitempty"`
	AnsweredAt string   `json:"answered_at,omitempty"`
	// Problem says why an earlier answer couldn't be carried out, when the
	// question has come back to be answered again.
	Problem string `json:"problem,omitempty"`
	// Requirement is what a spec-drift question cites.
	Requirement *RequirementRef `json:"requirement,omitempty"`
	// Applied is set once nacred has carried out a spec-drift answer, so it
	// isn't repeated when other questions are answered again.
	Applied bool `json:"applied,omitempty"`
}

// IntegrationStep is the merge nacred is working through.
type IntegrationStep struct {
	Bead string `json:"bead,omitempty"`
	// Phase is "merge", "conflict", "gates" or "final" for the round's own
	// merges, "preflight-merge" for unmerged work Ryder chose to bring in,
	// and "land-merge" or "land" for a target that moved before landing.
	Phase string `json:"phase"`
	// Branch is the branch being brought in during "preflight-merge".
	Branch string `json:"branch,omitempty"`
	// Rebump says the round's version bump was undone for a landing merge
	// and must be redone once it is resolved.
	Rebump bool `json:"rebump,omitempty"`
	// PreMerge is the integration commit before the merge, to reset to.
	PreMerge string `json:"pre_merge,omitempty"`
	RunID    string `json:"run_id,omitempty"`
	Attempts int    `json:"attempts"`
	// Problem is what the current integration turn was asked to fix.
	Problem string `json:"problem,omitempty"`
}

// RoundEvent is one line of a round's log.
type RoundEvent struct {
	Seq  int64  `json:"seq"`
	TS   string `json:"ts"`
	Kind string `json:"kind"`
	Bead string `json:"bead,omitempty"`
	Text string `json:"text"`
	// Commit is set on merges and the final commit.
	Commit string       `json:"commit,omitempty"`
	Gates  []GateResult `json:"gates,omitempty"`
}
