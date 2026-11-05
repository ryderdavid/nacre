package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.data.RepoGroup
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.Repo
import com.ryderdavid.nacre.data.model.Round
import com.ryderdavid.nacre.data.model.RoundStop
import com.ryderdavid.nacre.data.model.Run
import com.ryderdavid.nacre.data.model.ScreenContext
import com.ryderdavid.nacre.data.model.StopChoice
import com.ryderdavid.nacre.ui.BeadRoute
import com.ryderdavid.nacre.ui.BoardsRoute
import com.ryderdavid.nacre.ui.ChatsRoute
import com.ryderdavid.nacre.ui.InboxRoute
import com.ryderdavid.nacre.ui.RoundRoute
import com.ryderdavid.nacre.ui.ScreenOrigin
import com.ryderdavid.nacre.ui.rememberedOrigin
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** 8.7: what is sent with a message comes from the screen at the moment Send is pressed. */
class ScreenContextTest {
    private val issues = listOf(
        Issue("NAC-7", title = "Fix the dock", status = "in_progress", labels = listOf("ui")),
        Issue("NAC-8", title = "Other"),
    )
    private val round = Round(
        id = "r-12", repoId = "nacre", repoName = "Nacre", state = "paused", pauseReason = "gate failed",
        stop = RoundStop(reason = "app tests failed", phase = "gates", bead = "NAC-3", choices = listOf(StopChoice("retry", "Retry"), StopChoice("skip", "Skip it"))),
        hostId = "a",
    )
    private val run = Run(id = "run-4", repoId = "nacre", issueId = "NAC-3", status = "failed", error = "exit 1", hostId = "a")

    private fun bytes(s: String?) = s.orEmpty().toByteArray(Charsets.UTF_8).size

    /** Publishes what each screen shows, then shows it, and reads the context as Send would. */
    private fun sent(origin: ScreenOrigin, screen: ShownScreen): ShownScreen {
        val holder = ScreenContextHolder()
        holder.publish(origin, screen)
        holder.show(origin)
        return holder.atSend()
    }

    @Test fun `each screen's context is captured at Send, with selection and pane, within 4 KB`() {
        val board = sent(ScreenOrigin.Board("a~nacre", "Nacre"), ScreenSummaries.board("a~nacre", "Nacre", issues, issues[0]))
        assertEquals(ScreenContext.BOARD, board.context.screen)
        assertEquals("nacre", board.context.repo)
        assertEquals("NAC-7", board.context.selection)
        assertEquals("bead detail", board.context.pane)

        val bead = sent(ScreenOrigin.Bead("a~nacre", "NAC-7"), ScreenSummaries.bead("a~nacre", "NAC-7", issues[0], pane = "run"))
        assertEquals(ScreenContext.BEAD, bead.context.screen)
        assertEquals("NAC-7", bead.context.beadId)
        assertEquals("run", bead.context.pane)
        assertTrue(bead.context.summary!!.contains("Fix the dock"))

        val change = sent(
            ScreenOrigin.Change("a~nacre", "NAC-1"),
            ScreenSummaries.bead("a~nacre", "NAC-1", null, pane = "change", changeId = "agent-chat-tab", changePhase = "approved", changeRevision = "7fa1f73"),
        )
        assertEquals(ScreenContext.CHANGE, change.context.screen)
        assertEquals("agent-chat-tab", change.context.changeId)
        assertTrue(change.context.summary!!.contains("approved") && change.context.summary!!.contains("7fa1f73"))

        val r = sent(ScreenOrigin.Round("a~r-12"), ScreenSummaries.round(round, selection = "NAC-3", pane = "integration log"))
        assertEquals(ScreenContext.ROUND, r.context.screen)
        assertEquals("r-12", r.context.roundId)
        assertEquals("NAC-3", r.context.selection)
        assertEquals("integration log", r.context.pane)
        assertTrue(r.context.summary!!.contains("app tests failed") && r.context.summary!!.contains("Skip it"))

        val ru = sent(ScreenOrigin.Run("a~run-4"), ScreenSummaries.run(run))
        assertEquals(ScreenContext.RUN, ru.context.screen)
        assertTrue(ru.context.summary!!.contains("exit 1"))

        for (s in listOf("boards", "inbox", "activity")) {
            val all = sent(ScreenOrigin.AllBoards(s), ScreenSummaries.allBoards(s, listOf("Nacre: 3 open")))
            assertEquals(s, all.context.screen)
            assertNull(all.context.repo)
            assertEquals("all boards", all.label)
        }

        // A huge summary is cut to 4 KB without splitting a character.
        val many = (1..2000).map { Issue("NAC-$it", title = "Ünïcode bead número $it") }
        val big = sent(ScreenOrigin.Board("a~nacre", "Nacre"), ScreenSummaries.board("a~nacre", "Nacre", many, null))
        assertTrue(bytes(big.context.summary) <= ScreenContext.SUMMARY_MAX_BYTES)
        assertTrue(bytes(trimUtf8("é".repeat(5000))) <= ScreenContext.SUMMARY_MAX_BYTES)
        for (s in listOf(board, bead, change, r, ru)) assertTrue(bytes(s.context.summary) <= ScreenContext.SUMMARY_MAX_BYTES)
    }

    @Test fun `moving from a bead to a round changes the sent context and the chip`() {
        val holder = ScreenContextHolder()
        val beadOrigin = ScreenOrigin.Bead("a~nacre", "NAC-3")
        holder.publish(beadOrigin, ScreenSummaries.bead("a~nacre", "NAC-3", Issue("NAC-3", title = "Close me")))
        holder.show(beadOrigin)
        assertEquals("NAC-3", holder.atSend().context.beadId)
        assertEquals("About: NAC-3", aboutChip(holder.current.value.label))

        val roundOrigin = ScreenOrigin.Round("a~r-12")
        holder.publish(roundOrigin, ScreenSummaries.round(round))
        holder.show(roundOrigin)
        val sent = holder.atSend()
        assertEquals(ScreenContext.ROUND, sent.context.screen)
        assertEquals("r-12", sent.context.roundId)
        assertNull(sent.context.beadId)
        assertEquals("About: round r-12", aboutChip(holder.current.value.label))
    }

    @Test fun `before a screen has published, its kind and ids are still sent`() {
        val holder = ScreenContextHolder()
        holder.show(ScreenOrigin.Bead("a~nacre", "NAC-9"))
        assertEquals("NAC-9", holder.atSend().context.beadId)
        assertEquals("a~nacre", holder.atSend().repoKey)
    }

    @Test fun `the Chat tab uses the screen last shown before it, or all boards`() {
        val holder = ScreenContextHolder()
        var origin: ScreenOrigin? = null
        fun go(route: Any) {
            origin = rememberedOrigin(origin, route)
            holder.show(origin)
        }
        go(ChatsRoute)
        assertEquals(ScreenContext.BOARDS, holder.atSend().context.screen)
        assertEquals("all boards", holder.atSend().label)

        go(BoardsRoute)
        go(RoundRoute("a~r-12"))
        holder.publish(ScreenOrigin.Round("a~r-12"), ScreenSummaries.round(round))
        go(ChatsRoute)
        assertEquals("r-12", holder.atSend().context.roundId)

        go(InboxRoute)
        go(BeadRoute("a~nacre", "NAC-48p.5"))
        go(ChatsRoute)
        assertEquals("NAC-48p.5", holder.atSend().context.beadId)
        assertEquals("About: NAC-48p.5", aboutChip(holder.current.value.label))
    }

    private val groups = listOf(
        RepoGroup(
            "git:github.com/r/nacre", Repo("nacre", "Nacre", "/a/nacre", hostId = "a"),
            listOf(Repo("nacre", "Nacre", "/a/nacre", hostId = "a"), Repo("nacre2", "Nacre", "/b/nacre", hostId = "b")),
        ),
        RepoGroup("b~solo", Repo("solo", "Solo", "/b/solo", hostId = "b"), listOf(Repo("solo", "Solo", "/b/solo", hostId = "b"))),
    )
    private val names = mapOf("a" to "studio", "b" to "laptop")

    @Test fun `a repository is mapped to the chat's computer through its repo group`() {
        val shown = ScreenSummaries.bead("b~nacre2", "NAC-1", null)
        val sc = contextForChat(shown, "a", groups) { names[it].orEmpty() }
        assertEquals("nacre", sc.context.repo)
        assertNull(sc.context.elsewhere)
        assertNull(sc.elsewhere)
        assertEquals("NAC-1", sc.context.beadId)
    }

    @Test fun `a repository with no checkout on the chat's computer is marked elsewhere, with the hint`() {
        val shown = ScreenSummaries.board("b~solo", "Solo", emptyList(), null)
        val sc = contextForChat(shown, "a", groups) { names[it].orEmpty() }
        assertEquals("solo", sc.context.repo)
        assertEquals("laptop", sc.context.elsewhere)
        val e = assertNotNull(sc.elsewhere)
        assertEquals("b~solo", e.repoKey)
        assertEquals("This chat runs on studio. Solo is on laptop.", elsewhereHint("studio", e))
    }

    @Test fun `all boards needs no mapping`() {
        val sc = contextForChat(ScreenSummaries.allBoards("inbox", emptyList()), "a", groups) { "" }
        assertNull(sc.context.repo)
        assertNull(sc.elsewhere)
        assertEquals("inbox", sc.context.screen)
    }
}
