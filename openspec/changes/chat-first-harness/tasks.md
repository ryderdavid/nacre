# Tasks

The gates named below:
- **bridge**: `cd bridge && go vet ./... && go test -race ./...`
- **app**: `cd android && ./gradlew -q testDebugUnitTest`
- **specs**: `openspec validate --all --strict`

## 1. nacred: protected actions and the chat-scoped token

- [ ] 1.1 Mint a chat-scoped token per chat turn and put it in the turn's environment, never the device token. Verify with a Go test that reads a fake turn's environment. Gates: bridge.
- [ ] 1.2 Mark the protected routes (approve, land, finish, sign-off, limits and budget, spending or landing stop choices, adopt) and refuse the chat token on them with 403 `needs_decision`. Device tokens keep working. Verify with a table test over every protected and unprotected route, legacy routes included. Gates: bridge.
- [ ] 1.3 Document the token scopes and `needs_decision` in docs/API.md, and say in the chat briefing how to raise a decision. Verify with a test that the briefing names it. Gates: bridge, specs.

## 2. nacred: decisions

- [ ] 2.1 Generalize the permission-request store into a decision store. A record holds the kind, the targets, the exact action, the revision, the consequences, an expiry, the state and the answer. It persists across restarts. Verify with Go tests for create, load and expiry. Gates: bridge.
- [ ] 2.2 Add `POST /v1/chats/{id}/decisions`. The agent gives a kind and a target, and nacred builds the action, the revision and the consequences: approve (with or without run or land), land (target and push from `nacre.yaml`), finish, sign-off, limit, and adopt. Verify with a test per kind checking the computed consequences. Gates: bridge.
- [ ] 2.3 Answer a decision from a paired device only. Revalidate the revision, execute once (idempotent by decision id), and record done, failed or outcome unknown. Verify with tests for a stale revision, a double answer, a failed execution, and an answer from the Mac's own address (403). Gates: bridge.
- [ ] 2.4 The queue lifecycle: defer (out of the count for a day or until the state changes), reject per kind (revise with a note, Held round, send back), expiry with Ask again, and supersession. Verify with Go tests for each path. Gates: bridge.
- [ ] 2.5 Show attention items as decisions by mapping their server-defined options, keeping stable ids. Verify that one decision serves the queue and the chat. Gates: bridge.
- [ ] 2.6 Notify the phone about new decisions as permission requests are notified today, and stream decision events on the chat's event stream. Verify with a server test. Gates: bridge.

## 3. nacred: summaries for the new screens

- [ ] 3.1 Add `GET /v1/projects`: projects grouped by remote and computer, with live status, the Needs-you count, the active change and the last line. Verify with a server test. Gates: bridge.
- [ ] 3.2 List chats per checkout, with title, agent, waiting state and last line. Verify with a server test. Gates: bridge.
- [ ] 3.3 Add a work summary per round and run for the work card: progress, gates, spend, and the stall signal. Verify with a server test that the stall signal follows the configured threshold. Gates: bridge.
- [ ] 3.4 Document 3.1 to 3.3 and the decision API in docs/API.md. Gates: specs.

## 4. App: the shell and Projects

- [ ] 4.1 Add a "Chat-first layout" setting (on in debug builds) that switches between the old shell and the new one. Verify with a JVM test of the route choice. Gates: app.
- [ ] 4.2 Build Projects: grouped by computer, connection state with "last checked", project rows with status, count and change, the All / Needs you / Working filter, search, Settings in the overflow, and the "New Claude chat" split button with its project choice. Verify with JVM tests of grouping, filtering and offline state. Gates: app.
- [ ] 4.3 Build the workspace scaffold: Chat, Spec and Beads, the short navigation bar on compact widths, the rail with the project switcher on wider ones, and Back to Projects. Verify with a JVM test of the layout choice by width class. Gates: app.
- [ ] 4.4 Build the supporting pane: minimum widths, the 680dp threshold, divider presets computed from usable width, Reset as clamp(W/2, 360, W − 320) (with accessibility actions), clamped dragging, and Focus with the one-line chat bar. Verify with JVM tests at 728dp, 690dp, 680dp and 600dp. Gates: app.
- [ ] 4.5 Handle posture and state: hinge split through PaneLayout, tabletop, and compact height. Keep the thread, draft, selection and scroll across fold, unfold and resize. Verify with JVM tests of the saved state. Gates: app.

## 5. App: Chat

- [ ] 5.1 Threads per checkout: the title with computer and agent, the thread switcher (this checkout, other computers, and the split button), and the draft, scroll and panel selection restored per thread. Verify with ChatHub JVM tests. Gates: app.
- [ ] 5.2 Add the Viewing chip (removable for one message) and reference chips. Ask about this on beads, changes, requirements and selections adds a reference. Verify by testing the context that is sent. Gates: app.
- [ ] 5.3 Add the project-wide status strip on every destination, with two targets: the work summary opens the inspector, and the Needs-you count always opens the queue, and suggestion chips that fill the composer only. Verify with JVM tests. Gates: app.
- [ ] 5.4 Show agent states from the execution state (Working, No updates for N min, May be stalled from nacred's signal, Waiting, Reconnecting, Failed), with collapsible tool rows. Verify with a test of a long silent tool run that never says stalled. Gates: app.

## 6. App: decision cards

- [ ] 6.1 Card base: exact options, recommended first, with consequences, an optional note, separate Defer, Reject and Cancel, input kept per card, and the states with truthful results and receipts. Verify with JVM tests of the state mapping. Gates: app.
- [ ] 6.2 Reviewed-action cards: project, computer and revision, consequences, limits, evidence links, separate alternatives, and a required note on reject. Verify that Approve, run, I'll land (recommended), Approve, run and land to main, and Approve only are distinct and identical wherever the card shows. Gates: app.
- [ ] 6.3 Question cards (round question, spec drift, intake verdict with override) with the own-words answer and Ask about this. Verify with JVM tests. Gates: app.
- [ ] 6.4 Human-check, recovery (amount validation) and found-work cards. Verify with JVM tests, including amount validation. Gates: app.
- [ ] 6.5 The queue view: one list per project, cards inline in the thread that raised them, answered once and updated everywhere, and Held items with their next actions. Verify with JVM tests. Gates: app.

## 7. App: work card and work inspector

- [ ] 7.1 The work card in the strip and the thread: determinate progress, gates and spend, and Stop with its scope ("Stop this reply" or "Cancel round and its workers", with a confirmation). Verify with JVM tests. Gates: app.
- [ ] 7.2 The work inspector: beads with state, branch and verification, gates, failures, artifacts and changes, the live log, per-bead Retry, recovery choices, Cancel round and the full log. All of it works without the agent. Verify with RoundViewModel tests. Gates: app.

## 8. App: Spec panel

- [ ] 8.1 The change list by phase, plus specs and search. Verify with a JVM test of the grouping. Gates: app.
- [ ] 8.2 The change overview: phase and revision, the stale reason, the review verdict and findings, the approved plan beside live progress, and evidence links. Verify with JVM tests. Gates: app.
- [ ] 8.3 The document reader: document selector, outline, find, next and previous change, selectable text, Ask about the selection, Added, Modified and Removed labels, and the floating toolbar. Verify with a JVM test of the delta parsing. Gates: app.

## 9. App: Beads panel

- [ ] 9.1 The board by width: columns of at least 272dp when wide or focused, otherwise the compact board with lane tabs and counts, plus filters that explain what they hide. Verify with JVM tests. Gates: app.
- [ ] 9.2 Moving beads: long-press drag with the drop dock, Move to… in the menu and as a TalkBack action, Done closes with Undo, and Proposals and To check refuse drops. Verify with BoardModel tests. Gates: app.
- [ ] 9.3 The bead peek: description, acceptance, dependencies, comments and the latest run, with Ask about this, Start work (stopping before landing) and Open change. The board follows chat's changes live. Verify with JVM tests. Gates: app.

## 10. App: Material 3 Expressive look

- [ ] 10.1 Theme: dynamic color with the teal fallback, Roboto Flex emphasized headlines, expressive shapes for identity only, spring motion with reduced-motion crossfades, and stable card geometry. Verify with a contrast unit test of the fallback schemes against the app-accessibility thresholds (light and dark). Gates: app.

## 11. Remove what chat replaced

- [ ] 11.1 Remove the old shell, the Inbox and Activity tabs, the round and run screens, the change-action buttons, the sheets (StartRound, GatesSetup, NewBead and intake, FoundSomething, OwnAnswer, RoundQuestions, Signoff, RunLaunch, BeadSheets editors), the ✨ header chat, the docked, edge and sheet chat, the live-send action, and the setting from 4.1. Delete their tests or move them to the new screens. Verify with the app gate and a check against the inventory that every removed flow maps to chat or a card. Gates: app, specs.

## 12. Device checks (label the bead `human`)

- [ ] 12.1 On the Fold, cover and unfolded: open a project, switch threads, answer each card kind from the phone, land a round through a decision, retry a failed bead from the inspector, stop a round, read a spec delta, and move a bead. Record the evidence on the bead.
- [ ] 12.2 Accessibility runs: TalkBack, Switch Access and a hardware keyboard through the same flows, plus 200% font and dark theme. Record the results on the bead.

## Workflow follow-up

- Finish the epic and archive this change when every section is done and signed off.
