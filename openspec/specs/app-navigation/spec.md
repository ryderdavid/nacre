# app-navigation Specification

## Purpose
Whenever something doesn't fit Nacre's screens, Ryder has to leave the app for a freeform Claude Code or Codex session on the Mac. The bead chat and the ✨ board assistant are locked-down, read-only helpers (`--safe-mode`, `--permission-prompts none`, read-only `bd`). On NAC-48p.5 the bead chat could not build and install an APK for a sign-off. Ryder needs one fully capable agentic chat on the ho…

## Requirements

### Requirement: Four top-level destinations
The phone tab bar SHALL show Boards, Inbox, Activity and Chat as four equally spaced items, and the tablet navigation rail SHALL include Chat. Chat SHALL keep its state when switching tabs, as the other tabs do, and SHALL carry a badge counting chats waiting for permission.

#### Scenario: Phone
- **WHEN** the app opens on a phone-width window
- **THEN** the bar shows Boards, Inbox, Activity and Chat with equal widths

#### Scenario: Tablet
- **WHEN** the app opens on an unfolded Fold
- **THEN** the rail shows Boards, Inbox, Activity and Chat

#### Scenario: Waiting for permission
- **WHEN** one chat has a pending permission request
- **THEN** the Chat item shows a badge of 1

### Requirement: The tab bar is visible on every screen
The tab bar SHALL stay visible on bead, change, run and round screens. On phone layouts it SHALL hide only while the keyboard is open, and bottom sheets MAY cover it. The chat dock, the change screen's actions, the New FAB and snackbars MUST sit above the bar, so nothing at the bottom of a screen is covered. Back SHALL behave as before.

#### Scenario: Bead screen
- **WHEN** Ryder opens a bead on a phone
- **THEN** the tab bar is visible and Chat is one tap away

#### Scenario: Typing
- **WHEN** Ryder focuses a text field on a phone
- **THEN** the bar hides while the keyboard is open and returns when it closes

#### Scenario: Nothing covered
- **WHEN** a change awaiting approval is shown on a phone
- **THEN** Approve and run and Revise are fully above the bar, and a snackbar shown there also sits above the bar

#### Scenario: Back unchanged
- **WHEN** Ryder opens a bead from a board and presses Back
- **THEN** the board is shown, as before this change

### Requirement: The Chat tab talks about the screen Ryder came from
Tapping Chat SHALL show the current chat full screen, or the chat list when there is none. Messages sent from the Chat tab SHALL carry the context of the screen last shown before switching to Chat, and the list's first row SHALL read New · <label> for that screen.

#### Scenario: Chat from a bead
- **WHEN** Ryder taps Chat while bead NAC-48p.5 is open and the current chat exists
- **THEN** the chat is shown full screen with About: NAC-48p.5 on the composer, and its list starts with New · NAC-48p.5

#### Scenario: No chat yet
- **WHEN** Ryder taps Chat from Boards and has no chats
- **THEN** the list shows New · all boards and no past chats
