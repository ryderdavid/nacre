# Tasks

## 1. nacred: light and drafting turns in default mode

- [x] 1.1 In `bridge/internal/claude/chat.go`, let `Chat` carry `AllowedTools`, and pass them with
  `--allowed-tools`. Add the read-only allowlist from design decision 1 as one exported list in
  `bridge/internal/chat`. Light turns switch from `plan` to `default` with that allowlist.
- [x] 1.2 Record a probe with the installed CLI, using the method in `testdata/README.md`, and add it under
  `testdata/`. The probe makes three requests in default mode with the allowlist:
  - a Write into an added mockups directory, answered allow;
  - a Write into the repository, answered deny;
  - `Bash(cat x > y)`.

  Write down in the README whether a redirect matches `Bash(cat:*)`. If it does, drop `cat` from the list.
- [x] 1.3 In the light branch of the control handler (`turn.go:495-511`), allow `Write`, `Edit` and `MultiEdit`
  whose `file_path` resolves inside the chat's mockups directory (`filepath.EvalSymlinks`, reject `..` and
  anything outside). Everything else hands off as today. Change the handoff copy to "Switching to the full
  agent so it can …".
- [x] 1.4 Add `Record.Drafting` (JSON `drafting`). Set it on create for Claude `new_effort` chats. Records
  saved before it existed load as not drafting: no New effort chat has been created since 6134903 removed the
  entry point, so none is mid-draft.
- [x] 1.5 Drafting turns:
  - `send` skips the router;
  - the turn runs with no `--model`, in `default` mode, with the allowlist;
  - the control handler allows mockup writes and denies everything else, ExitPlanMode included, with the
    drafting reminder.
- [x] 1.6 Add `Manager.EndDrafting(chatID)`. In `server/decisions.go`, after a decision is answered: when its
  kind is intake, its `ChatID` is set and the chosen option's action (or any action in its `Then` chain)
  creates an issue or develops a change, call `EndDrafting`.
- [x] 1.7 Go tests, each one using fakeclaude:
  - a light turn writes a mockup without handing off;
  - a light turn writing a repository file hands off with the new copy;
  - a `..` path or a symlink out of the mockups directory is refused;
  - a drafting turn makes no routing call and uses default mode with no `--model`;
  - a drafting Write outside mockups, a writing Bash command and ExitPlanMode are each denied with the reminder;
  - answering the chat's intake card with Just a bead or New change ends drafting, and the next message is
    routed;
  - Drop, Defer, expiry and Already covered leave drafting on;
  - a Codex New effort chat doesn't draft.
- Gate: `cd bridge && go vet ./... && go test ./...`

## 2. nacred: intake from the recorded draft, digest, notes, failure card and copy

- [x] 2.1 Add the optional `digest` field to the draft block (`chat/drafts.go`) and to `specflow.IntakeDraft`,
  bounded to 2 KB. Reject an oversize digest as an unreadable draft, with the reason.
- [x] 2.2 In `raiseRequest`, handle `draft_seq`: look up that `KindDraft` event in the chat and use its draft.
  Return 410 (detail `draft_gone`) when the event is missing or isn't a draft. While a decision for the same `(chat,
  draft_seq)` is pending, return that decision instead of raising a new one. Add the feature `draft_intake` to
  health.
- [x] 2.3 In `buildIntake`:
  - the `create` body sets `notes` to the digest;
  - when `target.bead` is set, the develop feedback starts with "From the brainstorm:" and the digest;
  - both actions set `NoteField: "owner_note"`;
  - `fill` applies notes through the whole `Then` chain.
- [x] 2.4 Teach the issue-create route and the change route to read `owner_note` as an addition. The issue
  route appends "Ryder's note: …" to the description. The change route appends it to `feedback`.
- [x] 2.5 When intake fails, still build the card: an empty verdict, the failure as the rationale, and Just a
  bead and New change with no recommendation (design decision 7).
- [x] 2.6 Copy from design decision 8:
  - the intake consequences and option details;
  - approve and adopt `RejectLabel` "Send back for changes", plus the new note label;
  - "Locks in this version of the plan…";
  - "Original bead NAC-x".

  Check that no option label or detail contains `$`.
- [x] 2.7 Go tests:
  - `draft_seq` raises from the recorded draft;
  - a missing seq returns 410;
  - a repeat tap returns the pending decision;
  - a digest lands in the created bead's notes, and in the feedback for an existing bead;
  - a note reaches both the bead description and the develop feedback without replacing intake's feedback;
  - intake failure builds the fallback card with two options;
  - no intake option string contains a dollar amount;
  - the approve card's reject label and note label;
  - `health` lists `draft_intake`.
- Gate: `cd bridge && go vet ./... && go test ./...`

## 3. nacred: owner feedback log and author input

- [x] 3.1 Add `FeedbackLog []FeedbackEntry{At, Action, Text}` to change records. Each accepted develop or
  revise with non-empty feedback appends an entry, and `Feedback` stays the newest. On load, build a one-entry
  log from `Feedback` for records that have none.
- [x] 3.2 Add `owner_feedback` to the author and reviewer input (`specflow/author.go`): the first entry plus the
  last 7, with "(N earlier entries left out)" between them when entries were cut. The seed's notes are already
  passed; check that they reach the reviewer too.
- [x] 3.3 Go tests:
  - three revises give a log of four, intake included, and all four reach the author in order;
  - a log of 12 shows the first entry, a cut marker and the last 7;
  - the load-time backfill from `Feedback`;
  - seed notes, holding a digest, reach the author and the reviewer input.
- Gate: `cd bridge && go vet ./... && go test ./...`

## 4. nacred: prompts

- [x] 4.1 Rewrite `DraftingPrompt` (design decision 9):
  - restate, name assumptions and ask at most three numbered questions before the first draft;
  - put "Changed: …" before each new draft;
  - fill `digest` as the brainstorm settles;
  - raise intake only on a go-ahead, and otherwise point to the card's button;
  - remove the sheet sentence.
- [x] 4.2 Update the briefing:
  - the New work bullet waits for a go-ahead;
  - in a repository without `openspec/`, create the bead after Ryder says yes to the draft;
  - the draft block section mentions `digest`;
  - the mockups section adds "draw a flow of three or more parts" and the Mermaid-via-CDN pattern, using the
    `NACRE_MOCKUPS_DIR` name.

  Add a line to `LightAddendum` about writing mockups.
- [x] 4.3 Go tests: the prompt and briefing contain each required instruction, and no longer mention the New
  effort sheet.
- Gate: `cd bridge && go vet ./... && go test ./...`

## 5. API contract

- [x] 5.1 In `docs/API.md`, document:
  - `draft_seq` on `POST /v1/chats/{id}/decisions`, with the 410 `draft_gone` and the idempotency;
  - the `draft_intake` feature;
  - `drafting` on chat records and when it ends;
  - the draft block's `digest`;
  - `owner_note` on the issue and change routes;
  - `feedback_log` on change records;
  - the intake failure card.

## 6. App: New effort and the Draft card

- [x] 6.1 Parse `drafting` on `ChatRecord`, and the `draft_intake` feature from health. Add
  `NacreRepository.raiseDraftIntake(chatKey, seq)`, which maps a 410 to a "draft gone" result.
- [x] 6.2 New effort entry:
  - the Chats drawer's new-chat actions (`ChatDrawer.kt`) and the empty Chat (`ChatFirstShell.kt:365`) offer
    New effort first;
  - offer it only when the current project has OpenSpec and its computer lists `draft_intake`;
  - both call `ChatHub.startNewEffort`;
  - keep the found-work Out of scope prefill path working.
- [x] 6.3 Draft card:
  - `ChatPane` passes `onContinue` for the newest draft when the computer lists `draft_intake`;
  - the button reads "Where does this belong?", with "Or tell the chat what to change." below it;
  - a draft-gone result shows "That draft is gone. Ask the chat for a new one."
- [x] 6.4 Remove dead code since 6134903: `continueDraft`, `DraftContinuation`, `draftContinuation`,
  `draftForm` (if unused), and the `Sheet` branch of `newEffortStart`. Update the stale comment in
  `ChatTranscript.kt:59`.
- [x] 6.5 Change the handoff line copy (`ChatTranscript.kt:74`) if the app builds it, and drop the dollar figure
  from the expired-card composer text (`DecisionModel.kt:154-161`).
- [x] 6.6 JVM tests:
  - New effort shows only with OpenSpec and `draft_intake`;
  - the newest draft gets the button, and older ones and older bridges don't;
  - the raise call passes the draft's seq;
  - a 410 shows the draft-gone text;
  - the expired-card text has no `$`.
- Gate: `cd android && ./gradlew clean testDebugUnitTest`

## 8. Codex review fixes

- [x] 8.1 Run light and drafting turns with `--restricted` and a fixed `--tools` list, so the owner's allow
  rules and hooks don't apply. Keep only Bash rules with no option that writes. Record
  `testdata/restricted_drafting.jsonl`.
- [x] 8.2 Go-ahead while drafting: a reply ending with the line `NACRE_INTAKE` raises intake on the newest
  draft through the server's `intakeFromDraft`. Later drafting messages carry `DraftingTurnNote`.
- [x] 8.3 Guard `(chat, draft_seq)` while intake runs (409 `intake_running`). The app uses the slow client and
  disables the button meanwhile.
- [x] 8.4 A draft may name its existing bead (`"bead"`), and intake from that draft targets it. Bound the
  digest on every intake path.

## 9. Codex re-review fixes

- [x] 9.1 Remove every Bash allow rule. nacred allows a light or drafting turn's shell command only when it is
  a listed read-only command with listed flags and no shell syntax. `bd ready --claim` and bd's global
  `--db` are refused.
- [x] 9.2 A draft whose card failed part way or has an unknown outcome returns that card. Only dropped,
  expired or superseded cards let it be raised again.
- [x] 9.3 Found work's Out of scope records the filed bead on its chat (`effort_bead`), and intake from that
  chat's drafts uses it.
- [x] 9.4 Close gaps #3 and #4:
  - the briefing asks every chat to restate the idea and ask numbered questions;
  - the approve card drops the revision hash;
  - a failed intake says why in plain words;
  - the Draft card shows "What we settled".

## 7. Device check (label the bead `human`)

- [ ] 7.1 On the Fold, folded and unfolded, in an OpenSpec project:
  - start New effort from the drawer and from an empty project;
  - check that the first reply restates the idea and asks numbered questions;
  - ask "how does intake reach the author?" in a normal chat, and check that a diagram mockup appears without
    a handoff;
  - correct the draft once, then tap "Where does this belong?";
  - write a note on the intake card, pick New change, and check the bead's description, notes and the change's
    feedback;
  - send the proposal back once, and check that the second author pass mentions both pieces of feedback.

  Record the results on the bead.

## Workflow follow-up

- Archive `chat-first-harness` before this change, because this change modifies effort-intake's "Ryder
  confirms or overrides the verdict" in its chat-first form.
- Next change from the review: `agent-questions-on-the-phone` (AskUserQuestion, ExitPlanMode and Codex
  questions as cards).
