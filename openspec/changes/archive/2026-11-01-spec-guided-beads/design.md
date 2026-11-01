## Context

The Android client already displays editable beads and adaptive detail panes. nacred wraps bd, runs structured Claude calls, and owns rounds. OpenSpec 1.4 uses repo-local proposal/design/specs/tasks; Nacre currently has two active changes and no accepted specs. Existing assist sessions are memory-only, and round verification is about gates and commits, not requirement coverage.

## Goals / Non-Goals

Goals: a complete, recoverable path from a seed bead to a reviewed OpenSpec proposal and section beads, an exact-revision implementation handoff, visible review/evidence, and unchanged visual vocabulary. Preserve existing non-spec workflows and CLI interoperability.

Non-goals: multi-repository changes, a second task database, arbitrary agent file writes, automatic landing or canonical-spec archival, proving UX acceptance from compilation alone. This release calls the final stage "Implementation ready for review" until the user can establish landed and accepted behavior; it never calls a proposal complete merely because task boxes are checked.

## Decisions

### Ownership and interface

Use a `specflow` package with a persisted record per repo/seed and authenticated routes:

- `GET /v1/repos/{repo}/issues/{id}/change` -> `{available, reason, change}` (change is nullable; sibling section/epic resolves the same record).
- `POST /v1/repos/{repo}/issues/{id}/change` with `{action, revision, feedback}` -> the same envelope. Actions: `develop`, `revise`, `approve`, `refresh`, `resume`. Long author/review work returns immediately and runs independently of the HTTP request. Approve promotes the reviewed plan to Beads; no code runs as a side effect.

The change carries `id`, `seed_id`, `epic_id`, `change_name`, `phase`, `revision`, `approved_revision`, `summary`, `route`, `rationale`, `references[]`, `artifacts[] {path,content}`, `findings[] {id,severity,summary,detail,disposition}`, `sections[] {id,title,body,bead_id,depends_on[]}`, `history[] {at,kind,text}`, `error`, `cost_usd`, `updated_at`. Phases are `drafting`, `reviewing`, `needs_attention`, `awaiting_approval`, `promoting`, `ready`, `stale`, `interrupted`. UI labels are plain language. These phases are independent of bd status. A separate completion/progress assessment uses linked run/round verification and remains honest about missing evidence.

Store orchestration under the existing nacred data directory, not in the Beads database. Files are atomic and owner-only. Serialize operations per repository; GET is a detached snapshot. On restart, active calls become interrupted and offer Resume. Do not silently repeat a billable call or a partial external write. Repeat start while active returns existing state.

### Reconciliation and artifact authority

Collect bounded, deterministic inventories of openspec/config.yaml, AGENTS.md, accepted specs and every active change (exclude archive), including source paths/content digests. A missing/invalid configuration or unreadable/oversized inventory disables development with an explanation rather than omitting context. The seed includes description, design, acceptance and notes. A writer reads this context and can inspect source read-only; it returns structured artifact content and reconciliation rationale, not shell commands.

Routes: `new`, `amend`, `repair`, `covered`. Existing matches must cite paths actually present in the inventory. `covered` remains a recommendation for the owner and never closes the seed automatically. `repair` references unchanged requirements and creates only the needed work proposal. `amend` targets an active change; compare its baseline bytes before replacing any artifact. Never edit accepted specs here. If that change already has a mapped epic, reuse it; otherwise promote the seed only after review.

Generated paths are a simple change slug plus proposal.md, design.md, tasks.md and specs/<capability>/spec.md. Reject traversal, absolute paths, symlinked ancestors, duplicate paths, unbounded output and any other target. Preserve .openspec.yaml and unrelated artifacts. Write reviewed artifact content through the manager; agents cannot write files. Validate with installed OpenSpec (`validate <change> --strict --no-interactive`); missing CLI is actionable unavailability.

### Independent review and revision

Use existing subscription Headless runner with separate fresh author/reviewer sessions, maximum three author/reviewer passes per action and a total budget cap. Persist each call outcome and cost. Review sees project goals, inventory, seed, artifacts and previous findings, with a rubric for goal alignment, behavior coverage, failure/recovery, feasibility and testability. Reviewer returns goal assessments with source references plus findings. Missing assessments or malformed responses are failure, not approval. Unresolved blockers keep the phase needs_attention; feedback starts another bounded revision cycle. Valid lower-severity findings can remain visible for owner acceptance.

The approved revision is a digest of exact artifacts plus goal/context bytes and referenced baseline requirements. Approval must send this revision; recompute immediately before promotion and before execution. CLI edits or goal changes mark it stale and require a new review. Amendments while linked implementation is active are refused with the existing round link. Editorial changes may conservatively invalidate approval in this release.

### Mapping and Beads decision record

Default one child per task section; its body contains the section checklist, requirement references, required gates and current approved decision summary. No unchecked task can disappear from the section plan; parse tasks.md and validate the returned section coverage. Persistent opaque section IDs are assigned once and retained across rewrites; ambiguous splits/merges request review instead of guessing. Display ordering/heading numbers are never identity.

Before promotion persist intended epic and section operations. Use stable workflow/section labels and bd reads to recover successful creates after a lost response. On retry reuse the same epic/children. Never overwrite a manually edited generated body: store the last generated text and report conflicts. Type promotion preserves seed history; existing mapped change reuses its epic. Persist each returned bead ID before continuing. The workflow cannot become ready until all child/dependency writes and the decision comment succeed. Comments contain phase, revision, route rationale, reviewer disposition and generated-child links. Repeated retries must not manufacture duplicate decision comments.

### Execution and verification

Server hooks resolve spec-linked beads by persistent mappings and labels. Generic single-run and round creation must reject an unapproved/stale/missing workflow or an unmapped linked bead. The epic is an orchestration entry point, not an implementation task. Existing StartRound receives the exact approved child set; dependency order comes from bd and section dependencies. Each child description includes the approved artifact references and snapshot; round prompts include that contract, not merely a recent-comment tail. A revision is pinned for the duration of a run; no background proposal edit may change its meaning.

Observe round/bead results as evidence. A checkbox or bd closure is not verification. Show counts for planned sections, mapped beads, passing verified runs, pending human checks and landed rounds separately. Reopened beads or changed artifacts invalidate derived completion. This release does not automatically archive or close the spec epic: it displays the outstanding manual review/landing/archive obligations explicitly. Ordinary non-spec rounds retain their current behavior.

### Android experience

Add one `Change` card to bead detail after the intent fields and before activity. Unlinked bead: short explanation and `Develop a proposal`. Linked bead: phase, concise summary, review/section counts, and one `View change` action. Detail is a full-screen dialog reusing TopAppBar, existing MarkdownText, MaterialTheme, rounded Surface/cards, tonal buttons and the same window-inset treatment; it fits the existing side-pane context without adding a global tab.

Change detail uses a lazy vertically scrollable layout with expandable Proposal, Behavior, Design and Work sections; concise findings and decision history; one primary next action. Feedback is a persistent per-change field with IME padding; dismissing the view does not cancel work. Poll only while the view is observed, preserving the last snapshot on refresh errors. Old bridges return an unobtrusive unsupported explanation; they never crash normal bead detail. "Implement change" opens the existing round selection with exact children and explicit budget, never starts automatically.

## Risks / Trade-offs

- AI can make a superficially complete plan: fresh review, strict validation, task coverage checks and independent verification remain separate gates.
- CLI and app can edit the same proposal: digests detect conflicts; preserve user content and ask for reconciliation.
- Beads writes are not transactional: journal each operation and recover by stable labels before retrying.
- Large projects exceed safe context: fail with the scope/size reason rather than silently claim exhaustive reconciliation.
- A release without automatic archive requires a final CLI step: explicitly show that obligation and keep epic open.

## Migration Plan

### Adversarial review resolutions

Section identity is encoded immediately before each tasks.md level-two heading as `<!-- nacre-section: <opaque-id> -->`. The manager assigns IDs before review and hashing; rewrites preserve known IDs, reject duplicate IDs and return an explicit retired-section list with reasons. IDs are not inferred from headings or numbering.

Revision after promotion preserves all historical mappings. The reviewed diff shows added, changed and retired sections. Approval synchronizes the current sections, adds new children, and reopens affected closed children only after successful body synchronization. Retired sections remain in history with a reason and their existing beads are neither deleted nor automatically closed. They are excluded from future exact execution sets. Conservative revisions can invalidate all prior evidence and reopen current sections; that consequence is visible before approval. Active linked runs/rounds prohibit revision. A canonical index allows only one workflow per repository/change slug; new seeds amending it attach to that existing workflow/epic.

The `covered` route has its own terminal planning phase, `covered`: show the reviewed read-only disposition and citations, never promote or implement automatically. It does not require manufactured change artifacts. The owner can give feedback to dispute coverage. Missing workflow records with workflow labels fail closed instead of treating the bead as ordinary work. A linked epic cannot be submitted as a single-bead implementation.

Implementation provenance is explicit: add optional `spec_change_id`, `spec_revision`, `spec_section_id` to Run and RoundBead. Count verification only when provenance matches the current approved section/revision. `evidence` in change detail contains section ID, bead ID, matching run/round IDs, verified/human-check/landed observations and outstanding obligations. Unknown is not false success; original checkboxes remain observations. Any artifact/context change invalidates this assessment until reviewed again.

Before linked round execution, the manager supplies a pinned artifact snapshot. Install that exact snapshot into the integration workspace and commit the proposal-only changes through the existing controlled workspace mechanism, before creating bead workspaces. Verify digests in bead workspaces before the agent starts. Add the complete pinned contract to the prompt and persist it with the round; the approved proposal must be present in the final build commit. Generic single-bead execution of linked work is refused with "Start a specification round" so it cannot bypass workspace snapshot installation. The existing round controls can run one or all approved sections. The same guard applies when a round is resumed/retried; source changes never silently rewrite a pinned round.

Additive API and opt-in flow. Existing boards/runs work without workflow state. Build both bridge and APK; use isolated fixtures for orchestration and a debug/test connection for UI. Do not restart production nacred as part of this implementation. Removing the feature leaves OpenSpec files and ordinary Beads epics/tasks usable from CLI.

## Open Questions

No user input required for implementation. Defaults: three review passes per action, finite budget, explicit owner approval for promotion, no automatic landing/archive, one section per child, and conservative stale detection.
