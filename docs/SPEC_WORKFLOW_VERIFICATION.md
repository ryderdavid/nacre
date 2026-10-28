# Nacre 0.8.0 — specification workflow build

Built on 2026-10-28 from the isolated source at
`/Users/ryder/Code/HomeAgentic/nacre-spec-workflow`.

## Deliverables

- [Signed release APK](../dist/Nacre-0.8.0-specflow.apk): `com.ryderdavid.nacre`, version 0.8.0, version code 14. Uses the repository's existing sideload signing configuration.
- [Companion macOS bridge](../dist/nacred-0.8.0-specflow): reports `nacred 0.8.0-specflow`.
- [Source patch](../dist/spec-guided-beads.patch) and [changed-file inventory](../dist/changed-files.txt).
- [OpenSpec proposal](../openspec/changes/spec-guided-beads/proposal.md), [design](../openspec/changes/spec-guided-beads/design.md), [behavior specification](../openspec/changes/spec-guided-beads/specs/spec-guided-work/spec.md), [proposal review](../openspec/changes/spec-guided-beads/review.md), and [implementation review](../openspec/changes/spec-guided-beads/implementation-review.md).
- [Artifact checksums](../dist/SHA256SUMS).

The app and companion bridge are local build artifacts. The running production bridge and original app installation were not upgraded. Both components are needed for the new workflow; an older bridge displays an availability explanation.

## Result

A bead can develop an OpenSpec proposal after comparing accepted specifications and active proposals. Fresh adversarial review checks project alignment and behavior, recovery, feasibility and testability. Approval prepares a durable epic and section beads; implementation starts separately through existing round controls. The round carries exact approved artifacts and revision provenance into its workspaces. Decisions and review dispositions are recorded in Beads.

The existing navigation, typography, colors and Material 3 components are retained. The added change card leads to a detail surface with expandable artifacts, findings, work sections and history. Feedback survives dismissal, process replacement and folding. Active and stale rounds remain reachable; closed sections are excluded from new round selection. Verification, final build, human acceptance and landing remain distinct.

## Verification

| Check | Result |
| --- | --- |
| OpenSpec strict validation | All three active changes pass. |
| Go static checks | `go vet ./...` passes. |
| Whole-module compilation | `go test ./... -run '^$'` passes. |
| Workflow tests | 31 tests plus subcases pass with the race detector, using deterministic author/reviewer/Beads/validator fixtures. |
| Execution/API tests | Race-enabled `TestSpec` tests in rounds, server and daemon pass. |
| Existing affected core packages | Race-enabled Claude, run manager and Beads client suites pass. |
| Android unit tests | 120 tests pass; zero failures, errors or skips, including 11 new workflow tests. |
| Android lint | `lintDebug` passes. Existing warnings remain; the pre-existing notification permission error was fixed with explicit `SecurityException` handling. |
| Android release | `assembleRelease` passes; APK signature v2 verifies and package/version metadata matches the intended artifact. |
| Signed APK smoke test | The exact release APK installs, pairs with an isolated fixture, renders the change and review, submits the exact revision for approval, and shows Ready to implement. Android crash log is empty. |
| Bridge build | Native `go build -trimpath` succeeds; the built executable's `version` command reports the expected version. |

Saved check output is under [dist/evidence](../dist/evidence). The full Git-backed Go suite was not run; the selected tests deliberately exclude real worktree/merge/landing operations. A broader existing configuration test run also passed; those tests read the global Git username. No feature branch, commit, checkout or repository mutation through Git was performed.

## Interface evidence

An isolated preview package on the foldable emulator exercised the real Android UI against a loopback fixture. It verified folded/unfolded layouts, keyboard access, preserved feedback, stale approvals, conflict handling and older-bridge fallback. The fixture captured explicit approval followed by a separate round request with the exact approved section IDs, change ID, revision and budget. It refused execution, so this test did not launch AI implementation or mutate production project data.

- [Unfolded bead detail](../dist/evidence/unfolded-card.png)
- [Change detail](../dist/evidence/unfolded-change.png)
- [Folded keyboard and feedback](../dist/evidence/folded-keyboard-fixed.png)
- [Unfolded keyboard and feedback](../dist/evidence/unfolded-keyboard-fixed.png)
- [Approved section selection](../dist/evidence/folded-round.png)
- [Stale approval](../dist/evidence/stale.png)
- [Older bridge](../dist/evidence/old-bridge.png)
- [Captured mutation requests](../dist/evidence/requests.jsonl)

The preview package, fixture server and emulator reverse port were removed afterward. The original app's data was preserved.

The exact signed release APK was also installed into a separate disposable API 36 foldable emulator. It rendered the [reviewed change](../dist/evidence/release-change.png), submitted an [approval request with the exact revision](../dist/evidence/release-requests.jsonl), and displayed [Ready to implement](../dist/evidence/release-approved.png). The [crash log](../dist/evidence/release-crash-log.txt) was empty. A System UI startup timeout was dismissed before pairing; no Nacre crash occurred. The disposable emulator and its fixture server were shut down and the temporary AVD removed after verification.

## Remaining boundary

The session's explicit instruction, "Git authority: no git operations in this context," prevented creating the requested feature branch. Source changes were therefore made in an isolated copy. **NAC-mp6.6** tracks transferring the reviewed patch onto a feature branch and running the real Git-backed round/workspace/landing tests. The parent **NAC-mp6** remains open for that work.

The paid AI author/reviewer calls and a complete production proposal-to-landing cycle have not been exercised live. Compilation, fixture tests and emulator screens establish the listed behavior, not production success. Landing, final acceptance, epic closure and OpenSpec archival remain explicit owner actions. The implementation does not automatically close the epic or archive a change.
