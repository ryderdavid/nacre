## Why

After Ryder confirms Create and develop (or approves the assistant's `then_develop`/`develop_proposal`, or answers a spec-drift question with amend the spec first), the seed's card stays in the Proposals lane with a generic Planning chip. Nothing marks it as a proposal that doesn't exist yet. When drafting stops, the card falls back to Needs attention, and nothing says the proposal never got created. Ryder can't tell that a proposal is on its way, and can't tell when drafting stopped.

## What Changes

- nacred records `created_at` on a change record the first time its authored artifacts pass strict OpenSpec validation, or when a covered disposition is recorded. The field is never cleared, so later revisions don't count as drafts.
- Records saved by an older nacred get a `created_at` when they are loaded: the time of their first history entry, or the current time if there is none. An existing change being revised is never reported as a draft.
- The changes summary exposes a derived `draft` for changes with no `created_at`:
  - `pending` while the phase is `drafting`
  - `failed`, with nacred's message, when drafting stopped in `needs_attention` or `interrupted`. This covers every stop: failure, cost cap, author error, restart and shutdown.
- The changes summary also exposes `started_at`, the time the change record was started (its first history entry). It never changes after that.
- No new phase, event type or cancel action is added. The existing phase-transition events, which carry the change ID, are the signal.
- In the Proposals lane, a change with no created proposal shows as one placeholder with a "Drafting…" pending status, in place of the cards of all its seeds. It shows no title, change name, agent or computer.
- Within each rank group, the lane sorts by `started_at`, newest first, instead of by last update. Creating the proposal therefore doesn't move it: the real card takes the placeholder's position. A draft that becomes failed moves into the waiting group, as any change needing attention does today.
- When drafting stops before the proposal is created, the placeholder turns into an error state, "Drafting failed", with the change's existing Resume and a Dismiss. nacred keeps the error until it is dismissed. Resume makes the placeholder pending again.
- The effort-intake "Proposals lane" requirement is rewritten to cover placeholders. Membership is still derived from change records.

Out of scope:

- a working title, a partial proposal, or the drafting agent or computer
- a separate cancelled state or a new cancel action
- new notifications or new event types
- changes to intake verdicts, review, approval, or how interrupted and Resume work
- placeholders for revisions of changes that already exist
- placeholders outside the Proposals lane
- changing the lane's rank groups

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `effort-intake`: the Proposals lane shows one pending or failed draft placeholder per change whose proposal hasn't been created yet, sorts by when the change was started, and nacred reports when a proposal was created.

## Impact

- nacred, in `bridge/internal/specflow`:
  - `created_at`, `draft_dismissed` and a record format version on change records
  - a load-time backfill of `created_at` for older records
  - derived `draft` and `started_at` in the summary
  - the dismiss endpoint
  - Covered by Go tests.
- API contract: `docs/API.md` documents `created_at`, `started_at`, the `draft` field in the changes summary, and `POST /v1/repos/{repo}/changes/{id}/draft/dismiss`. Both sides change together.
- App: the `SpecChange` model, Proposals lane derivation and ordering (`BoardModel.kt`), and the placeholder card. Covered by JVM unit tests.
- Older bridges send no `draft` or `started_at`, and the lane looks and sorts as it does today.
