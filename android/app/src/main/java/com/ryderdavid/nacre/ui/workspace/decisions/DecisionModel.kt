package com.ryderdavid.nacre.ui.workspace.decisions

import com.ryderdavid.nacre.data.model.Decision
import com.ryderdavid.nacre.data.model.DecisionAnswer
import com.ryderdavid.nacre.data.model.DecisionOption

/** What Ryder has put into a card but not sent yet; kept per decision until it's answered. */
data class DecisionDraft(
    val option: String? = null,
    val input: String = "",
    /** The answer to the decision's ask input; empty takes its default. */
    val ask: String = "",
    val note: String = "",
    /** Rejecting: the note field is for the rejection. */
    val rejecting: Boolean = false,
    /** Add a note was tapped: the note field shows, empty. */
    val noting: Boolean = false,
)

/**
 * A project's queue as the strip shows it: [needsYou] waits on Ryder (pending or held) and is the badge
 * count; [inQueue] is the rest still listed (deferred, executing, failed, unknown or expired), which keeps
 * the queue reachable without counting as needing him.
 */
data class QueueCount(val needsYou: Int = 0, val inQueue: Int = 0) {
    val any get() = needsYou + inQueue > 0
}

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
    fun initialOption(d: Decision): String? {
        val usable = d.options.filter { it.disabled.isBlank() }
        return usable.firstOrNull { it.recommended }?.id ?: usable.singleOrNull()?.id
    }

    /** Why the input for [option] isn't acceptable yet, or null when it is. */
    fun inputProblem(option: DecisionOption?, input: String): String? = inputProblem(option?.input, input)

    /** Why [input] doesn't satisfy [spec] yet, or null when it does; an empty answer takes the default. */
    fun inputProblem(spec: com.ryderdavid.nacre.data.model.DecisionInput?, input: String): String? {
        if (spec == null) return null
        val v = input.trim().ifEmpty { spec.default }
        return when (spec.kind) {
            "amount" -> {
                val n = v.removePrefix("$").toDoubleOrNull()
                when {
                    n == null || !n.isFinite() -> "Enter an amount"
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
        if (option.disabled.isNotBlank()) return option.disabled
        return inputProblem(d.ask, draft.ask) ?: inputProblem(option, draft.input)
    }

    /** The answer body for the card as it stands. */
    fun answer(d: Decision, draft: DecisionDraft): DecisionAnswer = if (draft.rejecting) {
        DecisionAnswer(reject = true, note = draft.note.trim().ifBlank { null })
    } else {
        val option = d.options.firstOrNull { it.id == draft.option }
        DecisionAnswer(
            option = draft.option, note = draft.note.trim().ifBlank { null },
            input = draft.input.trim().removePrefix("$").ifBlank { null }?.takeIf { option?.input != null },
            ask = draft.ask.trim().ifBlank { null }?.takeIf { d.ask != null },
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

    /** Whether Ask again is offered: an expired card nacred can raise again. A chat's intake and found cards are raised again from the chat. */
    fun canRenew(d: Decision) = d.state == "expired" && !raisedFromChat(d)

    /**
     * Whether "Ask the chat again" is offered instead: an expired intake or found card a chat raised. nacred
     * can't rebuild it (it is built from what the chat sent, and intake is a paid model call), so the card
     * opens its chat with a message asking it to raise the card again, which Ryder reads and sends himself.
     */
    fun canAskChat(d: Decision) = d.state == "expired" && raisedFromChat(d) && !d.chatId.isNullOrBlank()

    private fun raisedFromChat(d: Decision) = d.source == "agent" && d.kind in setOf("intake", "found")

    /**
     * The message "Ask the chat again" puts in the chat's composer (the card itself goes with it as a
     * reference). Raising intake again runs intake again, so that message says so, with what it cost last time.
     */
    fun askChatText(d: Decision): String {
        val cost = d.consequences.firstNotNullOfOrNull { INTAKE_COST.find(it)?.groupValues?.get(1) }
        val spend = when {
            d.kind != "intake" -> ""
            cost != null -> " That runs intake again, which cost $cost last time."
            else -> " That runs intake again, which is a paid call."
        }
        return "This card expired before I answered it. Please raise it again so I can decide.$spend"
    }

    private val INTAKE_COST = Regex("""Intake cost (\$[0-9.,]+)""")

    /** Whether "Got it" is offered: a failed, unknown or expired outcome not yet acknowledged. */
    fun canAcknowledge(d: Decision) = d.acknowledgeable

    /** Needs-you order within a state: questions and found work first, then approvals, then landing. */
    fun kindRank(kind: String): Int = when (kind) {
        "question", "found", "recovery", "intake" -> 0
        "approve", "adopt", "signoff", "limit" -> 1
        else -> 2
    }

    /**
     * Ordering in the queue: pending, held, then unresolved outcomes (failed, unknown, expired), then work
     * still executing, then deferred; by kind within each; newest first within that.
     */
    fun queueOrder(list: List<Decision>): List<Decision> = list.sortedWith(
        compareBy<Decision> { stateRank(it.state) }
            .thenBy { kindRank(it.kind) }
            .thenByDescending { it.createdAt },
    )

    private fun stateRank(state: String) = when (state) {
        "pending" -> 0
        "held" -> 1
        "failed", "unknown", "expired" -> 2
        "executing" -> 3
        "deferred" -> 4
        else -> 5
    }

    /**
     * A project's queue as listed: its open decisions in [queueOrder], with the one the queue was opened
     * at ([focus], a decision id) first, shown even when it is no longer open so its outcome can be read.
     */
    fun queue(all: List<Decision>, focus: String? = null): List<Decision> {
        val ordered = queueOrder(all.filter { it.open })
        val focused = focus?.let { id -> all.firstOrNull { it.id == id } } ?: return ordered
        return listOf(focused) + ordered.filter { it.id != focused.id }
    }

    /** What a project's strip counts: decisions waiting on Ryder, and the rest of the open queue. */
    fun counts(all: List<Decision>): QueueCount {
        val open = all.filter { it.open }
        val waiting = open.count { it.waitsOnYou }
        return QueueCount(needsYou = waiting, inQueue = open.size - waiting)
    }

    /**
     * The decision a change waiting for Ryder links to: an open one whose target names the change (by
     * id or name) or one of its seed beads, an approval first, a pending one before others, the newest
     * otherwise. Null when the project has none for it (yet).
     */
    fun forChange(all: List<Decision>, changeId: String, changeName: String, seeds: Collection<String>): Decision? {
        val names = setOf(changeId, changeName).filter { it.isNotBlank() }
        return all.filter { d -> d.open && (d.target.change in names || (d.target.bead.isNotBlank() && d.target.bead in seeds)) }
            .sortedWith(compareBy<Decision> { if (it.kind == "approve") 0 else 1 }.thenBy { stateRank(it.state) }.thenByDescending { it.createdAt })
            .firstOrNull()
    }

    /** The options as a card shows them: the recommended one first, the rest in nacred's order. */
    fun ordered(d: Decision) = d.options.sortedByDescending { it.recommended }
}
