# Design

**Work until done is a round of one.** Rounds already work in their own worktree, check the result
against `nacre.yaml` themselves, commit, land, push and close the bead. A per-round
`land_when_ready` lands without waiting; it may be asked for specification sections too, since asking
is the owner's review.

**An epic is a branch.** A round's target is the shared epic's branch, `epic/<id>`, made from the
landing target the first time. Beads outside any epic land on the target as before. Mixing epics in
one round is refused rather than guessed. Versions are bumped and tagged only where work lands for
good, so epic-branch rounds do neither.

**Finishing is a round without beads.** Its workspace starts from the epic's branch, so the existing
final gates, build, bump, land, moved-target merge and recovery all apply unchanged. When the epic's
change still needs archiving but nothing is left on the branch, it starts from the target instead.
Archiving runs `openspec archive` in that workspace and commits it, so the archive lands with the work
and never touches the checkout. An epic with nothing to merge and no change is simply closed.

**No releases.** Publishing was tried and removed: it tied Nacre to one host. Pushing the target is
the hand-off to the repository's CI.
