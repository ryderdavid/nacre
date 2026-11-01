# impl-validator Report

Reviewed 2026-10-28 by an independent adversarial reviewer using the
`impl-validator` skill. Review was limited to source inspection and fixture tests;
the reviewer made no implementation edits.

**Goal:** Provide one Settings action that asks every connected computer to update
its idle nacred service to the latest published stable release, with independent,
verified results and rollback protection.

## Checks

| Check | Result | Evidence |
|---|---|---|
| Authenticated, fixed-purpose API | PASS | Both endpoints require bearer authentication. POST accepts no source, ref, command or other options. Duplicate active requests return the existing operation. |
| Idle-work admission | PASS | Mutation admission and update reservation share a lock. Owned active runs, nonterminal rounds, assistant turns, specification jobs and terminal-round background cleanup prevent updates. New mutations are rejected while an operation is active. |
| Trusted pinned release source | PASS | GitHub host and repository are fixed. Stable release tags are validated and resolved to an exact SHA. The archive download uses that SHA; extraction bounds size/count, rejects traversal and links, and only extracts bridge source. The isolated build reports matching version, commit and update protocol before installation. |
| Independent restart and rollback | PASS | A separate fixed-label LaunchAgent runs a private copy of the old daemon. The service identity is checked before dispatch. Atomic executable replacement follows candidate preparation and rollback backup. Failed restart or health verification restores and checks the old executable with a separate recovery timeout. These paths were exercised with fake service operations. |
| Persisted status and reconnection | PASS | Status is atomically persisted outside the service. The helper requires authenticated exact version, commit and host identity before writing success. Android checks the host/build, reattaches by GET, and uses a pre-POST operation snapshot to reject a historical result after an ambiguous POST. Timeouts and interruptions never imply success. |
| Existing interface alignment | PASS | Settings uses the existing computer rows, typography, color scheme and tonal button treatment. Each computer shows its own progress or actionable failure. The explanatory text states that idle daemons restart and busy computers retain their work. |
| Production execution proof | WARN | No real service update, LaunchAgent restart or production rollback was executed. The production release list was reported empty by the implementation owner; publishing a compatible stable release remains a prerequisite. |

## Resolved blocking findings

1. **A lost POST could reuse an old successful operation.** The original Android
   coordinator accepted any subsequent terminal result when it did not know the
   new operation ID. A POST that never arrived could therefore display yesterday's
   success. The coordinator now reads the previous operation before requesting an
   update, attaches to already-active work, and requires a new nonempty operation
   ID for a new request. `lostPostCannotReuseHistoricalSuccessfulOperation` covers
   the failure case.

2. **Terminal round state did not imply idle background work.** A landed round
   could still be closing specification children, and a cancelled round could
   still have an operation or label/comment cleanup in flight. Update admission
   previously inspected only nonterminal round state. `rounds.Manager.Busy` now
   also checks a counter reserved synchronously before `goOp` and `async` work,
   released after completion. `TestDaemonUpdateWaitsForTerminalRoundCleanup`
   covers blocked operations and side effects for both landed and cancelled
   rounds, and the server now uses that method.

## Observed validation

- Race-enabled updater and server fixture tests passed, including successful
  update, no-release/build/backup/install failure, rollback/recovery failure,
  duplicate start, persisted operation recovery, no downgrade, archive
  confinement, fixed-source requests, candidate protocol checks and exact
  authenticated health.
- Race-enabled rounds and server `TestDaemonUpdate` tests passed after the
  terminal cleanup admission fix.
- The current Android `DaemonUpdatesTest` report contains seven passing tests,
  including the new historical-operation regression, independent mixed-host
  outcomes, reconnect behavior and rejected mismatched host/build results.

The initial local health fixture hit the sandbox's loopback listener restriction;
the same fixture-only test command passed after approved host execution. No Git
commands, real update installation or production service restart were run.

## Overall: WARN

No source-level blocking findings remain. Proceed with the build handoff while
keeping the production verification limit explicit. Source/fixture correctness
does not establish that a real published release has been installed, restarted,
verified or rolled back on a user's computer. UI review here was source-based;
emulator inspection and release-build results belong to the implementation owner.

## Final Settings refresh check

The reviewer also inspected the final `refreshUpdatedComputer` correction after
the UI walkthrough exposed a stale cached daemon version. No blocker was found:
it uses the authenticated paired client after verified completion, rejects an
obsolete pairing instance or different host identity, preserves cancellation and
leaves durable update status untouched after transient health failure. This
follow-up was source inspection only; the implementation owner verifies the APK.
