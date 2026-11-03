package com.ryderdavid.nacre.data.claude

import com.ryderdavid.nacre.data.bridge.AssistMessage
import com.ryderdavid.nacre.data.bridge.BridgeException
import com.ryderdavid.nacre.data.model.AssistRequest
import com.ryderdavid.nacre.data.model.AssistResult
import com.ryderdavid.nacre.data.model.Features
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.RunEvent
import com.ryderdavid.nacre.data.settings.AppSettings
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import java.io.File
import java.net.SocketException
import java.net.SocketTimeoutException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

/**
 * NAC-axe: the Mac-backed chats ride out a brief drop in the connection to
 * nacred. Each test is one of the scenarios written down in the bead.
 */
class MacReattachTest {
    private val key = Keys.of("h1", "nacre")
    private val answer = buildJsonObject { put("answer", "it's in MacBackend.kt") }
    private val reply = buildJsonObject { put("reply", "picked up"); putJsonArray("proposals") {} }

    /** A computer that plays back [streams] for each POST and [resumes] for each reattach, in order. */
    private class Link(
        val streams: MutableList<(AssistRequest) -> Flow<AssistMessage>>,
        val resumes: MutableList<(String, Long) -> Flow<AssistMessage>> = mutableListOf(),
        val features: List<String> = listOf(Features.CLAUDE, Features.CHAT_IMAGES, Features.ASSIST_TURN_IDS),
    ) : MacLink {
        val sent = mutableListOf<AssistRequest>()
        val resumed = mutableListOf<Pair<String, Long>>()
        var json = 0
        override suspend fun claudeJson(key: String, req: AssistRequest): AssistResult { json++; error("a blocking call can't be reattached") }
        override fun claudeStream(key: String, req: AssistRequest): Flow<AssistMessage> { sent += req; return streams.removeAt(0)(req) }
        override fun claudeStreamAcross(hostId: String, req: AssistRequest): Flow<AssistMessage> = error("not used")
        override fun claudeResume(key: String, turn: String, after: Long): Flow<AssistMessage> { resumed += turn to after; return resumes.removeAt(0)(turn, after) }
        override suspend fun claudeCancel(key: String, turn: String) = Unit
        override fun hostName(hostId: String) = "Studio"
        override fun features(hostId: String): List<String>? = features
    }

    private class Notes : LinkNotes {
        val log = mutableListOf<String>()
        override fun dropped(host: String) { log += "dropped $host" }
        override fun back(host: String) { log += "back $host" }
        override fun gaveUp(host: String) { log += "gave up $host" }
    }

    private fun ev(seq: Long) = AssistMessage.Event(RunEvent(seq = seq, kind = "tool_use", tool = "Bash", toolId = "t$seq", text = "bd show NAC-1"))

    private val req = AssistRequest(prompt = "Where is followTurn?", system = "s")

    /**
     * Scenario 1: a bead chat Ask or Refine used to be one blocking POST. Any
     * drop (or just a call longer than the 45 s read timeout) failed it, and
     * nacred killed Claude because the request went away. Now it is a turn
     * the app reattaches to, and the chat says when the link went and came back.
     */
    @Test fun aBeadChatCallSurvivesADropMidAnswer() = runBlocking {
        val link = Link(
            streams = mutableListOf({ r -> flow { emit(AssistMessage.Turn(r.turnId)); emit(ev(1)); throw SocketException("Software caused connection abort") } }),
            resumes = mutableListOf({ _, _ -> flow { emit(ev(2)); emit(AssistMessage.Done(AssistResult(output = answer))) } }),
        )
        val notes = Notes()
        val out = withContext(notes) { MacBackend(link) { AppSettings() }.oneTurn(key, req) }
        assertEquals(answer, out.output)
        assertEquals(0, link.json)
        assertTrue(link.sent.single().turnId.startsWith("c_"))
        assertEquals(listOf(link.sent.single().turnId to 1L), link.resumed)
        assertEquals(listOf("dropped Studio", "back Studio"), notes.log)
    }

    @Test fun anOlderComputerStillGetsTheBlockingCall() = runBlocking {
        val link = object : MacLink by Link(mutableListOf(), features = listOf(Features.CLAUDE)) {
            var called = 0
            override suspend fun claudeJson(key: String, req: AssistRequest): AssistResult { called++; return AssistResult(output = buildJsonObject { put("answer", "old") }) }
        }
        val got = MacBackend(link) { AppSettings() }.ask(key, BeadContext("nacre", "NAC-1", "t", "task", 2, "open", emptyList(), "", "", "", "", null, emptyList(), emptyList()), emptyList(), "q", ChatTurnImages.NONE)
        assertEquals("old", got)
        assertEquals(1, link.called)
    }

    /**
     * Scenario 2: the board assistant's turn, and the drop comes before the
     * "turn" message (the app had nothing to resume, so the user got "Can't
     * reach that computer" and a Retry that asked again, running Claude
     * twice). Now the same request goes again with the same turn id.
     */
    @Test fun anAssistantTurnDroppedBeforeItsIdArrivedIsSentAgainAsTheSameTurn() = runBlocking {
        val link = Link(
            streams = mutableListOf(
                { _ -> flow { throw SocketTimeoutException("timeout") } },
                { r -> flow { emit(AssistMessage.Turn(r.turnId)); emit(ev(1)); emit(AssistMessage.Done(AssistResult(sessionId = "s-1", output = reply))) } },
            ),
        )
        val sink = Sink()
        val session = MacSession(link, { AppSettings() }, AssistantBoard(key, "nacre") { "system" })
        val result = session.turn(TurnInput.Say("what's next?"), sink)
        assertEquals(TurnStop.DONE, result.stop)
        assertEquals(2, link.sent.size)
        assertEquals(link.sent[0].turnId, link.sent[1].turnId)
        assertTrue(link.sent[0].turnId.isNotBlank())
        assertTrue("picked up" in sink.text.toString())
        // The conversation shows when it dropped and when it came back.
        val connection = sink.asides.single { it.second == CONNECTION_STEP }
        assertTrue(connection.third.startsWith("The connection to Studio dropped at"), connection.third)
        assertTrue(sink.done.single { it.first == connection.first }.second.startsWith("Reconnected at"))
        // Only the tool step from nacred shows besides it: nothing duplicated.
        assertEquals(1, sink.asides.count { it.second != CONNECTION_STEP })
    }

    /**
     * A drop that can't be picked up — here nacred restarted and forgot the
     * turn — says so plainly and offers a Retry, and the Retry asks again.
     */
    @Test fun aTurnTheComputerForgotOffersARetryThatAsksAgain() = runBlocking {
        val link = Link(
            streams = mutableListOf(
                { r -> flow { emit(AssistMessage.Turn(r.turnId)); emit(ev(1)); throw SocketException("reset") } },
                { r -> flow { emit(AssistMessage.Turn(r.turnId)); emit(AssistMessage.Done(AssistResult(output = reply))) } },
            ),
            resumes = mutableListOf({ _, _ -> flow { throw BridgeException("that answer is no longer available", code = 404) } }),
        )
        val session = MacSession(link, { AppSettings() }, AssistantBoard(key, "nacre") { "system" })
        val e = assertFailsWith<ClaudeUnavailableException> { session.turn(TurnInput.Say("hi"), Sink()) }
        assertTrue(e.canRetry)
        assertTrue("Retry asks again" in e.message.orEmpty(), e.message)
        val sink = Sink()
        assertEquals(TurnStop.DONE, session.turn(TurnInput.Say("hi"), sink).stop)
        assertTrue("picked up" in sink.text.toString())
        assertTrue(link.sent[0].turnId != link.sent[1].turnId)
    }

    /** The bead chat thread shows the drop and the reconnection, and never sends them to Claude. */
    @Test fun theBeadChatThreadShowsWhenTheLinkDroppedAndCameBack() = runBlocking {
        val file = File(java.nio.file.Files.createTempDirectory("bead-chats").toFile(), "bead-chats.json")
        val chats = BeadChats(file, CoroutineScope(Dispatchers.Default), Dispatchers.Unconfined)
        val chatKey = BeadChats.key(key, "NAC-1")
        val gate = CompletableDeferred<Unit>()
        chats.send(chatKey, BeadChatItem.Mine("where?", ChatMode.ASK), retrying = false) {
            val notes = kotlin.coroutines.coroutineContext[LinkNotes]!!
            notes.dropped("Studio")
            gate.await()
            notes.back("Studio")
            BeadChatItem.Answer("here")
        }
        val dropped = withTimeout(5_000) { chats.items(chatKey).first { list -> list.any { it is BeadChatItem.Connection } } }
        assertTrue((dropped[1] as BeadChatItem.Connection).text.startsWith("The connection to Studio dropped at"))
        gate.complete(Unit)
        val done = withTimeout(5_000) { chats.items(chatKey).first { list -> list.any { it is BeadChatItem.Answer } } }
        val note = done[1] as BeadChatItem.Connection
        assertTrue("Reconnected at" in note.text && !note.failed, note.text)
        assertEquals(listOf(true, false), done.turns().map { it.fromUser })
    }

    private class Sink : TurnSink {
        val text = StringBuilder()
        val asides = mutableListOf<Triple<String, String, String>>()
        val done = mutableListOf<Pair<String, String>>()
        override fun newBlock() = Unit
        override fun text(delta: String) { text.append(delta) }
        override fun thinking(delta: String) = Unit
        override fun aside(id: String, name: String, summary: String) { asides += Triple(id, name, summary) }
        override fun asideDone(id: String, result: String, isError: Boolean) { done += id to result }
    }
}
