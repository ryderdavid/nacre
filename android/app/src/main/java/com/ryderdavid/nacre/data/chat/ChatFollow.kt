package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.data.bridge.BridgeException
import com.ryderdavid.nacre.data.bridge.ChatStreamMessage
import com.ryderdavid.nacre.data.claude.ATTEMPT_MAX_MS
import com.ryderdavid.nacre.data.claude.RECONNECT_BUDGET_MS
import com.ryderdavid.nacre.data.claude.SLEPT_MS
import com.ryderdavid.nacre.data.claude.TRANSIENT_HTTP
import com.ryderdavid.nacre.data.claude.TurnLostException
import com.ryderdavid.nacre.data.claude.reconnectDelay
import com.ryderdavid.nacre.data.model.ChatEvent
import com.ryderdavid.nacre.data.model.ChatRecord
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import java.io.IOException

/** How following a chat's stream ended. */
sealed interface ChatFollowEnd {
    /** The chat is no longer running; [chat] is its state. */
    data class Settled(val chat: ChatRecord) : ChatFollowEnd

    /** The chat was deleted. */
    data class Gone(val reason: String) : ChatFollowEnd
}

/** Whether [e] is worth reattaching after: a drop or a "not right now", not a refusal. */
internal fun transient(e: IOException) = e !is BridgeException || e.code == 0 || e.code in TRANSIENT_HTTP

/**
 * The `followTurn` backoff, shared by following a stream and resending a message: 1 s, 2 s, 4 s,
 * 8 s, then every 10 s, giving up after [budgetMs] of attempts while the phone is awake.
 */
class Backoff internal constructor(
    private val budgetMs: Long = RECONNECT_BUDGET_MS,
    private val pause: suspend (attempt: Int) -> Unit = { delay(reconnectDelay(it)) },
    private val now: () -> Long = System::currentTimeMillis,
) {
    var attempt = 0
        private set
    private var spent = 0L
    private var tryingSince = 0L

    /** A call worked: the next drop starts counting again. */
    fun reset() {
        attempt = 0
        spent = 0
    }

    /** Waits before the next attempt, or throws [TurnLostException] once the budget is spent. */
    suspend fun next(cause: IOException) {
        if (attempt > 0) {
            spent += minOf(now() - tryingSince, ATTEMPT_MAX_MS)
            if (spent > budgetMs) throw TurnLostException(cause)
        }
        attempt++
        val asked = reconnectDelay(attempt)
        val before = now()
        pause(attempt)
        val took = now() - before
        spent += if (took > asked + SLEPT_MS) asked else took
        tryingSince = now()
    }
}

/**
 * Follows a chat's `/stream?after=seq` until the chat settles, reattaching after drops with the
 * `followTurn` backoff from the last event seen. Events are handed to [onEvent] once each, by seq.
 */
internal suspend fun followChat(
    open: (after: Long) -> Flow<ChatStreamMessage>,
    after: Long,
    onEvent: (ChatEvent) -> Unit,
    backoff: Backoff = Backoff(),
    onDrop: (attempt: Int) -> Unit = {},
    onBack: () -> Unit = {},
): ChatFollowEnd {
    var last = after
    while (true) {
        var end: ChatFollowEnd? = null
        try {
            open(last).collect { msg ->
                if (backoff.attempt > 0) {
                    backoff.reset()
                    onBack()
                }
                when (msg) {
                    is ChatStreamMessage.Event -> if (msg.event.seq == 0L || msg.event.seq > last) {
                        if (msg.event.seq > 0) last = msg.event.seq
                        onEvent(msg.event)
                    }
                    is ChatStreamMessage.State -> end = ChatFollowEnd.Settled(msg.chat)
                    is ChatStreamMessage.Gone -> end = ChatFollowEnd.Gone(msg.error)
                }
            }
            end?.let { return it }
            throw IOException("The computer closed the chat's stream early")
        } catch (e: IOException) {
            if (!transient(e)) throw e
            backoff.next(e)
            onDrop(backoff.attempt)
        }
    }
}

/**
 * Sends one message, resending the very same request (same `turn_id`) after a lost response;
 * nacred recognises the turn id and starts no second turn.
 */
internal suspend fun <T> sendWithResend(backoff: Backoff = Backoff(), send: suspend () -> T): T {
    while (true) {
        try {
            return send()
        } catch (e: IOException) {
            if (!transient(e)) throw e
            backoff.next(e)
        }
    }
}
