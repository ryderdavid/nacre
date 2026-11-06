package com.ryderdavid.nacre.ui.workspace.chat

import com.ryderdavid.nacre.data.chat.CardState
import com.ryderdavid.nacre.data.chat.ChatItem
import com.ryderdavid.nacre.data.chat.ConversationState
import com.ryderdavid.nacre.data.chat.PermissionCard
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.data.model.ChatWire
import com.ryderdavid.nacre.data.model.ScreenContext
import com.ryderdavid.nacre.data.model.WorkSummary
import com.ryderdavid.nacre.ui.workspace.ChatReference
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ChatModelTest {
    private val bead = ChatReference("bead", "NAC-7", "NAC-7")

    @Test
    fun `selection and pane - the viewed bead is the message's context`() {
        val c = ChatSend.context("r1", bead, dropped = false)
        assertEquals(ScreenContext.BEAD, c.screen)
        assertEquals("NAC-7", c.beadId)
        assertEquals("r1", c.repo)
    }

    @Test
    fun `not this time - a dropped viewing chip leaves only the project`() {
        val c = ChatSend.context("r1", bead, dropped = true)
        assertEquals(ScreenContext.BOARD, c.screen)
        assertEquals(null, c.beadId)
    }

    @Test
    fun `a viewed document is the change with its pane`() {
        val c = ChatSend.context("r1", ChatReference("document", "offline-chat-resume/design", "design"), false)
        assertEquals("offline-chat-resume", c.changeId)
        assertEquals("document", c.pane)
    }

    @Test
    fun `a document under openspec names its change and keeps its path`() {
        val c = ChatSend.context("r1", ChatReference("document", "openspec/changes/foo/proposal.md", "foo · Proposal"), false)
        assertEquals("foo", c.changeId)
        assertEquals("openspec/changes/foo/proposal.md · foo · Proposal", c.selection)
        assertEquals("bar", ChatSend.changeIdOf("openspec/changes/archive/bar/design.md"))
        assertEquals(null, ChatSend.changeIdOf("openspec/specs/agent-chat/spec.md"))
        assertEquals("run-7", ChatSend.context("r1", ChatReference("run", "run-7", "run · x"), false).runId)
    }

    @Test
    fun `references are named first and quotes kept`() {
        val t = ChatSend.text("Why?", listOf(bead, ChatReference("requirement", "agent-chat", "Chats resume", text = "SHALL NOT show an event twice")))
        assertTrue(t.startsWith("About: NAC-7 (bead); Chats resume (requirement agent-chat)"))
        assertTrue(t.contains("> SHALL NOT show an event twice"))
        assertTrue(t.endsWith("Why?"))
        assertEquals("plain", ChatSend.text(" plain ", emptyList()))
    }

    private fun running() = ConversationState(chat = ChatRecord("c1", state = ChatWire.RUNNING), loading = false)

    @Test
    fun `a long silent tool run says no updates, never stalled`() {
        val s = AgentStates.of(running(), sinceOutputMs = 6 * 60_000, stalled = false)
        assertEquals(AgentPhase.QUIET, s.phase)
        assertEquals("Working · no updates for 6 min", s.text)
        assertFalse(s.text.contains("stalled"))
    }

    @Test
    fun `may be stalled only from nacred's signal`() {
        assertEquals(AgentPhase.STALLED, AgentStates.of(running(), 0, stalled = true).phase)
        assertEquals(AgentPhase.WORKING, AgentStates.of(running(), 0, stalled = false).phase)
    }

    @Test
    fun `a pending permission is waiting for you`() {
        val s = running().copy(items = listOf(ChatItem.Permission("p", PermissionCard("r", "Bash", "ls", "", CardState.PENDING))))
        assertEquals(AgentPhase.WAITING, AgentStates.of(s, 0, false).phase)
    }

    @Test
    fun `suggestions follow what's going on and stay few`() {
        val s = suggestions(2, WorkSummary(state = "stopped"), bead)
        assertEquals(listOf("What needs me?", "Why did the round stop?", "Explain NAC-7", "What's next?"), s)
    }

    @Test
    fun `threads list this checkout first, then the project on other computers`() {
        val chats = listOf(
            ChatRecord("a", repo = "r1", title = "Old", updatedAt = "2026-11-01", hostId = "h1"),
            ChatRecord("b", repo = "r9", title = "Elsewhere", updatedAt = "2026-11-06", hostId = "h2"),
            ChatRecord("c", repo = "r1", title = "New", updatedAt = "2026-11-05", hostId = "h1"),
            ChatRecord("d", repo = "rX", title = "Other project", hostId = "h1"),
        )
        val rows = threadsFor(chats, "h1~r1", setOf("h1~r1", "h2~r9"))
        assertEquals(listOf("New", "Old", "Elsewhere"), rows.map { it.title })
        assertEquals(listOf(true, true, false), rows.map { it.here })
    }
}
