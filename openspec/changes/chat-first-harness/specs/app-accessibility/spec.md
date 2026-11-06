# Spec Delta

## Purpose

The accessibility contract every Nacre screen meets: operable without gestures or a touchscreen, readable
at large font sizes, perceivable without color, and verified with assistive technology before sign-off.

## ADDED Requirements

### Requirement: Every gesture has an alternative
Each gesture SHALL have a visible alternative and a TalkBack custom action:
- dragging a bead (Move to…)
- long-press menus
- resizing panes (the presets)
- swiping sheets

#### Scenario: Move with TalkBack
- **WHEN** a TalkBack user focuses a bead card
- **THEN** the actions include Move to… and Ask about this

### Requirement: Keyboard completes the core flows
With a hardware keyboard, Ryder SHALL be able to complete every core flow:
- open a project
- answer each decision card kind
- move a bead
- read a spec delta
- stop a round

Tab and the arrow keys move focus, Enter activates, Esc closes sheets, and Ctrl+Enter sends. Focus
SHALL be visible. Ctrl+F6 SHALL move between panes. When a resize or fold removes a pane, focus SHALL move
to the same item in the remaining pane.

#### Scenario: Fold closes with focus in the pane
- **WHEN** focus is on a bead in the Beads pane and the phone is folded
- **THEN** the cover screen shows Beads, with focus on that bead

### Requirement: Cards announce themselves
A decision card SHALL announce its kind, its target and its state. Focus SHALL stay stable when a card
collapses to a receipt. A sheet SHALL return focus to the control that opened it. Streaming replies SHALL
announce phase changes politely (started, needs you, done, failed) and never token by token.

#### Scenario: Card answered
- **WHEN** Ryder answers a card with TalkBack on
- **THEN** focus stays on the card's receipt, and "Done" is announced once the action finishes

### Requirement: Large text never hides a decision
At 200% font, buttons, chips and metadata SHALL wrap or stack, and no decision's title, consequence or
option SHALL be truncated.

#### Scenario: Card at 200%
- **WHEN** font scale is 200% and a reviewed-action card is shown on the cover screen
- **THEN** its options stack full width and every option's label is fully readable

### Requirement: Color, contrast and motion
- Statuses SHALL carry text or an icon, not color alone.
- Contrast SHALL be at least 4.5:1 for normal text, and 3:1 for large text, controls and state indicators, in light, dark and dynamic color.
- Interactive targets SHALL be at least 48dp, including split-button halves and chip remove buttons.
- With reduced motion, shape morphs and springs SHALL become crossfades.

#### Scenario: Remove a reference
- **WHEN** a reference chip is shown in the composer
- **THEN** its remove button has a 48dp target and the label "Remove reference"

### Requirement: Validated before sign-off
Before this change is signed off, the core flows SHALL be run with TalkBack, Switch Access and a
hardware keyboard on the Fold, and the results recorded on the change's human-check bead.

#### Scenario: Recorded runs
- **WHEN** the human-check bead is signed off
- **THEN** its comments record a TalkBack, a Switch Access and a keyboard run of each core flow
