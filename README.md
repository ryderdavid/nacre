# Nacre

A native Android (Material You) kanban for [beads](https://github.com/steveyegge/beads) issue trackers, built for foldables, with Claude in two roles:

- **Claude API on the phone** (your API key): refine a bead with a reviewable field-by-field diff, turn a rough note into a full bead, and a board assistant that can read the board and propose edits, new beads and dependencies. The chat bar on the boards screen is the same assistant over every project at once: ask about one of them, or line up work across them. You approve every change.
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
