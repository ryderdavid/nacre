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
import kotlin.test.assertTrue

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

    // ---- NAC-axe: the hiccups reproduced, one test each ----

    /**
     * Scenario 2: the phone sends a board-assistant turn and the link drops
     * before nacred's "turn" message arrives (Wi-Fi to cellular right after
     * Send). The app already knows the turn's id, so it resends the same
     * request; nacred attaches it to the turn it has, and nothing is asked twice.
     */
    @Test fun aDropBeforeTheTurnMessageResendsTheSameTurnInsteadOfFailing() = runBlocking {
        val progress = TurnProgress("hi", turn = "c_0123456789abcdef01234567")
        var resent = 0
        val relayed = mutableListOf<Long>()
        val drops = Drops()
        val out = followTurn(
            first = flow { throw java.net.SocketTimeoutException("timeout") },
            progress = progress,
            resume = { _, _ -> error("not confirmed yet, so not resumed") },
            relay = { relayed += it.seq },
            dropped = drops,
            resend = {
                resent++
                // nacred replays the turn it already had from its first event.
                flow { emit(AssistMessage.Turn("c_0123456789abcdef01234567")); emit(ev(1)); emit(ev(2)); emit(done) }
            },
            pause = {},
        )
        assertIs<TurnOutcome.Done>(out)
        assertEquals(1, resent)
        assertEquals(listOf(1L, 2L), relayed)
        assertEquals(listOf("lost 1", "back"), drops.log)
        assertTrue(progress.confirmed)
    }

    @Test fun onceTheTurnIsConfirmedADropResumesRatherThanResends() = runBlocking {
        val resumes = mutableListOf<Pair<String, Long>>()
        val relayed = mutableListOf<Long>()
        val out = followTurn(
            first = dropsAfter(AssistMessage.Turn("c_x1234567890"), ev(1)),
            progress = TurnProgress("hi", turn = "c_x1234567890"),
            resume = { t, after -> resumes += t to after; flow { emit(ev(2)); emit(done) } },
            relay = { relayed += it.seq },
            dropped = Drops(),
            resend = { error("a confirmed turn is resumed, never sent again") },
            pause = {},
        )
        assertIs<TurnOutcome.Done>(out)
        assertEquals(listOf("c_x1234567890" to 1L), resumes)
        assertEquals(listOf(1L, 2L), relayed)
    }

    @Test fun aComputerThatCantBeReachedBeforeAnythingIsSentStillFailsAtOnce() = runBlocking {
        assertFailsWith<java.net.ConnectException> {
            followTurn(
                flow { throw java.net.ConnectException("refused") }, TurnProgress("hi", turn = "c_x1234567890"),
                { _, _ -> error("no") }, {}, Drops(), resend = { error("nothing was sent, so nothing to resend") }, pause = {},
            )
        }
        Unit
    }

    /**
     * Scenario 3: something between the phone and nacred answers "not right
     * now" (a 503 or 504 while the network settles). That is a hiccup, not
     * the end of the turn.
     */
    @Test fun aTransientHttpAnswerIsADropNotTheEnd() = runBlocking {
        var resumes = 0
        val drops = Drops()
        val out = followTurn(
            first = dropsAfter(AssistMessage.Turn("t_1"), ev(1)),
            progress = TurnProgress("hi"),
            resume = { _, _ ->
                resumes++
                if (resumes == 1) flow { throw BridgeException("The computer stopped answering (503)", code = 503) }
                else flow { emit(ev(2)); emit(done) }
            },
            relay = {},
            dropped = drops,
            pause = {},
        )
        assertIs<TurnOutcome.Done>(out)
        assertEquals(2, resumes)
        assertEquals(listOf("lost 1", "lost 2", "back"), drops.log)
    }

    @Test fun aRefusalStillEndsTheTurnAtOnce() = runBlocking {
        val e = assertFailsWith<BridgeException> {
            followTurn(
                dropsAfter(AssistMessage.Turn("t_1")), TurnProgress("hi"),
                { _, _ -> flow { throw BridgeException("The Mac rejected the pairing token", code = 401) } },
                {}, Drops(), pause = {},
            )
        }
        assertEquals(401, e.code)
    }

    /**
     * Scenario 4: the phone sleeps mid-turn. The pause it slept through took
     * ten minutes of wall-clock time but none of the phone's; on waking the
     * network isn't back yet, so the first attempt fails too. It must keep
     * trying rather than give up because "three minutes have passed".
     */
    @Test fun aPauseThePhoneSleptThroughDoesNotUseUpTheBudget() = runBlocking {
        var clock = 0L
        var resumes = 0
        val out = followTurn(
            first = dropsAfter(AssistMessage.Turn("t_1"), ev(1)),
            progress = TurnProgress("hi"),
            resume = { _, _ ->
                resumes++
                if (resumes == 1) dropsAfter() else flow { emit(ev(2)); emit(done) }
            },
            relay = {},
            dropped = Drops(),
            budgetMs = RECONNECT_BUDGET_MS,
            pause = { attempt -> clock += if (attempt == 1) 10 * 60_000L else reconnectDelay(attempt) },
            now = { clock },
        )
        assertIs<TurnOutcome.Done>(out)
        assertEquals(2, resumes)
    }

    @Test fun anAttemptThatHangsWhileThePhoneSleepsCountsOnlyUpToALimit() = runBlocking {
        var clock = 0L
        var resumes = 0
        val out = followTurn(
            first = dropsAfter(AssistMessage.Turn("t_1")),
            progress = TurnProgress("hi"),
            resume = { _, _ ->
                resumes++
                // The first reattach hangs for an hour of wall-clock (the phone slept mid-connect).
                if (resumes == 1) flow { clock += 60 * 60_000L; throw java.net.SocketTimeoutException("timeout") }
                else flow { emit(done) }
            },
            relay = {},
            dropped = Drops(),
            pause = { clock += reconnectDelay(it) },
            now = { clock },
        )
        assertIs<TurnOutcome.Done>(out)
        assertEquals(2, resumes)
    }
}
