## 1. Daemon update contract

- [x] 1.1 Implement durable update state, pinned stable-release preparation, isolated helper and verified rollback; gate: updater unit/race tests
- [x] 1.2 Add authenticated status/start routes, atomic maintenance admission and active-work guards; gate: server tests and Go vet

## 2. Settings experience

- [x] 2.1 Add typed update client and durable server-status reconciliation per host; gate: Android JVM tests
- [x] 2.2 Add existing-style update action, per-host progress and compatibility states; gate: lint and emulator walkthrough

## 3. Verification and handoff

- [x] 3.1 Review failure paths and document release/bootstrap requirements; gate: strict OpenSpec validation and focused regression tests
- [x] 3.2 Build signed APK and bridge, preserve artifacts and report validation limits; gate: release build and isolated UI smoke test
