## ADDED Requirements

### Requirement: Attach images to chat messages
Per-bead chat (Ask and Refine) and the board assistant for one board SHALL offer an Attach image action beside the message field that opens the system photo picker limited to images, with multiple selection. Up to 5 JPEG, PNG, GIF or WebP images per message SHALL be shown as removable thumbnails and sent with the next message.

#### Scenario: Pick and send
- **WHEN** Ryder attaches two screenshots in a bead's Ask chat, types a question and sends
- **THEN** the message is sent with both images, and the sent message in the thread shows two thumbnails

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
- **WHEN** Ryder collapses a docked chat with pending images, or moves between the pane and the sheet, and returns
- **THEN** the same pending images are still attached and nothing was sent

### Requirement: Out-of-scope chats offer no attachments
The new-bead draft chat, the all-boards chat bar and the board assistant across boards SHALL NOT offer Attach image, on either backend.

#### Scenario: Out-of-scope chats
- **WHEN** Ryder uses the new-bead draft chat, the all-boards chat bar or the assistant across boards
- **THEN** no Attach image action is shown

### Requirement: Images are tied to their turn in one ordered list
Each bead chat request SHALL carry its images as one ordered list, numbered oldest turn first. Each image MUST be sent as an `Image N: <name>` text block and its image block, before the prompt text, and the prompt SHALL mark each image's turn with `[image N: <name>]`. A prompt with no images SHALL be today's prompt.

#### Scenario: Two turns with images
- **WHEN** an earlier turn had one screenshot and the current message has two photos
- **THEN** the request's images are numbered 1 to 3 in that order, the earlier turn's text holds `[image 1: …]`, and the current message's text holds `[image 2: …]` and `[image 3: …]`

#### Scenario: Text-only prompt unchanged
- **WHEN** no turn in the thread has images
- **THEN** the prompt text is identical to the one built before this change

### Requirement: Claude receives the images on the API backend
With the API backend, a bead chat user message with images MUST hold the labelled image blocks, each with its media type, followed by the text block. Board assistant user messages SHALL hold their own images before their text. Each image SHALL be downscaled to a long edge of at most 1568 px and at most 3 MB. A message without images SHALL be built exactly as today.

#### Scenario: Claude describes the image
- **WHEN** Ryder sends a screenshot with "what's wrong here?" using the API backend
- **THEN** the request's user message contains an image block and a text block, and Claude's reply can describe the screenshot

#### Scenario: Text-only unchanged
- **WHEN** Ryder sends a text-only message using the API backend
- **THEN** the request is identical to the one sent before this change

### Requirement: Claude receives the images on the Mac backend
With the Mac backend, the app SHALL send images in an optional ordered `images` array of `{media_type, name, data}` on `/v1/repos/{repo}/claude/json` and `/claude/stream`, to the host in the bead's or board's repository key. Attaching and sending images SHALL be enabled only while that computer's `features` contain `chat_images`. A request without `images` SHALL be handled as today.

#### Scenario: Claude on the Mac describes the image
- **WHEN** Ryder sends a photo of a sketch in Refine using the Mac backend
- **THEN** nacred receives the image over the existing connection, `claude -p` gets it as an image block, and the reply can describe the sketch

#### Scenario: Older bridge
- **WHEN** the computer that handles the chat does not list `chat_images` in its features
- **THEN** Attach image is disabled with a reason to update nacred on that computer, and text chat works as today

#### Scenario: Target loses support while images are pending
- **WHEN** images are pending and the target computer's refreshed features no longer list `chat_images`, or the backend is switched to Mac with such a computer
- **THEN** the pending images are kept, Send is disabled with the update reason, and removing the images lets the text be sent

#### Scenario: Text-only unchanged on the Mac
- **WHEN** Ryder sends a text-only message using the Mac backend
- **THEN** the request has no `images` field and nacred starts `claude -p` exactly as it does today

### Requirement: The Mac board assistant bounds images per conversation
On the Mac backend the board assistant SHALL send only the current turn's images, because the resumed session holds earlier ones. It MUST NOT send images that would take the conversation past 20 images or 15 MB decoded; such pending images SHALL be kept with Send disabled and a reason. Clearing the assistant SHALL reset the count.

#### Scenario: Earlier images are not resent
- **WHEN** Ryder sends a second message in a Mac assistant conversation whose first message had images
- **THEN** the second request's `images` holds only the second message's images

#### Scenario: Conversation budget reached
- **WHEN** the conversation has sent 18 images and Ryder attaches three more
- **THEN** Send is disabled with "This conversation has reached its image limit. Clear the assistant to send more images.", the images stay pending, and removing them lets the text be sent

#### Scenario: Cleared conversation
- **WHEN** Ryder clears the assistant after reaching the image limit
- **THEN** the next message can carry images again

### Requirement: nacred accepts image turns within bounded limits
nacred SHALL accept bodies up to 32 MiB on `/v1/repos/{repo}/claude/json` and `/v1/repos/{repo}/claude/stream` only, keeping 1 MiB on every other endpoint, and SHALL answer 413 with a reason above that. It MUST validate every image before writing any file or starting any process, and MUST answer 400 with a reason naming the image or limit on failure.

#### Scenario: Largest allowed request
- **WHEN** a turn request carries 20 valid images totalling 15 MB decoded
- **THEN** nacred accepts it and starts the turn

#### Scenario: Other endpoints unchanged
- **WHEN** a request over 1 MiB is sent to any other endpoint, including `/v1/claude/stream`
- **THEN** nacred rejects it as it does today

#### Scenario: Invalid image
- **WHEN** a request carries an image whose bytes don't match its media type, or that is over the size limit
- **THEN** nacred answers 400 naming the image, writes nothing and starts no process, and the app shows the reason

#### Scenario: What is checked
- **WHEN** nacred validates a request's images
- **THEN** it checks that each media type is JPEG, PNG, GIF or WebP, that the base64 decodes, that the bytes' signature matches the media type, that each image is at most 3 MB decoded, and that the request has at most 20 images and 15 MB of decoded image data

### Requirement: nacred stages images only for the turn
nacred MUST stage a turn's images in a private per-turn temp directory, pass them to `claude -p` as stream-json image content blocks with the prompt sent once on stdin, and delete the directory when the turn ends for any reason. It SHALL remove leftover turn directories at startup.

#### Scenario: Temp files removed
- **WHEN** a turn with images finishes, fails, is cancelled, times out or loses its client
- **THEN** its staged temp directory no longer exists

#### Scenario: Crash during a turn
- **WHEN** nacred restarts after crashing during a turn with images
- **THEN** it removes the leftover staged directory at startup

#### Scenario: Private staging
- **WHEN** nacred stages images for a turn
- **THEN** the directory has mode 0700, each file has mode 0600, and `claude -p` is started with `--input-format stream-json`

#### Scenario: Flags kept and prompt sent once
- **WHEN** an image turn is an Ask turn with `--json-schema`, or an assistant turn with `--resume`
- **THEN** the argv is today's with no positional prompt plus `--input-format stream-json`, and stdin holds one user message with the image blocks followed by the prompt text

### Requirement: Mac assistant images stay in Claude Code's session
On the Mac backend, Claude Code SHALL keep the single-board assistant's images in that conversation's saved session on the Mac until Claude Code's own cleanup removes them; clearing the assistant SHALL NOT delete them. nacred MUST NOT keep any other copy beyond the turn, and bead chat image turns MUST run with `--no-session-persistence`.

#### Scenario: Resumed assistant turn
- **WHEN** Ryder sends an image in the Mac single-board assistant and then a follow-up
- **THEN** the image is in that session's Claude Code transcript on the Mac, and nacred's staged temp directory for each turn no longer exists

#### Scenario: Assistant cleared
- **WHEN** Ryder clears the Mac assistant after sending images
- **THEN** the next message starts a new session, and the earlier session's transcript stays until Claude Code's own cleanup removes it

#### Scenario: Bead chat leaves no session
- **WHEN** Ryder sends an image in a bead chat using the Mac backend
- **THEN** `claude -p` runs with `--no-session-persistence` and no session transcript holds the image after the turn

### Requirement: Earlier images remain context within a request budget
Each request SHALL carry at most 20 images and 15 MB of decoded image data: the current message's images first, then earlier turns' images newest first while both limits allow. An earlier image that doesn't fit, or whose saved file is missing or unreadable, SHALL be replaced at its marker by a text note naming the file, and the turn SHALL still be sent.

#### Scenario: Follow-up about an earlier screenshot
- **WHEN** Ryder sends a screenshot and then asks a text-only follow-up about it
- **THEN** the follow-up request includes the earlier screenshot, and its marker is in the earlier turn's text

#### Scenario: Long thread with many images
- **WHEN** a thread's earlier turns hold 30 MB of images and Ryder sends another message with five images
- **THEN** the request carries the five new images and as many of the newest earlier images as fit in 15 MB and 20 images, the rest appear as image omitted notes, and the request is accepted

#### Scenario: Missing file
- **WHEN** a saved attachment file is missing when the thread is reopened
- **THEN** its thumbnail shows Image unavailable, and the history sent to Claude says the image is unavailable

### Requirement: Bead chat attachments live with their thread
Bead chat images SHALL be saved as separate files under `filesDir/chat-attachments/`, and `bead-chats.json` SHALL store only each image's id, MIME type and file name, never bytes or base64. When a thread is cleared or drops out under `MAX_THREADS`, its attachment files MUST be deleted. Unreferenced files SHALL be deleted when the store loads.

#### Scenario: Restart
- **WHEN** Ryder sends an image in a bead chat and restarts the app
- **THEN** the thread shows the image's thumbnail, the image file exists under `chat-attachments/`, and `bead-chats.json` contains its reference and no base64

#### Scenario: New conversation
- **WHEN** Ryder starts a new conversation on a bead whose thread has two images
- **THEN** both attachment files are deleted

#### Scenario: Thread ages out
- **WHEN** a 61st thread is used and the least recently used thread had images
- **THEN** that thread and its attachment files are removed

#### Scenario: Interrupted deletion
- **WHEN** the app stopped after saving a cleared thread but before deleting its files
- **THEN** the next load deletes the unreferenced files

#### Scenario: Saving fails
- **WHEN** writing an attachment file fails while sending
- **THEN** nothing is sent, files already written for that message are removed, the draft text and images are kept, and the reason is shown

#### Scenario: Older threads
- **WHEN** `bead-chats.json` was written before this change
- **THEN** its threads load as text-only and continue as they did before

### Requirement: Board assistant images stay in memory on the phone
On the phone, board assistant images SHALL be held only in memory with that conversation, SHALL NOT be written to the app's storage, and SHALL be dropped by `BoardAssistant.clear()`.

#### Scenario: Board assistant cleared on the phone
- **WHEN** Ryder clears the board assistant after sending images
- **THEN** the images are gone from the app's memory and nothing was written to the app's storage on the phone
