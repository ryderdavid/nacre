package com.ryderdavid.nacre.data.board

import com.ryderdavid.nacre.data.model.ChangeSummary
import com.ryderdavid.nacre.data.model.ChangesList
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.ui.rounds.roundCandidates
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ProposalsLaneTest {
    private fun issue(id: String, status: String = "open", type: String = "feature", labels: List<String> = emptyList()) =
        Issue(id = id, title = "t $id", status = status, type = type, labels = labels)

    private val board = BoardIndex(listOf(issue("S1"), issue("S2"), issue("S3"), issue("E", type = "epic"), issue("A"), issue("F", labels = listOf("found:c4", "found-amend")), issue("X")))
    private val changes = ChangesList(
        openspec = true, total = 7,
        changes = listOf(
            ChangeSummary("c1", "drafts", phase = "reviewing", seedIds = listOf("S1"), updatedAt = "2026-11-01T02:00:00Z"),
            ChangeSummary("c2", "chat", phase = "awaiting_approval", seedIds = listOf("S2"), updatedAt = "2026-11-01T01:00:00Z"),
            ChangeSummary("c3", "export", phase = "covered", seedIds = listOf("S3"), updatedAt = "2026-11-01T03:00:00Z"),
            // A promoted change being amended by a new seed: the seed plans, the epic stays put.
            ChangeSummary("c4", "rounds", phase = "drafting", epicId = "E", seedIds = listOf("E", "A")),
        ),
    )

    @Test fun proposalsLaneHoldsSeedsBeingPlannedWaitingOnRyderFirst() {
        val cols = board.columns(GroupBy.WORKFLOW, BoardFilter(), changes = changes)
        assertEquals(Lane.PROPOSED.key, cols.first().key)
        assertEquals(listOf("S2", "S1", "A", "S3"), cols.first().issues.map { it.id }, "awaiting approval first, then being worked newest first, then covered")
        assertEquals("Showing 4 of 7 changes", cols.first().note)
        assertTrue(cols.first { it.key == "ready" }.issues.any { it.id == "E" }, "the epic stays in its ordinary lane")
        assertEquals(4, board.stats(BoardFilter(), changes)[Lane.PROPOSED])
    }

    @Test fun noLaneWithoutOpenSpecOrChangeRecords() {
        assertFalse(board.columns(GroupBy.WORKFLOW, BoardFilter()).any { it.key == Lane.PROPOSED.key })
        assertFalse(board.columns(GroupBy.WORKFLOW, BoardFilter(), changes = changes.copy(openspec = false)).any { it.key == Lane.PROPOSED.key })
        assertEquals(Lane.READY, board.workflowLane(board.byId["S1"]!!, null))
    }

    @Test fun promotedAndClosedSeedsLeaveTheLane() {
        val promoted = changes.copy(changes = listOf(ChangeSummary("c1", "drafts", phase = "ready", epicId = "S1", seedIds = listOf("S1"))))
        assertEquals(Lane.READY, board.workflowLane(board.byId["S1"]!!, promoted))
        val closed = BoardIndex(listOf(issue("S1", status = "closed")))
        assertEquals(Lane.DONE, closed.workflowLane(closed.byId["S1"]!!, changes))
        val staleApproved = changes.copy(changes = listOf(ChangeSummary("c1", "drafts", phase = "stale", epicId = "E1", seedIds = listOf("S1"))))
        assertEquals(Lane.READY, board.workflowLane(board.byId["S1"]!!, staleApproved), "a stale approved change is implementation, not planning")
    }

    @Test fun proposalsAreNotADropTargetAndOtherCallersKeepLaneOf() {
        assertNull(moveActionFor(Lane.PROPOSED.key))
        assertEquals(Lane.READY, board.laneOf(board.byId["S1"]!!))
    }

    @Test fun roundsLeaveOutProposalsAndPendingAmendments() {
        val ids = roundCandidates(board, changes).map { it.id }
        assertFalse("S1" in ids || "A" in ids, "beads being planned aren't work yet")
        assertFalse("F" in ids, "a pending amendment is not runnable")
        assertTrue("X" in ids)
    }
}
