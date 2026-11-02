package com.ryderdavid.nacre.ui.bead

import com.ryderdavid.nacre.data.model.SpecChange

data class ChangeNextAction(val action: String, val label: String, val detail: String)

fun changeStatusTitle(change: SpecChange): String = when {
    change.phase == "ready" -> if (change.activeRoundId.isNotBlank() || change.evidence.isNotEmpty()) "Working" else "Ready"
    else -> changePhaseLabel(change.phase)
}

fun changePhaseLabel(phase: String): String = when (phase) {
    "drafting", "reviewing", "promoting" -> "Planning"
    "awaiting_approval" -> "Ready for your approval"
    "ready" -> "Ready to implement" // Legacy board callers have no round or evidence context.
    "needs_attention", "stale", "interrupted" -> "Needs attention"
    "covered" -> "Already covered"
    "archived" -> "Done"
    else -> "Check workflow status"
}

/** Existing work stays reachable; approval always starts the reviewed revision. */
fun changeNextAction(change: SpecChange, feedback: String = ""): ChangeNextAction? {
    if (change.phase == "archived") return null
    if (change.activeRoundId.isNotBlank()) return ChangeNextAction(
        "review", "View progress", "Open the running round to see progress or resolve what needs attention.",
    )
    return when (change.phase) {
        "awaiting_approval" -> if (change.revision.isBlank()) null else ChangeNextAction(
            "approve", "Approve and run", "Approve this revision and run its children. The epic finishes into main automatically.",
        )
        "ready" -> when {
            feedback.isNotBlank() -> ChangeNextAction("revise", "Revise", "Review the proposal again with your feedback.")
            change.allLanded -> ChangeNextAction("epic", "Finish epic", "Open the epic to finish it into main.")
            !change.hasCurrentApproval || change.implementationIds.isEmpty() -> ChangeNextAction("refresh", "Check approval", "Refresh the current approval and section mapping.")
            change.evidence.any { it.roundId.isNotBlank() || it.runId.isNotBlank() } -> ChangeNextAction("review", "View progress", "Inspect the work and its recorded evidence.")
            change.epicId.isNotBlank() -> ChangeNextAction("run", "Run epic", "Run the approved children and finish the epic into main automatically.")
            else -> ChangeNextAction("refresh", "Check approval", "Refresh the linked epic before starting work.")
        }
        "stale" -> if (change.adoption != null && change.approvedRevision.isBlank()) ChangeNextAction(
            "resume", "Review the files again", "Review the updated files before approval.",
        ) else ChangeNextAction("revise", "Revise", "Review the changed proposal and project context again.")
        "interrupted" -> ChangeNextAction("resume", "Continue planning", "Continue the saved workflow on the computer.")
        "needs_attention" -> if (change.adoption != null && change.approvedRevision.isBlank() && feedback.isBlank()) ChangeNextAction(
            "resume", "Review the files again", "Review the files again after addressing the findings.",
        ) else ChangeNextAction("revise", "Revise", "Use your guidance to address the review findings.")
        "covered" -> if (feedback.isNotBlank()) ChangeNextAction("revise", "Revise", "Reconsider the proposal with your feedback.") else null
        else -> null
    }
}

fun changeRouteLabel(route: String) = when (route) {
    "new" -> "New change"
    "amend" -> "Amend an existing change"
    "repair" -> "Repair against existing requirements"
    "covered" -> "Covered by existing work"
    "adopted" -> "Adopted as written"
    else -> "Checking existing specifications"
}
