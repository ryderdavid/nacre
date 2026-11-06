package decisions

// Decision is what clients see: one exact action Ryder authorizes, with what
// nacred computed about it. The action itself (method, path, body) stays on
// the Record, server side.
type Decision struct {
	ID string `json:"id"`
	// Repo is the repository's key on this computer; RepoName its name.
	Repo     string `json:"repo"`
	RepoName string `json:"repo_name,omitempty"`
	// ChatID is the chat that raised it; nil for Needs you items.
	ChatID       *string    `json:"chat_id"`
	Source       string     `json:"source"`
	Kind         string     `json:"kind"`
	Title        string     `json:"title"`
	Subtitle     string     `json:"subtitle"`
	Target       Target     `json:"target"`
	Consequences []string   `json:"consequences"`
	Evidence     []Evidence `json:"evidence"`
	// Body is longer text: a question, a failure excerpt, a human check.
	Body    string   `json:"body,omitempty"`
	Options []Option `json:"options"`
	// Ask is asked whatever option is chosen (which round bead found work
	// was found in); nil when there is nothing to ask.
	Ask *Input `json:"ask,omitempty"`

	AllowDefer      bool   `json:"allow_defer"`
	AllowReject     bool   `json:"allow_reject"`
	RejectLabel     string `json:"reject_label,omitempty"`
	RejectNeedsNote bool   `json:"reject_needs_note"`

	State string `json:"state"`
	// Result is nacred's own outcome of executing it.
	Result *Result `json:"result"`
	// Reason says why it expired, failed or is held.
	Reason       string `json:"reason,omitempty"`
	SupersededBy string `json:"superseded_by,omitempty"`
	// Answer is how Ryder answered, once he has.
	Answer *Answer `json:"answer,omitempty"`

	CreatedAt  string  `json:"created_at"`
	ExpiresAt  *string `json:"expires_at"`
	AnsweredAt *string `json:"answered_at"`
	// DeferredUntil is when a deferred decision counts again.
	DeferredUntil *string `json:"deferred_until,omitempty"`
	// Acknowledged is set once a failed or unknown outcome was seen.
	Acknowledged bool `json:"acknowledged,omitempty"`
}

// Target names what a decision acts on.
type Target struct {
	Change   string `json:"change,omitempty"`
	Bead     string `json:"bead,omitempty"`
	Round    string `json:"round,omitempty"`
	Revision string `json:"revision,omitempty"`
	Commit   string `json:"commit,omitempty"`
	// Question is a round question's id; Comment a Found: comment's.
	Question string `json:"question,omitempty"`
	Comment  string `json:"comment,omitempty"`
}

// Evidence links what a decision rests on: kind is review, changes, gates,
// bead, round or run, and ref names it.
type Evidence struct {
	Label string `json:"label"`
	Kind  string `json:"kind"`
	Ref   string `json:"ref"`
	// Repo is set when the ref is a bead or change of a repository.
	Repo string `json:"repo,omitempty"`
}

// Option is one exact answer.
type Option struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Detail      string `json:"detail,omitempty"`
	Recommended bool   `json:"recommended"`
	// Protected options run a protected action once approved.
	Protected bool   `json:"protected"`
	Input     *Input `json:"input"`
	// Disabled says why the option can't be chosen yet.
	Disabled string `json:"disabled,omitempty"`
}

// Input kinds.
const (
	InputAmount = "amount"
	InputText   = "text"
	InputChoice = "choice"
)

// Input is what an option asks for besides the choice itself.
type Input struct {
	Kind    string   `json:"kind"`
	Label   string   `json:"label,omitempty"`
	Min     *float64 `json:"min,omitempty"`
	Max     *float64 `json:"max,omitempty"`
	Unit    string   `json:"unit,omitempty"`
	Choices []Choice `json:"choices,omitempty"`
	// Default is the value an answer that leaves the input empty gets.
	Default  string `json:"default,omitempty"`
	Required bool   `json:"required"`
}

// Choice is one value of a choice input.
type Choice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Result is what executing the decision did.
type Result struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// Answer is Ryder's answer: an option (with its note and input), a defer or
// a reject.
type Answer struct {
	Option string `json:"option,omitempty"`
	Note   string `json:"note,omitempty"`
	Input  string `json:"input,omitempty"`
	// Ask is the answer to the decision's Ask.
	Ask    string `json:"ask,omitempty"`
	Defer  bool   `json:"defer,omitempty"`
	Reject bool   `json:"reject,omitempty"`
}

// Same reports whether two answers ask for the same thing.
func (a Answer) Same(b Answer) bool {
	return a.Option == b.Option && a.Input == b.Input && a.Ask == b.Ask && a.Defer == b.Defer && a.Reject == b.Reject
}

// Action is the exact request an answer executes, kept server side.
type Action struct {
	Method string         `json:"method"`
	Path   string         `json:"path"`
	Body   map[string]any `json:"body,omitempty"`
	// InputField is the body field an option's input fills, as a number for
	// amounts and a string otherwise.
	InputField string `json:"input_field,omitempty"`
	// NoteField is the body field the answer's note fills.
	NoteField string `json:"note_field,omitempty"`
	// AskField is the body field the answer to the decision's Ask fills.
	AskField string `json:"ask_field,omitempty"`
	// Then runs once this action succeeded, with "{id}" in its path and in
	// its string body values replaced by the id this action's response
	// names (a bead it created). If Then fails, what this action did stays
	// done and the decision says so.
	Then *Action `json:"then,omitempty"`
	// Label names what the action does, for saying it didn't happen
	// ("development", "the new effort").
	Label string `json:"label,omitempty"`
	// Done is the result message when the chain this action ends succeeds,
	// with "{id}" replaced as for Then.
	Done string `json:"done,omitempty"`
}

// MethodEffort is an action that starts a new effort in a chat instead of
// sending a request: body "text" is sent as a message to body "chat", or to
// a new chat in body "repo" when that chat is busy or gone.
const MethodEffort = "EFFORT"

// Record is a stored decision: what clients see plus what only nacred
// keeps, the actions and the revision they are bound to.
type Record struct {
	Decision Decision `json:"decision"`
	// Actions are each option's exact request, by option id. An option
	// without one is recorded and needs nothing executed.
	Actions map[string]Action `json:"actions,omitempty"`
	// Reject is executed when Ryder rejects; nil records the rejection only.
	Reject *Action `json:"reject,omitempty"`
	// RejectHolds: a rejection holds the work ("held") instead of declining.
	RejectHolds bool `json:"reject_holds,omitempty"`
	// Bound is the revision the actions were built for; answering checks it.
	Bound string `json:"bound,omitempty"`
	// Key names the target: a newer decision with the same key supersedes
	// an older open one.
	Key string `json:"key,omitempty"`
	// AttentionID is the Needs you item a decision was made from.
	AttentionID string `json:"attention_id,omitempty"`
	// Follow says the action finishes later (a land, a finish, an
	// approval): the decision stays executing until nacred sees the end.
	Follow bool `json:"follow,omitempty"`
}

// Open reports whether a decision still belongs in the queue: pending,
// deferred, held or executing, or an outcome Ryder hasn't acknowledged.
func (d Decision) Open() bool {
	switch d.State {
	case StatePending, StateDeferred, StateHeld, StateExecuting:
		return true
	case StateFailed, StateUnknown:
		return !d.Acknowledged
	}
	return false
}

// Waiting reports whether a decision counts toward Needs you: open and not
// deferred or executing.
func (d Decision) Waiting() bool {
	switch d.State {
	case StatePending, StateHeld:
		return true
	case StateFailed, StateUnknown:
		return !d.Acknowledged
	}
	return false
}

// Answerable reports whether options can still be chosen.
func (d Decision) Answerable() bool {
	return d.State == StatePending || d.State == StateDeferred || d.State == StateHeld
}
