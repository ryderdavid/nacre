## ADDED Requirements

### Requirement: Review the beads before any code
Before any bead in a round is implemented, nacred SHALL run a read-only review over all of the round's
beads (full text, comments, dependencies) and the repository, and SHALL produce a plan containing, for
each bead, the areas and files it expects to touch and any dependency it implies, and a list of
findings. The review SHALL NOT modify the repository or the beads beyond posting the plan.

#### Scenario: Plan produced
- **WHEN** a round with three beads enters `planning`
- **THEN** nacred stores a plan listing each bead's expected areas and the findings, and posts each bead's part of the plan as a comment on it

#### Scenario: Read-only
- **WHEN** the review runs
- **THEN** it runs in plan mode with only read access to the repository and bd, and the working tree is unchanged afterwards

### Requirement: Classify findings
Each finding SHALL be one of `contradiction`, `ambiguity`, `overlap`, `ordering`, `shared-artifact` or
`unmerged-work`, and
SHALL name the beads involved and explain the finding in a sentence.

#### Scenario: Contradiction
- **WHEN** one bead asks for the chat bar to always show and another asks for it to hide on scroll
- **THEN** the plan contains a `contradiction` finding naming both beads

#### Scenario: Shared artifact
- **WHEN** two beads would each bump the app's versionName
- **THEN** the plan contains a `shared-artifact` finding for the version, and neither bead's run is asked to change it

### Requirement: Look at unmerged work outside the round
The review SHALL be given every local and remote branch that is not merged into the round's target, with
the files each one changes, and SHALL report an `unmerged-work` finding for each such branch that touches
the files or areas a round bead expects to touch. Branches created by rounds that are still active SHALL be
excluded.

#### Scenario: Stale base caught
- **WHEN** a round targeting `main` includes a bead that edits `NewBeadSheet.kt`, and branch `integration-0.3` is not merged into `main` and also changes `NewBeadSheet.kt`
- **THEN** the plan contains an `unmerged-work` finding naming `integration-0.3`, the bead, and the shared file

#### Scenario: Unrelated branch ignored
- **WHEN** an unmerged branch only changes files no round bead expects to touch
- **THEN** the plan has no finding for it

### Requirement: Contradictions, ambiguities and unmerged work become questions
Every `contradiction`, `ambiguity` and `unmerged-work` finding SHALL become a question for Ryder with
suggested answers (for `unmerged-work`: merge the branch into the round's base first, ignore it, or stop),
and the round SHALL be `needs_decision` until every question has an answer. The other finding types
SHALL NOT ask him anything.

#### Scenario: Waiting for an answer
- **WHEN** the plan contains one contradiction and two overlaps
- **THEN** Ryder is asked one question, and implementation waits for its answer

#### Scenario: Merge the unmerged branch first
- **WHEN** Ryder answers an `unmerged-work` question with "merge it first"
- **THEN** nacred merges that branch into the integration branch before any bead starts, runs the gates, and records the new base

#### Scenario: No questions
- **WHEN** the plan contains only overlaps and orderings
- **THEN** the round goes straight to `implementing`

### Requirement: Answers reach the work
Each answer SHALL be posted as a comment on every bead the question named, and SHALL be included in the
prompt of every run and integration turn for those beads.

#### Scenario: Answer in the prompt
- **WHEN** Ryder answers "the bar hides on scroll only when folded"
- **THEN** both beads get that decision as a comment, and both beads' runs receive it in their prompts

### Requirement: Overlaps and orderings become sequence
An `overlap` finding SHALL make the later bead start only after the earlier one is integrated, so it
builds on the earlier bead's result. An `ordering` finding SHALL do the same and SHALL be shown in the plan
as a proposed bd dependency, added to bd only if Ryder accepts it. Ryder SHALL be able to reorder or
unlink beads in the plan before implementation starts.

#### Scenario: Overlapping beads are stacked
- **WHEN** NAC-1 and NAC-2 both change `BoardAssistant.kt` with compatible intent
- **THEN** NAC-2's workspace is created from the integration branch after NAC-1 has been integrated

#### Scenario: Ryder unlinks an overlap
- **WHEN** Ryder marks an overlap as independent before implementation starts
- **THEN** the two beads run in parallel and any conflict is left to integration
