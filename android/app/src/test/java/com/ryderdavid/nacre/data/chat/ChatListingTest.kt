package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.data.Connection
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.data.model.ChatWire
import com.ryderdavid.nacre.data.model.Features
import com.ryderdavid.nacre.data.model.Health
import com.ryderdavid.nacre.data.model.ScreenContext
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** 8.7: the Chat list across computers, the older-bridge message, the badge and the New chat sheet's agents. */
class ChatListingTest {

    @Test fun `a computer whose nacred predates Chat says to update it, and its old list is ignored`() {
        val hosts = listOf(host("a", "studio", *CHAT), host("b", "laptop", Features.CLAUDE))
        val merged = mergeChatLists(hosts, mapOf("a" to listOf(ChatRecord("c1")), "b" to listOf(ChatRecord("stale"))))
        assertEquals(listOf("Update nacred on laptop to use Chat"), merged.notices)
        assertEquals(listOf("a~c1"), merged.chats.map { it.key })
    }

    @Test fun `a nacred that knows Chat but can't run it gives its own reason`() {
        val h = host("a", "studio", connection = Connection.Online(Health(features = listOf(Features.CLAUDE), agentChatUnavailable = "Claude Code 2.3 or later is needed for host prompts.")))
        assertEquals(listOf("Chat isn't available on studio: Claude Code 2.3 or later is needed for host prompts."), mergeChatLists(listOf(h), emptyMap()).notices)
    }

    @Test fun `offline computers are left out until they connect`() {
        val merged = mergeChatLists(listOf(host("a", "studio", connection = Connection.Offline("timeout"))), mapOf("a" to listOf(ChatRecord("c1"))))
        assertTrue(merged.chats.isEmpty())
        assertTrue(merged.notices.isEmpty())
    }

    @Test fun `the badge counts chats waiting for permission`() {
        val chats = listOf(
            ChatRecord("a", state = ChatWire.WAITING_PERMISSION),
            ChatRecord("b", state = ChatWire.RUNNING),
            ChatRecord("c", state = ChatWire.WAITING_PERMISSION),
            ChatRecord("d", state = ChatWire.IDLE),
        )
        assertEquals(2, chatBadgeCount(chats))
        assertEquals(0, chatBadgeCount(emptyList()))
    }

    @Test fun `Codex is disabled with nacred's reason, and the sheet notes it has no classifier or USD limit`() {
        val health = Health(features = listOf(Features.CLAUDE, Features.AGENT_CHAT), agentChatCodexUnavailable = "Codex isn't signed in on studio.")
        val target = NewChatTarget("a", "studio", null, "all boards", health = health)
        val (claude, codex) = agentOptions(target)
        assertTrue(claude.enabled)
        assertFalse(codex.enabled)
        assertEquals("Codex isn't signed in on studio.", codex.reason)
        assertTrue(codex.note!!.contains("no permission classifier") && codex.note!!.contains("no USD runaway limit"))
        // The remembered Codex choice falls back to Claude while Codex can't be used.
        assertEquals(ChatWire.AGENT_CLAUDE, preselectedAgent(ChatWire.AGENT_CODEX, agentOptions(target)))
    }

    @Test fun `Codex is offered while nacred lists agent_chat_codex, and the remembered choice is kept`() {
        val health = Health(features = listOf(Features.AGENT_CHAT, Features.AGENT_CHAT_CODEX))
        val options = agentOptions(NewChatTarget("a", "studio", null, "all boards", health = health))
        assertTrue(options.all { it.enabled })
        assertNull(options[1].reason)
        assertEquals(ChatWire.AGENT_CODEX, preselectedAgent(ChatWire.AGENT_CODEX, options))
    }

    @Test fun `Codex without a reason from nacred names the update`() {
        val reason = codexChatBlocker("studio", Health(features = listOf(Features.AGENT_CHAT)))!!
        assertTrue(reason.contains("studio"))
    }

    @Test fun `a new chat runs on the screen's computer, or the across-boards computer for Boards`() {
        val hosts = listOf(host("a", "studio", *CHAT), host("b", "laptop", *CHAT))
        val repos = listOf(
            com.ryderdavid.nacre.data.model.Repo("nacre", "Nacre", "/n", hostId = "b"),
            com.ryderdavid.nacre.data.model.Repo("x", "X", "/x", hostId = "b"),
            com.ryderdavid.nacre.data.model.Repo("y", "Y", "/y", hostId = "a"),
        )
        val bead = ShownScreen(ScreenContext(ScreenContext.BEAD, repo = "y", beadId = "Y-1"), "a~y", "Y-1")
        val t = newChatTarget(bead, hosts, repos)!!
        assertEquals("a", t.hostId)
        assertEquals("y", t.repoId)
        assertNull(t.blocker)
        // Boards: the computer holding the most boards.
        val across = newChatTarget(bareScreen(null), hosts, repos)!!
        assertEquals("b", across.hostId)
        assertNull(across.repoId)
        // A computer without Chat can't start one, and says why.
        val old = newChatTarget(bead, listOf(host("a", "studio", Features.CLAUDE)), repos)!!
        assertEquals("Update nacred on studio to use Chat", old.blocker)
    }
}
