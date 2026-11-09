package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.data.bridge.ChatStreamMessage
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.data.model.ChatWire
import com.ryderdavid.nacre.data.model.PermissionOutcome
import com.ryderdavid.nacre.data.model.ScreenContext
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.runBlocking
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** 8.7: following a chat, resending, the handoff line and permission cards. */
class ChatConversationTest {
    private val key = "a~c1"
    private val running = ChatRecord("c1", state = ChatWire.RUNNING)
    private val idle = ChatRecord("c1", state = ChatWire.IDLE)

    private fun conversation(link: FakeChatLink, scope: CoroutineScope, notices: ChatNotices? = null, turnIds: Iterator<String> = generateSequence(1) { it + 1 }.map { "c_$it" }.iterator()) =
        ChatConversation(key, link, scope, notices, newTurnId = { turnIds.next() }, backoff = ::quickBackoff)

    @Test fun `a dropped stream is reattached after the last seq, with no event missing or shown twice`() = runBlocking {
        val link = FakeChatLink(detail = { BridgeClient.ChatDetail(running, listOf(ev(1, ChatWire.USER, "build it"))) })
        // The first stream drops after seq 3; nacred replays 3 again on reattach (a resend of the
        // last frame), then 4 and 5, and settles.
        link.streams += { _, _ -> stream(*events(ev(2, ChatWire.TEXT, "Building"), ev(3, ChatWire.TEXT, "…")), drop = true) }
        link.streams += { _, _ -> stream(*events(ev(3, ChatWire.TEXT, "…"), ev(4, ChatWire.TEXT, " done")), drop = true) }
        link.streams += { _, _ -> stream(*events(ev(4, ChatWire.TEXT, " done"), ev(5, ChatWire.TURN_END)), ChatStreamMessage.State(idle)) }
        val conv = conversation(link, this)
        conv.load()
        conv.startFollow().join()
        assertEquals(listOf(1L, 3L, 4L), link.opened.map { it.second })
        assertEquals(listOf(1L, 2L, 3L, 4L, 5L), conv.seqs)
        val s = conv.state.value
        assertEquals(ChatWire.IDLE, s.chat?.state)
        assertEquals("Building… done", (s.items[1] as ChatItem.Assistant).text)
        assertFalse(s.turnRunning)
        assertFalse(s.reconnecting)
    }

    @Test fun `reattaching asks for events after the last one shown`() = runBlocking {
        val link = FakeChatLink(detail = { BridgeClient.ChatDetail(running, listOf(ev(7, ChatWire.USER, "hi"))) })
        link.streams += { _, _ -> stream(*events(ev(8, ChatWire.TEXT, "a")), drop = true) }
        link.streams += { _, _ -> stream(ChatStreamMessage.State(idle)) }
        val conv = conversation(link, this)
        conv.load()
        conv.startFollow().join()
        assertEquals(listOf(7L, 8L), link.opened.map { it.second })
    }

    @Test fun `a resend after a lost response keeps its turn_id`() = runBlocking {
        val link = FakeChatLink(detail = { BridgeClient.ChatDetail(idle) })
        link.sendFailures = 2
        link.streams += { _, _ -> stream(*events(ev(1, ChatWire.USER, "hello"), ev(2, ChatWire.TEXT, "hi")), ChatStreamMessage.State(idle)) }
        val conv = conversation(link, this)
        conv.load()
        val turn = conv.sendNow("hello", ScreenContext(ScreenContext.INBOX))
        assertEquals("c_1", turn)
        assertEquals(3, link.sent.size)
        assertEquals(setOf("c_1"), link.sent.map { it.turnId }.toSet())
        assertTrue(link.sent.all { it.text == "hello" && it.context?.screen == ScreenContext.INBOX })
        until { conv.state.value.chat?.state == ChatWire.IDLE && conv.seqs.size == 2 }
        // The next message is a new turn.
        link.streams += { _, _ -> stream(ChatStreamMessage.State(idle)) }
        assertEquals("c_2", conv.sendNow("again", null))
    }

    @Test fun `Send is disabled while a turn runs and once the chat reached its limit`() {
        assertFalse(ConversationState(chat = running, loading = false).canSend)
        assertTrue(ConversationState(chat = running).turnRunning)
        assertTrue(ConversationState(chat = idle, loading = false).canSend)
        val limit = ConversationState(chat = ChatRecord("c1", state = ChatWire.LIMIT_REACHED, reason = LIMIT_REASON))
        assertFalse(limit.canSend)
        assertEquals("This chat reached its runaway limit. Start a new chat to keep going.", limit.limitReason)
        assertEquals(LIMIT_REASON, ConversationState(chat = ChatRecord("c1", state = ChatWire.LIMIT_REACHED)).limitReason)
    }

    @Test fun `the handoff shows as the switching-to-the-full-agent line`() {
        val t = ChatTranscript()
        t.add(ev(1, ChatWire.USER, "now do it"))
        t.add(ev(2, ChatWire.TEXT, "Let me look.").copy(route = ChatWire.ROUTE_LIGHT))
        t.add(ev(3, ChatWire.HANDOFF, "Switching to the full agent: needs to edit files"))
        t.add(ev(4, ChatWire.HANDOFF, reason = "wants to leave plan mode"))
        // An older bridge's line is shown as it came.
        t.add(ev(5, ChatWire.HANDOFF, "Handed to the full harness: needs to push"))
        t.add(ev(6, ChatWire.TEXT, "Editing now.").copy(route = ChatWire.ROUTE_FULL))
        val lines = t.shown.filterIsInstance<ChatItem.Handoff>().map { it.text }
        assertEquals(listOf("Switching to the full agent: needs to edit files", "Switching to the full agent: wants to leave plan mode", "Handed to the full harness: needs to push"), lines)
        // Light and full text stay separate items.
        assertEquals(listOf("light", "full"), t.shown.filterIsInstance<ChatItem.Assistant>().map { it.route })
    }

    @Test fun `tool steps pair with their results`() {
        val t = ChatTranscript()
        t.add(ev(1, ChatWire.TOOL_USE, tool = "Bash").copy(toolId = "t1", input = kotlinx.serialization.json.buildJsonObject { put("command", kotlinx.serialization.json.JsonPrimitive("bd ready")) }))
        t.add(ev(2, ChatWire.TOOL_RESULT, "NAC-1 ready").copy(toolId = "t1"))
        val step = t.shown.single() as ChatItem.ToolStep
        assertEquals("Bash · bd ready", step.summary)
        assertEquals("NAC-1 ready", step.result)
    }

    private fun cardOf(conv: ChatConversation, req: String) =
        conv.state.value.items.filterIsInstance<ChatItem.Permission>().single { it.card.requestId == req }.card

    private fun waiting(vararg reqs: String) = BridgeClient.ChatDetail(
        ChatRecord("c1", state = ChatWire.WAITING_PERMISSION),
        listOf(ev(1, ChatWire.USER, "build and send me this APK")) + reqs.mapIndexed { i, r -> permission(2L + i, r) },
    )

    @Test fun `Allow turns the card into a read-only Allowed record and clears its notification`() = runBlocking {
        val notices = RecordingNotices()
        val link = FakeChatLink(detail = { waiting("r1") })
        link.streams += { _, _ -> stream() } // not used: following is not started here
        val conv = conversation(link, this, notices)
        conv.load()
        conv.close()
        assertTrue(cardOf(conv, "r1").pending)
        assertEquals("Bash · ./gradlew assembleDebug", cardOf(conv, "r1").step)
        conv.answerNow("r1", allow = true, note = null)
        val card = cardOf(conv, "r1")
        assertEquals(CardState.ALLOWED, card.state)
        assertEquals("Allowed", card.outcomeLine)
        assertEquals(listOf(Triple("r1", true, null as String?)), link.answers)
        assertEquals(listOf("$key/r1"), notices.cleared)
    }

    @Test fun `Deny with a note records Denied with the note`() = runBlocking {
        val link = FakeChatLink(detail = { waiting("r1") })
        link.streams += { _, _ -> stream() }
        val conv = conversation(link, this)
        conv.load()
        conv.close()
        conv.answerNow("r1", allow = false, note = "use the debug build")
        val card = cardOf(conv, "r1")
        assertEquals(CardState.DENIED, card.state)
        assertEquals("Denied: use the debug build", card.outcomeLine)
        assertEquals("use the debug build", link.answers.single().third)
    }

    @Test fun `a request that timed out shows the timeout as a read-only outcome`() {
        val t = ChatTranscript()
        t.add(permission(1, "r1"))
        t.add(ev(2, ChatWire.PERMISSION_ANSWER, "Denied: No answer from the phone within 15 minutes", req = "r1", allowed = false, by = "timeout", reason = "No answer from the phone within 15 minutes"))
        val card = t.cardsById.getValue("r1")
        assertEquals(CardState.TIMED_OUT, card.state)
        assertFalse(card.pending)
        assertEquals("Denied: No answer from the phone within 15 minutes", card.outcomeLine)
        assertTrue(t.pending.isEmpty())
    }

    @Test fun `answered elsewhere, a 409 shows the recorded outcome`() = runBlocking {
        val notices = RecordingNotices()
        val link = FakeChatLink(detail = { waiting("r1") })
        link.streams += { _, _ -> stream() }
        // Another phone allowed it first; nacred answers 409 with the recorded answer.
        link.answer = { req, _, _ -> PermissionOutcome(ev(3, ChatWire.PERMISSION_ANSWER, "Allowed", req = req, allowed = true, by = "phone"), conflict = true) }
        val conv = conversation(link, this, notices)
        conv.load()
        conv.close()
        conv.answerNow("r1", allow = false, note = "no")
        assertEquals(CardState.ALLOWED, cardOf(conv, "r1").state)
        assertNull(conv.state.value.error)
        assertEquals(listOf("$key/r1"), notices.cleared)
    }

    @Test fun `a 409 without the outcome reads the log again`() = runBlocking {
        var decided = false
        val link = FakeChatLink(detail = {
            if (!decided) waiting("r1") else BridgeClient.ChatDetail(
                ChatRecord("c1", state = ChatWire.RUNNING),
                waiting("r1").events + ev(3, ChatWire.PERMISSION_ANSWER, req = "r1", allowed = false, by = "stop", reason = "Stopped"),
            )
        })
        link.answer = { _, _, _ -> decided = true; PermissionOutcome(null, conflict = true) }
        val conv = conversation(link, this)
        conv.load()
        conv.close()
        conv.answerNow("r1", allow = true, note = null)
        assertEquals(CardState.CLOSED, cardOf(conv, "r1").state)
        assertEquals("Denied: Stopped", cardOf(conv, "r1").outcomeLine)
    }

    @Test fun `a decided card keeps its outcome when the answer arrives again`() {
        val t = ChatTranscript()
        t.add(permission(1, "r1"))
        t.settle(ev(0, ChatWire.PERMISSION_ANSWER, req = "r1", allowed = true, by = "phone"))
        t.add(ev(2, ChatWire.PERMISSION_ANSWER, req = "r1", allowed = false, by = "timeout"))
        assertEquals(CardState.ALLOWED, t.cardsById.getValue("r1").state)
    }
}
