// Package repos maintains the registry of discovered beads repositories and a
// short-lived cache of each repo's issue list, from which summary counts are
// derived.
package repos

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ryderdavid/nacre/bridge/internal/api"
	"github.com/ryderdavid/nacre/bridge/internal/bd"
	"github.com/ryderdavid/nacre/bridge/internal/discovery"
	"github.com/ryderdavid/nacre/bridge/internal/gitinfo"
)

const (
	// IssueTTL is how long a fetched issue list is served from cache.
	IssueTTL = 2 * time.Second
	// rescanAfter triggers a lazy disk rescan on list requests.
	rescanAfter = 10 * time.Minute
	// countConcurrency bounds parallel bd calls when computing counts.
	countConcurrency = 4
	// errorBackoff is how long a failed fetch is cached before retrying, so
	// one broken repo (e.g. a hung embedded Dolt) does not stall every request.
	errorBackoff = 60 * time.Second
	// listWait bounds how long List waits for slow repos before answering
	// with whatever is cached; fetches continue in the background.
	listWait = 5 * time.Second
	// gitTTL is how long a repo's git remote/branch is cached.
	gitTTL = 5 * time.Minute
)

// ErrNotFound is returned for an unknown repo id.
var ErrNotFound = errors.New("unknown repo")

// standardStatuses are always present in Repo.counts.
var standardStatuses = []string{"open", "in_progress", "blocked", "deferred", "closed", "pinned", "hooked"}

// Scanner returns repo root paths (discovery.Scan bound to config).
type Scanner func() []string

// Registry is safe for concurrent use.
type Registry struct {
	scan       Scanner
	bd         *bd.Client
	ActiveRuns func(repoID string) int // optional
	// AdoptedRuns counts agent-owned sessions nacred only watches (optional).
	// It is kept apart from ActiveRuns so active_runs keeps meaning "runs
	// nacred is doing".
	AdoptedRuns func(repoID string) int

	mu        sync.Mutex
	list      []*entry
	byID      map[string]*entry
	scannedAt time.Time
}

type entry struct {
	id, name, path string

	fetchMu   sync.Mutex // held while fetching (dedupes concurrent fetches)
	mu        sync.Mutex // guards the fields below
	issues    []api.Issue
	meta      []issueMeta
	err       error
	fetchedAt time.Time
	prefix    string // from `bd config get issue_prefix`, fetched once
	git       gitinfo.Info
	gitAt     time.Time
}

type issueMeta struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
}

// New creates a registry. The first scan happens lazily.
func New(scan Scanner, client *bd.Client) *Registry {
	return &Registry{scan: scan, bd: client, byID: map[string]*entry{}}
}

// Rescan rediscovers repos on disk, keeping cached issue lists for repos that remain.
func (r *Registry) Rescan() {
	paths := r.scan()
	r.mu.Lock()
	defer r.mu.Unlock()
	byID := make(map[string]*entry, len(paths))
	list := make([]*entry, 0, len(paths))
	for _, p := range paths {
		id := discovery.RepoID(p)
		e := r.byID[id]
		if e == nil {
			e = &entry{id: id, name: filepath.Base(p), path: p}
		}
		byID[id] = e
		list = append(list, e)
	}
	sort.SliceStable(list, func(i, j int) bool {
		return strings.ToLower(list[i].name) < strings.ToLower(list[j].name)
	})
	r.list, r.byID, r.scannedAt = list, byID, time.Now()
}

func (r *Registry) ensureScanned(force bool) {
	r.mu.Lock()
	stale := force || r.scannedAt.IsZero() || time.Since(r.scannedAt) > rescanAfter
	r.mu.Unlock()
	if stale {
		r.Rescan()
	}
}

// Count returns the number of known repos.
func (r *Registry) Count() int {
	r.ensureScanned(false)
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.list)
}

// lookup finds a repo by id, rescanning once if unknown (it may be new on disk).
func (r *Registry) lookup(id string) (*entry, error) {
	r.ensureScanned(false)
	r.mu.Lock()
	e := r.byID[id]
	r.mu.Unlock()
	if e == nil {
		r.Rescan()
		r.mu.Lock()
		e = r.byID[id]
		r.mu.Unlock()
	}
	if e == nil {
		return nil, ErrNotFound
	}
	return e, nil
}

// Path returns the root path of repo id.
func (r *Registry) Path(id string) (string, error) {
	e, err := r.lookup(id)
	if err != nil {
		return "", err
	}
	return e.path, nil
}

// Invalidate drops the cached issue list of repo id (after a write).
func (r *Registry) Invalidate(id string) {
	r.mu.Lock()
	e := r.byID[id]
	r.mu.Unlock()
	if e != nil {
		e.mu.Lock()
		e.fetchedAt = time.Time{}
		e.mu.Unlock()
	}
}

// refresh makes sure e's issue cache is no older than maxAge.
func (r *Registry) refresh(e *entry, maxAge time.Duration) {
	fresh := func() bool {
		e.mu.Lock()
		defer e.mu.Unlock()
		age := maxAge
		if e.err != nil && age < errorBackoff {
			age = errorBackoff
		}
		return !e.fetchedAt.IsZero() && time.Since(e.fetchedAt) <= age
	}
	if fresh() {
		return
	}
	e.fetchMu.Lock()
	defer e.fetchMu.Unlock()
	if fresh() { // another goroutine fetched while we waited
		return
	}
	// Detached from any one request so a disconnecting client does not fail
	// other waiters; bd.Client applies its own timeout.
	ctx := context.Background()
	issues, err := r.bd.List(ctx, e.path)
	e.mu.Lock()
	needPrefix := e.prefix == ""
	e.mu.Unlock()
	prefix := ""
	if err == nil && needPrefix && len(issues) > 0 {
		prefix, _ = r.bd.Prefix(ctx, e.path)
	}
	var meta []issueMeta
	if err == nil {
		meta = make([]issueMeta, 0, len(issues))
		for _, is := range issues {
			var m issueMeta
			_ = json.Unmarshal(is, &m)
			meta = append(meta, m)
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.fetchedAt = time.Now()
	e.err = err
	if err == nil {
		e.issues, e.meta = issues, meta
	}
	if prefix != "" {
		e.prefix = prefix
	}
	if time.Since(e.gitAt) > gitTTL {
		e.mu.Unlock()
		info := gitinfo.Read(ctx, e.path)
		e.mu.Lock()
		e.git, e.gitAt = info, time.Now()
	}
}

// Issues returns repo id's summary and its full issue list (≤ IssueTTL old).
func (r *Registry) Issues(id string) (api.Repo, []api.Issue, error) {
	e, err := r.lookup(id)
	if err != nil {
		return api.Repo{}, nil, err
	}
	r.refresh(e, IssueTTL)
	e.mu.Lock()
	issues, ferr := e.issues, e.err
	e.mu.Unlock()
	if ferr != nil {
		return r.summary(e), nil, ferr
	}
	if issues == nil {
		issues = []api.Issue{}
	}
	return r.summary(e), issues, nil
}

// Summary returns the summary of one repo.
func (r *Registry) Summary(id string) (api.Repo, error) {
	e, err := r.lookup(id)
	if err != nil {
		return api.Repo{}, err
	}
	r.refresh(e, IssueTTL)
	return r.summary(e), nil
}

// List returns all repos with counts, refreshing issue caches concurrently
// (bounded). refresh=true rescans the disk first.
func (r *Registry) List(refresh bool) []api.Repo {
	r.ensureScanned(refresh)
	r.mu.Lock()
	list := append([]*entry(nil), r.list...)
	r.mu.Unlock()

	sem := make(chan struct{}, countConcurrency)
	var wg sync.WaitGroup
	for _, e := range list {
		wg.Add(1)
		go func(e *entry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r.refresh(e, IssueTTL)
		}(e)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(listWait):
	}
	out := make([]api.Repo, 0, len(list))
	for _, e := range list {
		out = append(out, r.summary(e))
	}
	return out
}

// Ref identifies a discovered repo. Unlike Summary it carries no counts, so
// it can be read without touching the issue cache.
type Ref struct {
	ID   string
	Name string
	Path string
}

// Refs returns the discovered repos, scanning the disk lazily. It never calls
// bd, so a caller polling on a timer cannot be stalled by a slow repo.
func (r *Registry) Refs() []Ref {
	r.ensureScanned(false)
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Ref, 0, len(r.list))
	for _, e := range r.list {
		out = append(out, Ref{ID: e.id, Name: e.name, Path: e.path})
	}
	return out
}

// IssueTitle returns the cached title of an issue, if known.
func (r *Registry) IssueTitle(id, issueID string) string {
	e, err := r.lookup(id)
	if err != nil {
		return ""
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, m := range e.meta {
		if m.ID == issueID {
			return m.Title
		}
	}
	return ""
}

func (r *Registry) summary(e *entry) api.Repo {
	e.mu.Lock()
	defer e.mu.Unlock()
	rp := api.Repo{ID: e.id, Name: e.name, Path: e.path, Counts: map[string]int{}}
	for _, s := range standardStatuses {
		rp.Counts[s] = 0
	}
	var last time.Time
	for _, m := range e.meta {
		if m.Status != "" {
			rp.Counts[m.Status]++
		}
		if t, err := time.Parse(time.RFC3339, m.UpdatedAt); err == nil && t.After(last) {
			last = t
		}
	}
	rp.Counts["total"] = len(e.meta)
	if !last.IsZero() {
		rp.LastUpdated = last.UTC().Format(time.RFC3339)
	}
	rp.Prefix = e.prefix
	rp.GitRemote, rp.GitBranch = e.git.Remote, e.git.Branch
	if rp.Prefix == "" && len(e.meta) > 0 {
		ids := make([]string, len(e.meta))
		for i, m := range e.meta {
			ids[i] = m.ID
		}
		rp.Prefix = Prefix(ids)
	}
	switch {
	case e.err != nil:
		rp.Error = e.err.Error()
	case e.fetchedAt.IsZero():
		rp.Error = "loading: bd has not answered yet"
	}
	if r.ActiveRuns != nil {
		rp.ActiveRuns = r.ActiveRuns(e.id)
	}
	if r.AdoptedRuns != nil {
		rp.AdoptedRuns = r.AdoptedRuns(e.id)
	}
	return rp
}

// Prefix guesses the issue-id prefix from ids when bd's issue_prefix config is
// unavailable: the longest common prefix of all ids, cut at its last '-'
// ("GanttGen-eed.4","GanttGen-x1" → "GanttGen"; "gvmcp-7-6","gvmcp-6-1" → "gvmcp").
func Prefix(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	lcp := ids[0]
	for _, id := range ids[1:] {
		n := 0
		for n < len(lcp) && n < len(id) && lcp[n] == id[n] {
			n++
		}
		lcp = lcp[:n]
	}
	if len(ids) == 1 {
		lcp = strings.TrimRight(lcp, "0123456789abcdefghijklmnopqrstuvwxyz.")
	}
	if i := strings.LastIndexByte(lcp, '-'); i > 0 {
		return lcp[:i]
	}
	return ""
}
