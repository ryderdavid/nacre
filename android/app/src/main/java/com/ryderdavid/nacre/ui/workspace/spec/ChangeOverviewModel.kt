package com.ryderdavid.nacre.ui.workspace.spec

import com.ryderdavid.nacre.data.model.BEAD_INTEGRATING
import com.ryderdavid.nacre.data.model.BEAD_PARKED
import com.ryderdavid.nacre.data.model.BEAD_RUNNING
import com.ryderdavid.nacre.data.model.BEAD_VERIFYING
import com.ryderdavid.nacre.data.model.ChangeEvidence
import com.ryderdavid.nacre.data.model.ChangeSummary
import com.ryderdavid.nacre.data.model.Round
import com.ryderdavid.nacre.data.model.SectionSummary
import com.ryderdavid.nacre.data.model.SpecChange
import com.ryderdavid.nacre.ui.bead.staleText

/** A section's live state, from its bead's evidence and the rounds working it. */
enum class SectionState(val label: String) {
    LANDED("Landed"),
    VERIFIED("Verified"),
    WORKING("In a round"),
    FAILED("Failed"),
    CHECK("Needs your check"),
    WAITING("Waiting"),
}

enum class EvidenceKind(val label: String) { ROUND("Round"), RUN("Run"), BEAD("Bead") }

/** Something that shows the work: a round (opens the work inspector), a run, or a bead. */
data class EvidenceLink(val kind: EvidenceKind, val id: String) {
    val label get() = "${kind.label} $id"
}

/** One section of the plan beside its live progress. */
data class PlanRow(
    val number: Int,
    val sectionId: String,
    val title: String,
    /** Task checkboxes in the section's part of tasks.md. */
    val tasks: Int,
    val beadId: String,
    /** Null before approval: the plan has no progress yet. */
    val state: SectionState?,
    /** Where the section's evidence opens: its round, else its run, else its bead. */
    val link: EvidenceLink?,
    /** Why it failed or what it recorded, when nacred said. */
    val note: String = "",
) {
    /** "3 tasks · NAC-axe.1" */
    val subtitle get() = listOfNotNull(
        when (tasks) { 0 -> null; 1 -> "1 task"; else -> "$tasks tasks" },
        beadId.ifBlank { null },
    ).joinToString(" · ")
}

enum class ReviewTone { PASSED, CHANGES, PENDING, NONE }

data class FindingRow(
    val id: String,
    val severity: String,
    val summary: String,
    val detail: String,
    val disposition: String,
    /** A blocker or high finding that is neither resolved nor rejected. */
    val blocking: Boolean,
    val needsOwner: Boolean,
    val choices: List<String>,
)

/** The independent review: a verdict in words, and its findings. */
data class ReviewSummary(val verdict: String, val tone: ReviewTone, val detail: String, val findings: List<FindingRow>) {
    val blocking get() = findings.count { it.blocking }
    val notes get() = findings.count { !it.blocking }
}

/** Everything the change overview shows. It holds no actions: those are asked for in chat or on a decision card. */
data class ChangeOverview(
    val id: String,
    val name: String,
    /** The change's summary, or its name when it has none yet. */
    val headline: String,
    /** The change's full summary, markdown; the headline is its first sentence. */
    val about: String = "",
    val group: SpecGroup?,
    /** The workflow step in words, e.g. "Reviewed, waiting for your approval". */
    val status: String,
    val revision: String,
    val approvedRevision: String,
    val approvedCurrent: Boolean,
    /** Set when the change is stale: why, and what it needs. */
    val stale: String?,
    val error: String?,
    val waiting: Boolean,
    val approved: Boolean,
    val plan: List<PlanRow>,
    val progress: SectionProgress?,
    val review: ReviewSummary,
    val evidence: List<EvidenceLink>,
    val documents: List<DocKind>,
) {
    val revisionText: String
        get() = when {
            revision.isBlank() -> "No revision yet"
            approvedCurrent -> "Approved revision $revision"
            approvedRevision.isNotBlank() && approvedRevision == revision -> "Approved revision $revision"
            approvedRevision.isNotBlank() -> "Revision $revision · approved $approvedRevision before it changed"
            else -> "Revision $revision, not approved yet"
        }

    val planTitle get() = if (approved) "Plan, as approved" else "Plan, as proposed"

    /** "3 of 5 sections landed · review passed, 2 notes" */
    val summaryLine: String
        get() = buildList {
            progress?.let { add("${it.landed} of ${it.total} sections landed" + if (it.failed > 0) ", ${it.failed} failed" else "") }
            add(review.verdict.replaceFirstChar { it.lowercase() }.let { v -> if (review.tone == ReviewTone.PASSED || review.tone == ReviewTone.CHANGES) "review $v" else v } +
                review.detail.takeIf { it.isNotBlank() }?.let { ", $it" }.orEmpty())
        }.joinToString(" · ")
}

object ChangeOverviewModel {
    private val TASK = Regex("""^\s*[-*] \[[ xX]]""")

    fun taskCount(body: String) = body.lines().count { TASK.containsMatchIn(it) }

    /** The evidence state for one section, folded the way nacred folds it. */
    fun evidenceState(e: ChangeEvidence): String = when {
        e.landed -> "landed"
        e.verified -> "verified"
        e.roundId.isNotBlank() && e.roundState != "landed" && e.roundState != "cancelled" -> "in_round"
        else -> "none"
    }

    /** The change as the list sees it, for a change the board doesn't list (yet). */
    fun summaryOf(change: SpecChange) = ChangeSummary(
        id = change.id, changeName = change.changeName, phase = change.phase, revision = change.revision,
        approvedRevision = change.approvedRevision, epicId = change.epicId, seedIds = listOf(change.seedId),
        sections = change.sections.map { s ->
            val ev = if (!change.hasCurrentApproval) "none" else change.evidence.filter { it.sectionId == s.id && it.beadId == s.beadId }
                .map(::evidenceState).maxByOrNull { listOf("none", "in_round", "verified", "landed").indexOf(it) } ?: "none"
            SectionSummary(s.id, s.title, s.beadId, ev)
        },
        error = change.error, staleReason = change.staleReason, updatedAt = change.updatedAt,
    )

    fun review(change: SpecChange): ReviewSummary {
        val findings = change.findings.map { f ->
            FindingRow(
                id = f.id, severity = severityLabel(f.severity), summary = f.summary, detail = f.detail,
                disposition = dispositionLabel(f.disposition),
                blocking = (f.severity == "blocker" || f.severity == "high") && f.disposition != "resolved" && f.disposition != "rejected",
                needsOwner = f.needsOwner, choices = f.choices,
            )
        }
        val rounds = change.history.count { it.kind == "review" }
        val blocking = findings.count { it.blocking }
        val notes = findings.size - blocking
        val recorded = change.adoption?.recordedReview == true
        val (verdict, tone) = when {
            change.phase == "reviewing" -> "Review under way" to ReviewTone.PENDING
            change.phase == "drafting" && rounds == 0 -> "Not reviewed yet" to ReviewTone.NONE
            blocking > 0 || change.phase == "needs_attention" -> "Changes requested" to ReviewTone.CHANGES
            recorded -> "Passed (its recorded review)" to ReviewTone.PASSED
            change.phase in setOf("awaiting_approval", "ready", "promoting", "covered", "stale", "archived") -> "Passed" to ReviewTone.PASSED
            rounds > 0 -> "Reviewed" to ReviewTone.PENDING
            else -> "Not reviewed yet" to ReviewTone.NONE
        }
        val detail = buildList {
            if (blocking > 0) add(if (blocking == 1) "1 blocking" else "$blocking blocking")
            if (notes > 0) add(if (notes == 1) "1 note" else "$notes notes")
            if (rounds > 1) add("after $rounds rounds")
        }.joinToString(", ")
        return ReviewSummary(verdict, tone, detail, findings)
    }

    fun severityLabel(s: String) = when (s) {
        "blocker" -> "Blocker"
        "high" -> "High"
        "medium" -> "Medium"
        "low" -> "Low"
        else -> s.replaceFirstChar { it.uppercase() }
    }

    fun dispositionLabel(d: String) = when (d) {
        "unresolved", "" -> "Open"
        "resolved" -> "Resolved"
        "accepted" -> "Accepted"
        "rejected" -> "Rejected"
        else -> d.replaceFirstChar { it.uppercase() }
    }

    /**
     * Builds the overview from the change record, the board's summary of it (section evidence), and the
     * project's rounds (a section whose bead is parked in an active round shows Failed and links to it).
     */
    fun build(change: SpecChange, summary: ChangeSummary?, rounds: List<Round> = emptyList()): ChangeOverview {
        val list = summary ?: summaryOf(change)
        val group = SpecList.group(list)
        val approved = change.approvedRevision.isNotBlank() && change.phase in setOf("promoting", "ready", "stale", "archived")
        val evidenceById = list.sections.associate { it.id to it.evidence }
        // The newest round per bead, active ones first: the round that says what the bead is doing now.
        val roundOf = rounds.sortedWith(compareByDescending<Round> { it.isActive }.thenByDescending { it.createdAt })
            .flatMap { r -> r.beads.map { b -> b.id to (r to b) } }
            .distinctBy { it.first }.toMap()

        val plan = change.sections.mapIndexed { i, s ->
            val observed = change.evidence.filter { it.sectionId == s.id && it.beadId == s.beadId }
            val inRound = roundOf[s.beadId]?.takeIf { s.beadId.isNotBlank() }
            val activeRound = inRound?.takeIf { it.first.isActive }
            val ev = evidenceById[s.id] ?: "none"
            val state = if (!approved) null else when {
                ev == "landed" -> SectionState.LANDED
                activeRound?.second?.state == BEAD_PARKED -> SectionState.FAILED
                ev == "verified" && observed.any { it.humanCheck } -> SectionState.CHECK
                ev == "verified" -> SectionState.VERIFIED
                ev == "in_round" || activeRound?.second?.state in setOf(BEAD_RUNNING, BEAD_VERIFYING, BEAD_INTEGRATING) -> SectionState.WORKING
                else -> SectionState.WAITING
            }
            val roundId = activeRound?.first?.id ?: observed.lastOrNull { it.roundId.isNotBlank() }?.roundId ?: inRound?.first?.id
            val runId = observed.lastOrNull { it.runId.isNotBlank() }?.runId ?: inRound?.second?.runId?.takeIf { it.isNotBlank() }
            val link = when {
                !roundId.isNullOrBlank() -> EvidenceLink(EvidenceKind.ROUND, roundId)
                !runId.isNullOrBlank() -> EvidenceLink(EvidenceKind.RUN, runId)
                s.beadId.isNotBlank() -> EvidenceLink(EvidenceKind.BEAD, s.beadId)
                else -> null
            }
            val note = if (state == SectionState.FAILED) activeRound?.second?.reason.orEmpty() else observed.lastOrNull { it.note.isNotBlank() }?.note.orEmpty()
            PlanRow(i + 1, s.id, s.title, taskCount(s.body), s.beadId, state, link, note)
        }

        val progress = if (!approved || plan.isEmpty()) null else SectionProgress(
            landed = plan.count { it.state == SectionState.LANDED },
            verified = plan.count { it.state == SectionState.VERIFIED || it.state == SectionState.CHECK },
            working = plan.count { it.state == SectionState.WORKING },
            failed = plan.count { it.state == SectionState.FAILED },
            total = plan.size,
        )

        val evidence = buildList {
            plan.mapNotNull { it.link?.takeIf { l -> l.kind == EvidenceKind.ROUND } }.forEach(::add)
            change.evidence.filter { it.roundId.isNotBlank() }.forEach { add(EvidenceLink(EvidenceKind.ROUND, it.roundId)) }
            change.evidence.filter { it.runId.isNotBlank() }.forEach { add(EvidenceLink(EvidenceKind.RUN, it.runId)) }
            change.sections.filter { it.beadId.isNotBlank() }.forEach { add(EvidenceLink(EvidenceKind.BEAD, it.beadId)) }
        }.distinct()

        return ChangeOverview(
            id = change.id,
            name = change.changeName.ifBlank { list.name.ifBlank { change.id } },
            headline = headlineOf(change.summary).ifBlank { change.changeName.ifBlank { "A change being drafted" } },
            about = change.summary.trim(),
            group = group,
            status = group?.let { SpecList.status(list, it, progress) } ?: "Archived: its requirements are in the project's specs",
            revision = SpecList.shortRevision(change.revision),
            approvedRevision = SpecList.shortRevision(change.approvedRevision),
            approvedCurrent = change.hasCurrentApproval,
            stale = if (change.phase == "stale") staleText(change.staleReason) else null,
            error = change.error.ifBlank { null },
            waiting = group == SpecGroup.WAITING,
            approved = approved,
            plan = plan,
            progress = progress,
            review = review(change),
            evidence = evidence,
            documents = change.artifacts.mapNotNull { DocKind.of(it.path) }.distinct().sorted(),
        )
    }
}

/**
 * A change's headline: the first sentence of its summary's first paragraph, without markdown marks, at most
 * [max] characters. The summary itself renders below as markdown.
 */
internal fun headlineOf(summary: String, max: Int = 110): String {
    val para = summary.trim().lineSequence().map { it.trim() }.dropWhile { it.isEmpty() || it.startsWith("#") }
        .takeWhile { it.isNotEmpty() }.joinToString(" ")
    val plain = para.removePrefix("- ").removePrefix("* ")
        .replace(Regex("\\[([^\\]]+)\\]\\([^)]*\\)"), "$1").replace("**", "").replace("__", "").replace("`", "")
    val sentence = Regex("^(.+?[.!?])(\\s|$)").find(plain)?.groupValues?.get(1) ?: plain
    return if (sentence.length <= max) sentence else sentence.take(max - 1).trimEnd() + "\u2026"
}
