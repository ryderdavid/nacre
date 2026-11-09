## MODIFIED Requirements

### Requirement: New effort drafts in Chat
In a repository with OpenSpec, on a computer whose Chat drafts, the Chats drawer and a project's empty Chat
SHALL offer New effort. New effort SHALL start a new Claude chat on the board's computer. That chat's first
message SHALL carry a drafting prompt for the board, which asks the agent to do three things before its first
draft:
- restate in a few lines what it thinks Ryder means;
- name its assumptions;
- ask at most three numbered questions.

Each later draft SHALL say in one line what changed. When the agent writes a `nacre-draft` block, nacred SHALL
record it as a draft. A draft MAY carry a short digest of the brainstorm: what was decided, what was ruled out,
open questions and the mockups agreed on.

The app SHALL show only the newest draft, as a Draft card with "Where does this belong?". That control SHALL
raise an intake card in the same thread from that recorded draft, so the draft Ryder read is the draft intake
sees. The card SHALL also say that he can tell the chat what to change.

#### Scenario: Draft and intake
- **WHEN** Ryder taps New effort in the Chats drawer, talks the idea through, and taps "Where does this belong?" on the Draft card
- **THEN** an intake card for that draft appears in the same thread, and nothing is created until he answers it

#### Scenario: Playback before the first draft
- **WHEN** Ryder describes an idea in a New effort chat
- **THEN** the agent's first reply restates what it understood, names its assumptions and asks at most three numbered questions, and contains no draft unless he asked for one

#### Scenario: Newer draft
- **WHEN** the agent writes a second draft after Ryder's correction
- **THEN** only the second Draft card is shown, the reply says in one line what changed, and the button raises intake from the second draft

#### Scenario: Empty project
- **WHEN** Ryder opens Chat in an OpenSpec project with no chat yet
- **THEN** New effort is offered beside the Claude and Codex new-chat actions

#### Scenario: Prefilled New effort
- **WHEN** Ryder answers a found item Out of scope
- **THEN** a New effort chat opens with the note in the composer, and intake raised from its draft creates a bead related to the epic

#### Scenario: Unreadable draft
- **WHEN** the agent's `nacre-draft` block doesn't parse or has no title
- **THEN** no Draft card is shown, the chat says "The draft couldn't be read" with the reason, and the conversation continues

#### Scenario: Older bridge
- **WHEN** the board's computer can't raise intake from a recorded draft
- **THEN** New effort still starts a drafting chat, the Draft card shows no button, and the agent raises intake when Ryder says to go ahead

#### Scenario: Repository without OpenSpec
- **WHEN** the project has no valid OpenSpec setup
- **THEN** New effort is not offered

### Requirement: A routing model chooses a quick answer or the full harness
Before each message in a Claude chat that is not drafting, nacred SHALL ask a small routing model, which has no
tools, to choose `light` or `full`. The router SHALL see only:
- the message text and its "On screen" note;
- how many images are attached;
- whether the chat is a New effort chat;
- the last six messages, trimmed to 2 KB.

A `light` answer SHALL run on a lighter model and `full` on the full harness. An error, a 15-second timeout or a
malformed answer SHALL mean `full`.

A light turn SHALL be able to write files in its chat's mockups directory and nowhere else, so it can show a
mockup or diagram without handing off.

While a New effort chat is drafting, its messages SHALL NOT be routed. Each one SHALL run on the full model. It SHALL
be able to read, and to write only in the chat's mockups directory. Any other step it asks for, including
leaving plan mode, SHALL be refused with a reminder to write or revise the draft. Drafting SHALL end when Ryder answers
the chat's intake card with an option that creates a bead or develops a change. After that, the chat's messages
SHALL be routed as in any other chat.

The app SHALL NOT offer a model choice.

#### Scenario: Plain question
- **WHEN** the router answers `light` for a message
- **THEN** the message runs as a light turn with the lighter model in the chat's same agent session

#### Scenario: Real work
- **WHEN** the router answers `full` for a message
- **THEN** the message runs as a full turn with the full-harness argv

#### Scenario: Router fails
- **WHEN** the routing call errors, times out or returns something other than `light` or `full`
- **THEN** the message runs as a full turn

#### Scenario: Light turn draws
- **WHEN** a light turn writes an HTML file into its chat's mockups directory and names it on a `MEDIA:` line
- **THEN** the write is allowed without handing off, and the mockup shows in the chat

#### Scenario: Light turn writes elsewhere
- **WHEN** a light turn asks to write a file outside its chat's mockups directory
- **THEN** the turn hands off to the full harness as it does for any step it can't take alone

#### Scenario: Drafting turn
- **WHEN** Ryder sends a message in a New effort chat whose intake card hasn't been answered with create or develop
- **THEN** no routing call is made, and the turn runs on the full model, able to write only mockups

#### Scenario: Drafting turn tries to act
- **WHEN** a drafting turn asks to edit a repository file, run a command that writes, or leave plan mode to act
- **THEN** the request is refused, and the agent is told it is still drafting

#### Scenario: Drafting ends
- **WHEN** Ryder answers the chat's intake card with Just a bead, New change or Amends
- **THEN** the chat's later messages are routed like any other chat's

#### Scenario: No model picker
- **WHEN** Ryder starts a chat or sends a message
- **THEN** no control offers a model choice

### Requirement: Every chat is briefed on Nacre
Every chat SHALL start with the Nacre briefing appended to its system prompt. The briefing SHALL cover:
- bd and OpenSpec changes;
- the "On screen" note;
- the `nacred` command and API for Needs you, changes, rounds, found work and sign-off;
- making code changes on a branch or in a git worktree, and committing or naming edits left in a checkout before
  a turn ends;
- the New effort draft block and its digest, and raising intake only when Ryder says to go ahead;
- showing mockups and diagrams;
- Taildrop delivery.

#### Scenario: Briefing
- **WHEN** any chat starts
- **THEN** its system prompt is appended with the briefing, and its environment has `NACRED_URL` set to nacred's own listen address

#### Scenario: Edits in a checkout
- **WHEN** any chat starts
- **THEN** its briefing tells the agent that nacred lands into the repository checkouts, to make code changes on a branch or in a git worktree, and before ending a turn that edited a checkout to commit the edits or name the files left uncommitted

#### Scenario: New work waits for a go-ahead
- **WHEN** Ryder describes new work in any chat
- **THEN** the briefing tells the agent to talk it through and draft it, and to raise intake only when he says to go ahead

#### Scenario: Phone unknown for Taildrop
- **WHEN** nacred cannot resolve the phone's Tailscale device from the request
- **THEN** the briefing says the device is unknown and to ask Ryder

## ADDED Requirements

### Requirement: Chat shows mockups and diagrams
Every chat SHALL have a mockups directory named in its environment. A file there that a reply names on its own
`MEDIA:` line SHALL be shown in the chat: an HTML file in a sandboxed view with no navigation, an image as an
image, and either one full screen with pinch zoom. Files outside the chat's mockups directory SHALL NOT be shown.

#### Scenario: Mockup shown
- **WHEN** a reply names an HTML file in its chat's mockups directory on a `MEDIA:` line
- **THEN** the page renders inline and opens full screen with pinch zoom

#### Scenario: File outside the directory
- **WHEN** a reply names a `MEDIA:` path outside the chat's mockups directory
- **THEN** no mockup is shown for it

### Requirement: Chat draws what it explains
The briefing SHALL tell the agent to draw a flow, architecture or sequence of three or more parts as a mockup
rather than only describe it. It SHALL also say that a diagram may be an HTML file that loads a pinned Mermaid
build from a CDN.

#### Scenario: Diagram from a question
- **WHEN** Ryder asks how intake reaches the author
- **THEN** the reply may include a diagram mockup drawn by a light turn, shown inline and openable full screen
