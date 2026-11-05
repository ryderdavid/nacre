<!-- nacre-section: sf8077563b43ae9d180faa -->
## 1. App: Chat tab and a tab bar on every screen

- [ ] 1.1 In `ui/NacreRoot.kt`, add `Tab.CHAT`, `ChatsRoute` and a fourth `item` (Chat), so phones get four equal items and the rail gets Chat. Chat uses `switchTab`.
- [ ] 1.2 Remove `detailRoute`. On `NavigationBar` layouts use `NavigationSuiteType.None` only while the keyboard is visible. The rail never hides.
- [ ] 1.3 Make the chat dock, the change screen's action row, the New FAB and snackbars sit above the bar on bead, change, run and round screens, and remove double bottom padding.
- [ ] 1.4 Remember the screen last shown before switching to Chat, for the Chat tab's context and New · <label>.
- [ ] 1.5 JVM tests:
  - the layout type per route and keyboard state: the bar shown on Bead, Run and Round, hidden only while the IME is visible, and the rail always shown
  - four items in order
  - the remembered screen and its label for board, bead, change, round, run, Boards, Inbox and Activity origins
  - Back from a bead returns to the board
- Gate: `cd android && ./gradlew testDebugUnitTest`

<!-- nacre-section: sf948ae40c21bcf640e5bb -->
## 2. nacred: chat records, full Claude turns, context notes and endpoints

- [ ] 2.1 Add `bridge/internal/chat`:
  - records in `chats/<id>.json` (mode 0600) and the event log in `chats/<id>.events.jsonl`, with `seq`, a 5,000-event cap and the `truncated` marker
  - retention of 200 chats, where a trimmed chat loses its record, log and `chats/<id>/` directory
  - the states `idle`, `running`, `waiting_permission`, `interrupted` and `limit_reached`
  - on startup, mark `running` and `waiting_permission` chats `interrupted`
- [ ] 2.2 Add the `claude.Chat` argv for full turns:
  - `-p --input-format stream-json --output-format stream-json --verbose --permission-mode auto --permission-prompts host --append-system-prompt <briefing>`
  - `--session-id` on the first turn, and `--resume` afterwards
  - `--max-budget-usd` set to the remainder of $50
  - `--add-dir` for every board on the host
  - none of `--safe-mode`, `--no-session-persistence`, `--tools`, `--model`, `--system-prompt`, `--setting-sources`, `--strict-mcp-config` or `--mcp-config`
- [ ] 2.3 Set the child environment:
  - inherit nacred's environment, including `HOME`
  - set no `CLAUDE_CONFIG_DIR`
  - set `CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD=1`
  - set `NACRED_URL` to `http://<address>`, where `<address>` is nacred's own listen address from `config.ResolveListen`, the address the server bound and the `nacred` command uses (the Tailscale address on a normal install, not 127.0.0.1)
- [ ] 2.4 Run turns detached. Write each user message to stdin, preceded by its "On screen" note, and keep stdin open.
  - The note has the full context when it differs from the previous message's, and the one-line reminder otherwise.
  - An `elsewhere` repository is named with its computer.
  - Append events before publishing them.
  - Add the result's cost to `cost_usd`, and set `limit_reached` at $50.
- [ ] 2.5 Until section 3 lands, deny any host permission request at once with "Chat can't ask the phone yet", so no turn waits.
- [ ] 2.6 Write the briefing text. It covers:
  - bd and OpenSpec
  - the "On screen" note
  - the `nacred` command and the API at `NACRED_URL`: Needs you first, develop only for behavior changes, rounds, found work (ask which answer applies before recording), and sign-off
  - working in a checkout: nacred lands rounds, commits `nacre.yaml` and finishes epics in the repository checkouts, so make code changes on a branch or in a git worktree, and before ending a turn that edited a checkout, commit the edits or name the files left uncommitted
  - the `nacre-draft` block
  - Taildrop

  Resolve the phone's Tailscale device with `tailscale whois` on the creating request's peer.
- [ ] 2.7 Set the working directory once, at creation: the repository checkout for a repo key, otherwise `chats/work` on the across-boards host.
- [ ] 2.8 Add the endpoints:
  - `GET/POST /v1/chats` (`{agent, repo, context, new_effort?}`), `GET /v1/chats/{id}`, `GET /v1/chats/{id}/stream?after=` and `DELETE /v1/chats/{id}`
  - `POST /v1/chats/{id}/messages` with `{turn_id, text, context}`. It is idempotent by `turn_id`, and answers 409 while a turn runs or the chat is `limit_reached`.
  - `POST /v1/chats/{id}/cancel`
- [ ] 2.9 Register chat turns as assistant calls in the maintenance reservation. Don't take a `max_concurrent_runs` slot. Don't advertise `agent_chat` yet (section 3 does).
- [ ] 2.10 Go tests:
  - the exact argv for a first turn, a resumed turn and an across-boards turn
  - the environment: `HOME` kept, no `CLAUDE_CONFIG_DIR`, and none of the setting or MCP override flags
  - `NACRED_URL` equals the server's bound address, including a server bound to a non-loopback address with nothing on 127.0.0.1, and a request from the child's environment reaches the server
  - the budget remainder, and `limit_reached` refusing with the reason
  - a `turn_id` resend returns the same turn
  - reattaching with `after` replays exactly the missing events
  - a restart marks the chat interrupted, and the next turn resumes
  - context notes: full on a change of screen, the one-liner on the same screen, all boards for Inbox, and the `elsewhere` note
  - a host prompt before section 3 is denied at once and the turn ends normally
  - the briefing text, with the whois fallback, including the working-in-a-checkout rule (branch or git worktree, and commit or name uncommitted edits before ending a turn)
  - an update is refused while a chat turn runs
  - retention and event-cap trimming
- Gate: `cd bridge && go vet ./... && go test ./...`

<!-- nacre-section: sfbc0ae2879cb583bcb99f -->
## 3. nacred: host permission prompts and advertising Chat

- [ ] 3.1 Capture the installed Claude Code's host permission control messages for a refused command into `testdata`, and parse them into `permission` events (id, tool, summary, reason). Set the chat to `waiting_permission`. This replaces the immediate deny from 2.5.
- [ ] 3.2 `POST /v1/chats/{id}/permissions/{req}` with `{allow, note?}`:
  - writes allow or deny back on stdin
  - is idempotent, and answers 409 when the request is already decided
  - answers 403 "Answer from a paired device" when the request comes from loopback or this computer's own Tailscale address
- [ ] 3.3 Deny automatically after 15 minutes with "No answer from the phone within 15 minutes". Cancel and restart deny pending requests.
- [ ] 3.4 Advertise `agent_chat` in `features` only now, and only when the installed CLI supports `--permission-prompts host`. Otherwise give a reason naming the required version.
- [ ] 3.5 Go tests, with a fake CLI replaying the fixture:
  - Allow lets the step run
  - Deny with a note refuses it, and the turn continues
  - the timeout denies it, and the turn ends normally
  - a second answer gets 409
  - an answer from loopback, and one from the computer's own `NACRED_URL` address, gets 403 and the request stays pending
  - cancel while a request is pending ends the turn
  - `agent_chat` present or absent according to CLI support
  - no turn is left running after any of these
- Gate: `cd bridge && go vet ./... && go test ./...`

<!-- nacre-section: sfb3cb2ad434c1ed7e3b3a -->
## 4. nacred: routing between a quick answer and the full harness

- [ ] 4.1 Add `chat.router_model` (default `haiku`) and `chat.light_model` (default `sonnet`) to nacred's config.
- [ ] 4.2 Add the router call before each Claude chat message:
  - a `Headless` call with `--model <router_model>`, `--safe-mode --permission-prompts none --no-session-persistence`, no tools, and a `--json-schema` of `{route, reason}`
  - $0.10 and 15 seconds at most
  - its input is the message text, the "On screen" note, the image count, whether it is a New effort chat, and the last six messages trimmed to 2 KB
  - any error, timeout or malformed answer means `full`
  - add its cost to `cost_usd`, and register it as an assistant call
- [ ] 4.3 Light turns use the `claude.Chat` argv in the same session with `--model <light_model>` and `--permission-mode plan` instead of `auto`, plus the handoff addendum in that message's note.
- [ ] 4.4 Record a real light plan-mode turn from the installed CLI into `testdata`, covering a plain answer, a read-only Bash step and an `ExitPlanMode` attempt. Note in the fixture's README whether the read-only step ran without asking or raised a host prompt.
- [ ] 4.5 Handoff happens on a final `NACRE_HANDOFF: <reason>` line, on any host permission request in a light turn, or on an `ExitPlanMode` call. For a permission request or `ExitPlanMode`, refuse it with "Handing this to the full harness" and stop the light process; `ExitPlanMode`'s plan text becomes the reason. Then:
  - strip the line
  - record a `handoff` event
  - start a full turn with `--resume` on the same session, using the continuation note and the same `turn_id`

  Stop cancels whichever part is running.
- [ ] 4.6 Record `route` on assistant events. Skip routing for Codex chats. If the CLI rejects the router or light model, go straight to full and log it once.
- [ ] 4.7 Go tests, with a fake CLI and the 4.4 recording:
  - a router answer of `light` runs the light argv, and `full` runs the full argv
  - router error, timeout and malformed output run full
  - the router input holds no files, includes the New effort flag, and is trimmed to 2 KB of history
  - the recorded plain answer stays light, with no handoff
  - the recorded read-only step is handled as the recording shows: kept in the light turn if it ran without asking, a handoff if it raised a host prompt
  - the recorded `ExitPlanMode` attempt hands off with its plan as the reason
  - a handoff line starts a full turn on the same session id with the same `turn_id` and no card
  - a permission request in a light turn is denied without a card and hands off
  - router and light costs count toward $50
  - Codex chats make no routing call
- Gate: `cd bridge && go vet ./... && go test ./...`

<!-- nacre-section: sf809dfb364c225301beb5 -->
## 5. nacred: Codex chats through app-server

- [ ] 5.1 Add a `codex app-server` JSON-RPC client:
  - `thread/start` with the working directory, the workspace-write sandbox with `writable_roots` for every board on the host, network access, `on-request` approvals and the briefing as developer instructions
  - `thread/resume` with the stored thread id, then `turn/start` with the context note first
  - translate its events into chat events
- [ ] 5.2 Turn command and file-change approval requests into `permission` events, answered through the same endpoint with the same rules and timeout. Leave `cost_usd` absent. Give Codex children the same `NACRED_URL` as in 2.3.
- [ ] 5.3 Advertise `agent_chat_codex` only when `codex.Check` passes and app-server starts, otherwise give the reason.
- [ ] 5.4 Go tests, with a fake app-server:
  - start and resume use the same thread
  - an approval request is allowed, denied and timed out
  - the feature is absent when Codex is not signed in
  - the child's `NACRED_URL` equals the server's bound address
- Gate: `cd bridge && go vet ./... && go test ./...`

<!-- nacre-section: sfd2d561413447fba71bd1 -->
## 6. nacred: images in chat messages

- [ ] 6.1 Check that the nacred pieces Chat reuses from NAC-48p are on the integration branch: `bridge/internal/server/images.go` (`maxTurnBody`, `decodeTurn`, `decodeImages` and its checks) and `api.FeatureChatImages`. The round's pre-flight answer for `epic/nac-48p` is "merge it first", which nacred carries out. If any piece is still missing, add it here as ordinary commits in this worktree, with its own tests. Don't merge branches.
- [ ] 6.2 Accept an optional ordered `images` array of `{media_type, name, data}` on `POST /v1/chats/{id}/messages`.
  - Read that body with `maxTurnBody` through `decodeTurn`, which gains a limit parameter, and answer 413 above it. Every other endpoint keeps its limit.
  - Validate with the `images.go` checks: the media type, the base64, the signature, and at most 3 MB each.
  - Add a chat-endpoint check of at most 5 images per message. The per-repo turn endpoints keep `maxImages` of 20 for older apps.
  - On a failure, answer 400 naming the image or the limit, before anything is written or started.
- [ ] 6.3 Enforce the per-chat budget of 20 images and 15 MB decoded with 409 and "This chat has reached its image limit. Start a new chat to send more images."
- [ ] 6.4 Store accepted images in `chats/<id>/images/<image-id>.<ext>` (directory 0700, files 0600) before the turn starts, and reference them from the user event.
  - Add `GET /v1/chats/{id}/images/{image}`.
  - Delete the directory with the chat and on retention trim.
- [ ] 6.5 Deliver the images:
  - Claude: an `Image N: <name>` text block, then the image block, before the message text in the stream-json user message
  - Codex: a local image input for each stored file in `turn/start`
  - only the current message's images are sent, and the router gets only the count
- [ ] 6.6 Go tests:
  - 5 images of 3 MB are accepted
  - 6 images in one chat message get 400 naming the 5-image limit, while a per-repo turn with 20 images is still accepted
  - a body over 32 MiB gets 413
  - each validation error gets 400 with nothing written and no turn
  - the per-chat limit gets 409
  - file and directory modes
  - deletion with the chat and on trim
  - the stream-json content order
  - a follow-up carries no images
  - Codex `turn/start` carries the local image path
  - the image endpoint serves the stored file
- Gate: `cd bridge && go vet ./... && go test ./...`

<!-- nacre-section: sf0baee93f2ebf6c40d681 -->
## 7. API contract

- [ ] 7.1 Document in `docs/API.md`:
  - the chat endpoints, record fields, states and events, including `route`, `handoff` and `draft`
  - the per-message `context` object, the "On screen" note and `elsewhere`, and `turn_id` idempotency
  - routing: what the router sees, `light` and `full`, the handoff (including `ExitPlanMode`), and that Ryder never picks a model
  - permission requests, answers, the 15-minute deny and the 403 for answers from the computer itself
  - that chats use nacred's command and API like any session on the Mac, with no extra card before spending
  - images: the field, the 5-per-message and per-chat limits, 400, 409 and 413, storage and the image endpoint
  - `new_effort` and the `nacre-draft` block
  - the $50 ceiling and `limit_reached`, and that Codex has no classifier, routing or USD limit
  - the briefing environment, with `NACRED_URL` set to nacred's own listen address, and the briefing's working-in-a-checkout rule
  - `agent_chat` and `agent_chat_codex`, and older-bridge behavior
- Gate: `openspec validate agent-chat-tab --strict`

<!-- nacre-section: sfad28ff57177e538d64ab -->
## 8. App: Chat tab, conversation, context and permission cards

- [ ] 8.1 Add the chat models, and `BridgeClient` and `NacreRepository` calls for every chat endpoint. Merge the lists from all paired computers, and show "Update nacred on <computer> to use Chat" for computers without `agent_chat`.
- [ ] 8.2 Add a screen-context holder. Board, bead, change, round, run, Boards, Inbox and Activity publish `{screen, repo, ids, selection, pane, summary ≤ 4 KB}` while they are shown.
  - Read it at Send.
  - In the Chat tab, use the screen from 1.4.
  - Map the repository to the chat's host through its repo group, or mark it `elsewhere`.
- [ ] 8.3 Add the `ui/chat/` Chat tab:
  - the current chat full screen, or the list
  - the list: New · <label> first, then past chats with computer, start repository, state and last activity
  - Delete in the item menu
- [ ] 8.4 Add the New chat sheet:
  - Claude or Codex, remembering the choice
  - Codex disabled with the reason when unavailable, and a note that Codex has no classifier or USD limit
  - the computer taken from the screen
  - no model choice
- [ ] 8.5 Add the conversation:
  - streamed text and collapsible tool steps
  - the "Handed to the full harness" line
  - the About: <label> chip, and the `elsewhere` hint with New chat on <computer>
  - Send disabled while a turn runs, and Stop
  - follow the stream with `after=seq` and the `followTurn` backoff, and resend with the same `turn_id`
  - the `limit_reached` reason, with New
- [ ] 8.6 Add permission cards and background watching:
  - Allow, and Deny with an optional note
  - a read-only outcome once decided, timed out or answered elsewhere, including after a 409, and clear that request's notification
  - extend `notify/RunWatcherService` so it also runs while any chat on a paired computer is `running` or `waiting_permission`: learn of them from the chat list when the app goes to the background and on the watcher's refresh, follow each one's `/stream?after=seq` with the `followTurn` backoff, drop a chat once it is `idle`, `interrupted` or `limit_reached`, and stop when nothing is left to watch
  - one "Chat needs your OK" notification per request on the round channel when the app is backgrounded, opening the chat at the card, with request ids remembered so a reattach never posts twice
  - the Chat tab badge
- [ ] 8.7 JVM tests:
  - the list merge across computers, and the older-bridge message
  - context captured at Send for each screen, including selection and pane, within 4 KB
  - bead → round changes the sent context and the chip
  - the Chat tab uses the last screen
  - `elsewhere` mapping and the hint
  - reattach without duplicate or missing events
  - a resend keeps its `turn_id`
  - the handoff line
  - card states for allow, deny, timeout and 409
  - one notification per request, including after a reattach
  - a `waiting_permission` chat with no active run keeps the watcher running and posts exactly one notification, and the watcher stops once no chat is running or waiting and no run is active
  - the badge count
  - Codex disabled with the reason
- Gate: `cd android && ./gradlew testDebugUnitTest`

<!-- nacre-section: sfd225ddab0332a0a55c1c -->
## 9. App: images in Chat

- [ ] 9.1 Check that the app pieces Chat reuses from NAC-48p are on the integration branch: `ImageSupport.kt` (picker, decode, downscale and re-encode). If any is still missing, add it here as ordinary commits in this worktree, with its own tests. Don't merge branches.
- [ ] 9.2 Add Attach image to the Chat composer, reusing that picker and downscaling:
  - `PickMultipleVisualMedia(ImageOnly)`, limited to 5 per message
  - a long edge of 1568 px and at most 3 MB
  - unsupported, undecodable or oversized files skipped with a snackbar
- [ ] 9.3 Keep pending images in the chat's `ChatViewState` with the draft. Show removable thumbnails, allow images-only sends, and send `images` with the message.
- [ ] 9.4 Track the chat's image count and bytes. When a send would pass 20 images or 15 MB, keep the pending images and disable Send with the image-limit reason. Show nacred's 400, 409 and 413 reasons and keep the draft.
- [ ] 9.5 Show thumbnails on sent messages from `GET …/images/{image}`, with "Image unavailable" when one can't be fetched.
- [ ] 9.6 JVM tests:
  - downscaling to 1568 px and 3 MB, and skipping an unsupported file
  - pending images survive collapse and moves between the pane, the sheet and the tab
  - the 5-image limit disables attach
  - removing a pending image
  - an images-only send
  - the per-chat limit keeps images pending and blocks Send with the reason
  - nacred error reasons are shown with the draft kept
  - the unavailable placeholder
- Gate: `cd android && ./gradlew testDebugUnitTest`

<!-- nacre-section: sf7948cec91ce49c5b0ef1 -->
## 10. New effort drafts in Chat

- [ ] 10.1 nacred:
  - `new_effort: true` on `POST /v1/chats` titles the chat "New effort · <board>" and puts the drafting prompt before its first message
  - parse fenced `nacre-draft` JSON blocks in any chat's assistant text into `draft` events (title, type, description, design, acceptance), and strip the block from the shown text
  - a block that doesn't parse or has no title records "The draft couldn't be read: <reason>"
- [ ] 10.2 App:
  - in OpenSpec repositories, New effort creates a `new_effort` chat on the board's computer and opens it in the pane, sheet or tab as the layout chooses, with the composer prefilled when New effort was opened prefilled
  - show the newest draft as a Draft card with Continue, which opens the New effort sheet at its intake step with the draft and any relation the prefill carried
- [ ] 10.3 Remove `DraftChat` from `NewBeadSheet.kt`.
  - In repositories without OpenSpec, the create action opens the New bead form directly.
  - On computers without `agent_chat`, New effort opens its sheet without a draft step.
- [ ] 10.4 Tests:
  - Go: draft parsing, the malformed-block note, and the drafting prompt on the first message only
  - JVM:
    - New effort opens a chat on the board's computer
    - Continue runs intake with the draft
    - the out-of-scope prefill keeps the epic relation
    - the non-OpenSpec form and the older-bridge sheet
    - no `DraftChat` remains
- Gate: `cd bridge && go vet ./... && go test ./...`
- Gate: `cd android && ./gradlew testDebugUnitTest`

<!-- nacre-section: sf2aff9c78c447bb2c0f65 -->
## 11. App: one chat behind ✨, the bead entry, the pane and the sheet

- [ ] 11.1 Rework `ChatStack` to hold one app-wide current chat id (persisted in settings), its collapsed state, and a `ChatViewState` per chat id (draft, pending images and scroll). Remove the per-screen `ChatContext` presentations.
- [ ] 11.2 Make the ✨ header action open the current chat, or New when there is none. Its menu lists New · <label> and the recent chats. Point the bead detail's chat entry at the same current chat.
- [ ] 11.3 Pane and sheet, reusing the existing chat-presentation layout:
  - keep the 760.dp check, the 360.dp minimum, the edge-tab collapse, the shared right pane on boards, and the Back order
  - keep the pane docked or collapsed across navigation on wide layouts
  - close the sheet when a link is followed on phones
- [ ] 11.4 When a chat is deleted, close its pane or sheet, drop its draft and pending images, and remove it from the header menu.
- [ ] 11.5 JVM tests:
  - ✨ and the bead entry open the same chat id
  - the pane stays docked and collapsed across a bead → round move
  - a phone link closes the sheet, and ✨ reopens it with its draft
  - Back collapses, then leaves
  - the embedded bead detail opens the chat in the board pane
  - deleting closes the view and drops the draft
- Gate: `cd android && ./gradlew testDebugUnitTest`

<!-- nacre-section: sfdc437d883b797ecadf5c -->
## 12. App: retire the bead chat, the board assistant, the old image paths and the superseded change

- [ ] 12.1 Remove `BoardAssistant`, its tools and approval flow, `AllBoardsRouter`, `ui/assistant/`, `ui/bead/BeadChat.kt` (Ask and Refine) and `BeadChats`, and their wiring in `NacreApp.kt`. Stop calling nacred's assist endpoints from the app; nacred keeps them for older apps.
- [ ] 12.2 Remove the NAC-48p parts that only served those surfaces, whichever are present:
  - bead chat attachment storage
  - the `MacSession` image budget
  - `ApiBackend` image history
  - the per-repo attach UI

  Keep what Chat reuses, and leave the API-key setting and `ClaudeBackend` in place.
- [ ] 12.3 Move Develop a proposal for unlinked beads to the bead's overflow menu only.
- [ ] 12.4 On the first launch after upgrading, delete `filesDir/bead-chats.json` and `filesDir/chat-attachments/`. Retry a failed delete at the next launch without blocking.
- [ ] 12.5 Delete `openspec/changes/chat-image-attachments/` without archiving it, so its requirements never reach the accepted specs. Leave the NAC-48p beads to section 13.
- [ ] 12.6 JVM tests:
  - no screen offers an AI chat other than Chat
  - the overflow menu offers Develop a proposal on an unlinked bead
  - upgrade cleanup deletes both, and a failed delete is retried
  - remove or port the tests of deleted classes
- Gate: `cd android && ./gradlew testDebugUnitTest`
- Gate: `openspec validate --all --strict`

<!-- nacre-section: sf6d2d3482e64f366442dc -->
## 13. Manual device checks (label the bead `human`)

- [ ] 13.1 On the Fold, folded and unfolded:
  - the bar or rail shows four destinations
  - the bar stays on bead, change, run and round screens and hides only while typing
  - nothing at the bottom is covered
- [ ] 13.2 In a Claude chat, ask which MCP servers, skills and CLAUDE.md instructions are loaded. Confirm they match a terminal `claude` session in the same repository on the Mac.
- [ ] 13.3 From the phone alone, in a Claude chat:
  - create and close a bead
  - start a proposal, confirming the agent reached nacred's API at `NACRED_URL`
  - start a round, confirming no extra card appears before it starts
  - build an APK and receive it by Taildrop
  - confirm that at least one step arrived as Allow/Deny, and that Deny refused it without hanging
  - after a turn that edited code, confirm the checkout has no unexplained uncommitted edits: the work is on a branch or worktree, committed, or named in the reply
- [ ] 13.4 Routing: ask a plain question and confirm a quick answer. Then ask "now do it" and confirm the handoff line, and that the full turn carries on with the earlier conversation.
- [ ] 13.5 With the chat docked on the unfolded Fold:
  - say "close this one" on a bead
  - then open a paused round and ask "why did this stop?"
  - confirm the answers use the bead, then the round's stop reason, without explanation
- [ ] 13.6 Confirm that ✨ on a board and the chat entry on a bead open the same chat, and that bead comments show no AI chat.
- [ ] 13.7 New effort: draft an effort in Chat, tap Continue, and confirm intake shows a verdict and the bead is created as chosen.
- [ ] 13.8 Images (carried over from NAC-48p.5):
  - send a screenshot in a Claude chat and in a Codex chat, and confirm each reply describes it
  - send a follow-up and confirm the resumed turn still knows the image
  - reopen the app and confirm the thumbnails
  - delete the chat and confirm nacred's copies are gone, noting that Claude Code's transcript still holds the image
- [ ] 13.9 Close and reopen the app mid-turn, and toggle airplane mode for 30 seconds mid-turn. Confirm the chat resumes with no lost or duplicated output.
- [ ] 13.10 Repeat a short task with Codex, and confirm an approval request reaches the phone.
- [ ] 13.11 With no run or round active, start a chat step that needs approval, background the app, and confirm one "Chat needs your OK" notification arrives and opens the card.
- [ ] 13.12 Once 13.8 passes, close NAC-48p.5, the other open children of epic NAC-48p and the epic itself, each with a comment that chat-image-attachments was superseded by agent-chat-tab.
- Gate: `openspec validate --all --strict`
