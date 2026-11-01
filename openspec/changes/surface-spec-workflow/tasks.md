<!-- nacre-section: ssw-attention -->
## 1. Needs you in nacred

- [x] 1.1 Attention item type, stable ids and option types; `bead_decision` and round kinds (`round_question`, `bead_parked`, `round_ready`) — `cd bridge && go test -race ./internal/server ./internal/rounds`
- [x] 1.2 Change kinds (`proposal_ready`, `review_blocked`, `approval_stale`, `workflow_interrupted`) — `cd bridge && go test -race ./internal/specflow ./internal/server`
- [x] 1.3 `GET /v1/attention` with per-repository caching, documented in docs/API.md — `cd bridge && go vet ./... && go test ./internal/server`
- [x] 1.4 `GET /v1/repos/{repo}/changes` with cached evidence and the inventory summary on change GET — `cd bridge && go test -race ./internal/specflow ./internal/server`
- [x] 1.5 `change` events on `/v1/events` — `cd bridge && go test -race ./internal/server ./internal/specflow`

<!-- nacre-section: ssw-preflight -->
## 2. Spec drift in preflight

- [x] 2.1 Exported, bounded `RequirementIndex` that skips repositories without valid OpenSpec — `cd bridge && go test -race ./internal/specflow`
- [x] 2.2 `spec-drift` finding with required citation and fixed options; exception answer's comment and `spec-touch:` label — `cd bridge && go test -race ./internal/rounds`
- [x] 2.3 Leave-out answer: `BeadSkipped` with reason, note, dependents skipped with "was left out" wording — `cd bridge && go test -race ./internal/rounds`
- [x] 2.4 Amend-first answer: leave the round, start `develop`, reopen the question on refusal — `cd bridge && go test -race ./internal/rounds ./internal/specflow`

<!-- nacre-section: ssw-archive -->
## 3. Stale reasons and readiness to archive

- [x] 3.1 `stale_reason` for file, seed-intent, section-edit and no-baseline causes, and `approval_stale` items for any cause — `cd bridge && go test -race ./internal/specflow ./internal/server`
- [x] 3.2 Provenance-based archive readiness and the `change_archivable` item — `cd bridge && go test -race ./internal/specflow`

<!-- nacre-section: ssw-assistant -->
## 4. Assistant

- [x] 4.1 `spends` flag on `ToolSpec`, always asking even in auto-apply — `cd android && ./gradlew -q testDebugUnitTest`
- [x] 4.2 `develop_proposal` and `create_bead.then_develop` with one approval card — `cd android && ./gradlew -q testDebugUnitTest`
- [x] 4.3 `needs_you` read tool, Mac prompt injection in nacred, and prompt rules for offering proposals — `cd android && ./gradlew -q testDebugUnitTest` and `cd bridge && go test ./internal/assist`

<!-- nacre-section: ssw-app -->
## 5. App surfaces

- [x] 5.1 Needs you section on Boards, merged across computers, with inline options — `cd android && ./gradlew -q testDebugUnitTest`
- [x] 5.2 Boards badge; Runs badge unchanged — `cd android && ./gradlew -q testDebugUnitTest`
- [x] 5.3 Bead card spec chip in the first label slot and the epic section strip — `cd android && ./gradlew -q testDebugUnitTest`
- [x] 5.4 Epic-led bead detail; Develop a proposal moved into Refine and the overflow menu — `cd android && ./gradlew -q testDebugUnitTest assembleDebug`
- [x] 5.5 Stale reason in change detail and on the card — `cd android && ./gradlew -q testDebugUnitTest`
- [x] 5.6 Change notifications and watching while proposals are being worked — `cd android && ./gradlew -q testDebugUnitTest`

<!-- nacre-section: ssw-intake -->
## 6. New effort intake

- [x] 6.1 `POST /v1/repos/{repo}/intake`: bounded read-only Headless call over the requirement index and active changes' Why, $0.50 and 90 s caps, citation checks, fallback to new, and `target_active` from `Active()`; the endpoint documented in docs/API.md — `cd bridge && go test -race ./internal/specflow ./internal/server`
- [x] 6.2 Optional `intake {verdict, change_name}` on `develop`, stored on the record; history entry when the author's route differs; both documented in docs/API.md — `cd bridge && go vet ./... && go test ./internal/specflow ./internal/server`
- [x] 6.3 New effort mode in `NewBeadSheet`: Continue, verdict card with overrides, spending label, disabled amend while work is active with "develop later", `target_tracked` note, sending `related`, `follow_on_of` and `found_comment` when opened from found work, refusal and failure states; New bead unchanged without OpenSpec or on older bridges — `cd android && ./gradlew -q testDebugUnitTest`

<!-- nacre-section: ssw-lane -->
## 7. Proposals lane

- [x] 7.1 Changes summary orders pre-promotion changes first within its cap; `workflowLane(issue, changes)` from `seed_ids` and phase, used only by Workflow `columns` and `stats`; lane ordering; "Showing 50 of n" — `cd bridge && go test ./internal/specflow ./internal/server` and `cd android && ./gradlew -q testDebugUnitTest`
- [x] 7.2 `StartRoundSheet` excludes Proposals and `found-amend` beads; assistant board tools report lane key `proposed`; scroll-to-selected, the bead detail lane label and `NacreColors.lane()` handle Proposals — `cd android && ./gradlew -q testDebugUnitTest`
- [x] 7.3 Not a drop target (`moveActionFor` returns none for `proposed`), cards don't drag, explicit `initialPage` on Ready, Proposals tile only when non-zero, covered cards offer Close, empty-lane hint — `cd android && ./gradlew -q testDebugUnitTest assembleDebug`
- [x] 7.4 Close alias seeds after their revision is approved and synchronized (`Close` on specflow's Beads interface); `seed_ids` includes aliases — `cd bridge && go test -race ./internal/specflow`

<!-- nacre-section: ssw-found -->
## 8. Found something

- [x] 8.1 `bd.CreateParams.NoInheritLabels`; every create through `POST …/issues` or the found endpoint whose parent is a spec epic uses it (promotion's section creates excluded) and adds `found:<change-id>`; guard test that such a bead runs as ordinary work; guards refuse `found-amend` beads; preflight spec drift skips `found:` beads — `cd bridge && go test -race ./internal/bd ./internal/server ./internal/rounds`
- [x] 8.2 `POST /v1/repos/{repo}/changes/{change}/found`: how/order (with optional `blocks` before a section), amend (found bead + `found-amend` + item), out of scope (prefill only), dismiss, optional `comment_id` on every answer, `discovered-from` when found in a bead; New effort create gains `related` and `follow_on_of` — `cd bridge && go test -race ./internal/specflow ./internal/server`
- [x] 8.3 Revise now: joins found notes under the feedback cap, records bead ids on the revise action, suppresses their items while pending, closes them after approval and synchronization; item states reopened-section count; disabled while work is active; follow-on closes the found bead with a link — `cd bridge && go test -race ./internal/specflow`
- [x] 8.4 Spec round prompt's stay-in-section rule; `Found:` comments on beads in any end state without a `[nacre-found:<id>]` reply become `found_work` items; answers and Dismiss post the marker — `cd bridge && go test -race ./internal/rounds ./internal/server`
- [x] 8.5 Found endpoint, `found_work` kind, `found:`/`found-amend` labels, the create fields and the extra archive condition in docs/API.md — `cd bridge && go test ./internal/server`, then the doc checked against those tests
- [x] 8.6 Found something sheet on spec epics, section and found beads and spec rounds (round-bead picker), "Must happen before" picker; `found_something` assistant tool; `found_work` items in Needs you: Answer opens the sheet, Revise now with its disabled reason and "in revision" state, Follow-on opens New effort; the two sheet `open` targets — `cd android && ./gradlew -q testDebugUnitTest`

<!-- nacre-section: ssw-plan -->
## 9. Plan and progress

- [x] 9.1 Archive readiness waits for open `found:` beads; `tasks.md`-edited stale reason — `cd bridge && go test -race ./internal/specflow`
- [x] 9.2 Change detail Work section and epic header: sections in plan order with bead status and evidence, "needs a new round" after a revision, "Added during work" group, checkboxes labelled Plan — `cd android && ./gradlew -q testDebugUnitTest`

<!-- nacre-section: ssw-verify -->
## 10. Verification

- [x] 10.1 Cover and unfolded screens on the Fold: inbox, chips, epic detail, New effort, Proposals lane, Found something — `nacred gates --build`, then a device walkthrough recorded on the bead
- [x] 10.2 One real cycle: a New effort routed to a change (or the assistant proposes one), Ryder approves, a round lands, and Needs you reports the change ready to archive — `nacred gates`, then `openspec validate --all --strict` on the landed target, with run and round ids recorded on the beads
