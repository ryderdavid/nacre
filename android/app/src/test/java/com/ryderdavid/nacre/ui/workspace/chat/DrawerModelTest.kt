package com.ryderdavid.nacre.ui.workspace.chat

import com.ryderdavid.nacre.data.BoardSnapshot
import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.model.ChangeSummary
import com.ryderdavid.nacre.data.model.ChangesList
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.data.model.ChatWorkLink
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.Repo
import com.ryderdavid.nacre.data.model.SectionSummary
import com.ryderdavid.nacre.data.model.WorkSummary
import java.time.Instant
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

class DrawerModelTest {
    private val host = "c0"
    private val repoKey = Keys.of(host, "nacre")

    private fun chat(id: String, updated: String, vararg links: ChatWorkLink, waiting: Int = 0) =
        ChatRecord(id = id, repo = "nacre", title = "Chat $id", updatedAt = updated, work = links.toList(), hostId = host, waitingCount = waiting, lastLine = "last $id")

    private val board = BoardSnapshot(
        Repo(id = "nacre", name = "Nacre", path = "/x/nacre", hostId = host, hostName = host),
        BoardIndex(
            listOf(
                Issue(id = "E", title = "Board is the UI", type = "epic", status = "in_progress"),
                Issue(id = "E.1", title = "One", parent = "E", status = "closed"),
                Issue(id = "E.2", title = "Two", parent = "E", status = "in_progress"),
                Issue(id = "E.3", title = "Three", parent = "E"),
                Issue(id = "B", title = "A bug", type = "bug"),
                Issue(id = "X", title = "Shipped", status = "closed"),
            ),
        ),
        ChangesList(
            changes = listOf(
                ChangeSummary(
                    id = "ch1", changeName = "jobs-tab", phase = "promoting", tasksDone = 3, tasksTotal = 8,
                    sections = listOf(SectionSummary("s1", "Model", beadId = "E.1"), SectionSummary("s2", "Drawer", beadId = "E.2")),
                ),
                ChangeSummary(id = "ch2", changeName = "untouched", phase = "draft", tasksDone = 0, tasksTotal = 4),
            ),
        ),
    )

    @Test fun chatCardsPutWaitingAndWorkingFirstAndCarryTheirJob() {
        val w = WorkSummary(kind = "round", id = "r1", change = "jobs-tab", state = "running", landed = 2, total = 5, chatId = "b")
        val cards = ChatCards.build(
            listOf(
                chat("a", "2026-11-07T12:00:00Z"),
                chat("b", "2026-11-07T09:00:00Z", ChatWorkLink("change", "nacre", "ch1")),
                chat("c", "2026-11-07T08:00:00Z", ChatWorkLink("bead", "nacre", "B"), waiting = 1),
            ),
            repoKey, setOf(repoKey), listOf(w), board,
        )
        assertEquals(listOf("b", "c", "a"), cards.map { Keys.id(it.key) })
        val b = cards.first()
        assertEquals("jobs-tab", b.job?.label)
        assertEquals(2 to 5, b.job!!.done to b.job!!.total)
        assertTrue(b.running)
        assertEquals("B", cards[1].job?.label)
        assertNull(cards[2].job)
        assertEquals("last a", cards[2].lastLine)
    }

    @Test fun jobsAreOnePerItemWithTheirChatsAndSteps() {
        val jobs = WorkJobsModel.build(
            listOf(
                chat("a", "2026-11-07T12:00:00Z", ChatWorkLink("change", "nacre", "ch1")),
                chat("b", "2026-11-07T11:00:00Z", ChatWorkLink("change", "nacre", "ch1")),
                chat("c", "2026-11-07T10:00:00Z", ChatWorkLink("bead", "nacre", "X")),
            ),
            repoKey, setOf(repoKey), emptyList(), board,
        )
        val change = jobs.now.single { it.key == "change:ch1" }
        assertEquals(listOf("Chat a", "Chat b"), change.chats.map { it.second })
        assertEquals(listOf(StepState.DONE, StepState.ACTIVE), change.steps.map { it.state })
        assertEquals("3 of 8 tasks", change.status)
        // The epic is in progress with no chat on it: it can have one started.
        val epic = jobs.now.single { it.key == "bead:E" }
        assertEquals("epic", epic.kind)
        assertEquals(1 to 3, epic.done to epic.total)
        assertTrue(epic.chats.isEmpty())
        assertEquals("bead", epic.reference?.kind)
        // A closed bead a chat worked on is done; a change nobody started isn't listed.
        assertEquals(listOf("bead:X"), jobs.done.map { it.key })
        assertTrue(jobs.now.none { it.key == "change:ch2" })
    }

    @Test fun aRoundJoinsItsChangeAndBoardWorkWithoutAChatIsAJob() {
        val mine = WorkSummary(kind = "round", id = "r1", change = "jobs-tab", state = "running", landed = 1, total = 4, chatId = "a")
        val loose = WorkSummary(kind = "run", id = "u1", title = "Run u1", state = "stopped")
        val jobs = WorkJobsModel.build(listOf(chat("a", "2026-11-07T12:00:00Z")), repoKey, setOf(repoKey), listOf(mine, loose), board)
        val change = jobs.now.first { it.key == "change:ch1" }
        assertTrue(change.running)
        assertEquals(1 to 4, change.done to change.total)
        assertEquals("r1", change.work?.id)
        assertEquals(1, jobs.runningCount)
        val run = jobs.now.single { it.key == "round:run:u1" }
        assertTrue(run.chats.isEmpty())
        assertEquals("u1", run.work?.id)
    }

    @Test fun agoReadsMinutesHoursAndDays() {
        val now = Instant.parse("2026-11-07T12:00:00Z").toEpochMilli()
        assertEquals("Just now", agoText("2026-11-07T11:59:40Z", now))
        assertEquals("4m ago", agoText("2026-11-07T11:56:00Z", now))
        assertEquals("3h ago", agoText("2026-11-07T09:00:00Z", now))
        assertEquals("2d ago", agoText("2026-11-05T11:00:00Z", now))
        assertNull(agoText("", now))
    }
}
