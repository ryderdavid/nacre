# chat-presentation Specification

## Purpose
Chat with Claude opens from one action in the top app bar and becomes part of the screen it was opened on: a docked pane beside the work on wide layouts, a sheet on phones, keeping its draft and place as it collapses, expands or moves between the two.

## Requirements

### Requirement: Chat presentation follows the existing wide-layout check
The app SHALL choose between the docked pane and the slide-up sheet with its existing side-pane width check (760.dp), shared by the Board, Runs and chat layouts, and SHALL NOT add a new breakpoint. On a wide layout an open chat SHALL show in a pane on the right that shares the width with the main content, which reflows beside it.

#### Scenario: Unfolded Fold starts a chat
- **WHEN** the window is at least 760.dp wide and Ryder starts a new chat on a board
- **THEN** the chat appears in a right-hand pane, the board is narrower but still fully visible and usable beside it, and no dialog is shown

#### Scenario: Too narrow for both
- **WHEN** the window passes the wide check but the pane width would leave the main content less than 360.dp
- **THEN** the chat opens as the slide-up sheet instead

### Requirement: Narrow layouts use the slide-up sheet, never a floating drawer
On a narrow layout an open chat SHALL show as a slide-up sheet. In no layout SHALL a chat open as a floating dialog or drawer over the content.

#### Scenario: Phone starts a chat
- **WHEN** the window is narrower than 760.dp and Ryder starts a new chat
- **THEN** the chat opens as a slide-up sheet

#### Scenario: Folding while a chat is open
- **WHEN** a chat is docked and the window becomes narrower than 760.dp
- **THEN** the same chat is shown as the slide-up sheet with its messages, draft and scroll position unchanged

### Requirement: The docked pane looks like part of the page
The docked pane MUST use the same width (the user's resizable detail pane width), surface colour and separation as the app's existing bead-detail and run-detail panes, with no drop shadow, elevation overlay or floating rounded card.

#### Scenario: Pane styling
- **WHEN** a chat is docked
- **THEN** the pane has the same width and surface as the board's bead-detail pane, is separated from the main content as that pane is, and has no shadow elevation

### Requirement: Chat entry point lives in the top header
Every screen that offers chat SHALL put its chat entry point in the top app bar in every layout, and SHALL NOT show the floating bottom-right chat button.
- The ✨ action SHALL open the current chat, or New when there is none.
- Its menu SHALL offer New · <label> for the current screen and the recent chats.
- The bead's chat entry SHALL open the same current chat.
- A chat SHALL remain openable from the header at every point the change lands.

#### Scenario: Header on phone and wide
- **WHEN** Ryder opens a board, a bead or the repositories screen in either layout
- **THEN** a chat action is in the top header, and no chat button floats in the bottom-right corner

#### Scenario: Picking from the header
- **WHEN** Ryder opens the header chat menu and chooses "New · <label>" or a recent chat
- **THEN** that chat becomes the current chat and opens in the pane or the sheet according to the layout

#### Scenario: Bead entry opens the same chat
- **WHEN** Ryder taps the chat entry on a bead after using ✨ on the board
- **THEN** the same chat opens, and its next message is about the bead

### Requirement: Chat and bead detail share the board's right pane
On a wide board, the chat and the bead-detail pane SHALL take turns in one right-side pane slot and SHALL NOT be stacked side by side. Swapping between them SHALL NOT end the chat or lose its view state. The chat entry in the bead detail shown inside the board's pane SHALL open the current chat in the board's pane, not in a nested pane.

#### Scenario: Chat opened while a bead is in the pane
- **WHEN** a bead is open in the board's right pane and Ryder opens the chat from the header
- **THEN** the chat takes the pane's place, and closing or collapsing the chat shows that bead in the pane again

#### Scenario: Bead opened while a chat is docked
- **WHEN** a chat is docked on the board and Ryder taps a bead card
- **THEN** the bead detail takes the pane, the chat becomes the collapsed edge tab, and tapping the tab brings the chat back with its draft unchanged

#### Scenario: Chat from the embedded bead detail
- **WHEN** Ryder uses the chat entry in the bead detail shown in the board's pane
- **THEN** the current chat opens in the board's right pane slot, no second chat pane appears inside the bead pane, and the next message carries that bead as the selection

### Requirement: The docked pane can be collapsed and expanded
On a wide layout the docked pane SHALL offer Collapse, which leaves a slim edge tab in the layout, and tapping the tab SHALL expand the pane again. Collapsing SHALL NOT end, reset or resend the chat, and SHALL keep its messages, draft text and scroll position.

#### Scenario: Collapse and expand
- **WHEN** Ryder collapses a docked chat that has a half-typed message and then taps the edge tab
- **THEN** the pane returns with the same messages, the same draft and the same scroll position, and nothing was sent

#### Scenario: Collapsed pane gives room back
- **WHEN** the pane is collapsed and no bead is in the board's pane
- **THEN** the main content takes all the width except the edge tab

### Requirement: Back closes the chat view before leaving
Back SHALL first collapse an expanded docked chat to its edge tab, then follow the screen's existing Back order. On a narrow layout Back SHALL dismiss the chat sheet. Dismissing the sheet SHALL close the view only, and the chat SHALL stay in the header list.

#### Scenario: Back on a wide board with a docked chat
- **WHEN** a chat is docked and expanded on the board and Ryder presses Back
- **THEN** the chat collapses to the edge tab, and the next Back follows the board's existing order (close the bead pane if open, then leave)

#### Scenario: Back on a phone sheet
- **WHEN** the chat sheet is open on a phone and Ryder presses Back
- **THEN** the sheet closes, the chat is still listed in the header, and reopening it shows the same messages and draft

### Requirement: The open chat follows Ryder between screens
The app SHALL keep one current chat for the whole app, with its draft, pending images and scroll position kept per chat.
- On a wide layout, a docked chat SHALL stay docked, or collapsed, as Ryder moves to another screen.
- On a narrow layout, following a link out of the sheet SHALL close the sheet, and opening the chat again SHALL show the same chat with its draft.

#### Scenario: Wide layout, bead to round
- **WHEN** a chat is docked beside a bead and Ryder opens a round
- **THEN** the same chat is docked beside the round with its messages and draft, and its composer reads About: <round>

#### Scenario: Link from a phone sheet
- **WHEN** Ryder taps a bead link inside the chat sheet on a phone
- **THEN** the sheet closes, the bead opens, and ✨ reopens the same chat with its draft

#### Scenario: Collapsed stays collapsed
- **WHEN** the docked chat is collapsed and Ryder moves to another screen
- **THEN** the new screen shows the collapsed edge tab

### Requirement: Deleted chats are not restored
When a chat is deleted:
- any pane or sheet showing it SHALL close
- its draft and pending images SHALL be discarded
- nothing SHALL be sent into it
- it SHALL leave the header menu and the Chat list

Closing a chat's view SHALL NOT delete it.

#### Scenario: Deleted from the Chat list
- **WHEN** Ryder deletes the current chat from the Chat list
- **THEN** no pane or sheet shows it, its draft is gone, and ✨ offers New

#### Scenario: View closed
- **WHEN** Ryder closes the chat sheet with Back
- **THEN** the chat is still in the header menu and the Chat list
