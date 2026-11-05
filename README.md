# Nacre

A native Android (Material You) kanban for [beads](https://github.com/steveyegge/beads) issue trackers, built for foldables, with Claude in two roles:

- **Claude API on the phone** (your API key): chat about a bead: ask questions read-only, or switch to Refine for edits you review field by field, in one thread per bead; turn a rough note into a full bead, and a board assistant that can read the board and propose edits, new beads and dependencies. The chat bar on the boards screen is the same assistant over every project at once: ask about one of them, or line up work across them. You approve every change.
- **Claude Code on the Mac Studio** (your subscription): send a bead to `nacred`, which runs `claude -p` in that repo. The live transcript streams to the phone; you can cancel, send follow-ups, or copy a `claude --resume` command to take over on the Mac.

```
 Android (Nacre)  ──Tailscale/HTTP+SSE──▶  nacred (Mac Studio, LaunchAgent)
   board, detail,                            ├─ bd -C <repo> … --json   (every .beads repo it finds)
   runs, settings                            └─ claude -p … --output-format stream-json
   └─ Claude API (anthropic-java) for Refine / Draft / Assistant
```

## Layout

| Path | What |
|---|---|
| `bridge/` | `nacred`, the Go service on the Mac (see `bridge/README.md`) |
| `android/` | The app: Kotlin, Jetpack Compose, Material 3 Expressive |
| `docs/API.md` | The bridge's HTTP/SSE contract |

## Setup

1. **Mac:** `cd bridge && make install` installs `~/.local/bin/nacred` and the LaunchAgent `com.ryderdavid.nacred`. It listens on the Mac's Tailscale IP, port 7420. Config is `~/.config/nacred/config.json`, logs are `~/Library/Logs/nacred.log`.
2. **Pair:** run `nacred pair` on the Mac and scan the QR code from Nacre › Settings › *Scan pairing code*, or enter the URL plus `nacred token` by hand.
3. **Claude API key (optional):** Settings › API key. It is encrypted with an Android Keystore key and only sent to the Claude API. The default model is Claude Opus 5.5. Server-side refusal fallbacks are on for the models that support them.
4. **Phone build:** `cd android && ./gradlew assembleRelease`, then sideload `app/build/outputs/apk/release/app-release.apk`. Debug builds use `assembleDebug`.

## Several computers

Run `nacred` on each Mac (`cd bridge && make install`, set `host_name` in its config if you like) and pair each one from Settings (the gear on Boards) › *Add a computer*. Boards from every computer are listed together, tagged with the computer they live on; Activity shows where each run ran.

Settings › **Update connected computers** asks every reachable Mac to install the latest published stable Nacre GitHub release. Each computer shows its own progress or reason it cannot update. Busy computers keep their work; retry after their runs, rounds, assistant turns or specification jobs finish. Updating idle computers briefly disconnects them while the daemon restarts, then verifies the exact release and computer identity. Failed health checks restore the previous binary.

This requires one initial installation of an update-capable nacred LaunchAgent on each Mac, plus Go and GitHub CLI (`gh`) authenticated for `ryderdavid/nacre`. The updater downloads the release's exact source commit into private staging; it does not alter a working checkout. A stable release must be published with a `vMAJOR.MINOR.PATCH` tag and include update protocol 1. If there is no release, the app reports that condition; it never substitutes the main branch. It does not update the Android app. State and rollback live in the nacred data directory's `updates/` folder. An interrupted helper or failed recovery requires checking the saved rollback on that Mac.

A repository checked out on more than one computer (same git `origin`, SSH host aliases resolved) shows as **one board**. Its *home* computer — first paired, or chosen from the board's menu under *Beads from* — is where the board's beads are read and edited. *Run with Claude Code* then offers **Run on** with each computer that has the checkout, so heavy work can go to the Studio and lighter work to the laptop. A run on a computer whose copy of the beads lacks that bead fails with a clear message: sync the beads (git or a Dolt remote) first.

## Agent sessions

When `adopt_sessions` is on in `nacred`'s config, the Mac also watches Claude Code sessions an **agent
started on its own** — claiming a bead from the terminal, say — and Nacre lists them in Activity
beside its own, marked **Agent** with what the session is doing (*working*, *waiting for input*,
*session ended*). The transcript, cost and bead link are all there, read live from the session's
own transcript. A session that has written nothing for 15 minutes (`adopt_idle_minutes`) is left
open rather than working: it drops out of Activity and its count, but its bead still shows the
agent, as idle.

They are read-only, because the Mac owns no process for them: there is no *Cancel* and no follow-up
box, signalling someone's live terminal or resuming it would fork their work. *Copy “take over on
Mac” command* is the way in — it puts the `claude --resume` line on the clipboard. For the same
reason an agent session never posts a run-finished notification and never keeps the foreground
watcher service alive: quitting a terminal you left open for an afternoon is not a run of ours
finishing.

## Getting work done

Nacre has one way to finish work: open it and tap its one primary button.

- **Start work** on a bead. nacred runs it as a round of one bead: a goal-driven agent works in its own
  worktree until the bead's acceptance criteria hold, nacred checks the result itself against
  `nacre.yaml`, then merges it into its target (`main`, or `epic/<id>` under an epic), pushes when
  `nacre.yaml` says so and closes the bead.
- **Run epic** on an epic. Every open child gets its own goal-driven agent in its own worktree, all at
  once where their dependencies allow. Each passes nacred's checks and merges into `epic/<id>`; once no
  child is left open, nacred finishes the epic by itself: it merges the branch into `main` with every
  gate, the build and one version bump, archives the epic's OpenSpec change when it has one, pushes,
  closes the epic and deletes its branch. **Finish epic** appears instead when every child is already
  closed. When such an epic has no branch and no OpenSpec change, there is nothing to merge, so Finish
  epic simply closes it, in any repository, even one without `nacre.yaml`.
- **Run an agent manually…**, in a bead's overflow, starts a plain Claude Code or Codex run with your
  choice of agent, computer, permissions, model and worktree. It leaves checking, committing and
  closing to you.

While it works, a round reviews its beads before any code is written and asks you only when they
genuinely disagree, sequences overlaps itself, sends a failing worker back with exactly what failed,
and fixes what it can (version bumps that collide, lockfiles, merges Claude can settle, a locked file
worth retrying) before it stops. When it does stop, Inbox says what's wrong in plain words with the
choices: let Claude try again with more budget, carry on after you've fixed it, set a bead aside, raise
the limit, or cancel. A bead that truly can't be finished is set aside; its epic stays open and says so.
Pushing `main` is where Nacre stops: building and publishing releases belongs to the repository's CI.

**Activity** lists rounds first, with their decisions, beads, spend and log, then individual runs. A
Claude Code session in a terminal can start a round too, with `nacred round start NAC-1 NAC-2 …`. A
repository needs a `nacre.yaml` first (see `docs/API.md`); `nacred gates` runs it the way rounds do.

## Specification-guided work

**Inbox** lists everything waiting on you across your computers: decisions and human checks,
proposals ready for approval, blocked reviews, round questions, set-aside beads and found work. Each
item says why and offers its answers inline, and only items whose answer can actually run are listed.
The board assistant starts from the same list when you ask what needs you.

In a repository with OpenSpec, the board's button is **New effort**. Continuing runs one cheap intake
check against your specs and active changes and suggests just a bead, a new change, an amendment of a
named change, or already covered; you confirm or pick another route, and only a confirmed change
develops a proposal (up to $5, and the button says so). Beads whose change is still being planned sit
in a **Proposals** lane before Ready. Cards carry one spec chip (phase, section and evidence, "Closed,
not verified", "Touches a spec", found work), spec epics show a section strip and open with their
change, and a stale approval names what changed. **Found something**, on spec epics, their beads and
spec rounds, sorts discovered work by whether it changes what "done" means: a bead under the epic, a
pending amendment that asks before revising (and says how many merged sections that would reopen),
or a follow-on effort. Agents in spec rounds report such work as `Found:` comments instead of doing it.
The approved `tasks.md` is the plan; progress is read from beads.

Develop a proposal for an existing bead from **Refine** or its overflow menu. **Develop a proposal** compares accepted
specifications and active changes, recommends a new change, amendment, repair or existing coverage,
then runs strict OpenSpec validation and a fresh adversarial review. Findings, source references,
artifacts and decisions stay visible in the change view and are recorded on the bead. Feedback is
saved when you leave the view. Each proposal action stops a runaway at five review passes, $30 and
2 hours; an interrupted operation can resume from its saved checkpoint.

A change reads **Planning** while it is drafted and reviewed, **Ready for your approval**, **Working**
once approved and **Done** once archived, with one **Needs attention** banner when something needs a
fix. **Approve and run** approves the reviewed revision, turns the seed into an epic with one child per
task section and runs that epic straight away: the sections fan out, merge into `epic/<id>`, and the
epic finishes into `main` with its change archived. The round carries the exact approved artifacts
into its workspaces; later proposal or project-context edits require another review. A checked task or
closed bead is not evidence that the specification has been implemented: the change view shows
verified runs and merges per section.

This requires an OpenSpec-enabled repository with project context in `openspec/config.yaml`,
`AGENTS.md`, the OpenSpec CLI and Claude Code on its bridge. Implementation also needs `nacre.yaml`
gates. Older bridges show an availability explanation without affecting ordinary bead editing.

## The board

Lanes are derived from bd, as in the static Thought Catalog board:

- **Ready**: open, with nothing open blocking it.
- **In progress**: `in_progress` or `hooked`.
- **Blocked**: status `blocked`, or still waiting on an open blocker. The card shows what it waits on.
- **Deferred**: `deferred` or `pinned`.
- **Done**: closed, newest first.

Beads labelled `human`, and open decisions, are marked **Needs you** and sort first. Epic chips filter to an epic's descendants and show its progress.

Long-press a card to drag it. On the unfolded screen you drop it on a column. On the cover screen a dock of lanes slides up to drop onto. Every move can be undone from the snackbar.

Unfolded, beads and chat open in a side pane; the chat button sits in the top app bar of Boards, a board and a bead, and on the cover screen chat opens as a sheet. Drag the divider to resize it; on a separating hinge the pane snaps to the hinge. Run transcripts keep their controls on the lower half in tabletop posture.

## Development

```bash
cd android && ./gradlew testDebugUnitTest assembleDebug
cd bridge && make test
```
