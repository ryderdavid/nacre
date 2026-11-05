## Context

- `ui/NacreRoot.kt` builds a `NavigationSuiteScaffold` with Boards, Inbox and Activity.
  - `detailRoute` (the Bead, Run and Round routes) forces `NavigationSuiteType.None` on phones.
  - The change screen is `BeadRoute(openChange = true)`.
  - Snackbars are padded 88.dp above the bar.
- `NacreApp.kt` builds one `BoardAssistant` per board, plus one over `AllBoardsRouter` for all boards, and constructs `ApiBackend`.
- `ChatStack.kt` keeps a `ChatPresentation` and a `ChatViewState` for each screen (`ChatContext.Multi/Board/Bead`). Those hold the draft, scroll position and pending images.
- `BeadChats` stores bead chat threads in `filesDir/bead-chats.json`, with images in `chat-attachments/`.
- `NewBeadSheet.kt` has `DraftChat`, a turn-based draft step before intake that runs on the assist endpoints.
- The bead chat and board assistant run through `Headless` (`bridge/internal/claude/headless.go`), always with `--safe-mode --permission-prompts none` and read-only tools.
- Assist turns are detached on nacred and can be reattached with `GET /v1/assist/{turn}/stream?after=seq` (`bridge/internal/assist/turns.go`, `MacBackend.followTurn`).
- The app's only background watcher is the foreground service `notify/RunWatcherService`, which runs while runs are active and posts round notifications.
- The work of `chat-image-attachments` (epic NAC-48p) is on branch `epic/nac-48p`. Part of it is already in the tree:
  - `bridge/internal/server/images.go`: `maxTurnBody`, `decodeTurn`, `decodeImages` and image validation, with `maxImages` = 20 per request
  - `api.FeatureChatImages`
  - `ImageSupport.kt`, and the image paths in `ApiBackend` and `MacBackend`

  Its device check, NAC-48p.5, is open.
- Bead runs use:
  - `claude.Turn`: `--permission-mode` and `--max-budget-usd`, with a `RunawayUSD` ceiling of $50
  - `codex exec`, which never asks for approval (`bridge/internal/codex/command.go`)
- `codex.Check` reports whether Codex is installed and signed in.
- `config.ResolveListen` returns nacred's listen address: the configured `listen`, otherwise the Tailscale IPv4 and port, and 127.0.0.1 only when Tailscale is down. The `nacred` command (for example `nacred round start`, `cmd/nacred/round.go`) reaches nacred at that address with the owner's token from nacred's config.
- nacred advertises optional capabilities in `features`. The update maintenance reservation refuses while assistant calls are active (daemon-updates).
- Repositories are keyed `<host>~<id>` and grouped by git remote. The across-boards assistant runs on `planAcross`'s host.
- Round agents edit and commit only inside their worktree. Merges are nacred's (round-integration), and pre-flight's unmerged-work question offers "merge it first", which nacred carries out (round-preflight).
- nacred also works in a repository's checkout: landing fast-forwards the target (round-integration), set-up-gates commits `nacre.yaml` on the checkout's branch, and finishing an epic merges into the target (epic-completion).

## Decisions

1. **Chat is a nacred record, not a phone conversation.**
   - nacred stores each chat in `chats/<id>.json` under its state directory, with mode 0600. A record holds:
     - the id, the agent (`claude` or `codex`) and the start repo key, or `null` for all boards
     - the working directory and added directories
     - a title: the first message trimmed to 80 characters, or "New effort · <board>"
     - `created_at` and `updated_at`
     - the agent session id: the Claude session UUID or the Codex thread id
     - the last message's context identity
     - `cost_usd` (Claude only)
     - the image count and bytes sent
     - the state: `idle`, `running`, `waiting_permission`, `interrupted` or `limit_reached`
   - The event log is `chats/<id>.events.jsonl`. Each event has a `seq`. Events are:
     - user messages, with their context and image references
     - assistant text and tool-use summaries, each with its `route` (`light` or `full`)
     - `handoff`
     - `draft`
     - permission requests and their answers
     - errors and turn ends
   - The log keeps the newest 5,000 events. Older ones are dropped with a `truncated` marker.
   - nacred keeps the 200 most recently updated chats. Trimming a chat removes its record, log and images.
   - Delete removes nacred's record, log and images. Claude Code's and Codex's own transcripts stay, as for any session on the Mac.
2. **Endpoints**, using the owner's bearer token like every other endpoint:
   - `GET /v1/chats` lists summaries, newest first.
   - `POST /v1/chats` takes `{agent, repo, context, new_effort?}` and creates a chat. There is no model field.
     - It answers 400 for an unknown repo.
     - It answers 409 with a reason when the agent is unavailable.
   - `GET /v1/chats/{id}` returns the record and the event tail.
   - `GET /v1/chats/{id}/stream?after=seq` is the SSE stream. It also honours `Last-Event-ID`.
   - `POST /v1/chats/{id}/messages` takes `{turn_id, text, context, images?}` and starts a turn.
     - `turn_id` is generated by the client. Repeating it returns the same turn.
     - It answers 409 while another turn runs, when the chat is `limit_reached`, or when images would pass the chat's image limit.
   - `GET /v1/chats/{id}/images/{image}` returns a stored image, for thumbnails.
   - `POST /v1/chats/{id}/cancel` stops the running turn.
   - `POST /v1/chats/{id}/permissions/{req}` takes `{allow, note?}`.
     - It is idempotent, and answers 409 once the request is decided.
     - It answers 403 when the request comes from this computer's own addresses (decision 6).
   - `DELETE /v1/chats/{id}` deletes the chat.
   - `features` gains `agent_chat` (decision 5), and `agent_chat_codex` when `codex.Check` passes and `codex app-server` starts.
3. **Full Claude turns.**
   - nacred starts `claude` in the chat's working directory (decision 12) with:
     - `-p --input-format stream-json --output-format stream-json --verbose --permission-mode auto --permission-prompts host`
     - `--append-system-prompt <briefing>`. It appends, so CLAUDE.md and the owner's system prompt still load.
     - `--session-id <uuid>` on the first turn, or `--resume <uuid>` afterwards
     - `--max-budget-usd <50 − cost_usd>`
     - `--add-dir` for every board on the host
   - The argv has none of:
     - `--safe-mode` or `--no-session-persistence`
     - `--tools`, `--model` or `--system-prompt`
     - `--setting-sources`, `--strict-mcp-config` or `--mcp-config`
   - The child inherits nacred's environment, including the owner's `HOME`, and nacred sets no `CLAUDE_CONFIG_DIR`. MCP servers, CLAUDE.md, skills and hooks therefore load as they do at the terminal.
   - The child environment sets `CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD=1`, so the added boards' CLAUDE.md files load. If the installed CLI ignores it, the briefing tells the agent to read a repository's CLAUDE.md before working there.
   - The child also gets `NACRED_URL` set to `http://<address>`, where `<address>` is nacred's own listen address from `config.ResolveListen`, the same one the server bound and the `nacred` command uses. On a normal install that is the Mac's Tailscale address, not 127.0.0.1.
   - The user message is written to stdin as one stream-json user message:
     - the context note first (decision 13)
     - then the labelled image blocks (decision 15)
     - then the text

     Stdin stays open for the turn, so nacred can answer host prompts on the control channel.
   - The builder is a new `claude.Chat` type. `Headless` and `Turn` are unchanged.
4. **Routing and the quick path (Claude chats only).**
   - **Router call.** Before each message, nacred runs one `Headless` call:
     - flags: `--model <chat.router_model>` (default `haiku`), `--safe-mode --permission-prompts none --no-session-persistence`, no tools, and a `--json-schema` of `{route: "light" | "full", reason}`
     - capped at $0.10 and 15 seconds
   - **What the router sees:**
     - the message text
     - its "On screen" note
     - the number of attached images, but not the images
     - whether the chat is a New effort chat
     - the last six user and assistant messages, trimmed to 2 KB

     It sees no files, tools or session.
   - **What it may choose:** `light` for a plain question that only needs reading and answering, or `full` for anything that changes files, beads, changes or rounds, runs builds, or sends things. Any error, timeout, cost cap or malformed answer means `full`.
   - **Light turn.** It is the same `claude.Chat` argv, in the same working directory and session (`--session-id` or `--resume` as usual), with:
     - `--model <chat.light_model>` (default `sonnet`)
     - `--permission-mode plan` in place of `auto`
     - a one-paragraph addendum to that message's note: answer if reading is enough; otherwise end with exactly `NACRE_HANDOFF: <reason>` on the last line
   - **Handoff.** A light turn hands off when any of these happens:
     - its final text ends with the handoff line
     - it raises any host permission request
     - it calls `ExitPlanMode`, which means it wants to act. Its plan text becomes the handoff reason.

     In the second and third cases nacred denies the request or tool call with "Handing this to the full harness" and stops the light process. Then nacred:
     - strips the handoff line
     - records a `handoff` event ("Handed to the full harness: <reason>")
     - starts a full turn with `--resume` on the same session, whose input is a note: "Continue with the full harness. Carry out Ryder's last message above." It is followed by the one-line "On screen: same as before" note.

     The client's `turn_id` and the stream continue unchanged, and no card reaches the phone for the refused light-turn request.
   - **Pinned by a recording.** The routing section first records a real light plan-mode turn from the installed CLI into `testdata`: a plain answer, a read-only Bash step, and an `ExitPlanMode` attempt. Read steps that plan mode runs without asking stay inside the light turn. Whatever the recording shows as a host prompt or `ExitPlanMode` is handled as a handoff above. Go tests replay each case.
   - **Costs.** Router, light and full costs are all added to `cost_usd`. Every call starts with the remaining budget as its limit.
   - **Not routed.**
     - Codex chats.
     - Messages to a chat whose router model or light model the CLI rejects. nacred then goes straight to full and logs it once.
   - The app shows no model choice. Each assistant event carries its `route` so tests and the API can see it. The conversation shows only the handoff line.
5. **Host permission prompts.**
   - Claude Code asks the host when the auto classifier won't allow a tool on its own. nacred then records a `permission` event with:
     - a request id
     - the tool name and a one-line summary: the command, the file path, or the MCP tool and server
     - the classifier's reason, when one is given
   - The chat moves to `waiting_permission`.
   - The answer is written back as allow or deny. The owner's note becomes the deny message.
   - A request unanswered for 15 minutes is denied with "No answer from the phone within 15 minutes", and the turn continues.
   - Cancel denies all pending requests and ends the turn.
   - The first task of that section records the installed CLI's exact control-message shapes as a test fixture.
   - `agent_chat` is advertised only once this handling exists and the CLI supports `--permission-prompts host`. Otherwise the features reason names the required Claude Code version.
   - Before that section lands, nacred denies any host prompt at once with "Chat can't ask the phone yet", so no turn hangs.
6. **Who answers a permission request.**
   - An answer that arrives from this computer's own addresses (loopback or its own Tailscale address) is refused with 403, "Answer from a paired device". A session on the Mac therefore can't approve its own step, including through `NACRED_URL`.
   - nacred adds no other card. Spending and landing actions are not held (decision 7).
7. **nacred actions from Chat (owner decision).**
   - The agent reaches nacred like any session on the Mac: through the `nacred` command, which uses the config token, or the API at `NACRED_URL` with the token from `nacred token`.
   - Develop, revise, approve, round start, land, finish an epic and raise limits run without a separate card.
   - What bounds them:
     - the auto classifier and its host prompts
     - the chat's $50 runaway ceiling
     - specflow's per-action limit
     - each round's own budget
   - There are no chat tokens and no guarded-endpoint table.
8. **Budget.**
   - Each Claude chat has a $50 runaway ceiling across its router, light and full calls.
   - At or above $50 the chat becomes `limit_reached`, and sending is refused with "This chat reached its runaway limit. Start a new chat to keep going."
   - New then starts a chat whose first message carries the old chat's title as a note.
   - No money is shown on buttons or cards.
9. **Codex turns.**
   - nacred runs `codex app-server` (JSON-RPC over stdio) for each active turn:
     - `thread/start` on the first turn, with:
       - the working directory and sandbox `workspace-write`, with `writable_roots` for every board on the host
       - network access on and approval policy `on-request`
       - the briefing as developer instructions
     - `thread/resume` with the stored thread id on later turns
     - then `turn/start` with the context note, the message text, and a local image input for each stored image of the message
   - Command and file-change approval requests become the same `permission` events, answered from the same endpoint with the same rules.
   - Codex has no classifier. The sandbox decides what runs without asking, and anything beyond it asks the phone.
   - Codex reports no USD cost, so `cost_usd` stays absent and the $50 ceiling doesn't apply. The new-chat sheet says so.
   - Codex children get the same `NACRED_URL` as Claude children (decision 3).
10. **Detached turns and recovery.**
    - Turns run detached from the HTTP request, like assist turns. Events are appended to the log before they are published.
    - The app follows `/stream?after=seq` and reattaches after drops with the `followTurn` backoff.
    - On nacred restart:
      - a chat that was `running` or `waiting_permission` becomes `interrupted`, with an event saying so
      - pending requests are closed as denied by restart
      - the next message resumes the agent session
    - If the agent process exits without a result, the turn ends with an error event and the chat returns to `idle`.
11. **Concurrency and maintenance.**
    - Each chat has at most one running turn, router and handoff included. Chats on the same computer may run at once.
    - Router calls and chat turns register as assistant calls in the update maintenance reservation.
    - Chat turns do not take a `max_concurrent_runs` slot. Commands a chat runs are its own and are not gate slots.
12. **Where a chat runs.**
    - A new chat runs on the computer of the screen it was started from:
      - the repository key's host for a board, bead, change, round or run
      - the across-boards host for Boards, Inbox or Activity
    - Its working directory is that repository's checkout, or `chats/work` under nacred's state directory for all boards. It is fixed for the life of the chat.
    - Every board on that host is added with `--add-dir`.
    - An unreachable host makes New show the computer's offline reason.
13. **Context with every message.**
    - The app keeps one screen-context holder. Whichever screen is showing publishes its `context` into it:
      - `{screen, repo, bead_id?, change_id?, round_id?, run_id?, selection?, pane?, summary}`
      - `screen` is one of `boards`, `inbox`, `activity`, `board`, `bead`, `change`, `round` or `run`
    - `selection` is the item the screen has selected, for example the bead open in the board's right pane, or the round bead whose run is expanded.
    - `pane` names the open pane or tab, for example `proposal.md`, findings or the integration log.
    - `summary` is up to 4 KB of what the screen shows:
      - titles, status and labels
      - the change's phase and revision
      - the round's state, stop reason and choices
      - the run's state and last error
    - The app reads the holder at the moment Send is pressed. Where that context comes from:
      - a docked pane or sheet: the screen beside it or under it
      - the Chat tab: the screen last shown before switching to Chat, or all boards if there was none
    - If the screen's repository has a checkout on the chat's host (the same repo group), the app sends that local key.
      - Otherwise it sends the key with `elsewhere: <computer>`.
      - The composer then shows "This chat runs on <A>. <repo> is on <B>.", with New chat on <B>. Sending is still allowed.
    - nacred puts an "On screen:" note before every user message.
      - The full note, with ids, local path, selection, pane and summary, is sent when the context differs from the previous message's.
      - Otherwise a one-line "On screen: same as before (<label>)" is sent.
      - Boards, Inbox and Activity read "across all boards on this computer".
    - The composer shows a chip **About: <label>**, for example "About: NAC-48p.5", "About: round r-12 · NAC-3" or "About: all boards". It updates as Ryder moves.
14. **Briefing.**
    - The briefing is fixed text in nacred, versioned in Go tests. It covers:
      - `bd` usage, including `bd prime`
      - OpenSpec changes and `openspec validate --strict`
      - the "On screen" note, and that "this" means what it names
      - the `nacred` command and the API at `NACRED_URL`, pointing at `docs/API.md` for:
        - Needs you: read it first when asked what to do or what needs Ryder
        - changes and develop: not for chores, small fixes or beads already linked to a change
        - rounds, found work (ask which of the three answers applies before recording it), approve and sign-off
      - **Working in a checkout.** Repository checkouts are shared with nacred, which lands rounds, commits `nacre.yaml` and finishes epics there. The agent makes code changes on a branch or in a git worktree (`git worktree add`), not on the checkout's current branch. Before ending a turn that edited a checkout, it commits the edits or tells Ryder exactly which files are left uncommitted. Work that should go through a round is started as a round instead.
      - the New effort draft block (decision 16)
      - APK delivery with `tailscale file cp <apk> <phone>:`
    - The phone's Tailscale device name is resolved from the creating request's peer address with `tailscale whois`. If that fails, the briefing says the device is unknown and to ask Ryder.
15. **Images.** This reuses NAC-48p's work from `epic/nac-48p`.
    - **Bringing it in.** The round's pre-flight raises an unmerged-work question for `epic/nac-48p`, and the answer is "merge it first", which nacred carries out before any bead starts (round-preflight). Agents never merge it. The nacred image section and the app image section each begin by checking that the pieces they reuse are present, and add any that are missing as ordinary commits in their own worktree. Their tests fail if those pieces are absent.
    - **On the phone:**
      - Attach image opens `PickMultipleVisualMedia(ImageOnly)` with a maximum of 5 minus the images already pending.
      - Each image is downscaled to a long edge of 1568 px and at most 3 MB, re-encoded as JPEG when needed.
      - Unsupported, undecodable or oversized files are skipped with a snackbar naming them.
      - Pending images live in the chat's `ChatViewState` next to the draft text.
    - **On the wire:** `images` is an ordered `[{media_type, name, data}]` array on `POST …/messages`, with `data` in base64. That endpoint reads its body with `maxTurnBody` (32 MiB) through `decodeTurn`, which gains a limit parameter. Every other endpoint keeps its limit, and a larger body gets 413.
    - **Validation** uses the checks already in `images.go`, before anything is written or started:
      - the media type is one of the four
      - the base64 decodes
      - the signature matches the media type
      - each image is at most 3 MB

      The chat endpoint adds its own check of at most 5 images per message, because `images.go`'s `maxImages` allows 20 per request. The per-repo turn endpoints keep 20 for older apps. A failure answers 400 naming the image or the limit.
    - **Per-chat budget.** The resumed session holds earlier images, so only the current message's images are sent. A message that would take the chat past 20 images or 15 MB decoded gets 409 with "This chat has reached its image limit. Start a new chat to send more images." The app checks the same limit before sending: it keeps the pending images and disables Send with that reason.
    - **Storage.** nacred writes accepted images to `chats/<id>/images/<image-id>.<ext>` (directory 0700, files 0600) before the turn starts, and the user event references them. `GET …/images/{image}` serves them for thumbnails, and they are deleted with the chat. Claude Code's and Codex's transcripts keep their own copies, as stated in Risks.
    - **Delivery:**
      - Claude: each image is an `Image N: <name>` text block followed by its image block, before the message text.
      - Codex: a local image input pointing at the stored file.
      - The router sees only the count.
    - A thumbnail whose image can't be fetched shows "Image unavailable".
    - The parts of NAC-48p that serve only the removed surfaces are removed with them, whichever are present: bead chat attachment storage, the `MacSession` image budget, `ApiBackend` image history and the per-repo attach UI. nacred's image support on the per-repo turn endpoints stays for older apps.
16. **New effort drafts in Chat.**
    - In a repository with OpenSpec, New effort creates a chat with `new_effort: true` on the board's computer and opens it in the pane, sheet or tab as the layout chooses. Its title is "New effort · <board>".
      - The composer is prefilled when New effort was opened prefilled, for example from Found something's out of scope or a follow-on change.
    - nacred puts a drafting prompt before that chat's first message: talk the effort through with Ryder, change nothing, and present the draft as a fenced `nacre-draft` JSON block. The block holds the fields of the New effort sheet's draft: title, type, description, and design and acceptance when present.
    - **Draft events.**
      - nacred parses every `nacre-draft` block in any chat's assistant text into a `draft` event, and strips the block from the shown text.
      - A block that doesn't parse or lacks a title records an error event, "The draft couldn't be read: <reason>", and the conversation continues.
    - The app shows the newest draft as a Draft card with Continue. Continue opens the New effort sheet at its intake step with that draft and any relation the prefill carried, for example related to an epic. Intake, the verdict, overrides and Create and develop work as effort-intake specifies.
    - **Repository without OpenSpec.** The create action stays New bead and opens the bead form directly, with no AI draft step and no intake. A draft from Chat in such a repository continues into that form.
    - **Older bridge without `agent_chat`.** New effort opens its sheet without a draft step, and intake still runs.
    - `DraftChat` is removed from `NewBeadSheet.kt`.
17. **App navigation.**
    - `Tab.CHAT` and `ChatsRoute` are added. Chat is the fourth `item` in the scaffold.
    - `detailRoute` is removed. On `NavigationBar` layouts the scaffold uses `NavigationSuiteType.None` only while `WindowInsets.isImeVisible`. The rail never hides.
    - The chat dock, the change screen's action row, the New FAB and snackbars are measured above the bar, and screens drop their duplicate bottom padding.
    - Tapping Chat goes through `switchTab`. Back is unchanged.
18. **One chat in the app.**
    - `ChatStack` keeps one app-wide current chat id (persisted in settings), its collapsed state, and a `ChatViewState` for each chat id. It holds the draft, scroll position and pending images.
    - The ✨ header action and the bead detail's chat entry open the current chat:
      - as the docked pane on wide layouts, using the existing 760.dp check and the 360.dp minimum for the main content
      - as the slide-up sheet on phones
    - The header menu offers **New · <label>** for the current screen and the recent chats. With no chat yet, ✨ opens New.
    - On wide layouts the docked pane stays docked, or collapsed, across screens. The chat and the bead detail still take turns in the board's right pane.
    - On phones, following a link out of the sheet closes it. ✨ reopens the same chat with its draft.
    - Back first collapses an expanded pane, then follows the screen's order. On phones Back closes the sheet.
19. **Chat tab.**
    - It shows the current chat full screen, or the list when there is none.
    - The list shows New · <label> first, then past chats from every paired computer. Each row shows its computer, start repository or all boards, state and last activity. Delete is in the item menu.
    - New opens a small sheet:
      - Claude or Codex, remembered. Codex is disabled with the reason when `agent_chat_codex` is missing, and notes that it has no classifier or USD limit.
      - the computer, which comes from the screen
    - The conversation shows:
      - streamed text and collapsible tool steps
      - the handoff line
      - Draft cards
      - permission cards
      - image thumbnails
      - the About chip
      - Attach image
      - Stop while a turn runs, when Send is disabled
    - The tab badge counts chats waiting for permission.
20. **Permission cards and notifications.**
    - A card names the step and its reason. It has Allow, and Deny with an optional note.
    - **Background watching.** The existing `RunWatcherService` also runs while any chat on a paired computer is `running` or `waiting_permission`, not only while runs are active.
      - It learns of such chats from the chat list when the app goes to the background and on its existing refresh.
      - It follows each one's `/stream?after=seq`, with the `followTurn` backoff.
      - It stops watching a chat once that chat is `idle`, `interrupted` or `limit_reached`, and stops itself when nothing is left to watch, as it does today.
    - When the app is backgrounded, one "Chat needs your OK" notification per request opens that chat at the card. It uses the existing round notification channel. Request ids are remembered, so a reattach never posts the same request twice.
    - A request answered elsewhere, or timed out, turns the card into a read-only record of the outcome and clears its notification.
21. **Retiring the old chats and the superseded change.**
    - Removed:
      - `BoardAssistant` and its tools and approval flow
      - `AllBoardsRouter` and `ui/assistant/`
      - `ui/bead/BeadChat.kt` (Ask and Refine) and `BeadChats`
      - `DraftChat`
      - the per-screen parts of `ChatStack`
      - the NAC-48p parts listed in decision 15
    - On the first launch after upgrading, the app deletes `bead-chats.json` and `chat-attachments/`.
    - Bead detail keeps its chat entry, which opens the one chat. Develop a proposal moves to the overflow menu only.
    - The app stops calling nacred's assist endpoints. nacred keeps them for older apps.
    - `openspec/changes/chat-image-attachments/` is deleted in the retirement section without being archived, so its requirements never reach the accepted specs. Closing epic NAC-48p and its open children, with a comment pointing to agent-chat-tab, is a step in the human device-check section once its image checks pass, because round agents don't close beads other than their own.

## Failure and recovery

- **Connection drops mid-turn:** the turn continues on the Mac, and the app reattaches with `after=seq`. A resend with the same `turn_id` doesn't start a second turn.
- **App closed and reopened:** the list, the chat and its thumbnails reload from nacred, and a running turn is followed again.
- **App in the background with nothing else running:** the watcher keeps following running and waiting chats, so a permission request still notifies. If the watcher can't reach the computer, it retries with the backoff, and an unanswered request is denied at 15 minutes as usual.
- **nacred restarts:** the chat is `interrupted` with a note, and the next message resumes the session.
- **Router fails or is slow:** the message goes to the full harness.
- **A quick answer needs more:** it hands off within the same turn and session, including when it tries to leave plan mode.
- **Permission card unanswered:** it is denied at 15 minutes with a reason.
- **Answering a request already decided:** nacred answers 409, and the card shows the recorded outcome.
- **Answer from the Mac itself:** nacred answers 403 and the request stays pending.
- **Agent unavailable:** the Claude CLI is missing or lacks host prompts, or Codex is not signed in. New, or that agent, is disabled with the reason from `features`.
- **Runaway limit:** the chat is `limit_reached` with a reason, and New carries its title.
- **Images invalid or over a limit:** nacred answers 400, 409 or 413 with a reason. The draft and pending images are kept, and nothing is written.
- **Unreadable draft block:** the chat says so and continues. Ryder can ask for the draft again.
- **Screen on another computer:** the composer says so and offers New chat there.
- **Chat left edits in a checkout:** the briefing has the agent commit them or name them at the end of its turn. If nacred's landing or set-up-gates then can't proceed, it fails with its existing reason, and nothing is overwritten.
- **Older nacred:** Chat shows "Update nacred on <computer>" to use Chat, and New effort opens its sheet without a draft step.
- **Taildrop target unknown:** the briefing tells the agent to ask.
- **Upgrade cleanup fails:** it is retried at the next launch and never blocks the app.
- **`epic/nac-48p` not merged at pre-flight** (for example, the question wasn't raised or another answer was chosen): the image sections add what they reuse as their own commits, and their tests fail until it is there.

## Risks

- Full-harness turns run with the owner's real permissions on the Mac, and by the owner's decision there is no separate card before spending or landing. The auto classifier, the host prompts, the $50 ceiling, specflow's per-action limit and round budgets are the guards. Deny is the default on timeout.
- A chat works in the same repository checkout where nacred lands rounds (round-integration), commits `nacre.yaml` (set-up-gates) and finishes epics (epic-completion). Uncommitted chat edits, or a chat writing during a landing, can make those steps fail or mix into them. A terminal session has the same risk, but a chat can start it from the phone without Ryder watching. The guards are the briefing rule to work on a branch or in a worktree and to commit or name edits before ending a turn, and nacred's existing refusals when the checkout isn't as expected. nacred's landing code is not changed here.
- The routing model can misroute:
  - A wrong `light` is caught by the handoff, because plan mode can't change anything and leaving it hands off.
  - A wrong `full` only costs more.
  - Router and light calls add a little to every Claude message.
  - If the recorded plan-mode turn shows ordinary read steps raising host prompts, most light turns will hand off. That costs more but loses nothing. The routing section's recording settles it, and this is revisited then.
- The host-prompt control protocol is pinned by a fixture from the installed CLI. A CLI change that breaks it removes `agent_chat` with a reason, rather than leaving turns hanging.
- Keeping the background watcher running for active chats keeps a foreground-service notification visible while a chat works. It stops as soon as no chat is running or waiting.
- Codex has no classifier, and its app-server image input is pinned only by a fake server and the device check. If the installed app-server rejects images, the message fails with its reason and nothing is lost.
- nacred now keeps chat images for the life of the chat, and Claude Code's and Codex's transcripts keep their own copies on the Mac until their own cleanup. This replaces chat-image-attachments' rule that nacred keeps no copy, so thumbnails survive a reopen.
- Merging `epic/nac-48p` first brings code for surfaces this change removes. The retirement section removes those parts, and the tests stay green between the two.
- Deleting the chat-image-attachments change leaves nacred's record of that change without its files until NAC-48p is closed in the device-check section. Its epic is not run in the meantime.
- With the bead chat, assistant and draft step gone, the app's API-key backend may have no caller left. It is not removed here.
- Saved bead chat threads are deleted at upgrade. They were short helper conversations, and the beads and comments are untouched.
- Showing the bar on detail screens costs vertical space on the folded cover screen. Hiding it while typing keeps chat and feedback fields usable.
