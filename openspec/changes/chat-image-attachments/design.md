## Context

- Every message goes through `ClaudeBackend`, which has two implementations:
  - `ApiBackend` calls the Claude API from the phone through anthropic-java.
  - `MacBackend` sends the turn to nacred, which runs `claude -p --output-format stream-json`.
- nacred's turn endpoints with a request body:
  - `POST /v1/repos/{repo}/claude/json` (bead chat)
  - `POST /v1/repos/{repo}/claude/stream` (bead chat and the board assistant for one board)
  - `POST /v1/claude/stream` (the board assistant across boards, on `planAcross`'s host)
  - `GET /v1/assist/{turn}/stream` reattaches to a running turn, and `/cancel` has no body.
- Per-board turns run on the host in the repository key (`Keys.host(repoKey)`).
- nacred reads every request body through `http.MaxBytesReader` with `maxBody = 1 << 20` (1 MiB) in `server.go`, through the shared `decode()` helper.
- nacred advertises optional capabilities as a `features` list (`s.features(cx)`), which the app reads through `Features` in `Models.kt`.
- `DraftTurn(fromUser, text)` in `ClaudeService.kt` and `TurnInput` carry only strings.
- Bead chat history has no per-turn structure on the wire. `ClaudeService.ask/refine` flatten the history into one prompt with `Prompts.ask/refine(ctx, history, …)`. `ApiBackend` sends it as one `.addUserMessage(prompt)`, and `MacBackend` as one `AssistRequest.prompt`.
- The board assistant on the API backend keeps real per-message turns. On the Mac backend, `MacSession` sends `sessionId` with `keep = true`, so each later turn is a `claude -p --resume`, and Claude Code replays earlier turns, including their images, from the session.
- nacred maps `keep` to `Persist: req.Keep`. Turns with `keep = true` (the single-board assistant) run without `--no-session-persistence`, so Claude Code writes each turn, including stream-json image blocks, into its own session transcript on the Mac. Bead chat turns are sent with `keep = false` and keep `--no-session-persistence`.
- `Headless.Args()` passes the prompt as the positional `-p <prompt>`, together with `--system-prompt`, `--json-schema` and, for the assistant, `--resume`.
- `BeadChats` saves threads in `filesDir/bead-chats.json` (`StoredChats`). It keeps the 60 most recently used threads (`MAX_THREADS`), and `clear(key)` deletes a thread.
- `BoardAssistant` keeps its conversation in memory until `clear()`.
- The Claude Messages API rejects requests over about 32 MB, and base64 inflates image data by about a third.

## Decisions

1. **Types.**
   - `ChatImage(id, mimeType, fileName)` is a reference. The id is a random UUID.
   - `DraftTurn` gains `images: List<ChatImage> = emptyList()`. The default keeps the existing JSON and the serialized form compatible.
   - `TurnInput` gains `images: List<ImagePayload>` for the turn being sent, where `ImagePayload` holds the `ChatImage` and its bytes.
   - A resolver hands each backend the bytes for images in earlier turns.
2. **Picking and preparing.**
   - Attach image opens `PickMultipleVisualMedia(ImageOnly)` with a maximum of 5 minus the images already pending. It needs no storage permission.
   - Each picked image is decoded and downscaled so its long edge is at most 1568 px, keeping its type when that type is supported. It is re-encoded as JPEG, lowering quality if needed, to stay within 3 MB. An image that can't be brought under 3 MB is skipped with a snackbar.
   - A file that is not JPEG, PNG, GIF or WebP, or that cannot be decoded, is not attached, and a snackbar names it. Animated GIFs send their first frame.
   - Pending images live in memory in the chat's view state next to the draft text. They survive collapse, sheet and pane swaps and returning to the screen, as chat-presentation requires for the draft. They do not survive process death.
3. **Sending.**
   - A message can be images only. Send is enabled when there is text or at least one image, subject to decisions 9 and 12.
   - For bead chats, each image is written to `filesDir/chat-attachments/<id>.<ext>` before the turn is recorded, via a temporary file and a rename.
   - If a write fails, any files already written for that message are deleted, nothing is sent, and the draft text and images are kept. A snackbar gives the reason.
4. **Thread storage.** `StoredChats` stores each user turn's `images` as `{id, mime_type, file_name}`, never bytes or base64. Threads saved by older versions have no `images` field and load as text-only.
5. **Lifetime and cleanup.**
   - `clear(key)` and the `MAX_THREADS` trim both collect the attachment ids of the removed threads. The JSON is saved first, then those files are deleted.
   - When `BeadChats` loads, it deletes files in `chat-attachments/` that no thread references. This finishes any deletion that was interrupted.
   - A file that fails to delete is retried at the next load and never blocks the user.
6. **Request budget and selection.**
   - A request carries at most 20 images and at most 15 MB of decoded image data (about 20 MB as base64), well under the API's limit.
   - The current message's images always fit: 5 × 3 MB = 15 MB.
   - The resolver selects the current message's images first, then earlier turns' images newest first, while both the count and the byte budget allow. A thread therefore never grows into requests that fail.
   - An earlier image that doesn't fit becomes `[image omitted: <file name>]`. A referenced file that is missing or unreadable becomes `[image unavailable: <file name>]`, and its thumbnail shows an "Image unavailable" placeholder. The turn is still sent.
7. **Request shape for bead chat (flattened history).**
   - Each request carries one ordered list of the selected images, numbered 1..N in chronological order: oldest turn first, the current message last.
   - `Prompts.ask/refine` write `[image N: <file name>]` into the text of the turn each image belongs to, in history and in the current message. An omitted or unavailable image gets its note at the same place instead of a number.
   - Each image is sent as a text block `Image N: <file name>` followed by its image block. All of them come before the prompt text block, so Claude can tie each image to its turn.
   - A turn with no selected images and no notes produces exactly today's prompt text.
8. **ApiBackend.**
   - Bead chat: the single user message is the labelled image blocks (base64 source, `media_type`) followed by the prompt text block, using anthropic-java's content block params.
   - Board assistant: each user message keeps its own images before its text, within the decision 6 budget. Omitted or unavailable images become the same notes in that message's text.
   - A turn with no images builds exactly today's message. If the API still answers request too large, the error's reason is shown, as with any failed turn.
9. **Mac protocol.**
   - `AssistRequest` gains one optional ordered `images` array of `{media_type, name, data}`, where `data` is base64, in the order of decision 7. There are no per-history-entry fields; the markers are in `prompt`.
   - Only `/v1/repos/{repo}/claude/json` and `/v1/repos/{repo}/claude/stream` accept `images`. `/v1/claude/stream` (across boards) does not.
   - A request without `images` is handled exactly as today.
   - nacred adds `chat_images` to its `features` list.
10. **Target computer and capability.**
    - On the Mac backend, the computer that handles a bead chat or single-board assistant turn is the host in that bead's or board's repository key, the one `MacBackend` already sends the turn to.
    - Attach image is enabled only when that computer's `features` contain `chat_images`. Otherwise it is disabled with "Update nacred on <computer> to send images". On the API backend no feature is needed.
    - Support is checked again whenever that computer's features refresh, when the backend setting changes, and at send. If images are pending and the target no longer supports them (for example, nacred was downgraded or the backend was switched to Mac), the pending images are kept, Send is disabled with the same reason, and removing the images lets the text be sent as today.
    - The board assistant across boards (`board.runKey == null`) offers no Attach image on either backend in v1.
11. **nacred handling.**
    - **Body limit.** `/v1/repos/{repo}/claude/json` and `/v1/repos/{repo}/claude/stream` read their bodies with a separate `maxTurnBody` of 32 MiB, through a `decode` variant that takes the limit. Every other endpoint, including `/v1/claude/stream`, keeps the 1 MiB `maxBody`. A larger turn body is answered 413 with a reason.
    - **Validation** happens before any file is written or process started. nacred checks:
      - the media type is one of the four
      - the base64 decodes
      - the bytes' signature matches the media type
      - each image is at most 3 MB decoded
      - there are at most 20 images and at most 15 MB of decoded image data in the request

      A failure returns 400 with a reason naming the image or the limit, and nothing is written.
    - **Staging.** Images are written to a per-turn directory under the OS temp directory with mode 0700 and files with mode 0600.
    - **Delivery.** `Headless` gains the staged images. When there are any, `Args()` emits `-p` with no positional prompt and adds `--input-format stream-json`. Every other flag stays as today, including `--system-prompt`, `--json-schema`, `--resume`, `--safe-mode`, `--no-session-persistence` when the turn isn't kept, and `--output-format stream-json --verbose`. nacred writes one stream-json user message to stdin, whose content is the labelled image blocks read from the staged files followed by the prompt text block, and then closes stdin. The prompt is sent once, on stdin only. Turns without images keep today's command line and stdin.
    - **Cleanup.** The turn directory is removed in a deferred cleanup that runs on success, error, cancel, timeout and client disconnect. At startup, nacred removes any leftover turn directories under its temp prefix. This covers nacred's own copies only. Claude Code's session transcript for a kept assistant session is Claude Code's, and nacred does not touch it (decision 12).
12. **Board assistant.**
    - On the phone, images are held in memory with the `BoardAssistant` messages, are never written to the app's storage, and are dropped by `clear()`.
    - On the API backend, the decision 6 request budget applies to each request.
    - On the Mac backend, the resumed session already holds earlier images, which the app can't omit. `MacSession` therefore sends only the current turn's images, with markers numbered within that turn, and counts the images and decoded bytes it has sent in the conversation.
    - A send that would take the conversation past 20 images or 15 MB is not made. The pending images are kept, and Send is disabled with "This conversation has reached its image limit. Clear the assistant to send more images." Removing the images lets the text be sent.
    - Images count when their turn is sent, and a failed turn doesn't give them back, since the session may already hold them. `reset()` and `clear()` start the count again at zero.
    - Every resumed request therefore stays within the same budget as a bead chat request.
    - **Mac session copy (stated limitation).** Because the Mac assistant is a kept session, Claude Code writes its turns, including the image blocks, into that session's transcript in Claude Code's own session storage on the Mac. The images stay there until Claude Code's own cleanup removes them. `clear()` starts a new session but does not delete the old transcript. The design is kept this way on purpose: resume depends on the transcript, and deleting Claude Code's sessions is out of scope. nacred keeps no other copy. Bead chat turns are not kept and run with `--no-session-persistence`, so they leave no transcript.

## Failure and recovery

- A picked file is unsupported, unreadable or can't be brought under 3 MB: it is skipped with a snackbar, and other picked images are still attached.
- More than 5 images are picked: the picker's maximum prevents it, and the attach action is disabled at 5.
- Saving an attachment fails: nothing is sent, the draft is kept, and the reason is shown. Retrying is safe.
- The turn fails (network, API error, or nacred 4xx/5xx): the message follows today's failure behavior for text messages. A 400 or 413 about images shows nacred's reason. Saved attachment files stay with the recorded message until the thread is removed.
- History has grown past the budget: older images become text notes at their markers, so later turns still fit.
- The Mac assistant conversation has used its image budget: image sends are blocked with a reason, text continues, and clearing the assistant starts a new budget.
- The target computer doesn't support images, or stops supporting them while images are pending: attach and image sends are disabled with a reason, pending images are kept until removed, and text chat is unchanged.
- Interrupted deletion: unreferenced files are swept at the next load.
- nacred crashes mid-turn: the startup sweep removes the staged directory.
- A referenced file is missing: a placeholder thumbnail is shown, the history gets a text note, and the chat keeps working.

## Risks

- Earlier images are sent again on every bead chat turn, which costs tokens. The count and byte budgets and downscaling bound this.
- A long Mac assistant conversation can't take more images after 20 images or 15 MB until it is cleared. This is stated in the UI, and it is the price of not failing resumed turns.
- Images sent in the Mac single-board assistant stay in Claude Code's session transcript on the Mac after the turn and after the assistant is cleared, until Claude Code's own cleanup removes them. This is stated in the proposal, spec and `docs/API.md`, and a manual check confirms where they are. Removing them is out of scope for v1.
- The two bead chat turn endpoints accept larger bodies than before. They are authenticated with the bearer token like today, and the limit applies only to them.
- Image turns depend on `claude -p --input-format stream-json` working together with `--resume` and `--json-schema`. Go tests pin the argv and stdin, and the manual device check confirms the real CLI.
- Images that were picked but not sent are lost on process death. This is acceptable for v1, and it is stated so it is not mistaken for persistence.
- Unit tests check how requests are built, not what Claude replies. A manual device check per backend confirms that Claude can describe the image.
