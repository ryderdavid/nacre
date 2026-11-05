## MODIFIED Requirements

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

## REMOVED Requirements

### Requirement: A chat belongs to the screen it was opened on
**Reason**: Owner decision: there is one chat, and it follows Ryder between screens. Each message takes its context from the screen showing when it is sent.
**Migration**: Replaced by "The open chat follows Ryder between screens".

### Requirement: Returning to a screen shows its chat again
**Reason**: Screens no longer own chats. The one current chat is the same on every screen.
**Migration**: Replaced by "The open chat follows Ryder between screens". The draft, pending images and scroll position are kept per chat.

### Requirement: Expired or dismissed chats are not restored
**Reason**: Chats are now nacred records that don't expire, and dismissing the view no longer ends a chat.
**Migration**: Replaced by "Deleted chats are not restored".

## ADDED Requirements

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
