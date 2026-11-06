package com.ryderdavid.nacre.ui.workspace.beads

import com.ryderdavid.nacre.data.board.BoardFilter
import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.board.Lane
import com.ryderdavid.nacre.data.board.MoveAction
import com.ryderdavid.nacre.data.board.moveActionFor

/**
 * The Beads panel's geometry, from the beads-board spec: lanes show as columns at least 272dp wide when the
 * pane has room for two of them (a wide or focused pane), and one lane at a time otherwise. Widths in dp.
 */
object BeadsLayout {
    const val COLUMN_MIN = 272
    const val COLUMN_MAX = 360
    /** Between columns. */
    const val GAP = 12
    /** Either side of the row of columns. */
    const val EDGE = 16

    /** The narrowest pane that shows columns: two minimum columns, the gap between them and both edges. */
    const val COLUMNS_MIN_WIDTH = 2 * COLUMN_MIN + GAP + 2 * EDGE

    fun usesColumns(widthDp: Int): Boolean = widthDp >= COLUMNS_MIN_WIDTH

    /** How many whole columns of at least [COLUMN_MIN] fit side by side. */
    fun columnsThatFit(widthDp: Int): Int = ((widthDp - 2 * EDGE + GAP) / (COLUMN_MIN + GAP)).coerceAtLeast(1)

    /**
     * Each column's width: the columns that fit share the pane, never narrower than [COLUMN_MIN] nor wider
     * than [COLUMN_MAX]; lanes beyond them scroll sideways.
     */
    fun columnWidth(widthDp: Int, lanes: Int): Int {
        val shown = minOf(columnsThatFit(widthDp), lanes.coerceAtLeast(1))
        return ((widthDp - 2 * EDGE - GAP * (shown - 1)) / shown).coerceIn(COLUMN_MIN, COLUMN_MAX)
    }

    /** "Lanes 1–3 of 7" for the header of a row that scrolls; empty when every lane is in view. */
    fun rangeLabel(first: Int, visible: Int, total: Int): String {
        if (total <= visible || total == 0) return ""
        val from = (first + 1).coerceIn(1, total)
        val to = (first + visible).coerceIn(from, total)
        return "Lanes $from–$to of $total"
    }
}

/** What a drop or a Move to… does. */
sealed interface MoveOutcome {
    /** Proposals and To check aren't drop targets; [reason] says why. */
    data class Refused(val reason: String) : MoveOutcome
    /** Already in that lane: nothing happens. */
    data object Unchanged : MoveOutcome
    /** Apply [action] through the repository, whose notice offers Undo. A drop on Done closes the bead. */
    data class Moves(val action: MoveAction) : MoveOutcome
}

/** Moving beads between lanes: which lanes take a drop, and what a drop means. */
object BeadMoves {
    /** Lanes that never take a drop: proposals move by approval, To check by sign-off. */
    val refusing = setOf(Lane.PROPOSED, Lane.CHECK)

    /** The drop targets, in board order, for a bead coming from [from]. */
    fun targets(from: Lane?): List<Lane> = Lane.entries.filter { it !in refusing && it != from }

    fun accepts(lane: Lane): Boolean = lane !in refusing

    /**
     * A bead can be picked up unless it is a proposal (those move by approval) or waits in To check (it
     * leaves when Ryder signs it off or sends it back, which a drop on Done must never stand in for).
     */
    fun canMove(from: Lane): Boolean = from !in refusing

    fun outcome(from: Lane, to: Lane): MoveOutcome = when {
        to == Lane.PROPOSED -> MoveOutcome.Refused("Proposals move by approval, not by dragging")
        to == Lane.CHECK -> MoveOutcome.Refused("To check fills when work waits on your sign-off")
        from == Lane.PROPOSED -> MoveOutcome.Refused("Proposals move by approval, not by dragging")
        from == Lane.CHECK -> MoveOutcome.Refused("Beads in To check move when you sign them off")
        to == from -> MoveOutcome.Unchanged
        else -> {
            moveActionFor(to.key)?.let { MoveOutcome.Moves(it) } ?: MoveOutcome.Refused("${to.title} doesn't take beads")
        }
    }

    /** The label a target shows: Done says that it closes the bead (it never means verified or signed off). */
    fun targetLabel(lane: Lane): String = if (lane == Lane.DONE) "Done · closes" else lane.title

    fun laneByKey(key: String?): Lane? = Lane.entries.firstOrNull { it.key == key }
}

/** The board's search and filters, said in words, so the panel can say what they hide. */
object BeadsFilters {
    /** Filter chips offered for types. */
    val TYPES = listOf("bug" to "Bugs", "feature" to "Features", "task" to "Tasks")

    /** Words for each active filter, in the order the chips show. */
    fun parts(filter: BoardFilter, epicTitle: (String) -> String?): List<String> = buildList {
        when (filter.epic) {
            "" -> Unit
            BoardFilter.NO_EPIC -> add("not in an epic")
            else -> add(epicTitle(filter.epic) ?: filter.epic)
        }
        if (filter.needsYou) add("Needs you")
        TYPES.filter { it.first in filter.types }.forEach { add(it.second) }
        (filter.types - TYPES.map { it.first }.toSet()).sorted().forEach { add(it) }
        if (filter.query.isNotBlank()) add("“${filter.query.trim()}”")
    }

    /** How many beads the filter hides. */
    fun hidden(index: BoardIndex, filter: BoardFilter): Int =
        if (!filter.isActive) 0 else index.issues.count { !index.matches(it, filter) }

    /**
     * The note above the board while a filter is on, e.g. "12 hidden by Needs you, Bugs"; null when no
     * filter is on.
     */
    fun note(index: BoardIndex, filter: BoardFilter): String? {
        if (!filter.isActive) return null
        val by = parts(filter) { id -> index.byId[id]?.title }
        val n = hidden(index, filter)
        val what = if (n == 1) "1 bead hidden" else "$n beads hidden"
        return "$what by ${by.joinToString(", ")}"
    }
}
