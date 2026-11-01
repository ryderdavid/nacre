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

Run `nacred` on each Mac (`cd bridge && make install`, set `host_name` in its config if you like) and pair each one from Settings › *Add a computer*. Boards from every computer are listed together, tagged with the computer they live on; the Runs tab shows where each run ran.

Settings › **Update connected computers** asks every reachable Mac to install the latest published stable Nacre GitHub release. Each computer shows its own progress or reason it cannot update. Busy computers keep their work; retry after their runs, rounds, assistant turns or specification jobs finish. Updating idle computers briefly disconnects them while the daemon restarts, then verifies the exact release and computer identity. Failed health checks restore the previous binary.

This requires one initial installation of an update-capable nacred LaunchAgent on each Mac, plus Go and GitHub CLI (`gh`) authenticated for `ryderdavid/nacre`. The updater downloads the release's exact source commit into private staging; it does not alter a working checkout. A stable release must be published with a `vMAJOR.MINOR.PATCH` tag and include update protocol 1. If there is no release, the app reports that condition; it never substitutes the main branch. It does not update the Android app. State and rollback live in the nacred data directory's `updates/` folder. An interrupted helper or failed recovery requires checking the saved rollback on that Mac.

A repository checked out on more than one computer (same git `origin`, SSH host aliases resolved) shows as **one board**. Its *home* computer — first paired, or chosen from the board's menu under *Beads from* — is where the board's beads are read and edited. *Run with Claude Code* then offers **Run on** with each computer that has the checkout, so heavy work can go to the Studio and lighter work to the laptop. A run on a computer whose copy of the beads lacks that bead fails with a clear message: sync the beads (git or a Dolt remote) first.

## Agent sessions

When `adopt_sessions` is on in `nacred`'s config, the Mac also watches Claude Code sessions an **agent
started on its own** — claiming a bead from the terminal, say — and Nacre lists them in the Runs tab
beside its own, marked **Agent** with what the session is doing (*working*, *waiting for input*,
*session ended*). The transcript, cost and bead link are all there, read live from the session's
own transcript. A session that has written nothing for 15 minutes (`adopt_idle_minutes`) is left
open rather than working: it drops out of the Runs tab and its count, but its bead still shows the
agent, as idle.

They are read-only, because the Mac owns no process for them: there is no *Cancel* and no follow-up
box, signalling someone's live terminal or resuming it would fork their work. *Copy “take over on
Mac” command* is the way in — it puts the `claude --resume` line on the clipboard. For the same
reason an agent session never posts a run-finished notification and never keeps the foreground
watcher service alive: quitting a terminal you left open for an afternoon is not a run of ours
finishing.

## Rounds

To get several beads done together, open a board's menu and choose **Work several beads until
done**, pick the beads, and optionally a budget. That computer's `nacred` carries them as a
**round**, without the phone attached:

1. It reviews the beads together before any code is written. Where they genuinely disagree, the
   round asks you, with suggested answers. Overlaps and orderings it sequences itself, and it asks
   about unmerged branches that touch the same files.
2. Each bead runs in its own worktree, on its own branch from the round's integration branch.
   `nacred` checks it, not the agent: a clean tree, commits on the branch, and every gate in the
   repository's `nacre.yaml`. When the check fails, it sends the same session back with exactly what
   failed. A bead that can't be fixed is parked, and the rest carry on.
3. It merges the beads one at a time, running the gates after each merge, and hands conflicts to
   Claude with both beads' intent. It finishes with one version bump, every gate and the build on
   one commit.
4. **Land** fast-forwards `main` and pushes it. If `main` moved meanwhile, the round merges it in
   first, redoes its version bump on top of `main`'s, and builds again.

A round fixes what it can before it bothers you: version bumps that collide, beads logs, lockfiles
it can rebuild, a merge Claude can settle, a locked file worth trying again. When it does stop, the
card says what's wrong in plain words and gives you choices: let Claude try again with more budget,
carry on after you've fixed it by hand (it checks something actually changed first), set a bead
aside, or cancel. Rounds land whole: a bead that truly can't be finished is set aside and listed
when you land, and the next round that includes it picks up its work where it stopped.

The Runs tab shows rounds first, with their decisions, beads, plan, spend and log. You get a
notification when a round needs a decision, parks a bead, pauses or is ready to land. A Claude
Code session in a terminal can start one too, with `nacred round start NAC-1 NAC-2 …`. A repository
needs a `nacre.yaml` first (see `docs/API.md`); `nacred gates` runs it the way rounds do.

## Specification-guided work

**Needs you**, at the top of Boards, lists everything waiting on you across your computers:
decisions and human checks, proposals ready for approval, blocked or stale reviews, interrupted
workflows, round questions, parked beads, rounds ready to land, changes ready to archive and found
work. Each item says why and offers its answers inline; Boards carries its own badge with the count,
and the board assistant starts from the same list when you ask what needs you.

In a repository with OpenSpec, the board's button is **New effort**. Continuing runs one cheap intake
check against your specs and active changes and suggests just a bead, a new change, an amendment of a
named change, or already covered; you confirm or pick another route, and only a confirmed change
develops a proposal (up to $5, and the button says so). Beads whose change is still being planned sit
in a **Proposals** lane before Ready. Cards carry one spec chip (phase, section and evidence, "Closed,
not verified", "Touches a spec", found work), spec epics show a section strip and open with their
change, and a stale approval names what changed. **Found something**, on spec epics, their beads and
spec rounds, sorts discovered work by whether it changes what "done" means: a bead under the epic, a
pending amendment that asks before revising (and says how many landed sections that would reopen),
or a follow-on effort. Agents in spec rounds report such work as `Found:` comments instead of doing it.
The approved `tasks.md` is the plan; progress is read from beads.

Develop a proposal for an existing bead from **Refine** or its overflow menu. **Develop a proposal** compares accepted
specifications and active changes, recommends a new change, amendment, repair or existing coverage,
then runs strict OpenSpec validation and a fresh adversarial review. Findings, source references,
artifacts and decisions stay visible in the change view and are recorded on the bead. Feedback is
saved when you leave the view. Each proposal action is bounded to three review passes, $5 and
30 minutes; an interrupted operation can resume from its saved checkpoint.

Approve the reviewed revision to turn the seed into an epic with one child per task section.
Approval prepares the work. **Implement change** opens the existing round controls with those
children and an explicit budget. The round carries the exact approved artifacts into its
workspaces; later proposal or project-context edits require another review. Generic runs cannot
bypass that handoff. Retries recover mapped children and preserve manually edited bead content.

The change view distinguishes task progress, verified runs, human checks and landing. A checked
task or closed bead is not evidence that the specification has been implemented. Specification
rounds require explicit landing; accepting the result, closing the epic and archiving the OpenSpec
change remain deliberate final steps. Other beads and rounds keep their existing behavior.

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

Long-press a card to drag it. On the unfolded screen you drop it on a column. On the cover screen a dock of lanes slides up to drop onto. Every move can be undone from the snackbar. You can also group by priority or type instead of workflow; dropping then changes that field.

Unfolded, beads and the assistant — the board's own, or the cross-project one from the boards screen — open in a side pane. Drag the divider to resize it; on a separating hinge the pane snaps to the hinge. Run transcripts keep their controls on the lower half in tabletop posture.

## Development

```bash
cd android && ./gradlew testDebugUnitTest assembleDebug
cd bridge && make test
```
