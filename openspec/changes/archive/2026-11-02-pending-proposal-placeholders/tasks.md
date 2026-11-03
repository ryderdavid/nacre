<!-- nacre-section: sf53469021131e7bb82cbd -->
## 1. nacred created marker, backfill and derived draft

- [ ] 1.1 In `bridge/internal/specflow`, add `created_at` and a record format version to change records. Set `created_at` once, when the first authored artifacts pass strict validation or a covered disposition is recorded. Treat adopted records as created.
- [ ] 1.2 On load, backfill records that have no format version and no `created_at`. Use the time of the first history entry, or now if there is none, then save with the current version.
- [ ] 1.3 Derive the summary fields:
  - `draft` for planning-phase changes without `created_at`: `pending` in `drafting`; `failed` with the change's error in `needs_attention` or `interrupted`; hidden when `draft_dismissed` is set
  - `started_at`: the time of the first history entry, or `created_at` when there is no history
- [ ] 1.4 Add the idempotent `POST /v1/repos/{repo}/changes/{id}/draft/dismiss`. Clear `draft_dismissed` when a new develop request is accepted.
- [ ] 1.5 Go tests:
  - pending → created
  - pending → failed through author error, cost cap, restart and shutdown
  - failed → Resume → pending
  - revision of a created change reports no draft
  - an older record in drafting is backfilled from its first history entry and reports no draft
  - an older record with no history is backfilled with the load time
  - a new record without `created_at` is not backfilled
  - backfill is stable across a second load
  - `started_at` is unchanged by later saves, including setting `created_at`
  - dismissing twice
  - dismissing a pending or created change returns the current state unchanged
  - a new develop after dismissal clears `draft_dismissed` and reports pending
  - summary rebuilt without events
- Gate: `cd bridge && go vet ./... && go test ./...`

<!-- nacre-section: sf4a1a5708d1b893b808f4 -->
## 2. API contract

- [ ] 2.1 In `docs/API.md`, document:
  - `created_at` and its backfill for older records
  - `started_at` and that it never changes
  - the summary `draft` field (`pending` or `failed`) and how it is derived
  - the dismiss endpoint
  - that phase events remain the signal
  - older-bridge behavior
- Gate: `openspec validate pending-proposal-placeholders --strict`

<!-- nacre-section: sfbbc02f9d63a8a4461aa7 -->
## 3. App placeholder in the Proposals lane

- [ ] 3.1 Parse `created_at`, `started_at` and `draft` into `SpecChange`. Missing fields mean there is no draft and the sort falls back to `updatedAt`.
- [ ] 3.2 In `BoardModel.kt`, build the Proposals lane keyed by change ID:
  - Sort within rank by `started_at` descending, falling back to `updatedAt`.
  - Draw one placeholder per change with a draft, and hide the cards of all its seeds.
  - Refresh the summary on a phase event for an unknown change.
- [ ] 3.3 Draw the pending placeholder with the existing Material 3 card components: a progress indicator and "Drafting…". No title, change name, agent or computer. The lane item model exposes `title = null`, `clickable = false` and `draggable = false`.
- [ ] 3.4 JVM tests in `ProposalsLaneTest`:
  - placeholder present and seed card hidden
  - a change with two seeds → one placeholder and neither seed's card
  - the placeholder item has no title or change name and is neither clickable nor draggable
  - created with a newer `updatedAt` → one real card in the same position, with other items unmoved
  - revision of a created change → ordinary card
  - older bridge without `started_at` → today's `updatedAt` order and unchanged lane
  - unknown-change event → refresh
- Gate: `cd android && ./gradlew testDebugUnitTest`

<!-- nacre-section: sfcd4dde95393e2c1c877f -->
## 4. App error state, Resume and dismiss

- [ ] 4.1 Draw the failed placeholder in the error colour with "Drafting failed", nacred's message, the change's existing Resume when offered, and Dismiss.
- [ ] 4.2 Wire up Dismiss and Resume:
  - Dismiss calls the endpoint and refreshes. On error, keep the placeholder and show the reason in a snackbar.
  - Resume uses the existing action. A refusal keeps the placeholder failed.
- [ ] 4.3 JVM tests:
  - a failed draft shows the error state rather than disappearing, and sits in the waiting group
  - Resume → pending back at its `started_at` position
  - dismissing shows the seed's ordinary card with Needs attention
  - a failed dismiss leaves the error placeholder in place
- Gate: `cd android && ./gradlew testDebugUnitTest`
