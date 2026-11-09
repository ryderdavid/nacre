# Spec Delta

## MODIFIED Requirements

### Requirement: Steps the agent can't take alone are asked on the phone
When Claude Code asks the host for permission in a full turn, or Codex sends an approval request, nacred
SHALL record a permission request naming the step and its reason, and the app SHALL show it above the
composer, ahead of any decision, as the step with Allow and Deny…, and as a receipt in the transcript.
Allow MUST let the step run, and Deny MUST refuse it. Neither answer SHALL leave the turn hanging.

#### Scenario: Allow
- **WHEN** the classifier won't allow a command on its own and Ryder taps Allow above the composer
- **THEN** the command runs, the turn continues, and the receipt reads Allowed

#### Scenario: Deny with a note
- **WHEN** Ryder taps Deny… and sends "use the debug build" from the answer box
- **THEN** the step is refused with that note, the agent continues its turn, and the receipt reads Denied

#### Scenario: Answered twice
- **WHEN** a request is answered from a second device after it was decided
- **THEN** nacred answers 409, and both devices show the recorded outcome

#### Scenario: Stop
- **WHEN** Ryder taps Stop while a request is pending
- **THEN** the request is denied, the turn ends, and the chat is idle

### Requirement: One status strip in the workspace
Every workspace destination SHALL show one project-wide status strip with the project's running work
(for example, "Round running · 3 of 7 landed"), which opens the work inspector. The Needs-you count SHALL
NOT be on the strip; it is the badge in the top bar.

Suggestions SHALL show only while the composer is empty and no decision or permission is being asked.
Picking one SHALL fill the composer and never send on its own.

#### Scenario: Decisions while work runs
- **WHEN** a round runs while two decisions wait
- **THEN** the strip shows the round, and the top bar shows "2 need you"

#### Scenario: A suggestion
- **WHEN** Ryder picks the suggestion "What's blocked?"
- **THEN** the composer holds "What's blocked?" and nothing is sent until he taps Send
