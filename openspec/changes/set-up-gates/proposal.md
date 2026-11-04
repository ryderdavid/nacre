## Why
Rounds refuse to run in a repository without `nacre.yaml`, and the only way to add one was writing it
by hand on the computer. From the phone that was a dead end, met only after paying for a proposal and
approving it (draw-things-android, dta-sa4).

## What Changes
- nacred drafts `nacre.yaml` with one read-only Claude call (up to $1.50), given this computer's
  branch, remotes, JDKs, SDKs and tools and the other repositories' files as examples.
- nacred saves an approved file and commits it alone on the checkout's branch, never overwriting one.
- The app offers Set up gates wherever work would start in a repository without gates: Start work,
  Run epic, Approve and run, and the Start work sheet. After saving, the action goes on.

## Capabilities
### New Capabilities
None.
### Modified Capabilities
- `repo-gates`: nacred can draft and save `nacre.yaml`.
- `round-app`: starting work without gates leads to setting them up instead of a disabled action.

## Impact
`bridge/internal/server/repoconfig.go`, `docs/API.md`, `android/.../ui/rounds/GatesSetupSheet.kt`,
`StartRoundSheet.kt`, `BeadDetail.kt`, `ChangeDetail.kt`, the bridge client and repository.
