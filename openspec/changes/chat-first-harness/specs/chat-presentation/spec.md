# Spec Delta

## REMOVED Requirements

### Requirement: Chat presentation follows the existing wide-layout check
**Reason**: Chat is the workspace's main destination, and its layout follows app-navigation's adaptive workspace rules.
**Migration**: See "Workspace layout adapts to width and posture" in app-navigation.

### Requirement: Narrow layouts use the slide-up sheet, never a floating drawer
**Reason**: On narrow layouts, Chat is a destination in the navigation bar, not a sheet over other screens.
**Migration**: Tap Chat in the navigation bar.

### Requirement: The docked pane looks like part of the page
**Reason**: The docked chat pane is removed. Spec and Beads sit beside Chat as a supporting pane instead.
**Migration**: See app-navigation.

### Requirement: Chat entry point lives in the top header
**Reason**: The ✨ header action is removed. Chat is always one tap away in the navigation bar or rail, and Ask about this opens it with a reference.
**Migration**: Use the Chat destination or Ask about this.

### Requirement: Chat and bead detail share the board's right pane
**Reason**: The bead detail is now a peek sheet over the Beads destination, and Chat is its own pane.
**Migration**: See beads-board, "Bead peek".

### Requirement: The docked pane can be collapsed and expanded
**Reason**: Focus Spec and Focus Beads replace collapsing the chat pane.
**Migration**: Use Focus. Chat collapses to a one-line chat bar.

### Requirement: Back closes the chat view before leaving
**Reason**: There is no chat view to close. Back follows the workspace's own order: focus, then document, then change list, then Projects.
**Migration**: See app-navigation.

### Requirement: The open chat follows Ryder between screens
**Reason**: Threads belong to a project checkout, and the workspace keeps its current thread across destinations.
**Migration**: See "Threads and the thread switcher" in app-navigation.
