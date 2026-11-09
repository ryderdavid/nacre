# Spec Delta

## ADDED Requirements

### Requirement: One badge is the way in
The app SHALL show what waits on Ryder as one "N need you" badge in the top bar, counting that project in
a workspace and every project on Projects, hidden at zero and not counting skipped items. Tapping it
SHALL open the project's Needs you chat, created on first use and then reused, and send `/needs-you`
unless a walk-through is already running there. No other surface SHALL open a list, sheet or walkthrough
of decisions.

#### Scenario: Badge on Projects
- **WHEN** Ryder taps "7 need you" on Projects while Nacre has 4 of them
- **THEN** Nacre's Needs you chat opens and the walk-through starts

#### Scenario: Walk-through already running
- **WHEN** Ryder taps the badge while the Needs you chat is mid-walk-through
- **THEN** that chat opens and nothing new is sent

#### Scenario: Badge in a workspace
- **WHEN** three decisions wait in Nacre and Ryder taps "3 need you" in its top bar
- **THEN** Chat opens on Nacre's Needs you chat and his message "Go through what needs me" is sent

#### Scenario: Nothing waits
- **WHEN** no decision is pending in the project
- **THEN** no badge is shown

#### Scenario: Counts elsewhere are not doors
- **WHEN** Ryder taps a project row on Projects that shows "3"
- **THEN** the project opens as it always does, and no walk-through starts

### Requirement: A new chat gives the heads-up
When nacred creates a chat in a repository with pending decisions, it SHALL add a heads-up to that
chat's transcript, without a model turn, naming the total and the counts by kind. The app SHALL show it
with two reply chips: Yes, one by one, which sends `/needs-you`, and Not now, which collapses it to a
line that can still start the walk-through. A chat SHALL get at most one heads-up.

#### Scenario: Fresh chat
- **WHEN** Ryder starts a new chat in a project where 3 approvals and 2 questions wait
- **THEN** the chat opens with "5 things need you here: 3 approvals, 2 questions. Go through them one by one?" and the chips Yes, one by one and Not now, before any model runs

#### Scenario: Typing past it
- **WHEN** Ryder ignores the heads-up and asks about something else
- **THEN** the chat answers his message and the heads-up stays as it was

### Requirement: `/needs-you` starts the walk-through
nacred SHALL expand a chat message `/needs-you`, optionally followed by a decision id, into its own
walk-through prompt for the agent, while the transcript keeps Ryder's short message. Walk-through turns
SHALL run as light turns. The agent SHALL NOT answer a decision or call a protected route.

#### Scenario: Short message, full prompt
- **WHEN** Ryder taps Yes, one by one
- **THEN** the transcript shows "Go through what needs me" and the agent receives nacred's walk-through prompt

#### Scenario: Starting at one item
- **WHEN** Ryder taps a notification for decision D
- **THEN** the walk-through starts with D

### Requirement: The walk-through goes one item at a time
The agent SHALL open with one line giving the count by kind, then take the items in nacred's queue order.
For each item it SHALL say in a few short lines what it is, why it waits and what each option does,
quoting nacred's consequences, and end that message by naming the item in a `nacre-ask` block. After
Ryder answers, it SHALL report nacred's result in one line and move on, and say in one line when none
are left.

#### Scenario: Nine items
- **WHEN** Ryder sends `/needs-you` while nine decisions wait
- **THEN** the agent says how many wait and of what kinds, then explains the first and asks about it

#### Scenario: A question mid-item
- **WHEN** Ryder asks "what does landing push?" while an approval is being asked
- **THEN** the agent answers, ending with the same approval's `nacre-ask` block

### Requirement: nacred records which item is being asked
nacred SHALL accept a `nacre-ask` block only if it names a pending decision of the chat's project, and
SHALL then record it as the chat's asking decision; otherwise it SHALL drop the block with a reason. If
a walk-through turn ends without a valid block while decisions are pending, the app SHALL offer the next
one in queue order, marked "Next".

#### Scenario: A forged ask
- **WHEN** an agent's message names a decision from another project in a `nacre-ask` block
- **THEN** nacred drops the block and the slot does not show that decision

#### Scenario: Forgotten block
- **WHEN** a walk-through reply ends with no `nacre-ask` block while two decisions wait
- **THEN** the slot above the composer shows "Next:" and the first of them

### Requirement: The asked item is answered by tapping above the composer
While a chat's asking decision is pending, the app SHALL show above the composer its title, one chip per
option with nacred's exact labels (recommended first), the consequence nacred states for the option in
focus, and Skip, plus Reject and In my own words where allowed. Labels and consequences SHALL come only
from nacred's decision record. Tapping an option SHALL answer the decision from the phone, then send
"Answered: <label>" carrying the decision id.

#### Scenario: One tap
- **WHEN** an approval is asked and Ryder taps "Approve, run, I'll land"
- **THEN** the phone answers the decision, "Answered: Approve, run, I'll land" is sent, and the agent reports the result and moves on

### Requirement: Typed text is never an answer on its own
Options that need input, and Reject, SHALL turn the composer into that decision's answer box, whose Send
answers the decision. Any other typed text SHALL be a message to the agent and never an answer.

#### Scenario: Typed yes
- **WHEN** an approval is asked and Ryder types "yes, approve it"
- **THEN** nothing is approved, and the agent points him at the chip

#### Scenario: Own words
- **WHEN** a round question is asked and Ryder taps In my own words, types "keep it per chat" and taps Send
- **THEN** that text is recorded as the answer and no chat message is sent in its place

### Requirement: Receipts in the transcript
A decision raised in a chat, or answered in one, SHALL appear in that chat's transcript as a one-line
receipt at the point it happened, showing its state as nacred reports it, and updating in place. A receipt
SHALL expand to the action, the revision and nacred's message. A decision a chat raises outside a
walk-through SHALL become that chat's asking decision.

#### Scenario: Landing fails
- **WHEN** Ryder approves landing r-41 and the push fails
- **THEN** the receipt reads "Failed: land r-41" with nacred's reason, and the agent offers what to do next

#### Scenario: Raised mid-conversation
- **WHEN** the agent raises a land decision while Ryder chats about a round
- **THEN** the transcript shows "Waiting for you: land r-41" and its chips appear above the composer

### Requirement: One notification for what waits
The app SHALL post one summary notification per computer, "N things need you", that lists projects
when expanded, updates as counts change, and clears at zero. Tapping it SHALL do what the badge on
Projects does. A decision raised while the app is in the background SHALL post under that summary, and
tapping it SHALL start the walk-through at that decision in the chat that raised it.

#### Scenario: Three raised in a row
- **WHEN** three decisions are raised while the phone is locked
- **THEN** the phone shows one summary reading "3 things need you", with the three under it
