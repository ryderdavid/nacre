package com.ryderdavid.nacre.data.claude

import com.ryderdavid.nacre.data.Connection
import com.ryderdavid.nacre.data.HostState
import com.ryderdavid.nacre.data.bridge.AssistMessage
import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.data.bridge.BridgeException
import com.ryderdavid.nacre.data.model.AssistRequest
import com.ryderdavid.nacre.data.model.AssistResult
import com.ryderdavid.nacre.data.model.Features
import com.ryderdavid.nacre.data.model.Health
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.settings.AppSettings
import com.ryderdavid.nacre.data.settings.Host
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import java.util.Base64
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** The Mac backend sending images: request shape, the assistant's conversation budget and the target's features. */
class MacImagesTest {
    private val key = Keys.of("h1", "r1")
    private val settings = AppSettings(hosts = listOf(Host("h1", "hid", "Studio", "http://studio", "t")))

    /** Records what would go to nacred; [features] is what the computer reports, null while it isn't connected. */
    private class FakeLink(var features: List<String>? = listOf(Features.CLAUDE, Features.CHAT_IMAGES)) : MacLink {
        val json = mutableListOf<AssistRequest>()
        val streamed = mutableListOf<AssistRequest>()
        var fail: BridgeException? = null
        private var turns = 0

        override suspend fun claudeJson(key: String, req: AssistRequest): AssistResult {
            json += req
            fail?.let { throw it }
            return AssistResult(output = buildJsonObject { put("answer", "ok") })
        }

        override fun claudeStream(key: String, req: AssistRequest): Flow<AssistMessage> {
            streamed += req
            fail?.let { e -> return flow { throw e } }
            turns++
            val output = buildJsonObject { put("reply", "seen"); putJsonArray("proposals") {} }
            return flowOf(AssistMessage.Turn("t$turns"), AssistMessage.Done(AssistResult(sessionId = "s1", output = output)))
        }

        override fun claudeStreamAcross(hostId: String, req: AssistRequest): Flow<AssistMessage> = claudeStream(hostId, req)
        override fun claudeResume(key: String, turn: String, after: Long): Flow<AssistMessage> = error("not resumed")
        override suspend fun claudeCancel(key: String, turn: String) = Unit
        override fun hostName(hostId: String) = if (hostId == "h1") "Studio" else ""
        override fun features(hostId: String) = features
    }

    private object Sink : TurnSink {
        override fun newBlock() = Unit
        override fun text(delta: String) = Unit
        override fun thinking(delta: String) = Unit
        override fun aside(id: String, name: String, summary: String) = Unit
        override fun asideDone(id: String, result: String, isError: Boolean) = Unit
    }

    private val ctx = BeadContext(
        repoName = "nacre", id = "NAC-1", title = "Crash", type = "bug", priority = 1, status = "open",
        labels = emptyList(), description = "It crashes", design = "", acceptance = "", notes = "",
        parentTitle = null, siblingTitles = emptyList(), childTitles = emptyList(),
    )

    private fun image(name: String, size: Int = 10, fill: Byte = 1) =
        ImagePayload(ChatImage.create("image/png", name), ByteArray(size) { fill })

    private fun encode(req: AssistRequest): JsonObject =
        BridgeClient.json.encodeToJsonElement(AssistRequest.serializer(), req).jsonObject

    private fun session(link: FakeLink, runKey: String? = key) =
        MacSession(link, { settings }, AssistantBoard(runKey, "nacre", span = { BoardSpan(listOf(SpanBoard("nacre", listOf(key))), "") }) { "system" })

    /** A bead chat Ask with one image in an earlier turn and two in the current message. */
    private fun beadChat(): Triple<List<DraftTurn>, List<ImagePayload>, ChatTurnImages> {
        val old = image("old.png", fill = 7)
        val a = image("a.png", fill = 8)
        val b = image("b.png", fill = 9)
        val history = listOf(DraftTurn(true, "(asking) what is this?", listOf(old.image)), DraftTurn(false, "A crash."))
        val bytes = listOf(old, a, b).associate { it.image.id to it.bytes }
        val request = resolveImages(history, listOf(a.image, b.image)) { bytes[it.id] }
        return Triple(history, listOf(old, a, b), ChatTurnImages(listOf(a.image, b.image), request))
    }

    @Test fun beadChatRequestCarriesImagesInMarkerOrder() = runBlocking {
        val link = FakeLink()
        val (history, all, images) = beadChat()
        MacBackend(link) { settings }.ask(key, ctx, history, "and now?", images)

        val body = encode(link.json.single())
        val sent = body["images"]!!.jsonArray.map { it.jsonObject }
        assertEquals(listOf("old.png", "a.png", "b.png"), sent.map { it["name"]!!.jsonPrimitive.content })
        assertEquals(listOf("image/png"), sent.map { it["media_type"]!!.jsonPrimitive.content }.distinct())
        assertEquals(all.map { it.bytes.toList() }, sent.map { Base64.getDecoder().decode(it["data"]!!.jsonPrimitive.content).toList() })

        // Each marker sits in its own turn, numbered as the array is ordered.
        val prompt = body["prompt"]!!.jsonPrimitive.content
        val at = listOf("[image 1: old.png]", "[image 2: a.png]", "[image 3: b.png]").map { prompt.indexOf(it) }
        assertTrue(at.all { it >= 0 } && at == at.sorted(), prompt)
        assertTrue(prompt.indexOf("[image 1: old.png]") < prompt.indexOf("<question>"))
        assertEquals(Prompts.ask(ctx, history, "and now?", readsRepo = true, images.current, images.request.marks), prompt)
    }

    @Test fun textOnlyRequestIsTodays() = runBlocking {
        val link = FakeLink(features = listOf(Features.CLAUDE))
        val history = listOf(DraftTurn(true, "(asking) hi"), DraftTurn(false, "hello"))
        MacBackend(link) { settings }.ask(key, ctx, history, "why?", ChatTurnImages.NONE)
        MacBackend(link) { settings }.refine(key, ctx, "tighten it", history, ChatTurnImages.NONE)

        val (ask, refine) = link.json
        val todayAsk = AssistRequest(
            prompt = Prompts.ask(ctx, history, "why?", readsRepo = true), system = Prompts.ADVISOR,
            schema = ClaudeService.askSchema.toSchemaJson(), model = settings.claudeModel, effort = settings.claudeEffort,
            bdReads = true, readRepo = true,
        )
        assertEquals(encode(todayAsk), encode(ask))
        assertFalse("images" in encode(ask))
        assertEquals(Prompts.refine(ctx, "tighten it", history), refine.prompt)
        assertFalse("images" in encode(refine))
    }

    @Test fun beadChatImageRequestsAreNotKept() = runBlocking {
        val link = FakeLink()
        val (history, _, images) = beadChat()
        MacBackend(link) { settings }.refine(key, ctx, "use the screenshot", history, images)
        MacBackend(link) { settings }.ask(key, ctx, history, "?", images)
        for (req in link.json) {
            assertFalse(req.keep)
            assertEquals("", req.sessionId)
            assertFalse("keep" in encode(req))
            assertEquals(3, req.images.size)
        }
    }

    @Test fun secondAssistantTurnCarriesOnlyItsOwnImages() = runBlocking {
        val link = FakeLink()
        val s = session(link)
        val a = image("a.png")
        val b = image("b.png")
        val c = image("c.png")
        s.turn(TurnInput.Say("look", listOf(a, b)), Sink)
        s.turn(TurnInput.Say("and this", listOf(c)), Sink)
        s.turn(TurnInput.Say("thanks"), Sink)

        val (first, second, third) = link.streamed
        assertEquals(listOf("a.png", "b.png"), first.images.map { it.name })
        assertEquals("look\n[image 1: a.png] [image 2: b.png]", first.prompt)
        assertEquals(listOf("c.png"), second.images.map { it.name })
        assertEquals("and this\n[image 1: c.png]", second.prompt)
        assertEquals("s1", second.sessionId)
        assertTrue(second.keep)
        assertEquals("thanks", third.prompt)
        assertFalse("images" in encode(third))
    }

    @Test fun conversationBudgetBlocksImagesUntilReset() = runBlocking {
        val link = FakeLink()
        val s = session(link)
        repeat(3) { s.turn(TurnInput.Say("five", List(5) { image("i$it.png") }), Sink) }
        s.turn(TurnInput.Say("three", List(3) { image("j$it.png") }), Sink)
        assertEquals(18, link.streamed.sumOf { it.images.size })

        val three = List(3) { image("k$it.png") }
        assertEquals(ImageReasons.CONVERSATION_LIMIT, s.imageLimit(3, three.byteCount()))
        val refused = assertFailsWith<ClaudeException> { s.turn(TurnInput.Say("more", three), Sink) }
        assertEquals(ImageReasons.CONVERSATION_LIMIT, refused.message)
        assertEquals(4, link.streamed.size, "nothing was sent")
        assertNull(s.imageLimit(2, 20), "two more fit")

        // Text alone still goes.
        s.turn(TurnInput.Say("just text"), Sink)
        assertEquals(5, link.streamed.size)

        s.reset()
        assertNull(s.imageLimit(3, three.byteCount()))
        s.turn(TurnInput.Say("more", three), Sink)
        assertEquals(3, link.streamed.last().images.size)
        assertEquals("", link.streamed.last().sessionId, "reset starts a new session")
    }

    @Test fun conversationByteBudgetBlocksPast15MB() = runBlocking {
        val link = FakeLink()
        val s = session(link)
        s.turn(TurnInput.Say("big", List(5) { image("big$it.jpg", size = 3_000_000) }), Sink)
        assertEquals(ImageReasons.CONVERSATION_LIMIT, s.imageLimit(1, 1))
        assertFailsWith<ClaudeException> { s.turn(TurnInput.Say("one more", listOf(image("x.png", size = 1))), Sink) }
        assertEquals(1, link.streamed.size)
    }

    @Test fun failedTurnKeepsItsImagesCounted() = runBlocking {
        val link = FakeLink().apply { fail = BridgeException("claude failed", code = 500) }
        val s = session(link)
        assertFailsWith<ClaudeUnavailableException> { s.turn(TurnInput.Say("five", List(5) { image("i$it.png") }), Sink) }
        link.fail = null
        repeat(3) { s.turn(TurnInput.Say("five", List(5) { image("j$it.png") }), Sink) }
        assertEquals(ImageReasons.CONVERSATION_LIMIT, s.imageLimit(1, 1))
    }

    @Test fun acrossBoardsSendsNoImages() = runBlocking {
        val link = FakeLink()
        val s = session(link, runKey = null)
        assertEquals(ImageReasons.ACROSS_BOARDS, s.imageLimit(1, 10))
        val e = assertFailsWith<ClaudeException> { s.turn(TurnInput.Say("look", listOf(image("a.png"))), Sink) }
        assertEquals(ImageReasons.ACROSS_BOARDS, e.message)
        assertTrue(link.streamed.isEmpty())
        s.turn(TurnInput.Say("text"), Sink)
        assertFalse("images" in encode(link.streamed.single()))
    }

    @Test fun missingFeatureRefusesImagesButNotText() = runBlocking {
        val link = FakeLink(features = listOf(Features.CLAUDE))
        val (history, _, images) = beadChat()
        val backend = MacBackend(link) { settings }
        val e = assertFailsWith<ClaudeException> { backend.ask(key, ctx, history, "?", images) }
        assertEquals("Update nacred on Studio to send images", e.message)
        assertTrue(link.json.isEmpty(), "nothing was sent")
        backend.ask(key, ctx, emptyList(), "text only", ChatTurnImages.NONE)
        assertEquals(1, link.json.size)

        val s = session(link)
        val a = assertFailsWith<ClaudeException> { s.turn(TurnInput.Say("look", listOf(image("a.png"))), Sink) }
        assertEquals("Update nacred on Studio to send images", a.message)
        assertTrue(link.streamed.isEmpty())
        assertNull(s.imageLimit(1, 10), "a refused send doesn't count")
        s.turn(TurnInput.Say("text"), Sink)
        assertEquals(1, link.streamed.size)

        // Not connected: what it can do isn't known, so images wait.
        link.features = null
        assertEquals(ImageReasons.unreachable("Studio"), assertFailsWith<ClaudeException> { backend.ask(key, ctx, history, "?", images) }.message)

        assertEquals("Update nacred on Studio to send images", imageSupport(true, key, listOf(host(listOf(Features.CLAUDE)))).reason)
        assertTrue(imageSupport(true, key, listOf(host(listOf(Features.CLAUDE, Features.CHAT_IMAGES)))).ok)
        assertTrue(imageSupport(false, key, listOf(host(listOf(Features.CLAUDE)))).ok, "the API backend needs no feature")
        assertEquals(ImageReasons.ACROSS_BOARDS, imageSupport(false, null, emptyList()).reason)
    }

    private fun host(features: List<String>?) = HostState(
        settings.hosts.single(),
        if (features == null) Connection.Offline("down") else Connection.Online(Health(features = features)),
    )

    @Test fun supportFollowsTheTargetsFeaturesAndTheBackend() = runBlocking {
        val prefs = MutableStateFlow(settings.copy(claudeRunner = AppSettings.RUNNER_API))
        val hosts = MutableStateFlow(listOf(host(listOf(Features.CLAUDE))))
        val support = ChatImageSupport(prefs, hosts).of(key)
        // Images pending on the API backend.
        assertTrue(support.first().ok)

        // Switched to the Mac, whose nacred is older.
        prefs.value = settings.copy(claudeRunner = AppSettings.RUNNER_MAC)
        assertEquals("Update nacred on Studio to send images", support.first().reason)

        // Updated: its refreshed features list chat_images.
        hosts.value = listOf(host(listOf(Features.CLAUDE, Features.CHAT_IMAGES)))
        assertTrue(support.first().ok)

        // Downgraded while images are pending: refused again, and at send too.
        hosts.value = listOf(host(listOf(Features.CLAUDE)))
        assertEquals("Update nacred on Studio to send images", support.first().reason)
        val link = FakeLink(features = hosts.value.single().let { (it.connection as Connection.Online).health.features })
        val (history, _, images) = beadChat()
        assertFailsWith<ClaudeException> { MacBackend(link) { settings }.refine(key, ctx, "x", history, images) }
        assertTrue(link.json.isEmpty())

        // Another board's computer doesn't matter.
        assertEquals("Update nacred on Studio to send images", imageSupport(true, key, hosts.value + HostState(Host("h2", "x", "Air", "u", "t"), Connection.Online(Health(features = listOf(Features.CHAT_IMAGES))))).reason)
    }

    @Test fun nacredRefusalsShowTheirReason() = runBlocking {
        val (history, _, images) = beadChat()
        for ((code, reason) in listOf(400 to "image 2 (a.png): bytes don't match image/png", 413 to "request body too large: turn bodies are limited to 32 MiB")) {
            val link = FakeLink().apply { fail = BridgeException(reason, code = code) }
            val e = assertFailsWith<ClaudeException> { MacBackend(link) { settings }.ask(key, ctx, history, "?", images) }
            assertEquals("The computer didn't accept the images: $reason", e.message)

            val s = session(link)
            val t = assertFailsWith<ClaudeException> { s.turn(TurnInput.Say("look", listOf(image("a.png"))), Sink) }
            assertEquals("The computer didn't accept the images: $reason", t.message)
        }
        // A text-only failure reads as it always has.
        val link = FakeLink().apply { fail = BridgeException("invalid JSON body", code = 400) }
        val e = assertFailsWith<ClaudeUnavailableException> { MacBackend(link) { settings }.ask(key, ctx, history.map { it.copy(images = emptyList()) }, "?", ChatTurnImages.NONE) }
        assertEquals("invalid JSON body", e.message)
    }
}
