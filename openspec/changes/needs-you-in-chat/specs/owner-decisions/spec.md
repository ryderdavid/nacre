# Spec Delta

## MODIFIED Requirements

### Requirement: Decision cards show exact options
The app SHALL show a pending decision as answer chips above the composer of the chat that is asking it,
and as a receipt in that chat's transcript, never as a card in a list or queue. The chips show its exact
options (recommended first), the focused option's consequences, and, where allowed, a note. Skip, Reject
and Cancel SHALL be separate actions. A decision's unsent input SHALL be kept until it is answered.

#### Scenario: Input kept
- **WHEN** Ryder types half an answer to a question, switches thread and comes back
- **THEN** the answer box still holds what he typed

### Requirement: Deferring and rejecting
- **Skip** SHALL keep the item pending, marked Skipped and outside the badge count, until its
  state changes or a day passes; the walk-through moves to the next item.
- **Reject** SHALL act by kind:
  - an approval sends the change back to its author with the note
  - a land or finish holds the round as "Held by you", listed with the note and the next actions
  - a human check sends it back

#### Scenario: Held round
- **WHEN** Ryder rejects landing a round with the note "wait for the device check"
- **THEN** the round is held with that note, and the walk-through offers Land and Cancel round when it reaches it again

#### Scenario: Skip
- **WHEN** Ryder taps Skip on an approval in the walk-through
- **THEN** the badge count drops by one and the agent moves to the next item

### Requirement: Expiry and supersession
A decision SHALL expire when its revision changes, or after 24 hours if the agent raised it. Its receipt
SHALL say why. When the walk-through reaches an expired decision, the agent SHALL offer to raise it again:
nacred rebuilds what it can (Ask again), and a chat's intake or found decision is raised again by the chat
that drafted it. A newer decision for the same target SHALL supersede the old one, whose receipt SHALL say
so. Work left unresolved SHALL stay in the walk-through with its reason and next action.

#### Scenario: Ask again
- **WHEN** an approval expired because revision 5 was written
- **THEN** the agent says so, and the chips for the new revision-5 decision appear when Ryder agrees

## REMOVED Requirements

### Requirement: Reviewed action cards
**Reason**: Replaced by the walk-through: the agent explains the project, revision, consequences, limits and evidence in words, and the chips carry nacred's exact options and consequences.
**Migration**: Covered by "The walk-through goes one item at a time" and "The item being asked is answered by tapping above the composer" in `needs-you-conversation`.

### Requirement: Question cards
**Reason**: Questions and intake verdicts are asked in the walk-through; In my own words is the composer's answer mode, and Ask about this is simply asking the agent.
**Migration**: Covered by `needs-you-conversation`.
