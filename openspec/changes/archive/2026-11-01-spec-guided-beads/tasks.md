## 1. Durable proposal and review

- [x] 1.1 Add workflow types, safe inventory/artifact IO, revision digests and persisted recoverable state — `cd bridge && go test ./internal/specflow`
- [x] 1.2 Implement bounded structured author/reviewer loop, strict OpenSpec validation, findings and feedback — `cd bridge && go test ./internal/specflow`
- [x] 1.3 Add deterministic tests for conflict, path escape, interruption, stale approval and review blockers — `cd bridge && go test -race ./internal/specflow`

## 2. Beads and execution contract

- [x] 2.1 Implement stable section parsing/mapping, recoverable promotion, conflict-safe synchronization and decision comments — `cd bridge && go test -race ./internal/specflow ./internal/bd`
- [x] 2.2 Add authenticated API, startup wiring, linked run/round guards and approved contract propagation — `cd bridge && go test ./internal/server ./internal/specflow && go vet ./...`
- [x] 2.3 Document HTTP fields, errors, durability, limits and manual completion/archive obligations — review against API tests

## 3. App workflow

- [x] 3.1 Add typed models, bridge/repository access, capability fallback and refresh state — `cd android && ./gradlew testDebugUnitTest`
- [x] 3.2 Add theme-aligned change card/detail, artifact review, feedback retention and explicit approval/round handoff — `cd android && ./gradlew testDebugUnitTest assembleDebug`
- [x] 3.3 Test phase labels, actions, stale/error states, evidence distinctions and folded keyboard behavior — JVM tests plus emulator walkthrough

## 4. Independent verification and build

- [x] 4.1 Adversarially review implementation against every required scenario; resolve blocking findings — evidence in review record and Beads
- [x] 4.2 Run strict OpenSpec validation, Go checks and Android unit tests/lint/release build — `openspec validate --all --strict`, `go vet`, applicable Go tests, `./gradlew testDebugUnitTest lintDebug assembleRelease`
- [x] 4.3 Exercise isolated backend workflow and folded/unfolded app, save verified APK and reproducible evidence — emulator and fixture evidence, including exact signed release smoke test; see `docs/SPEC_WORKFLOW_VERIFICATION.md`
