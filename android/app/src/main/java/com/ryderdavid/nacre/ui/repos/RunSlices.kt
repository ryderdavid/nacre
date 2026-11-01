package com.ryderdavid.nacre.ui.repos

import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.Repo

/** The board bar's segments, in drawing order: doing, ready, blocked, later, done. */
internal const val BAR_DOING = 0
internal const val BAR_READY = 1
internal const val BAR_BLOCKED = 2
internal const val BAR_LATER = 3
internal const val BAR_DONE = 4

/** How many beads each segment of [StatusBar] stands for, indexed by the BAR_* constants. */
internal fun barCounts(c: Repo.Counts): List<Int> =
    listOf(c.inProgress + c.hooked, c.open, c.blocked, c.deferred + c.pinned, c.closed)

/** The segment a bead with [status] sits in; one not known to the bar is taken as doing. */
internal fun barSegment(status: String): Int = when (status) {
    "open" -> BAR_READY
    "blocked" -> BAR_BLOCKED
    "deferred", "pinned" -> BAR_LATER
    "closed" -> BAR_DONE
    else -> BAR_DOING
}

/**
 * Which ticket slices of the bar to light, segment → slice indexes, for the beads [running] has a
 * live run on. Each segment is split into one fixed-width slice per bead it counts; a bead's slice
 * is its place among that segment's beads in [issues], the board as last loaded. Until the board
 * has loaded ([issues] null), a running bead is taken as in progress and placed among the other
 * running ones. Slices beyond what [counts] says the segment holds are dropped.
 */
internal fun runSlices(counts: Repo.Counts, running: Set<String>, issues: List<Issue>?): Map<Int, Set<Int>> {
    if (running.isEmpty()) return emptyMap()
    val sizes = barCounts(counts)
    val bySegment: Map<Int, List<String>> = issues?.groupBy({ barSegment(it.status) }, { it.id }) ?: emptyMap()
    val known = issues?.mapTo(HashSet()) { it.id } ?: emptySet()
    val out = mutableMapOf<Int, MutableSet<Int>>()
    // Running beads the board doesn't list (not loaded yet, or only on another computer's copy).
    val unplaced = running.filter { it !in known }.sorted()
    for (id in running) {
        val seg = issues?.firstOrNull { it.id == id }?.let { barSegment(it.status) } ?: BAR_DOING
        val i = if (id in known) bySegment[seg].orEmpty().indexOf(id) else bySegment[seg].orEmpty().size + unplaced.indexOf(id)
        if (i in 0 until sizes[seg]) out.getOrPut(seg) { mutableSetOf() } += i
    }
    return out
}
