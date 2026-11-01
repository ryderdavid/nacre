## Context

`spec-guided-beads` (0.8.1) gives every bead a Change card after Design/Notes, relabels a linked bead's
main button "View change", opens a full-screen change view, and offers a spec-only round sheet. Nothing
appears on the Boards screen, on bead cards, in the assistant, in preflight or in notifications.

Behavior this design is fitted to, from Ryder's Nacre sessions and board (83 beads: 33 features, 32
tasks, 10 bugs, 4 epics, 3 chores, 1 spike):
- He drives work by asking the board assistant and approving its write proposals; manual multi-step UI
  is rare.
- He asks "What should I work on next?" and "Which beads need my decision?" repeatedly, and once could
  not tell what a waiting run was asking.
- Rounds are usually two beads with a $25 budget, started from an assistant `start_round` proposal.
- Drift happens in behavior-changing features, not in small fixes.

Existing pieces this reuses: `Nacre.colors.needsYou` and `Round.needsYou`; `RunNotifier`'s round channel;
the assistant's approval cards and auto-apply mode; preflight's finding → question machinery; specflow's
bounded inventory of `openspec/config.yaml`, `AGENTS.md`, accepted specs and active changes.

Mockups of every surface in this design: `docs/mockups/surface-spec-workflow/` (PNG and HTML per
screen, with a README mapping each to its requirement).

**Amendment context (2026-11-01, NAC-mp6.7.1).** Ryder researched how others combine OpenSpec and
Beads and asked that every effort on a board start as a change proposal, with the system deciding new
versus amendment. The evidence is thin and community-built, but consistent:
- OpenSpec as the contract for what to build, Beads as the record of the work (`openspec-to-beads`
  skill; Beads discussion #240; OpenSpec discussion #1534, formerly issue #245, with no native
  integration yet).
- `tasks.md` is compiled into beads once, then either deleted (one first-hand report, with SpecKit),
  regenerated from the epic (`beads-plan view`), or left with an audit record of what was transcribed
  (Turma's `TRANSCRIBED.md`).
- Discovered work is triaged without silently widening the claimed issue: in-scope fix, in-scope task,
  or out-of-scope follow-up; spec gaps update OpenSpec first (the boardthatpowder skills).
- A change is archived when its epic has no open required work and validation passes.

specflow already does the import with more safety than those tools: stable section ids, journaled
promotion, label-based recovery, idempotent retries and a decision comment as the audit record. So
this amendment adopts the rules, not the tools. Deleting `tasks.md` is not an option with OpenSpec:
`validate` and `archive` read it.

## Goals / Non-Goals

**Goals:** proposals start from where Ryder already decides (the assistant, preflight, New effort);
everything that waits on him is in one self-explanatory list; the board tells planned, specified and
verified apart at a glance; a landed change is reported ready to archive; discovered work is triaged by
one rule. Keep the existing visual vocabulary and navigation.

**Non-Goals:** automatic proposal development; a new tab; group-by-change layout; changes to how
proposals are reviewed, approved or promoted beyond the additions listed under Modified Capabilities
(an `intake` field on `develop`, a `no_inherit_labels` create parameter, found-bead bookkeeping on
`revise`, one more archive-readiness condition); partial archive; archive from the phone (NAC-mp6.8).

## Decisions

### 1. Proposals start from assistant approvals, and spending tools always ask
`develop_proposal {repo, id, feedback?}` calls the existing `POST …/issues/{id}/change` with
`action: develop`. There is no per-call budget: specflow already caps each action at
`MaxActionCostUSD` ($5), and the approval card states that limit and what the bead will be compared
with (counts of accepted specs and active changes, from a new `inventory` summary on the change GET).

New beads can't be referred to by a later call in the same turn (the API backend runs calls in order
with literal inputs; the Mac backend returns all proposals in one answer). So `create_bead` gains an
optional `then_develop: true`. One approval card shows both; applying it creates the bead, then calls
`develop` with its id. If `develop` is refused, the bead stays created and the card reports why.
Declining the card declines both; to create without developing, Ryder edits the proposal or asks again.

`ToolSpec` gains `spends: Boolean`. `BoardAssistant.execute` asks for approval for any spending tool
even when auto-apply is on. Only `develop_proposal` and `create_bead` with `then_develop` are spending
in this change; `start_round` keeps its current behavior (out of scope here, noted for Ryder).

The system prompt tells the assistant when to offer it: features or epics whose intent changes behavior
an accepted spec or active change covers, or when Ryder asks. Never for chores, small fixes or beads
already linked.
*Alternative:* a label that auto-starts development. Rejected: beads are created quickly from chat and
each proposal makes paid calls nobody asked for.

### 2. Preflight gets a bounded requirement index and a `spec-drift` finding
specflow exports `RequirementIndex(repo)`: capability, requirement title, path and line for accepted
specs and active changes, at most 24 KB, truncated by whole capabilities with the omitted ones listed.
A repository whose OpenSpec setup is missing or invalid gets no index and preflight skips spec drift
silently, as today. New finding kind `spec-drift` must cite a path and requirement from the index;
uncited findings are dropped. Like every preflight question, it holds the round in "needs decision"
until answered; nothing about per-bead question gating changes. Answers:
- **Amend the spec first** — the bead leaves the round as in "leave it out" below, and nacred calls
  `develop` for it with feedback naming the requirement. If `develop` is refused
  (repository busy, record already exists, unavailable), the question reopens with that reason, the
  way a failed merge choice does today, and the bead stays in the round until answered again.
- **Proceed as an exception** — the answer is posted to the bead as a decision comment and the bead is
  labelled `spec-touch:<capability>`.
- **Leave it out of this round** — a new deterministic answer handled by `applyDecisions` (today it
  acts only on unmerged-work answers): the bead becomes `BeadSkipped` with reason "Left out: spec
  drift against <requirement>", a note is posted to it, and beads that build on it are skipped the way
  `maybeStart` already skips beads whose prerequisite was parked (`prerequisites` treats skipped like
  parked); the skip reason is reworded so it says "was left out" rather than "was parked".
These three options are fixed by nacred, not model-written free text.
Spec section beads are skipped: their contract is the pinned snapshot. `spec-touch:` is deliberately
not a `specflow:`/`spec-section:` prefix, so it never makes a bead linked or blocks ordinary runs.

### 3. Needs you: computed by nacred, merged by the app, injected for the Mac assistant
`GET /v1/attention` returns items for every repository on that computer:
`{id, kind, repo, bead_id?, change_id?, round_id?, title, why, source_ref?, options[], created_at}`.
`id` is a digest of kind and the underlying record and its state, so the same waiting state keeps its
id and the app can dedupe across refreshes and computers. Each option is either
`{type: "action", label, method, path, body}` (an exact endpoint) or `{type: "open", label, target}`
(navigate: bead, change, round, or one of two sheets with prefill, added for found work in §10:
`{sheet: "found_something", change, found_in, comment_id, note}` and
`{sheet: "new_effort", repo, note, related, follow_on_of, found_comment}`). `why` is one or two plain
sentences that stand alone.

| Kind | Source | Options |
|---|---|---|
| `bead_decision` | open bead with `human` label or `decision` type (`Issue.needsYou`) | open bead |
| `proposal_ready` | change phase `awaiting_approval` | open change at revision; `approve` with revision |
| `review_blocked` | phase `needs_attention` | open change (feedback lives there) |
| `approval_stale` | phase `stale`, or a ready/awaiting record whose live `current()` check fails for any reason | `revise`; open change |
| `workflow_interrupted` | phase `interrupted` | `resume`; open change |
| `round_question` | round `needs_decision`, per open question | the question's answers via `POST /v1/rounds/{round}/answers`; open round |
| `bead_parked` | round bead parked | `…/beads/{id}/retry`; open round |
| `round_ready` | round `ready` | `…/land`; open round |
| `change_archivable` | every section landed with matching evidence (§6) | open change |
| `found_work` | an unanswered `Found:` comment on a spec round bead, or an open `found-amend` bead (§10) | comment: Answer (opens the Found something sheet prefilled, which owns the section picker and the three answers), Dismiss (action); `found-amend` bead: Revise now (action whose body lists every open, not-pending `found-amend` bead of that change, disabled with a reason while work is active), Follow-on change (opens New effort prefilled), open bead |

Building the list reads every ready or awaiting change record, which runs inventory collection and bd
intent reads. Results are cached per repository by inventory digest and bd's last-modified time, and
refreshed at most every 30 seconds.

Human checks on spec sections are already `human`-labelled beads, so they arrive as `bead_decision`;
this change adds no new resolve endpoint. The board's existing "Needs you · n" chip keeps its meaning
(beads needing Ryder on that board) and is a subset of the inbox.

The Boards screen shows Needs you above the summary tiles when there are items, ordered questions,
decisions and found work first, then approvals, then landing and archive, newest first within each, at most five
before "Show all". An empty list or bridges without the endpoint hide it. The Runs tab badge is
unchanged; Boards gets its own badge with the attention count.

Assistant: the API backend gets a `needs_you` read tool. The Mac backend drops reads, so nacred
includes the current attention list (titles, why, ids) in the Mac assistant's prompt context for each
turn, and the prompt tells both to start from it for "what next?" / "what needs me?".

### 4. Card chips come from data the board already loads, plus one summary
`GET /v1/repos/{repo}/changes` returns per change `{id, change_name, phase, epic_id, seed_ids[],
sections[] {id, bead_id, evidence}}`, `evidence` = `none | in_round | verified | landed`, derived by
the existing evidence rules. Evidence is cached per change revision and round-event sequence and
recomputed only when either moves; the endpoint returns at most 50 changes. `BeadCard` shows the spec
chip in the first label slot and the first two other labels after it (`labels.take(2)`):
- seed or epic: phase label (`changePhaseLabel`), plus a section strip on epics;
- section bead: "§ n of <change>" and its evidence;
- spec-linked closed bead without verified evidence: "closed, not verified";
- `spec-touch:` label: "Touches a spec".
Colors: violet tint for spec state, `needsYou` for anything waiting on Ryder, success for verified and
landed.

### 5. Epic detail leads with the change; ordinary beads get a quiet entry
For a change's epic, `BeadDetail` places a change header (name, revision, phase), the section strip
with per-section evidence, open human checks and the primary next action from `changeNextAction`
above Description. Seeds keep the card where it is. Unlinked beads lose the Change card; "Develop a
proposal" moves into the Refine sheet and the overflow menu.

### 6. Stale approvals and readiness to archive, without new actions
**Stale.** `Contract()` already refuses any approval whose inventory digest moved, for any cause: another
change's draft, an accepted spec, AGENTS.md. Today that surfaces as a conflict when a round starts.
specflow now computes the cause when reading a ready or awaiting record: it compares the stored
per-file digests (`record.Baseline`, captured with the revision) with the current inventory and records
`stale_reason` ("AGENTS.md changed", "Active change resilient-rounds changed"). Causes that aren't files
are named too: "Seed bead NAC-x changed" when the intent revision moves, and "Section bead NAC-y was
edited" when `sectionCurrent` fails. Records with no baseline get "The project's specifications changed
since this was approved". Reading sets the phase to `stale` as the existing rules
already do. The item offers the existing `revise`. No re-baseline: a revision change invalidates
section provenance and the `Revision:` line in generated section bodies, and keeping evidence
across it needs the revision-alias design deferred to the archive follow-up.

**Ready to archive.** A change is reported as `change_archivable` when every current section of the
approved revision has evidence from a landed round with matching provenance, and no section bead is
reopened or carries an open `human` label, and no `found:<change-id>` bead under the epic is open (§10). The item opens the change, whose existing obligations list
shows the archive and epic-close steps.

**Deferred** (follow-up change under NAC-mp6): archive from the phone. Its known requirements, from this
change's review: a revision-alias set that `Contract`, evidence and `sectionCurrent` accept; an archive
round kind with its own states (no preflight, no beads, straight to gates and landing) and every bead
guard adjusted; change artifacts committed on the target, or the pinned snapshot installed before
`openspec archive`; and no reliance on a per-repository round mutex, which doesn't exist, with landing
relying on `land()` re-verifying a moved target.

### 7. Change events and notifications
nacred publishes `change` events on `/v1/events` on each phase transition. The repository layer
exposes them like round changes. `RunNotifier` raises "Proposal ready for your review" on the change
into `awaiting_approval` and "Change ready to archive" when an item of that kind first appears, once each,
on the round channel. `RunWatcherService` also watches while any change is drafting, reviewing or
promoting. When nothing is being watched, the app shows the notification on its next refresh rather
than claiming background delivery.

### 8. New effort: a cheap intake verdict before anything is written
In a repository whose OpenSpec setup is valid, `BoardScreen`'s "New bead" button reads "New effort"
and opens `NewBeadSheet` in effort mode. The fields and the Claude draft step are unchanged. Its
primary button becomes "Continue", which calls
`POST /v1/repos/{repo}/intake {title, description, design, acceptance, type, parent}` and returns
`{verdict, change_name?, rationale, references[] {path, requirement}, cost_usd}`, verdict one of
`bead | new | amend | covered`.

Intake is one structured Headless call on the Mac, read-only, with `--safe-mode` and no tools beyond
reading the repository, capped at $0.50 and 90 s. Its context is the requirement index from §2 plus each
active change's name and `## Why` (together within the same 24 KB budget), not specflow's full
inventory. It persists nothing. The verdict follows the boundary rule:
- `bead`: the effort doesn't change what any requirement or active change says "done" means. Chores,
  refactors, small fixes, and bugs that restore behavior a requirement already specifies.
- `amend`: it changes behavior an active change covers; `change_name` names it.
- `new`: it changes or adds specified behavior that no active change covers.
- `covered`: an accepted requirement or active change already asks for exactly this.
`amend` and `covered` must cite paths in the index, and `amend` must name an active change; otherwise
the verdict falls back to `new` with the reason shown. `repair` is not an intake verdict: a bug against
a specification is a bead; full reconciliation can still choose `repair` for larger gaps.

The sheet shows the verdict card with its rationale and citations and lets Ryder override it (Just a
bead, New change, Amend ▾ listing active changes). Confirming:
- **Just a bead**: `create`, exactly as today.
- **New change / Amend**: `create`, then `POST …/issues/{id}/change {action: develop, feedback,
  intake: {verdict, change_name}}`. `intake` is a new optional structured field on `develop`; specflow
  stores it on the record and, when the author's route differs, writes the history entry below from
  it rather than parsing feedback. The button says "Create and develop · up to $5". If `develop` is
  refused the bead stays and the sheet says why, as `then_develop` does in §1.
- **Amend a change with active work**: the intake response carries `target_active` (the round or run
  that `Active()` reports for that change, or empty). When it is set, Create and develop is disabled
  with "resilient-rounds has an active round R; it can be amended when that stops", and the sheet
  offers "Create the bead; develop later". This avoids a `develop` that would pass its synchronous
  checks and then fail in `canonicalize` after a paid author call (author.go). `Active()` is keyed by
  specflow's `Change.ID`, so it only sees changes with a specflow record. For an active change written
  by hand (today, all of them), intake returns `target_tracked: false` and `target_active` empty, and
  the verdict card says "Nacre isn't tracking work on this change". Confirming then uses `develop`'s
  existing amend path unchanged; adopting hand-written changes is NAC-mp6.9. If work starts between
  intake and confirming, that late failure still happens and is shown on the bead's change card as
  today.
- **Already covered**: "Open <change>" and "Create as a bead anyway".
The author still reconciles against the full inventory; if it picks a different route, the change's
history records "Intake suggested amend X; reconciliation chose new because …".

Intake failure (timeout, cost cap, malformed result) leaves the sheet usable: it shows the reason and
offers Just a bead and New change. Repositories without valid OpenSpec, and bridges without the
endpoint, keep today's "New bead" sheet. The assistant keeps its own path (§1): its prompt rules use
the same boundary definitions, so `create_bead` without `then_develop` is the assistant's "just a bead".
*Alternative:* develop every effort and let reconciliation route it. Rejected: each `develop` is a
writer/reviewer loop up to $5, and most efforts are just beads.

### 9. The Proposals lane is derived from change records
`Lane` gains `PROPOSED("proposed", "Proposals")`, ordered first. The lane appears exactly where New
effort does (§8): a valid OpenSpec setup and a bridge that returns the changes summary. `laneOf(issue)` is unchanged; a new
`workflowLane(issue, changes)` in `data/board` returns `PROPOSED` or falls back to `laneOf`, and only
`columns(GroupBy.WORKFLOW)` and `stats` use it. Other `laneOf` callers keep their meaning, with two
adjustments: `StartRoundSheet` leaves out beads whose `workflowLane` is `PROPOSED` (their proposal
isn't approved, so they are not work yet), the assistant's board tools report lane key `proposed`
for them, `BoardScreen`'s scroll-to-selected uses `workflowLane` so a selected proposal scrolls to
Proposals, `BeadDetail`'s lane label shows "Proposals", and the exhaustive `when` in `NacreColors.lane()`
(`Theme.kt`) gets the violet spec tint for `proposed`. `moveActionFor` (`BoardModel.kt`), which turns
a lane key into a drop action, returns no action for `proposed`. `Lane.PROPOSED.status` maps to `open` only to keep the enum total; nothing drops into the
lane.

Membership: an open bead is in Proposals when the changes summary (§4) lists it in a change's
`seed_ids` and that change's phase is before promotion: `drafting`, `reviewing`, `needs_attention`,
`awaiting_approval`, `interrupted`, `covered`, or `stale` (a stale change still awaiting approval;
a stale approved change is implementation, not planning). The epic itself is never in Proposals. When
an already-promoted change is amended by a new seed (attached as an alias, author.go), that seed sits
in Proposals and the epic stays in its ordinary lane with its phase chip ("Revision in review"). A
revision from found work (§10) has no new seed, so only the epic's chip changes. §4's `seed_ids`
includes alias seeds (`r.Aliases`), not just `SeedID`.

Alias seeds today stay open forever after promotion: they are labelled (promote.go) but not mapped, so
`Contract` treats them as unmapped and they can't be run. This change closes them: when a revision
that attached alias seeds is approved and synchronized, nacred closes each with a comment "Folded into
<change> at revision R; the work continues under <epic>". That needs a `Close` on specflow's Beads
interface (it has `Reopen`). Closed seeds can't be pulled back into Proposals by later revisions,
because the lane holds only open beads.

The summary's 50-change cap orders changes before promotion first, so the lane is complete unless more
than 50 are being planned at once; then the lane header says "Showing 50 of n". Within the lane,
cards waiting on Ryder (awaiting approval, needs attention, stale, interrupted) come first, then
drafting and reviewing, then covered, newest first. Covered cards say "Already covered by <change>"
and their card offers Close directly, since closing is the usual answer.

The lane is not a drop target and its cards don't start a drag, because moving one would only change
bd status, not the plan; the overflow menu still offers Close and Defer. The folded pager opens on
Ready explicitly (`initialPage` = index of Ready, since today it is page 0). The summary tile row
shows a Proposals tile only when the count is non-zero, so boards without proposals look as today. An
empty lane shows "Start a New effort".
*Alternative:* a bd status or label for proposals. Rejected: it would duplicate the change phase and
drift from it.

### 10. Found something: one rule for discovered work, while and after work runs
**Labels and creation.** Found beads carry `found:<change-id>` (specflow's stable `Change.ID`, not the
change name). Any bead created through nacred's create paths as a child of a spec epic (the found
endpoint and `POST …/issues`, which New effort, New bead and the assistant's `create_bead` all use) is
created with a new
`bd.CreateParams.NoInheritLabels` (`--no-inherit-labels`) and gets `found:<change-id>`: bd otherwise
copies the epic's `specflow:` label, and the guards in `server/changes.go` and `rounds/spec.go` would
treat a linked bead with no mapping as failing closed. nacred decides this from the parent's labels,
so the app needs no change for ordinary creates. specflow's own section creates during promotion
(promote.go) don't go through these paths and are unaffected: section beads never get `found:`. Every found bead also gets a `discovered-from`
dependency on the bead where the work was found when there is one (an ordinary New bead under the
epic has none), and its description ends with `Found in: <bead>; source comment: <comment-id>` when it
came from a `Found:` comment.

**Keeping `found-amend` beads out of work.** A `found-amend` bead is a pending change to the plan, not
work. The run and round guards (`server/changes.go`, `rounds/spec.go`) refuse it with "This is a
pending amendment to <change>: revise the change or make it a follow-on", and `StartRoundSheet` leaves
it out. Preflight spec drift (§2) skips every `found:` bead, not only section beads: they belong to
the change already, and drift against their own change would start a second change.

**From Ryder.** The overflow menu of a spec epic, its section beads and found beads, and the round view
of a spec round gain "Found something". From the round view, the sheet asks which round bead the work
was found in, defaulting to the bead whose run is open. The sheet takes a short note and offers three
answers:
- **Only how or in what order** → a found bead under the epic. It is ordinary work: it runs as a
  normal run or round, has no pinned contract, and leaves the approved plan unchanged. An optional
  "Must happen before" picker lists the change's open sections and adds a `blocks` dependency from
  the found bead to the chosen section bead; `sectionCurrent` only checks that required dependencies
  are present, so an extra one doesn't stale the section.
- **Changes what "done" means** → a found bead under the epic, additionally labelled `found-amend`,
  and a `found_work` Needs you item. Nothing is revised automatically, because for a promoted change a
  revision is expensive: approving a new revision reopens every closed section bead (promote.go),
  rewrites their `Revision:` lines and invalidates their evidence, so landed sections must be run and
  landed again; and until it is approved, `Contract` refuses all section work. The item states this
  with the counts ("Revising reopens 4 landed sections; they will need new rounds") and offers:
  **Revise now** (spends, up to $5), **Make it a follow-on change** (the out-of-scope path, after which
  the found bead is closed with a link to the new bead), or open the bead. Revise now is disabled with
  the reason while linked work is active, which is the existing `revise` refusal. Revision-alias work in NAC-mp6.8 is
  what would let a revision keep earlier evidence.
- **Out of scope** → the New effort sheet, prefilled with the note; the new bead gets a `related`
  dependency on the epic, no parent and no `found:` label.

**Revise now** goes through `POST /v1/repos/{repo}/changes/{change-id}/found
{answer: "revise", found_beads: [...]}`, not the plain `revise` action. While that revise is pending
(the record's pending revise action lists the bead), the bead's `found_work` item is not raised and
the bead shows "in revision" under Added during work; it returns only if the record is reset
without approval, for example a new revise started another way replacing that pending action. nacred joins the notes of every
listed open `found-amend` bead (oldest first, each quoted with its bead id) into one feedback, refuses
with "Too many notes for one revision; revise with fewer" when the joined text exceeds specflow's
feedback cap, and records the bead ids on the pending revise action. When that revision is approved
and synchronized, nacred closes those beads with a comment linking the revision. A revision started
some other way leaves found beads open. The other answers use the same endpoint:
`{answer: "how_order", note, found_in, before_section?, comment_id?}`,
`{answer: "amend", note, found_in, comment_id?}`,
`{answer: "out_of_scope", note, found_in, comment_id?}` (returns the New effort prefill; creates
nothing and posts no marker) and `{answer: "dismiss", comment_id}`. When `comment_id` is given with an
answer that creates a bead, nacred derives `found_in` from the comment's bead, writes the source line
into the created bead and posts the `[nacre-found:<comment-id>]` reply. For out of scope the marker
is posted only when New effort actually creates the bead, through `found_comment` below, so cancelling
the sheet leaves the item in Needs you. The out-of-scope and follow-on paths create their bead through
New effort, whose create request gains two optional fields handled by nacred: `related: [epic]` adds
the `related` dependency, `follow_on_of: <found bead>` closes that found bead with a comment
linking the new one once it is created, and `found_comment: <comment-id>` posts that comment's marker
once the bead exists.

The assistant gets a `found_something` write tool with the same answers. It is approved like any
write, and `revise` through it counts as spending.

**From agents.** Spec round prompts (`rounds/prompts.go`, which already permits bd comments) add: stay
inside this section; when you find work outside it, don't do it, add a bd comment on your bead starting
`Found:` with one line, and continue. After a spec round bead reaches any end state (verified,
parked, skipped or failed), nacred lists that bead's comments and raises one `found_work` item per `Found:` comment that has no answer marker. Answering an
item through the found endpoint, or dismissing it, posts a reply comment on the same bead containing
`[nacre-found:<comment-id>]` plus the answer ("answered: how/order → NAC-q3z", "dismissed"). That
marker is the persisted state: items stay derived from current bd state, and a comment with a marker
never raises an item again. The item id is the digest of the comment id, so the app dedupes it across
refreshes. Agents never create beads.

**Archive readiness** (§6) also requires that no `found:<change-id>` bead under the epic is open.
*Alternative:* let agents file beads directly (`discovered-from`). Rejected: it widens the change's
work without a decision, which is the drift this change exists to stop.

### 11. The plan is frozen at approval; progress is read from beads
Nothing in the approval model changes: `tasks.md` is already part of the approved revision, and
checkboxes are already observations rather than evidence. This decision makes that visible and keeps
anything from writing progress back:
- Change detail's Work section and the epic header (§5) show the approved sections in plan order,
  each with its bead's status and evidence (§4), and a separate "Added during work" group listing
  `found:` beads under the epic. Evidence counts only for the current approved revision, as today, so
  after a revision is approved, reopened sections show "needs a new round" rather than landed. `tasks.md` checkboxes are shown as the plan, labelled "Plan", never as
  done.
- Nothing in nacred or the app ticks `tasks.md` checkboxes. A hand edit to `tasks.md` after approval
  is already an inventory change that stales the approval (§6); its stale reason reads "tasks.md was
  edited after approval. Progress is tracked in beads; revise to change the plan."
- At archive, a final `tasks.md` written from bead state (`beads-plan view`'s idea: sections with their
  checklists checked from bead closure and evidence, bead ids as comments) is recorded as a requirement
  of the archive follow-up, NAC-mp6.8, where the revision-alias work makes that write safe.

## Risks / Trade-offs

- [Preflight over-asks with spec drift] → Citations required; only behavior contradictions, not mere
  mentions; the plan shows the question count, as today.
- [Assistant offers proposals too eagerly] → Prompt rules exclude small work; every offer needs approval
  and shows a budget; declines are remembered for the bead in that conversation.
- [Needs you grows stale or noisy] → Items are derived from current state on each request, not stored;
  answered items disappear on refresh.
- [Stale approvals are frequent, because any OpenSpec edit stales every change] → The reason is now named
  and actionable; reducing how often it happens belongs with the revision-alias work.
- [Archive stays a CLI step for now] → It is listed in Needs you with the change's obligations, so it
  isn't forgotten.
- [Two computers report the same repo] → Items carry the repo key; the app dedupes by `id`.
- [Intake disagrees with full reconciliation] → It is advice: the author's route stands, and the
  disagreement is recorded in the change history. Intake sees a 24 KB index, not every artifact.
- [Intake says "just a bead" for behavior-changing work] → Preflight spec drift (§2) still catches it
  before a round, with the same three answers.
- [Revising a promoted change is costly] → Found work that changes "done" never revises
  automatically; the item states how many landed sections would reopen and offers a follow-on change
  instead. Revision aliases (NAC-mp6.8) are the real fix.
- [Agents don't follow the Found: convention] → Then nothing is raised, as today; verification and
  evidence rules are unchanged, so out-of-section work still fails review rather than passing silently.
- [The Proposals lane fills with abandoned drafts] → Close and Defer remain in the overflow menu, and
  stale or interrupted ones also show in Needs you.

## Migration Plan

Additive. Older bridges without `/v1/attention` or `/changes` leave the Needs you section hidden and
chips absent; bead detail falls back to today's Change card. Ship nacred first, then the app.

## Open Questions

None blocking. Defaults: specflow's $5 per-action limit, three review passes (both unchanged), 24 KB
requirement index, five inbox items before "Show all", intake capped at $0.50 and 90 s.

Decided by Ryder on 2026-11-01: amend this change rather than `spec-guided-beads` (implemented) or a
new change; intake may answer "just a bead"; add the Proposals lane; `tasks.md` is the plan beads are
made from, frozen after approval, with existing tools' ideas borrowed rather than installed. For Ryder: should `start_round` also become a
spending tool that always asks? Left unchanged here.
