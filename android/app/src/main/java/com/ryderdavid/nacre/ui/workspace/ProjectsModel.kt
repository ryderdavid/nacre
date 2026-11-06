package com.ryderdavid.nacre.ui.workspace

import com.ryderdavid.nacre.data.ChatFirstData
import com.ryderdavid.nacre.data.model.AttentionItem
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.ProjectSummary
import com.ryderdavid.nacre.data.model.Repo
import com.ryderdavid.nacre.data.model.WorkSummary

enum class ProjectFilter(val label: String) { ALL("All"), NEEDS_YOU("Needs you"), WORKING("Working") }

/** One project checkout on Projects. [needsYou] is null while its computer is offline: unknown, never zero. */
data class ProjectRow(
    val key: String,
    val name: String,
    val status: String,
    val statusText: String,
    val needsYou: Int?,
    val activeChange: String,
    val lastLine: String,
    val work: WorkSummary?,
)

/** A computer and its projects. [checked] says when an offline computer was last heard from. */
data class ComputerSection(
    val hostId: String,
    val name: String,
    val online: Boolean,
    val checked: String,
    val rows: List<ProjectRow>,
)

/** A computer as Projects needs it. */
data class ComputerInfo(val hostId: String, val name: String, val online: Boolean, val lastSeenMs: Long?)

object ProjectsModel {
    /** "Last checked 12 min ago" for an offline computer; "Connected" for one online. */
    fun checked(online: Boolean, lastSeenMs: Long?, nowMs: Long): String = when {
        online -> "Connected"
        lastSeenMs == null -> "Not reached yet"
        else -> {
            val min = ((nowMs - lastSeenMs) / 60_000).coerceAtLeast(0)
            when {
                min < 1 -> "Last checked just now"
                min < 60 -> "Last checked $min min ago"
                else -> "Last checked ${min / 60} h ago"
            }
        }
    }

    /**
     * The sections for Projects: one per computer, with that computer's projects. A computer's own
     * `/projects` report wins; without it (older nacred) rows come from what the app knows.
     */
    fun sections(
        computers: List<ComputerInfo>,
        repos: List<Repo>,
        reported: Map<String, List<ProjectSummary>?>,
        attention: List<AttentionItem>,
        work: Map<String, List<WorkSummary>>,
        filter: ProjectFilter,
        query: String,
        nowMs: Long,
    ): List<ComputerSection> = computers.map { c ->
        val mine = repos.filter { it.hostId == c.hostId }
        val byRepo = reported[c.hostId]?.associateBy { it.repo }
        val rows = mine.map { r ->
            val key = Keys.of(c.hostId, r.id)
            val w = work[key].orEmpty().firstOrNull { it.open }
            val summary = byRepo?.get(r.id) ?: ChatFirstData.fallbackProject(r, attention.count { it.repoKey == key }, w)
            ProjectRow(
                key = key,
                name = summary.name.ifBlank { r.name },
                status = if (c.online) summary.status else "unknown",
                statusText = if (c.online) summary.statusText.ifBlank { ChatFirstData.statusText(summary.work ?: w) } else "Status unknown while offline",
                needsYou = if (c.online) summary.needsYou else null,
                activeChange = summary.activeChange,
                lastLine = summary.lastLine,
                work = summary.work ?: w,
            )
        }.filter { row ->
            (query.isBlank() || row.name.contains(query.trim(), ignoreCase = true)) && when (filter) {
                ProjectFilter.ALL -> true
                ProjectFilter.NEEDS_YOU -> (row.needsYou ?: 0) > 0
                ProjectFilter.WORKING -> row.status == "working" || row.work?.running == true
            }
        }.sortedWith(compareByDescending<ProjectRow> { (it.needsYou ?: 0) > 0 }.thenByDescending { it.work != null }.thenBy { it.name.lowercase() })
        ComputerSection(c.hostId, c.name, c.online, checked(c.online, c.lastSeenMs, nowMs), rows)
    }.filter { it.rows.isNotEmpty() || filter == ProjectFilter.ALL }

    /** How many items need Ryder across every online computer. */
    fun needsYouTotal(sections: List<ComputerSection>): Int = sections.sumOf { s -> s.rows.sumOf { it.needsYou ?: 0 } }
}
