package com.ryderdavid.nacre.ui.bead

import com.ryderdavid.nacre.data.model.SpecChange

data class ChangeNextAction(val action: String, val label: String, val detail: String)

fun changeStatusTitle(change: SpecChange): String =
    if (change.implementationReadyForReview) "Implementation ready for review"
    else changePhaseLabel(change.phase)

fun changePhaseLabel(phase: String): String = when (phase) {
    "drafting" -> "Developing proposal"
    "reviewing" -> "Independent review"
    "needs_attention" -> "Needs your input"
    "awaiting_approval" -> "Ready for your review"
    "promoting" -> "Preparing implementation beads"
    "ready" -> "Ready to implement"
    "stale" -> "Review needs updating"
    "interrupted" -> "Work interrupted"
    "covered" -> "Already covered"
    "archived" -> "Archived"
    else -> "Check workflow status"
}

/** A phase alone never grants execution; the revision and entire mapping must agree. */
fun changeNextAction(change: SpecChange, feedback: String = ""): ChangeNextAction? {
    if (change.activeRoundId.isNotBlank()) return ChangeNextAction(
        "review", "View implementation", "Continue the existing round to see progress, answer questions, review its build or decide on landing. Resolve active work before revising the proposal.",
    )
    if (feedback.isNotBlank() && change.activeRoundId.isBlank() && change.phase in setOf("awaiting_approval", "ready", "covered")) return ChangeNextAction(
        "revise", if (change.phase == "covered") "Reconsider with feedback" else "Revise with feedback",
        "Uses your feedback in another bounded author and independent reviewer cycle. Review the revised proposal before implementation.",
    )
    return when (change.phase) {
    "awaiting_approval" -> if (change.revision.isBlank()) null else ChangeNextAction(
        "approve", if (change.epicId.isBlank()) "Approve and prepare beads" else "Approve revised work",
        if (change.epicId.isBlank()) "Approves this reviewed revision and prepares an epic with one bead per task section. Implementation starts separately."
        else "Updates the linked section beads and may reopen completed work. Previous verification will need to be established again. Implementation starts separately.",
    )
    "ready" -> if (change.implementationReadyForReview) ChangeNextAction(
        "review", "Review implementation", "Inspect the verified round, resolve human checks, and review landing. Acceptance and archival remain separate steps.",
    ) else if (change.allLanded) null
    else if (change.implementationIds.isNotEmpty()) ChangeNextAction(
        "implement", "Implement change", "Choose the approved sections and review the round budget before starting.",
    ) else ChangeNextAction("refresh", "Check approval", "This change does not have a complete, current approval and bead mapping.")
    "stale" -> ChangeNextAction("revise", "Review updated proposal", "The proposal or project context changed. A new independent review is required before implementation.")
    "interrupted" -> ChangeNextAction("resume", "Resume workflow", "Continues the saved workflow on the computer. This may use more of your subscription.")
    "needs_attention" -> if (change.adoption != null && change.approvedRevision.isBlank() && feedback.isBlank()) ChangeNextAction(
        "resume", "Review the files again",
        "After fixing the change's files yourself, an independent review checks them again (up to $2.50). To have Nacre address the findings instead, add guidance below and Revise.",
    ) else ChangeNextAction("revise", "Revise proposal", "Add guidance below, then run another bounded author and reviewer cycle.")
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
