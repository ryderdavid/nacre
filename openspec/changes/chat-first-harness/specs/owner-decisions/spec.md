# Spec Delta

## Purpose

Owner decisions let the agent in a chat prepare any nacred action while only Ryder, from a paired
device, authorizes the protected ones, each bound to one exact action and revision.

## ADDED Requirements

### Requirement: Protected actions need the owner
nacred SHALL treat these actions as protected on every route that performs them, legacy routes included:
- approving a change
- landing a round
- finishing an epic
- signing off or sending back a human check
- raising a limit or budget
- answering a round stop with a choice that spends more or lands
- adopting a change as written

A protected action SHALL run only for a paired device's token, or through an approved decision.

#### Scenario: Agent tries to land
- **WHEN** a chat turn calls the land route with its chat-scoped token
- **THEN** nacred refuses it with 403 `needs_decision` and says how to raise a decision

#### Scenario: Older app
- **WHEN** an app from before this change lands a round with its device token
- **THEN** the round lands as before

### Requirement: Other actions stay open to chat
Actions that are not protected SHALL stay open to chat turns. These include:
- creating, editing and closing beads
- starting a proposal
- starting a round or run that doesn't land
- commenting, retrying, stopping and cancelling

#### Scenario: Cancelling stays open
- **WHEN** a chat turn cancels a round with its chat-scoped token
- **THEN** the round is cancelled

### Requirement: Chat turns use a chat-scoped token
nacred SHALL give each chat turn a token that it accepts for unprotected routes only, and SHALL NOT give
chat turns a paired device's token. The app and its documentation SHALL NOT claim more than this: the
agent runs as Ryder on the Mac, and its direct git and filesystem actions remain governed by Claude
Code's or Codex's own permission system and the repository's branch protections.

#### Scenario: Token in the turn
- **WHEN** a chat turn starts
- **THEN** its environment holds a chat-scoped token and no device token

### Requirement: A decision binds one action and revision
nacred, not the agent, SHALL build each decision from:
- its kind
- the target ids
- the exact action (method, path and body)
- the revision it applies to
- the consequences, which nacred computes
- an expiry

Approving a decision SHALL authorize that action once. nacred SHALL check that the revision is unchanged
and the action still applies before executing it. Execution SHALL be idempotent by decision id.

#### Scenario: Revision changed
- **WHEN** Ryder approves a decision for change revision 4 after revision 5 was written
- **THEN** nacred does not execute it and marks it expired, naming the new revision

#### Scenario: Answered twice
- **WHEN** the same approval reaches nacred twice
- **THEN** the action runs once and both answers get its result

### Requirement: Only a paired device answers a decision
nacred SHALL accept a decision's answer only from a paired device that is not the computer running the
agent, as it does for permission requests.

#### Scenario: Answer from the Mac
- **WHEN** an answer to a decision arrives from the Mac's own address
- **THEN** nacred refuses it with 403 "Answer from a paired device"

### Requirement: Decision cards show exact options
The app SHALL show each decision as a card, both in the chat where it was raised and in the project's
queue. A card shows its exact options (recommended first), each option's consequences, and an optional
note. Defer, Reject and Cancel SHALL be separate actions. A card SHALL keep its unsent input until it is
answered.

#### Scenario: Input kept
- **WHEN** Ryder types half an answer on a question card, switches thread and comes back
- **THEN** the card still holds what he typed

### Requirement: Reviewed action cards
A card for approving, approving and running, landing, finishing or adopting SHALL show:
- the project, computer and revision
- nacred's consequences, including whether it lands to main and pushes
- the limits
- links to the evidence (review findings, changes, gates)

Alternatives SHALL be separate options. Rejecting an approval SHALL require a note.

#### Scenario: Approve and run without landing
- **WHEN** a change awaits approval and its round could land to main
- **THEN** the card offers "Approve, run, I'll land" (recommended, stops before main), "Approve, run and land to main" and "Approve only" as separate options, each stating its consequences

### Requirement: Question cards
A card for a round question, spec drift or an intake verdict SHALL show the choices with their
consequences, an "In my own words" answer, and Ask about this, which explains and never answers. An intake
verdict SHALL offer Amends <change>, New change, Just a bead and Already covered, and Ryder may pick any of
them.

#### Scenario: Own words
- **WHEN** Ryder picks In my own words and sends "keep it per chat"
- **THEN** nacred records that text as the answer to the question

### Requirement: Human-check cards
A human-check card SHALL show the exact check, the build and how to try it, with Sign off, and Not right
with a required note.

#### Scenario: Not right without a note
- **WHEN** Ryder taps Not right and leaves the note empty
- **THEN** Send back stays disabled until he writes one

### Requirement: Recovery cards
A recovery card SHALL show the server's choices with their costs. Raise the limit SHALL expand an amount
field that must be more than what's spent and at most the runaway ceiling.

#### Scenario: A budget amount
- **WHEN** Ryder picks Raise the limit and enters an amount below what's spent
- **THEN** Choose stays disabled and the field says why

### Requirement: Found-work cards
A found-work card SHALL show the source bead or comment and offer:
- Changes how or order, with an optional "Required before which section?"
- Changes what done means
- Out of scope

#### Scenario: Before a section
- **WHEN** Ryder answers Changes how or order and picks section 3
- **THEN** the new bead blocks section 3's bead

### Requirement: Decision states are shown truthfully
A decision SHALL show one of these states:
- pending, sending, executing
- done, failed or outcome unknown
- declined, expired or superseded

Approving a decision SHALL NOT be shown as success. A done decision SHALL collapse to a receipt that can
be expanded.

#### Scenario: Execution fails
- **WHEN** Ryder approves a land and the push fails
- **THEN** the card shows Failed with nacred's reason, and the round stays ready to land

### Requirement: Deferring and rejecting
- **Defer** SHALL keep the item in the queue, marked Deferred and outside the badge count, until its
  state changes or a day passes.
- **Reject** SHALL act by kind:
  - an approval sends the change back to its author with the note
  - a land or finish holds the round as "Held by you", listed with the note and the next actions
  - a human check sends it back

#### Scenario: Held round
- **WHEN** Ryder rejects landing a round with the note "wait for the device check"
- **THEN** the round is listed under Held with that note and offers Land and Cancel round

### Requirement: Expiry and supersession
A decision SHALL expire when its revision changes, or after 24 hours if the agent raised it. The card
SHALL say why, and offer a way to raise it again (see the scenarios). A newer
decision for the same target SHALL supersede the old one, and the old card SHALL link to it. Work left
unresolved SHALL stay listed with its reason and next action.

#### Scenario: Ask again
- **WHEN** an approval card expired because revision 5 was written
- **THEN** Ask again shows a new card for revision 5

#### Scenario: Ask the chat again
- **WHEN** a chat's intake or found card expired unanswered, which nacred can't rebuild without the chat
- **THEN** the card offers Ask the chat again, which opens that chat with the card attached and a
  message asking it to raise the card again that says intake runs again and what it cost; nothing is
  sent or spent until Ryder sends it
