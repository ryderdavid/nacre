# Spec Delta

## Purpose

The Beads panel is the project checkout's kanban: a strong visual board for seeing and moving work, with
everything beyond moving handed to chat.

## ADDED Requirements

### Requirement: Board by width
The board SHALL show its lanes (Proposals, Ready, In progress, To check, Blocked, Deferred, Done) as
columns at least 272dp wide when the panel is focused or wide enough. Otherwise it SHALL show one lane at
a time, with lane tabs carrying counts. Search and filters (an epic or change, Needs you, type) SHALL say
when they hide beads, and offer to clear them.

#### Scenario: Cover screen
- **WHEN** Ryder opens Beads on the cover screen
- **THEN** one lane shows at a time, with its count on its tab

#### Scenario: Focused on the unfolded screen
- **WHEN** Ryder focuses Beads on the unfolded screen
- **THEN** the lanes show as columns, and chat collapses to a one-line chat bar

### Requirement: Moving a bead
Long-pressing a card SHALL start a drag. On compact layouts a drop dock SHALL name the target lanes.
- Dropping on Ready, In progress, Blocked or Deferred SHALL set the status.
- Dropping on Done SHALL close the bead, with Undo. It SHALL NOT mean verified or signed off.
- Proposals and To check SHALL NOT be drop targets.
- A Move to… action in the card's menu, and a matching TalkBack custom action, SHALL do the same without dragging.

#### Scenario: Drop on Done
- **WHEN** Ryder drops NAC-12 on Done
- **THEN** NAC-12 is closed and a snackbar offers Undo

### Requirement: Bead peek
Tapping a card SHALL open a peek that can be expanded to full height. It SHALL show:
- the description, acceptance, dependencies and comments
- the latest run, with a link to its work inspector
- Ask about this, Start work and Open change

Start work SHALL default to stopping before landing. Editing fields SHALL be asked for in chat.

#### Scenario: Ask about a bead
- **WHEN** Ryder taps Ask about this on NAC-axe.3
- **THEN** Chat shows with NAC-axe.3 attached as a reference chip

### Requirement: Board follows chat
Changes the agent makes to beads SHALL appear on the board without a manual refresh.

#### Scenario: Agent closes a bead
- **WHEN** the agent closes NAC-7 while the board is shown beside the chat
- **THEN** NAC-7 moves to Done on the board
