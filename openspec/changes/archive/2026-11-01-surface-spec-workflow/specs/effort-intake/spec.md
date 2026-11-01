## ADDED Requirements

### Requirement: New effort runs intake
In a repository with a valid OpenSpec setup, the board's create action SHALL be "New effort". After the existing draft step, continuing SHALL run one read-only intake call, bounded in cost and time, that compares the draft with the requirement index and the active changes. It SHALL return a verdict of just a bead, new change, amendment to a named active change, or already covered, with a rationale. Intake SHALL NOT write files, create beads or start development.

#### Scenario: Small fix
- **WHEN** Ryder describes a typo fix and continues
- **THEN** the verdict is just a bead and confirming creates the bead exactly as New bead does today

#### Scenario: Repository without OpenSpec
- **WHEN** the repository has no valid OpenSpec setup, or the computer doesn't support intake
- **THEN** the sheet is today's New bead and no intake call is made

#### Scenario: Already covered
- **WHEN** the verdict is already covered
- **THEN** the sheet offers to open the covering change or create the bead anyway, and offers no development

#### Scenario: Intake fails
- **WHEN** the intake call times out, reaches its cost cap or returns a malformed verdict
- **THEN** the sheet shows the reason and still offers just a bead and new change

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
Wherever New effort is offered, the Workflow board SHALL show a Proposals lane before Ready holding open seed beads of changes not yet promoted or being revised, with those waiting on Ryder first. A change's epic SHALL NOT be in it. Membership SHALL be derived from change records, not bd status, and approval and synchronization SHALL move seeds to their ordinary lanes.

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
- **THEN** beads in Proposals are not offered as candidates

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
The board assistant SHALL offer a found_something write tool with the same three answers, applied only after Ryder approves it.

#### Scenario: Assistant
- **WHEN** Ryder tells the assistant he found something in a section
- **THEN** it proposes found_something with one of the three answers, and nothing is written until he approves

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
