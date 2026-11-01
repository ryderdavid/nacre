# surface-spec-workflow verification (Nacre 0.9.0)

Task 10.1 of `openspec/changes/surface-spec-workflow`, on 2026-11-01. The release APK (0.9.0, code 16)
ran on the Nacre_Fold emulator (Pixel 9 Pro Fold, unfolded and cover displays) against an isolated
nacred 0.9.0 on port 7499 with its own data directory, a fixture repository (`demo`) with OpenSpec,
beads covering every surface, and three hand-written change records (an approved epic with two
sections, a proposal needing attention, a drafting proposal that loads as interrupted). Production
nacred and its boards were not changed. Screenshots: `docs/screenshots/surface-spec-workflow/`.

| Checked | Result |
|---|---|
| Needs you on Boards, unfolded and cover | Five items (interrupted, found work, decision, review blocked, stale approval) with reasons and inline options; Boards badge 5; Runs badge unchanged. A computer on nacred 0.8.1 contributes nothing and doesn't break the section. |
| Proposals lane and tile | Seeds of changes being planned in Proposals, waiting-on-Ryder first; Proposals tile; the folded pager opens on Ready with Proposals as the first tab. |
| Spec chips and section strip | Phase chips, "§ 2 of retain-drafts · no evidence yet", "Touches a spec", "Changes retain-drafts", "Added to retain-drafts"; the epic's strip "0 of 2 verified". |
| Epic led by its change | Phase, revision, stale reason, strip, sections with evidence and the next action above the bead's own fields. |
| Found something from the epic | How/order with "before § 2" created a bead under the epic with `found:sfdemo1`, no workflow labels, and section 2 now blocked by it. **Found a bug**: bd refuses a `discovered-from` link from a child to its own parent, so work found on the epic itself failed with 502 after creating the bead. Fixed in nacred (no such link when found in the epic; the parent link records it), with a test; the sheet had shown the bd error and stayed open. |
| New effort, intake failing | With no working Claude on the walkthrough computer, intake failed and the sheet said so, offering New change and Just a bead; Just a bead created a plain bead and no change record. |
| Open change from Needs you | Opened the change view for the interrupted proposal directly, on the cover screen. |

Not exercised here: intake with a real verdict, Revise now, spec drift in preflight and agents'
`Found:` comments, which need Claude Code calls; they are covered by the Go and JVM tests. Task
10.2, one real cycle to ready-to-archive, needs this nacred installed on the Mac Studio and real
rounds, and is left for Ryder.
