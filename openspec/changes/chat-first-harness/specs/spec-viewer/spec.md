# Spec Delta

## Purpose

The Spec panel is a read-only OpenSpec viewer for one project checkout: its changes by phase, a change's
overview with live progress, and a reader for its documents and requirement deltas.

## ADDED Requirements

### Requirement: Changes by phase
The Spec destination SHALL list the checkout's changes in these groups:
- Drafting
- Waiting for you
- In progress, with section progress from beads
- Ready to archive

It SHALL also list the project's current specs. It SHALL have search.

#### Scenario: A change waiting for approval
- **WHEN** a change's reviewed revision awaits approval
- **THEN** it is listed under Waiting for you, and opening it links to its decision card

### Requirement: Change overview
A change SHALL open to an overview showing:
- its phase and revision
- the stale reason when stale
- the independent review verdict and findings
- the approved plan's sections beside their live progress from beads, styled differently from each other
- links to evidence: beads, runs and rounds

#### Scenario: Failed section
- **WHEN** a section's bead failed its gates
- **THEN** the section shows Failed, and its evidence link opens that round's work inspector

### Requirement: Document reader
The reader SHALL show the proposal, design, tasks and spec changes:
- rendered, with selectable text
- with a document selector, an outline, find in document, and next and previous change
- full width on compact layouts

Requirement deltas SHALL carry the text labels Added, Modified and Removed. Ask about the selection SHALL
put the selection into the chat composer as a reference.

#### Scenario: Ask about a requirement
- **WHEN** Ryder selects a sentence in a requirement and taps Ask
- **THEN** Chat opens with that requirement attached as a reference chip and the composer focused

### Requirement: The Spec panel doesn't act
The Spec panel SHALL NOT approve, revise, run or archive. Those actions SHALL be asked for in chat, or
answered on a decision card.

#### Scenario: Approval
- **WHEN** Ryder opens a change waiting for approval
- **THEN** the panel shows Waiting for you with a link to the decision card, and has no Approve button
