<!-- nacre-section: ssw-attention -->
## 1. Needs you in nacred

- [ ] 1.1 Attention item type, stable ids and option types; `bead_decision` and round kinds (`round_question`, `bead_parked`, `round_ready`) — `cd bridge && go test -race ./internal/server ./internal/rounds`
- [ ] 1.2 Change kinds (`proposal_ready`, `review_blocked`, `approval_stale`, `workflow_interrupted`) — `cd bridge && go test -race ./internal/specflow ./internal/server`
- [ ] 1.3 `GET /v1/attention` with per-repository caching, documented in docs/API.md — `cd bridge && go vet ./... && go test ./internal/server`
- [ ] 1.4 `GET /v1/repos/{repo}/changes` with cached evidence and the inventory summary on change GET — `cd bridge && go test -race ./internal/specflow ./internal/server`
- [ ] 1.5 `change` events on `/v1/events` — `cd bridge && go test -race ./internal/server ./internal/specflow`

<!-- nacre-section: ssw-preflight -->
## 2. Spec drift in preflight

- [ ] 2.1 Exported, bounded `RequirementIndex` that skips repositories without valid OpenSpec — `cd bridge && go test -race ./internal/specflow`
- [ ] 2.2 `spec-drift` finding with required citation and fixed options; exception answer's comment and `spec-touch:` label — `cd bridge && go test -race ./internal/rounds`
- [ ] 2.3 Leave-out answer: `BeadSkipped` with reason, note, dependents skipped with "was left out" wording — `cd bridge && go test -race ./internal/rounds`
- [ ] 2.4 Amend-first answer: leave the round, start `develop`, reopen the question on refusal — `cd bridge && go test -race ./internal/rounds ./internal/specflow`

<!-- nacre-section: ssw-archive -->
## 3. Stale reasons and readiness to archive

- [ ] 3.1 `stale_reason` for file, seed-intent, section-edit and no-baseline causes, and `approval_stale` items for any cause — `cd bridge && go test -race ./internal/specflow ./internal/server`
- [ ] 3.2 Provenance-based archive readiness and the `change_archivable` item — `cd bridge && go test -race ./internal/specflow`

<!-- nacre-section: ssw-assistant -->
## 4. Assistant

- [ ] 4.1 `spends` flag on `ToolSpec`, always asking even in auto-apply — `cd android && ./gradlew -q testDebugUnitTest`
- [ ] 4.2 `develop_proposal` and `create_bead.then_develop` with one approval card — `cd android && ./gradlew -q testDebugUnitTest`
- [ ] 4.3 `needs_you` read tool, Mac prompt injection in nacred, and prompt rules for offering proposals — `cd android && ./gradlew -q testDebugUnitTest` and `cd bridge && go test ./internal/assist`

<!-- nacre-section: ssw-app -->
## 5. App surfaces

- [ ] 5.1 Needs you section on Boards, merged across computers, with inline options — `cd android && ./gradlew -q testDebugUnitTest`
- [ ] 5.2 Boards badge; Runs badge unchanged — `cd android && ./gradlew -q testDebugUnitTest`
- [ ] 5.3 Bead card spec chip in the first label slot and the epic section strip — `cd android && ./gradlew -q testDebugUnitTest`
- [ ] 5.4 Epic-led bead detail; Develop a proposal moved into Refine and the overflow menu — `cd android && ./gradlew -q testDebugUnitTest assembleDebug`
- [ ] 5.5 Stale reason in change detail and on the card — `cd android && ./gradlew -q testDebugUnitTest`
- [ ] 5.6 Change notifications and watching while proposals are being worked — `cd android && ./gradlew -q testDebugUnitTest`

<!-- nacre-section: ssw-verify -->
## 6. Verification

- [ ] 6.1 Cover and unfolded screens on the Fold: inbox, chips, epic detail — `nacred gates --build`, then a device walkthrough recorded on the bead
- [ ] 6.2 One real cycle: the assistant proposes, Ryder approves, a round lands, and Needs you reports the change ready to archive — `nacred gates`, then `openspec validate --all --strict` on the landed target, with run and round ids recorded on the beads
