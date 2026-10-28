## Why

Updating nacred on several paired Macs currently requires visiting each computer. Settings should offer one action with an honest result for every computer while protecting work already running there.

## What Changes

- Add Update connected computers beside the existing computer controls, with per-computer progress, offline/older-daemon explanations and verified results.
- Add authenticated asynchronous update/status endpoints. Select the latest published stable release from the fixed Nacre GitHub repository, resolve its tag to a commit, download and build that exact source independently of working checkouts.
- Refuse updates while runs, rounds, assistant turns or specification jobs are active. Reserve maintenance before starting and reject new mutations until completion.
- Use a separate LaunchAgent helper to preserve rollback, replace the executable atomically, restart, and confirm authenticated health/version/commit before reporting success. Restore the previous executable if verification fails.

## Capabilities

### New Capabilities
- `daemon-updates`: Coordinated Settings requests and verified per-computer stable-release installation.

### Modified Capabilities
None. Existing round and specification contracts remain unchanged.

## Impact

Android Settings, repository/client models; nacred server, version metadata, maintenance coordination and a macOS updater helper. The helper requires Go and authenticated GitHub CLI on each Mac. Older bridges need one manual upgrade to gain the endpoint. No published releases currently exist (GitHub API checked 2026-10-28); absence is a clear result, never an implicit main-branch update.

Out of scope: publishing a release, updating this phone, forced interruption, arbitrary repositories/commands, unattended scheduling, non-macOS service managers, and live production installation during development.
