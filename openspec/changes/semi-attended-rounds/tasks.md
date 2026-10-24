Gates used below: **G-bridge** = `cd bridge && go vet ./... && go test -race ./...`;
**G-app** = `cd android && ./gradlew -q testDebugUnitTest`; **G-build** = `cd android && ./gradlew -q assembleRelease`
(with `JAVA_HOME` set to Android Studio's JBR).

## 1. Repository gates (`nacre.yaml`)

- [x] 1.1 Parse and validate `nacre.yaml` (gates, build, env, version, resources, land, limits) in a new `internal/repoconfig` package, with errors that name the field and line — G-bridge
- [x] 1.2 Expose whether a repository has a valid `nacre.yaml` (and its gate names) on the Repo API entry; update `docs/API.md` — G-bridge
- [x] 1.3 Gate runner: run a named command in a directory with env and timeout, kill the process group on timeout, keep exit status, duration and the last 200 lines; a computer-wide `max_concurrent_gates` limit (config, default 2) that queues rather than fails — G-bridge
- [x] 1.4 Add Nacre's own `nacre.yaml` and `scripts/bump-version.sh <patch|minor>` (versionCode +1; versionName patch or minor, minor resetting the patch) — G-bridge, then run the new gate runner against it by hand
- [x] 1.5 `nacred gates [dir] [--build]`: run a repository's gates (and build) from the terminal exactly as nacred will, so agents and people can check before handing work over — G-bridge

## 2. Workspaces

- [ ] 2.1 Create the integration branch and worktree at a recorded base, outside the repository, via `bd worktree create`; record path, branch and base — G-bridge
- [ ] 2.2 Create a bead branch and worktree from the integration branch's current head — G-bridge
- [ ] 2.3 Let a run start in a given working directory without `--worktree`; add `round_id`, `branch`, `workspace`, `base`, `verified` to Run and `docs/API.md` — G-bridge
- [ ] 2.4 Remove worktrees after integration and branches after landing; startup sweep for orphans; `nacred round gc` — G-bridge

## 3. Verification and follow-up

- [ ] 3.1 Verify a finished round run: clean tree, commits beyond base, gates pass; record the result on the run — G-bridge
- [ ] 3.2 Follow up a failed verification in the same session with the specific failure, up to `fix_attempts`, then park — G-bridge (fake claude that fixes on the second turn, and one that never does)
- [ ] 3.3 Recover a stalled round run once (cancel + resume), park on the second stall — G-bridge
- [ ] 3.4 Round run prompt contract (bead, decisions, gates, commit on branch, the must-nots) — G-bridge (golden test of the rendered prompt)

## 4. Rounds core

- [ ] 4.1 `internal/rounds`: round and bead state machines, persistence (`round.json`, `events.jsonl`), reload on startup — G-bridge
- [ ] 4.2 Create a round from bead ids or an epic, with the refusals in the spec; label beads `round:<id>` — G-bridge
- [ ] 4.3 Scheduler: start beads respecting bd dependencies, the plan's orderings and concurrency — G-bridge
- [ ] 4.4 Budgets: per-round USD and wall-clock limits, `--max-budget-usd` per turn, pause and resume — G-bridge
- [ ] 4.5 Post plan entries, decisions, verification and integration outcomes as bead comments — G-bridge
- [ ] 4.6 Cancel a round — G-bridge

## 5. Pre-flight

- [ ] 5.1 Pre-flight prompt and JSON schema (per-bead areas, implied dependencies, findings with the five kinds) run read-only in plan mode — G-bridge (schema and prompt tests, fake claude returning a canned plan)
- [ ] 5.2 Turn contradictions and ambiguities into questions; `needs_decision` until all answered; record answers on beads and in later prompts — G-bridge
- [ ] 5.3 Turn overlaps and orderings into sequencing; propose bd dependencies; accept reordering and unlinking before implementation — G-bridge
- [ ] 5.4 Mark shared artifacts (version etc.) as round-owned and tell runs not to touch them — G-bridge
- [ ] 5.5 Collect unmerged branches (local and remote, excluding active round branches) with their changed files, give them to the review, and turn matching `unmerged-work` findings into questions; "merge it first" merges into the integration branch and re-runs the gates before any bead starts — G-bridge (temp repo with an unmerged branch touching a bead's file)

## 6. Integration and landing

- [ ] 6.1 Serialized `--no-ff` merge by nacred into the integration worktree, gates after each merge — G-bridge (temp git repos in tests)
- [ ] 6.2 Integration turn for conflicts and post-merge failures; reset and park after `fix_attempts` — G-bridge
- [ ] 6.3 Finish: one version bump (`minor` if any integrated bead is a feature or epic, else `patch`), final gates and build, record final SHA and artifacts, close beads (keeping human-check beads open); parked beads don't block finishing and stay open labelled `round-parked` — G-bridge
- [ ] 6.4 Land: fast-forward target, merge-and-reverify if it moved, push when configured, `auto` landing — G-bridge

## 7. Exclusive resources

- [ ] 7.1 Leases: one holder per resource, queueing outside concurrency, release on turn end, revoke on stall — G-bridge
- [ ] 7.2 Pass the leased identity in prompt and `NACRE_RESOURCE_<NAME>`; honour `needs:<resource>` labels — G-bridge

## 8. API and CLI

- [ ] 8.1 Routes: create, list, get, stream, decisions, reorder, land, cancel, retry bead; SSE `round` events on `/v1/events`; `docs/API.md` — G-bridge
- [ ] 8.2 `nacred round start|status|land|cancel|gc` against the local nacred; document it in `bridge/README.md` so a Claude Code session can delegate a round — G-bridge

## 9. App

- [ ] 9.1 Models and BridgeClient/NacreRepository calls for rounds; round events in the event stream — G-app
- [ ] 9.2 Multi-select on boards and "Work these until done" (disabled without `nacre.yaml`), computer and budget choice — G-app
- [ ] 9.3 Rounds list and round screen (bead states, spend, live integration log, links to runs) — G-app
- [ ] 9.4 Decisions sheet: questions with suggested answers, plan ordering, reorder and unlink — G-app
- [ ] 9.5 Land screen with confirmation; notifications for decision needed, bead parked, paused, ready to land — G-app
- [ ] 9.6 Assistant `start_round` write tool — G-app, G-build

## 10. Proving it

- [ ] 10.1 End-to-end test in Go: a temp repo with `nacre.yaml`, three beads (two overlapping, one depending on another), a scripted fake claude; the round ends `ready_to_land` with all three integrated and the gates green — G-bridge
- [ ] 10.2 Dogfood on Nacre: a real round of three small beads, landed from the phone; record what needed Ryder and why — G-bridge, G-app, G-build
