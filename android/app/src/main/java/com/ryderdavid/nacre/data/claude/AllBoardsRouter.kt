package com.ryderdavid.nacre.data.claude

import com.ryderdavid.nacre.data.NacreRepository
import com.ryderdavid.nacre.data.RepoGroup
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.settings.SettingsStore
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.withTimeoutOrNull

/**
 * [BoardRouter] over every board on the boards screen, addressed by board name. Each
 * call lands on the same [RepoBoardTools] the single-board assistant uses, so a write
 * from here goes through the same approval flow and the same bridge call.
 */
class AllBoardsRouter(
    private val repo: NacreRepository,
    private val settings: SettingsStore,
) : BoardRouter {
    override val only: BoardTools? = null

    /** Boards as the boards screen groups them; waits for the first scan if it hasn't landed. */
    private suspend fun boards(): List<RepoGroup> {
        repo.groups.value.takeIf { it.isNotEmpty() }?.let { return it }
        repo.refreshRepos().join()
        return withTimeoutOrNull(SCAN_WAIT_MS) { repo.groups.first { it.isNotEmpty() } }.orEmpty()
    }

    private fun toolsFor(g: RepoGroup) = RepoBoardTools(repo, g.home.key, g.home.name)

    private fun RepoGroup.label() = "${home.name} (${home.path.replace(Regex("^/Users/[^/]+"), "~")})"

    override suspend fun resolve(board: String?): BoardTools {
        val all = boards()
        if (all.isEmpty()) throw IllegalStateException("No boards are reachable. Ryder needs to pair a computer in Nacre's settings.")
        val choices = "Boards: " + all.joinToString("; ") { it.label() }
        if (board.isNullOrBlank()) throw IllegalArgumentException("This call needs a board. $choices")
        val hits = matchBoards(board, all)
        return when (hits.size) {
            0 -> throw IllegalArgumentException("No board called \"$board\". $choices")
            1 -> toolsFor(hits.single())
            else -> throw IllegalArgumentException("\"$board\" matches several boards: ${hits.joinToString("; ") { it.label() }}. Use one name exactly.")
        }
    }

    override suspend fun listBoards(query: String?): String {
        val hidden = settings.current().hiddenRepos
        val rows = boards().filter { g ->
            query.isNullOrBlank() || g.home.name.contains(query, true) || g.home.path.contains(query, true) || g.home.gitRemote.contains(query, true)
        }
        if (rows.isEmpty()) return if (query.isNullOrBlank()) "No boards are reachable." else "No board matches \"$query\"."
        return rows.sortedBy { it.home.name.lowercase() }.joinToString("\n") { g ->
            val r = g.home
            val c = r.counts
            val runs = g.members.sumOf { it.activeRuns }
            buildString {
                append(r.name).append(" — ").append(c.open).append(" open")
                if (c.inProgress > 0) append(", ").append(c.inProgress).append(" in progress")
                if (c.blocked > 0) append(", ").append(c.blocked).append(" blocked")
                if (c.deferred > 0) append(", ").append(c.deferred).append(" later")
                append(", ").append(c.closed).append(" done")
                if (runs > 0) append("; ").append(runs).append(" Claude Code run(s) going")
                append("; on ").append(g.members.joinToString { it.hostName.ifBlank { "a computer" } })
                append("; ").append(r.path.replace(Regex("^/Users/[^/]+"), "~"))
                if (g.id in hidden) append("; hidden on the boards screen")
                if (r.error.isNotBlank()) append("; error: ").append(r.error)
            }
        }
    }

    /**
     * With no board, reads the few boards [planSweep] picks — each one a full board fetch
     * over the bridge — and ends with [sweepNote] naming everything it didn't read.
     */
    override suspend fun listBeads(board: String?, lane: String?, query: String?, limit: Int): String {
        if (!board.isNullOrBlank()) return resolve(board).listBeads(lane, query, limit)
        val plan = planSweep(boards(), settings.current().hiddenRepos, lane)
        if (plan.targets.isEmpty()) return "No boards have beads to show." + sweepNote(plan)
        val per = limit.coerceIn(1, 200).coerceAtMost(ROWS_PER_BOARD)
        val sections = coroutineScope {
            plan.targets.map { g ->
                async { g.home.name to runCatching { toolsFor(g).listBeads(lane, query, per) }.getOrElse { "(couldn't read this board: ${it.friendly()})" } }
            }.awaitAll()
        }
        val body = sections.filterNot { it.second == "No beads match." }.joinToString("\n\n") { (name, rows) -> "## $name\n$rows" }
        return body.ifBlank { "No beads match on the boards read." } + sweepNote(plan)
    }

    /** What a board-less list_beads reads, and what it leaves out. */
    internal data class SweepPlan(val targets: List<RepoGroup>, val notRead: List<RepoGroup>, val hidden: List<RepoGroup>)

    companion object {
        /**
         * The boards [name] could mean: an exact name wins outright, otherwise anything
         * whose name or path contains it (or whose key it is) counts as a candidate.
         */
        internal fun matchBoards(name: String, boards: List<RepoGroup>): List<RepoGroup> =
            boards.filter { it.home.name.equals(name, ignoreCase = true) }
                .ifEmpty { boards.filter { it.home.name.contains(name, true) || it.home.path.contains(name, true) || it.home.key == name } }

        /**
         * Picks the boards a sweep reads from the scan's counts alone, without fetching any:
         * hidden, errored and empty boards are out (and, for a lane of open work, boards with
         * nothing open), then the most recently updated [MAX_BOARDS_SWEPT] win.
         */
        internal fun planSweep(boards: List<RepoGroup>, hiddenIds: Set<String>, lane: String?): SweepPlan {
            val (hidden, shown) = boards.partition { it.id in hiddenIds }
            val candidates = shown
                .filter { it.home.error.isBlank() && it.home.counts.total > 0 }
                .filter { lane !in OPEN_LANES || it.home.counts.active > 0 }
                .sortedByDescending { it.home.lastUpdated }
            return SweepPlan(candidates.take(MAX_BOARDS_SWEPT), candidates.drop(MAX_BOARDS_SWEPT), hidden.sortedBy { it.home.name.lowercase() })
        }

        /** The trailing note on a sweep: says plainly when it was cut short, and which hidden boards it skipped. */
        internal fun sweepNote(plan: SweepPlan): String = buildString {
            if (plan.notRead.isNotEmpty()) {
                val read = plan.targets.size
                append("\n\n(Sweep cut short: read only the $read most recently updated of ${read + plan.notRead.size} boards with matching beads. ")
                append("Not read: ").append(plan.notRead.joinToString { it.home.name }).append(". Name a board to read it.)")
            }
            if (plan.hidden.isNotEmpty()) {
                append("\n\n(Plus ${plan.hidden.size} hidden: ").append(plan.hidden.joinToString { it.home.name })
                append(". Hidden boards are left out of sweeps; name one to read it.)")
            }
        }

        /** Lanes that only hold beads which aren't done, so empty boards can be skipped. */
        val OPEN_LANES = setOf("ready", "doing", "blocked", "open")
        /** Boards one sweep fetches at most; the rest are named in [sweepNote] for the assistant to pick from. */
        const val MAX_BOARDS_SWEPT = 5
        const val ROWS_PER_BOARD = 25
        const val SCAN_WAIT_MS = 8_000L
    }
}
