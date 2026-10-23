# nacred HTTP API (v1)

`nacred` runs on the Mac Studio and is the only thing the Nacre Android app talks to for
beads data and Claude Code runs. It wraps the `bd` CLI (one process per call, `bd -C <repo root> … --json`)
and launches Claude Code headless (`claude -p … --output-format stream-json --verbose`) under the
user's own subscription login.

- Base URL: `http://<tailscale-ip>:7420` (default listen address is the Mac's Tailscale IPv4).
- Auth: every `/v1/*` route except `/v1/ping` requires `Authorization: Bearer <token>`.
  401 on a missing or wrong token. Token comparison is constant-time.
- Content type: `application/json; charset=utf-8` both ways. Field names are snake_case.
- Errors: non-2xx with body `{"error": "human readable message", "detail": "optional stderr/tail"}`.
  400 bad input, 401 auth, 404 unknown repo/issue/run, 409 conflict (e.g. follow-up while running),
  502 `bd`/`claude` failed, 500 anything else. (bd rejecting a value, e.g. an unknown status or
  type, is reported as 400 with bd's message.) Unknown routes and wrong methods are JSON 404s.
- Timestamps: RFC 3339 UTC strings.

## Types

### Repo
```json
{
  "id": "3f9a1c0b2d4e",          // first 12 hex chars of sha256(absolute repo root)
  "name": "GanttGen",             // basename of repo root
  "path": "/Users/ryder/Code/GanttGen",
  "prefix": "GanttGen",           // issue id prefix if known (from first issue id), else ""
  "counts": {"open": 3, "in_progress": 1, "blocked": 0, "deferred": 0, "closed": 12,
             "pinned": 0, "hooked": 0, "total": 16},
  "last_updated": "2026-07-18T11:38:59Z",   // max updated_at across issues, "" if none
  "active_runs": 1,
  "error": "",                    // non-empty when bd failed for this repo (repo still listed)
  "git_remote": "github.com/ryderdavid/nacre", // normalized origin URL, "" if none or a local path
  "git_branch": "main"            // current branch, "" when detached or not a git repo
}
```

### Issue
Passed through **verbatim** from `bd list --all --limit 0 --json` (so new bd fields flow to the
app untouched). Always present: `id`, `title`, `status`, `priority` (int 0–4), `issue_type`.
Commonly present: `description`, `design`, `acceptance_criteria`, `notes`, `assignee`, `owner`,
`labels` (string[]), `created_at`, `updated_at`, `closed_at`, `close_reason`, `parent`,
`dependencies` (`[{"issue_id","depends_on_id","type","created_at"}]` — outgoing edges; `type` is
`blocks`, `parent-child`, `related`, `discovered-from`, …), `dependency_count`, `dependent_count`,
`comment_count`, `due_at`/`defer_until` when set.

Statuses: `open`, `in_progress`, `blocked`, `deferred`, `closed`, `pinned`, `hooked`
(plus any custom statuses configured in the repo).

### Comment
`{"id": "…", "issue_id": "NAC-013", "author": "Ryder Cobean", "text": "…", "created_at": "…"}`

### Run
```json
{
  "id": "r_01JB…",                 // sortable unique id
  "repo_id": "3f9a1c0b2d4e", "repo_name": "GanttGen", "repo_path": "/Users/ryder/Code/GanttGen",
  "issue_id": "GanttGen-eed.4", "issue_title": "Finish Auth0-enabled deployment…",
  "status": "queued|running|succeeded|failed|cancelled",
  "mode": "auto",                   // claude --permission-mode
  "model": "",                      // claude --model ("" = Claude Code default)
  "worktree": false,
  "session_id": "uuid or empty until init",
  "created_at": "…", "started_at": "…", "ended_at": "…",
  "cost_usd": 0.42, "duration_ms": 183000, "num_turns": 17, "turns": 1,   // turns = prompts sent (1 + follow-ups)
  "tool_calls": 23,
  "last_text": "most recent assistant text, ≤ 280 chars (for list rows / notifications)",
  "result_text": "final result text of the latest turn",
  "error": "",
  "resume_command": "cd '/Users/ryder/Code/GanttGen' && claude --resume <session_id>",
  "session_cwd": "/Users/ryder/Code/GanttGen"   // cwd Claude reported at init (differs for worktree runs); omitted until init
}
```

### RunEvent
```json
{"seq": 12, "ts": "…", "kind": "text", "text": "…"}
```
`kind` is one of:
- `user` — prompt or follow-up sent to Claude (`text`).
- `system` — session init and other system notices (`text`, e.g. "Session started · model claude-…").
- `text` — assistant text block (`text`, markdown).
- `thinking` — assistant thinking summary (`text`), if present.
- `tool_use` — `tool` (name, e.g. `Bash`, `Edit`), `tool_id`, `input` (raw JSON object),
  `text` (one-line human summary, e.g. the Bash command or file path).
- `tool_result` — `tool_id`, `text` (truncated to 4000 chars), `is_error`.
- `result` — end of a turn: `text` (result), `is_error`.
- `stderr` — a line of process stderr (`text`).
- `status` — run status change (`status`).

`seq` starts at 1 and increases monotonically for the life of the run (follow-ups continue it).

## Routes

| Method | Path | Body | Response |
|---|---|---|---|
| GET | `/v1/ping` (no auth) | – | `{"name":"nacred","version":"0.1.0","host_id":"h_…","host_name":"Mac-Studio"}` |
| GET | `/v1/health` | – | `{"name","version","hostname","host_id","host_name","bd_version","claude_version","repos":n,"active_runs":n}` |
| GET | `/v1/repos[?refresh=1]` | – | `{"repos":[Repo]}` (refresh=1 rescans disk) |
| GET | `/v1/repos/{repo}/issues` | – | `{"repo":Repo,"issues":[Issue]}` — all issues incl. closed |
| GET | `/v1/repos/{repo}/issues/{id}` | – | `{"issue":Issue,"comments":[Comment]}` |
| POST | `/v1/repos/{repo}/issues` | CreateIssue | `{"issue":Issue}` (201) |
| PATCH | `/v1/repos/{repo}/issues/{id}` | UpdateIssue | `{"issue":Issue}` |
| POST | `/v1/repos/{repo}/issues/{id}/close` | `{"reason":"…"}` (optional) | `{"issue":Issue}` |
| POST | `/v1/repos/{repo}/issues/{id}/reopen` | – | `{"issue":Issue}` |
| POST | `/v1/repos/{repo}/issues/{id}/comments` | `{"text":"…"}` | `{"comments":[Comment]}` (full list after add) |
| POST | `/v1/repos/{repo}/deps` | `{"issue_id","depends_on_id","type":"blocks"}` | `{"ok":true}` — `type` ∈ blocks, tracks, related, parent-child, discovered-from, until, caused-by, validates, relates-to, supersedes (default blocks) |
| DELETE | `/v1/repos/{repo}/deps?issue_id=&depends_on_id=` | – | `{"ok":true}` |
| POST | `/v1/repos/{repo}/issues/{id}/runs` | RunRequest | `Run` (201) |
| GET | `/v1/runs[?repo={repo}&issue={id}&active=1]` | – | `{"runs":[Run]}` newest first |
| GET | `/v1/runs/{run}` | – | `{"run":Run,"events":[RunEvent]}` |
| GET | `/v1/runs/{run}/stream[?after=seq]` | – | SSE (below) |
| POST | `/v1/runs/{run}/cancel` | – | `Run` (queued → `cancelled` at once; running → returned still `running`, final `cancelled` arrives via SSE/GET within ~5 s; finished → unchanged, 200) |
| POST | `/v1/runs/{run}/followup` | `{"message":"…","mode":"auto"}` | `Run` (409 if running/queued or no session_id) |
| GET | `/v1/events` | – | SSE: global `run` and `repo` notifications |
| GET | `/v1/config` | – | `{"default_mode","default_model","prompt_template","modes":[…]}` |

`{repo}` is a Repo `id`. `{id}` is the bead id.

### CreateIssue
`{"title" (required), "description", "issue_type", "priority", "parent", "labels": [], "design",
"acceptance_criteria", "notes", "assignee"}` → `bd create --silent …`, then the created issue is
returned from a fresh list.

### UpdateIssue
Any subset of `{"title","description","design","acceptance_criteria","notes","status","priority",
"issue_type","assignee","labels","parent"}`. `labels` replaces the full set (`--set-labels`).
Setting `status` to `closed` uses `bd close`; changing a closed issue to another status reopens it first.
Long text fields are passed via flags (not a shell), so any content is safe.

### RunRequest
`{"mode": "auto|acceptEdits|plan|bypassPermissions|dontAsk", "model": "", "instructions": "extra text appended to the prompt", "worktree": false, "prompt": "full override (rare)"}`
All fields optional; defaults come from config.

### SSE
`/v1/runs/{run}/stream`: first replays stored events with `seq > after`, then sends the current
`event: run` snapshot, then streams live ones.
Each message: `id: <seq>`, `event: event`, `data: <RunEvent JSON>`. Whenever run metadata changes
(status, cost, session id): `event: run`, `data: <Run JSON>`. When the run is no longer
active, the server sends a final `event: run` and keeps the stream open (a follow-up may resume it)
until the client disconnects. Keep-alive comment `: ping` every 15 s. Honors `Last-Event-ID`.

`/v1/events`: `event: run` + Run JSON on any run change; `event: repo` + `{"repo_id":"…"}` after
any write through the bridge and whenever a run finishes (Claude may have edited beads).

## Several computers
Each computer runs its own nacred. `host_id` is a stable random id generated on first run (stored in
the config file), and `host_name` is the display name (`host_name` in the config, else the hostname).
Clients key repos and runs by `(host_id, id)`: repo and run ids are only unique per computer. Repos on
different computers with the same non-empty `git_remote` are the same repository.

## Pairing
`nacred pair` prints a terminal QR code and the URL
`nacre://pair?url=http%3A%2F%2F100.110.97.44%3A7420&token=<token>&name=<host_name>&host_id=<host_id>`.
The app scans it (or the user types URL + token).

## Implementation notes (nacred 0.1.0)

Behaviour clients should rely on, as verified against bd 1.2.2 and Claude Code 2.1.263:

- **Optional issue keys are omitted, not empty.** bd leaves out `labels`, `dependencies`,
  `assignee`, `closed_at`, `close_reason`, `parent`, … when unset (an issue whose labels were
  cleared has no `labels` key). Treat missing as empty/null.
- **Dependency `created_at` timestamps are local wall-clock time with a `Z` suffix** (bd quirk;
  e.g. `2026-10-22T20:08:08Z` for an edge created at 20:08 PDT). Issue/comment timestamps are real UTC.
- Issue ids are hash-style (`NAC-m4v`) or hierarchical (`NAC-013.7`); some repos use dashes inside
  ids (`gvmcp-7-6`). `Repo.prefix` comes from `bd config get issue_prefix`, so do not split ids on `-`.
- `PATCH` with `"status":"closed"` closes with bd's default reason (`close_reason: "Closed"`); use
  `/close` to give a reason. `"labels": []` clears all labels.
- Comments added through the bridge (and issues it creates) carry the configured actor as
  author / `created_by` (default `"<git user.name> via Nacre"`).
- `Repo.counts` always has the seven built-in statuses plus `total`, and also any custom status
  present in the repo. `Repo.error` may be `"loading: bd has not answered yet"` when a repo's bd is
  slow (`GET /v1/repos` answers within ~5 s with whatever is cached); a failing repo is retried at
  most once a minute.
- Run `status` events are emitted for every transition, including `queued` at creation and at each
  follow-up, so a follow-up turn's events start `status:queued`, `user`, `status:running`, `system`, ….
- `event: run` (per-run stream and `/v1/events`) is sent on significant changes only: status,
  session id, end-of-turn result (cost/duration/num_turns/result_text) and error. `last_text` and
  `tool_calls` change continuously but are only visible in the next `run` message or `GET`.
- A cancelled run keeps any `session_id` it got, so it can be resumed with a follow-up; it reports
  `error: "cancelled"` and no cost for the interrupted turn. Runs active when the bridge stops are
  `failed` with `error: "bridge stopped"` (clean shutdown) or `"bridge restarted"` (found on startup).
- `RunRequest.prompt` (override) gets the same `{{id}}`/`{{title}}`/`{{repo}}`/`{{instructions}}`
  substitution as the configured template; instructions are appended when it has no placeholder.
