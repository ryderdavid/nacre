# agent-chat Specification

## Purpose
Whenever something doesn't fit Nacre's screens, Ryder has to leave the app for a freeform Claude Code or Codex session on the Mac. The bead chat and the ✨ board assistant are locked-down, read-only helpers (`--safe-mode`, `--permission-prompts none`, read-only `bd`). On NAC-48p.5 the bead chat could not build and install an APK for a sign-off. Ryder needs one fully capable agentic chat on the ho…

## Requirements

### Requirement: Chat runs the full agent harness on the host
nacred SHALL run full Chat turns with the official `claude` CLI in the owner's normal setup:
- without `--safe-mode`, `--no-session-persistence` or tool restrictions
- with `--permission-mode auto` and `--permission-prompts host`
- with the Nacre briefing appended to the system prompt
- with every board on the host added as a directory

The app MUST NOT call the Claude or OpenAI API for Chat, and the CLI's credentials MUST NOT leave the Mac.

#### Scenario: Owner's setup loads
- **WHEN** a full Claude chat turn starts
- **THEN** the argv contains `--permission-mode auto`, `--permission-prompts host`, `--append-system-prompt`, `--input-format stream-json` and an `--add-dir` for each board on the host, and contains none of `--safe-mode`, `--no-session-persistence`, `--tools` or `--system-prompt`

#### Scenario: Owner's configuration is not overridden
- **WHEN** a Claude chat turn starts
- **THEN** its environment keeps the owner's `HOME` and sets no `CLAUDE_CONFIG_DIR`, and its argv has none of `--setting-sources`, `--strict-mcp-config` or `--mcp-config`

#### Scenario: First and later turns
- **WHEN** a chat sends its first message and then a second
- **THEN** both turns run in the same working directory, the first starts with `--session-id <uuid>`, and the second with `--resume <uuid>` for the same id

#### Scenario: CLI without host prompts
- **WHEN** the installed Claude Code does not support `--permission-prompts host`
- **THEN** nacred does not advertise `agent_chat`, and the app shows the reason naming the required Claude Code version

### Requirement: A routing model chooses a quick answer or the full harness
Before each message in a Claude chat, nacred SHALL ask a small routing model, which has no tools, to choose `light` or `full`. The router SHALL see only:
- the message text and its "On screen" note
- how many images are attached
- whether the chat is a New effort chat
- the last six messages, trimmed to 2 KB

A `light` answer SHALL run on a lighter model and `full` on the full harness. An error, a 15-second timeout or a malformed answer SHALL mean `full`. The app SHALL NOT offer a model choice.

#### Scenario: Plain question
- **WHEN** the router answers `light` for a message
- **THEN** the message runs as a light turn with the lighter model in the chat's same agent session

#### Scenario: Real work
- **WHEN** the router answers `full` for a message
- **THEN** the message runs as a full turn with the full-harness argv

#### Scenario: Router fails
- **WHEN** the routing call errors, times out or returns something other than `light` or `full`
- **THEN** the message runs as a full turn

#### Scenario: No model picker
- **WHEN** Ryder starts a chat or sends a message
- **THEN** no control offers a model choice

### Requirement: A quick answer hands off to the full harness
A `light` turn SHALL run in the chat's own agent session with the lighter model in plan permission mode. A handoff happens when the light turn ends with a handoff line, asks permission for any step, or tries to leave plan mode. nacred SHALL then:
- refuse that step
- stop the light turn
- continue the same turn as a full turn that resumes the same session

Ryder does not resend, and nothing said so far is lost.

#### Scenario: Handoff keeps the conversation
- **WHEN** a message routed `light` ends with `NACRE_HANDOFF: needs to edit files`
- **THEN** the chat shows "Handed to the full harness", a full turn starts with `--resume` for the same session id, the turn's `turn_id` is unchanged, and the full turn's session holds every earlier message

#### Scenario: Permission asked during a quick answer
- **WHEN** a light turn raises a host permission request
- **THEN** the request is denied without a card on the phone, and a full turn on the same session carries out the message

#### Scenario: Quick answer tries to leave plan mode
- **WHEN** a light turn calls `ExitPlanMode` with a plan
- **THEN** the call is refused, the handoff records the plan as its reason, and a full turn on the same session carries out the message

#### Scenario: Route is recorded
- **WHEN** a turn produces assistant text
- **THEN** each assistant event records whether it came from the `light` or the `full` route

### Requirement: Chat respects the runaway ceiling
Each Claude chat MUST start every call with a budget limit of $50 minus what the chat has spent. This covers routing, quick answers and full turns. Once the chat has spent $50, nacred SHALL refuse new messages with a reason, and the app SHALL offer a new chat. Buttons and cards SHALL NOT show money.

#### Scenario: Remaining budget
- **WHEN** a chat that has spent $12 sends a message routed `full`
- **THEN** the turn starts with `--max-budget-usd 38.00`

#### Scenario: Routing counts
- **WHEN** a routing call costs $0.01 and the quick answer $0.20
- **THEN** the chat's spending rises by $0.21

#### Scenario: Limit reached
- **WHEN** a chat's spending reaches $50
- **THEN** the chat is `limit_reached`, sending answers 409 with "This chat reached its runaway limit. Start a new chat to keep going.", and New starts a chat that notes the old chat's title

### Requirement: Codex can be chosen instead
The new-chat sheet SHALL offer Claude or Codex.
- A Codex chat SHALL run through `codex app-server` with the workspace-write sandbox, network access and `on-request` approvals, and SHALL resume its thread on later turns.
- Codex chats SHALL NOT be routed.
- Codex SHALL be offered only while nacred lists `agent_chat_codex`.
- The sheet MUST state that Codex has no permission classifier and no USD runaway limit.

#### Scenario: Codex chosen
- **WHEN** Ryder starts a chat with Codex and sends two messages
- **THEN** nacred starts a thread with those settings, and the second message resumes the same thread id

#### Scenario: Codex isn't routed
- **WHEN** Ryder sends a plain question in a Codex chat
- **THEN** no routing call is made and the message runs as a Codex turn

#### Scenario: Codex not signed in
- **WHEN** Codex is not installed or not signed in on the computer
- **THEN** Codex is disabled in the sheet with nacred's reason, and Claude remains available

### Requirement: Steps the agent can't take alone are asked on the phone
When Claude Code asks the host for permission in a full turn, or Codex sends an approval request, nacred SHALL record a permission request naming the step and its reason, and the app SHALL show it as an Allow/Deny card. Allow MUST let the step run, and Deny MUST refuse it. Neither answer SHALL leave the turn hanging.

#### Scenario: Allow
- **WHEN** the classifier won't allow a command on its own and Ryder taps Allow
- **THEN** the command runs, the turn continues, and the card records Allowed

#### Scenario: Deny with a note
- **WHEN** Ryder taps Deny and adds "use the debug build"
- **THEN** the step is refused with that note, the agent continues its turn, and the card records Denied

#### Scenario: Answered twice
- **WHEN** a request is answered from a second device after it was decided
- **THEN** nacred answers 409, and both devices show the recorded outcome

#### Scenario: Stop
- **WHEN** Ryder taps Stop while a request is pending
- **THEN** the request is denied, the turn ends, and the chat is idle

### Requirement: Unanswered permission requests are denied
nacred MUST deny a chat permission request that has had no answer for 15 minutes, with a reason, and SHALL let the turn continue.

#### Scenario: No answer
- **WHEN** a request has been pending for 15 minutes
- **THEN** it is denied with "No answer from the phone within 15 minutes" and the turn continues

### Requirement: Permission requests reach the phone in the background
While any chat on a paired computer is running or waiting for permission, the app SHALL keep watching it in the background, even when no run is active. When the app is in the background, it SHALL post exactly one "Chat needs your OK" notification per permission request, which opens that chat at the card.

#### Scenario: App in the background
- **WHEN** a permission request arrives while the app is in the background
- **THEN** one "Chat needs your OK" notification opens that chat at the card

#### Scenario: Background with nothing else running
- **WHEN** the app goes to the background while a chat turn runs and no run or round is active, and the turn then raises a permission request
- **THEN** the app is still watching that chat, and exactly one "Chat needs your OK" notification is posted for the request

### Requirement: Only a paired device answers a permission request
nacred SHALL refuse, with 403 and a reason, an answer to a chat's permission request that comes from its own computer, whether through loopback or its own Tailscale address. A session on the Mac therefore can't approve its own step.

#### Scenario: Agent answers its own card
- **WHEN** a process on the Mac posts to `/v1/chats/{id}/permissions/{req}` at `NACRED_URL` with the owner's token
- **THEN** nacred answers 403 with "Answer from a paired device" and the request stays pending

#### Scenario: Answer from the phone
- **WHEN** the paired phone answers the same request
- **THEN** nacred accepts it

### Requirement: Chat uses nacred like any session on the Mac
Chat SHALL reach nacred through the `nacred` command and the API at `NACRED_URL`, which SHALL be nacred's own listen address, the one the `nacred` command uses. nacred SHALL NOT hold a call, or add a card, before developing, approving, starting a round, landing, finishing an epic or raising limits for a chat. Only these SHALL guard those actions:
- the auto classifier and its host prompts
- the runaway ceilings
- nacred's existing limits

#### Scenario: Start a round from Chat
- **WHEN** the agent runs `nacred round start NAC-1 NAC-2` in a chat
- **THEN** the round starts with no card on the phone, within the round's own budget, and Chat links to it

#### Scenario: Develop from Chat
- **WHEN** the agent asks nacred to develop a proposal for a bead through the API at `NACRED_URL`
- **THEN** development starts with no separate card, bounded by specflow's per-action limit, and the change appears in Proposals

#### Scenario: nacred listens only on Tailscale
- **WHEN** nacred is bound to the Mac's Tailscale address and nothing listens on 127.0.0.1
- **THEN** the chat's `NACRED_URL` names that Tailscale address and the agent's API calls reach nacred

### Requirement: Chats are long-lived and resumable
nacred SHALL keep each chat's record and event log across app closes, connection drops and nacred restarts, and SHALL run turns detached from the request.
- The app SHALL reattach to a running turn from the last event it has.
- A resend with the same `turn_id` MUST NOT start a second turn.
- The Chat tab SHALL list past chats from every paired computer.

#### Scenario: App closed and reopened
- **WHEN** Ryder closes the app during a turn and reopens Chat later
- **THEN** the chat is listed, its messages and thumbnails are shown, and the turn's output continues or is complete

#### Scenario: Brief connection drop
- **WHEN** the connection drops for 30 seconds mid-turn
- **THEN** the app reattaches with `after=<last seq>`, misses no events and shows none twice

#### Scenario: Resent message
- **WHEN** the app resends a message with the same `turn_id` after a lost response
- **THEN** nacred returns the existing turn and starts no second one

#### Scenario: nacred restarts mid-turn
- **WHEN** nacred restarts while a chat turn runs
- **THEN** the chat shows interrupted with a note, pending requests are closed as denied, and the next message resumes the same agent session

#### Scenario: Older bridge
- **WHEN** a computer does not list `agent_chat`
- **THEN** Chat shows "Update nacred on <computer> to use Chat" for it, and the other tabs are unchanged

### Requirement: Every message carries the screen it was sent from
The app SHALL attach to each message the context of the screen shown when Send is pressed:
- the screen kind and repository
- the bead, change, round or run it shows
- its selection and open pane
- a summary of up to 4 KB

With a docked pane or a sheet, that is the screen beside or under it. In the Chat tab it is the screen last shown before Chat, or all boards. The composer SHALL show what the next message is about.

#### Scenario: Selection and pane
- **WHEN** Ryder sends from a wide board with NAC-7 open in the right pane
- **THEN** the message's context names the board and NAC-7 as the selection

#### Scenario: Chat tab context
- **WHEN** Ryder opens a round, switches to the Chat tab and sends
- **THEN** the message's context is that round

#### Scenario: Repository on another computer
- **WHEN** the screen's repository has no checkout on the chat's computer
- **THEN** the composer says so and offers New chat on that computer, and a message sent anyway carries a note naming the other computer

#### Scenario: About chip
- **WHEN** Ryder moves from a bead to a round with the chat docked
- **THEN** the composer's chip changes from About: <bead id> to About: <round>

### Requirement: nacred tells the agent what was on screen
nacred SHALL put an "On screen" note before every user message. The note SHALL carry the full context when it differs from the previous message's, and a one-line reminder otherwise. On Boards, Inbox or Activity the note SHALL say the message is about all boards on that computer.

#### Scenario: This moves with Ryder
- **WHEN** Ryder asks "close this one" on bead NAC-3, then opens round r-12 and asks "what about this?" in the same chat
- **THEN** the first message's note names NAC-3, and the second's names round r-12 with its state, stop reason and choices

#### Scenario: Same screen again
- **WHEN** Ryder sends two messages from the same bead screen
- **THEN** the second note is the one-line "same as before" reminder

#### Scenario: From Inbox
- **WHEN** Ryder sends a message while Inbox is showing
- **THEN** the note says the message is about all boards on this computer

### Requirement: Chat starts where it was opened
A new chat SHALL run on the computer of the screen it was started from:
- in that repository's checkout for a board, bead, change, round or run screen
- across all boards on the across-boards computer for Boards, Inbox or Activity

#### Scenario: From a bead
- **WHEN** Ryder starts a chat with New · NAC-48p.5
- **THEN** it runs on that bead's computer in its repository's checkout

#### Scenario: From Boards
- **WHEN** Ryder starts a chat from the Boards screen
- **THEN** it runs on the across-boards computer with every board on that computer added as a directory

### Requirement: Every chat is briefed on Nacre
Every chat SHALL start with the Nacre briefing appended to its system prompt. The briefing SHALL cover:
- bd and OpenSpec changes
- the "On screen" note
- the `nacred` command and API for Needs you, changes, rounds, found work and sign-off
- making code changes on a branch or in a git worktree, and committing or naming edits left in a checkout before a turn ends
- the New effort draft block
- Taildrop delivery

#### Scenario: Briefing
- **WHEN** any chat starts
- **THEN** its system prompt is appended with the briefing, and its environment has `NACRED_URL` set to nacred's own listen address

#### Scenario: Edits in a checkout
- **WHEN** any chat starts
- **THEN** its briefing tells the agent that nacred lands into the repository checkouts, to make code changes on a branch or in a git worktree, and before ending a turn that edited a checkout to commit the edits or name the files left uncommitted

#### Scenario: Phone unknown for Taildrop
- **WHEN** nacred cannot resolve the phone's Tailscale device from the request
- **THEN** the briefing says the device is unknown and to ask Ryder

### Requirement: Chat can do the owner's work end to end
From the phone alone, a Claude chat SHALL be able to:
- create and close a bead
- start a proposal
- start a round
- build an APK and send it to the phone by Taildrop

It SHALL do these with the repository's own commands and nacred's command and API.

#### Scenario: Send me this build
- **WHEN** Ryder, on a bead, asks the chat "build and send me this APK"
- **THEN** the agent builds with the repository's build command, asks for any step the classifier won't allow, and runs `tailscale file cp` to the phone, and the APK arrives on the phone

### Requirement: Chat turns protect running work
A running chat turn, including its routing call, SHALL count as an active assistant call for the update maintenance reservation. It SHALL NOT take a `max_concurrent_runs` slot.

#### Scenario: Update during a chat
- **WHEN** an update is requested while a chat turn runs
- **THEN** the update is refused as busy until the turn ends

### Requirement: One chat replaces every other AI chat in the app
The app SHALL hold no AI chat other than Chat.
- The ✨ header action and the bead's chat entry SHALL open the current chat.
- The following SHALL be removed:
  - the board assistant, for one board and across boards
  - the bead chat (Ask and Refine)
  - the New effort draft step
- Bead comments SHALL stay plain comments.
- A lighter model SHALL be used only through routing within the same chat.

#### Scenario: Same chat from the header and the bead
- **WHEN** Ryder opens the chat with ✨ on a board, then taps the chat entry on a bead
- **THEN** both open the same chat with the same messages and draft

#### Scenario: Bead comments
- **WHEN** Ryder opens a bead's comments
- **THEN** they are plain comments with no AI reply or chat

#### Scenario: No second chat
- **WHEN** Ryder opens a board, a bead, the repositories screen or the New effort sheet
- **THEN** no assistant, bead chat or draft chat other than Chat is offered

### Requirement: New effort drafts in Chat
In a repository with OpenSpec, New effort SHALL start a new chat on the board's computer whose first message carries a drafting prompt for that board. When the agent writes a `nacre-draft` block, nacred SHALL record it as a draft. The app SHALL show the draft as a Draft card with Continue, which opens the New effort sheet with that draft and runs intake on it.

#### Scenario: Draft and intake
- **WHEN** Ryder taps New effort on a board, talks the idea through, and taps Continue on the Draft card
- **THEN** the New effort sheet opens with the drafted title and description, runs intake, and shows the verdict for Ryder to confirm or override

#### Scenario: Prefilled New effort
- **WHEN** Ryder answers a found item out of scope
- **THEN** a New effort chat opens with the note in the composer, and continuing from its draft creates a bead related to the epic

#### Scenario: Unreadable draft
- **WHEN** the agent's `nacre-draft` block doesn't parse or has no title
- **THEN** no Draft card is shown, the chat says "The draft couldn't be read" with the reason, and the conversation continues

#### Scenario: Older bridge
- **WHEN** the board's computer does not list `agent_chat`
- **THEN** New effort opens its sheet without a draft step, and intake still runs

### Requirement: Attach images to Chat messages
Chat SHALL offer Attach image beside the message field, in Claude and Codex chats. It SHALL open the system photo picker limited to images, with multiple selection. Up to 5 JPEG, PNG, GIF or WebP images per message SHALL be:
- downscaled to a long edge of 1568 px and at most 3 MB
- shown as removable thumbnails
- sent with the next message

#### Scenario: Pick and send
- **WHEN** Ryder attaches two screenshots, types a question and sends
- **THEN** the message is sent with both images, and the sent message shows two thumbnails

#### Scenario: Images only
- **WHEN** Ryder attaches an image and sends with an empty text field
- **THEN** the message is sent with the image

#### Scenario: Remove before sending
- **WHEN** Ryder removes one of three pending thumbnails and sends
- **THEN** only the two remaining images are sent

#### Scenario: Unsupported or unreadable file
- **WHEN** a picked file is not a supported image, cannot be decoded, or cannot be brought under 3 MB
- **THEN** it is not attached, a snackbar names it, and the other picked images stay attached

#### Scenario: Limit reached
- **WHEN** five images are pending
- **THEN** Attach image is disabled until one is removed or the message is sent

#### Scenario: Pending images follow the draft
- **WHEN** Ryder collapses a docked chat with pending images, or moves between the pane, the sheet and the Chat tab, and returns
- **THEN** the same pending images are still attached and nothing was sent

### Requirement: nacred accepts image messages within bounded limits
nacred SHALL accept bodies up to 32 MiB on `POST /v1/chats/{id}/messages`, keeping every other endpoint's limit, and SHALL answer 413 with a reason above it. It MUST validate every image before writing any file or starting any turn, MUST refuse more than 5 images in one chat message, and MUST answer 400 with a reason naming the image or limit on failure.

#### Scenario: Largest allowed message
- **WHEN** a message carries 5 valid images of 3 MB each
- **THEN** nacred accepts it and starts the turn

#### Scenario: Too many images in one message
- **WHEN** a chat message carries 6 valid images
- **THEN** nacred answers 400 naming the 5-image limit, writes nothing and starts no turn

#### Scenario: Invalid image
- **WHEN** a message carries an image whose bytes don't match its media type, or that is over 3 MB
- **THEN** nacred answers 400 naming the image, writes nothing and starts no turn, and the app keeps the draft and shows the reason

#### Scenario: What is checked
- **WHEN** nacred validates a message's images
- **THEN** it checks that each media type is JPEG, PNG, GIF or WebP, that the base64 decodes, that the bytes' signature matches the media type, that each image is at most 3 MB decoded, and that there are at most 5

### Requirement: Chat images reach the agent
nacred SHALL pass a message's images to Claude as image content blocks in the stream-json user message, each after an `Image N: <name>` text block and before the message text. To Codex it SHALL pass them as local image inputs from its stored copies. Only the current message's images SHALL be sent, because the session holds earlier ones.

#### Scenario: Claude describes the screenshot
- **WHEN** Ryder sends a screenshot with "what's wrong here?" in a Claude chat
- **THEN** the stream-json user message holds an `Image 1` label, the image block and the text, and the reply can describe the screenshot

#### Scenario: Follow-up sends no images again
- **WHEN** Ryder sends a text-only follow-up about that screenshot
- **THEN** the turn carries no image blocks, and the resumed session still has the screenshot

#### Scenario: Codex gets the image
- **WHEN** Ryder sends a photo in a Codex chat
- **THEN** `turn/start` carries a local image input pointing at the stored copy

### Requirement: Each chat bounds its images
nacred and the app SHALL refuse an image message that would take a chat past 20 images or 15 MB decoded. The app SHALL keep the pending images and disable Send with "This chat has reached its image limit. Start a new chat to send more images." Removing the images SHALL let the text be sent.

#### Scenario: Chat budget reached
- **WHEN** a chat has sent 18 images and Ryder attaches three more
- **THEN** Send is disabled with the image-limit reason, the images stay pending, and removing them lets the text be sent

#### Scenario: nacred refuses too
- **WHEN** a message that passes the chat's image limit reaches nacred
- **THEN** nacred answers 409 with the image-limit reason and starts no turn

### Requirement: Chat images live with their chat
nacred SHALL store each sent image under the chat's directory, with the directory at mode 0700 and the file at 0600. It SHALL serve the image for thumbnails and delete it when the chat is deleted or trimmed. Claude Code and Codex keep their own copies in their session transcripts on the Mac until their own cleanup. A thumbnail that can't be loaded SHALL show Image unavailable.

#### Scenario: Reopen shows thumbnails
- **WHEN** Ryder reopens a chat with images on another day or another paired phone
- **THEN** the sent messages show their thumbnails, fetched from nacred

#### Scenario: Chat deleted
- **WHEN** Ryder deletes a chat that sent images
- **THEN** nacred's stored copies of its images no longer exist

#### Scenario: Missing image
- **WHEN** a stored image can't be fetched
- **THEN** its thumbnail shows Image unavailable and the chat keeps working

### Requirement: Saved bead chat threads are removed on upgrade
On the first launch after upgrading, the app SHALL delete the stored bead chat threads and their attachment files. A failed delete SHALL be retried at the next launch and SHALL NOT block the app.

#### Scenario: Upgrade cleanup
- **WHEN** the app starts for the first time after this change with `bead-chats.json` and `chat-attachments/` present
- **THEN** both are deleted

#### Scenario: Cleanup fails
- **WHEN** deleting them fails
- **THEN** the app opens normally and the delete is tried again at the next launch
