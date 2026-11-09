# Proposal

## Why

Needs you is spread over too many layers.
- The count of what waits on Ryder appears in nine places: Projects rows and the filter, the project drawer, the strip, the rail badge, the chat header, the board top bar, the chat drawer, bead tiles and the peek sheet.
- There are thirteen ways in, through notifications, chips, banners and tiles. Most of them lead to a sheet, which leads to a walkthrough, which leads to an evidence pill, which closes the sheet and opens yet another pane.
- Each decision is a large card. It shows in the chat that raised it and again in the queue, with a model-run "card check" ranking it.

Ryder's verdict: "click after click after click". Claude would handle this in a conversation instead. It would say "Nine things need you. Go through them one by one?", explain each one, take his questions, and move on once he answers. Chat is now the center of the app, so Needs you should be a conversation in it too.

## What Changes

- **One badge, one way in.** A single "N need you" badge sits in the top bar. Tapping it opens Chat and starts the Needs you walk-through. Projects rows and the project drawer keep a plain count, but neither is another path in.
- **A heads-up when a chat opens.** A new chat in a project with waiting items opens with nacred's own line, for example "Nine things need you here: 3 approvals, 2 rounds to land, 4 questions. Go through them one by one?". It offers two reply chips, Yes, one by one and Not now. This line is instant and free; it is not a model turn.
- **A Needs you skill.** `/needs-you` runs a walk-through that nacred owns. The agent reads the queue and says how many items wait and what they are. Then it takes one item at a time:
  - It says in plain words what the item is, why it waits, and what each answer would do.
  - It takes Ryder's questions, in his words, for as long as he likes.
  - Once he has answered, it reports the result and moves to the next item, until none are left.
- **Answers are chips above the composer, not cards.** While an item is being discussed, its options sit where the composer's suggestions go: nacred's exact labels, its one-line consequence for the option in focus, and Skip. Tapping a chip is the answer. "In my own words" and notes turn the composer into the answer box for that item. The transcript keeps a one-line receipt: what was answered and what nacred did.
- **Protection is unchanged.**
  - Only a tap on a paired phone answers a decision.
  - The agent never answers one.
  - Chip labels and consequences come from nacred's decision record, never from the agent's text.
- **Tool-permission requests use the same chips.** In-turn Allow/Deny prompts move from transcript cards into the same slot above the composer.
- **Notifications collapse to one.** A single "N things need you" notification updates as the count changes. Tapping it opens Chat and starts the walk-through. A notification about one particular item starts the walk-through at that item.
- **Removed (BREAKING for the app's UI):**
  - the queue sheet and the walkthrough
  - the card check line and its model ranking
  - decision cards in the transcript and in the queue
  - the strip, header and board top-bar "need you" chips and the rail badge
  - the Projects Needs you filter and its card checks
  - per-chat and per-job "need you" pills in the chat drawer
  - the "open its decision" banners in the Spec overview, the work inspector and the bead peek sheet; each becomes "Talk it through", which starts the walk-through at that item
  - the work inspector's separate round stop choices; they are answered as the round's decision
  - the dead `NeedsYouTag` and the `NEEDS_YOU` chip tone

## Capabilities

### New Capabilities
- `needs-you-conversation`: covers the badge, the heads-up, the `/needs-you` walk-through, answer chips above the composer, receipts, and the single notification.

### Modified Capabilities
- `needs-you`:
  - Inbox becomes the conversational walk-through rather than a queue shown in three places.
  - The assistant's briefing points to `/needs-you`.
- `owner-decisions`: decision cards become answer chips above the composer, with receipts in the transcript. Defer becomes Skip. Reject stays an option that requires a note. Ask again and Ask the chat again become things the agent offers in the walk-through. This builds on `chat-first-harness`, which must be archived first.
- `agent-chat`: permission requests are answered from chips above the composer instead of Allow/Deny cards.

## Impact

- **App:**
  - `ui/workspace/decisions/*`: Walkthrough.kt is deleted. DecisionCard.kt becomes an answer-chip row and a receipt.
  - `Interim.kt` (DecisionQueue): deleted.
  - `ChatFirstShell.kt`, `WorkspaceScreen.kt`, `Workspace.kt`, `BoardWorkspace.kt`: the badge, and the chips removed.
  - `ChatPane.kt`, `ChatTimeline.kt`, `ChatModel.kt`: the ask slot, receipts, heads-up and reply chips.
  - `ProjectsScreen.kt`: the filter and card checks removed.
  - `ChatDrawer.kt`, `ChangeOverview.kt`, `WorkInspector.kt`, `BeadPeek*`: pills and banners removed or changed to Talk it through.
  - `notify/ChatNotifier.kt`, `DecisionCheckWorker.kt`, `RunNotifier.kt`: one summary notification.
  - `MainActivity` deep links.
- **nacred:**
  - The `/needs-you` command is expanded into the skill prompt.
  - The heads-up event is added to a new chat's transcript.
  - A `nacre-ask` reference lets the agent point the app at the item under discussion.
  - The answer result is appended to the chat as a receipt, which wakes the agent for the next item.
  - The card check route is retired.
  - The briefing changes.
- **APIs:** `POST /v1/repos/{repo}/decisions/check` and its status route are removed. `/v1/decisions/{id}/answer` is unchanged.

## Out of scope

- AskUserQuestion and ExitPlanMode in full turns. These are `agent-questions-on-the-phone`; the ask slot built here is where they will land.
- Changing which actions are protected, or who may answer them.
- Cross-project walk-throughs. The badge on Projects opens the project with the most waiting; a walk-through across all projects comes later.
- Codex-specific behavior beyond the expanded command text, which is plain prompt text and works for Codex too.
