# surface-spec-workflow mockups

Phone mockups for `openspec/changes/surface-spec-workflow` (bead NAC-mp6.7), drawn at 390×844 in the
app's light theme colors from `ui/theme/Theme.kt`. Each screen is a standalone HTML file plus a 2×
PNG render. The editable canvas is the Artifact
https://claude.ai/artifact/P61ewA5BcVarSpdFir5zBX (private to Ryder's account).

Bead ids, costs, file paths and line numbers on these screens are illustrative, not real state.
Where a mockup and the specs disagree, the specs win.

| Screen | Shows | Requirement |
|---|---|---|
| [1](1-boards-needs-you.png) ([html](1-boards-needs-you.html)) | Needs you at the top of Boards, inline answers, summary tiles below | needs-you: Needs you on the Boards screen |
| [2](2-board-spec-chips.png) ([html](2-board-spec-chips.html)) | Spec chip in the first label slot: phase, epic section strip, section evidence, Touches a spec, Closed, not verified | spec-surfacing: Board cards show spec state and evidence |
| [3](3-epic-led-by-change.png) ([html](3-epic-led-by-change.html)) | Spec epic detail led by change header, section strip, human checks, next action | spec-surfacing: Spec epics lead with their change |
| [4](4-assistant-create-and-develop.png) ([html](4-assistant-create-and-develop.html)) | One approval card for `create_bead` with `then_develop`, $5 limit, asks under auto-apply | spec-surfacing: Assistant proposes developing a proposal |
| [5](5-preflight-spec-drift.png) ([html](5-preflight-spec-drift.html)) | Spec-drift question holding the round, three fixed answers, citation | spec-surfacing: Preflight reports spec drift |
| [6](6-needs-you-stale-and-archive.png) ([html](6-needs-you-stale-and-archive.html)) | Stale approval naming its cause; ready to archive with the remaining CLI steps | spec-surfacing: Stale approvals name their cause; Ready to archive is visible |
| [7](7-new-effort-intake.png) ([html](7-new-effort-intake.html)) | New effort sheet after intake: verdict, rationale, citation, override, spending button | effort-intake: New effort runs intake; Intake verdicts cite their basis; Ryder confirms or overrides the verdict |
| [8](8-proposals-lane.png) ([html](8-proposals-lane.html)) | Proposals lane on the folded board: phases, waiting-on-Ryder first, covered, New effort button | effort-intake: Proposals lane; Proposals are not moved by dragging |
| [9](9-found-something.png) ([html](9-found-something.html)) | Found something on a section bead with the three answers | effort-intake: Found something; Discovered work that changes done waits for a decision to revise |
| [10](10-plan-and-progress.png) ([html](10-plan-and-progress.html)) | Change detail: approved sections with progress from beads, work added along the way, the frozen plan | effort-intake: Progress is shown from beads; The plan is frozen at approval |

Screens 7–10 were added by the 2026-11-01 amendment (NAC-mp6.7.1) and rendered with headless Chrome
from their HTML.
