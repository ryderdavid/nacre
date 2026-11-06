# Spec Delta

## MODIFIED Requirements

### Requirement: Ryder confirms or overrides the verdict
Intake's verdict SHALL be shown as a question card in the thread that started the effort, with its
rationale and citations. Ryder SHALL be able to choose just a bead, a new change, or amending any active
change instead. Confirming a change verdict SHALL create the bead and then request development with the
verdict as feedback; that option SHALL say it spends, and show the per-action limit. Confirming covered
SHALL offer to open the covering change or to create a bead anyway. Development SHALL NOT start without
this confirmation.

#### Scenario: Override to just a bead
- **WHEN** intake says new change and Ryder picks just a bead
- **THEN** only the bead is created and nothing is spent beyond the intake call

#### Scenario: Development refused
- **WHEN** the bead is created but development is refused
- **THEN** the bead remains and the card states why development didn't start

#### Scenario: Amending a change with active work
- **WHEN** intake says amend a change whose round is active
- **THEN** the develop option is disabled with the round named, and the card offers to create the bead and develop later

#### Scenario: Reconciliation disagrees
- **WHEN** intake suggested an amendment but the author's full reconciliation chooses a new change
- **THEN** the author's route stands and the change history records both, with the author's reason

### Requirement: Found something
Telling the chat about found work in a spec epic, its section beads, beads added under them, or a spec
round SHALL raise a found-work card. The card records where the work was found, a note, and one of three
answers:
- **Changes what done means**
- **Only how or in what order**: SHALL create an ordinary bead under the epic, optionally required before a
  chosen open section, leaving the plan unchanged.
- **Out of scope**: SHALL start a new effort in chat, prefilled, and link the new bead as related to the epic.

#### Scenario: How, not what
- **WHEN** Ryder says a migration must run before section 3 and answers only how or in what order, choosing section 3
- **THEN** a bead appears under the epic that blocks section 3's bead, the approved revision stays current, and the bead can run as an ordinary run

#### Scenario: From a round
- **WHEN** Ryder reports found work while a spec round's work inspector is open
- **THEN** the card asks which round bead the work was found in, defaulting to the one shown

#### Scenario: Out of scope
- **WHEN** Ryder answers out of scope
- **THEN** a new effort starts in chat, prefilled, and the resulting bead is related to the epic, not a child of it
