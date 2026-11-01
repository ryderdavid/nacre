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
  "active_runs": 1,              // runs nacred dispatched and is running for this repo
  "adopted_runs": 2,             // live agent-owned sessions nacred is only watching; omitted when 0
  "error": "",                    // non-empty when bd failed for this repo (repo still listed)
  "git_remote": "github.com/ryderdavid/nacre", // normalized origin URL, "" if none or a local path
  "git_branch": "main",           // current branch, "" when detached or not a git repo
  "gates": ["bridge", "app tests"], // gate names from the repo's nacre.yaml; omitted when it has none
  "gates_error": ""               // why nacre.yaml can't be used ("nacre.yaml line 4: …"); omitted when fine
}
```

A repository's `nacre.yaml` (at its root) says what "done" means: the gates every piece of work must
pass, the build that ends a round, their environment, the version bump, exclusive resources and how a
round lands. Rounds need it; `nacred gates` runs it from a terminal. Format:

```yaml
gates:                 # required; each must exit 0
  - name: bridge
    run: cd bridge && go test ./...
  - name: lint
    run: ./lint.sh
    final_only: true   # skipped when one bead is checked; runs at the end of a round
build:                 # run at the end of a round
  - name: release apk
    run: cd android && ./gradlew -q assembleRelease
    artifact: android/app/build/outputs/apk/release/app-release.apk
env: {JAVA_HOME: /path}  # added to every gate and build command; "~/" means home
version:
  bump: scripts/bump-version.sh {level}   # {level} is patch or minor
  tag: v{version}      # default when bump is set; "none" turns it off. The landed commit is
                       # tagged (and pushed with it); {version} is the first word after the
                       # bump output's last "->", e.g. 0.6.0
resources: [emulator]  # or {name, id}; exclusive, one run at a time
land: {target: main, auto: false, push: true}       # defaults shown
limits: {gate_timeout_minutes: 20, fix_attempts: 2} # defaults shown
merge:
  regenerate:          # files rebuilt rather than merged, e.g. lockfiles
    - path: android/*.lockfile   # path.Match pattern; "*" doesn't cross "/"
      run: cd android && ./gradlew -q dependencies --write-locks
```
When a round's merge conflicts only in files a round can settle by itself, it does so without Claude:
`.beads/*.jsonl` logs get every line from both sides once, and a `merge.regenerate` file takes the
target's side and is rebuilt by its command. The gates run afterwards either way.
Unknown fields are errors. Commands run with `/bin/sh -c` in the repository (or worktree) root, in
their own process group, killed on timeout; the last 200 lines of output are kept.
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

nacred adds one field of its own, **`active_elsewhere`**, to issues it returns (the issue list,
`GET …/issues/{id}`, and the issue echoed after a write). It is present only when bd says someone
is working on the bead but nothing on this computer is:

- the status is `in_progress` or `hooked`, and
- `assignee` is set and is not nacred's configured `actor`, and
- no live run on this computer is linked to the bead — neither one nacred dispatched nor a working
  adopted session. A **stale** adopted session does not count: left open is not working, so the
  claim in bd is the better signal, and
- the claim is not one a Nacre run made. The agent in a run nacred dispatches claims its bead under
  the git user, not the actor, and may leave it in progress when it finishes; so a claim made at or
  before the end of the bead's latest dispatched run here is taken to be that run's. A claim made
  after it ended is reported.

```json
"active_elsewhere": {"assignee": "codex", "since": "2026-10-24T10:00:00Z", "self": false}
```

`since` is the bead's `started_at`, else its `updated_at`. `self` is `true` when the assignee is the
person nacred acts for — the actor without its ` via Nacre` suffix, e.g. `Ryder Cobean` for the
default actor `Ryder Cobean via Nacre` — meaning their own work outside Nacre (another computer, a
terminal, an agent without adoption). It is a **marker, not a run**: there is no transcript and
nothing to cancel or follow up. It covers agents nacred cannot adopt (anything but Claude Code and Codex), computers
with `adopt_sessions` off, and sessions that ended or were never adopted. A client that also knows
of a live run linked to the bead on another paired computer should show that run's marker instead.

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

A run a **round** started adds these (all omitted otherwise):
```jsonc
{
  "round_id": "rd_01…",
  "kind": "bead",                  // "bead": work on one bead; "integration": a merge or finishing turn
  "branch": "nacre/r/rd_01…/NAC-1", // the branch nacred made for it
  "workspace": "/Users/ryder/Library/Application Support/nacred/worktrees/<repo>/<round>/NAC-1",
  "base": "bd299b7…",              // commit the branch started from
  "verification": {                // nacred's own check once the turn ended
    "ok": false, "reason": "gate \"app tests\" failed with exit status 1",
    "uncommitted": [], "commits": 2,
    "gates": [{"name": "app tests", "ok": false, "exit_code": 1, "duration_ms": 41200, "tail": "…"}],
    "at": "2026-10-24T09:00:00Z"
  }
}
```

A run dispatched to **Codex** (`RunRequest.agent` `"codex"`) carries the same fields plus
`"agent": "codex"`. Its `session_id` is the Codex thread id, `resume_command` is
`cd '…' && codex resume <thread id>`, `model` is `""` unless one was asked for (Codex's configured
default), and `cost_usd` stays 0 because Codex reports no cost; `duration_ms` is wall time. Its
events are the same kinds a Claude run has: Codex's messages are `text`, its reasoning `thinking`,
and its commands, file changes, MCP calls, web searches and plans are `tool_use`/`tool_result`
pairs named like Claude Code's tools (`Bash`, `Edit`, `mcp__server__tool`, `WebSearch`,
`TodoWrite`), so a client shows both alike. Cancel and follow-up work as for a Claude run (a
follow-up is `codex exec resume`).

A run nacred dispatched carries exactly the fields above. An **adopted** run — a session an agent
started on its own, which nacred only watches (see [Adopted runs](#adopted-runs)) — adds these.
Every one of them is omitted for a dispatched run, so a client written against the fields above
gets back byte-for-byte what it got before adopted runs existed:

```json
{
  "origin": "adopted",             // "adopted"; absent means nacred dispatched the run
  "agent": "claude-code",          // tool that owns the session; absent means Claude Code
  "session_name": "fix-the-parser", // the name the agent gave its session, as Claude Code recorded it
  "owner_pid": 54321,              // the process that owns the session — not a child of nacred
  "activity": "working",           // "working" | "idle" | "ended"; reported beside status, see below
  "stale": true,                   // alive but no transcript write within adopt_idle_minutes; absent otherwise
  "last_activity_at": "2026-10-23T19:53:01.123Z", // newest transcript entry read; absent until there is one
  "can_cancel": false,             // absent means allowed
  "can_followup": false,           // absent means allowed
  "can_send_live": true,           // POST /v1/runs/{run}/live is available; absent means it is not
  "live_send_unavailable": "",     // why not, in a sentence, on an adopted Claude Code run; absent when it is
  "peer_messages": [               // newest 20 live sends, oldest first; absent when none
    {"msg_id": "…", "text": "also run the tests", "sent_at": "…",
     "status": "held",             // sent | held | denied | expired | delivered | refused | dropped | unconfirmed
     "reason": "…",                // the session's own explanation, when it gave one
     "confirmed": true,            // the session's transcript shows it (origin.msg_id matches)
     "confirmed_at": "…", "idle_at": "…"}
  ]
}
```

`activity` is reported **beside** `status`, not inside it: `status` stays one of the five values
above so existing status handling keeps working, and an adopted session that is alive but parked
waiting for input is `"status": "running"` with `"activity": "idle"`. `"ended"` means the owning
process is gone, which nacred reports as `"status": "succeeded"` with no `error` — nobody reports a
result for a session nacred did not run, so the status means "over, nothing failed on our side" and
`activity` carries the truth.

`activity` follows the session's transcript, not just its registry record: a session counts as
`"working"` while its transcript keeps changing, and as `"idle"` once Claude Code marks it parked
or it has gone `adopt_idle_minutes` (default 15) without writing. The second case also sets
`"stale": true` — the session is alive but left open, not a job in progress — see
[Adopted runs](#adopted-runs) for what that changes.

`can_cancel` and `can_followup` are **absent** on a dispatched run and `false` on an adopted one.
Treat an absent flag as `true`: it means "no restriction beyond the usual status rules", which are
unchanged (cancelling a finished run is still a no-op 200; a follow-up while a run is active is
still a 409).

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
| GET | `/v1/ping` (no auth) | – | `{"name":"nacred","version":"0.6.0","commit":"<sha>","host_id":"h_…","host_name":"Mac-Studio"}` |
| GET | `/v1/health` | – | `{"name","version","commit","hostname","host_id","host_name","bd_version","claude_version","codex_version","codex_unavailable","repos":n,"active_runs":n,"features":["claude","codex"]}` — `version` comes from git when nacred is built: the release tag (`0.6.0`), how far past it the build is (`0.6.0-3-gabc1234`), `-dirty` for uncommitted changes, or `dev` when it wasn't built through `make`; `commit` is the full commit it was built from; `active_runs` excludes adopted runs; `features` has `codex` when the Codex CLI is installed and signed in, and otherwise `codex_unavailable` says why in a sentence |
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
| POST | `/v1/repos/{repo}/claude/json` | AssistRequest | `{"session_id","output":{…},"text","cost_usd"}` |
| POST | `/v1/repos/{repo}/claude/stream` | AssistRequest | SSE (below) |
| GET | `/v1/assist/{turn}/stream[?after=seq]` | – | SSE: reattach to a streaming answer (404 once it has expired) |
| POST | `/v1/assist/{turn}/cancel` | – | 204; stops that answer's claude process |
| GET | `/v1/runs[?repo={repo}&issue={id}&active=1&include=adopted[,stale]]` | – | `{"runs":[Run]}` newest first; adopted runs only with `include=adopted`, stale ones only with `stale` too |
| GET | `/v1/runs/{run}` | – | `{"run":Run,"events":[RunEvent]}` |
| GET | `/v1/runs/{run}/stream[?after=seq]` | – | SSE (below) |
| POST | `/v1/runs/{run}/cancel` | – | `Run` (queued → `cancelled` at once; running → returned still `running`, final `cancelled` arrives via SSE/GET within ~5 s; finished → unchanged, 200; **409 for an adopted run**) |
| POST | `/v1/runs/{run}/followup` | `{"message":"…","mode":"auto"}` | `Run` (409 if running/queued or no session_id; **409 for an adopted run**) |
| POST | `/v1/runs/{run}/live` | `{"message":"…"}` | `Run` with the send in `peer_messages`; **409** unless the run is a live adopted session with `can_send_live`, or if the send fails. See [Live sends](#live-sends) |
| GET | `/v1/events` | – | SSE: global `run`, `repo` and `change` notifications |
| POST | `/v1/sessions` | Claude Code hook payload | `{"accepted":bool,"reason"?}` (202): a hook telling nacred about a session so the adoption poll runs now. See [Session hints](#session-hints) |
| GET | `/v1/attention` | – | `{"items":[AttentionItem]}`: Needs you for every repository on this computer (below) |
| GET | `/v1/config` | – | `{"default_mode","default_model","prompt_template","modes":[…]}` |

`{repo}` is a Repo `id`. `{id}` is the bead id.

### CreateIssue
`{"title" (required), "description", "issue_type", "priority", "parent", "labels": [], "design",
"acceptance_criteria", "notes", "assignee", "related": [], "follow_on_of", "found_comment": {bead,
comment_id}}` → `bd create --silent …`, then the created issue is returned from a fresh list.

A child of a spec epic (a parent labelled `specflow:<change-id>`) is created with
`--no-inherit-labels` and gets `found:<change-id>`, so it is ordinary work rather than an unmapped
linked bead. `related` adds a `related` dependency from the new bead on each listed one;
`follow_on_of` closes that found bead with "Made a follow-on change: <new>"; `found_comment` posts the
answered marker for that `Found:` comment. A failure in these links is logged; the bead stays
created.

### UpdateIssue
Any subset of `{"title","description","design","acceptance_criteria","notes","status","priority",
"issue_type","assignee","labels","parent"}`. `labels` replaces the full set (`--set-labels`).
Setting `status` to `closed` uses `bd close`; changing a closed issue to another status reopens it first.
Long text fields are passed via flags (not a shell), so any content is safe.

### Stalled runs
A running run whose `claude` has printed nothing for `stall_minutes` (config, default 15; negative
turns the check off) gets `"stalled": true` and `"stalled_since"` (when it last printed), a system
event saying it may be stuck, and a `run` notification on `/v1/events`. nacred does not stop it: the
person decides whether to wait, or cancel and follow up. The flag clears as soon as `claude` prints
again or the turn ends. The window is longer than Claude Code's longest foreground tool call
(10 minutes), so a slow build is not mistaken for a hang.

### Adopted runs
When `adopt_sessions` is enabled in nacred's config, nacred also watches Claude Code's own session
registry (`~/.claude/sessions/`) and Codex's rollout files (`~/.codex/sessions/`), and mirrors
sessions an **agent started on its own** as runs with `"origin": "adopted"`. They are linked to the
bead the agent moved; a session that moved none is reported with an empty `issue_id` rather than a
guessed link. `agent` names the tool that owns the session (`claude-code` or `codex`),
`session_name` and `owner_pid` identify it, and `activity` says whether it is working, parked or
gone.

A **Codex** session has no pid (`owner_pid` is absent) — Codex keeps no session registry — so its
liveness comes from its rollout file instead:

- it is `"working"` while a turn is open and the rollout keeps changing, and `"idle"` between turns
  or after `adopt_idle_minutes` without a write (the latter also `"stale": true`);
- it ends (`"activity": "ended"`) once the rollout has gone **twice** `adopt_idle_minutes` without
  a write, or, for a `codex exec` session, as soon as its one turn completes or is aborted, because
  `codex exec` exits then. A session that writes again after ending resumes the same run;
- its repo is the one its starting directory is in or, when that is outside every repo (the Codex
  desktop app starts threads in the home directory), the one its commands run in: such a session is
  adopted once a command runs in a discovered repo, with its whole rollout, and stays with that repo;
- `session_name` is the thread title from `~/.codex/session_index.jsonl`, when Codex gave it one;
- `cost_usd` stays 0 (Codex writes token counts, not cost), and `resume_command` is
  `cd … && codex resume <session_id>`. Helper threads Codex spawns for another thread (approval
  reviewers, subagents) are never adopted.

**They are read-only, and the API says so explicitly.** nacred owns no process for an adopted run,
so:

- `POST /v1/runs/{run}/cancel` → **409**. Signalling the session would kill work nacred did not
  start — in practice someone's live interactive terminal.
- `POST /v1/runs/{run}/followup` → **409**. `claude -p --resume <session>` against a live session
  forks it rather than continuing it, so the follow-up would land in a copy nobody is watching.
- Both refusals carry the reason in `error`, and neither touches the run or the session.
- `can_cancel` and `can_followup` are `false`, so a client knows this without having to try. A
  client should hide both controls for a run with `"origin": "adopted"`, and should not treat such
  a run as work in progress for notification or foreground-service purposes — the "run" may be a
  terminal a human leaves open for hours.
- **`resume_command` is the take-over affordance.** It is the one thing a client can usefully offer
  for an adopted run: the `cd … && claude --resume <session_id>` (or `codex resume`) line to run on
  the Mac to pick the session up by hand, instead of nacred reaching into it.

Adopted runs also stay out of nacred's own accounting: they never enter the queue, never occupy a
`max_concurrent_runs` slot, and are **not** counted by `active_runs` on `Repo` or `/v1/health`,
which keep meaning "runs nacred is doing". `Repo.adopted_runs` counts them separately, leaves out
stale ones, and is omitted when zero.

Everything else works as usual: `GET /v1/runs/{run}` and `/v1/runs/{run}/stream` serve an adopted
run by id, with the session's transcript as `RunEvent`s. Its first event is a `system` notice saying
what was found, where, which signal linked it to a bead, and that it is read-only. After it come the
agent's own `text`, `thinking`, `tool_use`, `tool_result` and prompt (`user`) events, read from the
on-disk transcript each poll and stamped with the transcript's own times; subagent turns are left
out. `cost_usd`, `duration_ms`, `tool_calls`, `num_turns` (assistant turns), `turns` (prompts),
`model` and `last_text` come from the transcript's running totals. `result_text` stays empty: a
transcript has no result entry. Only the newest 1000 events are kept in memory.

#### Live sends
An adopted **Claude Code** session that publishes a messaging socket can be handed a message
without forking it: `POST /v1/runs/{run}/live` with `{"message":"…"}`. nacred sends it over the
session's own session-to-session channel (Claude Code's `messagingSocketPath`, peerProtocol 1,
NDJSON over a Unix socket), prefixed with `<actor> (via Nacre): `. The session queues it as a turn
of its own, shown to whoever is at the terminal, and runs it under that session's permission mode.
It arrives as a **peer message**, not as the user typing: it cannot answer a pending permission
prompt and is never run as a slash command. A client must make it an explicit action and say so.

It is offered only behind a version guard, re-checked on every send, and `can_send_live` is `true`
only when the record half passes: `kind` is `interactive`, the record's `pidDomain` is this
computer's, `messagingSocketPath` is set and the socket exists, `peerProtocol` is exactly 1, and
`entrypoint` is not `claude-desktop` (desktop-app sessions are untested). At send time nacred also
checks that the pid is alive with the start time the record claims and that the socket accepts a
connection. Anything else — and any send error — is a **409** with the reason, and the run stays
read-only. `POST .../followup` is unaffected: it stays a 409 for every adopted run.

What became of a send is reported on the run, in `peer_messages` and as `system` events:

- `status` starts as `sent` and then takes the session's receipt: `held` (waiting for the
  session's user to approve it; a session in bypassPermissions holds every peer message),
  `delivered` (queued), `denied`, `expired`, `refused` (the session does not take peer messages)
  or `dropped`. Receipts reach nacred on its own inbox socket, so a restart of nacred loses those
  still to come.
- `confirmed` is set once the session's transcript shows a peer-origin user entry with the send's
  `msg_id`: the message reached Claude. This is the delivery confirmation; a receipt alone is not.
- `unconfirmed` means neither a receipt nor the transcript said anything within two minutes.
  nacred never resends on its own. A later transcript entry still sets `confirmed`.
- `idle_at` is when the session said it was idle again afterwards (`notify_when_idle`, when the
  session offers it): a hint that the turn finished, nothing more.

#### Session hints
`POST /v1/sessions` lets a Claude Code hook (`nacred hook`, an opt-in install step; see the bridge
README) tell nacred about a session the moment something happens in it, instead of waiting up to
`adopt_poll_seconds` for the next poll. Same bearer auth as every other route. The body is Claude
Code's hook payload as is; nacred reads:

| field | meaning |
|---|---|
| `session_id` | required; the Claude Code session id (400 if missing or malformed) |
| `transcript_path` | the session's transcript. Used instead of globbing `~/.claude/projects` when it is an absolute `.jsonl` path |
| `cwd` | the session's working directory; used only when the registry record has none |
| `hook_event_name` | informational (`SessionStart`, `PostToolUse`, `Stop`) |
| `tool_name`, `tool_input.command` | for `"tool_name":"Bash"`, the command that ran. A `bd update <id> --claim` (or `--status in_progress`) links the session to that bead (the run's `system` event says `linked to <id> (hook)`), replacing an earlier link; a `bd close <id>` links a session that has no bead yet. The id must be a bead of the session's repo |

Everything else in the payload, such as `tool_response`, is ignored. The answer is **202**
`{"accepted":true}` once the hint is queued, and the watcher polls at once. A hint never adopts a
session on its own: the session must still have a live record in `~/.claude/sessions/`, be in a
discovered repo and not be one of nacred's own, exactly as for polling, so the hook and the poll
for one session produce **one** adopted run. When the record has not appeared yet (SessionStart can
run first), nacred looks again every second for a few seconds; a hint for a session that never
shows up is dropped after 15 s. With `adopt_sessions` off the route is a no-op: 202
`{"accepted":false,"reason":"adopt_sessions is off"}`; a hint for a session nacred launched itself
is likewise not accepted.

**Listing is opt-in.** `GET /v1/runs` omits adopted runs unless `include=adopted` is passed, because
a client that does not know about them would offer controls that cannot work. `include` takes a
comma-separated list and ignores names it does not know. A **stale** adopted run (`"stale": true`)
is a second opt-in: it is listed only with `?include=adopted,stale`, so the plain adopted list holds
the sessions doing work right now. A client that shows each bead's agent session — working or idle —
asks for both and keeps stale runs out of its runs list and counts itself. Fetching one by id is not
gated: a client that was handed an adopted run's id can always follow it.

### Specification workflow

| Method | Path | Request | Response |
| --- | --- | --- | --- |
| GET | `/v1/repos/{repo}/issues/{id}/change` | – | `{available, reason?, change}`; the change is nullable. Epic, seed aliases and section beads resolve the same workflow. |
| POST | `/v1/repos/{repo}/issues/{id}/change` | `{action, revision?, feedback?, change_name?, recorded_review?}` | The same envelope; long operations run independently of the HTTP connection. |
| POST | `/v1/repos/{repo}/intake` | `{title, description?, design?, acceptance?, type?, parent?}` | IntakeVerdict (below). One bounded, read-only call; writes nothing. |
| POST | `/v1/repos/{repo}/changes/{change}/found` | FoundRequest | `{bead_id?, prefill?, change?}`: answers discovered work on a promoted change (below). |
| GET | `/v1/repos/{repo}/changes` | – | `{changes:[ChangeSummary], total, openspec}`: `openspec` says the repository's OpenSpec setup is valid (New effort, intake and the Proposals lane apply); the repository's changes for the board, at most 50, changes still being planned (no epic yet) first, then newest. `total` counts all of them. A bridge without specification workflows answers an empty list. |

Actions are `develop`, `revise`, `approve`, `refresh`, `resume` and `adopt`. `develop` may carry
`intake: {verdict, change_name?}`, the New effort verdict it was confirmed with; the change keeps it
as `intake`, and when the author's full reconciliation picks a different route its history records
"Intake suggested …; reconciliation chose … because …". The author's route stands. `approve` must name the exact
reviewed `revision`; it promotes/synchronizes Beads but never starts code execution. Conflicts return
409, unavailable prerequisites 503. `GET` reports unsupported prerequisites as `available:false`
with a reason. All routes require the ordinary bearer authentication.

`adopt` takes over an active change written outside Nacre (by hand or in a CLI session) through a
bead without a workflow, which becomes its seed and, on approval, its epic. It names the change in
`change_name`. The change's files are kept as written. tasks.md only gets `nacre-section` markers for
headings that lack one; existing markers are kept. Sections are linked to beads that already exist
under the seed whose title is the section heading, alone or after the change name ("my-change 1.
Save drafts"). The change is validated strictly, then gets one independent review. With
`recorded_review: true`, its `review.md` stands in for that review: no AI call is made, and approval
is still the owner's. When the review finds blocking problems, the change stops at
`needs_attention` and is not redrafted. `revise` (or `resume`) lets the author address the findings.
The first approval of an adopted change keeps sections that were already finished closed (every task
in them checked), and records a linked bead's own text in a comment before its fields become the
section contract. Those sections count as landed evidence while their tasks stay checked and their
beads stay closed. 409 when the change is already tracked, the bead already has a change, the change
lacks a canonical artifact, or `recorded_review` is set without a `review.md`. For a bead without a
workflow, `GET` also carries `adoptable`: the repository's untracked active changes as
`{change_name, why, tasks_done, tasks_total, has_review, suggested, problem?}`. `suggested` means the
bead names the change, or parents beads named after it. `problem` says why the change can't be
adopted as it stands. An adopted change carries
`adoption: {at, recorded_review, completed, linked}` and the route `adopted`.

`change` contains `id`, `seed_id`, `epic_id`, `change_name`, `phase`, `revision`, `approved_revision`,
`summary`, `route`, `rationale`, `references`, `artifacts`, `findings`, `sections`, `retired_sections`,
`history`, `evidence`, `cost_usd`, `updated_at`, an optional `error`, and while the phase is `stale`
a `stale_reason` naming what broke the approval: the files that changed since review ("AGENTS.md
changed", "Active change X changed", "Accepted spec Y changed", "tasks.md was edited after approval…"),
"Seed bead N changed", "Section bead N was edited…", or a generic reason when the record has no
baseline. Approval is never restored without a new review (`revise`). The envelope also carries
`inventory: {accepted_specs, active_changes}` (what a proposal is compared with) whenever the
repository's OpenSpec setup can be read. Artifacts are exact
`{path,content}` pairs. Sections are `{id,title,body,bead_id,depends_on}`; their IDs come from
`<!-- nacre-section: ID -->` markers, never heading numbers. Findings carry
`{id,severity,summary,detail,disposition}`. History entries are `{at,kind,text}`.

Phases: `drafting`, `reviewing`, `needs_attention`, `awaiting_approval`, `promoting`, `ready`,
`stale`, `interrupted`, `covered`. Routes: `new`, `amend`, `repair`, `covered`, `adopted`. Coverage is a reviewed
read-only recommendation and does not create an epic or close the seed. A proposal action permits
at most three author/reviewer passes, $5 total and 30 minutes. The bridge persists checkpoints and
does not silently resume a paid call after restart.

Start implementation through `POST /v1/repos/{repo}/rounds` with approved child `beads`,
`spec_change_id` and `spec_revision`. A stale or omitted revision, missing workflow, linked epic as
a task, or mixed contract selection is refused. One or more current section beads may be selected.
The round stores `spec_contracts`, keyed by bead ID, including exact artifact content and section
intent. It installs those artifacts in its integration workspace before creating task workspaces.
Automatic landing is disabled for these rounds. Generic run creation/follow-up cannot bypass the
round's revision guards; spec runs expose `can_followup:false`.

`Run` and `RoundBead` optionally carry `spec_change_id`, `spec_revision`, `spec_section_id`.
Evidence entries contain `{section_id,bead_id,run_id?,round_id?,round_state?,verified,build_verified,human_check,landed,note}`.
`verified` records a passing section run; `build_verified` additionally requires that the section
is integrated into the round's final commit with passing final gates and build.
Only a passing verification of the current approved revision counts; unknown, reopened or changed
work never becomes successful from a checkbox or status alone. Landing, human acceptance and
OpenSpec archive are separate obligations; this workflow never closes the epic automatically.

`IntakeVerdict` is `{verdict, change_name?, rationale, references:[{path, requirement?}], cost_usd,
fallback?, target_tracked, target_active?, active_changes}` (`active_changes` names every active change, for overriding the verdict). `verdict` is `bead` (changes no specified "done": chores,
refactors, small fixes, bugs that restore specified behavior), `new`, `amend` (`change_name` is an
active change) or `covered`. The call sees the requirement index and each active change's name and
`## Why` (24 KB together), has no tools, and is capped at $0.50 and 90 s. An `amend` or `covered`
verdict that doesn't cite a listed requirement (or, for `amend`, a file of the named active change)
becomes `new`, with `fallback` saying why. `target_tracked` is true when Nacre has a workflow record
for the amended change; only then can `target_active` name the round or run using it (a change
written by hand is not tracked). A repository without valid OpenSpec answers 503; a failed, timed
out or unreadable call answers 502 with `{error, cost_usd}`.

### Found work
`FoundRequest` is `{answer, note?, found_in?, before_section?, comment_id?, found_beads?}` for the
change `{change}` (its `id`), which must have an epic.
- `how_order`: a bead under the epic labelled `found:<change-id>`, created with
  `--no-inherit-labels`, with a `discovered-from` dependency on `found_in` when given. It is ordinary
  work and leaves the approved plan unchanged. `before_section` (a current section id) also makes that
  section's bead depend on it (`blocks`), which doesn't stale the section.
- `amend`: the same, also labelled `found-amend`: a pending change to the plan, refused by run and
  round creation ("a pending amendment …") and skipped by spec drift. It shows in Needs you until it
  is revised into the change or made a follow-on.
- `revise`: `found_beads` lists open `found-amend` beads of the change. Their descriptions are joined
  into one revision's feedback (refused over 12000 characters); `revise` is refused as usual while the
  change has active work. The change records them as `pending_found`; once that revision is approved
  and synchronized they are closed with a link to it. A revision started any other way clears
  `pending_found`, so they return to Needs you.
- `out_of_scope`: creates nothing; returns `prefill: {title, description, related: [epic],
  found_comment?}` for New effort.
- `dismiss`: needs `comment_id` and `found_in`; posts the answered marker.
With `comment_id` (a `Found:` comment on `found_in`), the note defaults to the comment's text, a
created bead's description ends "Found in: <bead>; source comment: <id>", and nacred replies on
`found_in` with `[nacre-found:<comment-id>] answered: … → <bead>`. That marker is what keeps an
answered comment answered. Spec round agents are told to stay in their section and report found
work as a `Found:` comment on their bead instead of doing it or creating beads.

`ChangeSummary` is `{id, change_name, phase, revision?, approved_revision?, epic_id?, seed_ids,
sections, error?, stale_reason?, pending_found?, active?, updated_at}`; `active` names the round or
run implementing the change. `seed_ids` lists the original seed first, then any later seeds that
amended the change. Each section is `{id, title, bead_id?, evidence}`, `evidence` one of `none`,
`in_round` (a round that hasn't landed or been cancelled is working on it), `verified` (a passing run
of the current approved revision) or `landed`, folded from the evidence entries above. Evidence is
cached per change revision and recomputed when a round in the repository moves.

Workflow journals live in the bridge data directory's `changes/` folder. Back this up with Beads
and OpenSpec files: stable `specflow:` and `spec-section:` labels are recovery identities, not a
replacement for the journal. Lost records fail closed. External file edits invalidate review;
manual edits to generated bead fields are preserved and require reconciliation before retry.

### RunRequest
`{"mode": "auto|acceptEdits|plan|bypassPermissions|dontAsk", "model": "", "instructions": "extra text appended to the prompt", "worktree": false, "prompt": "full override (rare)", "agent": "claude-code|codex"}`
All fields optional; defaults come from config.

`agent` `"codex"` runs the bead with the OpenAI Codex CLI (`codex exec --json`) instead of Claude
Code; `""` or `"claude-code"` is Claude Code. It answers 409 with `codex_unavailable`'s reason as
`detail` when Codex can't run on this computer. For Codex, `default_model` (a Claude model) is not
applied, and `mode` picks Codex's sandbox, since `codex exec` asks no one for approval: `plan` is
read-only; `acceptEdits` and `dontAsk` may write in the repository without network; `auto` may
also use the network; `bypassPermissions` runs with no sandbox.

### AssistRequest
`{"prompt" (required), "system", "schema": {…}, "model": "", "effort": "low|medium|high|xhigh|max",
"session_id": "", "bd_reads": false, "read_repo": false, "keep": false, "needs_you": false}`

Runs one `claude -p` in the repo on **this computer's Claude subscription**, which is how the app's
Refine, Draft and board-assistant features work without an API key on the phone. The app owns the
prompt, the system prompt and the JSON schema, so both of its backends return the same shapes.

The call is sandboxed: `--safe-mode` (no hooks, MCP servers, CLAUDE.md, skills or plugins),
`--permission-prompts none` so nothing can block on a prompt nobody can answer, and no tools at all
unless `bd_reads` is set — which grants only read-only `bd` subcommands
(`bd list|show|comments|ready|blocked|stats|search|stale|orphans|dep tree`). `read_repo` also lets
it read the repository: `Read`, `Grep`, `Glob` and read-only `git log|diff|show|status|branch` and
`ls`; the bead chat's Ask mode uses it to answer questions about the code. Writes are refused:
the assistant returns the changes it wants as proposals and the app applies the approved ones
through the routes above, so every change still goes through the user. `keep` saves the session so
the next turn can continue it with `session_id` (`--resume`).

`needs_you` appends this computer's Needs you list (at most 8 KB: kind, repository, ids, title and
why per item) to the prompt, for the board assistant on this backend, which can't call a read tool.

`output` is the answer validated against `schema` (the CLI's `structured_output`); `text` is the raw
final message. A failure is a 502 with `{"error"}` carrying Claude's own wording.

### SSE
`/v1/repos/{repo}/claude/stream`: first `event: turn` with `{"turn_id"}`, then `id: <seq>` +
`event: event` + RunEvent JSON for each step as it happens (thinking, the `bd` commands and their
output), then exactly one terminal message — `event: done` with the same body as `/claude/json`, or
`event: error` with `{"error"}`. Keep-alive `: ping` every 15 s.

The answer runs independently of the request. If the connection drops, claude keeps going; the
client reattaches with `GET /v1/assist/{turn}/stream?after=<last seq>` (or `Last-Event-ID`), which
replays the missed events and then follows live, ending with the same `done`/`error`. A finished
answer stays readable for 10 minutes. `POST /v1/assist/{turn}/cancel` stops one the person no
longer wants; its stream then ends with `event: error`.

`/v1/runs/{run}/stream`: first replays stored events with `seq > after`, then sends the current
`event: run` snapshot, then streams live ones.
Each message: `id: <seq>`, `event: event`, `data: <RunEvent JSON>`. Whenever run metadata changes
(status, cost, session id): `event: run`, `data: <Run JSON>`. When the run is no longer
active, the server sends a final `event: run` and keeps the stream open (a follow-up may resume it)
until the client disconnects. Keep-alive comment `: ping` every 15 s. Honors `Last-Event-ID`.

`/v1/events`: `event: run` + Run JSON on any run change; `event: repo` + `{"repo_id":"…"}` after
any write through the bridge and whenever a run finishes (Claude may have edited beads);
`event: change` + `{"repo_id","change_id","change_name","phase","seed_id","epic_id"}` on every
specification change phase transition.

### Needs you (`/v1/attention`)
Every item waiting on Ryder on this computer, derived from current state on each request (each
repository's beads and changes are re-read at most every 30 s, sooner after a `repo` event). The app
merges computers and dedupes by `id`, a digest of the kind and the underlying record and its state.
Items are ordered questions and decisions first, then approvals, then landing and archive, newest
first within each.

`AttentionItem`: `{id, kind, repo, repo_name, bead_id?, change_id?, round_id?, title, why,
source_ref?, options, created_at}`. `why` is one or two plain sentences that stand alone. Each option
is `{type:"action", label, method, path, body?, disabled?}`, an exact request to send as is (when
`disabled` is set it says why the action can't be used yet), or `{type:"open", label, target}`, a
navigation where `target.kind` is `bead`, `change` (`id` is the seed bead; `revision` the one to
review) or `round` with `id`, or a sheet to open with a prefill: `found_something` (`change`,
`found_in`, `comment_id`, `note`) or `new_effort` (`note`, `related`, `follow_on_of`).

| Kind | Source | Options |
|---|---|---|
| `bead_decision` | open bead labelled `human` or typed `decision` | open bead |
| `proposal_ready` | change `awaiting_approval` | open change at its revision; `approve` with that revision |
| `review_blocked` | change `needs_attention` | open change |
| `approval_stale` | change `stale`, including a ready or awaiting change whose live check fails when the list is built; `why` is its `stale_reason` | `revise`; open change |
| `workflow_interrupted` | change `interrupted` | `resume`; open change |
| `round_question` | each unanswered question of a round in `needs_decision` | each answer via `POST /v1/rounds/{round}/answers`; open round |
| `bead_parked` | parked bead of an active round | `POST /v1/rounds/{round}/beads/{id}/retry`; open round |
| `round_ready` | round `ready_to_land` | `POST /v1/rounds/{round}/land`; open round |
| `found_work` | an unanswered `Found:` comment on a spec section bead that has finished in a round (any end state); or an open `found-amend` bead not carried by a revision under way | comment: Answer (opens `found_something`), Dismiss (action), open bead; `found-amend` bead: Revise now (action with every such bead of the change; `disabled` while the change has active work; `why` states how many landed sections a revision reopens), Make it a follow-on change (opens `new_effort`), open bead |
| `change_archivable` | approved change whose every current section has `landed` evidence, every section bead is closed, and no `found:<change-id>` bead is open | Finish: archive and merge (`POST /v1/repos/{repo}/epics/{epic}/finish`); open change |
| `epic_finishable` | an open epic, not a change's, with at least one bead under it and all of them closed | Finish epic (`POST /v1/repos/{repo}/epics/{epic}/finish`); open epic |
| `bead_stuck` | a non-epic bead `in_progress` for 24 hours with no active run (adopted sessions included) and in no active round | Work until done (`POST /v1/repos/{repo}/rounds` with the bead and `land_when_ready`); Back to Ready (`PATCH` the bead to `open`); open bead |

A repository whose beads can't be read contributes no items; its board shows the error.

## Rounds
A round carries several beads in one repository from a pre-flight review to one commit that passes
the repository's `nacre.yaml` gates and builds (see `openspec/changes/semi-attended-rounds`). It runs
on the computer that has the repository, without any client connected.

| Method | Path | Body | Response |
|---|---|---|---|
| POST | `/v1/repos/{repo}/rounds` | `{"beads":["NAC-1",…] \| "epic":"NAC-9", "target", "budget_usd", "time_limit_minutes", "land_when_ready"}` | `Round` (201); 409 with a reason when the repo has no `nacre.yaml`, a bead is closed or missing, a bead is in another active round, or the beads belong to different epics |
| POST | `/v1/repos/{repo}/epics/{id}/finish` | – | `{"round": Round}` (201): a round that finishes the epic; or `{"closed": true}` (200) when its work is already on the landing target and it has no change to archive. 409 while beads under it are open or a round still works on its branch |
| GET | `/v1/rounds[?repo={repo}&active=1]` | – | `{"rounds":[Round]}` newest first |
| GET | `/v1/rounds/{round}` | – | `{"round":Round,"events":[RoundEvent]}` |
| GET | `/v1/rounds/{round}/stream[?after=seq]` | – | SSE: `event: event` (RoundEvent, `id: seq`) for log entries after `after`, and `event: round` (Round) whenever it changes; honours `Last-Event-ID` |
| POST | `/v1/rounds/{round}/answers` | `{"question_id","answer"}` | `Round` |
| POST | `/v1/rounds/{round}/plan` | `{"unlink":[["NAC-2","NAC-1"]], "accept_dependencies":[["NAC-2","NAC-1"]]}` | `Round`; unlink only before the bead starts |
| POST | `/v1/rounds/{round}/limits` | `{"budget_usd","time_limit_minutes"}` | `Round`; a round stopped by a limit resumes |
| POST | `/v1/rounds/{round}/choose` | `{"choice", "budget_usd", "time_limit_minutes"}` | `Round`; carries out one of `stop.choices`. `budget_usd` is the extra budget for `retry-with-budget` and the new budget for `raise-limits`. 409 when the round isn't stopped, the choice isn't offered, or `continue` is chosen while nothing has changed since the failure |
| POST | `/v1/rounds/{round}/land` | – | `Round`; 409 unless `ready_to_land` |
| POST | `/v1/rounds/{round}/cancel` | – | `Round` (branches are kept) |
| POST | `/v1/rounds/{round}/resume` | – | `Round`; the `continue` choice: 409 unless `paused`, at a limit, or when nothing has changed since the failure |
| POST | `/v1/rounds/{round}/beads/{id}/retry` | – | `Round`; puts a parked or skipped bead back |

**Epic branches.** A round lands where its beads belong: beads under an epic land on the epic's
branch, `epic/<lower-case id>`, made from `land.target` the first time and pushed with `land.push`;
other beads land on `land.target`. One round can't mix epics (409). Epic-branch rounds neither bump
nor tag the version. Finishing an epic is a round with no beads (`finish_epic` names the epic) that
starts from the epic's branch, or from `land.target` when nothing is left on the branch but the
epic's change still needs archiving. For a change's epic it runs `openspec archive <change> --yes`
in the round's workspace and commits it. Then it runs every gate and the build with one minor
version bump, lands on `land.target` without waiting, pushes, closes the epic, deletes its branch
here and on `origin`, and the change's workflow ends in the phase `archived`. `land_when_ready` (or
`land.auto` in `nacre.yaml` for rounds without specification sections) lands a round as soon as it
is verified; the round reports it as `auto_land`.

`/v1/events` also carries `event: round` with the Round on every change.

### Round
```jsonc
{
  "id": "rdtnf62ywld", "repo_id": "…", "repo_name": "Nacre", "repo_path": "/Users/ryder/Code/Nacre",
  "epic": "",                       // set when started from an epic
  "state": "implementing",          // planning | needs_decision | implementing | integrating | ready_to_land | landed | cancelled | paused
  "paused_from": "", "pause_reason": "",     // pause_reason repeats stop.reason for older clients
  "stop": {                         // only while paused: why, and the ways forward
    "kind": "merge-conflict",       // merge-conflict | gates-failed | git-error | bump-failed | review-failed | limits | nothing-integrated | config | legacy
    "reason": "main moved since the round started and Claude couldn't merge it in: …",
    "phase": "land",                // the step a choice goes back to: preflight | final | land | "" (the paused-from state)
    "bead": "", "branch": "",       // set when the stop is about one bead or a pre-flight branch
    "diagnosis": {"summary": "…", "cause": "…", "cost_usd": 0.04}, // Claude's read-only account; absent when it couldn't be made
    "choices": [{"id": "retry-with-budget", "label": "Let Claude try again", "explanation": "…"}],
                                    // ids: retry-with-budget | skip-branch | skip-bead | continue | raise-limits | cancel
    "attempted": ["fix:version", "turn:land-merge×2", "retry:2"],
    "fingerprint": "…",             // the state it failed in; continue is refused while unchanged
    "spend": [{"bead":"NAC-1","spent_usd":8.1,"done":true}], "estimate_usd": 8.1 // limits stops only
  },
  "target": "main", "base": "bd299b7…",          // branch it started from and lands on; commit it started at
  "branch": "nacre/r/rdtnf62ywld/integration", "workspace": "…/worktrees/<repo>/rdtnf62ywld/integration",
  "beads": [{
    "id": "NAC-1", "title": "…", "type": "feature", "priority": 2,
    "state": "running",             // waiting | running | verifying | integrating | integrated | parked | skipped
    "after": ["NAC-0"],             // round beads it waits for (bd dependencies and the plan's orderings)
    "needs": ["emulator"],          // exclusive resources it leases
    "branch": "nacre/r/…/NAC-1", "workspace": "…", "base": "…", "run_id": "r_…",
    "fix_attempts": 0, "stalls": 0, "verification": {…}, "merge_commit": "",
    "reason": "",                   // why it was parked or skipped
    "human_check": "",              // what only a person can verify; such beads stay open
    "closed": false,
    "carried_from": "",             // an earlier round's kept branch this bead's work continues from
    "picked_up_by": ""              // the later round that continued this bead's kept branch
  }],
  "plan": {"summary": "…", "beads": [{"id","areas","files","depends_on","needs","human_check"}],
           "findings": [{"id":"f1","kind":"overlap","beads":["NAC-1","NAC-2"],"branch":"","files":[],"text":"…","accepted":false,"unlinked":false}],
           "cost_usd": 0.4},
  "questions": [{"id":"q1","finding_id":"f2","kind":"contradiction","beads":[…],"branch":"","text":"…","options":["…"],"answer":"","answered_at":"","problem":""}], // problem: why an earlier answer couldn't be carried out (a merge that conflicted or broke the gates); the question is open again
  "integration": {"bead":"NAC-2","phase":"conflict","pre_merge":"…","run_id":"r_…","attempts":1,"problem":"…",
                  "branch":"","rebump":false}, // the merge in progress, if any. phase: merge | conflict | gates | final for the
                                    // round's own merges; preflight-merge (branch set) for unmerged work Ryder chose to bring
                                    // in; land-merge | land for a target that moved before landing
  "budget_usd": 25, "spent_usd": 3.1, "overhead_usd": 0.04, // overhead: diagnoses, part of spent "time_limit_minutes": 240, "mode": "auto", "model": "",
  "run_ids": ["r_…"],               // every run the round started
  "created_at": "…", "started_at": "…", "updated_at": "…", "ended_at": "",
  "final_commit": "", "version": "0.3.3 -> 0.4.0", "artifacts": ["…/rounds/<id>/artifacts/app-release.apk"],
  "landed_commit": "", "tag": "v0.4.0", "pushed": false, "auto_land": false, "error": ""
}
```
Finding kinds: `contradiction` and `ambiguity` (asked as questions), `overlap` and `ordering` (become
`after` sequencing), `shared-artifact` (runs are told to leave it alone), `unmerged-work` (asked: merge
the branch into the round first, ignore it, or stop).

`spec-drift` (asked): one bead would change behavior a requirement specifies. Only raised in
repositories with a valid OpenSpec setup, never for spec section beads or beads labelled `found:`,
and only when the finding cites a requirement from nacred's index of accepted specs and active
changes (at most 24 KB; whole capabilities left out are listed in the plan's `spec_unchecked`). The
finding and question carry `requirement: {capability, title, path, line}`. The answers are fixed:
`Amend the spec first` (nacred starts proposal development for the bead with the requirement as
feedback and leaves the bead out; if development is refused the question reopens with that reason as
`problem` and the bead stays), `Proceed as an exception` (a decision comment and a
`spec-touch:<capability>` label on the bead, which stays ordinary work) and `Leave it out of this
round` (the bead is skipped with a reason starting "Left out"; beads that build on it are skipped as
having built on one that "was left out"). A question's `applied` is set once its answer is carried out.

### RoundEvent
`{"seq", "ts", "kind", "bead", "text", "commit", "gates":[GateResult]}`; kind is one of `state`,
`plan`, `question`, `answer`, `bead`, `verify`, `merge`, `conflict`, `gates`, `build`, `land`, `note`,
`recover` (something the round fixed or retried by itself: a mechanical fix, a recovery turn, a retry
after a transient error, continuing a bead's earlier work).

Before a round stops it tries, in order: a mechanical fix (the version re-bump at landing, the beads
logs, `merge.regenerate` files); a recovery turn in which Claude settles the merge or fixes the gates in
the integration worktree, judged by the gates (bead merges, unmerged work brought in before the beads
start, and a target that moved before landing all get one); and up to two retries, after 5 s and
30 s, for environmental errors (a locked index or database, a dropped connection, a process that
can't start). Budget and time limits stop at once. A bead that an earlier round parked, or left on
cancel, continues from that round's kept branch the next time a round includes it; the kept branch is
deleted once that round lands.

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


## Daemon updates

`GET /v1/daemon/update` reads the persistent updater status. `POST /v1/daemon/update` with `{}` requests the latest stable release of the fixed `ryderdavid/nacre` repository. Both require the normal pairing bearer token. No repository, ref, shell command or force option is accepted. `health.features` includes `daemon-update` when this protocol exists; `health.commit` reports build provenance. `nacred version --json` returns version, commit and `update_protocol: 1`.

A successful start returns HTTP 202 with the status object. Repeating a request while active returns the same operation. Busy work or an unsupported installation returns 409 with a user-facing error; unknown request fields return 400. Older daemons return 404/405/501 and need one manual bootstrap installation. During maintenance, other mutations return 503; authenticated reads and update status remain available. Maintenance admission is serialized with mutation admission and checks owned runs, all nonterminal rounds and unfinished round cleanup, assistant calls and specification jobs.

Status fields: `available`, `id`, `host_id`, `phase`, `message`, `active`, `current_version`, `current_commit`, `target_version`, `target_commit`, `started_at`, `updated_at`. Current identity comes from the daemon answering the request. Phases include `idle`, `unavailable`, `checking`, `building`, `restarting`, `rolling_back`, `updated`, `current`, `failed`, `rolled_back`, `recovery_failed` and `interrupted`. Only `updated` with matching current/target version and commit confirms the new release; `current` means no installation was needed. Clients preflight GET to record the prior operation ID and reconcile a lost POST response using GET, never an automatic repeated POST. Historical terminal status with that same ID does not confirm a new request.

The standard macOS LaunchAgent installation is required. The helper is a separate LaunchAgent, uses the existing local gh authentication, resolves the stable tag to a commit, downloads bounded source, builds with `-buildvcs=false`, and verifies the candidate's declared version/commit/update protocol. It preserves the old binary, replaces atomically, restarts the daemon and checks authenticated health against exact version, commit and host ID. Failed verification attempts restoration and separately reports whether the old daemon verified. Source staging is removed; status and `previous-nacred` remain in the `updates` data directory. The operation is bounded; stale or unreadable state never implies success and requires local inspection.

No published release is an explicit failure; main is never a substitute. The updater does not publish releases, force-stop active work, update Android, alter Git checkouts, change pairing configuration, or automatically queue a retry for busy/offline hosts.
