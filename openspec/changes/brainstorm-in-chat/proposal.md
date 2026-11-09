## Why

The developer UX review of the ideation phase (`docs/devux/2026-11-08-ideation.md`) found that the way from an
idea to a bead or change no longer works as specified:

- Commit 6134903 ("remove what chat replaced") deleted the New effort entry point and the sheet that Continue
  opened. Nothing calls `startNewEffort`, so the drafting prompt is never sent. In the chat-first Chat the
  Draft card has no button, and Ryder has to know to type "go ahead" before the agent raises intake.
- While Ryder is talking an idea through, the turn runs on the lighter model, and "change nothing" is only an
  instruction.
- What he settles in the brainstorm never reaches the proposal's author. The intake card drops the note he
  types. Each revision's feedback replaces the last.
- An explain turn can't draw a diagram, because the light route can't write the mockup file.

## What Changes

- **New effort comes back in Chat.**
  - The Chats drawer and the empty Chat of a project offer New effort on OpenSpec boards. It starts a
    `new_effort` chat, which sends the existing drafting prompt.
  - Found work answered Out of scope keeps opening a prefilled New effort chat.
- **The Draft card has a way forward.**
  - The latest Draft card offers "Where does this belong?". It raises the intake card in the same thread,
    built from that recorded draft. What Ryder read is what intake sees and what the bead is created from.
  - The card also says he can tell the chat what to change.
  - The New effort sheet stays removed.
- **Drafting is a real mode.**
  - Every turn of a New effort chat runs on the full model with nacred deciding each step. It can read, and the
    only writes it can make are files in its mockups directory.
  - That mode lasts until Ryder answers the chat's intake card with something that creates or develops.
  - The drafting prompt asks the agent to do three things before the first draft: restate what it thinks
    Ryder means, name its assumptions, and ask at most three numbered questions. Each new draft says in one
    line what changed.
  - The agent raises intake only when Ryder says to go ahead or taps the button. The briefing no longer says
    to draft and raise intake straight away.
- **The brainstorm reaches the author.**
  - A draft may carry a short brainstorm digest: decided, ruled out, open questions and mockups.
  - Creating the bead from intake puts the digest in the bead's notes, which the author and reviewer already
    read.
- **Intake never strands the draft.** If the intake call fails, the card still offers Just a bead and New change,
  as effort-intake already requires.
- **Repositories without OpenSpec.** The chat creates the bead once Ryder says yes to the draft, with no intake.
- **Intake notes are kept.** A note on the intake card reaches the bead it creates and the development
    request it starts.
- **Owner feedback accumulates.** Each develop or revise request's feedback is kept on the change record, and
  the author sees all of it, oldest first, instead of only the newest.
- **Plainer cards.**
  - The intake card says "Suggested: <route>, because …" and "Nothing happens until you pick one".
  - Its option details describe what happens ("Files it, then writes and reviews a proposal for you to
    approve"). Cost appears only in the card's cost line, never on an option.
  - On the approve card, "Not right…" becomes "Send back for changes…".
  - The handoff line says "Switching to the full agent so it can …" instead of "Handed to the full harness".
- **Chat can draw.**
  - Light turns move from plan mode, which can't write a mockup, to default mode with a read-only allowlist.
    nacred allows writes to the chat's mockups directory and hands off anything else, as today.
  - The briefing tells the agent to draw a flow of three or more parts, and suggests Mermaid in a
    self-contained HTML mockup.
  - The 0.42.0 mockup behaviour gets the requirement it never had.

Out of scope:

- AskUserQuestion, ExitPlanMode and Codex `request_user_input` as real cards. That is the next change
  (`agent-questions-on-the-phone`). Here, ExitPlanMode in a drafting chat is refused with a pointer to the
  draft.
- Codex New effort chats in a read-only mode. New effort starts with Claude, and Codex chats keep how they
  run today.
- Bringing back the New effort sheet, or editing draft fields directly on the card.
- Draft fields for decided and open questions shown as chips, draft version history, contextual reply
  suggestions, and card answers sent back to the chat (Next in the review).
- Native Mermaid rendering in the app, mockup versioning, a pinned canvas, and views built from project data
  (the review's Visual thinking section).
- Changing intake's verdicts, the author and reviewer loop's passes, approval, or protected-action rules.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `agent-chat`:
  - New effort starts from Chat, and the Draft card raises intake.
  - New effort chats draft in plan mode on the full model, and light turns may write mockups.
  - The briefing covers drafting, intake on a go-ahead, and drawing.
  - New requirements cover mockups in chat, and drawing what the agent explains.
- `effort-intake`:
  - New effort runs intake from the Draft card's recorded draft.
  - The intake card speaks plainly, keeps cost off its options, and keeps Ryder's note.
- `spec-guided-work`: the author and reviewer see the brainstorm digest and every earlier piece of owner
  feedback.

## Impact

- **nacred:**
  - `bridge/internal/chat`: routing for New effort chats; light-turn control for mockup writes; the drafting
    prompt and briefing; drafting state on the chat record; the digest field on drafts; handoff copy.
  - `bridge/internal/server`: raising intake from a draft event; the intake card's copy, notes and digest;
    the approve card's reject label.
  - `bridge/internal/specflow`: `IntakeDraft` digest, the owner-feedback log on records, and the author
    input.
  - Go tests for each.
- **API contract (`docs/API.md`):**
  - `draft_seq` on `POST /v1/chats/{id}/decisions` (kind intake);
  - the draft block's `digest`;
  - `drafting` on chat records;
  - `owner_note` on the intake actions.
- **App:**
  - New effort in the Chats drawer and the empty Chat (`ChatDrawer.kt`, `ChatFirstShell.kt`), wired to
    `ChatHub.startNewEffort`.
  - The Draft card's button (`Conversation.kt`, `ChatPane.kt`), with a raise call in the repository.
  - JVM unit tests for each.
- **Ordering:** this change modifies effort-intake's "Ryder confirms or overrides the verdict" in the form
  `chat-first-harness` gives it, so `chat-first-harness` should be archived first.
- **Older bridges:** without `drafting` or `draft_seq` support, New effort still starts a chat. The Draft card
  then shows no button, and the agent raises intake on a go-ahead, as it does today.
