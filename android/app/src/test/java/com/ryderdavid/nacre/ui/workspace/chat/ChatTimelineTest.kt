package com.ryderdavid.nacre.ui.workspace.chat

import com.ryderdavid.nacre.data.chat.ChatItem
import com.ryderdavid.nacre.data.chat.ChatTranscript
import com.ryderdavid.nacre.data.model.ChatEvent
import com.ryderdavid.nacre.data.model.ChatWire
import com.ryderdavid.nacre.data.model.Decision
import com.ryderdavid.nacre.data.model.WorkSummary
import kotlin.test.Test
import kotlin.test.assertEquals

class ChatTimelineTest {
    private fun user(key: String) = ChatItem.User(key, "hi $key", null, emptyList())

    @Test fun anOldCardStaysWhereItWasRaisedAndNewMessagesFollowIt() {
        // The chat that showed the bug: cards from yesterday, messages from today.
        val items = listOf(user("e1"), user("e2"), user("e3"))
        val times = mapOf(
            "e1" to "2026-11-06T22:27:00Z",
            "e2" to "2026-11-07T22:36:16.102422Z",
            "e3" to "2026-11-07T22:36:51.8884Z",
        )
        val card = Decision(id = "dc1", createdAt = "2026-11-06T22:27:46.98925Z")
        val rows = ChatTimeline.merge(items, times, listOf(card), emptyList())
        assertEquals(listOf("e1", "d:dc1", "e2", "e3"), rows.map { it.key })
    }

    @Test fun cardsAndWorkInterleaveByTimeNotByText() {
        val items = listOf(user("e1"), user("e2"))
        // As text "10:00:00Z" sorts after "10:00:00.5Z"; as times it is half a second before.
        val times = mapOf("e1" to "2026-11-07T10:00:00Z", "e2" to "2026-11-07T10:00:01Z")
        val card = Decision(id = "mid", createdAt = "2026-11-07T10:00:00.5Z")
        val work = WorkSummary(kind = "round", id = "r1", startedAt = "2026-11-07T09:59:59Z")
        val late = Decision(id = "late", createdAt = "2026-11-07T11:00:00Z")
        val rows = ChatTimeline.merge(items, times, listOf(late, card), listOf(work))
        assertEquals(listOf("w:roundr1", "e1", "d:mid", "e2", "d:late"), rows.map { it.key })
    }

    @Test fun aCardWithoutATimeGoesLast() {
        val items = listOf(user("e1"))
        val rows = ChatTimeline.merge(items, mapOf("e1" to "2026-11-07T10:00:00Z"), listOf(Decision(id = "x")), emptyList())
        assertEquals(listOf("e1", "d:x"), rows.map { it.key })
    }

    @Test fun theTranscriptRecordsEachItemsTime() {
        val t = ChatTranscript()
        t.add(ChatEvent(seq = 1, ts = "2026-11-07T10:00:00Z", kind = ChatWire.USER, text = "hello"))
        t.add(ChatEvent(seq = 2, ts = "2026-11-07T10:00:01Z", kind = ChatWire.TEXT, text = "Hi", route = "full"))
        t.add(ChatEvent(seq = 3, ts = "2026-11-07T10:00:02Z", kind = ChatWire.TEXT, text = " there", route = "full"))
        // A reply's later chunks join its item, which keeps the time it started.
        assertEquals(mapOf("e1" to "2026-11-07T10:00:00Z", "e2" to "2026-11-07T10:00:01Z"), t.times.filterKeys { k -> t.shown.any { it.key == k } })
    }
}
