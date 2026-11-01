## Why

Nacre can run beads to a verified build, but deciding what to build still requires moving between an editor, OpenSpec and the board. A checked task can disagree with the app's behavior: the round-app specification requests board multi-selection while its checked implementation task opens a separate picker. The missing link is a reviewed, revision-specific contract connecting intent to work and evidence.

## What Changes

- Add a change workflow inside bead detail, using the existing Material 3 surfaces, typography, spacing, side panes and full-screen destinations. No new top-level navigation or replacement board.
- Develop a bead into a proposal after comparing the project context, accepted specs and all active changes. Record whether it is new work, an amendment, a repair of existing requirements, or already covered, with citations and rationale.
- Persist a bounded writer/reviewer loop on the Mac. A fresh adversarial reviewer challenges completeness, feasibility, scope and alignment with project goals. Record findings, dispositions and cost; restart-interrupted calls remain recoverable.
- Require explicit approval of the reviewed artifact revision before promoting the originating bead to an epic or reusing the existing change epic. Generate one child per stable task section, preserving checklists and recording decisions in Beads. Repeated promotion is idempotent.
- Pass the approved specification and section into existing implementation runs and rounds. Reject stale or unapproved linked work server-side. Reconcile evidence without treating a checkbox, process success or a closed bead as proof.
- Show next actions for review, revision, task synchronization, implementation and evidence inspection. Changes remain incomplete while required sections, human verification or landing are missing.

## Capabilities

### New Capabilities
- `spec-guided-work`: Durable reconciliation, proposal/review, approval, epic mapping, implementation eligibility and evidence tracking for repo-local OpenSpec changes.

### Modified Capabilities
None in the accepted spec store. Existing rounds remain the execution mechanism; their active specification is referenced rather than duplicated.

## Impact

Go: new specflow package, authenticated repo-scoped API, Headless runner integration, Beads adapters and eligibility hooks for linked runs/rounds. Android: typed models/client/repository, change card and detail sheet/screen within bead detail, refresh and contextual actions. Documentation: HTTP contract and development walkthrough. No new external services or database dependencies.

Out of scope: replacing OpenSpec schemas, cross-repository proposals, simultaneous coauthoring of one proposal, automatic publication/landing, automatic archive of partially implemented changes, changing existing non-spec bead behavior, and a generic workflow engine. Existing CLI archive remains available; the app reports archive/completion truthfully and links the required next step rather than prematurely archiving.
