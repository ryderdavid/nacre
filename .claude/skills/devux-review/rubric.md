# Rubric

## Who it's for

- **Ryder, primary.** A solo developer running several repositories from a Samsung Z Fold. He uses the
  folded cover screen (about 400 dp, one thumb, often walking, voice dictation) and the unfolded inner screen
  (about 900 dp, tabletop, thinking). He works through beads and OpenSpec. He wants plain language, no jargon,
  no dollar amounts on buttons, and to be asked only what only he can decide.
- **A developer like Ryder, secondary.** Someone who has Claude Code or Codex on a Mac and wants to steer it
  from a phone. Nacre should be desirable to them too: it should feel obvious, calm and trustworthy without a
  tour.

## Standing constraints (findings must respect them)

- Nacre keeps the subscription path: it drives the official `claude` and `codex` CLIs on the host and never
  lifts OAuth tokens.
- Protected actions (approve, land, finish, sign-off, limits) happen only through decision cards that Ryder
  answers on a paired device.
- Beads is the task tracker, and OpenSpec is the behaviour contract. Don't propose a third system.
- Material 3 Expressive on Android, for the folded phone, the unfolded phone and the tablet.
- Chat is the centre (Chat · Spec · Beads). Proposals should fit into Chat, cards and panels, not add
  destinations.

## Phases

- **ideation**: covers everything from "I have an idea" (or "something's off") until a bead exists or an
  OpenSpec change is approved. That includes:
  - the New effort chat and the `nacre-draft` Draft card;
  - the intake verdict and the New effort sheet;
  - drafting by the author and reviewer loop, and proposals in the Proposals lane;
  - the change screen and approval;
  - questions the agent asks, mockups, and images Ryder attaches.
- **confusion**: covers moments when Ryder doesn't understand what he's seeing, why he's being asked, or what
  will happen. It also covers the reverse, when the agent misunderstands him, along with recovery, undo and
  explanation.
- **execution**, **review** and **return**: see SKILL.md.

## Benchmark affordances (what the other apps give in these moments)

Reviewers compare Nacre against these and should add newer ones they know of:

- **Claude Code (desktop app, web, CLI):**
  - plan mode, ending in a plan to approve or edit (ExitPlanMode);
  - AskUserQuestion: up to four structured multiple-choice questions, with previews and an "Other" answer;
  - an explore or brainstorm mode that thinks without acting;
  - inline visuals: diagrams (native Mermaid), mockups, charts, and elicitation widgets the user clicks to answer;
  - design canvases, and Artifacts with version history;
  - Artifacts: shareable pages and docs;
  - conversation rewind and checkpoints, forking a conversation, `/compact`, chapters and a table of contents;
  - side sessions or background tasks, with subagents for research;
  - @-mentions for files and symbols, image paste, and voice;
  - slash commands and skills such as `/opsx:explore` and `/opsx:propose`;
  - a diff pane, a browser pane and a simulator pane;
  - memory files.
- **Codex (desktop app, cloud, CLI):**
  - Ask and Code modes;
  - several attempts in parallel (best-of-N), compared side by side;
  - a worktree per task;
  - a task list with status;
  - review of a diff before applying it, with inline comments;
  - AGENTS.md, skills and automations;
  - a plan checklist that updates as the agent works.

## Finding schema

Each lens returns findings as JSON objects:

```json
{
  "id": "short-slug",
  "phase": "ideation",
  "moment": "where in the flow (e.g. 'Draft card first appears', 'intake verdict is wrong')",
  "observed": "what Nacre does today, with file:line or spec requirement as evidence",
  "gap": "what Ryder can't do, or what costs him, in his words",
  "benchmark": "what Claude Code / Codex give here, if anything (or 'none, Nacre could lead')",
  "proposal": "the smallest change that closes the gap, in Chat/card/panel terms",
  "user_cost": 1-5,
  "frequency": 1-5,
  "effort": 1-5,
  "evidence": ["path:line", "spec: <capability> / <requirement>"],
  "covered_by": "open bead or active change that already covers it, or null"
}
```

The score is `user_cost × frequency / effort`.

- **user_cost:** 5 means he gives up and goes to the Mac. 3 means friction he notices every time. 1 means polish.
- **frequency:** 5 means every effort. 3 means weekly. 1 means rarely.
- **effort:** 1 means copy or a prompt change. 3 means one OpenSpec change. 5 means a new subsystem.

## Report outline

```text
# Developer UX review: <phase> (<date>)
One-paragraph answer to the question asked.
## What Nacre already does well here      (3–6 bullets, so we keep it)
## The gaps, ranked                         (table: finding · score · lenses · covered by)
## Now / Next / Later                       (each finding: gap → proposal → evidence)
## First slice                              (one OpenSpec change: name, why, what changes)
## Not recommended                          (ideas the lenses raised that break a constraint or don't pay)
## Method                                   (lenses run, files read, what wasn't checked on a device)
```
