## Why

Whenever something doesn't fit Nacre's screens, Ryder has to leave the app for a freeform Claude Code or Codex session on the Mac. The bead chat and the ✨ board assistant are locked-down, read-only helpers (`--safe-mode`, `--permission-prompts none`, read-only `bd`). On NAC-48p.5 the bead chat could not build and install an APK for a sign-off.

Ryder needs one fully capable agentic chat on the host, reachable from every screen, that knows what he is looking at whenever he speaks.

## What Changes

- **Chat tab.** A fourth destination, Chat, joins Boards, Inbox and Activity. The phone tab bar has four equal items, and the tablet rail includes Chat.
- **Tab bar everywhere.**
  - The `detailRoute` rule in `ui/NacreRoot.kt`, which hides the bar on bead, run and round screens, is removed.
  - On phones the bar hides only while the keyboard is open. Bottom sheets may cover it.
  - The chat dock, the change screen's actions, the New FAB and snackbars sit above the bar.
  - Back behaves as it does today.
- **One chat (owner decision).**
  - Chat replaces the ✨ board assistant (for one board and across boards), the bead chat (Ask and Refine) and the New effort draft step (`DraftChat`). Nothing else in the app holds an AI chat.
  - The ✨ header action and the bead's chat entry open the same chat: docked beside the screen on wide layouts, a sheet on phones, full screen in the Chat tab.
  - Bead comments stay plain comments.
- **Routing inside the one chat (owner decision).**
  - Before each message, a small routing model with no tools reads the message, its screen context, how many images are attached, whether it is a New effort chat and the last few messages. It chooses a quick answer or the full harness.
  - Plain questions go to a lighter model in the same agent session, in read-only plan mode. Real work goes to the full harness.
  - A quick answer that turns out to need real work hands off to the full harness in the middle of the turn, resuming the same session, so nothing said so far is lost and Ryder doesn't resend.
  - Ryder never picks a model. If routing fails, the message goes to the full harness.
- **Context from the screen when you speak.**
  - Every message carries what is on screen at the moment it is sent:
    - the board, bead, change, run or round
    - any selection or open pane
    - a short summary of the state
  - The chat follows Ryder between screens. "This one" on a bead, then "what about this?" on a round, refers to the round.
  - The repository comes from the current screen. On Boards, Inbox or Activity the message is about all boards.
  - The composer shows what the next message is about.
- **Full harness on the subscription.**
  - nacred runs the official `claude` CLI with the owner's normal setup: MCP servers, CLAUDE.md, skills and hooks, with no `--safe-mode`.
  - Flags: `--permission-mode auto` and `--permission-prompts host`, with nacred answering the host prompts.
  - The OAuth token never leaves the Mac, and the app never calls the API for Chat.
  - The usual $50 runaway ceiling applies to each chat, counting routing and quick answers.
- **Codex instead of Claude.** A chat can run on Codex through `codex app-server`, signed in with ChatGPT. Codex approval requests reach the phone the same way, and Codex chats are not routed.
  - **Codex has no classifier.** The closest equivalent is the workspace-write sandbox with network access and `on-request` approvals, so anything outside the sandbox asks the phone.
  - Codex reports no USD cost, so the $50 ceiling can't be enforced for Codex chats. The ChatGPT plan's own usage limits apply, and the app says so when Codex is chosen.
- **Allow/Deny on the phone.**
  - A step the classifier (or Codex's sandbox) won't allow on its own becomes an Allow/Deny card in the chat. A notification is sent when the app is in the background, even when nothing else is running.
  - Allow runs the step and Deny refuses it.
  - An unanswered card is denied after 15 minutes with a reason, so the session never hangs. Stop ends the turn at any time.
  - Only a paired device can answer a card. An answer from the Mac itself is refused, so a session there can't approve its own step.
- **No extra card before spending (owner decision).**
  - Chat uses nacred's API and the `nacred` command like any session on the Mac. The agent reaches the API at nacred's own listen address, the one the `nacred` command uses. Developing, approving, starting rounds, landing, finishing epics and raising limits get no separate Allow card.
  - The guards are:
    - the auto classifier and its host prompts
    - the chat's $50 runaway ceiling
    - the limits nacred already applies: specflow's per-action limit and each round's budget
- **New effort drafts in Chat (owner decision).**
  - New effort opens a new chat on the board's computer with a drafting prompt.
  - When the agent writes its draft, the chat shows a Draft card. Continue opens the New effort sheet with that draft, and intake runs as it does today.
  - `DraftChat` is removed.
- **Images in Chat (owner decision).**
  - Attach image works in Chat from the start, in Claude and Codex chats. It allows up to 5 images per message, downscaled to at most 3 MB each, with a limit per chat.
  - This builds on the work of `chat-image-attachments` (epic NAC-48p) on branch `epic/nac-48p`. The round's pre-flight question about that unmerged branch is to be answered "merge it first", which nacred carries out. The nacred and app image sections bring in, as ordinary commits, any piece of it Chat reuses that is still missing.
  - `chat-image-attachments` is superseded and is withdrawn by this change: its change directory is deleted without archiving, and epic NAC-48p is closed with a pointer here once this change's image device checks pass. Its open device check, NAC-48p.5, becomes part of this change's device checks.
- **Long-lived sessions.**
  - Chats are records on nacred, with an event log that survives the app closing, connection drops and nacred restarts.
  - Each message resumes the agent's own session (`--resume` / `thread/resume`).
  - The app reattaches to a running turn the same way assistant turns do today.
  - The Chat tab lists past chats from every paired computer.
- **Briefing.** Every chat starts with a short briefing on:
  - bd and OpenSpec changes
  - nacred's API and command: Needs you, rounds, changes, found work and sign-off
  - making code changes on a branch or in a git worktree, and not leaving uncommitted edits in a repository checkout that nacred also lands into
  - the New effort draft format
  - APK delivery to the phone by Taildrop
- **Capability flags.**
  - nacred advertises `agent_chat` in `features` only once it can handle host permission prompts.
  - It advertises `agent_chat_codex` when Codex can serve chats.
  - With an older nacred, Chat explains that an update is needed for that computer.
- **Retired.**
  - `BoardAssistant`, `AllBoardsRouter`, the bead chat UI and its `bead-chats.json` store, and `DraftChat` are removed. Saved bead chat threads and their `chat-attachments/` files are deleted once, on upgrade.
  - Accepted requirements that named the assistant, Refine or the draft step are rewritten to name Chat: needs-you, round-app, run-rounds, spec-surfacing and effort-intake.
  - The spec-surfacing rule that spending tools always wait for Ryder's approval is replaced by the owner's decision above.
  - The chat-presentation requirements that tied a chat to one screen are replaced by a chat that follows Ryder.

Out of scope:

- calling the Claude or OpenAI API from the phone for Chat, or using an API key for Chat
- removing the app's API-key setting or `ClaudeBackend`, even where Chat leaves them unused
- removing nacred's assist endpoints or the per-repo turn endpoints, which older apps still call
- a model picker, or routing for Codex chats
- a permission classifier for Codex, or USD cost limits for Codex chats
- non-image attachments such as PDFs, a full-screen image viewer, editing or camera capture
- chats that span several computers in one session
- deleting Claude Code's or Codex's own session transcripts on the Mac, including images in them
- syncing a chat's draft or pending images between two phones
- new release or build tooling: Chat uses the repository's own build commands and `tailscale file cp`
- counting chat turns against `max_concurrent_runs`
- changing how nacred lands, commits `nacre.yaml` or finishes epics when the checkout has someone else's edits

## Capabilities

### New Capabilities

- `agent-chat`: one full-harness Claude Code or Codex chat on the host, with:
  - routing between a quick answer and the full harness
  - host permission prompts answered from the phone
  - resumable sessions
  - context taken from the screen at send time
  - images
  - New effort drafting
  - a Nacre briefing

  It replaces the bead chat, the board assistant and the draft step.
- `app-navigation`: four top-level destinations, with the tab bar visible on every screen except while the keyboard is open.

### Modified Capabilities

- `chat-presentation`: the ✨ action and the bead's chat entry open the one chat, which follows Ryder between screens. Deleted chats are not restored.
- `needs-you`: Chat, not the board assistant, starts from Needs you.
- `round-app`: Chat starts rounds through nacred.
- `run-rounds`: the app's third way to start rounds is directly or from Chat.
- `spec-surfacing`:
  - Chat starts proposal development through nacred, without a separate approval card.
  - Develop a proposal moves from Refine to the overflow menu.
- `effort-intake`:
  - New effort drafts in Chat before intake.
  - Chat records found work with the three answers.

## Impact

- **nacred:**
  - a new `bridge/internal/chat` package: records, the turn runner, routing and handoff, per-message context notes, host-prompt handling, chat images and New effort drafts
  - `bridge/internal/claude`: a new `Chat` argv builder next to `Headless` and `Turn`
  - `bridge/internal/codex`: an app-server JSON-RPC client
  - `bridge/internal/server`:
    - the `/v1/chats` endpoints and features
    - the image body limit on chat messages, reusing `images.go` (`decodeTurn` gains a limit parameter)
    - the maintenance reservation, which counts chat turns
  - nacred config: `chat.router_model` and `chat.light_model`
- **API contract:** `docs/API.md` documents:
  - the chat endpoints, events and per-message context
  - routing and handoff
  - permission requests
  - images and drafts
  - features and limits

  Both sides change together.
- **App:**
  - `ui/NacreRoot.kt`: the Chat tab and bar visibility
  - new `ui/chat/` screens, and a screen-context holder fed by the board, bead, change, round, run, Boards, Inbox and Activity screens
  - `ChatStack`: one app-wide open chat
  - Attach image in Chat, reusing NAC-48p's picker and downscaling
  - the Draft card, and New effort opening Chat
  - `data/bridge/BridgeClient.kt`, `Models.kt` and `NacreRepository`: chats
  - a notification for permission requests, with the background watcher also following active chats
  - removal of `BoardAssistant`, `AllBoardsRouter`, `BeadChats`, `ui/bead/BeadChat.kt`, `ui/assistant/` and `DraftChat`, with a one-time cleanup of their files
  - bottom padding on bead, change, run and round screens
- **Other changes:** `openspec/changes/chat-image-attachments` is deleted without archiving, and epic NAC-48p and its open children are closed with a pointer here after the image device checks. Its branch `epic/nac-48p` is brought in by the round's pre-flight "merge it first" answer.
- **Tests:**
  - Go tests for nacred
  - JVM unit tests for the app
  - manual device checks for the end-to-end acceptance, including NAC-48p.5's image checks
- **Older bridges:** for a computer without `agent_chat`, Chat shows an update message, and New effort opens its sheet without a draft step.
