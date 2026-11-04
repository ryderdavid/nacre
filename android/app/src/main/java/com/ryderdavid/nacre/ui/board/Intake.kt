package com.ryderdavid.nacre.ui.board

import com.ryderdavid.nacre.data.model.IntakeHint
import com.ryderdavid.nacre.data.model.IntakeVerdict

/** Where a New effort goes once Ryder confirms: intake's verdict or his override. */
sealed interface EffortRoute {
    data object Bead : EffortRoute
    data object NewChange : EffortRoute
    data class Amend(val change: String) : EffortRoute
    data object Covered : EffortRoute

    val develops get() = this is NewChange || this is Amend

    companion object {
        fun of(v: IntakeVerdict): EffortRoute = when (v.verdict) {
            "bead" -> Bead
            "amend" -> if (v.changeName.isNotBlank()) Amend(v.changeName) else NewChange
            "covered" -> Covered
            else -> NewChange
        }
    }
}

/** Plain words for a route, as the verdict card and radio buttons show it. */
fun EffortRoute.headline() = when (this) {
    EffortRoute.Bead -> "Just a bead"
    EffortRoute.NewChange -> "New change"
    is EffortRoute.Amend -> "Amends $change"
    EffortRoute.Covered -> "Already covered"
}

/** The confirming button: says it spends when it develops a proposal. */
fun EffortRoute.confirmLabel() = when (this) {
    EffortRoute.Bead -> "Create bead"
    EffortRoute.NewChange, is EffortRoute.Amend -> "Create and develop"
    EffortRoute.Covered -> "Create as a bead anyway"
}

/** What develop is told about the verdict it was confirmed with. */
fun EffortRoute.hint(): IntakeHint? = when (this) {
    EffortRoute.NewChange -> IntakeHint("new")
    is EffortRoute.Amend -> IntakeHint("amend", change)
    else -> null
}

/** Feedback for develop, carrying intake's reason when the route is intake's own. */
fun EffortRoute.feedback(v: IntakeVerdict?): String {
    val own = v != null && EffortRoute.of(v) == this
    val why = if (own) v!!.rationale.takeIf { it.isNotBlank() }?.let { ", because $it" }.orEmpty() else ", chosen by Ryder over intake's verdict"
    return when (this) {
        EffortRoute.NewChange -> "Intake: a new change$why"
        is EffortRoute.Amend -> "Intake: amend $change$why"
        else -> ""
    }
}

/**
 * Why developing can't start now, or null. Amending a change whose work Nacre
 * can see is active waits until that work stops.
 */
fun EffortRoute.blockedBy(v: IntakeVerdict?): String? =
    if (this is EffortRoute.Amend && v != null && v.changeName == change && v.targetActive.isNotBlank())
        "$change has active work (${v.targetActive.trimEnd('.')}); it can be amended when that stops."
    else null
