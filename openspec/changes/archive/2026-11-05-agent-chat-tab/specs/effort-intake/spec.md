## MODIFIED Requirements

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
