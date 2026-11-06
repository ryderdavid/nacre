# Spec Delta

## MODIFIED Requirements

### Requirement: Chat can do the owner's work end to end
From the phone alone, a Claude chat SHALL be able to:
- create, edit and close beads
- start a proposal
- start a round
- build an APK and send it to the phone by Taildrop

It SHALL do these with the repository's own commands and nacred's command and API. For protected actions
(see owner-decisions) it SHALL raise a decision instead of acting, and report the decision's result once
Ryder answers it.

#### Scenario: Send me this build
- **WHEN** Ryder, viewing a bead, asks the chat "build and send me this APK"
- **THEN** the agent builds with the repository's build command, asks for any step the classifier won't allow, and runs `tailscale file cp` to the phone, and the APK arrives on the phone

#### Scenario: Land it
- **WHEN** Ryder asks the chat to land a ready round
- **THEN** the agent raises a land decision, and the round lands only after Ryder approves the card from the phone

### Requirement: Every message carries the screen it was sent from
The app SHALL attach to each message what the workspace shows when Send is pressed:
- the project checkout
- the Spec or Beads item open beside or before Chat (a change, document, requirement, bead or round)
- its selection
- a summary of up to 4 KB

The composer SHALL show this as a Viewing chip, which can be removed for the next message. References that
Ryder adds himself SHALL show as separate chips. A message sent while a referenced repository has no
checkout on the thread's computer SHALL carry a note naming the other computer, and the composer SHALL
offer a new chat on that computer.

#### Scenario: Selection and pane
- **WHEN** Ryder sends from Chat with NAC-7 open in the Beads pane
- **THEN** the message's context names the checkout and NAC-7 as the selection, and the composer showed "Viewing: NAC-7" before sending

#### Scenario: Chat tab context
- **WHEN** Ryder opens a round's work inspector, closes it and sends from Chat
- **THEN** the message's context is that round

#### Scenario: Repository on another computer
- **WHEN** a referenced repository has no checkout on the thread's computer
- **THEN** the composer says so and offers a new chat on that computer, and a message sent anyway carries a note naming the other computer

#### Scenario: About chip
- **WHEN** Ryder moves from a bead to a change in the supporting pane
- **THEN** the Viewing chip changes from the bead's id to the change's name

#### Scenario: Not this time
- **WHEN** Ryder removes the Viewing chip and sends
- **THEN** the message carries the project but no viewed item, and the chip returns for the next message

### Requirement: Chat starts where it was opened
A new chat SHALL run in the workspace's project checkout, on that checkout's computer, unless Ryder picks
another computer from the New chat menu.

#### Scenario: From a bead
- **WHEN** Ryder taps Ask about this on NAC-48p.5 in a checkout with no thread yet
- **THEN** a new thread runs on that bead's computer in its repository's checkout

#### Scenario: From Boards
- **WHEN** Ryder taps New Claude chat on Projects
- **THEN** the app asks which project, and the chat runs in that project's checkout on its computer

### Requirement: One chat replaces every other AI chat in the app
The app SHALL hold no AI chat surface other than the workspace's Chat destination. Ask about this on a
bead, a change, a requirement or a selection SHALL open Chat with a reference chip. Bead comments SHALL
stay plain comments. A lighter model SHALL be used only through routing within the same chat.

#### Scenario: Same chat from the header and the bead
- **WHEN** Ryder taps Ask about this on a bead, then on a change
- **THEN** both open the same current thread with its messages and draft, each adding its reference

#### Scenario: Bead comments
- **WHEN** Ryder opens a bead's comments
- **THEN** they are plain comments with no AI reply or chat

#### Scenario: No second chat
- **WHEN** Ryder opens Projects, a bead peek or a change
- **THEN** no assistant other than the workspace's Chat is offered

## ADDED Requirements

### Requirement: Agent state follows the execution state
The app SHALL state what the agent is doing in text, from the turn's execution state:
- Working, while the process runs (the loading indicator shows only while output arrives)
- No updates for N min, as a plain observation
- May be stalled, only when nacred's stall signal fires
- Waiting for you, Reconnecting, Failed

Tool steps SHALL collapse to a summary row that stays expandable.

#### Scenario: Long test run
- **WHEN** the agent runs a 6-minute test command with no output
- **THEN** Chat shows Working with "No updates for 6 min", not "Stalled"

### Requirement: One status strip in the workspace
Every workspace destination SHALL show one project-wide status strip, which combines the project's
running work (for example, "Round running · 3 of 7 landed") with its Needs-you count. The strip SHALL have
two separate targets:
- the work summary SHALL open the work inspector
- the Needs-you count SHALL open the project's queue, whether or not work is running

Suggestion chips SHALL show only while the composer is empty. They SHALL fill the composer and never send
on their own.

#### Scenario: Decisions while work runs
- **WHEN** a round runs and Ryder taps "2 need you" in the strip
- **THEN** the project's queue opens, not the work inspector

#### Scenario: A suggestion
- **WHEN** Ryder taps the suggestion "What's blocked?"
- **THEN** the composer holds "What's blocked?" and nothing is sent until he taps Send
