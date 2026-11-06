# Proposal

## Why

The app has grown to 9 screens plus a change overlay, about 140 actions and 25 sheets and dialogs. Yet the
Chat tab can already do the owner's work end to end through nacred, with Claude Code or Codex running on
the Mac. Most of those buttons are a second, worse way to ask nacred for something chat can ask for.
Reorienting the app around chat leaves Ryder fewer places to look and fewer forms to learn. What a
picture shows better stays visual: the kanban and the specs.

## What Changes

- **BREAKING (app):** Two levels replace the four tabs (Boards, Inbox, Activity, Chat).
  - **Projects** (home): every project across the paired computers.
  - **The workspace**: one project with three destinations, **Chat · Spec · Beads**.
  - On the cover screen the destinations sit in a short navigation bar. On the unfolded screen a
    navigation rail chooses Chat alone, or Chat beside Spec or Beads, and a project switcher sits on
    top of the rail.
- Chat becomes the way work is asked for. It has:
  - several threads per project checkout, with a thread switcher;
  - a visible, removable "Viewing" context;
  - reference chips;
  - agent states that follow the real execution state;
  - one status strip combining running work and what needs Ryder.
- **Owner decisions:**
  - Protected nacred actions need an answer from a paired device. These are approve, land, finish,
    sign off, raise a limit, and adopt a change as written.
  - Chat turns get a chat-scoped token that can't call protected actions directly. The agent raises a
    decision instead.
  - nacred binds each decision to one exact action and revision, revalidates it, executes it, and
    reports the result.
  - Decisions appear as cards in chat. There are five kinds: permission, reviewed action, question,
    human check and recovery. Each has defined fields and a defined lifecycle in the Needs-you queue:
    defer, reject, expire and supersede.
- **Work card and work inspector** replace the round and run screens. The inspector keeps direct
  controls that must not depend on an agent: Stop with a stated scope, Cancel round, per-bead Retry,
  the server's recovery choices, and the full log.
- **Spec panel:** a read-only OpenSpec viewer.
  - Changes are grouped by phase.
  - A change overview shows its revision, the review verdict, and the approved plan beside live
    progress from beads.
  - A document reader has an outline, find, Ask about the selection, and labelled requirement deltas.
- **Beads panel:** the kanban.
  - On compact widths it keeps today's board: lane tabs, one lane at a time, the drop dock, and Undo.
  - On wide or focused layouts it shows columns.
  - Tapping a card opens a bead peek with Ask about this, Start work and Open change.
- **Removed from the app (chat or a card does them now):**
  - the Inbox and Activity tabs, and the round and run detail screens;
  - the Start round, Gates setup, New bead, New effort and intake, Found something, Own answer, Round
    questions, Sign-off and Run launch sheets;
  - the change screen's action buttons and the bead field editors;
  - the ✨ header chat with its docked, edge and sheet variants;
  - the live-send action.
- Settings keeps computers and pairing, reconnect, Claude sign-in, updating computers, notifications
  and theme.
- **Look:** Material 3 Expressive throughout.
  - Dynamic color, with Nacre's mother-of-pearl teal as the fallback seed.
  - Roboto Flex, with emphasized headlines.
  - Expressive shapes for identity only.
  - Spring motion, and stable geometry for anything you act on.
- **Palettes:** Settings › Appearance gets a palette picker: Wallpaper (dynamic color), Pearl (the
  default), Ocean (blues throughout), Sage, Ember and Orchid. Each is a full Material 3 scheme in light
  and dark that meets the contrast contract, and each applies immediately.
- An accessibility contract:
  - keyboard completion of the core flows, and TalkBack custom actions for every gesture;
  - 200% font that never truncates a decision;
  - stated contrast thresholds, and validation with TalkBack, Switch Access and a keyboard.

**Out of scope:**
- the round engine, and Beads and OpenSpec semantics;
- removing nacred API endpoints (older apps keep working);
- iOS and the web;
- new agent backends;
- sandboxing the agent beyond nacred's protected operations. The agent still runs as Ryder on his Mac:
  direct git and filesystem actions stay under Claude Code's and Codex's own permission systems and
  the repository's branch protections.

## Capabilities

### New Capabilities
- `owner-decisions`:
  - protected nacred actions and the chat-scoped token;
  - decisions bound to one action and revision;
  - the card kinds and their fields, states and queue lifecycle.
- `spec-viewer`: the read-only Spec panel, covering the change list, the change overview and the
  document reader.
- `beads-board`: the Beads panel, covering the compact and column boards, moving beads, the bead
  peek and Ask about this.
- `app-palettes`: the palette picker, palettes as full Material 3 schemes, and remembering the palette
  and theme.
- `app-accessibility`: keyboard, TalkBack and Switch Access, font scale, contrast and the validation
  required before sign-off.

### Modified Capabilities
- `app-navigation`:
  - Projects and the Chat · Spec · Beads workspace replace the four tabs;
  - adaptive layout, posture, the project switcher and the thread switcher.
- `agent-chat`:
  - threads per checkout and the Viewing context;
  - protected actions go through decisions;
  - states that follow the execution state;
  - one chat surface.
- `chat-presentation`: the docked, edge and sheet chat presentations and the header entry are removed.
- `needs-you`: the Inbox destination is replaced by the queue shown in the workspace and on Projects.
- `round-app`:
  - the round screen becomes the work card and work inspector;
  - questions and landing become decision cards.
- `effort-intake`: the intake verdict and Found something are answered on cards.
- `spec-guided-work`: Approve and run becomes a reviewed-action decision that states whether it lands.

## Impact

- **App (android/):**
  - the navigation shell (NacreRoot, ShellNav);
  - new Projects, Workspace, Spec panel and work inspector screens;
  - Chat (threads, cards, status strip);
  - removal of the Inbox, Activity, round, run, change-action, sheet and docked-chat code;
  - tests for each.
- **nacred (bridge/):**
  - the chat-scoped token and enforcement on protected routes;
  - the decisions API: create, list, answer, revalidate, execute and receipts;
  - decision views of attention items;
  - projects and per-project summaries;
  - chats listed per checkout;
  - a summary per round and run for the work card.
- **docs/API.md:** the decision contract, the token scopes and the new summary endpoints. Older apps
  keep their device tokens and existing routes.
- **Design reference:** the mockup canvas "Nacre chat-first" (19 artboards across the cover and
  unfolded screens, including the palette picker and the Ocean palette). It was reviewed in three rounds by an independent Codex design review.
