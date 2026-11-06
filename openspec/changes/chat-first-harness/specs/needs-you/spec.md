# Spec Delta

## MODIFIED Requirements

### Requirement: Inbox
The app SHALL show Needs you as one queue per project, built from nacred's attention list, and never as a
separate destination. The queue SHALL be ordered with questions, decisions and discovered work first, then
approvals, then landing and archive.

It SHALL appear in three places:
- in the workspace, through the status strip, which expands to the list
- as decision cards, inline in the thread that raised them
- on Projects, as each project's count and as the Needs you filter across all projects and computers

Each card SHALL be a view of its queue item. Answering it anywhere SHALL update it everywhere, and opening
another thread SHALL NOT move or copy it.

#### Scenario: Approve from the list
- **WHEN** Ryder taps the status strip's "2 need you" and then a proposal item
- **THEN** its reviewed-action card is shown at that revision, ready to answer

#### Scenario: A human check on a section
- **WHEN** a spec section bead carries the `human` label
- **THEN** it appears as a human-check card in the queue

#### Scenario: Boards first
- **WHEN** Ryder opens Projects while items wait
- **THEN** the projects are the first thing on the screen, each with its count

#### Scenario: Across projects
- **WHEN** Ryder picks the Needs you filter on Projects
- **THEN** only projects with waiting items show, each with its count, and offline computers say when they were last checked
