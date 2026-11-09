## 0. Before starting

- [ ] 0.1 Archive `chat-first-harness` so that `owner-decisions` and the strip requirement exist in `openspec/specs/`. Re-run `openspec validate needs-you-in-chat --strict` and confirm it shows no archive refusals.
- [ ] 0.2 Drop the `chat-first-harness` tasks this change makes moot, and say so in their bead (NAC-3az):
  - 15.1 and 15.2 (decision card option shapes and button group)
  - the decision card in 17.1's screenshots
  - the permission-card split button

## 1. nacred: the walk-through

- [x] 1.1 `chat.NeedsYouPrompt(start string)` contains the walk-through prompt (design decision 1), and tests pin it.
- [x] 1.2 A chat message that is exactly `/needs-you [decision-id]` is expanded into that prompt for the agent. The transcript keeps the short message, and the turn is routed as a light turn.
- [x] 1.3 The briefing's Needs you line offers the walk-through, and a typed "yes" to a decision is answered by pointing at the chip. Update the briefing tests.

## 2. nacred: asking, receipts and the heads-up

- [x] 2.1 Parse a `nacre-ask` block in recorded agent messages:
  - If it names a pending decision of the chat's repository, keep it and set the chat's `asking`.
  - Otherwise, drop it with a reason.
  - Add tests, including a forged id from another repository.
- [x] 2.2 Fill in `asking` from:
  - a decision the chat raises outside a walk-through
  - the first pending permission request, which takes priority
- [x] 2.3 Add the `needs_you` heads-up event to a new chat in a repository with pending decisions. It carries the counts by group and is added once per chat. Test with zero and with several.
- [x] 2.4 Wire up the "Answered: <label>" message metadata:
  - The decision id rides on the message.
  - The turn's context includes nacred's current state of that decision.
- [x] 2.5 Retire the card check: keep `POST …/decisions/check` and its status route answering for one release, mark them deprecated in docs/API.md, and remove the ranking prompt from new work.

## 3. App: the ask slot and receipts

- [x] 3.1 Add an ask slot above the composer that shows the asking decision or permission request:
  - its title
  - option chips, using the existing option fold
  - the focused option's consequence
  - Skip, plus an overflow with Reject… and In my own words
- [x] 3.2 Tapping a chip answers the decision through `answerDecision`, shows the sending state, and then sends "Answered: <label>" with the decision id.
- [x] 3.3 Add the composer answer mode:
  - a placeholder reading "Answer: <title>" and a dismissible tag
  - Send posts the answer
  - unsent input is kept per decision id, reusing `DecisionDrafts`
- [x] 3.4 Fallback: when a walk-through turn ends with no `asking` while decisions are pending, show "Next: <title>" for the first pending decision in queue order.
- [x] 3.5 Receipts replace transcript decision cards and permission cards:
  - one line per decision or permission request, updating in place
  - expandable to the action, the revision and nacred's message
  - expired and superseded states are shown
- [x] 3.6 Permission requests use the slot: Allow, and Deny… through answer mode.
- [x] 3.7 Show the heads-up event as an assistant line with Yes, one by one and Not now. Not now collapses it to "N need you · Go through them".

## 4. App: one way in

- [x] 4.1 Add a top-bar badge in the workspace (all postures) and on Projects. It is hidden at zero and doesn't count skipped decisions.
- [x] 4.2 Tapping the badge opens or creates the project's "Needs you" chat and sends `/needs-you`, unless a walk-through is already running there. On Projects, it picks the project with the most waiting.
- [x] 4.3 Remove:
  - the strip's Needs-you chip
  - the phone header chip
  - the board top-bar chip
  - the rail badge
- [x] 4.4 Remove the Projects Needs you filter and its card checks. Rows and the project drawer keep a plain count.
- [x] 4.5 Chat drawer:
  - remove the "N need you" pills on chat and job cards
  - rename the "Working and waiting on you" section to "Working"
- [x] 4.6 Spec overview, work inspector and bead peek:
  - replace the "open its decision" banners with "Talk it through", which sends `/needs-you <id>` in the Needs you chat
  - remove the work inspector's own stop choices
- [x] 4.7 Delete:
  - the `DecisionQueue` sheet in `Interim.kt` (Handled elsewhere included)
  - `Walkthrough.kt`
  - `CheckLine` and the card-check calls in `NacreRepository`/`BridgeClient`
  - the full `DecisionCard` composable
  - the dead `NeedsYouTag` (the `NEEDS_YOU` chip tone stays: spec chips still use it)
  - the `focusQueue`/`QueueWatcher` plumbing

## 5. App: one notification

- [x] 5.1 Post one summary notification per computer, "N things need you". Its expanded view lists projects; it updates in place and clears at zero. Tapping it does what the Projects badge does.
- [x] 5.2 A decision raised while the app is in the background posts as a child of the summary. Tapping it starts `/needs-you <id>` in the chat that raised it. Keep the `AnnouncedDecisions` dedupe.
- [x] 5.3 `RunNotifier`'s decision, ready-to-merge and proposal-ready notices update the summary instead of posting on their own. Update the `MainActivity` deep links.

## 6. Checks

- [x] 6.1 Unit tests:
  - the slot's model (which item, fallback, the permission taking priority)
  - receipt states
  - heads-up copy
  - the badge count excluding skipped decisions
- [x] 6.2 Go tests for 1.x and 2.x. Run `go test ./...` in bridge and `./gradlew testDebugUnitTest assembleRelease`.
- [ ] 6.3 (Done on 0.46.0 with real items, unfolded and on the cover: badge → Needs you chat → opening line → first item explained → nacred's options above the composer. Not done: answering, Skip, own words and a typed "yes" — answering real items is Ryder's, and sending into his chat was refused.) Emulator check on the cover and inner screens. Use a seeded project with an approval, a land and a question:
  - the badge
  - the walk-through to the end
  - a question mid-item
  - own words
  - Skip
  - a typed "yes"
  - the permission request in the slot
- [ ] 6.4 Device check (human): Ryder goes through real Needs you on the Fold.
