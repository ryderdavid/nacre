package com.ryderdavid.nacre.data.claude

import com.ryderdavid.nacre.data.bridge.AssistMessage
import com.ryderdavid.nacre.data.bridge.BridgeException
import com.ryderdavid.nacre.data.model.AssistResult
import com.ryderdavid.nacre.data.model.RunEvent
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.runBlocking
import java.io.IOException
import java.net.SocketException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertIs

/** A Mac turn outlives a dropped connection: the app reattaches and carries on. */
class FollowTurnTest {
    private fun ev(seq: Long) = AssistMessage.Event(RunEvent(seq = seq, kind = "tool_use", toolId = "t$seq"))
    private val done = AssistMessage.Done(AssistResult(sessionId = "s-1"))

    private class Drops : DropListener {
        val log = mutableListOf<String>()
        override fun lost(attempt: Int) { log += "lost $attempt" }
        override fun back() { log += "back" }
    }

    private fun dropsAfter(vararg msgs: AssistMessage): Flow<AssistMessage> = flow {
        msgs.forEach { emit(it) }
        throw SocketException("Software caused connection abort")
    }

    @Test fun reattachesAfterADropAndSkipsWhatItAlreadyShowed() = runBlocking {
        val progress = TurnProgress("hi")
        val relayed = mutableListOf<Long>()
        val resumes = mutableListOf<Pair<String, Long>>()
        val drops = Drops()
        val out = followTurn(
            first = dropsAfter(AssistMessage.Turn("t_1"), ev(1)),
            progress = progress,
            resume = { t, after ->
                resumes += t to after
                // nacred replays from `after`, but a duplicate must not show twice either.
                flow { emit(ev(1)); emit(ev(2)); emit(done) }
            },
            relay = { relayed += it.seq },
            dropped = drops,
            pause = {},
        )
        assertIs<TurnOutcome.Done>(out)
        assertEquals(listOf(1L, 2L), relayed)
        assertEquals(listOf("t_1" to 1L), resumes)
        assertEquals(listOf("lost 1", "back"), drops.log)
    }

    @Test fun aStreamThatEndsWithoutAnAnswerCountsAsADrop() = runBlocking {
        var resumed = 0
        val out = followTurn(
            first = flow { emit(AssistMessage.Turn("t_1")) },
            progress = TurnProgress("hi"),
            resume = { _, _ -> resumed++; flow { emit(AssistMessage.Failed("usage limit")) } },
            relay = {},
            dropped = Drops(),
            pause = {},
        )
        assertEquals(TurnOutcome.Failed("usage limit"), out)
        assertEquals(1, resumed)
    }

    @Test fun givesUpAfterTheBudgetButAlwaysTriesOnceMoreAfterEachPause() = runBlocking {
        var clock = 0L
        var attempts = 0
        assertFailsWith<TurnLostException> {
            followTurn(
                first = dropsAfter(AssistMessage.Turn("t_1")),
                progress = TurnProgress("hi"),
                resume = { _, _ -> attempts++; dropsAfter() },
                relay = {},
                dropped = Drops(),
                budgetMs = 10_000,
                pause = { clock += 6_000 },
                now = { clock },
            )
        }
        // Lost at 0; tries at 6 s and 12 s, then stops because 12 s is past the budget.
        assertEquals(2, attempts)
    }

    @Test fun aComputerThatCantBeReachedIsNotADrop() = runBlocking {
        // Nothing started, so there is nothing to reattach to: the caller explains it.
        val e = assertFailsWith<java.net.ConnectException> {
            followTurn(flow { throw java.net.ConnectException("refused") }, TurnProgress("hi"), { _, _ -> error("no") }, {}, Drops(), pause = {})
        }
        assertEquals("refused", e.message)
    }

    @Test fun anOlderNacredWithNoTurnIdCannotBeResumed() = runBlocking {
        assertFailsWith<TurnLostException> {
            followTurn(dropsAfter(ev(1)), TurnProgress("hi"), { _, _ -> error("no") }, {}, Drops(), pause = {})
        }
        Unit
    }

    @Test fun anExpiredTurnEndsAtOnce() = runBlocking {
        val e = assertFailsWith<BridgeException> {
            followTurn(
                dropsAfter(AssistMessage.Turn("t_1")), TurnProgress("hi"),
                { _, _ -> flow { throw BridgeException("that answer is no longer available", code = 404) } },
                {}, Drops(), pause = {},
            )
        }
        assertEquals(404, e.code)
    }

    @Test fun backoffSettlesAtTenSeconds() {
        assertEquals(listOf(1_000L, 2_000L, 4_000L, 8_000L, 10_000L, 10_000L), (1..6).map(::reconnectDelay))
    }
}
