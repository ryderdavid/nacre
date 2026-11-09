# Design

## Context

There is one store of decisions behind Needs you. nacred builds a decision for each attention item and for everything an agent raises. Each decision holds the exact action, the revision, the options, the consequences and the state. Only a tap on a paired device answers one (`POST /v1/decisions/{id}/answer`); a chat's own token gets `403 needs_decision`.

Nothing in this change touches that store. What changes is how the app presents it. Today it fans out into:
- nine count surfaces
- thirteen entry paths
- a queue sheet, a walkthrough, and a model-run card check
- full cards placed both in the raising chat and in the queue
- banners on three other panes
- one notification per decision, plus a 15-minute poll

The new shape is a conversation in Chat, run as a nacred-owned skill. Taps on nacred's own options stay the only way to answer.

## Goals / Non-Goals

**Goals**
- One way in (the badge) and one heads-up (a new chat's first line).
- The agent explains one item at a time and takes questions, and Ryder answers with one tap.
- Delete the queue sheet, the walkthrough, the card check, the transcript cards and the scattered chips and banners.
- Keep the authorization model exactly as it is.

**Non-Goals**
- AskUserQuestion and ExitPlanMode, which belong to `agent-questions-on-the-phone`.
- Changing protected kinds or who may answer.
- A walk-through across projects.

## Decisions

### 1. `/needs-you` is a nacred-expanded command, not a Claude Code skill file

When a chat message is exactly `/needs-you` (optionally followed by a decision id), nacred replaces the text it passes to the agent with the walk-through prompt from `chat.NeedsYouPrompt(start string)`. The transcript keeps the short message Ryder sent, shown as "Go through what needs me".

Why not ship a `SKILL.md` into each repository's `.claude/skills`?
- Nacre doesn't own those directories.
- Codex wouldn't see a skill file.
- The prompt has to change in step with nacred's API.

The prompt is versioned with nacred and pinned by tests, as the briefing is.

What the prompt tells the agent to do:
1. Read `GET /v1/decisions?repo=<repo>&state=pending`, in nacred's queue order. A decision id given with the command comes first.
2. Open with one short line: the count, grouped by kind ("3 approvals, 2 rounds to land, 4 questions"). Then start with the first item without asking again, since Ryder already said yes.
3. For each item, say in two to five short lines:
   - what it is
   - why it waits now
   - what the recommended option does
   - what the alternatives do

   Then end the message with a `nacre-ask` block naming the item. Never answer it, and never call a protected route.
4. If Ryder asks something, answer it and end with the same `nacre-ask` again.
5. When a message says the item was answered, read `GET /v1/decisions/{id}` and report nacred's result in one line. A result still executing is reported as underway. Then move to the next item.
6. When none are left, say so in one line and stop.

Walk-through turns are routed as light turns. They only read nacred and the repository. The restricted tool set (Bash, Read, Grep, …) is enough, and it keeps the walk-through cheap.

### 2. `nacre-ask` points the app at the item under discussion

The agent ends a walk-through message with a fenced block whose language is `nacre-ask`:

````
```nacre-ask
{"decision": "dec_…"}
```
````

nacred validates the block when it records the message.
- **Pending in this chat's project:** nacred keeps the block, and the chat's `asking` field becomes that id.
- **Otherwise:** nacred drops the block and records why, as it does for a bad `nacre-draft`.

The app never renders the block as text. It renders the decision itself (decision 3), always from nacred's decision record. The agent's text can't change a label, a consequence or the action.

Why a block rather than nacred tracking the "current item" on its own? It is the same pattern as `nacre-draft`. The agent decides when an item has been explained enough to ask. A question Ryder asks mid-item then keeps the same ask.

Fallback: a walk-through turn might end with no valid block while items are still pending. The ask slot then shows the next pending item in queue order, labelled "Next: <title>", so Ryder is never stuck.

### 3. Answers are chips above the composer

While a chat's `asking` decision is pending, the composer's suggestion slot shows it as follows:
- a one-line title
- one chip per option, recommended first, using nacred's exact labels
- for the option in focus, that option's consequence line from nacred (for example "Stops before main · no push")
- **Skip**, which is today's Defer
- an overflow holding **Reject…** and **In my own words**, whichever the decision allows

A long option list uses the existing fold: the first two and the last two show, plus "Show N more".

Tapping an option:
1. answers it (`POST /v1/decisions/{id}/answer`);
2. shows the chip's sending state;
3. on success, sends the chat message "Answered: <label>". The decision id travels in the message metadata.

That message is Ryder's own, visible in the transcript. It triggers the agent's next walk-through turn. nacred adds the decision's current state to that turn's context, so the agent doesn't have to guess.

Options that need input work as follows.
- **In my own words, a note, an amount or a choice:** the composer turns into that decision's answer box. Its placeholder is "Answer: <title>", it shows a small "answering" tag that can be dismissed, and Send posts the answer, not a chat message.
- **Reject:** it needs a note, so it opens the answer box with "Why?".

Unsent input survives a thread switch. It is keyed by decision id, as `DecisionDrafts` is today.

Typing while an ask is open, outside answer mode, is a question to the agent. A typed "yes, approve it" is not an answer. The prompt tells the agent to point at the chip ("Tap Approve, run, I'll land below"). That costs one tap, and in return nothing the model says can authorize a protected action. Ryder asked for typed answers where they are safe. Mapping free text to an option inside the app was rejected: it would be the app guessing at authorization.

### 4. Receipts replace transcript cards

A decision raised in a chat, or answered during a walk-through, appears in the transcript as a one-line receipt at the place it happened, for example:
- "Approved add-export-chats · running, stops before main"
- "Skipped: land round r-41"
- "Failed: land r-41 · push rejected"

A receipt updates in place as nacred's state moves (executing → done or failed or unknown). It expands to show the action, the revision and nacred's message. The full card composable goes away.

A decision an agent raises outside a walk-through ("I raised the land decision") gets a receipt reading "Waiting for you: land r-41". The ask slot takes it up at once, so Ryder can answer right there without starting a walk-through.

Expired and superseded decisions show that state on their receipt. Ask again (nacred can rebuild the decision) and Ask the chat again (it can't) become things the agent offers when it reaches the item. The renew route is unchanged.

### 5. The heads-up is a nacred event, not a model turn

When nacred creates a chat in a repository with pending decisions, it appends a `needs_you` event to that chat's transcript. The event holds the counts by group and the total, taken at that moment.

The app renders the event as an assistant-style line: "Nine things need you here: 3 approvals, 2 rounds to land, 4 questions. Go through them one by one?" It has two reply chips:
- **Yes, one by one** sends `/needs-you`.
- **Not now** collapses the line to "9 need you · Go through them".

Typing anything else just starts the conversation. The heads-up is never repeated in that chat. The walk-through reads the live list, so a stale count does no harm.

### 6. The badge

One badge in the top bar replaces the strip chip, the phone header chip, the board top-bar chip and the rail badge. It reads "9 need you", and the count excludes skipped items.
- **In a workspace:** it counts that project. Tapping it opens the project's Needs you thread and sends `/needs-you`.
  - That thread is reused per project. It is created on first use and titled "Needs you", so a walk-through never interleaves with a working chat.
  - If a walk-through is already running there, tapping only opens the thread.
- **On Projects:** it counts every project. Tapping it opens the project with the most waiting, as above.

The badge is hidden when the count is zero.

Projects rows and the project drawer keep a plain number. It is a fact about the project, not a way in. Tapping a row opens the project as it does today.

### 7. One notification

`ChatNotifier` and `DecisionCheckWorker` post one ongoing summary per computer: "9 things need you". The expanded view lists projects. It updates as counts change and clears at zero.
- Tapping the summary opens the badge flow for the project with the most waiting.
- A decision raised by a chat while the app is in the background posts as a child of that summary. Tapping it starts `/needs-you <id>` in that chat.

The per-decision dedupe in `AnnouncedDecisions` is kept for the child notifications. `RunNotifier`'s "needs your decision", "Ready to merge" and "Proposal ready" notices become summary updates.

### 8. Tool-permission requests share the slot

A pending in-turn permission request (`/v1/chats/{id}/permissions/{request}`) shows in the same slot, ahead of any decision. It has:
- the step on one line, with its reason under it
- **Allow** and **Deny…**, which takes an optional note in the answer box

The transcript keeps a receipt: "Allowed: run ./gradlew assembleRelease". The route, the 15-minute auto-deny and the background notification are unchanged.

### 9. What is deleted

| Removed | Replaced by |
|---|---|
| Queue sheet (`Interim.kt` DecisionQueue, Handled elsewhere) | Walk-through. Reopen moves into the agent's reach as a non-protected route it may call when Ryder asks. |
| Walkthrough.kt | Walk-through |
| Card check (`/decisions/check`, its status route, CheckLine, model ranking) | nacred queue order. The agent may bring forward an item that unblocks others, and says so. |
| DecisionCard full card, in transcript and queue | Ask slot and receipt |
| Strip, header and board top-bar chips; rail badge | Badge |
| Projects Needs you filter and its card checks | Badge on Projects |
| Chat drawer "N need you" pills; "Working and waiting on you" title | "Working" |
| Spec overview, work inspector and bead peek "open its decision" banners | "Talk it through" → `/needs-you <id>` |
| Work inspector round stop choices (`chooseRound`) | The round's decision |
| BeadsPanel "Needs you · N" filter chip | Kept. It filters beads and is a board view, not a way in. |
| Dead `NeedsYouTag`, `NEEDS_YOU` tone | Nothing |

## Risks / Trade-offs

- **Every item costs a model turn.** Mitigation: light turns, a short prompt, and no re-reading of the transcript.
- **The agent may explain an item wrongly.** Mitigation: labels and the consequence line in the slot are nacred's own. The prompt tells the agent to quote nacred's consequences rather than paraphrase what lands or pushes.
- **The agent may forget the `nacre-ask` block.** Mitigation: the fallback "Next:" slot (decision 2).
- **A typed "yes" is not an answer, which is one more tap than Ryder may expect.** Mitigation: the agent points straight at the chip. It is the price of keeping authorization out of model text.
- **Losing the side-by-side list view.** Mitigation: the opening line names every item. Asking "list them" gets the whole list in chat.

## Migration Plan

1. Archive `chat-first-harness`, which owns `owner-decisions`, before this change's deltas apply.
2. nacred first, which is backwards compatible:
   - `/needs-you` expansion
   - `nacre-ask` handling and the `asking` field
   - the `needs_you` event
   - receipt state on chat decision events

   The old app ignores the new events.
3. The app in one release: add the badge, slot and receipts, and delete the old surfaces in the same release, so there is never a period with both.
4. Remove the card check routes once the app no longer calls them, one release later.

## Open Questions

None blocking. Ryder can tune the copy of the heads-up and the opening line after the device check.
