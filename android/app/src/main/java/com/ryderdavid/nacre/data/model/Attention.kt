package com.ryderdavid.nacre.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject

/**
 * Something waiting on Ryder, from a computer's `/v1/attention`. [hostId] and
 * [hostName] are set by the app; [id] is stable for the same waiting state, so
 * items from two computers dedupe.
 */
@Serializable
data class AttentionItem(
    val id: String,
    val kind: String,
    val repo: String,
    @SerialName("repo_name") val repoName: String = "",
    @SerialName("bead_id") val beadId: String = "",
    @SerialName("change_id") val changeId: String = "",
    @SerialName("round_id") val roundId: String = "",
    val title: String = "",
    val why: String = "",
    @SerialName("source_ref") val sourceRef: String = "",
    val options: List<AttentionOption> = emptyList(),
    /** Every open question of a round, asked together in one item. */
    val questions: List<AttentionQuestion> = emptyList(),
    @SerialName("created_at") val createdAt: String = "",
    val hostId: String = "",
    val hostName: String = "",
) {
    val repoKey get() = Keys.of(hostId, repo)

    /** Questions and decisions first, then approvals, then landing and archive. */
    val group get() = when (kind) {
        ROUND_QUESTION, BEAD_DECISION, BEAD_PARKED, REVIEW_BLOCKED, WORKFLOW_INTERRUPTED, FOUND_WORK -> 0
        PROPOSAL_READY, APPROVAL_STALE, BEAD_SIGNOFF -> 1
        else -> 2
    }

    companion object {
        const val BEAD_DECISION = "bead_decision"
        const val PROPOSAL_READY = "proposal_ready"
        const val REVIEW_BLOCKED = "review_blocked"
        const val APPROVAL_STALE = "approval_stale"
        const val WORKFLOW_INTERRUPTED = "workflow_interrupted"
        const val ROUND_QUESTION = "round_question"
        const val BEAD_PARKED = "bead_parked"
        const val ROUND_READY = "round_ready"
        const val CHANGE_ARCHIVABLE = "change_archivable"
        const val FOUND_WORK = "found_work"
        const val EPIC_FINISHABLE = "epic_finishable"
        const val BEAD_STUCK = "bead_stuck"
        const val BEAD_UNCLOSED = "bead_unclosed"
        const val BEAD_SIGNOFF = "bead_signoff"
    }
}

/** An exact action ([type] "action") or a navigation ([type] "open"). */
@Serializable
data class AttentionOption(
    val type: String,
    val label: String,
    val method: String = "",
    val path: String = "",
    val body: JsonObject? = null,
    val target: AttentionTarget? = null,
    /** Why the action can't be used yet; shown instead of sending it. */
    val disabled: String = "",
    /** The answer Claude recommends. */
    val recommended: Boolean = false,
)

/** One round question: its answers, the recommended one first, and why. */
@Serializable
data class AttentionQuestion(
    val id: String,
    val text: String = "",
    val reason: String = "",
    val options: List<AttentionOption> = emptyList(),
)

/**
 * Where an "open" option goes: a `bead`, `change` ([id] is the seed bead) or
 * `round`; or a sheet with a prefill: `found_something` or `new_effort`.
 */
@Serializable
data class AttentionTarget(
    val kind: String,
    val repo: String = "",
    val id: String = "",
    val revision: String = "",
    val change: String = "",
    @SerialName("found_in") val foundIn: String = "",
    @SerialName("comment_id") val commentId: String = "",
    val note: String = "",
    val related: List<String> = emptyList(),
    @SerialName("follow_on_of") val followOnOf: String = "",
)

@Serializable
data class AttentionList(val items: List<AttentionItem> = emptyList())

/** A specification change phase transition from `/v1/events`. */
@Serializable
data class ChangeEvent(
    @SerialName("repo_id") val repoId: String = "",
    @SerialName("change_id") val changeId: String = "",
    @SerialName("change_name") val changeName: String = "",
    val phase: String = "",
    @SerialName("seed_id") val seedId: String = "",
    @SerialName("epic_id") val epicId: String = "",
)
