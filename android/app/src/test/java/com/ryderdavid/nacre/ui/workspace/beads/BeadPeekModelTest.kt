package com.ryderdavid.nacre.ui.workspace.beads

import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.board.Lane
import com.ryderdavid.nacre.data.model.BEAD_PARKED
import com.ryderdavid.nacre.data.model.BEAD_RUNNING
import com.ryderdavid.nacre.data.model.ChangeSummary
import com.ryderdavid.nacre.data.model.ChangesList
import com.ryderdavid.nacre.data.model.Comment
import com.ryderdavid.nacre.data.model.Dependency
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.ROUND_IMPLEMENTING
import com.ryderdavid.nacre.data.model.ROUND_LANDED
import com.ryderdavid.nacre.data.model.ROUND_PAUSED
import com.ryderdavid.nacre.data.model.Round
import com.ryderdavid.nacre.data.model.RoundBead
import com.ryderdavid.nacre.data.model.RoundStop
import com.ryderdavid.nacre.data.model.Run
import com.ryderdavid.nacre.data.model.SectionSummary
import com.ryderdavid.nacre.ui.workspace.ChatReference
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/** 9.3: what the bead peek shows, and that it follows the board. */
class BeadPeekModelTest {
    private val axe3 = Issue(
        id = "NAC-axe.3", title = "Reattach the event stream after a drop", status = "in_progress", priority = 1,
        description = "When the connection drops mid-reply, the app reopens the stream.",
        acceptance = "- [ ] A 30 s drop resumes with no gap\n* TestResumeAfterDrop passes under -race\n\n",
        parent = "NAC-axe",
        dependencies = listOf(Dependency("NAC-axe.3", "NAC-axe.2", "blocks"), Dependency("NAC-axe.3", "OTHER-1", "blocks"), Dependency("NAC-axe.3", "NAC-axe", "parent-child")),
        commentCount = 2,
    )
    private val board = listOf(
        Issue(id = "NAC-axe", title = "Offline chat resume", type = "epic"),
        Issue(id = "NAC-axe.2", title = "Event ids", status = "closed"),
        axe3,
        Issue(id = "NAC-axe.5", title = "Manual device check", dependencies = listOf(Dependency("NAC-axe.5", "NAC-axe.3", "blocks"))),
    )
    private val changes = ChangesList(
        openspec = true, total = 1,
        changes = listOf(ChangeSummary("c1", "offline-chat-resume", phase = "approved", epicId = "NAC-axe", sections = listOf(SectionSummary("2.3", beadId = "NAC-axe.3")))),
    )

    private fun round(id: String, state: String, bead: RoundBead, updated: String, stop: RoundStop? = null) =
        Round(id = id, state = state, beads = listOf(bead), updatedAt = updated, stop = stop, hostId = "h")

    @Test fun showsDescriptionAcceptanceDependenciesAndComments() {
        val comments = listOf(Comment("1", text = "Looks flaky on Wi-Fi"), Comment("2", text = "[nacre-round] internal"))
        val p = BeadPeekModel.build(axe3, BoardIndex(board), changes, comments, emptyList(), emptyList())
        assertEquals(Lane.DOING, p.lane)
        assertEquals("When the connection drops mid-reply, the app reopens the stream.", p.description)
        assertEquals(listOf("A 30 s drop resumes with no gap", "TestResumeAfterDrop passes under -race"), p.acceptance)
        assertEquals(listOf(PeekDependency("NAC-axe.2", "Event ids", done = true), PeekDependency("OTHER-1", "", done = false, known = false)), p.waitsOn)
        assertEquals("not on this board", p.waitsOn[1].stateText)
        assertEquals(listOf("NAC-axe.5"), p.blocks.map { it.id })
        assertEquals("NAC-axe", p.parent?.id)
        assertEquals(listOf("Looks flaky on Wi-Fi"), p.comments.map { it.text }, "Nacre's own bookkeeping comments are left out")
        assertEquals(2, p.commentCount)
    }

    @Test fun askAboutThisAttachesTheBead() {
        val p = BeadPeekModel.build(axe3, BoardIndex(board), changes, emptyList(), emptyList(), emptyList())
        assertEquals(ChatReference("bead", "NAC-axe.3", "NAC-axe.3"), p.reference)
    }

    @Test fun openChangeNamesTheChangeAndSection() {
        val p = BeadPeekModel.build(axe3, BoardIndex(board), changes, emptyList(), emptyList(), emptyList())
        assertEquals(PeekChange("c1", "offline-chat-resume · 2.3"), p.change)
        val epic = BeadPeekModel.build(board[0], BoardIndex(board), changes, emptyList(), emptyList(), emptyList())
        assertEquals(PeekChange("c1", "offline-chat-resume"), epic.change)
        assertNull(BeadPeekModel.build(board[3], BoardIndex(board), changes, emptyList(), emptyList(), emptyList()).change)
        assertNull(BeadPeekModel.changeOf(axe3, changes.copy(openspec = false)), "no OpenSpec, no change")
    }

    @Test fun startWorkStopsBeforeLandingOrOpensTheRoundAlreadyWorking() {
        assertEquals(PeekStartWork("Start work", BeadPeekModel.STOPS_BEFORE_LANDING), BeadPeekModel.startWork(axe3, null))
        val r = round("r1", ROUND_IMPLEMENTING, RoundBead("NAC-axe.3", state = BEAD_RUNNING), "2026-11-06T10:00:00Z")
        assertEquals(Keys.of("h", "r1"), BeadPeekModel.startWork(axe3, r)?.opensRound)
        assertEquals("Running", BeadPeekModel.startWork(axe3, r)?.label)
        assertEquals("Needs you", BeadPeekModel.startWork(axe3, r.copy(state = ROUND_PAUSED))?.label)
        assertNull(BeadPeekModel.startWork(axe3.copy(status = "closed"), null), "nothing to start on a closed bead")
        assertNull(BeadPeekModel.startWork(axe3.copy(labels = listOf("signoff")), null), "nor on one waiting on sign-off")
        assertNull(BeadPeekModel.startWork(board[0], null), "an epic is worked from its change")
    }

    @Test fun theLatestRunIsTheNewestRoundOrRun() {
        val stopped = round("rdto3k", ROUND_PAUSED, RoundBead("NAC-axe.3", state = BEAD_PARKED, reason = "TestResumeAfterDrop timed out"), "2026-11-06T10:00:00Z", RoundStop(reason = "gates failed"))
        val older = round("r0", ROUND_LANDED, RoundBead("NAC-axe.3"), "2026-11-01T10:00:00Z")
        val run = Run(id = "run1", issueId = "NAC-axe.3", status = "done", createdAt = "2026-11-02T10:00:00Z", hostId = "h")
        val latest = BeadPeekModel.latestRun("NAC-axe.3", listOf(run), listOf(older, stopped))!!
        assertEquals(PeekRun.Kind.ROUND, latest.kind)
        assertEquals(Keys.of("h", "rdto3k"), latest.key)
        // A paused round waits on Ryder: that leads over the parked bead.
        assertEquals(PeekRunState.NEEDS_YOU, latest.state)
        assertEquals("TestResumeAfterDrop timed out", latest.detail)
        val failed = BeadPeekModel.latestRun("NAC-axe.3", listOf(run), listOf(stopped.copy(state = ROUND_IMPLEMENTING)))!!
        assertEquals(PeekRunState.FAILED, failed.state)
        assertEquals("Stopped in round rdto3k", failed.title)
        val justRun = BeadPeekModel.latestRun("NAC-axe.3", listOf(run, run.copy(id = "run2", status = "running", createdAt = "2026-11-03T10:00:00Z")), listOf(older))!!
        assertEquals(Keys.of("h", "run2"), justRun.key)
        assertEquals(PeekRunState.RUNNING, justRun.state)
        assertNull(BeadPeekModel.latestRun("NAC-axe.5", listOf(run), listOf(older)))
    }

    @Test fun thePeekFollowsTheBoard() {
        // The agent closes the bead from chat: the next snapshot moves it to Done and Start work goes away.
        val closed = axe3.copy(status = "closed")
        val p = BeadPeekModel.build(closed, BoardIndex(board.map { if (it.id == closed.id) closed else it }), changes, emptyList(), emptyList(), emptyList())
        assertEquals(Lane.DONE, p.lane)
        assertNull(p.startWork)
    }
}
