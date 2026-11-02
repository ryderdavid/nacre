# Docked chat pane on wide layouts

## Why
Tapping the floating chat button in the bottom-right corner opens the assistant in a floating drawer, and the drawer covers the work on screen. On a tablet, a desktop or an unfolded Fold, the chat should sit beside the work as part of the page, so the work stays visible. The entry point also belongs back in the top header, where it used to be.

## What Changes
- Wide layouts (the app's existing side-pane check, `maxWidth >= 760.dp`): starting or opening a chat shows it in a docked pane on the right. The main content reflows to make room. The pane uses the same width (`detailPaneDp`, which the user can resize) and the same styling as the existing bead-detail and run-detail panes, with no shadow and no floating card.
- On the board, the chat and the bead-detail pane take turns in one right-side pane slot and are never stacked. Swapping between them never loses the chat.
- Narrow (phone) layouts: the chat keeps opening as a slide-up sheet.
- The chat entry point (new chat and the list of active chats) moves into the top app bar on every screen that offers chat, in every layout. The floating bottom-right `ChatFab` button and the Dialog-based `ChatWindow` are removed in the same step that adds their replacement.
- On wide layouts the docked pane can be collapsed to a slim edge tab and expanded again without losing the conversation, the draft or the scroll position.
- A chat belongs to the screen it was opened on. Moving to another screen, including through a link inside the chat, doesn't bring the pane along. Coming back shows the same chat again.
- Back collapses an expanded docked chat first, then follows the board's existing Back order.

Out of scope:
- Changes to assistant behavior, tools, backends, prompts or cost limits.
- Any nacred or `docs/API.md` change.
- New breakpoints, or changing the 600.dp board-column threshold or the default pane width.
- Showing the chat pane and the bead-detail pane side by side.
- Keeping chats across app process death, beyond what the chat stack already does.
- Multi-computer coordination (NAC-h1m). This change doesn't depend on it.

## Capabilities

### New Capabilities
- `chat-presentation`: how and where the assistant chat is shown, its header entry point, how it shares the right-side pane with bead detail, collapsing the pane, Back, and tying each chat to a screen.

### Modified Capabilities
(none)

## Impact
- Android app only. `ui/assistant/ChatFab.kt` is replaced by a header action and a pane/sheet host. Also touched: `ui/assistant/AssistantPane.kt` and `ui/bead/BeadChat.kt` (shared content composables), `ui/board/BoardScreen.kt` (shared pane slot, Back order, `rememberPaneWidth` moved to `ui/common`), `ui/bead/BeadDetail.kt` (header action, embedded-pane behavior), `ui/repos/ReposScreen.kt`, `ui/runs/RunsScreens.kt` (shared wide check), and possibly `ui/common/HandleDismissSheet.kt`.
- The chat stack (`container.chatStack`) gains per-screen presentation state (selected chat, collapsed) and a per-chat view state (draft text, list position). Conversations are stored as they are today.
- JVM unit tests for the presentation state, the view-state holder, the layout decision and the board slot and Back decisions.
