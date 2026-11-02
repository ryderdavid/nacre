## Why

The NAC-1tn walkthrough found that starting a round from a searched or epic-filtered board listed every open bead again, that a ready-to-land round showed its build and commit only as text with no way to see what it changes, and that the landing dialog said it pushes "if nacre.yaml says to" instead of saying whether this round will push (NAC-3mc).

## What Changes

- Starting a round from a board opens with the board's search and epic, type or needs-you scope; the sheet has its own search, and the scope can be turned off. Beads already picked stay picked when a search hides them, and the sheet says so.
- nacred resolves `land.push` from the round's nacre.yaml into the round (`land_push`), and adds `GET /v1/rounds/{round}/changes` listing the commits and per-file line counts the result adds to the round's base.
- A ready-to-land round can show those changes before landing, shows its build's path on the computer as copyable text, and states whether landing pushes; the confirmation names the push behaviour ("Fast-forward main and push", or "Fast-forward main, don't push"). An older nacred that doesn't resolve push keeps the conditional wording.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `round-app`: Start a round from a board keeps the board's context; Land offers a review of the changes and resolves push behaviour.

## Impact

Android round setup sheet and round screen; nacred round model (`land_push`) and one read-only round endpoint. No change to how landing itself works.
