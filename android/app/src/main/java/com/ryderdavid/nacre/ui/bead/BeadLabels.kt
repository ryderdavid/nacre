package com.ryderdavid.nacre.ui.bead

import com.ryderdavid.nacre.data.model.Health
import com.ryderdavid.nacre.data.model.Repo

/** Internal workflow labels are retained when editing, but never shown as user labels. */
fun isMachineLabel(label: String): Boolean =
    label == com.ryderdavid.nacre.data.model.SIGNOFF || listOf("specflow:", "round:", "spec-section:", "nacre-").any(label::startsWith)

fun staleText(reason: String) = reason.ifBlank { "The project's specifications changed since this was approved." } + " A new review is needed before its sections can run."

fun changePhaseLabel(phase: String): String = when (phase) {
    "drafting", "reviewing", "promoting" -> "Planning"
    "awaiting_approval" -> "Ready for your approval"
    "ready" -> "Ready to implement"
    "needs_attention", "stale", "interrupted" -> "Needs attention"
    "covered" -> "Already covered"
    "archived" -> "Done"
    else -> "Check workflow status"
}

val RUN_MODES = listOf(
    Triple("plan", "Plan", "Read-only. Claude investigates and proposes a plan."),
    Triple("acceptEdits", "Edits", "File edits allowed; commands that need approval are refused."),
    Triple("auto", "Auto", "Claude Code's auto mode decides which actions are safe."),
    Triple("bypassPermissions", "Bypass", "Everything allowed, no checks. Only for trusted repos."),
)

/** One computer a bead can be sent to; [health] is what it last reported, null while unreachable. */
data class RunTarget(val repo: Repo, val online: Boolean, val health: Health? = null) {
    /** Why this computer can't run the bead with Codex, or null when it can. */
    val codexBlocker: String? get() = when {
        !online -> "${repo.hostName.ifBlank { "This computer" }} isn't reachable right now"
        else -> health?.codexBlocker
    }
}
