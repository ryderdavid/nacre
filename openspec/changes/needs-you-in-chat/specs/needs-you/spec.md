# Spec Delta

## MODIFIED Requirements

### Requirement: Inbox
The app SHALL show Needs you as a conversation in Chat, never as a separate destination, list, sheet or
queue. Its only way in SHALL be the "N need you" badge in the top bar, a new chat's heads-up, and the
summary notification; each starts the walk-through, which takes items in nacred's queue order: questions,
decisions and discovered work first, then approvals, then landing and archive. Answering an item anywhere
SHALL update it everywhere.

#### Scenario: Approve from the list
- **WHEN** Ryder taps "2 need you" and the first item is a proposal awaiting approval
- **THEN** the agent explains it and its approval options appear above the composer

#### Scenario: A human check on a section
- **WHEN** a spec section bead carries the `human` label
- **THEN** it is an item in the walk-through, asked as a sign-off

#### Scenario: Boards first
- **WHEN** Ryder opens Projects while items wait
- **THEN** the projects are the first thing on the screen, each with its count, and the top bar shows the total badge

### Requirement: The assistant starts from Needs you
Chat SHALL see the same Needs you list through nacred's API or the `nacred` command, which its briefing
names. The briefing SHALL tell it to read that list first when Ryder asks what to work on or what needs
him, and to offer the walk-through when items wait.

#### Scenario: What should I work on next
- **WHEN** Ryder asks Chat what to work on next while a proposal awaits approval
- **THEN** the answer mentions the waiting proposal before suggesting new work and offers to go through what waits

#### Scenario: Mac assistant
- **WHEN** Chat runs on the Mac and answers what needs Ryder
- **THEN** it reads the attention list from nacred on that computer before answering
