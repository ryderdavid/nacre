package com.ryderdavid.nacre.data.board

import com.ryderdavid.nacre.data.model.ChangeSummary
import com.ryderdavid.nacre.data.model.ChangesList
import com.ryderdavid.nacre.data.model.Issue

/**
 * Workflow lanes are derived, not raw bd statuses: an open bead that still waits on an
 * open blocker is shown as Blocked, so Ready really means "nothing stands in the way".
 */
enum class Lane(val key: String, val title: String) {
    /** Changes being planned; derived from change records, never a bd status. */
    PROPOSED("proposed", "Proposals"),
    READY("ready", "Ready"),
    DOING("doing", "In progress"),
    BLOCKED("blocked", "Blocked"),
    LATER("later", "Deferred"),
    DONE("done", "Done");

    /** The bd status a bead gets when it is dropped into this lane. */
    val status: String
        get() = when (this) {
            // Nothing drops into Proposals; "open" only keeps this total.
            PROPOSED, READY -> "open"
            DOING -> "in_progress"
            BLOCKED -> "blocked"
            LATER -> "deferred"
            DONE -> "closed"
        }
}

enum class GroupBy(val title: String) { WORKFLOW("Workflow"), PRIORITY("Priority"), TYPE("Type") }

/** [note] qualifies the column, e.g. when only some of its changes are listed. */
data class Column(val key: String, val title: String, val issues: List<Issue>, val total: Int, val note: String = "")

data class EpicSummary(val id: String, val title: String, val total: Int, val done: Int) {
    val fraction get() = if (total == 0) 0f else done.toFloat() / total
}

data class BoardFilter(
    /** "" = all work, [NO_EPIC] = beads outside any epic, else an epic id. */
    val epic: String = "",
    val needsYou: Boolean = false,
    val query: String = "",
    val types: Set<String> = emptySet(),
) {
    val isActive get() = epic.isNotEmpty() || needsYou || query.isNotBlank() || types.isNotEmpty()

    companion object {
        const val NO_EPIC = "__none"
    }
}

/** An indexed snapshot of one repo's beads with everything the board derives from it. */
class BoardIndex(val issues: List<Issue>) {
    val byId: Map<String, Issue> = issues.associateBy { it.id }

    private val children: Map<String, List<Issue>> = issues.filter { it.parent.isNotEmpty() }.groupBy { it.parent }

    private val dependents: Map<String, List<String>> = buildMap<String, MutableList<String>> {
        for (i in issues) for (b in i.blockerIds) getOrPut(b) { mutableListOf() } += i.id
    }

    /** Blockers that are not closed. Unknown ids (another repo, external refs) count as open. */
    fun openBlockers(issue: Issue): List<String> = issue.blockerIds.filter { byId[it]?.isClosed != true }

    fun laneOf(issue: Issue): Lane = when {
        issue.isClosed -> Lane.DONE
        issue.status == "in_progress" || issue.status == "hooked" -> Lane.DOING
        issue.status == "deferred" || issue.status == "pinned" -> Lane.LATER
        issue.status == "blocked" -> Lane.BLOCKED
        openBlockers(issue).isNotEmpty() -> Lane.BLOCKED
        else -> Lane.READY
    }

    fun childrenOf(id: String): List<Issue> = children[id].orEmpty()

    /** Beads that list [id] as a blocker. */
    fun dependentsOf(id: String): List<Issue> = dependents[id].orEmpty().mapNotNull { byId[it] }

    /** The outermost epic above a bead (or the bead itself when it is an epic). */
    fun epicOf(issue: Issue): Issue? {
        var cur: Issue? = issue
        var epic: Issue? = if (issue.isEpic) issue else null
        val seen = HashSet<String>()
        while (cur != null && cur.parent.isNotEmpty() && seen.add(cur.id)) {
            cur = byId[cur.parent]
            if (cur?.isEpic == true) epic = cur
        }
        return epic
    }

    /** "Epic › sub-parent" breadcrumb, omitting levels that are the bead itself. */
    fun breadcrumb(issue: Issue): String {
        val epic = epicOf(issue)?.takeIf { it.id != issue.id }
        val parent = byId[issue.parent]?.takeIf { it.id != epic?.id }
        return listOfNotNull(epic?.title, parent?.title).joinToString(" › ")
    }

    /** All descendants of an epic (not counting the epic). */
    fun descendants(id: String): List<Issue> {
        val out = mutableListOf<Issue>()
        val stack = ArrayDeque(childrenOf(id))
        val seen = HashSet<String>()
        while (stack.isNotEmpty()) {
            val c = stack.removeLast()
            if (!seen.add(c.id)) continue
            out += c
            stack.addAll(childrenOf(c.id))
        }
        return out
    }

    val epics: List<EpicSummary> by lazy {
        issues.filter { it.isEpic && !it.isClosed }.map { e ->
            val d = descendants(e.id)
            EpicSummary(e.id, e.title, d.size, d.count { it.isClosed })
        }.sortedWith(compareByDescending<EpicSummary> { it.total - it.done }.thenBy { it.id })
    }

    fun matches(issue: Issue, f: BoardFilter): Boolean {
        if (f.needsYou && !issue.needsYou) return false
        if (f.types.isNotEmpty() && issue.type !in f.types) return false
        when (f.epic) {
            "" -> Unit
            BoardFilter.NO_EPIC -> if (epicOf(issue) != null) return false
            else -> if (issue.id != f.epic && epicOf(issue)?.id != f.epic) return false
        }
        val q = f.query.trim().lowercase()
        if (q.isNotEmpty()) {
            val hay = buildString {
                append(issue.id).append(' ').append(issue.title).append(' ')
                append(issue.labels.joinToString(" ")).append(' ').append(issue.description)
            }.lowercase()
            if (!q.split(Regex("\\s+")).all { it in hay }) return false
        }
        return true
    }

    /**
     * The Workflow lane, with Proposals for beads that seed a change still being
     * planned. Only Workflow columns and stats use it; everything else keeps [laneOf].
     */
    fun workflowLane(issue: Issue, changes: ChangesList?): Lane =
        if (proposalOf(issue, changes) != null) Lane.PROPOSED else laneOf(issue)

    /** The change [issue] seeds while it is being planned, if any. */
    fun proposalOf(issue: Issue, changes: ChangesList?): ChangeSummary? {
        if (changes == null || !changes.openspec || issue.isClosed) return null
        return changes.changes.firstOrNull { c -> c.planning && issue.id in c.seedIds && issue.id != c.epicId }
    }

    fun columns(groupBy: GroupBy, filter: BoardFilter, doneLimit: Int = 25, changes: ChangesList? = null): List<Column> {
        val visible = issues.filter { matches(it, filter) }
        return when (groupBy) {
            GroupBy.WORKFLOW -> {
                val byLane = visible.groupBy { workflowLane(it, changes) }
                Lane.entries.filter { it != Lane.PROPOSED || changes?.openspec == true }.map { lane ->
                    val all = if (lane == Lane.PROPOSED) sortProposals(byLane[lane].orEmpty(), changes!!) else sortLane(lane, byLane[lane].orEmpty())
                    val shown = if (lane == Lane.DONE) all.take(doneLimit) else all
                    val note = if (lane == Lane.PROPOSED && changes!!.total > changes.changes.size) "Showing ${changes.changes.size} of ${changes.total} changes" else ""
                    Column(lane.key, lane.title, shown, all.size, note)
                }
            }
            GroupBy.PRIORITY -> {
                val open = visible.filterNot { it.isClosed }
                (0..4).map { p ->
                    val all = sortLane(Lane.READY, open.filter { it.priority == p })
                    Column("p$p", PRIORITY_NAMES[p], all, all.size)
                }
            }
            GroupBy.TYPE -> {
                val open = visible.filterNot { it.isClosed }
                val types = (DEFAULT_TYPES + open.map { it.type }).distinct()
                types.map { t ->
                    val all = sortLane(Lane.READY, open.filter { it.type == t })
                    Column("t:$t", t.replaceFirstChar { it.uppercase() }, all, all.size)
                }.filter { it.total > 0 || it.key.removePrefix("t:") in CORE_TYPES }
            }
        }
    }

    fun stats(filter: BoardFilter, changes: ChangesList? = null): Map<Lane, Int> =
        issues.filter { matches(it, filter.copy(needsYou = false)) }.groupingBy { workflowLane(it, changes) }.eachCount()

    /** Waiting on Ryder first, then being worked, then judged covered; newest first within each. */
    private fun sortProposals(list: List<Issue>, changes: ChangesList): List<Issue> {
        fun rank(c: ChangeSummary?) = when (c?.phase) {
            "awaiting_approval", "needs_attention", "stale", "interrupted" -> 0
            "covered" -> 2
            else -> 1
        }
        return list.map { it to proposalOf(it, changes) }
            .sortedWith(compareBy<Pair<Issue, ChangeSummary?>> { rank(it.second) }.thenByDescending { it.second?.updatedAt.orEmpty() })
            .map { it.first }
    }

    fun needsYouCount(filter: BoardFilter): Int =
        issues.count { it.needsYou && matches(it, filter.copy(needsYou = false)) }

    private fun sortLane(lane: Lane, list: List<Issue>): List<Issue> =
        if (lane == Lane.DONE) list.sortedWith(compareByDescending<Issue> { it.closedAt.ifEmpty { it.updatedAt } }.then(idOrder))
        else list.sortedWith(compareByDescending<Issue> { it.needsYou }.thenBy { it.priority }.then(idOrder))

    companion object {
        val PRIORITY_NAMES = listOf("P0 · Critical", "P1 · High", "P2 · Medium", "P3 · Low", "P4 · Backlog")
        val CORE_TYPES = listOf("task", "bug", "feature")
        val DEFAULT_TYPES = listOf("bug", "feature", "task", "chore", "epic", "decision", "spike", "story")

        /** Natural id order so NAC-2 sorts before NAC-10 and NAC-1.2 before NAC-1.10. */
        val idOrder = Comparator<Issue> { a, b -> naturalCompare(a.id, b.id) }

        fun naturalCompare(a: String, b: String): Int {
            val ra = Regex("\\d+|\\D+").findAll(a).map { it.value }.toList()
            val rb = Regex("\\d+|\\D+").findAll(b).map { it.value }.toList()
            for (i in 0 until minOf(ra.size, rb.size)) {
                val x = ra[i]; val y = rb[i]
                val c = if (x[0].isDigit() && y[0].isDigit()) x.length.compareTo(y.length).takeIf { it != 0 } ?: x.compareTo(y)
                else x.compareTo(y)
                if (c != 0) return c
            }
            return ra.size.compareTo(rb.size)
        }
    }
}

/** What dropping a bead into a column means. */
sealed interface MoveAction {
    data class SetStatus(val status: String) : MoveAction
    data object Close : MoveAction
    data class SetPriority(val priority: Int) : MoveAction
    data class SetType(val type: String) : MoveAction
}

fun moveActionFor(columnKey: String): MoveAction? = when {
    columnKey.startsWith("p") && columnKey.length == 2 -> MoveAction.SetPriority(columnKey[1].digitToInt())
    columnKey.startsWith("t:") -> MoveAction.SetType(columnKey.removePrefix("t:"))
    // Proposals move by approval, not by dragging.
    columnKey == Lane.PROPOSED.key -> null
    else -> Lane.entries.firstOrNull { it.key == columnKey }?.let { lane ->
        if (lane == Lane.DONE) MoveAction.Close else MoveAction.SetStatus(lane.status)
    }
}
