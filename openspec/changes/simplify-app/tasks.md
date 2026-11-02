# Tasks

Gates: `cd bridge && go vet ./... && go test -race ./...`, `cd android && ./gradlew testDebugUnitTest
assembleDebug`, `openspec validate --all --strict`.

## 1. nacred: goal-driven fan-out and Run epic (NAC-965.1)
- [ ] 1.1 Every round worker gets its own per-bead goal; new rounds never use one shared goal session
- [ ] 1.2 Pushing follows `nacre.yaml` only; goal committed/pushed options are ignored
- [ ] 1.3 `finish` on an epic round finishes the epic into the target once no children remain open
- [ ] 1.4 `run` on approval starts the epic's run once promotion is ready
- [ ] 1.5 Needs you omits stuck beads in repositories without `nacre.yaml` and nacred's own sync edits don't stale an approval
- [ ] 1.6 Gates: `cd bridge && go vet ./... && go test -race ./...`

## 2. App shell, chat, board and settings (NAC-965.2)
- [ ] 2.1 Boards, Inbox, Activity; Settings in the Boards top app bar; no Needs you or totals above the boards
- [ ] 2.2 docked-chat-pane sections 1–5
- [ ] 2.3 One New button; workflow lanes only; no count strip; Running badge; internal labels hidden
- [ ] 2.4 Settings trimmed
- [ ] 2.5 Gates: `cd android && ./gradlew testDebugUnitTest assembleDebug`

## 3. Work flow UI (NAC-965.3)
- [ ] 3.1 Start work on a bead; Run an agent manually… in the overflow
- [ ] 3.2 Run epic / Finish epic on an epic
- [ ] 3.3 Approve and run; Planning / Ready for your approval / Working / Done
- [ ] 3.4 Round view without legacy Resume and dependency overrides
- [ ] 3.5 Gates: `cd android && ./gradlew testDebugUnitTest assembleDebug`

## 4. Specs and docs (NAC-965.4)
- [ ] 4.1 Delta specs for the modified capabilities; `openspec validate --all --strict`
- [ ] 4.2 README.md and docs/API.md describe the new flow
- [ ] 4.3 Device check on the Fold, folded and unfolded
