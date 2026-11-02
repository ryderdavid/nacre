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
fun changeNextAction(change: SpecChange, feedback: String = ""): ChangeNextAction? =
    baseNextAction(change, feedback)?.let { if (it.action == "revise") reviseAction(change, feedback) else it }

/** The first finding the reviewer still blocks on, as the computer judges it. */
fun blockingFinding(change: SpecChange) = change.findings.firstOrNull {
    (it.severity == "blocker" || it.severity == "high") && it.disposition != "resolved" && it.disposition != "rejected"
}

/** Revise drafts and reviews the whole change again, so it says what it costs and what it goes on. */
fun reviseAction(change: SpecChange, feedback: String): ChangeNextAction {
    val blocked = blockingFinding(change)?.summary
    return if (feedback.isBlank()) ChangeNextAction(
        "revise", "Try again (up to \$5)",
        if (blocked != null) "Still blocked on: $blocked. Without your feedback Claude will likely argue the same point; say how to decide it above."
        else "Drafts and reviews the change again from scratch. Add feedback above to steer it.",
    ) else ChangeNextAction(
        "revise", "Revise with your feedback (up to \$5)",
        "Drafts the change again following your feedback, then a fresh reviewer checks it.",
    )
}

private fun baseNextAction(change: SpecChange, feedback: String): ChangeNextAction? {
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

/** Phases that wait on Ryder: the screen leads with a summary and one recommended step. */
private val guidedPhases = setOf("awaiting_approval", "needs_attention", "stale", "covered")

fun isGuided(change: SpecChange) = change.phase in guidedPhases && change.activeRoundId.isBlank()

/** The recommended step and the alternatives: the computer's guide, or the screen's own next action until it is written. */
fun guideChoices(change: SpecChange): Pair<com.ryderdavid.nacre.data.model.GuideOption, List<com.ryderdavid.nacre.data.model.GuideOption>> {
    val g = change.guide
    if (g?.recommended != null) return g.recommended to g.options.filter { it != g.recommended }
    val shelve = com.ryderdavid.nacre.data.model.GuideOption("close", "Shelve it", why = "Closes ${change.seedId}; nothing has been built.")
    val next = changeNextAction(change)
    val recommended = when {
        change.phase == "covered" -> com.ryderdavid.nacre.data.model.GuideOption("close", "Close it as covered", why = "Existing work already does this.")
        next == null || next.action !in setOf("approve", "revise", "resume") -> com.ryderdavid.nacre.data.model.GuideOption("revise", "Revise", why = "Draft and review the change again.")
        else -> com.ryderdavid.nacre.data.model.GuideOption(next.action, next.label.substringBefore(" (up to"), why = next.detail)
    }
    val others = buildList {
        if (recommended.action != "revise") add(com.ryderdavid.nacre.data.model.GuideOption("revise", "Revise with your feedback", why = "Drafts and reviews it again, up to \$5."))
        if (recommended.action != "close") add(shelve)
    }
    return recommended to others
}

/** What the main button says for a step. */
fun guideButton(option: com.ryderdavid.nacre.data.model.GuideOption, seedId: String) = when (option.action) {
    "approve" -> "Approve and run"
    "revise" -> "Revise · up to \$5"
    "resume" -> "Continue"
    "close" -> "Shelve $seedId"
    else -> option.label
}

/** The plain summary lines: the guide's, or what the record says until it is written. */
fun guideSummary(change: SpecChange): Triple<String, String, List<String>> {
    val g = change.guide
    if (g != null && g.summary.isNotBlank()) return Triple(g.summary, g.status, g.blocking)
    val blocking = change.findings.filter {
        (it.severity == "blocker" || it.severity == "high") && it.disposition != "resolved" && it.disposition != "rejected"
    }.map { it.summary }
    return Triple(change.summary, change.error, blocking)
}
