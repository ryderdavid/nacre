package com.ryderdavid.nacre.ui.workspace.beads

import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.board.Lane
import com.ryderdavid.nacre.data.model.BEAD_INTEGRATED
import com.ryderdavid.nacre.data.model.BEAD_PARKED
import com.ryderdavid.nacre.data.model.BEAD_SKIPPED
import com.ryderdavid.nacre.data.model.ChangeSummary
import com.ryderdavid.nacre.data.model.ChangesList
import com.ryderdavid.nacre.data.model.Comment
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.ROUND_CANCELLED
import com.ryderdavid.nacre.data.model.ROUND_LANDED
import com.ryderdavid.nacre.data.model.Round
import com.ryderdavid.nacre.data.model.Run
import com.ryderdavid.nacre.data.model.foundChange
import com.ryderdavid.nacre.ui.workspace.ChatReference

/** A bead this one relates to, as the peek lists it. */
data class PeekDependency(val id: String, val title: String, val done: Boolean, val known: Boolean = true) {
    /** Not color alone: the state is in words. */
    val stateText get() = when { !known -> "not on this board"; done -> "done"; else -> "open" }
}

enum class PeekRunState(val text: String) { RUNNING("Working"), NEEDS_YOU("Needs you"), FAILED("Stopped"), DONE("Finished") }

/** The bead's latest run: a round it is part of, or a run of it on its own. Opens the work inspector. */
data class PeekRun(
    val kind: Kind,
    /** The round's or run's key, for the work inspector. */
    val key: String,
    val title: String,
    val detail: String,
    val state: PeekRunState,
    /** When it last moved, for "14 min ago". */
    val at: String,
) {
    enum class Kind { ROUND, RUN }
}

/** The change a bead belongs to, for Open change. */
data class PeekChange(val id: String, val label: String)

/**
 * Start work: a one-bead round that stops before landing. When a round is already working on the bead,
 * the button names it and opens it instead ([opensRound]).
 */
data class PeekStartWork(val label: String, val note: String, val opensRound: String? = null)

/** Everything the bead peek shows; read-only, since editing a bead is asked for in chat. */
data class BeadPeek(
    val id: String,
    val title: String,
    val lane: Lane,
    val priority: Int,
    val type: String,
    val needsYou: Boolean,
    val awaitsSignoff: Boolean,
    val description: String,
    val acceptance: List<String>,
    val waitsOn: List<PeekDependency>,
    val blocks: List<PeekDependency>,
    val parent: PeekDependency?,
    val comments: List<Comment>,
    val commentCount: Int,
    val latestRun: PeekRun?,
    val change: PeekChange?,
    val startWork: PeekStartWork?,
) {
    /** What Ask about this attaches to the composer, and what the pane is viewing. */
    val reference get() = beadReference(id)
}

fun beadReference(id: String) = ChatReference(kind = "bead", id = id, label = id)

object BeadPeekModel {
    const val STOPS_BEFORE_LANDING = "Stops before landing"

    fun build(
        issue: Issue,
        index: BoardIndex,
        changes: ChangesList?,
        comments: List<Comment>,
        runs: List<Run>,
        rounds: List<Round>,
    ): BeadPeek {
        val shownComments = comments.filterNot { it.text.startsWith("[nacre-") }
        val active = rounds.firstOrNull { r -> r.isActive && r.beads.any { it.id == issue.id } }
        return BeadPeek(
            id = issue.id,
            title = issue.title,
            lane = index.workflowLane(issue, changes),
            priority = issue.priority,
            type = issue.type,
            needsYou = issue.needsYou,
            awaitsSignoff = issue.awaitsSignoff,
            description = issue.description.trim(),
            acceptance = acceptanceItems(issue.acceptance),
            waitsOn = issue.blockerIds.map { dep(it, index) },
            blocks = index.dependentsOf(issue.id).map { PeekDependency(it.id, it.title, it.isClosed) },
            parent = issue.parent.takeIf { it.isNotBlank() }?.let { dep(it, index) },
            comments = shownComments,
            commentCount = maxOf(shownComments.size, issue.commentCount),
            latestRun = latestRun(issue.id, runs, rounds),
            change = changeOf(issue, changes)?.let { c -> PeekChange(c.id, changeLabel(c, issue.id)) },
            startWork = startWork(issue, active),
        )
    }

    private fun dep(id: String, index: BoardIndex): PeekDependency =
        index.byId[id]?.let { PeekDependency(id, it.title, it.isClosed) } ?: PeekDependency(id, "", done = false, known = false)

    /** Acceptance criteria as items: one per line, list markers and checkboxes removed. */
    fun acceptanceItems(text: String): List<String> =
        text.lines().map { it.trim().replace(Regex("^([-*+]\\s+(\\[[ xX]]\\s+)?|\\d+[.)]\\s+|\\[[ xX]]\\s+)"), "").trim() }.filter { it.isNotEmpty() }

    /** The change the bead belongs to: as its epic, a section, found work, or a seed of a proposal. */
    fun changeOf(issue: Issue, changes: ChangesList?): ChangeSummary? {
        val list = changes?.takeIf { it.openspec }?.changes ?: return null
        return list.firstOrNull { c -> c.epicId == issue.id || c.sections.any { it.beadId == issue.id } || (issue.foundChange.isNotEmpty() && c.id == issue.foundChange) }
            ?: list.firstOrNull { issue.id in it.seedIds }
    }

    /** "offline-chat-resume · 2.3" for a section's bead, else the change's name. */
    fun changeLabel(c: ChangeSummary, beadId: String): String =
        c.sections.firstOrNull { it.beadId == beadId }?.let { "${c.name} · ${it.id}" } ?: c.name

    fun startWork(issue: Issue, activeRound: Round?): PeekStartWork? = when {
        // A closed bead, one waiting on sign-off, or an epic (worked as a whole from the change) has nothing to start.
        issue.isClosed || issue.awaitsSignoff || issue.isEpic -> null
        activeRound != null -> PeekStartWork(if (activeRound.needsYou) "Needs you" else "Running", "Open its round", opensRound = activeRound.key)
        else -> PeekStartWork("Start work", STOPS_BEFORE_LANDING)
    }

    /** The most recent of the rounds and runs that worked on [beadId]. */
    fun latestRun(beadId: String, runs: List<Run>, rounds: List<Round>): PeekRun? {
        val fromRounds = rounds.mapNotNull { r ->
            val b = r.beads.firstOrNull { it.id == beadId } ?: return@mapNotNull null
            val state = when {
                r.needsYou -> PeekRunState.NEEDS_YOU
                b.state == BEAD_PARKED || r.stop != null || r.error.isNotBlank() -> PeekRunState.FAILED
                b.state == BEAD_INTEGRATED || b.state == BEAD_SKIPPED || r.state == ROUND_LANDED || r.state == ROUND_CANCELLED -> PeekRunState.DONE
                else -> PeekRunState.RUNNING
            }
            val title = when (state) {
                PeekRunState.RUNNING -> "Working in round ${r.id}"
                PeekRunState.NEEDS_YOU -> "Round ${r.id} needs you"
                PeekRunState.FAILED -> "Stopped in round ${r.id}"
                PeekRunState.DONE -> if (r.state == ROUND_CANCELLED) "Round ${r.id} cancelled" else "Finished in round ${r.id}"
            }
            val detail = b.reason.ifBlank { r.stop?.reason.orEmpty() }.ifBlank { r.error }
            PeekRun(PeekRun.Kind.ROUND, r.key, title, detail, state, r.updatedAt.ifBlank { r.createdAt })
        }
        val fromRuns = runs.filter { it.issueId == beadId }.map { run ->
            val state = when {
                run.isActive -> PeekRunState.RUNNING
                run.status == "failed" || run.error.isNotBlank() -> PeekRunState.FAILED
                else -> PeekRunState.DONE
            }
            val title = when (state) {
                PeekRunState.RUNNING -> "Run working"
                PeekRunState.FAILED -> "Run failed"
                else -> "Run ${run.status.replace('_', ' ')}"
            }
            PeekRun(PeekRun.Kind.RUN, run.key, title, run.error.ifBlank { run.lastText }, state, run.endedAt.ifBlank { run.startedAt }.ifBlank { run.createdAt })
        }
        return (fromRounds + fromRuns).maxByOrNull { it.at }
    }
}
