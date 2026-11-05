package com.ryderdavid.nacre.ui.bead

import com.ryderdavid.nacre.data.model.ChangeEnvelope
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.hasSpecWorkflow

/** One entry of the bead detail's overflow menu. */
enum class BeadMenuItem(val label: String) {
    CopyId("Copy id"),
    Develop("Develop a proposal"),
    Found("Found something"),
    RunManually("Run an agent manually…"),
    Reopen("Reopen"),
    Close("Close…"),
}

/**
 * Whether [issue] can have a proposal developed for it: it isn't linked to a change (no spec
 * workflow, none on the board, none from nacred) and the computer offers spec work. Such a bead
 * offers Develop a proposal from its overflow menu only, never from a card or a chat.
 */
fun offersDevelop(issue: Issue?, onBoardChange: Boolean, envelope: ChangeEnvelope?): Boolean =
    issue != null && !issue.hasSpecWorkflow && !onBoardChange && envelope?.change == null && envelope?.available == true

/** The bead detail's overflow menu, in order. [found] when the bead belongs to a promoted change. */
fun beadMenu(issue: Issue?, develop: Boolean, found: Boolean): List<BeadMenuItem> = buildList {
    add(BeadMenuItem.CopyId)
    if (issue == null) return@buildList
    if (develop) add(BeadMenuItem.Develop)
    if (found) add(BeadMenuItem.Found)
    add(BeadMenuItem.RunManually)
    add(if (issue.isClosed) BeadMenuItem.Reopen else BeadMenuItem.Close)
}
