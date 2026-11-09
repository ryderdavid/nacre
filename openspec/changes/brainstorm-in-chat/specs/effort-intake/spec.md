## MODIFIED Requirements

### Requirement: New effort runs intake
In a repository with a valid OpenSpec setup, new work SHALL be drafted in a New effort chat. "Where does this
belong?" on the Draft card, or the chat agent acting on Ryder's go-ahead, SHALL raise an intake card in that
thread. Raising it SHALL run one read-only intake call, bounded in cost and time, that compares the draft with
the requirement index and the active changes. When the card is raised from the Draft card, intake SHALL read the
draft that card showed. Intake SHALL:
- return a verdict of just a bead, new change, amendment to a named active change, or already covered, with a
  rationale;
- NOT write files, create beads or start development.

#### Scenario: Drafted in Chat
- **WHEN** Ryder drafts the work in a New effort chat and taps "Where does this belong?"
- **THEN** an intake card with the verdict appears in that thread, and its draft is the one the Draft card showed

#### Scenario: Agent raises it on a go-ahead
- **WHEN** Ryder tells the chat "go ahead" after a draft
- **THEN** the agent raises the intake card with its newest draft, and the card appears in that thread

#### Scenario: Small fix
- **WHEN** Ryder describes a typo fix and raises intake
- **THEN** the verdict is just a bead, and choosing it creates the bead

#### Scenario: Already covered
- **WHEN** the verdict is already covered
- **THEN** the card offers to open the covering change or create the bead anyway, and offers no development

#### Scenario: Intake fails
- **WHEN** the intake call times out, reaches its cost cap or returns a malformed verdict
- **THEN** the chat shows the reason, and Ryder can still choose just a bead or new change

#### Scenario: Draft gone
- **WHEN** "Where does this belong?" names a draft the chat no longer holds
- **THEN** no card is raised, and the app says the draft is gone and to ask the chat for a new one

#### Scenario: Repository without OpenSpec
- **WHEN** the repository has no valid OpenSpec setup, or the computer doesn't support intake
- **THEN** New effort isn't offered and no intake call is made, and the chat creates new work as a bead only after Ryder says yes to its draft

#### Scenario: Older bridge
- **WHEN** the board's computer can't raise intake from a recorded draft
- **THEN** the Draft card shows no button, and the agent raises intake on Ryder's go-ahead

### Requirement: Ryder confirms or overrides the verdict
Intake's verdict SHALL be shown as a question card in the thread that started the effort. The card SHALL show the
verdict as "Suggested: <route>, because <reason>", with its citations, and say that nothing happens until Ryder
picks. Ryder SHALL be able to choose just a bead, a new change, or amending any active change instead.

Each option SHALL say in plain words what it does. A change option SHALL say that it files the work and then
writes and reviews a proposal for Ryder to approve. Spending SHALL appear only in the card's cost line, which
names intake's cost and the per-action limit. No option label or option detail SHALL show a dollar amount.

Confirming a change verdict SHALL create the bead and then request development with the verdict as feedback.
Confirming covered SHALL offer to open the covering change or to create a bead anyway.

A note Ryder writes on the card SHALL reach what he chooses. It SHALL be added to the description of a bead the
card creates, and to the feedback of a development the card requests.

When the draft carries a brainstorm digest, a bead the card creates SHALL hold the digest in its notes.
Development SHALL NOT start without this confirmation.

#### Scenario: Override to just a bead
- **WHEN** intake says new change and Ryder picks just a bead
- **THEN** only the bead is created and nothing is spent beyond the intake call

#### Scenario: Plain options
- **WHEN** an intake card is shown
- **THEN** no option label or detail contains a dollar amount, and the cost line names intake's cost and the per-action limit

#### Scenario: Note reaches the proposal
- **WHEN** Ryder writes "keep it to the folded screen" on the card and picks New change
- **THEN** the created bead's description and the development request's feedback both carry that note

#### Scenario: Digest reaches the bead
- **WHEN** the draft carried a digest of what was decided and ruled out, and Ryder picks New change
- **THEN** the created bead's notes hold that digest

#### Scenario: Development refused
- **WHEN** the bead is created but development is refused
- **THEN** the bead remains and the card states why development didn't start

#### Scenario: Amending a change with active work
- **WHEN** intake says amend a change whose round is active
- **THEN** the develop option is disabled with the round named, and the card offers to create the bead and develop later

#### Scenario: Reconciliation disagrees
- **WHEN** intake suggested an amendment but the author's full reconciliation chooses a new change
- **THEN** the author's route stands and the change history records both, with the author's reason
