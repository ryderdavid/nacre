package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.data.model.ChatEvent
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.data.model.ChatWire
import com.ryderdavid.nacre.data.model.CreateChat
import com.ryderdavid.nacre.data.model.Health
import com.ryderdavid.nacre.data.model.ScreenContext
import com.ryderdavid.nacre.data.model.SendChatMessage
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/** 8.1: the chat models read and write nacred's `/v1/chats` JSON. */
class ChatWireTest {
    private val json = BridgeClient.json

    @Test fun `a chat record and its events decode from nacred's JSON`() {
        val detail = json.decodeFromString<BridgeClient.ChatDetail>(
            """{"chat":{"id":"k3x","agent":"claude","repo":null,"host_id":"h","host_name":"studio","cwd":"/s","add_dirs":[],
               "title":"New chat · all boards","created_at":"2026-11-05T10:00:00Z","updated_at":"2026-11-05T10:01:00Z",
               "state":"waiting_permission","turn_id":"c_1","last_seq":3,"session_started":true},
               "events":[{"seq":1,"ts":"t","kind":"user","turn_id":"c_1","text":"hi","context":{"screen":"inbox"}},
                         {"seq":2,"ts":"t","kind":"text","text":"Hello","route":"full"},
                         {"seq":3,"ts":"t","kind":"permission","request_id":"r1","tool":"Bash","text":"Bash · rm -rf build","reason":"Deletes files","input":{"command":"rm -rf build"}}]}""",
        )
        assertEquals(ChatWire.WAITING_PERMISSION, detail.chat.state)
        assertEquals(null, detail.chat.repo)
        assertEquals(3, detail.chat.lastSeq)
        assertEquals("inbox", detail.events[0].context?.screen)
        assertEquals("full", detail.events[1].route)
        assertEquals("r1", detail.events[2].requestId)
        val t = ChatTranscript().apply { addAll(detail.events) }
        assertEquals("Bash · rm -rf build", t.pending.single().step)
    }

    @Test fun `messages carry turn_id and the context in snake case, without empty fields`() {
        val body = json.encodeToString(
            SendChatMessage.serializer(),
            SendChatMessage("c_1", "close this one", ScreenContext(ScreenContext.BEAD, repo = "nacre", beadId = "NAC-3", elsewhere = null)),
        )
        assertTrue(body.contains("\"turn_id\":\"c_1\""))
        assertTrue(body.contains("\"bead_id\":\"NAC-3\""))
        assertFalse(body.contains("elsewhere"))
        val create = json.encodeToString(CreateChat.serializer(), CreateChat("codex", null, ScreenContext(ScreenContext.BOARDS)))
        assertTrue(create.contains("\"agent\":\"codex\""))
        assertFalse(create.contains("model"))
    }

    @Test fun `health says why Chat is unavailable`() {
        val h = json.decodeFromString<Health>("""{"features":["claude"],"agent_chat_unavailable":"Needs Claude Code 2.3."}""")
        assertEquals("Needs Claude Code 2.3.", h.agentChatUnavailable)
        assertFalse(servesChat(h))
        assertTrue(servesChat(Health(features = listOf("agent_chat"))))
    }

    @Test fun `a record's key names its paired computer`() {
        assertEquals("h1~k3x", ChatRecord("k3x", hostId = "h1").key)
        assertEquals("", ChatEvent().kind)
    }
}
