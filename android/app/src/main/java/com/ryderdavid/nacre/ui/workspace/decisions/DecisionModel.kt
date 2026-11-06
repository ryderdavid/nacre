package com.ryderdavid.nacre.ui.workspace.decisions

import com.ryderdavid.nacre.data.model.Decision
import com.ryderdavid.nacre.data.model.DecisionAnswer
import com.ryderdavid.nacre.data.model.DecisionOption

/** What Ryder has put into a card but not sent yet; kept per decision until it's answered. */
data class DecisionDraft(
    val option: String? = null,
    val input: String = "",
    val note: String = "",
    /** Rejecting: the note field is for the rejection. */
    val rejecting: Boolean = false,
)

object DecisionModel {
    /** The header a card shows for its kind. */
    fun kindLabel(kind: String): String = "Needs you · " + when (kind) {
        "approve" -> "Approve change"
        "land" -> "Land the round"
        "finish" -> "Finish the epic"
        "signoff" -> "Check and sign off"
        "limit" -> "Raise the limit"
        "adopt" -> "Adopt as written"
        "recovery" -> "Choose how to recover"
        "found" -> "Found work"
        "intake" -> "New effort"
        else -> "Question"
    }

    /** The send button's label for a kind (the selected option says what happens). */
    fun sendLabel(d: Decision): String = when (d.kind) {
        "approve", "adopt" -> "Approve"
        "land" -> "Land"
        "finish" -> "Finish"
        "signoff" -> d.options.singleOrNull()?.label ?: "Sign off"
        "recovery", "limit" -> "Choose"
        else -> "Send answer"
    }

    /** The option preselected on a fresh card: the recommended one, else the only one. */
    fun initialOption(d: Decision): String? = d.options.firstOrNull { it.recommended }?.id ?: d.options.singleOrNull()?.id

    /** Why the input for [option] isn't acceptable yet, or null when it is. */
    fun inputProblem(option: DecisionOption?, input: String): String? {
        val spec = option?.input ?: return null
        val v = input.trim()
        return when (spec.kind) {
            "amount" -> {
                val n = v.removePrefix("$").toDoubleOrNull()
                when {
                    n == null -> "Enter an amount"
                    spec.min != null && n <= spec.min -> "More than %s, which is already spent".format(money(spec.min, spec.unit))
                    spec.max != null && n > spec.max -> "At most %s".format(money(spec.max, spec.unit))
                    else -> null
                }
            }
            "text" -> if (spec.required && v.isEmpty()) "Write your answer" else null
            "choice" -> if (spec.required && v.isEmpty()) "Pick one" else if (v.isNotEmpty() && spec.choices.none { it.id == v }) "Pick one" else null
            else -> null
        }
    }

    private fun money(x: Double, unit: String) = if (unit == "usd") "$" + "%.2f".format(x).removeSuffix(".00") else "%.2f".format(x)

    /** Why the card can't be sent as it stands, or null when Send is enabled. */
    fun problem(d: Decision, draft: DecisionDraft): String? {
        if (!d.pending) return "Already answered"
        if (draft.rejecting) return if (d.rejectNeedsNote && draft.note.isBlank()) "Say why in a note" else null
        val option = d.options.firstOrNull { it.id == draft.option } ?: return "Pick an answer"
        return inputProblem(option, draft.input)
    }

    /** The answer body for the card as it stands. */
    fun answer(d: Decision, draft: DecisionDraft): DecisionAnswer = if (draft.rejecting) {
        DecisionAnswer(reject = true, note = draft.note.trim().ifBlank { null })
    } else {
        val option = d.options.firstOrNull { it.id == draft.option }
        DecisionAnswer(
            option = draft.option, note = draft.note.trim().ifBlank { null },
            input = draft.input.trim().removePrefix("$").ifBlank { null }?.takeIf { option?.input != null },
        )
    }

    /** The receipt or state line for a decision that isn't pending; null while pending. Approval is never shown as success. */
    fun stateLine(d: Decision): String? = when (d.state) {
        "pending" -> null
        "executing" -> "Approved · working on it"
        "done" -> d.result?.message?.ifBlank { null } ?: "Done"
        "failed" -> "Failed" + (d.result?.message ?: d.reason).let { if (it.isBlank()) "" else ": $it" }
        "unknown" -> "Outcome unknown" + d.reason.let { if (it.isBlank()) ". Check the work inspector" else ": $it" }
        "declined" -> "Rejected" + d.reason.let { if (it.isBlank()) "" else ": $it" }
        "expired" -> "Expired" + d.reason.let { if (it.isBlank()) "" else ": $it" }
        "superseded" -> "Replaced by a newer request"
        "deferred" -> "Deferred"
        "held" -> "Held by you" + d.reason.let { if (it.isBlank()) "" else ": $it" }
        else -> d.state
    }

    /** Whether the card can be asked again (expired). */
    fun canRenew(d: Decision) = d.state == "expired"

    /** Ordering in the queue: pending first, then held and deferred, then the rest; newest first within. */
    fun queueOrder(list: List<Decision>): List<Decision> = list.sortedWith(
        compareBy<Decision> { when (it.state) { "pending" -> 0; "held" -> 1; "failed", "unknown" -> 2; "deferred" -> 3; else -> 4 } }
            .thenByDescending { it.createdAt },
    )
}
