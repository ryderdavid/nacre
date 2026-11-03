<!-- nacre-section: sf76644754568d20c31146 -->
## 1. Attachment model and bead chat storage

- [ ] 1.1 Add `ChatImage(id, mimeType, fileName)` and `images` (default empty) to `DraftTurn`, and add `ImagePayload` bytes to `TurnInput`. Text-only turns are unchanged.
- [ ] 1.2 In `BeadChats.kt`/`StoredChats`, persist image references only. Write files to `filesDir/chat-attachments/<id>.<ext>` via a temporary file and a rename. On a write failure, remove that message's partial files and report an error. `NacreApp.kt` provides the directory.
- [ ] 1.3 Delete attachment files on `clear(key)` and on the `MAX_THREADS` trim, after the JSON is saved. Sweep unreferenced files on load.
- [ ] 1.4 Add a resolver that selects each request's images: the current message's images first, then earlier images newest first while the request stays within 20 images and 15 MB of decoded data. It returns them in one list numbered 1..N, oldest turn first. Have `Prompts.ask/refine` write `[image N: <name>]` into each image's turn, `[image omitted: <name>]` for images that don't fit, and `[image unavailable: <name>]` for missing or unreadable files. A thread without images builds today's prompt.
- [ ] 1.5 JVM tests:
  - references round-trip with no base64 in the JSON
  - older JSON loads as text-only
  - clear deletes files
  - the 61st thread evicts the oldest thread and its files
  - an orphan sweep
  - a failed write leaves no files and no turn
  - the 20-image cap and the 15 MB byte budget, including a thread with 30 MB of earlier images plus five new 3 MB images
  - numbering and markers across two turns with images, and omitted and missing-file notes at their markers
  - a text-only thread's prompt equals today's
- Gate: `cd android && ./gradlew testDebugUnitTest`

<!-- nacre-section: sf0a5faec9ce482bfb76da -->
## 2. ApiBackend image blocks

- [ ] 2.1 In `ApiBackend`, build the bead chat user message as, for each resolved image, an `Image N: <name>` text block and its image block (base64, media type), followed by the prompt text block. For the board assistant, put each user message's own images before its text within the same budget, with notes for the rest. A turn with no images builds exactly today's params. A request-too-large API error surfaces its reason like other failed turns.
- [ ] 2.2 JVM tests:
  - built params for an image turn
  - for a bead chat with earlier images, labels in marker order, and omitted-image notes
  - for a board assistant history with images in their own messages
  - for a text-only turn equal to today's
- Gate: `cd android && ./gradlew testDebugUnitTest`

<!-- nacre-section: sf440f657276aacf83b9c9 -->
## 3. nacred image turns and API contract

- [ ] 3.1 In `bridge/internal/server`, read the bodies of `/v1/repos/{repo}/claude/json` and `/v1/repos/{repo}/claude/stream` with a separate `maxTurnBody` of 32 MiB, through a `decode` variant that takes the limit, and answer 413 with a reason above it. Keep `maxBody` (1 MiB) for every other endpoint, including `/v1/claude/stream`. Accept one optional ordered `images` array on those two requests. Validate before writing any file or starting any process:
  - the media type is one of the four
  - the base64 decodes
  - the bytes' signature matches the media type
  - each image is at most 3 MB decoded
  - at most 20 images and 15 MB of decoded image data per request
  - a failure returns 400 naming the image or the limit
- [ ] 3.2 Stage the images in a per-turn 0700 temp directory with 0600 files. In `bridge/internal/claude` `Headless`, when there are images, drop the positional prompt, add `--input-format stream-json` and keep every other flag (`--system-prompt`, `--json-schema`, `--resume`, `--safe-mode`, and `--no-session-persistence` when the turn isn't kept). Write one stream-json user message to stdin with the `Image N: <name>` text blocks and image blocks in array order, followed by the prompt text, then close stdin. Remove the directory in a deferred cleanup on every way a turn can end. Sweep leftover directories at startup. Requests without images keep today's command line and stdin.
- [ ] 3.3 Add `chat_images` to the list returned by `s.features(cx)`.
- [ ] 3.4 Document in `docs/API.md`:
  - the `images` field, its order and the `[image N: …]` markers in `prompt`
  - which endpoints accept it, and the turn-endpoint body limit
  - the image limits, the 400 and 413 errors and the temp-file lifetime
  - that a kept assistant session (`keep: true`) holds its images in Claude Code's session transcript on the Mac until Claude Code's own cleanup removes them, and that nacred does not delete it
  - the `chat_images` feature and older-bridge behavior
- [ ] 3.5 Go tests:
  - a request with 20 valid images totalling 15 MB is accepted on each of the two endpoints
  - a 32 MiB + 1 turn body gets 413, and `/v1/claude/stream` and another endpoint still reject bodies over 1 MiB
  - the exact argv and stdin for an image Ask turn with `--json-schema` and an image assistant turn with `--resume`: no positional prompt, `--input-format stream-json`, other flags unchanged, and the prompt text appears once, on stdin
  - an image turn with `keep: false` still has `--no-session-persistence`, and one with `keep: true` doesn't, as today
  - a text-only turn has an unchanged command line and stdin
  - each validation error returns 400 with no process and no files
  - the temp directory is removed after success, error, cancel, timeout and client disconnect
  - the startup sweep
  - `features` contains `chat_images`
- Gate: `cd bridge && go vet ./... && go test ./...`
- Gate: `openspec validate chat-image-attachments --strict`

<!-- nacre-section: sfd7b230ee7b6708cab599 -->
## 4. MacBackend sends images

- [ ] 4.1 Add an optional `images` list (`media_type`, `name`, base64 `data`) to `AssistRequest`. In `MacBackend` for bead chat, fill it from the resolver in marker order, with the markers in `prompt`. Omit the field when there are none. Surface nacred's 400 and 413 reasons.
- [ ] 4.2 In `MacSession` for one board, send only the current turn's images, with markers numbered within that turn. Count the images and decoded bytes sent in the conversation, refuse a send that would pass 20 images or 15 MB with the conversation-limit reason, and reset the count in `reset()`. Send no images from the across-boards branch.
- [ ] 4.3 Expose whether the target computer, the host in the bead's or board's repository key, lists `chat_images` in its features. Re-evaluate it when its features refresh, when the backend setting changes and at send, so the UI can disable attach and image sends with a reason.
- [ ] 4.4 JVM tests:
  - request JSON with images in marker order
  - a text-only request identical to today's
  - bead chat image requests are sent with `keep = false`
  - a second Mac assistant turn carries only its own images
  - the conversation budget blocks a send that would pass 20 images or 15 MB, and `reset()` clears it
  - the feature is missing
  - the target's features change while images are pending
  - 400 and 413 reasons are shown
- Gate: `cd android && ./gradlew testDebugUnitTest`

<!-- nacre-section: sf361690a8127f5726a32f -->
## 5. Attach image in bead chat and the board assistant

- [ ] 5.1 In `ui/bead/BeadChat.kt` (Ask and Refine) and `ui/assistant/` for one board:
  - add an Attach image icon button using `PickMultipleVisualMedia(ImageOnly)`, limited to 5 per message
  - downscale to a long edge of 1568 px and at most 3 MB, re-encoding as JPEG when needed
  - skip unsupported, undecodable or oversized files with a snackbar
  - show removable pending thumbnails held in the chat's view state with the draft
  - enable send when there is text or at least one image
  - when the backend can't take images, or the Mac assistant conversation has reached its image limit, disable attach with the reason; if images are pending, keep them and disable Send with the reason until they are removed
- [ ] 5.2 Show thumbnails on sent user messages, with an "Image unavailable" placeholder for missing files.
- [ ] 5.3 In `BoardAssistant.kt`, keep images in memory with the messages, never write them to the app's storage, and drop them in `clear()`. Do not add attach to the new-bead draft chat, `AllBoardsRouter.kt` or the assistant across boards.
- [ ] 5.4 JVM tests:
  - the pending-image state survives collapse and pane swaps
  - the limit disables attach
  - removing a pending image
  - an images-only send
  - losing image support with images pending keeps them and blocks Send until they are removed
  - reaching the Mac assistant's conversation limit keeps pending images and blocks Send with the reason
  - `clear()` drops the assistant's images and nothing is written to the app's storage
  - the out-of-scope chats, including the assistant across boards, have no attach action
- [ ] 5.5 Manual device check (label the bead `human`):
  - On the API backend and on the Mac backend, send a screenshot in a bead chat and in the board assistant, and confirm Claude's reply describes it.
  - On the Mac, send a follow-up in the assistant and confirm the resumed turn works and that each turn's nacred temp directory is gone afterwards.
  - On the Mac, after that resumed assistant turn, confirm the image is in that session's Claude Code transcript under the Mac user's Claude Code session storage and in no nacred directory. Clear the assistant and confirm the transcript is still there, as the spec states.
  - On the Mac, after a bead chat image turn, confirm no session transcript holds the image.
- Gate: `cd android && ./gradlew testDebugUnitTest`
- Gate: `openspec validate chat-image-attachments --strict`
