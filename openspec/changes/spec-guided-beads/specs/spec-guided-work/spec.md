## ADDED Requirements

### Requirement: Reconcile intent with project specifications
The bridge SHALL compare a seed bead with project goals, accepted specifications and active changes before proposing work, and SHALL record a new/amend/repair/covered recommendation with existing source references and rationale. It SHALL refuse incomplete inventory rather than claim exhaustive reconciliation.

#### Scenario: Existing active change
- **WHEN** the requested behavior belongs to an active proposal
- **THEN** the recommendation cites it and proposes an amendment instead of silently creating a duplicate change

#### Scenario: Missing implementation
- **WHEN** accepted requirements describe the desired behavior but implementation lacks it
- **THEN** the recommendation records a repair against the existing requirement without weakening that requirement

#### Scenario: Unreadable inventory
- **WHEN** a required context file cannot be read or inventory exceeds its limit
- **THEN** the workflow reports the limitation and does not advance to approval

#### Scenario: Already covered
- **WHEN** independent review confirms that existing work already covers the request
- **THEN** the app shows a read-only covered disposition with citations and offers feedback, without generating another epic or starting implementation

### Requirement: Durable bounded author and reviewer loop
The bridge SHALL persist workflow phase, generated artifacts, findings, dispositions, costs and operation history. It SHALL use a fresh independent review context to assess project alignment, completeness, feasibility and verification. Unresolved blocking findings or malformed review output SHALL prevent promotion. Review calls SHALL be bounded by iteration and cost limits.

#### Scenario: Phone disconnects
- **WHEN** the phone disconnects during proposal review
- **THEN** the Mac continues and the phone can retrieve the latest persisted result

#### Scenario: Daemon restart
- **WHEN** the daemon restarts during an AI call
- **THEN** the workflow is reported as interrupted with a Resume action and does not claim review success

#### Scenario: Reviewer finds conflict with goals
- **WHEN** a reviewer identifies a blocking conflict with an explicit project goal
- **THEN** the author revises within the bounded loop or the app shows the unresolved finding for a decision

### Requirement: Confined validated proposal artifacts
For new, amend and repair routes, the workflow SHALL produce proposal, design, behavior specifications and task sections within the selected active change, validate them through OpenSpec, and reject unsafe paths, symlink escapes and concurrent edits. The covered route SHALL remain a reviewed read-only disposition. Accepted specifications SHALL remain unchanged by proposal development.

#### Scenario: Concurrent edit
- **WHEN** a CLI user edits an amendment target after the workflow read it
- **THEN** the workflow preserves that edit and reports a conflict rather than overwriting it

#### Scenario: Invalid proposal
- **WHEN** OpenSpec validation fails
- **THEN** the workflow records the error and cannot become awaiting approval

### Requirement: Revision bound owner approval
The workflow SHALL bind review and owner approval to exact artifact and project-context revisions. Promotion and all linked implementation entry points SHALL recheck the revision server-side. Changes that invalidate the contract SHALL require renewed review, and active implementation SHALL prevent silent proposal amendments.

#### Scenario: Stale phone
- **WHEN** approval names a revision older than current artifacts
- **THEN** approval is rejected and current state explains that review must be refreshed

#### Scenario: Generic execution bypass
- **WHEN** a client invokes a normal single-run or round endpoint for an unapproved linked bead
- **THEN** the bridge refuses execution with an actionable explanation

### Requirement: Recoverable epic and section mapping
After owner approval the workflow SHALL promote the originating bead to an epic or reuse an existing epic for the same change, preserving history. Each task section SHALL map to a persistent identity and one child bead by default, with its checklist, requirement references, acceptance and dependencies. Promotion SHALL be recoverable and idempotent, and SHALL preserve manual edits.

#### Scenario: Partial promotion failure
- **WHEN** a child is created but a subsequent operation fails or its response is lost
- **THEN** retry recovers the existing child and continues without duplicates

#### Scenario: Section rename
- **WHEN** a reviewed task section is renamed or reordered
- **THEN** its existing bead and decision history remain linked by stable identity

#### Scenario: Manual bead edit
- **WHEN** a linked generated body has been edited outside the workflow
- **THEN** synchronization reports the conflict and does not overwrite it silently

#### Scenario: Revision after promotion
- **WHEN** the owner approves revised sections after previous implementation has ended
- **THEN** stable sections retain bead identity, affected closed children reopen after synchronization, added sections receive new children, and retired sections remain visible with reasons without automatic closure

#### Scenario: Second seed for the same change
- **WHEN** a second bead is reconciled to a change already mapped to an epic
- **THEN** it resolves to the existing workflow and epic rather than creating a second one

### Requirement: Decision and implementation traceability
The workflow SHALL record reconciliation, review dispositions and promotion on the relevant beads, and implementation SHALL receive the approved requirements, current decisions and section scope. The app SHALL retain access to proposal artifacts, findings, mapped beads and history.

#### Scenario: Old decision is still relevant
- **WHEN** more comments arrive after approval
- **THEN** implementation still receives the approved decision contract even if its original comment is no longer recent

#### Scenario: Worktree starts from an older target
- **WHEN** the approved proposal is absent from the target branch used for a new round
- **THEN** the exact approved snapshot is installed and committed in the integration workspace before bead work starts, verified in bead workspaces, and included in the final build commit

#### Scenario: Lost workflow state
- **WHEN** a bead bears a workflow label but the bridge has no corresponding record
- **THEN** execution is refused with a recovery explanation

### Requirement: Evidence distinguishes progress from completion
The app SHALL distinguish mapped tasks, implementation outcomes, verified runs, human checks and landing. Checked task boxes or closed beads alone SHALL NOT establish requirement completion. Missing work or unresolved checks SHALL leave the change epic incomplete; automatic archive is outside this release.

#### Scenario: Checked task without evidence
- **WHEN** tasks.md marks a section complete but no verified implementation evidence exists
- **THEN** the app reports the checkbox observation without claiming the requirement is verified

#### Scenario: Round parks a required section
- **WHEN** a round finishes with a required child parked
- **THEN** the change remains incomplete and shows the unfinished section

#### Scenario: Old successful run
- **WHEN** a previous spec revision has a verified run for the same bead
- **THEN** that result remains historical and does not verify the current revision

### Requirement: Existing design and adaptive navigation
The app SHALL expose the workflow within bead/epic detail using its existing Material 3 theme and components. It SHALL show a clear phase and next action, readable artifacts and findings, preserved feedback, and an explicit handoff to existing round controls. Normal beads and older bridges SHALL remain usable.

#### Scenario: Folded screen and keyboard
- **WHEN** the owner reads a proposal and types revision feedback on the cover screen
- **THEN** content scrolls, the focused field and action remain reachable, and leaving/reopening preserves the feedback

#### Scenario: Older bridge
- **WHEN** the paired computer does not implement change endpoints
- **THEN** existing bead detail continues to work and the change card explains that a bridge update is needed

#### Scenario: Approval is not execution
- **WHEN** the owner approves a reviewed change
- **THEN** its epic and children are prepared, and implementation only starts through an explicit round action
