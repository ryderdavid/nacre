## MODIFIED Requirements

### Requirement: Durable bounded author and reviewer loop
The bridge SHALL persist workflow phase, generated artifacts, findings, dispositions, costs and operation history. It SHALL use a fresh independent review context to assess project alignment, completeness, feasibility and verification. Unresolved blocking findings or malformed review output SHALL prevent promotion. Review calls SHALL be bounded by iteration and cost limits; the cost limit SHALL be a guard against a runaway, set well above what ordinary drafting spends. The reviewer SHALL mark each finding as one the author can fix or one only the owner can settle, giving concrete choices for the latter. The author SHALL keep revising while unresolved findings it can fix remain, within the iteration limit, and the workflow SHALL stop for the owner early only for findings that need the owner. The app SHALL NOT present text the owner did not choose or write as the owner's feedback, and SHALL NOT show money on its action buttons.

#### Scenario: Phone disconnects
- **WHEN** the phone disconnects during proposal review
- **THEN** the Mac continues and the phone can retrieve the latest persisted result

#### Scenario: Daemon restart
- **WHEN** the daemon restarts during an AI call
- **THEN** the workflow is reported as interrupted with a Resume action and does not claim review success

#### Scenario: Reviewer finds conflict with goals
- **WHEN** a reviewer identifies a blocking conflict with an explicit project goal
- **THEN** the author revises within the bounded loop or the app shows the unresolved finding for a decision

#### Scenario: The author fixes its own findings
- **WHEN** a review leaves a medium finding the author can fix and passes remain
- **THEN** the author revises again before the owner sees the change, without asking him

#### Scenario: A decision only the owner can make
- **WHEN** a review leaves a finding that needs the owner
- **THEN** drafting stops and the app offers its choices, with an empty box for his own words
