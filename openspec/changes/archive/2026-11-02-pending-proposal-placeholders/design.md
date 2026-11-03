## Context

What exists today, confirmed in the source:

- `drafting` is already one of the app's planning phases (`SpecChange.PLANNING_PHASES`). A seed whose change is being drafted already appears in Proposals as an ordinary card with a Planning chip.
- nacred saves a failed workflow operation as `needs_attention` or `interrupted`, with an error message (`specflow/manager.go`). A restart makes the change `interrupted`, with Resume.
- Phase transitions are published as events with the change ID (spec-surfacing "Spec notifications").
- Change operations have no cancel endpoint. specflow's only cancellable context is the manager-wide one, which shutdown cancels.
- The Proposals lane is sorted by rank (the WAITING set in `SpecChips.kt` first), then by `updatedAt` descending (`BoardModel.kt`).
- A change can have several seeds (`seedIds`, spec-guided-work "Second seed for the same change").

What's missing:

- A way to tell "no proposal exists yet" from "an existing change is being revised". Both are `drafting`.
- A failure before creation that stays visible as a draft error until Ryder dismisses it.
- A placeholder that shows no details.
- A sort key that doesn't change when the proposal is created. `updatedAt` moves forward on every save.

## Decisions

1. **Reuse phases and events.** No separate draft state machine, no new event types and no cancel action. The draft ID is the change record ID, which the develop response and the phase events already carry.
2. **`created_at` marker.**
   - nacred sets it once, the first time the author's artifacts pass strict validation or a covered disposition is recorded. It is never cleared.
   - Records adopted from existing changes count as created.
3. **Backfill for older records.**
   - Change records gain a record format version.
   - When nacred loads a record with no format version and no `created_at`, it sets `created_at` to the time of the record's first history entry, or to the current time if there is none. It then saves the record with the current version.
   - Backfill runs once per record. Loading again changes nothing.
   - Records created by this version always carry the format version, so a new draft is never backfilled.
4. **Derived `draft` in the summary.** It is present only when `created_at` is empty and the change is in a planning phase:
   - `drafting` gives `pending`.
   - `needs_attention` or `interrupted` gives `failed`. Every stop before creation counts as failed: an author error, the cost cap, invalid output, a restart, an interruption or shutdown. There is no separate cancelled kind.
   - The `message` is the change's existing error text.
   - If `draft_dismissed` is true, a failed draft is hidden.
5. **Stable order.**
   - The summary carries a derived `started_at`: the time of the record's first history entry, or `created_at` when there is no history. It is not stored and never changes after that.
   - Within each rank group, the lane sorts by `started_at`, newest first, instead of `updatedAt`. The rank groups are unchanged.
   - Pending and created-but-not-approved phases (`drafting`, `reviewing`, `promoting`) share a rank group, so creating the proposal doesn't move the item.
   - A draft that becomes failed is in `needs_attention` or `interrupted`, so it moves into the waiting group like any change needing attention today. Resume puts it back at its `started_at` position among pending items.
   - If `started_at` is missing (an older bridge), the lane falls back to `updatedAt`, which is today's order.
6. **Placement.**
   - The lane is keyed by change ID.
   - A change with a `draft` is drawn as exactly one placeholder, and the cards of all its seeds are hidden. That covers changes with several seeds.
   - When `draft` disappears, the change's card or cards take the placeholder's position.
7. **Content.**
   - Pending: a progress indicator and "Drafting…". No seed title, change name, working title, agent or computer. The placeholder can't be dragged and opens nothing. The lane item model exposes `title = null`, `clickable = false` and `draggable = false`, so JVM tests can check this.
   - Failed: the error colour, "Drafting failed", nacred's one-line message, Resume when the change offers it, and Dismiss.
8. **Transitions.**
   - Resume, or a new develop request, returns the change to `drafting` with `created_at` still empty, so the placeholder shows pending again.
   - A new develop request clears `draft_dismissed`.
   - Dismiss, `POST /v1/repos/{repo}/changes/{id}/draft/dismiss`, sets `draft_dismissed` and is idempotent. Calling it on a pending or created change changes nothing and returns the current state.
   - After dismissal, the seed shows its ordinary card with the existing Needs attention banner and Resume.

## Failure and recovery

- Lost events: the next summary refresh rebuilds placeholders from `draft`.
- A phase event for a change that isn't in the summary: the app refreshes the summary instead of guessing a slot.
- Dismiss fails: the error placeholder stays, and a snackbar shows the reason. Retrying is safe.
- Resume is refused: the existing refusal message is shown, and the placeholder stays failed.
- Backfill can't be saved: the loaded record still counts as created in memory, so no draft is reported. The save is retried on the next write. If nacred restarts before then, a record with no history may be backfilled again with a different time. Nothing visible changes.
- Older bridge without `draft` or `started_at`: no placeholders, and the lane looks and sorts as it does today.

## Risks

- Today a drafting seed's card shows its title. With this change it doesn't, until the proposal is created. The seed asked for no partial details, and the bead can still be reached from search and bead links. Revisions of changes that already exist are not affected.
- Within a rank group, other proposals now sort by when they were started instead of their latest activity. A long-running change that was revised recently no longer rises to the top of its group. Waiting items still come first.
- A first draft already in progress when nacred is upgraded is backfilled as created. It shows today's ordinary card instead of a placeholder until it finishes. This is acceptable for a one-time upgrade, and it never misreports a revision as a draft.
