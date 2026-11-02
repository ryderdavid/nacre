## MODIFIED Requirements

### Requirement: Proposals lane
Wherever New effort is offered, the Workflow board SHALL show a Proposals lane before Ready holding open seed beads of changes not yet promoted or being revised, with those waiting on Ryder first. A change with no created proposal yet SHALL show as one placeholder instead of its seeds' cards. A change's epic SHALL NOT be in it. Membership SHALL be derived from change records, not bd status, and approval and synchronization SHALL move seeds to their ordinary lanes.

#### Scenario: Proposal awaiting approval
- **WHEN** a seed's change reaches awaiting approval
- **THEN** its card is at the top of Proposals with the phase chip

#### Scenario: Promoted
- **WHEN** an approved change is promoted
- **THEN** its epic leaves Proposals and appears in its ordinary lane

#### Scenario: Amending a promoted change
- **WHEN** a new seed amends a change that already has an epic
- **THEN** the seed is in Proposals while the revision is planned, and the epic stays in its lane showing the revision's phase

#### Scenario: Empty lane
- **WHEN** no change is being planned
- **THEN** the lane shows a hint to start a New effort, and the board summary has no Proposals tile

#### Scenario: Many changes being planned
- **WHEN** more changes are being planned than the changes summary returns
- **THEN** the lane header says how many of the total it shows

#### Scenario: Covered card
- **WHEN** a seed's change is judged already covered
- **THEN** its card names the covering change and offers Close directly

#### Scenario: Round candidates
- **WHEN** Ryder starts a round
- **THEN** beads in Proposals, including seeds shown as draft placeholders, are not offered as candidates

#### Scenario: No seed appears twice
- **WHEN** a change with no created proposal yet has one or more seeds
- **THEN** the lane shows exactly one placeholder for that change and no card for any of its seeds

#### Scenario: Revision of an existing change
- **WHEN** a change whose proposal was already created returns to drafting for a revision
- **THEN** its seed shows its ordinary card with the phase chip, not a placeholder

#### Scenario: Folded board
- **WHEN** the board opens on the cover screen
- **THEN** the pager still opens on Ready, with Proposals as the first tab

#### Scenario: Older bridge
- **WHEN** the computer doesn't provide change records
- **THEN** the board has no Proposals lane and looks as it does today

## ADDED Requirements

### Requirement: nacred reports whether a proposal was created
nacred SHALL record `created_at` on a change record the first time its authored artifacts pass strict OpenSpec validation or a covered disposition is recorded, and SHALL never clear it. For a change in a planning phase without `created_at`, the changes summary SHALL report a derived `draft`: `pending` while drafting, otherwise `failed` with nacred's message, unless dismissed.

#### Scenario: Development accepted
- **WHEN** nacred accepts development for a new seed
- **THEN** the response carries the change ID, the phase event for `drafting` carries it too, and the summary reports that change's draft as pending

#### Scenario: Proposal created
- **WHEN** the author's first artifacts for that change pass strict validation
- **THEN** `created_at` is set, the summary no longer reports a draft, and the change continues through its existing phases

#### Scenario: Any stop before creation is failed
- **WHEN** drafting stops before creation through an author error, the cost cap, a restart, an interruption or shutdown
- **THEN** the draft is reported as failed with nacred's message, and the change keeps its existing phase and Resume

#### Scenario: Older bridge
- **WHEN** the computer's changes summary has no draft field
- **THEN** the app shows no placeholders and the Proposals lane looks as it does today

### Requirement: Records from older nacred count as created
When nacred loads a change record saved by an older version, which has no record format version and no `created_at`, it MUST set `created_at` to the time of the record's first history entry, or to the current time if there is none, and save the record with the current format version. A backfilled record SHALL NOT be reported as a draft.

#### Scenario: Existing change being revised at upgrade
- **WHEN** nacred starts with an older record whose change is in `drafting` or `needs_attention` for a revision
- **THEN** the record gains `created_at` from its first history entry, the summary reports no draft, and its seed shows its ordinary card

#### Scenario: No history
- **WHEN** an older record has no history entries
- **THEN** `created_at` is set to the time of loading

#### Scenario: New record is not backfilled
- **WHEN** a record created by this version has no `created_at` yet
- **THEN** loading leaves it without `created_at` and its draft is still reported

#### Scenario: Loaded twice
- **WHEN** nacred restarts again after a backfill
- **THEN** the stored `created_at` is unchanged

### Requirement: Pending draft placeholder
A pending draft's placeholder SHALL show a visible pending status, "Drafting…" with a progress indicator, that sets it apart from existing proposals. It SHALL NOT show a title, change name, agent or computer, and SHALL NOT start a drag or open a detail view.

#### Scenario: Pending
- **WHEN** a change's draft is pending
- **THEN** the Proposals lane shows one placeholder for it reading Drafting…, with no proposal details

#### Scenario: Tapping or dragging
- **WHEN** Ryder taps or long-presses a pending placeholder
- **THEN** nothing opens and no drag starts

#### Scenario: Event for an unknown change
- **WHEN** the app receives a phase event for a change not in its summary
- **THEN** it refreshes the changes summary instead of guessing a slot

### Requirement: Created proposal replaces its placeholder
nacred SHALL report `started_at`, the time the change record was started, which never changes. Within each rank group the lane SHALL sort by `started_at`, newest first, instead of last update. When the proposal is created, the real card SHALL take the placeholder's position and no placeholder for that change SHALL remain. A draft that becomes failed SHALL move into the waiting group.

#### Scenario: Created in place
- **WHEN** a phase event or refresh shows the change now has `created_at` and a later update time, while other proposals are in the lane
- **THEN** the seed's proposal card is in the position the placeholder held, the other items haven't moved, and exactly one card for that change is shown

#### Scenario: Failed draft moves to waiting
- **WHEN** a pending draft becomes failed
- **THEN** its placeholder moves into the waiting group at the top of the lane, ordered there by `started_at`

#### Scenario: Older bridge order
- **WHEN** the changes summary has no `started_at`
- **THEN** the lane sorts by last update as it does today

### Requirement: Failed drafts stay visible until dismissed
When drafting stops before the proposal is created, for any reason, the placeholder SHALL switch to a visible error state reading Drafting failed with nacred's message, the change's Resume when offered, and Dismiss, instead of disappearing. nacred SHALL keep it until dismissed. Dismiss SHALL be idempotent, and Resume or a new develop SHALL return it to pending.

#### Scenario: Drafting stops
- **WHEN** a draft fails, is interrupted or is stopped by shutdown
- **THEN** its placeholder shows Drafting failed in the error style with the message, Resume and Dismiss, and is still shown after a refresh, a restart or on another paired client

#### Scenario: Resume from the placeholder
- **WHEN** Ryder taps Resume on a failed placeholder and nacred accepts it
- **THEN** the change returns to drafting and its pending placeholder is back at its `started_at` position

#### Scenario: Dismiss
- **WHEN** Ryder taps Dismiss on a failed placeholder
- **THEN** nacred records the dismissal, the placeholder disappears, and the seed shows its ordinary card with the existing Needs attention banner and Resume

#### Scenario: New develop after dismissal
- **WHEN** a new develop request is accepted for a change whose failed draft was dismissed
- **THEN** the dismissal is cleared and the pending placeholder is shown

#### Scenario: Dismiss fails
- **WHEN** the dismiss request fails
- **THEN** the placeholder stays in its error state and the app shows the reason, and dismissing again is safe
