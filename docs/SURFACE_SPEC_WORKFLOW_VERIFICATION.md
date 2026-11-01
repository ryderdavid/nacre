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

## One real cycle (task 10.2)

The same isolated nacred, now with the real Claude Code CLI (subscription) and OpenSpec 1.15, took
the demo repository through the whole cycle. About $5 was spent in total.

| Step | Result |
|---|---|
| Intake, real verdict | "Make Enter send the message": `new`, citing `openspec/specs/boards/spec.md` "Enter inserts a newline", $0.02, 4.5 s. |
| Spec drift in preflight | A round with that bead stopped in `needs_decision` with one `spec-drift` question citing the requirement (line 4) and the three fixed answers. An earlier run reported the drift only in its summary and raised no finding: **fixed** (citations are normalized, the prompt asks for a finding, and dropped findings are logged as set aside). |
| Amend the spec first | The bead was left out ("Left out to amend the spec first…"), the round stopped with "Every bead was left out at pre-flight", and a proposal was developed: route amend of a new `enter-sends-message`, 4 sections, two author/review passes, $0.92. Needs you showed "Proposal ready". |
| Approve and promote | Epic DEMO-fff with four section beads. |
| Spec round | A preflight ambiguity question (answered from Needs you's round question); all four sections integrated and passed the gates and final build for $2.45. The agents reported two out-of-section problems as `Found:` comments, which appeared in Needs you. |
| Land | Landing first paused: the change's artifacts sat untracked in the checkout (written by the proposal author) and the round's commit brings the same files, so `git merge --ff-only` refused. **Fixed** (untracked files byte-identical to what the landing commit adds are cleared first; anything different is kept and the merge still refuses). "Try again" was then refused as "nothing has changed", because the failure fingerprint ignored the target checkout: **fixed** (landing failures include the checkout's status). The round landed and every section showed `landed`. |
| Ready to archive | Sections with human checks stayed open, correctly; with those closed (simulated in this fixture, marked as such), Needs you showed "enter-sends-message is ready to archive". The `Found:` items had disappeared after landing because Needs you read only active rounds: **fixed** (ended rounds are read too). |
| Found work, amend | Answering section 1's `Found:` comment as "changes what done means" created DEMO-fff.5 (`found-amend`), posted the answered marker (the comment's item went away), and the amendment's item said "Revising enter-sends-message reopens 4 landed sections; they will need new rounds". Ready to archive was withdrawn while it was open. |
| Revise now | Sent from the item's exact action: a revision carrying DEMO-fff.5 was written and reviewed (`pending_found` recorded). Approving it closed DEMO-fff.5 ("Revised into enter-sends-message at revision a07acf1f4239"), reopened the four sections and added a fifth. |

Four defects were found by running the real system and are fixed with tests: the parent
`discovered-from` link, uncited spec-drift findings, landing over identical untracked artifacts,
and the landing-failure fingerprint; plus Needs you missing `Found:` comments from ended rounds.
