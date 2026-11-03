package com.ryderdavid.nacre.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/** Additive bridge contract: unknown phases remain readable but never runnable. */
@Serializable
data class ChangeEnvelope(
    val available: Boolean = false, val reason: String = "", val change: SpecChange? = null,
    /** What a proposal is compared with; null on older bridges or without readable OpenSpec. */
    val inventory: InventorySummary? = null,
    /** Active changes written outside Nacre that no workflow tracks yet; offered on a bead without one. */
    val adoptable: List<AdoptableChange> = emptyList(),
) {
    /** The written change this bead looks like the owner of, if any. */
    val suggestedAdoption get() = adoptable.firstOrNull { it.suggested && it.problem.isBlank() }
}

/** An active change on disk with no workflow: [problem] says why it can't be adopted as it stands. */
@Serializable
data class AdoptableChange(
    @SerialName("change_name") val changeName: String,
    val why: String = "",
    @SerialName("tasks_done") val tasksDone: Int = 0,
    @SerialName("tasks_total") val tasksTotal: Int = 0,
    @SerialName("has_review") val hasReview: Boolean = false,
    val suggested: Boolean = false,
    val problem: String = "",
)

/** Set when a change was written outside Nacre and adopted as it stood. */
@Serializable
data class ChangeAdoption(
    val at: String = "",
    @SerialName("recorded_review") val recordedReview: Boolean = false,
    val completed: List<String> = emptyList(),
    val linked: List<String> = emptyList(),
)

@Serializable
data class InventorySummary(@SerialName("accepted_specs") val acceptedSpecs: Int = 0, @SerialName("active_changes") val activeChanges: Int = 0)

@Serializable
data class ChangeAction(
    val action: String, val revision: String = "", val feedback: String = "", val intake: IntakeHint? = null,
    /** The active change an `adopt` takes over, and whether its review.md stands in for a new review. */
    @SerialName("change_name") val changeName: String = "",
    @SerialName("recorded_review") val recordedReview: Boolean = false,
    val run: Boolean = false,
)

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
    val adoption: ChangeAdoption? = null,
    @SerialName("cost_usd") val costUsd: Double = 0.0,
    @SerialName("updated_at") val updatedAt: String = "",
    /** When the proposal first existed; empty while it is still a draft (or on older bridges). */
    @SerialName("created_at") val createdAt: String = "",
    @SerialName("started_at") val startedAt: String = "",
    /** Set only while the proposal hasn't been created yet; null on older bridges. */
    val draft: ChangeDraft? = null,
    /** Plain summary and recommended next step while the change waits on Ryder; null until written. */
    val guide: ChangeGuide? = null,
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
    /** Every current section landed, including sections finished before the change was adopted. */
    val allLanded get() = hasCurrentApproval && sections.isNotEmpty() && sections.all { s -> evidence.any { it.sectionId == s.id && it.beadId == s.beadId && it.landed } }
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
    /** When the proposal first existed; empty while it is still a draft (or on older bridges). */
    @SerialName("created_at") val createdAt: String = "",
    /** When the change record was started; never changes. Empty on older bridges. */
    @SerialName("started_at") val startedAt: String = "",
    /** Set only while the proposal hasn't been created yet; null on older bridges. */
    val draft: ChangeDraft? = null,
) {
    /** Still being planned: no epic yet, or (for an amending seed) a revision before approval. */
    val planning get() = phase in PLANNING_PHASES && (phase != "stale" || epicId.isBlank())
    val landed get() = sections.count { it.evidence == "landed" }
    val verified get() = sections.count { it.evidence == "verified" || it.evidence == "landed" }
    val name get() = changeName.ifBlank { seedIds.firstOrNull().orEmpty() }
    /** A proposal that doesn't exist yet: shown as one placeholder instead of its seeds' cards. */
    val shownAsDraft get() = planning && draft?.kind in ChangeDraft.KINDS
    /** nacred offers `resume` for a stopped operation (`needs_attention` or `interrupted`). */
    val offersResume get() = phase == "needs_attention" || phase == "interrupted"
    /** The lane's order within a rank group: when the change was started, else (older bridges) its last update. */
    val laneOrder get() = startedAt.ifBlank { updatedAt }

    companion object {
        val PLANNING_PHASES = setOf("drafting", "reviewing", "needs_attention", "awaiting_approval", "interrupted", "stale", "covered")
    }
}

/** A proposal still being drafted (`pending`) or that stopped before it was created (`failed`). */
@Serializable
data class ChangeDraft(val kind: String, val message: String = "") {
    val pending get() = kind == PENDING
    val failed get() = kind == FAILED

    companion object {
        const val PENDING = "pending"
        const val FAILED = "failed"
        val KINDS = setOf(PENDING, FAILED)
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

/** Where a change waiting on Ryder stands, in plain words, and what to do next. */
@Serializable
data class ChangeGuide(
    val summary: String = "",
    val status: String = "",
    val blocking: List<String> = emptyList(),
    val recommended: GuideOption? = null,
    val options: List<GuideOption> = emptyList(),
)

/** One next step: approve, revise (with [feedback] as Ryder's guidance), resume, or close (shelve the bead). */
@Serializable
data class GuideOption(val action: String, val label: String, val feedback: String = "", val why: String = "")
