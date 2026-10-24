package com.ryderdavid.nacre.data.claude

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import java.io.File
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

class BeadChatsTest {
    private val file = File.createTempFile("bead-chats", ".json").also { it.delete(); it.deleteOnExit() }
    private fun chats() = BeadChats(file, CoroutineScope(Dispatchers.Unconfined), Dispatchers.Unconfined)
    private val key = BeadChats.key("mac~nacre", "NAC-1")

    @Test fun aReplyFollowsTheMessageAndSeesOnlyEarlierTurns() = runBlocking {
        val c = chats()
        var seen: List<BeadChatItem>? = null
        c.send(key, BeadChatItem.Mine("Why?", ChatMode.ASK), retrying = false) { seen = it; BeadChatItem.Answer("Because.") }
        c.send(key, BeadChatItem.Mine("Tighten it", ChatMode.REFINE), retrying = false) { seen = it; BeadChatItem.Proposal(RefineProposal(summary = "Shorter")) }
        assertEquals(2, seen!!.size, "the second call sees the first exchange, not its own message")
        val items = c.items(key).first()
        assertEquals(listOf("Mine", "Answer", "Mine", "Proposal"), items.map { it::class.simpleName })
    }

    @Test fun aFailureKeepsTheMessageAndRetryDoesNotRepeatIt() = runBlocking {
        val c = chats()
        val msg = BeadChatItem.Mine("Why?", ChatMode.ASK)
        c.send(key, msg, retrying = false) { throw ClaudeUnavailableException("Mac is asleep", offerApiKey = true) }
        val f = assertNotNull(c.failure(key).first())
        assertTrue(f.offerApiKey)
        c.send(key, f.retry, retrying = true) { BeadChatItem.Answer("Because.") }
        assertNull(c.failure(key).first())
        assertEquals(listOf(msg, BeadChatItem.Answer("Because.")), c.items(key).first())
    }

    @Test fun oneMessageAtATimePerBead() = runBlocking {
        val c = BeadChats(file, CoroutineScope(Dispatchers.Default), Dispatchers.Unconfined)
        val gate = CompletableDeferred<Unit>()
        c.send(key, BeadChatItem.Mine("one", ChatMode.ASK), retrying = false) { gate.await(); BeadChatItem.Answer("1") }
        c.send(key, BeadChatItem.Mine("two", ChatMode.ASK), retrying = false) { BeadChatItem.Answer("2") }
        assertEquals(1, c.current(key).size, "the second message is refused while the first is unanswered")
        gate.complete(Unit)
        c.thinking(key).first { !it }
        assertEquals(2, c.current(key).size)
    }

    @Test fun threadsAndModeSurviveARestartUntilCleared() = runBlocking {
        val c = chats()
        c.setMode(ChatMode.ASK)
        c.send(key, BeadChatItem.Mine("Why?", ChatMode.ASK), retrying = false) { BeadChatItem.Answer("Because.") }
        c.set(key, 1, BeadChatItem.Answer("Edited"))
        val again = chats()
        assertEquals(ChatMode.ASK, again.mode.first())
        assertEquals(BeadChatItem.Answer("Edited"), again.current(key)[1])
        again.clear(key)
        assertEquals(emptyList(), chats().current(key))
    }

    @Test fun turnsSayWhatBecameOfEachProposal() {
        val turns = listOf(
            BeadChatItem.Mine("Why?", ChatMode.ASK),
            BeadChatItem.Answer("Because."),
            BeadChatItem.Mine("", ChatMode.REFINE),
            BeadChatItem.Proposal(RefineProposal(summary = "Added criteria"), BeadChatItem.Outcome.APPLIED, applied = 2),
            BeadChatItem.Proposal(RefineProposal(summary = "Raised priority"), BeadChatItem.Outcome.DISCARDED),
        ).turns()
        assertEquals(DraftTurn(true, "(asking) Why?"), turns[0])
        assertEquals(DraftTurn(false, "Because."), turns[1])
        assertTrue("applied 2" in turns[3].text && "Added criteria" in turns[3].text)
        assertTrue("discarded" in turns[4].text)
    }

    @Test fun promptsCarryTheConversation() {
        val ctx = BeadContext(
            "Nacre", "NAC-1", "Title", "task", 2, "open", emptyList(), "Desc", "", "", "", null, emptyList(), emptyList(),
            comments = listOf("ryder: looks close"),
        )
        val history = listOf(DraftTurn(true, "(asking) Why --dir?"), DraftTurn(false, "So reads name their board."))
        val ask = Prompts.ask(ctx, history, "And the allowlist?", readsRepo = true)
        assertTrue("<question>And the allowlist?</question>" in ask)
        assertTrue("<ryder>(asking) Why --dir?</ryder>" in ask && "<you>So reads name their board.</you>" in ask)
        assertTrue("ryder: looks close" in ask)
        assertTrue("may read its code" in ask)
        assertTrue("may read its code" !in Prompts.ask(ctx, emptyList(), "q", readsRepo = false))
        val refine = Prompts.refine(ctx, "Make it agent-ready", history)
        assertTrue("<you>So reads name their board.</you>" in refine)
        assertTrue("<conversation>" !in Prompts.refine(ctx, "x"))
    }
}
