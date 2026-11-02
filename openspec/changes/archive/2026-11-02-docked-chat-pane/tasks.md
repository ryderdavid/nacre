# Tasks

Gate commands for every section: `cd android && ./gradlew testDebugUnitTest` and `cd android && ./gradlew assembleDebug`, plus `openspec validate docked-chat-pane --strict`. After each section lands, a chat can still be opened in the app.

<!-- nacre-section: sffdc64040f446d4ef5b00 -->
## 1. Shared wide-layout check
- [x] 1.1 Add `isPaneWide(maxWidth)` in `ui/common` with the existing 760.dp value, plus a unit test at 759/760.dp
- [x] 1.2 Replace the inline `maxWidth >= 760.dp` in `BoardScreen.kt` and `RunsScreens.kt` with it, with no behavior change
- [x] 1.3 Move `rememberPaneWidth` from `BoardScreen.kt` to `ui/common` unchanged, so the chat pane can share it
- [x] 1.4 Gates: `./gradlew testDebugUnitTest assembleDebug`

<!-- nacre-section: sf2f2a052b1099415250a8 -->
## 2. Per-screen chat presentation and view state
- [x] 2.1 Add `ChatPresentation(selected, collapsed)` to `chatStack`, keyed by the screen's context, with select, collapse, expand and close
- [x] 2.2 Add `ChatViewState(draft, firstVisibleIndex, firstVisibleOffset)` per chat in `chatStack`
- [x] 2.3 Clear the presentation and view state when a chat expires or is dismissed; Close clears only the presentation
- [x] 2.4 Unit tests: separate screens don't share presentation; collapse keeps selection and view state; draft and position survive collapse/expand and a pane-to-sheet move; expiry and dismiss clear selection and draft; return restores selection and collapsed state
- [x] 2.5 Gates: `./gradlew testDebugUnitTest assembleDebug`

<!-- nacre-section: sf579e700e81dfb92bdef5 -->
## 3. Shared chat content
- [x] 3.1 Extract `BeadChatContent` from `BeadChatSheet` in `BeadChat.kt`; `BeadChatSheet` keeps wrapping it with no behavior change
- [x] 3.2 Add `ChatContent(selected)` choosing `BeadChatContent` or `AssistantPane(inPane = true)`, both reading and writing `ChatViewState`
- [x] 3.3 Gates: `./gradlew testDebugUnitTest assembleDebug`

<!-- nacre-section: sfaf3f2a64ee27a6cac09b -->
## 4. Chat host and header entry point
- [x] 4.1 Add pure `chatLayout(isWide, available, paneWidth, minBody, presentation, beadPaneOpen)` returning None/Docked/EdgeTab/Sheet (no dialog value)
- [x] 4.2 Unit tests for `chatLayout`: wide, narrow, too narrow for 360.dp body, collapsed then narrow, bead pane open
- [x] 4.3 Add `ChatHost(screenContext)`: docked right pane using the shared pane width and existing pane style (surfaceContainerLow, no insets, no shadow), or `HandleDismissSheet` when narrow, both rendering `ChatContent`
- [x] 4.4 Add `ChatHeaderAction` (AI icon, active-count badge, dropdown reusing the New/active/dismiss rows) to the top app bar of Board, Bead detail and Repos screens
- [x] 4.5 In the same change, remove `ChatFab` from the FAB columns and delete `ChatFab` and the Dialog-based `ChatWindow`
- [x] 4.6 Bead and run links inside the chat navigate without clearing the screen's presentation
- [x] 4.7 Move the same chat between pane and sheet when the window crosses the threshold
- [x] 4.8 Gates: `./gradlew testDebugUnitTest assembleDebug`

<!-- nacre-section: sf5cc877f16c71c75c34e0 -->
## 5. Board pane sharing, collapse and Back
- [x] 5.1 Add Collapse to a 40.dp edge tab at the far right and Expand back; Close clears the screen's presentation but keeps the chat in the header list
- [x] 5.2 Board: one right-side pane slot; opening a chat replaces the bead pane and remembers the bead; closing or collapsing the chat restores it; tapping a bead card while docked collapses the chat to the edge tab
- [x] 5.3 Embedded `BeadDetail(inPane = true)`: its header chat action calls the board host instead of creating a nested host
- [x] 5.4 Back order: collapse expanded docked chat, then the existing bead-pane close, then navigation; on phone Back dismisses the sheet
- [x] 5.5 Unit tests for the board slot decision (bead open + chat opened, chat docked + bead tapped, chat closed restores bead) and the Back order as a pure function
- [x] 5.6 Gates: `./gradlew testDebugUnitTest assembleDebug`

<!-- nacre-section: sf59c0dc9a16da8ab5ee62 -->
## 6. Device check
- [ ] 6.1 Manually check on the Fold, folded and unfolded: header entry with no floating button, docked pane styling matches the bead pane, chat/bead swap, collapse/expand keeps draft and scroll, Back order, link out and back, navigate away and back, IME on the cover screen
- [ ] 6.2 Gates: `./gradlew testDebugUnitTest assembleDebug`
