# Nacre, chat first: Chat · Spec · Beads (draft 3, after review 2: REVISE, 10/13 resolved)

## The ask (Ryder, owner and only user)
Collapse the many button-driven flows of the Nacre Android app into a chat harness back to Claude Code
and Codex running on his Macs (via nacred), plus a dedicated OpenSpec viewer and a Beads kanban. All the
orchestration (proposals, rounds, landing, sign-off, gates, updates) happens through chat; the kanban
stays a strong visual UI. Projects across several computers are still maintained and navigated. Look
and feel: Material 3 Expressive, as polished as a first-party Google app for Pixel. Device: Galaxy Z
Fold (cover screen = compact width; inner screen ≈ 840dp wide unfolded, also split-screen/tabletop).

## Today (from the code inventory)
9 routes + a change overlay, ~140 distinct actions, 25 sheets/dialogs, 6 overflow menus. Chat already
runs the full Claude Code / Codex harness on the Mac with Allow/Deny cards, images, routing and screen
context, and the agent can already do the owner's work end to end through nacred's API with ordinary
credentials (create beads, start proposals/rounds, approve, land, finish, raise limits). Needs-you items
(`GET /v1/attention`) already carry server-defined options (method, path, body) that the Inbox performs
generically. Dragging a bead maps to PATCH status / close with Undo. The compact board already shows one
lane at a time with lane tabs, counts, search and a drop dock; drag starts on long-press.

## Principles
1. Chat is the verb, panels are the nouns. Work is asked for in chat; Spec and Beads show the truth.
2. Owner decisions are explicit, specific and stay one tap when the facts are on the card.
3. Direct manipulation where it beats words (drag a bead), and deterministic controls where an agent
   must not be the only way (Stop, cancel a round, retry a parked bead, recovery choices, Settings).
4. Point, don't type: reference chips; automatic "Viewing" context is visible and removable.
5. One canonical Needs-you queue, shown where you are, never moved by opening another thread.

## 1. Owner authority (review C1)
- **Protected actions** (nacred enforces, every route incl. legacy): approve a change (incl. approve
  and run), land/merge a round, finish an epic, sign off or send back a human check, raise a limit or
  budget, answer a round stop with a choice that spends or lands, adopt a change as written.
- **Unprotected** (chat may do directly, as today): create/edit/close beads, start a proposal, start
  a round or run (with the default land-when-ready off), comment, retry, cancel/stop anything.
- **Credentials**: chat turns get a chat-scoped token (env) that nacred accepts for unprotected routes
  only; protected routes need a paired-device token or a decision answered from a paired device.
  Older apps keep working (they hold a device token). A protected call with the chat token gets 403
  `needs_decision` with how to raise one.
- **Decisions**: the agent (or nacred itself, for attention items) raises a decision; nacred builds it
  server-side, binding one immutable action (method, path, body), the target ids, the exact revision
  (change revision, round final commit, build), consequences, and an expiry. Approving it authorizes
  that action once; nacred revalidates (same revision, still applicable) and executes it itself,
  idempotent by decision id.
- **Honest scope**: this guards nacred operations against mistakes and drift. The agent still runs as
  Ryder on his Mac; direct git/filesystem actions stay under Claude Code's / Codex's own permission
  systems (the classifier and host prompts) and the repo's branch protections. The design copy never
  claims more than that.

## 2. Decision cards (review C2, M2)
A small family, all rendered from server data, each with exact options (recommended first), optional
note, and Defer (keeps it in the queue) distinct from Reject (records a no) and Cancel (stops the work):
- **Permission**: Allow / Deny (existing).
- **Reviewed action**: approve, approve and run, land, finish, adopt. Shows host · project ·
  revision; a consequences list computed by nacred (e.g. "Merges 4 beads at a1b2c3d into main and
  pushes; closes NAC-12, NAC-13; NAC-14 stays open"); limits; links to evidence (review findings, diff
  summary, gates) that open in the Spec/Beads panel or a sheet. Approve-and-run states whether it lands
  to main; the same three options everywhere: "Approve, run, I'll land" (recommended), "Approve, run and
  land to main", "Approve only".
- **Question** (round questions, spec drift, intake verdict): choices with consequences + "Answer in
  my own words" (inline field) + Ask about this (explains, never answers).
- **Human check**: the exact check, build and how to try it; Sign off / Not right (with note).
- **Recovery** (round stops): server choices (retry, raise limit, set aside, cancel) with costs.
- **Fields per kind** (each card keeps its unsent input per card id until answered):
  - Found work: the source bead or comment; Changes how or order (optional "Required before which section?"),
    Changes what done means (revise the change), Out of scope (new effort, prefilled); optional note.
  - Recovery: server choices; Raise the limit expands an amount field (above what's spent, at most
    the runaway ceiling), validated inline before Choose enables.
  - Intake verdict: Amends <change> / New change / Just a bead / Already covered, recommended first,
    the owner may pick any; Already covered names what covers it.
  - Not right (human check): a note is required. Reject on approve: a note is required.
- **Lifecycle in the queue**:
  - Defer: stays in the queue marked Deferred, out of the badge count until its state changes or a day
    passes.
  - Reject: approve → the change goes back to its author with the note (today's revise path) and
    the item is replaced by the change's new state; land/finish → the round is Held by you, listed
    under Held with the note and its next actions (Land, Cancel round); sign-off → Send back.
  - Expired (bound revision changed, or 24 h for an agent-raised decision): the card says why and
    offers Ask again, which builds a fresh decision from the current state.
  - Superseded: a newer decision for the same target replaces it; the old card links to the new one.
  - Attention-derived items vanish when the state stops waiting (as today); nothing unresolved
    disappears silently: held or rejected work stays listed with its reason and next action.
- **States**: pending → sending → executing → done / failed / outcome unknown; declined; expired;
  superseded (a newer revision). A done card collapses to a receipt you can expand. Approval ≠ success:
  the card shows the execution result.

## 3. Identity (review M3)
- **Project** = repos grouped by git remote (as today); **checkout** = that project on one computer.
- A **thread** is bound to one checkout and one agent. The app bar shows `Project` with a subtitle
  `Mac Studio · Claude`. Panels show that checkout's beads and changes; if a panel shows another
  checkout it says so.
- Context: an automatic **Viewing** chip (the open panel item) above the composer, inspectable and
  removable for the next message, distinct from **attached reference chips** the user adds.
- Drafts are kept per thread; switching project never retargets a draft. Suggestion chips fill the
  composer; nothing mutates until Send (and, for protected actions, the decision).

## 4. Needs you (review M4)
- The queue is nacred's attention list per project (stable ids, derived from state). Every decision
  card is a view of a queue item. A decision raised in a thread appears inline there and in the queue;
  answering anywhere updates everywhere; opening another thread doesn't move it.
- In a project: one compact status strip under the app bar combining work and attention
  ("Round running · 3 of 7 landed · 2 need you") on every destination, with two
  targets: the work summary opens the inspector, the Needs-you count always opens the queue.
- On Projects: a "Needs you" filter across all projects and computers; an offline computer shows
  "Last checked 5 min ago", never a zero badge.

## 5. Information architecture and adaptive layout (review M5)
- **Projects** (home): grouped by computer with connection state; rows show live status, Needs-you
  count, active change, last line. Filters: All / Needs you / Working. Search. Settings in overflow.
- **Workspace**: Chat · Spec · Beads are the three destinations at every size.
- **Threads**: tapping the title opens the thread switcher (sheet on compact, menu on wide): this
  checkout's threads with title, agent, waiting state and last line; other checkouts of the project
  under their computer; the "New Claude chat ▾" split button on top. Opening a project opens its most
  recent thread. Back from a conversation goes to Projects. Each thread restores its draft, scroll
  position and panel selection when reopened.
- **Compact** (cover, split-screen < 600dp): short navigation bar (Chat · Spec · Beads), hidden while
  typing; back arrow to Projects; single pane.
- **Medium/Expanded** (unfolded ≈ 840dp): navigation rail with a project switcher on top (opens a
  modal drawer listing projects by computer, with the same All / Needs you / Working filter) and
  Chat · Spec · Beads below. The rail chooses the layout: Chat alone, or Chat (main pane, min 360dp)
  with Spec or Beads in the supporting pane (min 320dp) when both fit, else that destination alone.
  The pane has no separate Spec | Beads toggle; its header carries Search and Focus.
  Presets are defined on usable content width W (window − rail − divider; ≈ 728dp unfolded): Chat
  wide (pane at its 320 min), Even (W/2), Pane wide (chat at its 360 min); split only when
  W ≥ 680dp, presets that would break a minimum are hidden; drag clamps to the minimums. Reset
  gives Chat clamp(W/2, 360, W − 320) and the pane the rest (at W = 680: 360/320). The divider has accessibility actions for each preset.
- **Posture**: a separating hinge (book posture) splits the panes at the hinge and nothing straddles
  it (the app's PaneLayout hinge helper). Tabletop (horizontal fold): conversation or panel above
  the fold, composer and status below. Compact height (< 480dp, e.g. landscape cover): the status
  strip folds into the app bar subtitle and the nav bar becomes a rail.
- **Focus Spec / Focus Beads**: either takes the whole content area; chat collapses to a one-line
  chat bar (status + "Message…") that expands back. Kanban shows several columns only when focused or
  wide enough; otherwise the compact board.
- Fold/unfold and resizing preserve thread, draft, selection and scroll. Back: focus → split; Spec
  document → change → list; then Projects.

## 6. Chat surface (review m2, m3)
- Hierarchy on the cover screen: app bar (identity) → one status strip → conversation → composer.
  Suggestion chips only while the composer is empty and collapse once typing starts. No floating chat
  button on Spec/Beads (the nav bar has Chat); an "Ask about this" action appears with a selection.
- Agent states in text, not just motion, tied to execution state: Working (process running; the M3E
  loading indicator while output flows), "No updates for 4 min" as a plain observation, May be
  stalled only when nacred's stall signal fires (its configurable threshold, 15 min by default),
  Waiting for you, Reconnecting, Failed. Tool steps collapse to
  a row that stays expandable.
- **Work card** (a running round/run, in the strip and in the thread where it started): determinate
  progress (beads landed / total, gates), cost so far, and **Stop…** with explicit scope: "Stop this
  reply" vs "Cancel round and its workers" (confirm states what keeps running). Tap → **work
  inspector** sheet: beads with states, gates, failures, artifacts, changes, per-bead Retry, and the
  server's recovery choices — all usable without asking the agent.

## 7. Spec panel (review M7)
- List-detail: changes by phase (Drafting · Waiting for you · In progress · Ready to archive) and the
  project's specs.
- Change overview: phase, revision, stale reason, independent review verdict and findings, the
  approved plan vs live progress from beads (visually distinct), evidence links (beads, runs, rounds).
- Document reader (Proposal · Design · Tasks · Spec changes) full width on compact; outline sheet,
  find in document, selectable text, "Ask about selection" — a floating toolbar here only. Requirement
  deltas inline with text labels Added / Modified / Removed.
- No mutating buttons; "Waiting for you" links to its decision card.

## 8. Beads panel (review M6)
- Keep the compact board: lane tabs with counts, one lane at a time, search, clear-filter feedback,
  drop dock. Wide/focused: columns (≥ 272dp each).
- Long-press = drag (as today). "Ask about this" lives in the bead peek and the card's ⋮ menu, with
  TalkBack custom actions Move to… and Ask about this.
- Moves: Ready / In progress / Blocked / Deferred set status; Done closes (with Undo) and never means
  verified or signed off; Proposals and To check aren't drop targets (sign-off is a decision card).
- Tap → bead peek sheet that expands to full height: description, acceptance, dependencies, comments,
  latest run with link to the inspector; actions: Ask about this, Start work (unprotected, defaults
  to not landing), Open change.

## 9. Removed (chat or a card does them)
Inbox and Activity tabs; round/run detail screens (→ work card + inspector); StartRound (dead),
GatesSetup, NewBead, NewEffort/Intake form, FoundSomething, OwnAnswer, RoundQuestions, Signoff sheets
(→ cards); the change screen's action buttons (→ cards); bead field editors (→ chat); RunLaunch sheet
(→ chat); the ✨ header chat, docked/edge/sheet chat variants; live-send action (→ chat). Settings
keeps computers/pairing, reconnect, Claude sign-in, update computers, notifications, theme.

## 10. Accessibility contract (review M8)
- Every gesture has a visible alternative and a TalkBack custom action (Move to, Ask about, Resize).
- Cards announce kind, target and state; focus stays stable when a card collapses; sheets return
  focus to their opener.
- Streaming: polite announcements of phase changes only (started, needs you, done, failed).
- 200% font: actions, chips and metadata wrap or stack; peeks expand; nothing truncates a decision.
- Color never alone: statuses carry text and icon; contrast verified in light, dark and dynamic color;
  reduced motion respected (no shape morph, crossfades instead).
- Targets ≥ 48dp incl. split-button halves and chip remove.
- Keyboard: every core flow completes with a hardware keyboard (Tab/arrows, Enter, Esc closes sheets,
  Ctrl+Enter sends, Move to… replaces dragging); visible focus ring; Ctrl+F6 moves between panes;
  when a resize or fold removes a pane, focus moves to the same item in the remaining pane.
- Contrast thresholds: 4.5:1 normal text, 3:1 large text, 3:1 for controls and state indicators,
  in light, dark and dynamic color.
- Validation: TalkBack, Switch Access and hardware-keyboard runs of the core flows (open a project,
  answer each card kind, move a bead, read a spec delta, stop a round) before sign-off.

## 11. Look: Material 3 Expressive with jobs (review m1, m3)
- Dynamic color; Nacre's mother-of-pearl teal is the fallback seed. Roboto Flex; emphasized
  headlines (heavier weight, a touch wider) for screen titles and card titles only.
- Components with jobs: short navigation bar (compact) / navigation rail (wide); connected button
  group for the Projects filter and the change's document tabs; split button "New Claude chat ▾" (menu:
  Codex chat, on another computer); floating toolbar only in the document reader; M3E loading
  indicator for live work; large-radius cards; expressive shapes for identity (project avatars),
  never for state; spring motion on navigation and sheets, stable geometry for actionable cards.

## nacred work
Chat-scoped token and protected routes; decisions (create, list, answer, states, revalidate, execute,
receipts) generalized from permissions and attention options; per-project attention summary; a
projects summary for the home list; chat listing per checkout; work-card summary per round/run.

## Out of scope
Round engine, beads and OpenSpec semantics; removing API endpoints; iOS/web; new agent backends;
sandboxing the agent beyond nacred's protected operations.
