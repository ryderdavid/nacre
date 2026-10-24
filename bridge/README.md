# nacred

The Mac-side bridge for Nacre. It discovers beads repositories, wraps the `bd` CLI, and runs
Claude Code headless (`claude -p … --output-format stream-json`) for a bead, streaming the
transcript over SSE. The HTTP contract is in [`../docs/API.md`](../docs/API.md).

## Build, test, run

```sh
make build            # bin/nacred
make test             # go vet + go test
./bin/nacred serve    # or just ./bin/nacred
./bin/nacred pair     # QR code + URL/token for the app
```

Subcommands: `serve` (default), `pair`, `token`, `version`, `install`, `uninstall`, `gates`, `round`.

`nacred gates [dir] [--build] [--skip-final] [--only name] [--json]` runs a repository's
`nacre.yaml` gates (see `docs/API.md`) exactly as nacred does when it checks work: from the
repository root found at or above `dir`, with the file's `env`, timeout and output tail. It exits 1 if
anything fails, printing the end of the failing output. Agents should run it before handing work over.

`nacred round start [--repo dir|name] [--epic ID] [--budget USD] [--minutes N] BEAD...` hands several
beads to the local nacred as a round: it reviews them together, works each on its own branch, checks
and merges them, and finishes on one commit that passes the gates and builds. `nacred round status
[ROUND]`, `answer ROUND QUESTION ANSWER`, `land`, `cancel`, `resume`, `retry ROUND BEAD`, `limits` and
`gc` (remove worktrees of finished rounds) manage rounds. A Claude Code session asked to finish several
beads should start a round rather than do them inline.
Flags: `--config <path>` (default `~/.config/nacred/config.json`), `--listen host:port`.

`make install` (or `nacred install`) copies the binary to `~/.local/bin/nacred`, writes
`~/Library/LaunchAgents/com.ryderdavid.nacred.plist` (RunAtLoad + KeepAlive, PATH with
`/opt/homebrew/bin` and `~/.local/bin`, logs to `~/Library/Logs/nacred.log`) and bootstraps it
into `gui/$UID`. `nacred uninstall` boots it out and removes the plist.

## Configuration

Created on first run with a random token. Fields (defaults in brackets):

| field | meaning |
|---|---|
| `listen` | `host:port`; empty = Tailscale IPv4 `:7420` (waits up to 60 s for Tailscale at boot), else `127.0.0.1:7420` |
| `token` | bearer token (32 random bytes, base64url) |
| `host_id` | stable random id for this computer (generated on first run); lets the app tell computers apart |
| `host_name` | name shown in the app [the hostname] — e.g. "Work laptop" |
| `scan_roots`, `scan_depth` | [`~/Code`, `~/Documents`, `~/Beads`], [4] |
| `exclude` | directory basenames or `~/` paths to skip [node_modules, Library, .Trash, build, …, `~/.traycer`]; dot-directories are never descended, and `~/.beads` (home itself) is never a repo |
| `extra_repos`, `hidden_repos` | repo roots to force in / hide |
| `bd_path`, `claude_path` | resolved from PATH / standard locations |
| `actor` | `bd --actor` for writes ["<git user.name> via Nacre"] |
| `max_concurrent_runs` | [3]; extra runs stay queued |
| `round_budget_usd`, `round_time_limit_minutes` | [25], [240] default limits for a new round; a round pauses at either |
| `max_concurrent_gates` | [2] gate and build commands (`nacre.yaml`) running at once on this computer, across single runs and rounds; set 1 on a laptop |
| `default_mode`, `default_model` | ["auto"], [""] |
| `prompt_template` | `{{id}}`, `{{title}}`, `{{repo}}`, `{{instructions}}` placeholders |
| `data_dir` | run history root [`~/Library/Application Support/nacred`] |
| `adopt_sessions` | [false] watch Claude Code's session registry and mirror agent-started sessions as read-only adopted runs |
| `adopt_poll_seconds` | [10] how often the session registry is polled |
| `adopt_idle_minutes` | [15] how long an adopted session may go without writing its transcript before it counts as idle; past it the session stays on its bead but leaves the runs list and `adopted_runs` |

## Layout

```
cmd/nacred          subcommands, wiring, pairing QR
internal/api        JSON wire types
internal/config     config file, token, Tailscale address
internal/discovery  .beads repo discovery
internal/bd         bd CLI wrapper (argv only, 30 s timeout, per-repo write lock)
internal/repos      repo registry, 2 s issue cache, counts
internal/repoconfig nacre.yaml: gates, build, env, version bump, resources, landing
internal/gates      runs gate and build commands (timeout, process group, output tail, computer-wide limit)
internal/gitx       the git nacred runs itself: worktrees, merges, resets, fast-forwards, pushes
internal/workspace  round worktrees outside the repo on nacre/r/<round>/integration and nacre/r/<round>/<bead>
internal/rounds     rounds: pre-flight, scheduling, verification, follow-ups, merging, landing, leases
internal/claude     claude argv (runs and headless calls) + stream-json → RunEvent parser
internal/assist     one-shot `claude -p` calls for the app's Refine, Draft and assistant
internal/prompt     prompt template rendering
internal/runs       run queue, processes (own process group), persistence
internal/sessions   Claude Code session registry watcher (adopted runs)
internal/sse        SSE writer + broker
internal/server     HTTP handlers, auth, request log
internal/launchd    LaunchAgent install/uninstall
```

Run history lives in `<data_dir>/runs/<run id>/{meta.json,events.jsonl}`; the newest 200 runs are kept.
Runs still active when nacred stops are marked failed on the next start — except adopted runs,
whose processes nacred never owned; those are re-resolved against the live session registry instead.

With `adopt_sessions` on, nacred also watches `~/.claude/sessions/` for Claude Code sessions an
agent started on its own, maps each one to a discovered repo (resolving a git worktree to its main
checkout) and links it to the bead the agent moved: by session name, then by the `bd` command found
in the session transcript, then by bd's claim state. A session is only adopted while its process is
genuinely alive (pid plus process start time, so a reused pid cannot resurrect a dead record), and
nacred's own sessions are skipped. Adopted runs are read-only: cancel and follow-up both answer
409, `can_cancel`/`can_followup` say so up front, and `resume_command` is the take-over affordance
instead. Each poll also tails the session's transcript (`~/.claude/projects/*/<session id>.jsonl`), so
an adopted run carries the agent's text, tool calls and results as events, and its cost, duration and
counts from the transcript's running totals. Activity follows transcript freshness: a session quiet
for longer than `adopt_idle_minutes` is idle and *stale*. They are counted by `Repo.adopted_runs`
(stale ones excepted), never by `active_runs`, and `GET /v1/runs` omits them unless asked
(`?include=adopted`, plus `stale` for the quiet ones). See docs/API.md, "Adopted runs".

## Notes

- When run under launchd, the first scan of `~/Documents` may trigger a macOS privacy prompt
  for `nacred`; allow it (or remove `~/Documents` from `scan_roots`).
- Child `claude` processes get nacred's environment minus Claude Code session markers
  (`CLAUDECODE`, `CLAUDE_CODE_*`), so starting nacred from inside a Claude Code session is safe.
- `/v1/repos/{repo}/claude/{json,stream}` run the app's Claude features on this computer's Claude
  subscription, so the phone needs no API key. They are sandboxed with `--safe-mode` and
  `--permission-prompts none`, and the board assistant gets only read-only `bd` commands: it
  returns the changes it wants as proposals, and the app applies the ones the user approves through
  the ordinary issue routes. See `docs/API.md` (AssistRequest).
