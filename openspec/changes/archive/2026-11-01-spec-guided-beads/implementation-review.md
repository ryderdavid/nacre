# Implementation adversarial review

Reviewed 2026-10-28 against the approved `spec-guided-beads` proposal and the
isolated implementation source.

## Disposition

Proceed with the built-app handoff. No blocking source-level findings remain in
the reviewed proposal orchestration, execution handoff, evidence policy, or
Android workflow. This disposition does not establish successful production
operation of the whole workflow; the unexecuted paths are recorded below.

The independent portion of this review covers `bridge/internal/specflow`, the
server and daemon wiring, and the Android change model, policy, detail interface,
and round handoff. This reviewer authored the specification snapshot integration
in `bridge/internal/rounds`; the coordinating agent reviewed that code separately.

## Resolved findings

| Finding | Final behavior and review evidence |
|---|---|
| Interrupted promotion could offer an unusable revision action | Promotion and decision-write interruptions retain their stage and expose Resume. Stable child labels and comment markers recover partial writes without duplicate beads or decisions. |
| Goal alignment could be asserted without citing project goals | The independent review rubric requires an explicit `openspec/config.yaml` or `AGENTS.md` reference for goal alignment. Blocking findings prevent approval; a fresh reviewer sees the current artifacts and previous findings. |
| Cached author/reviewer results could be lost during recovery | Model outcomes and spending are checkpointed. Resume consumes retained results rather than automatically repeating a successful paid call. |
| Multiple seeds or retired sections could lose durable identity | Canonical repository/change ownership reuses the epic, retains all seed intents in review, preserves opaque section markers through edits, and records retired sections for safe recovery and restoration. |
| A manual section edit could retain approval or historical verification | Execution validates generated title, body, design, acceptance, parent, stable labels, and required dependency edges. Reading a ready workflow checks these fields before computing evidence and marks drift stale, including edits to a closed child. Ordinary progress notes remain editable. |
| Feedback could be cleared after a silently ignored request | Busy revise/approve submissions and duplicate develop requests containing feedback return a conflict. The client retains its draft on failure rather than treating the feedback as consumed. |
| A second round could be offered while the first still needed a decision | Active round identity is retained even when approval becomes stale. The primary action opens the existing implementation round before offering revision or new implementation. |
| Partial implementation could preselect already closed sections | The specification round picker excludes closed sections, validates the complete mapping, and explains when all sections are closed without sufficient evidence. |
| Historical success could appear as a verified implementation | Evidence requires current approved revision, matching change/section/bead provenance, a successful run, passing verification after the run ended, and suitable round-bead state. Final build and landing have additional round evidence requirements. Stale approval preserves navigation to active work while clearing success flags. |
| Linked execution could omit approval identity or lose proposal files | Linked round requests require explicit change and revision. Exact approved artifacts, including a behavior specification, are pinned to the round, installed before child workspaces, and checked at execution and landing boundaries. Mixed ordinary/specification rounds and generic linked single runs fail closed. |

Source inspection also confirmed bounded author/reviewer iteration, complete
accepted-specification and active-change inventory, repository path
canonicalization, fail-closed missing workflow records, and safe proposal path
validation. Specification rounds disable automatic landing. Child closure follows
successful landing; the epic and archival remain explicit owner actions.

## Design alignment

The Android implementation keeps the existing board and navigation structure.
The bead detail adds a change card and an existing-style full-screen detail
surface, with progressive disclosure for artifacts, review findings, section
evidence and history. Its primary action follows the current workflow state.
Feedback survives dismissal and failed requests. Source review found no competing
visual system or new top-level navigation.

The Android author performed emulator checks of the detail surface and keyboard
behavior. Those checks used a local fixture, not a production workflow. This
reviewer inspected the UI source and policy tests, not the emulator independently.

## Verification and limits

The following checks were directly observed by this reviewer:

- `go test ./internal/rounds -run '^TestSpec' -count=1` passed.
- `go test -race ./internal/rounds -run '^TestSpec' -count=1` passed.
- `go vet ./internal/rounds` passed.
- A final race-enabled run of the specflow regressions for explicit goal evidence,
  post-approval section drift, closed-child evidence invalidation, busy feedback
  rejection and duplicate-develop feedback rejection passed.
- The saved Android `SpecChangeTest` report records eight passing tests and no
  failures, errors or skipped tests. These cover payload compatibility, revision
  identity, evidence gates, active-round navigation and round selection policy.

The rounds helper tests exercise exact snapshot bytes, approved selection,
required behavior specifications, path confinement and symlink refusal, mapping
consistency and complete prompts. They do not run real Git commands.

Real Git/worktree creation, snapshot commits, merges and landing were not executed
in this session because its execution policy prohibited Git operations. The
runtime implementation contains those operations, but compilation and pure helper
tests do not prove their integrated execution. No feature branch or commit was
created by this review.

Paid AI author and reviewer calls were exercised through deterministic fakes;
there was no live paid-model end-to-end proposal run. Fake Beads clients and
fault injection establish recovery behavior in tests, not a complete production
promotion/implementation/landing cycle. The built app and local emulator fixture
must not be represented as proof of that production cycle.

The coordinating agent owns release-build results and final handoff. This report
is review evidence; durable implementation status remains on the Beads board.
