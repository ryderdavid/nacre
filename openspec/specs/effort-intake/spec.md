# effort-intake Specification

## Purpose
New effort checks a new piece of work against the accepted specifications and active changes before anything is created, and suggests just a bead, a new change, an amendment of a named change, or already covered.

## Requirements

### Requirement: New effort runs intake
In a repository with a valid OpenSpec setup, the board's create action SHALL be "New effort", which drafts the effort in Chat. Continue on the resulting draft SHALL run one read-only intake call, bounded in cost and time, that compares the draft with the requirement index and the active changes. Intake SHALL:
- return a verdict of just a bead, new change, amendment to a named active change, or already covered, with a rationale
- NOT write files, create beads or start development

#### Scenario: Drafted in Chat
- **WHEN** Ryder taps New effort, drafts the work in the chat it opens, and taps Continue on the Draft card
- **THEN** the New effort sheet opens with that draft and the intake verdict, and no other draft chat is shown

#### Scenario: Small fix
- **WHEN** Ryder describes a typo fix and continues
- **THEN** the verdict is just a bead and confirming creates the bead exactly as New bead does today

#### Scenario: Repository without OpenSpec
- **WHEN** the repository has no valid OpenSpec setup, or the computer doesn't support intake
- **THEN** the create action opens the New bead form, without an AI draft step, and no intake call is made

#### Scenario: Already covered
- **WHEN** the verdict is already covered
- **THEN** the sheet offers to open the covering change or create the bead anyway, and offers no development

#### Scenario: Intake fails
- **WHEN** the intake call times out, reaches its cost cap or returns a malformed verdict
- **THEN** the sheet shows the reason and still offers just a bead and new change

#### Scenario: Older bridge
- **WHEN** the board's computer does not offer Chat
- **THEN** New effort opens its sheet without a draft step, and intake runs on what Ryder types

### Requirement: Intake verdicts cite their basis
Amendment and covered verdicts SHALL cite paths present in the requirement index, and an amendment SHALL name an active change. A verdict that doesn't SHALL fall back to new change, with the reason shown.

#### Scenario: Work an active change covers
- **WHEN** the draft changes behavior that an active change specifies
- **THEN** the verdict is an amendment naming that change, citing its path, with a one- or two-sentence rationale

#### Scenario: Uncited amendment
- **WHEN** intake returns an amendment without a citation from the index
- **THEN** the sheet shows new change and says the amendment couldn't be confirmed

### Requirement: Ryder confirms or overrides the verdict
The sheet SHALL show the verdict, rationale and citations, and SHALL let Ryder choose just a bead, new change, or amend any active change instead. Confirming a change verdict SHALL create the bead and then request development with the verdict as feedback; that control SHALL say it spends and show the per-action limit. Confirming covered SHALL offer to open the covering change or create a bead anyway. Development SHALL NOT start without this confirmation.

#### Scenario: Override to just a bead
- **WHEN** intake says new change and Ryder picks just a bead
- **THEN** only the bead is created and nothing is spent beyond the intake call

#### Scenario: Development refused
- **WHEN** the bead is created but development is refused
- **THEN** the bead remains and the sheet states why development didn't start

#### Scenario: Amending a change with active work
- **WHEN** intake says amend a change whose round is active
- **THEN** Create and develop is disabled with the round named, and the sheet offers to create the bead and develop later

#### Scenario: Reconciliation disagrees
- **WHEN** intake suggested an amendment but the author's full reconciliation chooses a new change
- **THEN** the author's route stands and the change history records both, with the author's reason

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

### Requirement: Amending seeds close after approval
When a revision that attached additional seed beads to a change is approved and synchronized, nacred SHALL close each of those seeds with a comment naming the change, the revision and the epic where the work continues.

#### Scenario: Seed folded into an existing change
- **WHEN** a bead routed as an amendment to a promoted change has its revision approved and synchronized
- **THEN** the bead is closed with a comment pointing to the epic, and later revisions don't bring it back into Proposals

### Requirement: Proposals are not moved by dragging
The Proposals lane SHALL NOT accept drops and its cards SHALL NOT start a drag. Close and Defer SHALL remain available from the card menu.

#### Scenario: Dragging a ready bead
- **WHEN** Ryder drags a Ready card across the board
- **THEN** Proposals is not offered as a drop target

### Requirement: Found something
Spec epics, their section beads, beads added under them, and spec rounds SHALL offer Found something, recording where the work was found, a note, and one of three answers: changes what done means, only how or in what order, or out of scope. Only how or in what order SHALL create an ordinary bead under the epic, optionally required before a chosen open section, leaving the plan unchanged. Out of scope SHALL open New effort prefilled and link the new bead as related to the epic.

#### Scenario: How, not what
- **WHEN** Ryder reports that a migration must run before section 3 and answers only how or in what order, choosing section 3
- **THEN** a bead appears under the epic that blocks section 3's bead, the approved revision stays current, and the bead can run as an ordinary run

#### Scenario: From a round
- **WHEN** Ryder opens Found something from a spec round's view
- **THEN** the sheet asks which round bead the work was found in, defaulting to the one whose run is open

#### Scenario: Out of scope
- **WHEN** Ryder answers out of scope
- **THEN** New effort opens prefilled and the resulting bead is related to the epic, not a child of it

### Requirement: The assistant can report found work
When Ryder tells Chat he found something under a spec epic, Chat SHALL ask which of the three answers applies, unless he said, and SHALL write nothing until it knows. It SHALL record the work through the same nacred calls the Found something sheet uses. The three answers are:
- changes what done means
- only how or in what order
- out of scope

For out of scope, Chat SHALL create the new bead related to the epic, not as a child of it.

#### Scenario: Assistant
- **WHEN** Ryder tells Chat he found something in a section without saying which answer applies
- **THEN** Chat asks which of the three answers applies, and nothing is written until he answers

#### Scenario: Chat records found work
- **WHEN** Ryder tells Chat that a migration must run before section 3 and that this only changes the order
- **THEN** a found bead appears under the epic that blocks section 3's bead, and the approved revision stays current

#### Scenario: Out of scope from Chat
- **WHEN** Ryder tells Chat the found work is out of scope
- **THEN** the new bead is related to the epic, not a child of it

### Requirement: Discovered work that changes done waits for a decision to revise
Answering changes what done means SHALL create a found bead under the epic and a Needs you item, and SHALL NOT revise the change by itself. For a promoted change the item SHALL state how many landed sections a revision would reopen and offer Revise now or a follow-on change. Revise now SHALL be unavailable, with the reason, while linked work is active.

#### Scenario: Pending amendment can't be run
- **WHEN** Ryder tries to run a found bead that changes what done means, alone or in a round
- **THEN** it is refused with the reason that it is a pending amendment to revise or make a follow-on

#### Scenario: Cost stated
- **WHEN** a found item that changes done is shown for a change with four landed sections
- **THEN** it says revising reopens four landed sections that will need new rounds, and offers a follow-on change instead

#### Scenario: During a round
- **WHEN** the change's round is active
- **THEN** Revise now is unavailable with the round named, and becomes available once the round has stopped

### Requirement: A revision from found work closes its found beads
A revision started with Revise now SHALL carry the notes of the found beads it was given, and SHALL refuse with a reason when those notes exceed the feedback limit. While it is pending, those beads SHALL NOT raise Needs you items. When it is approved and synchronized, those found beads SHALL be closed with a comment linking the revision.

#### Scenario: Amendment approved
- **WHEN** the revision started from found beads is approved and synchronized
- **THEN** those found beads are closed with a comment linking the revision, and reopened sections show that they need a new round

#### Scenario: Revision pending
- **WHEN** Revise now has started a revision that is not yet approved
- **THEN** Needs you no longer offers Revise now for those found beads, and they show as in revision

#### Scenario: Too many notes
- **WHEN** the selected found notes together exceed the feedback limit
- **THEN** nothing is revised and the reason asks for fewer notes

#### Scenario: Follow-on change
- **WHEN** Ryder chooses make it a follow-on change
- **THEN** New effort opens prefilled, and the found bead is closed with a link to the new bead

### Requirement: Found beads are traceable ordinary work
Every bead created as a child of a spec epic, by any path, SHALL NOT inherit the epic's workflow labels and SHALL carry a `found:<change-id>` label, so run and round guards treat it as ordinary work. When the work was found in a bead, the found bead SHALL record that bead as a discovered-from dependency. Preflight spec drift SHALL skip found beads.

#### Scenario: Guard check
- **WHEN** a found bead under a spec epic is started as a single run
- **THEN** the run starts, without a specification-round refusal

#### Scenario: New bead under a spec epic
- **WHEN** Ryder creates an ordinary bead with a spec epic as its parent
- **THEN** it has no workflow labels, carries `found:<change-id>`, has no discovered-from dependency, and appears under Added during work

### Requirement: Agents report discovered work instead of doing it
Spec round prompts SHALL tell agents to stay inside their section and to report work found outside it as a bd comment on their bead beginning `Found:`, without doing that work or creating beads. After a spec round bead reaches any end state, including parked or failed, nacred SHALL raise one Needs you item per `Found:` comment offering the three answers and Dismiss.

#### Scenario: Agent finds a missing endpoint
- **WHEN** an agent working a section comments `Found: the export endpoint has no pagination`
- **THEN** after its bead finishes, Needs you shows that note with the three answers and Dismiss

#### Scenario: Answer from Needs you
- **WHEN** Ryder taps Answer on a found item
- **THEN** the Found something sheet opens with the note, the bead it was found in and the three answers

#### Scenario: Out of scope cancelled
- **WHEN** Ryder answers a found comment out of scope and then cancels New effort
- **THEN** no bead is created and the found item is still in Needs you

#### Scenario: Parked bead
- **WHEN** a spec round bead with a `Found:` comment is parked
- **THEN** the comment still raises a Needs you item

### Requirement: Answered found comments stay answered
Answering or dismissing a found item SHALL post a reply comment carrying a marker for that comment, and a comment with a marker SHALL NOT raise an item again.

#### Scenario: Dismissed
- **WHEN** Ryder dismisses a found item
- **THEN** a reply carrying the comment's marker is posted and the item does not return, even after nacred restarts

### Requirement: The plan is frozen at approval
After approval, `tasks.md` SHALL be treated as the approved plan: neither nacred nor the app SHALL write progress into it. A hand edit to `tasks.md` after approval SHALL stale the approval with a reason saying that progress lives in beads and the plan changes through a revision.

#### Scenario: Someone ticks a box by hand
- **WHEN** `tasks.md` is edited after approval
- **THEN** the change becomes stale with the reason that progress is tracked in beads and the plan changes through a revision

### Requirement: Progress is shown from beads
Change detail and the epic SHALL show each approved section in plan order with its bead's status and evidence, SHALL list beads added under the epic separately, and SHALL show `tasks.md` checkboxes only as the plan. A change SHALL NOT be reported ready to archive while any found bead under its epic is open.

#### Scenario: Section landed but plan unticked
- **WHEN** a section bead is verified and landed while its `tasks.md` boxes are unchecked
- **THEN** change detail shows the section as landed, and `tasks.md` is unchanged

#### Scenario: Open found work
- **WHEN** every section has landed but a found bead under the epic is still open
- **THEN** the change is not reported ready to archive

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
