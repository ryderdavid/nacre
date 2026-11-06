package com.ryderdavid.nacre.ui.workspace.spec

import com.ryderdavid.nacre.data.model.ChangeSummary

/** The Spec panel's change groups, in the order a change moves through them. */
enum class SpecGroup(val label: String) {
    DRAFTING("Drafting"),
    WAITING("Waiting for you"),
    IN_PROGRESS("In progress"),
    READY_TO_ARCHIVE("Ready to archive"),
}

/** How far a change's approved sections have come, from beads and rounds. */
data class SectionProgress(val landed: Int, val verified: Int, val working: Int, val failed: Int, val total: Int) {
    val waiting get() = (total - landed - verified - working - failed).coerceAtLeast(0)

    /** The progress in words, so it never depends on the bar's colors. */
    val text: String
        get() = buildList {
            add("$landed of $total sections landed")
            if (failed > 0) add("$failed failed")
            if (working > 0) add("$working in a round")
            if (verified > 0) add("$verified verified")
        }.joinToString(" · ")
}

/** One change as the list shows it. */
data class ChangeRow(
    /** The change record's id. */
    val id: String,
    /** The bead the change is loaded through (its first seed, else its epic). */
    val openId: String,
    val name: String,
    val group: SpecGroup,
    /** One line in words: what the change is doing or waiting on. */
    val status: String,
    /** Short revision, e.g. "a1b2c3d"; empty before the first draft. */
    val revision: String,
    /** Section progress from beads; null until the change is approved and has sections. */
    val progress: SectionProgress?,
    val updatedAt: String,
)

/** A spec the project's changes touch, from the changes' spec deltas. */
data class SpecRow(
    val capability: String,
    /** Names of the changes that touch it, most recent first. */
    val changes: List<String>,
    /** The change opened when the row is tapped (the most recent one). */
    val changeId: String,
)

/** A requirement change whose name matches the search. */
data class RequirementHit(val changeId: String, val changeName: String, val capability: String, val requirement: String, val kind: DeltaKind?)

/** The Spec destination's list: changes by group, the specs they touch, and the counts for the header. */
data class SpecListModel(
    val groups: List<Pair<SpecGroup, List<ChangeRow>>>,
    val specs: List<SpecRow>,
    /** Requirement changes matching the search; empty without one. */
    val requirements: List<RequirementHit>,
    /** Specs accepted in openspec/specs, when the computer reported it. */
    val acceptedSpecs: Int?,
    /** Changes shown before search narrows them. */
    val changeCount: Int,
    val query: String,
) {
    val rows get() = groups.flatMap { it.second }
    val isEmpty get() = groups.isEmpty() && specs.isEmpty() && requirements.isEmpty()

    /** "4 changes · 18 specs" for the header. */
    val countsText: String
        get() = buildList {
            add(if (changeCount == 1) "1 change" else "$changeCount changes")
            acceptedSpecs?.let { add(if (it == 1) "1 spec" else "$it specs") }
        }.joinToString(" · ")
}

object SpecList {
    /** Phases where the author is still writing or the reviewer is still checking. */
    val DRAFTING_PHASES = setOf("drafting", "reviewing")

    /** Phases that wait on Ryder: approval, a finding only he can settle, a stale or stopped workflow. */
    val WAITING_PHASES = setOf("awaiting_approval", "needs_attention", "stale", "interrupted", "covered")

    /** Approved and being worked: preparing beads, or running them. */
    val WORKING_PHASES = setOf("promoting", "ready")

    /** Where [c] is listed; null for an archived change, which now lives in the specs. */
    fun group(c: ChangeSummary): SpecGroup? = when {
        c.phase == "archived" -> null
        c.draft?.failed == true -> SpecGroup.WAITING
        c.draft?.pending == true -> SpecGroup.DRAFTING
        c.phase in DRAFTING_PHASES -> SpecGroup.DRAFTING
        c.phase in WAITING_PHASES -> SpecGroup.WAITING
        c.phase == "ready" && c.sections.isNotEmpty() && c.sections.all { it.evidence == "landed" } -> SpecGroup.READY_TO_ARCHIVE
        c.phase in WORKING_PHASES -> SpecGroup.IN_PROGRESS
        // A phase this app doesn't know yet: shown, but never as something Ryder must act on.
        else -> SpecGroup.DRAFTING
    }

    /** Section progress: landed, failed (its bead parked in a round), verified and in a round. */
    fun progress(c: ChangeSummary, failedBeads: Set<String>): SectionProgress? {
        if (c.sections.isEmpty() || c.phase !in WORKING_PHASES) return null
        var landed = 0; var verified = 0; var working = 0; var failed = 0
        c.sections.forEach { s ->
            when {
                s.evidence == "landed" -> landed++
                s.beadId.isNotBlank() && s.beadId in failedBeads -> failed++
                s.evidence == "verified" -> verified++
                s.evidence == "in_round" -> working++
            }
        }
        return SectionProgress(landed, verified, working, failed, c.sections.size)
    }

    /** The one-line status, in words. */
    fun status(c: ChangeSummary, group: SpecGroup, progress: SectionProgress?): String = when {
        c.draft?.failed == true -> "Drafting failed" + c.draft.message.takeIf { it.isNotBlank() }?.let { ": $it" }.orEmpty()
        c.draft?.pending == true -> "Author writing the proposal"
        c.phase == "drafting" -> "Author writing"
        c.phase == "reviewing" -> "Independent review under way"
        c.phase == "awaiting_approval" -> "Reviewed, waiting for your approval"
        c.phase == "needs_attention" -> c.error.ifBlank { "The review found something only you can settle" }
        c.phase == "stale" -> c.staleReason.ifBlank { "The project's specs changed since it was approved" }
        c.phase == "interrupted" -> "Planning stopped before it finished"
        c.phase == "covered" -> "Existing work already covers it"
        c.phase == "promoting" -> "Approved, preparing its beads"
        group == SpecGroup.READY_TO_ARCHIVE -> "Every section landed"
        progress != null -> progress.text
        c.phase == "ready" -> "Approved, no sections yet"
        else -> "Phase \"${c.phase}\": check the workflow on the computer"
    }

    fun shortRevision(revision: String) = if (revision.length > 8) revision.take(7) else revision

    /** The bead a change is opened through. */
    fun openId(c: ChangeSummary) = c.seedIds.firstOrNull { it.isNotBlank() } ?: c.epicId

    fun row(c: ChangeSummary, failedBeads: Set<String>): ChangeRow? {
        val group = group(c) ?: return null
        val progress = progress(c, failedBeads)
        return ChangeRow(
            id = c.id, openId = openId(c), name = c.name.ifBlank { c.id }, group = group,
            status = status(c, group, progress), revision = shortRevision(c.revision), progress = progress,
            updatedAt = c.updatedAt.ifBlank { c.laneOrder },
        )
    }

    /** The capability a change artifact path belongs to: `openspec/changes/x/specs/<capability>/spec.md`. */
    fun capabilityOf(path: String): String? {
        val parts = path.split('/')
        val i = parts.lastIndexOf("specs")
        return if (i >= 0 && i + 2 == parts.lastIndex && parts.last() == "spec.md") parts[i + 1] else null
    }

    /**
     * Builds the list. [specPaths] maps a change id to its artifact paths (from the changes loaded so
     * far); [failedBeads] are beads parked in a round, which make their section Failed; [deltas] are the
     * loaded changes' parsed spec deltas, which search looks through for requirement names.
     */
    fun build(
        changes: List<ChangeSummary>,
        specPaths: Map<String, List<String>> = emptyMap(),
        acceptedSpecs: Int? = null,
        failedBeads: Set<String> = emptySet(),
        query: String = "",
        deltas: Map<String, List<SpecDelta>> = emptyMap(),
    ): SpecListModel {
        val rows = changes.mapNotNull { row(it, failedBeads) }.sortedByDescending { it.updatedAt }
        val byId = rows.associateBy { it.id }
        val specs = specPaths.entries
            .filter { it.key in byId }
            .flatMap { (id, paths) -> paths.mapNotNull(::capabilityOf).distinct().map { it to byId.getValue(id) } }
            .groupBy({ it.first }, { it.second })
            .map { (cap, touching) ->
                val recent = touching.sortedByDescending { it.updatedAt }
                SpecRow(cap, recent.map { it.name }, recent.first().id)
            }
            .sortedBy { it.capability }
        val q = query.trim().lowercase()
        val shown = if (q.isEmpty()) rows else rows.filter { r -> listOf(r.name, r.status, r.id, r.group.label).any { q in it.lowercase() } }
        val shownSpecs = if (q.isEmpty()) specs else specs.filter { s -> q in s.capability.lowercase() || s.changes.any { q in it.lowercase() } }
        val groups = SpecGroup.entries.mapNotNull { g -> shown.filter { it.group == g }.takeIf { it.isNotEmpty() }?.let { g to it } }
        val requirements = if (q.isEmpty()) emptyList() else rows.flatMap { row ->
            deltas[row.id].orEmpty().flatMap { d ->
                d.requirements.filter { q in it.name.lowercase() || q in it.from.lowercase() }.map { RequirementHit(row.id, row.name, d.capability, it.name, it.kind) }
            }
        }
        return SpecListModel(groups, shownSpecs, requirements, acceptedSpecs, rows.size, query)
    }
}
