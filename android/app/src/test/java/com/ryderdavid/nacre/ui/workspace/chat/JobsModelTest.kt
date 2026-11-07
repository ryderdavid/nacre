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
import com.ryderdavid.nacre.data.model.WorkSummary
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class JobsModelTest {
    private val host = "c0"
    private val repoKey = Keys.of(host, "nacre")

    private fun chat(id: String, updated: String, vararg links: ChatWorkLink) =
        ChatRecord(id = id, repo = "nacre", title = "Chat $id", updatedAt = updated, work = links.toList(), hostId = host)

    private val board = BoardSnapshot(
        Repo(id = "nacre", name = "Nacre", path = "/x/nacre", hostId = host, hostName = host),
        BoardIndex(
            listOf(
                Issue(id = "E", title = "Board is the UI", type = "epic"),
                Issue(id = "E.1", title = "One", parent = "E", status = "closed"),
                Issue(id = "E.2", title = "Two", parent = "E"),
                Issue(id = "B", title = "A bug", type = "bug"),
            ),
        ),
        ChangesList(changes = listOf(ChangeSummary(id = "ch1", changeName = "jobs-tab", phase = "promoting", tasksDone = 3, tasksTotal = 8))),
    )

    @Test fun aChatWithoutWorkIsAConversation() {
        val list = Jobs.build(listOf(chat("a", "2026-11-07T10:00:00Z")), repoKey, setOf(repoKey), emptyList(), board)
        assertEquals(emptyList(), list.jobs)
        assertEquals(listOf(Keys.of(host, "a")), list.conversations.map { it.key })
    }

    @Test fun progressComesFromTheOpenRoundFirst() {
        val w = WorkSummary(kind = "round", id = "r1", title = "Round r1", change = "jobs-tab", state = "running", landed = 2, total = 5, chatId = "a")
        val list = Jobs.build(listOf(chat("a", "2026-11-07T10:00:00Z", ChatWorkLink("change", "nacre", "ch1"))), repoKey, setOf(repoKey), listOf(w), board)
        val j = list.jobs.single()
        assertEquals("jobs-tab", j.subject)
        assertEquals(2 to 5, j.done to j.total)
        assertEquals(true, j.running)
        assertEquals("r1", j.work?.id)
    }

    @Test fun withoutARoundTheChangeTasksThenTheEpicCount() {
        val list = Jobs.build(
            listOf(
                chat("a", "2026-11-07T10:00:00Z", ChatWorkLink("change", "nacre", "ch1")),
                chat("b", "2026-11-07T09:00:00Z", ChatWorkLink("bead", "nacre", "E")),
                chat("c", "2026-11-07T08:00:00Z", ChatWorkLink("bead", "nacre", "B")),
            ),
            repoKey, setOf(repoKey), emptyList(), board,
        )
        val byTitle = list.jobs.associateBy { it.title }
        assertEquals(3 to 8, byTitle.getValue("Chat a").let { it.done to it.total })
        assertEquals("Board is the UI", byTitle.getValue("Chat b").subject)
        assertEquals(1 to 2, byTitle.getValue("Chat b").let { it.done to it.total })
        assertEquals("A bug", byTitle.getValue("Chat c").subject)
        assertEquals(0 to 0, byTitle.getValue("Chat c").let { it.done to it.total })
        assertEquals("Open", byTitle.getValue("Chat c").status)
        assertNull(byTitle.getValue("Chat c").work)
    }

    @Test fun openWorkFromTheBoardIsAJobWithoutAChat() {
        val w = WorkSummary(kind = "round", id = "r2", title = "Round r2", state = "stopped", landed = 1, total = 3)
        val done = WorkSummary(kind = "run", id = "u1", title = "Run", state = "done")
        val list = Jobs.build(emptyList(), repoKey, setOf(repoKey), listOf(w, done), board)
        val j = list.jobs.single()
        assertNull(j.chatKey)
        assertEquals(true, j.stopped)
        assertEquals("r2", j.work?.id)
    }

    @Test fun linksToAnotherRepositoryDoNotMakeAJob() {
        val list = Jobs.build(listOf(chat("a", "2026-11-07T10:00:00Z", ChatWorkLink("bead", "other", "B"))), repoKey, setOf(repoKey), emptyList(), board)
        assertEquals(0, list.jobs.size)
        assertEquals(1, list.conversations.size)
    }
}
