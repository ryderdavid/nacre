## 1. Rounds land where work belongs

- [x] 1.1 `land_when_ready` on round creation, allowed for specification sections; `go test -race ./internal/rounds ./internal/server`
- [x] 1.2 Epic branches as round targets; mixed epics refused; no bump or tag off the target; `go test -race ./internal/rounds`
- [x] 1.3 Finishing rounds from the epic branch (or the target for archive-only); close the epic, delete its branch; `go test -race ./internal/rounds`

## 2. Specification changes finish too

- [x] 2.1 Archive the change in the finishing round's workspace and commit it; the workflow ends `archived`; `go test ./internal/specflow ./internal/rounds`
- [x] 2.2 Adoption keeps section markers in the workflow, not the checkout; `go test ./internal/specflow`

## 3. Needs you and the app

- [x] 3.1 Needs you: Finish for done epics and archivable changes, Work until done or Back to Ready for stuck beads; `go test ./internal/attention`
- [x] 3.2 App: Work until done in Implement, the epic completion card, round cards, labels, the outdated-nacred note; `./gradlew testDebugUnitTest`

## 4. Review findings

- [x] 4.1 Modified deltas for spec-surfacing, needs-you and spec-guided-work; out of scope; failure and recovery scenarios; `openspec validate finish-to-main --strict`
- [x] 4.2 Tests: a mixed round names each epic's beads; a branch already in the target closes the epic; a spec round lands when asked and not on `land.auto` alone; a finishing round that fails before landing leaves the target and epic untouched; `go test -race ./internal/rounds`
