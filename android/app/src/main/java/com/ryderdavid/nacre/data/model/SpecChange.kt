package com.ryderdavid.nacre.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/** Additive bridge contract: unknown phases remain readable but never runnable. */
@Serializable
data class ChangeEnvelope(
    val available: Boolean = false, val reason: String = "", val change: SpecChange? = null,
    /** What a proposal is compared with; null on older bridges or without readable OpenSpec. */
    val inventory: InventorySummary? = null,
)

@Serializable
data class InventorySummary(@SerialName("accepted_specs") val acceptedSpecs: Int = 0, @SerialName("active_changes") val activeChanges: Int = 0)

@Serializable
data class ChangeAction(val action: String, val revision: String = "", val feedback: String = "", val intake: IntakeHint? = null)

/** The New effort verdict a develop was confirmed with. */
@Serializable
data class IntakeHint(val verdict: String, @SerialName("change_name") val changeName: String = "")

@Serializable
data class ChangeArtifact(val path: String, val content: String)

@Serializable
data class ChangeFinding(
    val id: String, val severity: String, val summary: String,
    val detail: String = "", val disposition: String = "",
)

@Serializable
data class ChangeSection(
    val id: String, val title: String, val body: String = "",
    @SerialName("bead_id") val beadId: String = "",
    @SerialName("depends_on") val dependsOn: List<String> = emptyList(),
)

@Serializable
data class RetiredSection(val id: String, val reason: String, @SerialName("bead_id") val beadId: String = "")

@Serializable
data class ChangeHistory(val at: String, val kind: String, val text: String)

@Serializable
data class ChangeEvidence(
    @SerialName("section_id") val sectionId: String,
    @SerialName("bead_id") val beadId: String,
    @SerialName("run_id") val runId: String = "",
    @SerialName("round_id") val roundId: String = "",
    @SerialName("round_state") val roundState: String = "",
    val verified: Boolean = false,
    @SerialName("build_verified") val buildVerified: Boolean = false,
    @SerialName("human_check") val humanCheck: Boolean = false,
    val landed: Boolean = false,
    val note: String = "",
)

@Serializable
data class SpecChange(
    val id: String,
    @SerialName("seed_id") val seedId: String,
    @SerialName("epic_id") val epicId: String = "",
    @SerialName("change_name") val changeName: String = "",
    val phase: String,
    val revision: String = "",
    @SerialName("approved_revision") val approvedRevision: String = "",
    val summary: String = "",
    val route: String = "",
    val rationale: String = "",
    val references: List<String> = emptyList(),
    val artifacts: List<ChangeArtifact> = emptyList(),
    val findings: List<ChangeFinding> = emptyList(),
    val sections: List<ChangeSection> = emptyList(),
    @SerialName("retired_sections") val retiredSections: List<RetiredSection> = emptyList(),
    val history: List<ChangeHistory> = emptyList(),
    val evidence: List<ChangeEvidence> = emptyList(),
    val error: String = "",
    @SerialName("stale_reason") val staleReason: String = "",
    val intake: IntakeHint? = null,
    @SerialName("cost_usd") val costUsd: Double = 0.0,
    @SerialName("updated_at") val updatedAt: String = "",
) {
    val isWorking get() = phase in setOf("drafting", "reviewing", "promoting")
    val hasCurrentApproval get() = phase == "ready" && revision.isNotBlank() && revision == approvedRevision
    /** All current sections must have distinct mapped children; partial promotion cannot execute. */
    val implementationIds: List<String> get() = sections.map { it.beadId }.takeIf {
        hasCurrentApproval && it.isNotEmpty() && it.all(String::isNotBlank) && it.distinct().size == it.size
    }.orEmpty()
    /** Evidence is supplied for the current approved revision; stale snapshots never count. */
    val verifiedSections get() = if (!hasCurrentApproval) 0 else sections.count { section ->
        evidence.any { it.sectionId == section.id && it.beadId == section.beadId && it.verified && it.runId.isNotBlank() }
    }
    val buildVerifiedSections get() = if (!hasCurrentApproval) 0 else sections.count { section ->
        evidence.any { it.sectionId == section.id && it.beadId == section.beadId && it.verified && it.buildVerified && it.runId.isNotBlank() && it.roundId.isNotBlank() }
    }
    val implementationReadyForReview get() = sections.isNotEmpty() && implementationIds.isNotEmpty() && buildVerifiedSections == sections.size
    /** Active work stays reachable even if a CLI edit invalidates its original approval. */
    val activeRoundId get() = evidence.lastOrNull { e ->
        e.roundId.isNotBlank() && e.roundState.isNotBlank() && e.roundState !in setOf("landed", "cancelled") &&
            sections.any { it.id == e.sectionId && it.beadId == e.beadId }
    }?.roundId.orEmpty()
}

val Issue.hasSpecWorkflow get() = labels.any { it.startsWith("specflow:") || it.startsWith("spec-section:") }

/** The change a spec epic belongs to, from its `specflow:<id>` label. */
val Issue.specEpicChange get() = labels.firstOrNull { it.startsWith("specflow:") }?.removePrefix("specflow:").orEmpty()

/** Work found while implementing a change: `found:<change-id>`. */
val Issue.foundChange get() = labels.firstOrNull { it.startsWith("found:") }?.removePrefix("found:").orEmpty()

/** Found work that would change what the change's "done" means; not runnable. */
val Issue.isPendingAmendment get() = "found-amend" in labels

/** Beads preflight let through as an exception to a requirement. */
val Issue.touchesSpec get() = labels.any { it.startsWith("spec-touch:") }

/** One section of a change and its evidence (`none`, `in_round`, `verified`, `landed`). */
@Serializable
data class SectionSummary(val id: String, val title: String = "", @SerialName("bead_id") val beadId: String = "", val evidence: String = "none")

/** A change as the board needs it: chips, the Proposals lane, the epic strip. */
@Serializable
data class ChangeSummary(
    val id: String,
    @SerialName("change_name") val changeName: String = "",
    val phase: String = "",
    val revision: String = "",
    @SerialName("approved_revision") val approvedRevision: String = "",
    @SerialName("epic_id") val epicId: String = "",
    @SerialName("seed_ids") val seedIds: List<String> = emptyList(),
    val sections: List<SectionSummary> = emptyList(),
    val error: String = "",
    @SerialName("stale_reason") val staleReason: String = "",
    @SerialName("pending_found") val pendingFound: List<String> = emptyList(),
    val active: String = "",
    @SerialName("updated_at") val updatedAt: String = "",
) {
    /** Still being planned: no epic yet, or (for an amending seed) a revision before approval. */
    val planning get() = phase in PLANNING_PHASES && (phase != "stale" || epicId.isBlank())
    val landed get() = sections.count { it.evidence == "landed" }
    val verified get() = sections.count { it.evidence == "verified" || it.evidence == "landed" }
    val name get() = changeName.ifBlank { seedIds.firstOrNull().orEmpty() }

    companion object {
        val PLANNING_PHASES = setOf("drafting", "reviewing", "needs_attention", "awaiting_approval", "interrupted", "stale", "covered")
    }
}

/** A repository's changes; [total] counts beyond the bridge's cap of 50. */
@Serializable
data class ChangesList(
    val changes: List<ChangeSummary> = emptyList(),
    val total: Int = 0,
    /** The repository's OpenSpec setup is valid: New effort, intake and the Proposals lane apply. */
    val openspec: Boolean = false,
)

@Serializable
data class IntakeDraft(
    val title: String,
    val description: String = "",
    val design: String = "",
    val acceptance: String = "",
    val type: String = "",
    val parent: String = "",
)

@Serializable
data class IntakeReference(val path: String, val requirement: String = "")

/** Intake's advice for a New effort: `bead`, `new`, `amend` or `covered`. */
@Serializable
data class IntakeVerdict(
    val verdict: String,
    @SerialName("change_name") val changeName: String = "",
    val rationale: String = "",
    val references: List<IntakeReference> = emptyList(),
    @SerialName("cost_usd") val costUsd: Double = 0.0,
    val fallback: String = "",
    @SerialName("target_tracked") val targetTracked: Boolean = false,
    @SerialName("target_active") val targetActive: String = "",
    @SerialName("active_changes") val activeChanges: List<String> = emptyList(),
)

@Serializable
data class FoundComment(val bead: String, @SerialName("comment_id") val commentId: String)

@Serializable
data class FoundRequest(
    val answer: String,
    val note: String = "",
    @SerialName("found_in") val foundIn: String = "",
    @SerialName("before_section") val beforeSection: String = "",
    @SerialName("comment_id") val commentId: String = "",
    @SerialName("found_beads") val foundBeads: List<String> = emptyList(),
)

@Serializable
data class NewEffortPrefill(
    val title: String = "",
    val description: String = "",
    val related: List<String> = emptyList(),
    @SerialName("found_comment") val foundComment: FoundComment? = null,
    /** Set by the app for a follow-on change, not by the bridge. */
    val followOnOf: String = "",
)

@Serializable
data class FoundResult(@SerialName("bead_id") val beadId: String = "", val prefill: NewEffortPrefill? = null, val change: SpecChange? = null)
