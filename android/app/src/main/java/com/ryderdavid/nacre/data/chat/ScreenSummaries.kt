package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.Round
import com.ryderdavid.nacre.data.model.Run
import com.ryderdavid.nacre.data.model.ScreenContext
import com.ryderdavid.nacre.ui.ScreenOrigin

/**
 * What each screen publishes into the [ScreenContextHolder]: its kind, repository, ids, selection,
 * open pane and a summary of up to 4 KB of what it shows.
 */
object ScreenSummaries {
    private fun issueLine(i: Issue) = "${i.id} [${i.type}, ${i.status}, P${i.priority}] ${i.title}" +
        if (i.labels.isEmpty()) "" else " (labels: ${i.labels.joinToString()})"

    /** Boards, Inbox or Activity: about every board on the computer. */
    fun allBoards(screen: String, lines: List<String>): ShownScreen = ShownScreen(
        ScreenContext(screen, summary = summaryOf("" to lines.joinToString("\n"))),
        null, ScreenOrigin.ALL_BOARDS,
    )

    /** A board, with the bead open in its right pane as the selection. */
    fun board(repoKey: String, name: String, issues: List<Issue>, selected: Issue?): ShownScreen {
        val open = issues.filter { !it.isClosed }
        return ShownScreen(
            ScreenContext(
                ScreenContext.BOARD, repo = Keys.id(repoKey), beadId = selected?.id, selection = selected?.id,
                pane = selected?.let { "bead detail" },
                summary = summaryOf(
                    "Board" to name,
                    "Selected" to selected?.let(::issueLine),
                    "Open beads" to "${open.size} (${open.count { it.status == "in_progress" }} in progress, ${open.count { it.status == "blocked" }} blocked)",
                    "" to open.take(40).joinToString("\n", transform = ::issueLine),
                ),
            ),
            repoKey, if (selected != null) selected.id else name.ifBlank { Keys.id(repoKey) },
        )
    }

    /** A bead, or its change view when [changeId] is set; [pane] names the open tab or artifact. */
    fun bead(
        repoKey: String,
        beadId: String,
        issue: Issue?,
        pane: String? = null,
        changeId: String? = null,
        changePhase: String? = null,
        changeRevision: String? = null,
        asChange: Boolean = changeId != null,
    ): ShownScreen {
        val change = asChange
        return ShownScreen(
            ScreenContext(
                if (change) ScreenContext.CHANGE else ScreenContext.BEAD,
                repo = Keys.id(repoKey), beadId = beadId, changeId = changeId, pane = pane,
                summary = summaryOf(
                    "Bead" to (issue?.let(::issueLine) ?: beadId),
                    "Change" to changeId,
                    "Phase" to changePhase,
                    "Revision" to changeRevision,
                    "Parent" to issue?.parent,
                    "Blocked by" to issue?.blockerIds?.joinToString(),
                    "Description" to issue?.description?.take(1500),
                ),
            ),
            repoKey, if (change) "change · $beadId" else beadId,
        )
    }

    /** A round: its state, stop reason and choices, with the expanded bead as the selection. */
    fun round(round: Round, selection: String? = null, pane: String? = null): ShownScreen {
        val stop = round.stop
        return ShownScreen(
            ScreenContext(
                ScreenContext.ROUND, repo = round.repoId.ifBlank { null }, roundId = round.id, selection = selection, pane = pane,
                summary = summaryOf(
                    "Round" to "${round.id} on ${round.repoName}",
                    "State" to round.state,
                    "Paused" to round.pauseReason,
                    "Stop reason" to stop?.reason,
                    "Stopped at" to stop?.let { listOf(it.phase, it.bead).filter(String::isNotBlank).joinToString(" · ") },
                    "Choices" to stop?.choices?.joinToString { it.label.ifBlank { it.id } },
                    "Epic" to round.epic,
                    "Beads" to round.beads.joinToString { "${it.id} (${it.state})" },
                    "Target" to round.target,
                ),
            ),
            round.repoId.takeIf { it.isNotBlank() }?.let { Keys.of(round.hostId, it) },
            "round ${round.id}" + (selection?.let { " · $it" } ?: ""),
        )
    }

    /** A run: its state and last error. */
    fun run(run: Run): ShownScreen = ShownScreen(
        ScreenContext(
            ScreenContext.RUN, repo = run.repoId.ifBlank { null }, runId = run.id, beadId = run.issueId.ifBlank { null },
            summary = summaryOf(
                "Run" to "${run.id} of ${run.issueId} ${run.issueTitle}",
                "State" to run.status,
                "Last error" to run.error,
                "Last output" to run.lastText.take(1000),
            ),
        ),
        run.repoId.takeIf { it.isNotBlank() }?.let { Keys.of(run.hostId, it) },
        "run ${run.id}" + (run.issueId.takeIf { it.isNotBlank() }?.let { " · $it" } ?: ""),
    )
}
