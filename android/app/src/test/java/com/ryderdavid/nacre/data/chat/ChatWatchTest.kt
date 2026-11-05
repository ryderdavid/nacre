package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.data.bridge.ChatStreamMessage
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.data.model.ChatWire
import com.ryderdavid.nacre.data.model.Keys
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.runBlocking
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/** 8.7: background watching of chats at work, and one "Chat needs your OK" per request. */
class ChatWatchTest {
    private fun rec(id: String, state: String) = ChatRecord(id, state = state, hostId = "a")

    @Test fun `a waiting chat with no active run keeps the watcher running and posts exactly one notification`() = runBlocking {
        val notices = RecordingNotices()
        val settle = CompletableDeferred<Unit>()
        val link = FakeChatLink(
            listing = ChatListing(listOf(rec("c1", ChatWire.WAITING_PERMISSION), rec("c2", ChatWire.IDLE))),
            detail = { BridgeClient.ChatDetail(ChatRecord(Keys.id(it), state = ChatWire.WAITING_PERMISSION), listOf(ev(1, ChatWire.USER, "send me the APK"), permission(2, "r1"))) },
        )
        // The stream drops once (the watcher reattaches and is sent r1 again), then the answer
        // arrives from the phone, the turn ends and the chat settles.
        link.streams += { _, _ -> stream(*events(permission(2, "r1")), drop = true) }
        link.streams += { _, _ ->
            flow {
                emit(ChatStreamMessage.Event(permission(2, "r1")))
                settle.await()
                emit(ChatStreamMessage.Event(ev(3, ChatWire.PERMISSION_ANSWER, req = "r1", allowed = true, by = "phone")))
                emit(ChatStreamMessage.Event(ev(4, ChatWire.TURN_END)))
                emit(ChatStreamMessage.State(ChatRecord("c1", state = ChatWire.IDLE)))
            }
        }
        val watch = ChatWatch(link, this, notices, inBackground = { true }, backoff = ::quickBackoff)
        watch.refresh()
        // Only the chat at work is watched; no run is active, yet the watcher keeps running.
        assertEquals(setOf("a~c1"), watch.watching.value)
        assertTrue(watcherShouldRun(activeRuns = 0, workingChanges = 0, watchedChats = watch.watching.value.size))
        until { link.opened.size == 2 }
        // A refresh while watching doesn't start a second follower.
        watch.refresh()
        assertEquals(2, link.opened.size)
        assertEquals(listOf("a~c1/r1"), notices.posted)

        settle.complete(Unit)
        until { watch.watching.value.isEmpty() }
        assertEquals(listOf("a~c1/r1"), notices.posted)
        assertEquals(listOf("a~c1/r1"), notices.cleared)
        // Nothing left to watch and no run active: the watcher stops.
        assertFalse(watcherShouldRun(activeRuns = 0, workingChanges = 0, watchedChats = watch.watching.value.size))
        assertTrue(watcherShouldRun(activeRuns = 1, workingChanges = 0, watchedChats = 0))
    }

    @Test fun `a reattach after the watcher dropped a chat never posts the same request twice`() = runBlocking {
        val notices = RecordingNotices()
        val link = FakeChatLink(
            listing = ChatListing(listOf(rec("c1", ChatWire.WAITING_PERMISSION))),
            detail = { BridgeClient.ChatDetail(ChatRecord("c1", state = ChatWire.WAITING_PERMISSION), listOf(permission(1, "r1"))) },
        )
        // First follow fails outright (refused), so the chat is dropped; the next refresh learns it again.
        link.streams += { _, _ -> flow { throw com.ryderdavid.nacre.data.bridge.BridgeException("forbidden", code = 403) } }
        link.streams += { _, _ ->
            stream(*events(permission(1, "r1"), permission(2, "r2")), ChatStreamMessage.State(ChatRecord("c1", state = ChatWire.WAITING_PERMISSION)))
        }
        val watch = ChatWatch(link, this, notices, inBackground = { true }, backoff = ::quickBackoff)
        watch.refresh()
        until { watch.watching.value.isEmpty() }
        watch.refresh()
        until { watch.watching.value.isEmpty() && link.opened.size == 2 }
        assertEquals(listOf("a~c1/r1", "a~c1/r2"), notices.posted)
    }

    @Test fun `nothing is posted while the app is in the foreground, and posted once it is backgrounded`() = runBlocking {
        val notices = RecordingNotices()
        var background = false
        val link = FakeChatLink(
            listing = ChatListing(listOf(rec("c1", ChatWire.WAITING_PERMISSION))),
            detail = { BridgeClient.ChatDetail(ChatRecord("c1", state = ChatWire.IDLE), listOf(permission(1, "r1"))) },
        )
        val watch = ChatWatch(link, this, notices, inBackground = { background }, backoff = ::quickBackoff)
        watch.refresh()
        until { watch.watching.value.isEmpty() }
        assertTrue(notices.posted.isEmpty())
        background = true
        watch.refresh()
        until { watch.watching.value.isEmpty() && notices.posted.isNotEmpty() }
        assertEquals(listOf("a~c1/r1"), notices.posted)
    }

    @Test fun `idle, interrupted and limit_reached chats are not watched`() = runBlocking {
        val link = FakeChatLink(listing = ChatListing(listOf(rec("a", ChatWire.IDLE), rec("b", ChatWire.INTERRUPTED), rec("c", ChatWire.LIMIT_REACHED))))
        val watch = ChatWatch(link, this, RecordingNotices(), inBackground = { true }, backoff = ::quickBackoff)
        watch.refresh()
        assertTrue(watch.watching.value.isEmpty())
        assertTrue(link.opened.isEmpty())
    }
}
