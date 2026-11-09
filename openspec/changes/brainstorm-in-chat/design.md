## Context

See proposal.md for why. This is the state of the code the design works from:

- **New effort.**
  - The app keeps `ChatHub.startNewEffort` (`data/chat/ChatHub.kt:392`), which creates a chat with
    `new_effort: true`, but nothing calls it. Chat-first's 6134903 removed its callers.
  - nacred still marks such chats (`chat.Record.NewEffort`) and puts `DraftingPrompt` before the first
    message (`chat/turn.go:149`).
- **Routing.** A Claude chat's message is routed light or full (`chat/route.go`).
  - Light turns run `--permission-mode plan` on the light model (`turn.go:333`). In a light turn, any
    `can_use_tool` request hands off to the full harness (`turn.go:495-511`).
  - Full turns run `--permission-mode auto`.
- **Plan mode can't write mockups.** Probed on 2026-11-09 with the installed CLI, `--permission-prompt-tool
  stdio`:
  - In plan mode the model writes only its plan file. A Write to an added directory never reaches the host as
    `can_use_tool`, and the model calls ExitPlanMode instead.
  - In `default` mode, every Write reaches the host as `can_use_tool` with its `file_path`. Allowing one inside
    the added directory wrote the file, and denying one outside left nothing behind.
- **Drafts.**
  - `ExtractDrafts` turns each `nacre-draft` block into a `KindDraft` event with a `Seq` (`chat/drafts.go`,
    `turn.go:540`).
  - The app shows only the newest one, and the chat-first ChatPane passes no `onContinue`
    (`ui/workspace/chat/ChatPane.kt:233`).
- **Intake.**
  - `POST /v1/chats/{id}/decisions` with kind `intake` takes a `draft` object from the agent and builds the
    card (`server/decision_intake.go:75`). On an intake error it returns the error and builds no card.
  - Options are `create` (POST issue) and `create → develop` (`Then`) actions.
  - `fill` copies a note into `NoteField`, replacing that field (`server/decisions.go:626`). No intake
    action sets `NoteField`.
- **Author input.**
  - Seed (title, description, design, acceptance, notes), `seed_comments` (comments without a `[nacre-`
    marker), inventory, current record, and `feedback` (`specflow/author.go:69-76`).
  - `manager.go:632` overwrites `r.Feedback` on every develop and revise.

## Goals / Non-Goals

**Goals:**
- Every rule in the specs is enforced by nacred, not only stated in a prompt: drafting can't write outside the
  mockups directory, and intake reads the recorded draft.
- One new route parameter and one new chat field. No new endpoint family.
- Works unchanged for chats that aren't New effort chats, apart from light turns being able to draw.

**Non-Goals:**
- Turning AskUserQuestion or ExitPlanMode into cards (next change). Drafting refuses ExitPlanMode.
- Codex drafting. New effort always starts a Claude chat.
- Any app rendering change for mockups.

## Decisions

### 1. Light and drafting turns run in `default` permission mode, with nacred deciding each request

**Decision.** Light turns and drafting turns use `--permission-mode default` plus an `--allowed-tools`
read-only allowlist:

- tools: Read, Grep, Glob, WebFetch, WebSearch;
- Bash, limited to these commands: `bd show|list|ready|search|comments|dep tree`, `openspec
  list|show|validate|status`, `git log|show|diff|status|blame`, `ls`, `rg`, `cat`, `head`, `wc`.

With the allowlist, the CLI doesn't ask about those. Every other request reaches nacred's control handler,
which:

- allows `Write`, `Edit` and `MultiEdit` whose `file_path` resolves (symlinks evaluated, `..` rejected) inside
  the chat's mockups directory;
- for a light turn, hands off to the full harness on anything else, as today;
- for a drafting turn, denies anything else with "Still drafting: nothing changes until Ryder raises intake.
  Revise the draft instead." That includes ExitPlanMode if the model calls it.

**Why not plan mode?** The probe showed that plan mode never asks the host about writes, so it can't allow
mockups.

**Why not `auto` with deny rules?** The auto classifier would still approve repository edits on its own, so
nothing would enforce "change nothing".

**Trade-off.** The light turn's ExitPlanMode handoff (`turn.go:502`) no longer fires. In default mode the model
simply attempts the step, and the existing "needs a step it can't take alone" handoff carries it. The recorded
`light_exit_plan.jsonl` test is kept for the drafting refusal path.

### 2. Drafting is a chat-record flag that nacred clears

**Decision.** `Record.Drafting` is set when a `new_effort` chat is created and sent as `drafting` in chat
records.

- While it is true, `send` skips the router and runs a drafting turn: full model (the owner's default, no
  `--model`), default mode with the allowlist from decision 1.
- nacred clears it when a decision this chat raised, of kind intake, is answered with any option whose action
  creates a bead or develops a change.
- The decision answer path already knows the raising chat (`decisions.Record.ChatID`). It calls a new
  `chat.Manager.EndDrafting(chatID)`.
- Dropping, deferring, expiry and "Already covered" leave it set.

**Alternative considered:** a router rule "New effort means full". It would still run turns in auto mode, so
nothing would be enforced.

### 3. Intake from the recorded draft: `draft_seq` on the existing route

**Decision.** `POST /v1/chats/{id}/decisions` with `{"kind":"intake","draft_seq":N}` looks up event `N` in that
chat. It must be a `KindDraft` event, and its draft becomes the intake draft. If it is missing, the route
returns 410 with `detail` `draft_gone`.

- The app calls this route with the device token for "Where does this belong?".
- The agent may still send `draft`, which is unchanged for the go-ahead path.
- Health gains the feature `draft_intake`. The app shows the button only when the board's computer lists it.

**Why not send the draft object from the app?** Looking up the recorded event is what guarantees "what
Ryder read is what intake sees". It also keeps the parsing in one place.

A repeat tap while a card for that draft is pending returns the existing decision. The `(chat, draft_seq)`
pair is the idempotency key.

### 4. Drafts carry an optional digest; created beads keep it in `notes`

**Decision.**

- `nacre-draft` JSON and `specflow.IntakeDraft` gain an optional `digest` field, under 2 KB, holding decided,
  ruled out, open questions and mockups as short lines.
- The intake card's `create` action body sets `notes` to the digest. The author already reads seed notes, so
  this needs no prompt change beyond decision 6.
- When the card targets an existing bead (`target.bead`), the digest is put at the start of the develop
  feedback, labelled "From the brainstorm:".
- Intake itself ignores the digest. It is not part of the verdict.

**Alternative considered:** a seed comment. The bead doesn't exist until `create` runs, so posting a comment
would need a third chained action. `notes` fits in the one create call.

### 5. Intake notes are appended, not substituted

**Decision.** Both intake actions set `NoteField: "owner_note"`. nacred's routes treat `owner_note` as an
addition:

- `POST …/issues` appends "\n\nRyder's note: <note>" to the description.
- `POST …/issues/{id}/change` appends the same line to `feedback`.

`fill` applies the note to every action in a `Then` chain. Both the created bead and the develop request get
it.

**Alternative considered:** `NoteField: "feedback"`. That would replace intake's rationale and citations.

### 6. Owner feedback becomes a bounded log on the change record

**Decision.**

- Records gain `FeedbackLog []FeedbackEntry{At, Action, Text}`. Each accepted develop or revise request with
  non-empty feedback appends an entry. `Feedback` keeps holding the newest entry, so older readers keep working.
- The author and reviewer input gains `owner_feedback`: the first entry, then the most recent 7, with
  "(N earlier entries left out)" between them when entries were cut.
- Records saved without a log are given one on load from `Feedback`, as a single entry dated from their last
  history entry.

**Alternative considered:** posting each feedback as a seed comment. That writes to bd on every revise, and the
comment filter already hides Nacre's marked comments. The record keeps it local and testable.

### 7. Intake failure still offers a way forward

**Decision.** When the intake call fails (timeout, cost cap or malformed verdict), `buildIntake` builds the
card anyway. Its verdict is empty, its rationale is the failure, and its options are Just a bead and New change,
with no recommendation. This is what the existing effort-intake "Intake fails" scenario has always required. A
`Retry` is the card's Ask again.

### 8. Copy

Server-side strings live in `server/decision_intake.go` and `decision_build.go`. App strings live in
`DecisionModel.kt` and `ChatTranscript.kt`. Changes:

- Intake consequences:
  - "Suggested: <label>, because <first sentence of rationale>";
  - "Nothing happens until you pick one";
  - one cost line, "Intake cost <x>. Writing a proposal is capped at <limit>".
- Option details:
  - Just a bead: "Files it as a bead. Nothing else happens."
  - New change: "Files it, then writes and reviews a proposal for you to approve."
  - Amends X: "Files it, then adds it to the X proposal for you to approve again."
  - Already covered: "Nothing is filed. Its links show what already covers it."
- Approve and adopt cards:
  - RejectLabel "Send back for changes", with the note label "What should change? It's revised and sent back
    to you".
  - Consequence "Locks in this version of the plan: N sections, M tasks".
  - Evidence "Original bead NAC-x".
- Handoff line: "Switching to the full agent so it can <reason>". The reason drops the "the quick answer needs
  a step it can't take alone:" prefix.
- The expired-card composer text drops its dollar figure.

### 9. Prompts

- **`DraftingPrompt`:**
  - restate in up to three lines, name assumptions, ask at most three numbered questions, and write no draft
    unless asked or until the questions are answered;
  - each new draft is preceded by "Changed: …";
  - fill `digest` as the brainstorm settles;
  - raise intake only when Ryder says to go ahead. Otherwise, say that "Where does this belong?" on the card
    takes it on.
  - Remove the sentence about the deleted sheet.
- **Briefing:**
  - The New work bullet says "talk it through and draft it; raise intake only when Ryder says to go ahead".
  - Repositories without `openspec/` create the bead after Ryder says yes to the draft, with no intake.
  - The mockups section gains "draw a flow of three or more parts" and the Mermaid-via-CDN pattern. The
    mockup CSP already allows https scripts.
  - Fix the env name (`NACRE_MOCKUPS_DIR`) wherever docs say otherwise.
- **`LightAddendum`:** "You may write mockups into $NACRE_MOCKUPS_DIR; draw when a picture explains it
  better."

### 10. App

- **New effort entry.**
  - The Chats drawer's new-chat actions gain New effort when the current project has OpenSpec and its
    computer serves Chat.
  - The empty Chat (`ChatFirstShell.kt:365`) shows New effort first, beside Claude and Codex.
  - Both call `ChatHub.startNewEffort`.
  - Found work's Out of scope path keeps using the prefill.
- **Draft card.**
  - `ChatPane` passes `onContinue` for the newest draft when the computer lists `draft_intake`.
  - The button reads "Where does this belong?" and calls the raise route with the draft's `seq`.
  - Below it: "Or tell the chat what to change."
  - A 410 shows "That draft is gone. Ask the chat for a new one."
  - The resulting card arrives through the chat's existing decision events.
- Remove `continueDraft`, `DraftContinuation` and `newEffortStart`'s Sheet branch, which are dead since
  6134903.

### 11. After the Codex review: restricted turns, a go-ahead marker, a guarded raise

The Codex review found three problems with the first build:

- A prefix allow rule approves any flag, so `Bash(git diff:*)` would let `git diff --output=f` write a file.
- The owner's own allow rules and hooks (user, project and local settings) still applied in default mode.
- A drafting turn couldn't raise intake on "go ahead", because it can't call nacred.

The fixes:

- Light and drafting turns run with `--restricted` and `--tools Bash,Read,Grep,Glob,Write,Edit,MultiEdit,
  WebFetch,WebSearch`. Restricted mode ignores settings files. `testdata/restricted_drafting.jsonl` records a
  repository's own allow rules for Edit and `touch` being ignored. `ReadOnlyTools` keeps only commands with no
  option that writes: read-only `bd` and `openspec` subcommands, `git status` and `ls`.
- A drafting reply that ends with the line `NACRE_INTAKE` makes nacred raise intake on the chat's newest draft
  after the turn. It uses the server's own `intakeFromDraft`, the same path as the button, through
  `chat.Manager.SetIntakeRaiser`.
- `intakeFromDraft` marks `(chat, seq)` while intake runs. A second raise gets 409 (`detail` `intake_running`),
  and the app disables the button meanwhile. The app sends the raise with its slow HTTP client, since intake
  may take 90 s.
- A draft may name the bead the effort already has (`"bead"`, for found work filed out of scope). Intake from
  that draft targets it, so no second bead is created.
- The digest bound is checked on every intake path, not only when a block is parsed.

A second Codex review found that a prefix rule still approved `bd ready --claim` and bd's global `--db`. So
Bash has no allow rule at all. nacred checks each command against `readOnlyCommands`: a listed command, listed
flags, and one plain word per argument. The second review also found that a card that failed part way could
be raised again from the same draft, and that the found-work bead depended on the agent filling in `"bead"`.
So any card that may have filed something blocks a second intake, and found work records its bead on the chat
as `effort_bead`.

## Risks / Trade-offs

- **The mockup path check has a check-then-use race.** `insideDir` resolves the path when it allows the
  request, and the CLI writes later. Escaping would need another process to swap a checked directory for a
  symlink in between. The turns themselves can't: they have no Bash that writes, and the file tools don't
  create symlinks. → Accepted. The mockups directory belongs to nacred under its state directory.

- **The allowlist is too tight for drafting.** If the agent needs a read-only command that isn't listed (for
  example `jq`), the request is denied with the drafting reminder. → Denials show in the turn's
  `permission_denials`. Log them, and widen the list from real use. The model can still read files directly.
- **Bash pattern rules leak writes.** `cat > file` could match `Bash(cat:*)`. → Leave `cat` out if the
  CLI's rule matching treats redirects as part of the command. Verify with a recorded test in the task, and fall
  back to Read for file contents. The CLI already vets each stage of a pipeline (see the nacre-headless-claude
  memory).
- **Default mode asks about more than plan mode did, so light turns hand off more often.** → The allowlist
  covers the common read paths. Measure handoff rates before and after in nacred's log.
- **Drafting runs on the full model, which costs more per turn than the light model.** → It is bounded by the
  chat's runaway ceiling, and drafting chats are short and owner-initiated.
- **Spec ordering.** This change modifies effort-intake's "Ryder confirms or overrides the verdict" in its
  chat-first form. → Archive `chat-first-harness` first. If it's still open, archive this one after it.

## Migration Plan

- nacred ships first:
  - `drafting` and `draft_intake` are additive;
  - older apps ignore them;
  - existing `new_effort` chats load as not drafting (none has been created since 6134903);
  - records without `FeedbackLog` get one built on load.
- The app shows New effort and the Draft card button only when the computer lists `draft_intake`. Against an
  older nacred, Chat behaves as it does today.
- Rollback: an older nacred ignores `drafting` and `feedback_log` on load, and keeps `Feedback`.
