## Why

Nacre chats only accept text. When Ryder wants Claude to look at a screenshot of a bug or a photo of a sketch, he has to describe it in words. Images are context for the conversation, just like the chat text: they inform what Claude answers in Ask or changes in Refine. They are not a lasting record on the bead.

## What Changes

- Per-bead chat (Ask and Refine, `ui/bead/BeadChat.kt`) and the board assistant for one board (`ui/assistant/`) gain an Attach image action next to the message field. It opens the system photo picker, which shows images only and supports picking several. Picked images appear as removable thumbnails above the field, and they are sent with the next message.
- A sent user message shows thumbnails of its images in the thread.
- A user turn can carry image attachments. `DraftTurn` gains an `images` list of references (id, MIME type, file name), and `TurnInput` carries the image bytes for the current turn.
- Bead chat history is one flattened prompt today, so each request carries one ordered list of images. Each image is labelled `Image N`, and the turn it belongs to carries a matching `[image N: name]` marker in its text.
- `ApiBackend` sends the labelled image blocks before the text block in the user message.
- `MacBackend` sends the images to nacred in an `images` array on the existing `/v1/repos/{repo}/claude/json` and `/v1/repos/{repo}/claude/stream` requests. Those two endpoints get their own 32 MiB body limit; every other endpoint keeps today's 1 MiB. nacred validates the images, stages them in a per-turn temp directory, passes them to `claude -p` as stream-json image content blocks, and deletes the directory when the turn ends.
- nacred advertises a `chat_images` entry in its existing `features` list. If the computer that handles the chat (the host of the bead's or board's repository) doesn't list it, Attach image is disabled on the Mac backend with a reason.
- Bead chat images are saved as files under `filesDir/chat-attachments/<id>.<ext>`. `bead-chats.json` stores only the id, MIME type and file name. The files are deleted when their thread is cleared ("new conversation") or drops out under `MAX_THREADS`. Files that no thread references are swept when the store is loaded.
- On the phone, board assistant images stay in memory with the rest of that conversation and are dropped by `BoardAssistant.clear()`. On the Mac backend the assistant resumes its Claude Code session, so it sends only the current turn's images and limits the images of the whole conversation to the request budget.
- Known limitation on the Mac: the single-board assistant runs as a saved Claude Code session so it can resume. Claude Code therefore keeps that conversation's images in its own session history on the Mac until Claude Code's own cleanup removes them. Clearing the assistant in Nacre does not delete them. nacred keeps no copy of its own beyond the turn, and bead chat turns run without session persistence, as they do today, so they leave no session behind.
- Limits:
  - JPEG, PNG, GIF and WebP only
  - at most 5 images per message
  - each image downscaled to a long edge of 1568 px and at most 3 MB
  - per request, counting earlier turns: at most 20 images and 15 MB of decoded image data, which keeps requests well under the Claude API's request size limit. Older images beyond that are replaced by a text note.
  - per Mac board assistant conversation: the same 20 images and 15 MB, after which image sends are blocked with a reason until the assistant is cleared
- Text-only messages send the same requests as today on both backends.

Out of scope:

- the new-bead draft chat, the all-boards chat bar (`AllBoardsRouter.kt`) and the board assistant across boards (`/v1/claude/stream`)
- non-image files such as PDFs or text files
- storing images on the bead or in bd, or nacred keeping its own copy of images beyond the duration of a turn
- deleting or redacting Claude Code's saved assistant sessions on the Mac
- a full-screen image viewer, editing, annotation, or a camera capture action
- attachments in run follow-ups, live session sends or the Runs screen
- syncing chat threads or attachments between devices
- raising the body limit of any nacred endpoint other than `/v1/repos/{repo}/claude/json` and `/v1/repos/{repo}/claude/stream`

## Capabilities

### New Capabilities

- `chat-attachments`: attaching images to bead chat and board assistant messages, delivering them to Claude on both backends within bounded request sizes, and storing them with the same lifetime as the thread they belong to.

### Modified Capabilities

None.

## Impact

- App, `data/claude/`:
  - `ClaudeService.kt`: `DraftTurn`, `TurnInput`, and image markers in `Prompts.ask`/`Prompts.refine`
  - `ClaudeBackend.kt`, `ApiBackend`, `MacBackend`, `MacSession` (per-conversation image budget)
  - `BeadChats.kt`: `StoredChats`, attachment files, deleting them with their thread
  - `BoardAssistant.kt`
  - `NacreApp.kt`: the attachment directory
  - `Models.kt`: `AssistRequest.images` and reading `chat_images` from features
- App UI: `ui/bead/BeadChat.kt` and `ui/assistant/`, for the picker, pending thumbnails and sent thumbnails.
- nacred, in `bridge/internal/server` and `bridge/internal/claude` (`Headless`):
  - a 32 MiB body limit for `/v1/repos/{repo}/claude/json` and `/v1/repos/{repo}/claude/stream` only
  - accept `images` on those requests and validate them
  - stage them in temp files and clean them up
  - pass them to `claude -p` as stream-json input, with the prompt only on stdin
  - add `chat_images` to `features`
- API contract: `docs/API.md` documents the `images` field, its order and markers, its limits, the 400 and 413 errors, the feature, and that a kept assistant session holds its images in Claude Code's session history. Both sides change together.
- Tests: Go tests for nacred, JVM unit tests for the app, and manual device checks on each backend, including where Mac assistant images live after a resumed turn.
- Older bridges: text chats work unchanged, and image attach is disabled with a reason.
