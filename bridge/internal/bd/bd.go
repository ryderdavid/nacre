// Package bd wraps the beads `bd` CLI. Every call execs bd with an argv slice
// (never a shell) as `bd -C <repo root> … --json`, with a timeout. Writes to a
// repo are serialized; reads may run concurrently but never overlap a write.
package bd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
)

// DefaultTimeout bounds each bd invocation.
const DefaultTimeout = 30 * time.Second

// Client runs bd commands.
type Client struct {
	Path    string // bd binary
	Actor   string // --actor for writes
	Timeout time.Duration

	mu    sync.Mutex
	locks map[string]*sync.RWMutex
}

// New returns a Client for the bd binary at path.
func New(path, actor string) *Client {
	return &Client{Path: path, Actor: actor, Timeout: DefaultTimeout, locks: map[string]*sync.RWMutex{}}
}

// Error is a failed bd invocation.
type Error struct {
	Args     []string
	Message  string // best human-readable message (from bd's JSON error or stderr)
	Stderr   string
	NotFound bool // the issue (or one of the issues) does not exist
	Invalid  bool // bd rejected the input (e.g. unknown status or type)
}

func (e *Error) Error() string { return "bd: " + e.Message }

// Detail returns stderr/stdout context for API error bodies.
func (e *Error) Detail() string { return tail(e.Stderr, 2000) }

// IsInvalid reports whether err is bd rejecting its input.
func IsInvalid(err error) bool {
	var be *Error
	return errors.As(err, &be) && be.Invalid
}

// IsNotFound reports whether err is a bd "no issue found" failure.
func IsNotFound(err error) bool {
	var be *Error
	return errors.As(err, &be) && be.NotFound
}

func (c *Client) lock(repo string) *sync.RWMutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	l, ok := c.locks[repo]
	if !ok {
		l = &sync.RWMutex{}
		c.locks[repo] = l
	}
	return l
}

// run executes bd in repo. write selects the exclusive per-repo lock and adds --actor.
func (c *Client) run(ctx context.Context, repo string, write bool, args ...string) ([]byte, error) {
	l := c.lock(repo)
	if write {
		l.Lock()
		defer l.Unlock()
	} else {
		l.RLock()
		defer l.RUnlock()
	}
	argv := []string{"-C", repo}
	if write && c.Actor != "" {
		argv = append(argv, "--actor="+c.Actor)
	}
	argv = append(argv, args...)
	return c.exec(ctx, argv)
}

func (c *Client) exec(ctx context.Context, argv []string) ([]byte, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Path, argv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.Bytes(), nil
	}
	be := &Error{Args: argv, Stderr: strings.TrimSpace(stderr.String())}
	if ctx.Err() == context.DeadlineExceeded {
		be.Message = fmt.Sprintf("timed out after %s", timeout)
		return nil, be
	}
	// bd --json reports failures as {"error": "..."} on stdout.
	var je struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &je) == nil && je.Error != "" {
		be.Message = je.Error
	} else if be.Stderr != "" {
		be.Message = lastLine(be.Stderr)
	} else {
		be.Message = err.Error()
	}
	be.Message = strings.TrimPrefix(be.Message, "Error: ")
	if be.Stderr == "" {
		be.Stderr = strings.TrimSpace(stdout.String())
	}
	low := strings.ToLower(be.Message + " " + be.Stderr)
	be.Invalid = strings.Contains(low, "invalid ")
	be.NotFound = strings.Contains(low, "no issue found") || strings.Contains(low, "no issues found") || strings.Contains(low, "issue not found")
	return nil, be
}

// Version returns the bd version string (e.g. "1.2.2").
func (c *Client) Version(ctx context.Context) (string, error) {
	out, err := c.exec(ctx, []string{"version"})
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(out))
	// "bd version 1.2.2 (Homebrew)" → "1.2.2"
	if f := strings.Fields(s); len(f) >= 3 && f[0] == "bd" && f[1] == "version" {
		return f[2], nil
	}
	return s, nil
}

// Prefix returns the repo's configured issue-id prefix (bd config issue_prefix).
func (c *Client) Prefix(ctx context.Context, repo string) (string, error) {
	out, err := c.run(ctx, repo, false, "config", "get", "issue_prefix")
	if err != nil {
		return "", err
	}
	p := strings.TrimSpace(string(out))
	if p == "" || strings.ContainsAny(p, " \t\n") {
		return "", &Error{Message: "no issue_prefix configured"}
	}
	return p, nil
}

// List returns every issue (including closed) verbatim.
func (c *Client) List(ctx context.Context, repo string) ([]api.Issue, error) {
	out, err := c.run(ctx, repo, false, "list", "--all", "--limit", "0", "--json")
	if err != nil {
		return nil, err
	}
	return decodeIssues(out)
}

// Get returns one issue in `bd list` shape (which, unlike `bd show`, carries
// dependency edges and labels in the same form as the board list).
func (c *Client) Get(ctx context.Context, repo, id string) (api.Issue, error) {
	out, err := c.run(ctx, repo, false, "list", "--all", "--limit", "0", "--id="+id, "--json")
	if err != nil {
		return nil, err
	}
	issues, err := decodeIssues(out)
	if err != nil {
		return nil, err
	}
	for _, is := range issues {
		var m struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(is, &m) == nil && m.ID == id {
			return is, nil
		}
	}
	return nil, &Error{Message: fmt.Sprintf("no issue found matching %q", id), NotFound: true}
}

// Children lists the beads under parent, closed ones included.
func (c *Client) Children(ctx context.Context, repo, parent string) ([]api.Issue, error) {
	out, err := c.run(ctx, repo, false, "list", "--all", "--limit", "0", "--parent="+parent, "--json")
	if err != nil {
		return nil, err
	}
	return decodeIssues(out)
}

func decodeIssues(out []byte) ([]api.Issue, error) {
	out = bytes.TrimSpace(out)
	if len(out) == 0 || bytes.Equal(out, []byte("null")) {
		return []api.Issue{}, nil
	}
	var issues []api.Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, &Error{Message: "unexpected bd list output: " + err.Error(), Stderr: tail(string(out), 500)}
	}
	return issues, nil
}

// Comments returns the comments on an issue (oldest first).
func (c *Client) Comments(ctx context.Context, repo, id string) ([]api.Comment, error) {
	out, err := c.run(ctx, repo, false, "comments", "--json", "--", id)
	if err != nil {
		return nil, err
	}
	out = bytes.TrimSpace(out)
	comments := []api.Comment{}
	if len(out) == 0 || bytes.Equal(out, []byte("null")) {
		return comments, nil
	}
	if err := json.Unmarshal(out, &comments); err != nil {
		return nil, &Error{Message: "unexpected bd comments output: " + err.Error(), Stderr: tail(string(out), 500)}
	}
	if comments == nil {
		comments = []api.Comment{}
	}
	return comments, nil
}

// AddComment adds a comment authored by the configured actor.
func (c *Client) AddComment(ctx context.Context, repo, id, text string) error {
	return c.AddCommentAs(ctx, repo, id, c.Actor, text)
}

// AddCommentAs adds a comment from author; empty uses bd's own default.
func (c *Client) AddCommentAs(ctx context.Context, repo, id, author, text string) error {
	args := []string{"comments", "add"}
	if author != "" {
		args = append(args, "--author="+author)
	}
	args = append(args, "--", id, text)
	_, err := c.run(ctx, repo, true, args...)
	return err
}

// CreateParams are the fields accepted by Create.
type CreateParams struct {
	Title              string
	Description        string
	IssueType          string
	Priority           *int
	Parent             string
	Labels             []string
	Design             string
	AcceptanceCriteria string
	Notes              string
	Assignee           string
	// NoInheritLabels keeps a child from copying its parent's labels.
	NoInheritLabels bool
}

// Create creates an issue and returns its id.
func (c *Client) Create(ctx context.Context, repo string, p CreateParams) (string, error) {
	args := []string{"create", "--silent", "--title=" + p.Title}
	add := func(flag, v string) {
		if v != "" {
			args = append(args, flag+"="+v)
		}
	}
	add("--description", p.Description)
	add("--type", p.IssueType)
	if p.Priority != nil {
		args = append(args, "--priority="+strconv.Itoa(*p.Priority))
	}
	add("--parent", p.Parent)
	for _, l := range p.Labels {
		add("--labels", l)
	}
	add("--design", p.Design)
	add("--acceptance", p.AcceptanceCriteria)
	add("--notes", p.Notes)
	add("--assignee", p.Assignee)
	if p.NoInheritLabels {
		args = append(args, "--no-inherit-labels")
	}
	out, err := c.run(ctx, repo, true, args...)
	if err != nil {
		return "", err
	}
	id := lastLine(strings.TrimSpace(string(out)))
	if id == "" || strings.ContainsAny(id, " \t") {
		return "", &Error{Message: "bd create returned no issue id", Stderr: string(out)}
	}
	return id, nil
}

// UpdateParams are the optional fields accepted by Update (nil = unchanged).
type UpdateParams struct {
	Title              *string
	Description        *string
	Design             *string
	AcceptanceCriteria *string
	Notes              *string
	Status             *string
	Priority           *int
	IssueType          *string
	Assignee           *string
	Labels             *[]string // replaces the full set
	// RemoveLabels removes labels; needed to clear the set because bd
	// ignores an empty --set-labels.
	RemoveLabels []string
	// AddLabels adds labels, keeping the rest.
	AddLabels []string
	Parent    *string
	// SetMetadata sets metadata keys; UnsetMetadata removes them.
	SetMetadata   map[string]string
	UnsetMetadata []string
}

// Empty reports whether no field is set.
func (p UpdateParams) Empty() bool { return len(p.flags()) == 0 }

func (p UpdateParams) flags() []string {
	var f []string
	str := func(flag string, v *string) {
		if v != nil {
			f = append(f, flag+"="+*v)
		}
	}
	str("--title", p.Title)
	str("--description", p.Description)
	str("--design", p.Design)
	str("--acceptance", p.AcceptanceCriteria)
	str("--notes", p.Notes)
	str("--status", p.Status)
	if p.Priority != nil {
		f = append(f, "--priority="+strconv.Itoa(*p.Priority))
	}
	str("--type", p.IssueType)
	str("--assignee", p.Assignee)
	if p.Labels != nil {
		for _, l := range *p.Labels {
			f = append(f, "--set-labels="+l)
		}
	}
	for _, l := range p.RemoveLabels {
		f = append(f, "--remove-label="+l)
	}
	str("--parent", p.Parent)
	for _, l := range p.AddLabels {
		f = append(f, "--add-label="+l)
	}
	keys := make([]string, 0, len(p.SetMetadata))
	for k := range p.SetMetadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		f = append(f, "--set-metadata="+k+"="+p.SetMetadata[k])
	}
	for _, k := range p.UnsetMetadata {
		f = append(f, "--unset-metadata="+k)
	}
	return f
}

// Update applies p to an issue with `bd update`.
func (c *Client) Update(ctx context.Context, repo, id string, p UpdateParams) error {
	flags := p.flags()
	if len(flags) == 0 {
		return nil
	}
	args := append([]string{"update"}, flags...)
	args = append(args, "--", id)
	_, err := c.run(ctx, repo, true, args...)
	return err
}

// Close closes an issue with an optional reason.
func (c *Client) Close(ctx context.Context, repo, id, reason string) error {
	args := []string{"close"}
	if reason != "" {
		args = append(args, "--reason="+reason)
	}
	args = append(args, "--", id)
	_, err := c.run(ctx, repo, true, args...)
	return err
}

// CloseForce closes an issue even while issues it depends on are open.
func (c *Client) CloseForce(ctx context.Context, repo, id, reason string) error {
	args := []string{"close", "--force"}
	if reason != "" {
		args = append(args, "--reason="+reason)
	}
	args = append(args, "--", id)
	_, err := c.run(ctx, repo, true, args...)
	return err
}

// Reopen reopens a closed issue.
func (c *Client) Reopen(ctx context.Context, repo, id string) error {
	_, err := c.run(ctx, repo, true, "reopen", "--", id)
	return err
}

// DepAdd records that issue depends on dependsOn with the given type ("" = blocks).
func (c *Client) DepAdd(ctx context.Context, repo, issue, dependsOn, typ string) error {
	args := []string{"dep", "add"}
	if typ != "" {
		args = append(args, "--type="+typ)
	}
	args = append(args, "--", issue, dependsOn)
	_, err := c.run(ctx, repo, true, args...)
	return err
}

// DepRemove removes the dependency edge issue → dependsOn.
func (c *Client) DepRemove(ctx context.Context, repo, issue, dependsOn string) error {
	_, err := c.run(ctx, repo, true, "dep", "remove", "--", issue, dependsOn)
	return err
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return s
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
