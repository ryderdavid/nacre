# Spec Delta

## ADDED Requirements

### Requirement: Projects home
The app SHALL open on Projects, which lists every project (repositories grouped by git remote), grouped
by computer, with each computer's connection state. Each project SHALL show its live status, its Needs-you
count and its active change. There SHALL be filters for All, Needs you and Working. Settings SHALL be in
the overflow.

#### Scenario: Offline computer
- **WHEN** a computer hasn't answered for 12 minutes
- **THEN** its projects show "Last checked 12 min ago" and "Status unknown", never a zero count

### Requirement: The workspace has three destinations
Opening a project SHALL open its workspace for that project checkout, with three destinations: Chat,
Spec and Beads. It SHALL open on Chat, in the project's most recent thread. Back from the workspace SHALL
return to Projects.

#### Scenario: Open a project
- **WHEN** Ryder taps Nacre on Projects
- **THEN** the workspace opens on Chat, in Nacre's most recent thread

### Requirement: Compact workspace layout
At compact widths (the cover screen, or split screen under 600dp), the workspace SHALL show one
destination at a time, with a short navigation bar for Chat, Spec and Beads that hides while the keyboard
is open.

#### Scenario: Typing on the cover screen
- **WHEN** Ryder focuses the composer on the cover screen
- **THEN** the navigation bar hides until the keyboard closes

### Requirement: Navigation follows Material 3 Expressive sizes
The short navigation bar SHALL be 64dp tall, with a 56dp by 32dp indicator. The navigation rail SHALL be
96dp wide, with each destination at least 64dp tall. The selected destination SHALL show a filled icon on
the indicator, and its label in the secondary color.

#### Scenario: Selected destination
- **WHEN** Beads is selected on the cover screen
- **THEN** the Beads icon is filled on its indicator, and its label is in the secondary color

### Requirement: Wide workspace layout
At wider widths, a navigation rail SHALL show a project switcher, and Chat, Spec and Beads below it. The
rail chooses Chat alone, or Chat beside Spec or Beads:
- Chat is the main pane, at least 360dp wide. The supporting pane is at least 320dp wide.
- Both panes show only when the usable width (the window minus the rail and the divider) is at least
  680dp. Otherwise the chosen destination shows alone.
- Focus gives Spec or Beads the whole content area, with chat collapsed to a one-line chat bar.

#### Scenario: Unfolded with Beads
- **WHEN** Ryder chooses Beads in the rail on the unfolded screen (about 728dp usable)
- **THEN** Chat and Beads show side by side

### Requirement: Pane divider presets
The divider between Chat and the supporting pane SHALL offer the presets Chat wide, Even and Pane wide,
hiding any preset that would break a pane's minimum, and Reset. Reset SHALL give Chat
clamp(W/2, 360dp, W − 320dp) of the usable width W, and the pane the rest. Dragging the divider SHALL
clamp to the minimums. Each preset SHALL also be an accessibility action on the divider.

#### Scenario: Presets at 728dp
- **WHEN** the usable width is 728dp
- **THEN** Chat wide sets the pane to 320dp, Even sets each pane to 364dp, and Pane wide sets Chat to 360dp

#### Scenario: Reset at 680dp
- **WHEN** the usable width is 680dp and Ryder taps Reset
- **THEN** Chat gets 360dp and the pane 320dp, and Even is not offered

### Requirement: Posture and state survive
- With a separating hinge in book posture, the panes SHALL split at the hinge.
- In tabletop posture, content SHALL sit above the fold, and the composer and status below it.
- Below 480dp of height, the status strip SHALL fold into the app bar, and the navigation bar SHALL become a rail.
- Folding, unfolding and resizing SHALL keep the thread, the draft, the selection and the scroll positions.

#### Scenario: Fold while typing
- **WHEN** Ryder folds the phone with a half-written message beside the Beads pane
- **THEN** the cover screen shows Chat with the message intact

### Requirement: Threads belong to a checkout and an agent
Each project checkout SHALL have any number of threads, each bound to that checkout and to one agent
(Claude or Codex). The workspace title SHALL name the project, with the computer and agent beneath it.
Reopening a thread SHALL restore its draft, its scroll position and its panel selection.

#### Scenario: Switch thread
- **WHEN** Ryder switches from "Resume after drops" to "Chat-first redesign" and back
- **THEN** each thread shows its own draft and scroll position as he left them

### Requirement: Thread switcher
Tapping the workspace title SHALL open the thread switcher. It lists:
- this checkout's threads, each with its title, agent, waiting state and last line
- the project's threads on other computers, under each computer's name

It also has a "New Claude chat" split button, whose menu offers a Codex chat or a chat on another computer.

#### Scenario: Codex chat
- **WHEN** Ryder opens the split button's menu and picks Codex chat
- **THEN** a new Codex thread starts in the same checkout

## REMOVED Requirements

### Requirement: Four top-level destinations
**Reason**: Projects and the Chat · Spec · Beads workspace replace the Boards, Inbox, Activity and Chat tabs.
**Migration**: Boards becomes Projects. Inbox becomes the Needs-you queue, shown in the workspace's status strip and as the Projects filter. Activity becomes the work card and the work inspector. Chat becomes the workspace's main destination.

### Requirement: The tab bar is visible on every screen
**Reason**: The workspace's navigation bar or rail replaces the tab bar, and detail screens are now sheets or panes within a destination.
**Migration**: The navigation bar keeps the same keyboard behavior. Sheets may cover it, as before.

### Requirement: The Chat tab talks about the screen Ryder came from
**Reason**: Chat is now always about the workspace's project. What is open in Spec or Beads becomes the visible Viewing context (see agent-chat).
**Migration**: Use the Viewing chip and reference chips instead of the "About" label and "New · <screen>".
