package com.ryderdavid.nacre.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement

/*
 * The chat-first API (docs/API.md): projects, work summaries and owner decisions.
 * Every field defaults, so older or newer nacred versions parse.
 */

@Serializable
data class ProjectList(val projects: List<ProjectSummary> = emptyList())

/** One repository on one computer, as Projects shows it. */
@Serializable
data class ProjectSummary(
    val repo: String = "",
    val name: String = "",
    val remote: String = "",
    /** working, waiting, stalled or idle. */
    val status: String = "idle",
    @SerialName("status_text") val statusText: String = "",
    @SerialName("needs_you") val needsYou: Int = 0,
    @SerialName("active_change") val activeChange: String = "",
    @SerialName("last_line") val lastLine: String = "",
    @SerialName("last_at") val lastAt: String = "",
    val work: WorkSummary? = null,
)

@Serializable
data class WorkList(val work: List<WorkSummary> = emptyList())

/** A running, stopped or ready round or run, for the work card and the status strip. */
@Serializable
data class WorkSummary(
    /** round or run. */
    val kind: String = "round",
    val id: String = "",
    val title: String = "",
    val change: String = "",
    val branch: String = "",
    /** running, stopped, ready_to_land, held, done, failed or cancelled. */
    val state: String = "",
    val landed: Int = 0,
    val total: Int = 0,
    val gates: List<GateState> = emptyList(),
    @SerialName("spend_usd") val spendUsd: Double = 0.0,
    @SerialName("budget_usd") val budgetUsd: Double = 0.0,
    @SerialName("started_at") val startedAt: String = "",
    @SerialName("last_output_at") val lastOutputAt: String = "",
    /** True only when nacred's own stall threshold fires. */
    val stalled: Boolean = false,
    @SerialName("chat_id") val chatId: String? = null,
    @SerialName("decision_id") val decisionId: String? = null,
) {
    val running get() = state == "running"
    val stopped get() = state == "stopped" || state == "failed"
    val open get() = state in setOf("running", "stopped", "ready_to_land", "held", "failed")
}

@Serializable
data class GateState(val name: String = "", val state: String = "pending")

@Serializable
data class DecisionList(val decisions: List<Decision> = emptyList())

/** An owner decision: one exact action, bound to a revision, answered from a paired device. */
@Serializable
data class Decision(
    val id: String = "",
    val repo: String = "",
    @SerialName("chat_id") val chatId: String? = null,
    /** agent or attention. */
    val source: String = "agent",
    /** approve, land, finish, signoff, limit, adopt, question, recovery, found or intake. */
    val kind: String = "",
    val title: String = "",
    val subtitle: String = "",
    val target: DecisionTarget = DecisionTarget(),
    val consequences: List<String> = emptyList(),
    val evidence: List<DecisionEvidence> = emptyList(),
    val body: String = "",
    val options: List<DecisionOption> = emptyList(),
    /** An input asked whatever option is chosen (found work from a round: which round bead). */
    val ask: DecisionInput? = null,
    @SerialName("allow_defer") val allowDefer: Boolean = true,
    @SerialName("allow_reject") val allowReject: Boolean = true,
    @SerialName("reject_label") val rejectLabel: String = "Reject",
    @SerialName("reject_needs_note") val rejectNeedsNote: Boolean = false,
    /** pending, executing, done, failed, unknown, declined, expired, superseded, deferred or held. */
    val state: String = "pending",
    val result: DecisionResult? = null,
    val reason: String = "",
    @SerialName("superseded_by") val supersededBy: String? = null,
    @SerialName("created_at") val createdAt: String = "",
    @SerialName("expires_at") val expiresAt: String? = null,
    @SerialName("answered_at") val answeredAt: String? = null,
    @SerialName("repo_name") val repoName: String = "",
    /** While deferred: when it counts as pending again (a day after Defer). */
    @SerialName("deferred_until") val deferredUntil: String? = null,
    /** Ryder took this failed, unknown or expired outcome out of the queue ("Got it"). */
    val acknowledged: Boolean = false,
    /** Set by the app: the computer the decision came from. */
    @kotlinx.serialization.Transient val hostId: String = "",
) {
    val key get() = Keys.of(hostId, id)
    val pending get() = state == "pending"

    /** Waits on Ryder and counts in Needs you: pending, or held by him (land or cancel). Deferred doesn't. */
    val waitsOnYou get() = state == "pending" || state == "held"

    /**
     * Listed in the queue: everything that waits, is deferred or still running, and an outcome Ryder
     * hasn't acknowledged: failed, unknown, and expired when the agent raised it (Ask again builds a
     * fresh one). An expired Needs you item no longer waits; nacred raises the next one itself.
     */
    val open get() = when (state) {
        "pending", "deferred", "held", "executing" -> true
        "failed", "unknown" -> !acknowledged
        "expired" -> !acknowledged && source != SOURCE_ATTENTION
        else -> false
    }

    /** An outcome "Got it" can take out of the queue. */
    val acknowledgeable get() = !acknowledged && (state == "failed" || state == "unknown" || (state == "expired" && source != SOURCE_ATTENTION))
    val answered get() = state in setOf("executing", "done", "failed", "unknown", "declined")

    companion object {
        const val SOURCE_ATTENTION = "attention"
    }
}

@Serializable
data class DecisionTarget(
    val change: String = "",
    val bead: String = "",
    val round: String = "",
    /** The change revision (a hash) the decision is bound to. */
    val revision: String = "",
    val commit: String = "",
)

@Serializable
data class DecisionEvidence(val label: String = "", val kind: String = "", val ref: String = "")

@Serializable
data class DecisionOption(
    val id: String = "",
    val label: String = "",
    val detail: String = "",
    val recommended: Boolean = false,
    val protected: Boolean = false,
    /** Why the option can't be used now; shown instead of letting it be chosen. */
    val disabled: String = "",
    val input: DecisionInput? = null,
)

/** Extra input an option needs: an amount, text, or a choice. */
@Serializable
data class DecisionInput(
    /** amount, text or choice. */
    val kind: String = "",
    val label: String = "",
    val min: Double? = null,
    val max: Double? = null,
    val unit: String = "",
    val required: Boolean = false,
    val choices: List<DecisionChoice> = emptyList(),
    /** What an empty answer means (a choice's id). */
    val default: String = "",
)

@Serializable
data class DecisionChoice(val id: String = "", val label: String = "")

@Serializable
data class DecisionResult(val ok: Boolean = false, val message: String = "")

/** The body of an answer: an option (with note and input), a defer, or a reject. */
@Serializable
data class DecisionAnswer(
    val option: String? = null,
    val note: String? = null,
    val input: String? = null,
    /** The answer to the decision's [Decision.ask], beside the option. */
    val ask: String? = null,
    val defer: Boolean? = null,
    val reject: Boolean? = null,
    /** Acknowledges a failed or unknown outcome so it leaves the queue. */
    val acknowledge: Boolean? = null,
)

/** A chat event carrying a decision raised in that chat. */
@Serializable
data class DecisionEvent(val decision: Decision? = null)

/** `GET /v1/repos/{repo}/specs`: the capabilities under openspec/specs. */
@Serializable
data class SpecInventoryList(val specs: List<CurrentSpec> = emptyList())

/** One current spec: its capability name, its requirement headings in order, and its path. */
@Serializable
data class CurrentSpec(
    val name: String = "",
    val requirements: List<String> = emptyList(),
    val path: String = "",
)

/** Unused placeholder so older payloads with arbitrary extra fields still decode in tests. */
@Serializable
internal data class AnyJson(val value: JsonElement? = null)
