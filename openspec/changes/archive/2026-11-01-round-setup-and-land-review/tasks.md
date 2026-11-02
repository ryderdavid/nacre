## 1. Round setup context

- [x] 1.1 Start-round sheet opens narrowed to the board's search and scope, with its own search and a scope toggle; gate: Android JVM tests (WalkthroughFixesTest)

## 2. Pre-land review and push behaviour

- [x] 2.1 nacred resolves land_push into the round and serves the round's commits and file changes; gate: go vet and race tests (TestRoundEndToEnd, TestRoundSurvivesRestart, TestCreateRefusals)
- [x] 2.2 Land card reviews the changes, shows the build path, and the dialog states whether it pushes; gate: Android JVM tests and emulator check
