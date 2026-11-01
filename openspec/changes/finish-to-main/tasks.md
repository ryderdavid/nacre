## 1. Rounds land where work belongs

- [x] 1.1 `land_when_ready` on round creation, allowed for specification sections
- [x] 1.2 Epic branches as round targets; mixed epics refused; no bump or tag off the target
- [x] 1.3 Finishing rounds from the epic branch (or the target for archive-only); close the epic, delete its branch; `go test -race ./internal/rounds`

## 2. Specification changes finish too

- [x] 2.1 Archive the change in the finishing round's workspace and commit it; the workflow ends `archived`
- [x] 2.2 Adoption keeps section markers in the workflow, not the checkout; `go test ./internal/specflow`

## 3. Needs you and the app

- [x] 3.1 Needs you: Finish for done epics and archivable changes, Work until done or Back to Ready for stuck beads; `go test ./internal/attention`
- [x] 3.2 App: Work until done in Implement, the epic completion card, round cards, labels, the outdated-nacred note; `./gradlew testDebugUnitTest`
