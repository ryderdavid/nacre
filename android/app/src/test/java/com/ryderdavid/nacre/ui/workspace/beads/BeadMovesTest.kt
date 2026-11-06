package com.ryderdavid.nacre.ui.workspace.beads

import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.board.Lane
import com.ryderdavid.nacre.data.board.MoveAction
import com.ryderdavid.nacre.data.board.moveActionFor
import com.ryderdavid.nacre.data.model.Dependency
import com.ryderdavid.nacre.data.model.Issue
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertIs
import kotlin.test.assertTrue

/** 9.2: moving beads. The drop dock, Move to… and the TalkBack action all go through [BeadMoves]. */
class BeadMovesTest {
    private fun issue(id: String, status: String = "open", blocks: List<String> = emptyList(), labels: List<String> = emptyList()) =
        Issue(id = id, title = "t $id", status = status, labels = labels, dependencies = blocks.map { Dependency(id, it, "blocks") })

    @Test fun proposalsAndToCheckAreNeverDropTargets() {
        for (from in Lane.entries) {
            val targets = BeadMoves.targets(from)
            assertFalse(Lane.PROPOSED in targets, "from $from")
            assertFalse(Lane.CHECK in targets, "from $from")
            assertFalse(from in targets, "a bead isn't offered its own lane")
        }
        assertEquals(listOf(Lane.READY, Lane.BLOCKED, Lane.LATER, Lane.DONE), BeadMoves.targets(Lane.DOING))
        assertIs<MoveOutcome.Refused>(BeadMoves.outcome(Lane.READY, Lane.PROPOSED))
        assertIs<MoveOutcome.Refused>(BeadMoves.outcome(Lane.READY, Lane.CHECK))
    }

    @Test fun proposalsAndBeadsInToCheckDontMove() {
        assertFalse(BeadMoves.canMove(Lane.PROPOSED))
        assertFalse(BeadMoves.canMove(Lane.CHECK))
        assertIs<MoveOutcome.Refused>(BeadMoves.outcome(Lane.PROPOSED, Lane.READY))
        assertIs<MoveOutcome.Refused>(BeadMoves.outcome(Lane.CHECK, Lane.DONE), "Done must not stand in for a sign-off")
        assertTrue(listOf(Lane.READY, Lane.DOING, Lane.BLOCKED, Lane.LATER, Lane.DONE).all(BeadMoves::canMove))
    }

    @Test fun dropsSetTheStatusAndDoneCloses() {
        assertEquals(MoveOutcome.Moves(MoveAction.Close), BeadMoves.outcome(Lane.DOING, Lane.DONE))
        assertEquals(MoveOutcome.Moves(MoveAction.SetStatus("open")), BeadMoves.outcome(Lane.DONE, Lane.READY))
        assertEquals(MoveOutcome.Moves(MoveAction.SetStatus("in_progress")), BeadMoves.outcome(Lane.READY, Lane.DOING))
        assertEquals(MoveOutcome.Moves(MoveAction.SetStatus("blocked")), BeadMoves.outcome(Lane.READY, Lane.BLOCKED))
        assertEquals(MoveOutcome.Moves(MoveAction.SetStatus("deferred")), BeadMoves.outcome(Lane.READY, Lane.LATER))
        assertEquals(MoveOutcome.Unchanged, BeadMoves.outcome(Lane.READY, Lane.READY))
        assertEquals("Done · closes", BeadMoves.targetLabel(Lane.DONE))
        // BoardModel's own mapping agrees: Proposals refuses, Done closes.
        assertEquals(null, moveActionFor(Lane.PROPOSED.key))
        assertEquals(MoveAction.Close, moveActionFor(Lane.DONE.key))
    }

    /** The repository's optimistic patch, as `NacreRepository.move` applies it before the server answers. */
    private fun apply(issue: Issue, action: MoveAction) = when (action) {
        is MoveAction.SetStatus -> issue.copy(status = action.status)
        MoveAction.Close -> issue.copy(status = "closed")
        else -> issue
    }

    @Test fun eachDropLandsTheBeadInThatLaneAndUndoPutsItBack() {
        val start = listOf(issue("A"), issue("D", status = "in_progress"), issue("X", status = "closed"))
        for (before in start) {
            val ix = BoardIndex(start)
            val from = ix.laneOf(before)
            for (to in BeadMoves.targets(from)) {
                val o = BeadMoves.outcome(from, to)
                assertIs<MoveOutcome.Moves>(o, "${before.id} $from → $to")
                val moved = apply(before, o.action)
                assertEquals(to, BoardIndex(start.map { if (it.id == before.id) moved else it }).laneOf(moved), "${before.id} dropped on $to")
                // Undo (the snackbar's action) restores the status the bead had, reopening a closed one.
                val undone = moved.copy(status = before.status)
                assertEquals(from, BoardIndex(start.map { if (it.id == before.id) undone else it }).laneOf(undone), "Undo of ${before.id} → $to")
            }
        }
    }

    @Test fun aBeadWithAnOpenBlockerDroppedOnReadyStillShowsBlocked() {
        val ix = BoardIndex(listOf(issue("A"), issue("B", status = "deferred", blocks = listOf("A"))))
        val o = BeadMoves.outcome(Lane.LATER, Lane.READY)
        assertIs<MoveOutcome.Moves>(o)
        val moved = apply(ix.byId["B"]!!, o.action)
        assertEquals(Lane.BLOCKED, BoardIndex(listOf(issue("A"), moved)).laneOf(moved), "Ready means nothing stands in the way")
    }
}
