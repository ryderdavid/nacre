Gates used below: **G-bridge** = `cd bridge && go vet ./... && go test -race ./...`;
**G-app** = `cd android && ./gradlew -q testDebugUnitTest`; **G-build** = `cd android && ./gradlew -q assembleRelease`
(with `JAVA_HOME` set to Android Studio's JBR); **G-specs** = `openspec validate --all --strict`.
`nacred gates --build` runs all four.

## 1. One stop path

- [x] 1.1 `api.Stop` (`kind`, `reason`, `diagnosis`, `choices`, `attempted`) on Round, with `pause_reason` kept in step for older apps; `recover` event kind; update `docs/API.md` — G-bridge
- [x] 1.2 `round.stop(Stop)` replacing every `pause` call in `internal/rounds`, each with the right kind and context; attempts persisted so a reload doesn't repeat a rung; rounds paused before the upgrade load as kind `legacy` — G-bridge (reload test mid-ladder)

## 2. Mechanical fixes

- [x] 2.1 Catalogue type and dispatch: resolve a merge only when every conflicted path is handled, then run the gates; undo and fall through otherwise — G-bridge
- [x] 2.2 Move the version re-bump (NAC-gkc) and upstream-branch skip (NAC-2zc) into the catalogue without changing behaviour — G-bridge (existing tests stay green)
- [x] 2.3 `beads-jsonl`: union merge of `.beads/*.jsonl` with duplicates dropped and order kept — G-bridge
- [x] 2.4 `nacre.yaml` `merge.regenerate` (path pattern + command), validated with field names in errors; the `regenerate` fix — G-bridge

## 3. Recovery turns for every merge

- [x] 3.1 Integration turn phases `preflight-merge`, `land-merge` and `land`, with prompts that state each side's intent and the never-revert-the-target rule — G-bridge (golden prompt tests)
- [x] 3.2 Pre-flight merge conflicts and gate failures go to a `preflight-merge` turn before the question is reopened — G-bridge (fake claude that resolves, and one that can't)
- [x] 3.3 Landing merge conflicts go to a `land-merge` turn; a moved target that breaks the gates goes to a `land` turn; land only after the final gates and build pass — G-bridge

## 4. Transient retry

- [x] 4.1 Classifier for environmental errors from real strings (git `index.lock`, bd lock and I/O errors, process start failures, connection resets) — G-bridge (table test)
- [x] 4.2 Retry with 5 s / 30 s backoff at workspace creation, bd reads, run and follow-up start, pre-flight review, and the git steps at landing — G-bridge (fakes that fail once, and always)

## 5. Diagnosis and choices

- [x] 5.1 Diagnosis call through the headless assist path (bd reads + `read_repo`), JSON schema `{summary, cause, choices}`, $0.50 cap, 3-minute timeout, skipped without budget — G-bridge (fake headless)
- [x] 5.2 Choices per stop kind and `POST /v1/rounds/{round}/choose`; `retry-with-budget`, `skip-branch` (pre-flight branch stops only), `skip-bead` (single-bead stops only), `raise-limits`, `cancel`; no partial-landing choice — G-bridge
- [x] 5.2a Limits stop: spend per bead and an estimate for the unfinished beads from the finished ones' average — G-bridge
- [x] 5.3 `continue` compares the failing state with now and refuses when nothing changed; Resume maps to `continue` — G-bridge
- [x] 5.4 `nacred round status` shows the diagnosis and choices; `nacred round choose ROUND CHOICE [--budget USD]` — G-bridge

## 5b. Set-aside work carries over

- [x] 5b.1 Find a bead's newest kept round branch (from stored rounds: parked beads and cancelled rounds) with commits beyond its base — G-bridge
- [x] 5b.2 Start the new bead branch from the integration branch with the kept branch merged in (mechanical fixes, then recovery turn); prompt says the earlier work is there and why it was set aside — G-bridge (round A parks, round B continues)
- [x] 5b.3 Record the pickup on the old round; delete the kept branch after the new round lands — G-bridge

## 6. App

- [x] 6.1 Models for `stop`, choices and `recover` events; `choose` in BridgeClient and NacreRepository — G-app
- [x] 6.2 Stop card: diagnosis summary, one button per choice with its explanation, budget field for `retry-with-budget`, spend per bead and the estimate on a limits stop; recovery events in the round log — G-app, G-build
- [x] 6.3 Notify only on a real stop, not on a successful recovery — G-app

## 7. Release and check

- [x] 7.1 README "Rounds" section: what a round fixes by itself and when it stops; `docs/API.md` complete — G-specs
- [x] 7.2 Replay the two incidents end to end on a scratch repository through a trial nacred: a stale rebased branch chosen for merging, and a target that ships its own release mid-round; both finish without a stop — G-bridge, then by hand
- [x] 7.3 Ryder: one real round on Nacre's own beads with the upgraded nacred; note any stop that still needed him — 2026-11-01: round rdtnuaevyam (NAC-xlr, nacred 0.10.0) landed on its own; main had moved, and the version clash was settled mechanically ("bump undone and redone on top of the target"); nothing needed Ryder
