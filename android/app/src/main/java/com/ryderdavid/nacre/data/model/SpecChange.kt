package com.ryderdavid.nacre.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/** Additive bridge contract: unknown phases remain readable but never runnable. */
@Serializable
data class ChangeEnvelope(val available: Boolean = false, val reason: String = "", val change: SpecChange? = null)

@Serializable
data class ChangeAction(val action: String, val revision: String = "", val feedback: String = "")

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
