# Nacre 0.8.1 — connected computer updates

Built on 2026-10-28 in the isolated source at
`/Users/ryder/Code/HomeAgentic/nacre-spec-workflow`.

## Deliverables

- [Signed release APK](../dist/Nacre-0.8.1-updates.apk): `com.ryderdavid.nacre`, version 0.8.1, version code 15, using the existing sideload signing configuration.
- [Companion macOS bridge](../dist/nacred-0.8.1-updates): version `0.8.1-updates`, update protocol 1. No source commit is claimed for this isolated build.
- [Cumulative source patch](../dist/Nacre-0.8.1-updates.patch), [changed files](../dist/0.8.1-changed-files.txt) and [checksums](../dist/SHA256SUMS-0.8.1).
- [OpenSpec proposal](../openspec/changes/update-connected-daemons/proposal.md), [design](../openspec/changes/update-connected-daemons/design.md), [behavior specification](../openspec/changes/update-connected-daemons/specs/daemon-updates/spec.md) and [independent adversarial review](../openspec/changes/update-connected-daemons/review.md).

This build also includes the earlier specification workflow described in
[its verification report](SPEC_WORKFLOW_VERIFICATION.md). Its earlier artifacts
and checksum manifest are preserved separately.

## Behavior

Settings → Your computers → **Update connected computers** requests an update
on each reachable paired computer. Each existing computer row shows its daemon
version, progress and result. Busy computers retain their work and explain why
they cannot update. Older daemons explain the one-time manual upgrade needed to
enable the action. The screen keeps the existing Material 3 components, colors,
typography and navigation, including folded and unfolded layouts.

The authenticated daemon chooses the latest published stable release from the
fixed `ryderdavid/nacre` GitHub repository, resolves its tag to an exact commit,
downloads bounded source and builds in private staging. It verifies the candidate's
version, commit and update protocol before replacement. A separate LaunchAgent
helper preserves a rollback executable, restarts the service and verifies
authenticated health against the expected host and build. Failure after replacement
restores and verifies the previous executable. Durable operation status supports
reconnection; an ambiguous request, timeout or interrupted helper never implies
success. Active runs, rounds, terminal-round cleanup, assistant turns, specification
jobs and admitted mutations prevent updates.

## Verification

| Check | Result |
| --- | --- |
| Android JVM tests | 127 pass; zero failures, errors or skips. Seven update tests cover mixed results, ambiguous POST recovery, historical-operation rejection, duplicate requests, reconnection, mismatched identity/build and timeout. |
| Android lint and release | `lintDebug` and `assembleRelease` pass. APK signature v2 verifies; package/version metadata matches the artifact. |
| Updater and affected Go packages | Race-enabled daemonupdate, assist and specflow suites pass. |
| Admission and execution regressions | Selected race-enabled `TestDaemonUpdate` and `TestSpec` tests in rounds, server and daemon pass. Terminal landed/cancelled operations remain busy through background completion. |
| Go static checks and compilation | `go vet ./...` and compile-only `go test ./... -run '^$'` pass. The native bridge builds and reports update protocol 1. |
| OpenSpec | All four active changes pass strict validation. |
| Signed release UI | Installed in a disposable API 36 foldable emulator; three fixture computers exercise independent success, busy-work rejection and old-daemon explanation. Progress and result layouts remain readable folded and unfolded. |
| Reconnection | Reopening the release app restores the completed operation through status reads, without another update POST. |
| Source patch | Applies cleanly to an isolated reconstruction of the original files and reproduces the changed source bytes. |

Saved output is in [dist/evidence/daemon-updates](../dist/evidence/daemon-updates).
The exact release walkthrough uses only a local HTTP fixture. It does not download,
build, restart or replace a real daemon. Fixture request capture contains one empty
POST per eligible host; the older host is detected by preflight GET and receives no
POST. A final UI correction refreshes authenticated daemon health after successful
completion so the version beside the computer updates immediately.

- [Settings before the request](../dist/evidence/daemon-updates/settings-before.png)
- [Per-computer progress](../dist/evidence/daemon-updates/settings-progress.png)
- [Independent results](../dist/evidence/daemon-updates/settings-results.png)
- [Folded layout](../dist/evidence/daemon-updates/settings-folded.png)
- [Restored result after relaunch](../dist/evidence/daemon-updates/settings-reopened.png)
- [Captured update requests](../dist/evidence/daemon-updates/requests.jsonl)
- [Release crash log](../dist/evidence/daemon-updates/release-crash-log.txt)

The release crash log is empty. After verification, the fixture server and
disposable emulator were stopped and the temporary AVD removed. The existing
emulator and physical tablet were not altered.

The full Go suite was not run because existing tests perform Git operations.
Selected tests use isolated fixtures and fake service operations. The production
daemon, existing app installations and original source checkout were not changed.

## Shipping prerequisites and remaining verification

Older daemons require one manual bootstrap to this updater-capable bridge. The
initial implementation supports the standard per-user macOS LaunchAgent install,
with Go and authenticated GitHub CLI available to that service. Nonstandard
installations report their limitation rather than replacing another executable.

The authenticated release listing was empty during this work. A compatible stable
release must be published before this action can install a release; the updater
reports missing releases and never silently falls back to `main`. No release was
published, and no real update, LaunchAgent replacement or production rollback was
executed. **NAC-iho.1** tracks that verification on an explicitly chosen idle Mac.

Implementation and fixture verification are recorded in **NAC-iho**. The session's
explicit instruction, "Git authority: no git operations in this context," still
prevents creating the originally requested branch. The cumulative patch is ready
for transfer; **NAC-mp6.6** retains the branch and real Git-backed workflow checks.
