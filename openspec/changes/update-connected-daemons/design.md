## Context

Each paired Mac already runs a bearer-authenticated per-user LaunchAgent. Settings uses Material 3 list rows and tonal actions. No accepted specification covers updates; the active round and proposal changes must not be interrupted.

## Goals / Non-Goals

Deliver a single Settings action with independent, recoverable per-host results. Preserve existing design, pairing, configuration and checkouts. Do not publish releases, update mobile packages, force a busy daemon to stop, or execute an update on production during development.

## Decisions

1. Resolve GitHub's latest stable release in the fixed `ryderdavid/nacre` repository through the computer's authenticated `gh` CLI, then resolve its tag to a commit. Build an isolated source archive of that commit with explicit version/commit metadata. Never accept a repository, command or ref from the phone. Downloading main was rejected because it is not a published release. No releases means a clear failure result.
2. Reserve maintenance atomically with mutation admission. Refuse if any mutation request, owned run, nonterminal round or unfinished round cleanup, assistant turn or specification job exists. Reject new mutations while updating; keep authenticated reads and status available. Busy hosts return a reason and require a later explicit retry.
3. Use a separate, fixed-label LaunchAgent helper copied from the running binary. It survives replacement of the service, writes owner-private atomic status, keeps the previous executable, and health-checks exact version, commit and host identity. Failure after replacement restores and verifies the previous executable. A bounded operation prevents a dead helper appearing active forever.
4. The phone starts all currently reachable hosts independently and polls authenticated status. GET reconciles an operation after navigation or process restart. A preflight GET records the existing operation ID before POST; an ambiguous POST is accepted only when GET observes a new operation ID, so historical success cannot confirm a new request. A disconnect is reconnecting, never success. Older daemons explain the one-time manual update requirement. Existing host rows show version and progress; the action uses the current tonal button style.

## Risks / Trade-offs

- No published release exists today → show that result and document the publishing prerequisite; do not silently install main.
- Older daemons have no endpoint → one manual bootstrap is necessary.
- The updater executes trusted upstream source using Go and gh → fixed origin, pinned commit, bounded archive extraction, no shell interpolation, no token logging, owner-private staging and explicit build provenance.
- Restart breaks the connection → independent helper verifies and rolls back; phone reports success only after receiving terminal verified status from the same host.
- Nonstandard service installations → refuse with actionable guidance instead of replacing an unrelated executable.
- Helper termination or machine shutdown → deadline exposes interrupted status and manual recovery guidance; never infer successful installation.

## Migration Plan

Ship app and bridge together. Install the bridge once on older Macs. Publish stable tagged releases containing the update protocol. Subsequent Settings actions update idle, reachable Macs. Retain rollback executable after each attempt. Development validates fake release/build/service boundaries and the Android UI; a live production restart requires separate authorization.
