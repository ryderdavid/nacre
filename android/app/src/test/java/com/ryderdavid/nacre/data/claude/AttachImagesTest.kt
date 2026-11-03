package com.ryderdavid.nacre.data.claude

import com.ryderdavid.nacre.data.Connection
import com.ryderdavid.nacre.data.HostState
import com.ryderdavid.nacre.data.bridge.AssistMessage
import com.ryderdavid.nacre.data.model.AssistRequest
import com.ryderdavid.nacre.data.model.AssistResult
import com.ryderdavid.nacre.data.model.Features
import com.ryderdavid.nacre.data.model.Health
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.settings.AppSettings
import com.ryderdavid.nacre.data.settings.Host
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import java.io.File
import java.nio.file.Files
import kotlin.test.AfterTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** Attaching images in bead chats and the board assistant (task 5.4). */
class AttachImagesTest {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    @AfterTest fun tearDown() = scope.cancel()

    private val key = Keys.of("h1", "r1")
    private val bead = ChatContext.Bead(key, "NAC-1")
    private val board = ChatContext.Board(key)

    private fun image(name: String, size: Int = 10) = ImagePayload(ChatImage.create("image/png", name), ByteArray(size) { 7 })

    // ---- pending images live with the draft ----

    @Test fun pendingImagesSurviveCollapseAndPaneSwaps() {
        val stack = ChatStack()
        val shot = image("shot.png")
        stack.select(board, bead)
        stack.updateView(bead) { it.copy(draft = "what's wrong?", images = listOf(shot)) }

        stack.collapse(board)
        stack.expand(board)
        assertEquals(listOf(shot), stack.view(bead).images, "collapsing and expanding keeps them")

        // Moving from the board's docked pane to the bead's own screen (the sheet) and back.
        stack.select(bead, bead)
        stack.close(board)
        assertEquals(listOf(shot), stack.view(bead).images)
        stack.select(board, bead)
        stack.close(bead)
        assertEquals(ChatViewState(draft = "what's wrong?", images = listOf(shot)), stack.view(bead))
        assertTrue(stack.active.value.isEmpty(), "nothing was sent")
    }

    @Test fun theLimitDisablesAttachUntilOneIsRemoved() {
        val stack = ChatStack()
        stack.select(bead, bead)
        val five = List(5) { image("$it.png") }
        stack.updateView(bead) { it.copy(images = five) }
        val full = composerGate(false, stack.view(bead).images, ImageSupport.YES)
        assertFalse(full.canAttach)
        assertEquals(ComposerReasons.FULL, full.attachReason)
        assertEquals(0, full.remaining)
        assertTrue(full.canSend, "five images can still be sent")

        stack.removePending(bead, five[2].image.id)
        val four = composerGate(false, stack.view(bead).images, ImageSupport.YES)
        assertTrue(four.canAttach)
        assertNull(four.attachReason)
        assertEquals(1, four.remaining)

        assertEquals(5, List(4) { image("a$it") }.plusPicked(List(3) { image("b$it") }).size, "a pick never goes past five")
    }

    @Test fun removingAPendingImageKeepsTheOthersInOrder() {
        val stack = ChatStack()
        stack.select(bead, bead)
        val (a, b, c) = List(3) { image("$it.png") }
        stack.updateView(bead) { it.copy(draft = "look", images = listOf(a, b, c)) }
        stack.removePending(bead, b.image.id)
        assertEquals(listOf(a, c), stack.view(bead).images)
        assertEquals("look", stack.view(bead).draft)
    }

    // ---- images-only sends ----

    @Test fun anImagesOnlyBeadChatMessageIsSentWithItsImage() = runBlocking {
        val dir = Files.createTempDirectory("bead-chats").toFile()
        val chats = BeadChats(File(dir, "bead-chats.json"), CoroutineScope(Dispatchers.Unconfined), Dispatchers.Unconfined)
        val shot = image("shot.png")
        val gate = composerGate(hasText = false, pending = listOf(shot), support = ImageSupport.YES)
        assertTrue(gate.canSend, "an image alone enables Send")
        assertFalse(composerGate(false, emptyList(), ImageSupport.YES).canSend, "nothing at all doesn't")

        val message = assertNotNull(beadChatMessage("  ", ChatMode.ASK, listOf(shot)))
        assertEquals("", message.text)
        assertEquals(listOf(shot.image), message.images)
        assertNull(beadChatMessage("", ChatMode.ASK), "an empty Ask without images sends nothing")

        val key = BeadChats.key(key, "NAC-1")
        var asked: RequestImages? = null
        chats.send(key, message, retrying = false, images = listOf(shot)) { history ->
            asked = chats.requestImages(history.turns(), message.images)
            BeadChatItem.Answer("A screenshot")
        }
        assertEquals(listOf(message, BeadChatItem.Answer("A screenshot")), chats.items(key).first())
        assertEquals(listOf(shot), asked!!.images, "the request carries the saved image")
    }

    @Test fun anImagesOnlyAssistantMessageIsSentWithItsImage() = runBlocking {
        val session = Recorder()
        val a = assistant(session)
        val shot = image("shot.png")
        assertTrue(a.send("", listOf(shot)))
        a.awaitReply()
        assertEquals<List<TurnInput>>(listOf(TurnInput.Say("", listOf(shot))), session.inputs)
        assertEquals(listOf(shot), a.items.value.filterIsInstance<ChatItem.User>().single().images)
        assertFalse(a.send("", emptyList()), "nothing to send")
    }

    // ---- losing image support with images pending ----

    @Test fun losingImageSupportKeepsPendingImagesAndBlocksSendUntilTheyAreRemoved() = runBlocking {
        val stack = ChatStack()
        stack.select(bead, bead)
        val shot = image("shot.png")
        stack.updateView(bead) { it.copy(draft = "see this", images = listOf(shot)) }

        val settings = MutableStateFlow(AppSettings(claudeRunner = AppSettings.RUNNER_API, hosts = listOf(Host("h1", "hid", "Studio", "http://studio", "t"))))
        val hosts = MutableStateFlow(listOf(host(listOf(Features.CLAUDE, Features.CHAT_IMAGES))))
        val support = ChatImageSupport(settings, hosts).of(key)
        assertTrue(composerGate(true, stack.view(bead).images, support.first()).canSend, "the API backend takes images")

        // Switched to the Mac, whose nacred is older; then the Mac's features refresh the same way.
        settings.value = settings.value.copy(claudeRunner = AppSettings.RUNNER_MAC)
        hosts.value = listOf(host(listOf(Features.CLAUDE)))
        val lost = composerGate(true, stack.view(bead).images, support.first())
        assertFalse(lost.canSend)
        assertFalse(lost.canAttach)
        assertEquals(ImageReasons.update("Studio"), lost.sendReason)
        assertEquals(ImageReasons.update("Studio"), lost.attachReason)
        assertEquals(listOf(shot), stack.view(bead).images, "the pending images are kept")

        stack.removePending(bead, shot.image.id)
        val text = composerGate(true, stack.view(bead).images, support.first())
        assertTrue(text.canSend, "the text alone can be sent as today")
        assertNull(text.sendReason)
        assertFalse(text.canAttach)
    }

    // ---- the Mac assistant's conversation budget ----

    @Test fun reachingTheMacConversationLimitKeepsPendingImagesAndBlocksSendWithTheReason() = runBlocking {
        val link = Link()
        val mac = MacBackend(link) { macSettings }
        val a = BoardAssistant(ClaudeBackends(mac, mac), { macSettings }, SingleBoard(Tools), scope)
        val stack = ChatStack()
        stack.select(board, board)

        // 18 images sent across four messages.
        assertTrue(a.send("first", List(5) { image("a$it.png") })); a.awaitReplies(1)
        repeat(3) { n -> assertTrue(a.send("more $n", List(5) { image("b$n$it.png") }.take(if (n == 2) 3 else 5))); a.awaitReplies(n + 2) }
        assertEquals(18, link.streamed.sumOf { it.images.size })

        val three = List(3) { image("c$it.png") }
        stack.updateView(board) { it.copy(draft = "and these", images = three) }
        val gate = composerGate(true, stack.view(board).images, ImageSupport.YES, a::imageLimit)
        assertFalse(gate.canSend)
        assertEquals(ImageReasons.CONVERSATION_LIMIT, gate.sendReason)
        assertFalse(gate.canAttach)
        assertFalse(a.send("and these", three), "the send isn't made")
        assertEquals(4, link.streamed.size)
        assertEquals(three, stack.view(board).images, "the images stay pending")

        three.forEach { stack.removePending(board, it.image.id) }
        assertTrue(composerGate(true, stack.view(board).images, ImageSupport.YES, a::imageLimit).canSend, "the text can go")

        a.clear()
        assertTrue(composerGate(true, three, ImageSupport.YES, a::imageLimit).canSend, "a cleared assistant takes images again")
    }

    // ---- clear() and storage ----

    @Test fun clearDropsTheAssistantsImagesAndNothingIsWrittenToTheAppsStorage() = runBlocking {
        val filesDir = Files.createTempDirectory("files").toFile()
        // The app's storage: the bead chat store and its attachment directory live here.
        BeadChats(File(filesDir, "bead-chats.json"), CoroutineScope(Dispatchers.Unconfined), Dispatchers.Unconfined)
        val before = filesDir.walk().map { it.relativeTo(filesDir).path }.toSet()

        val session = Recorder()
        val a = assistant(session)
        val shot = image("shot.png", 2048)
        assertTrue(a.send("what is this?", listOf(shot)))
        a.awaitReply()
        assertEquals(listOf(shot), a.items.value.filterIsInstance<ChatItem.User>().single().images, "held in memory with the message")

        a.clear()
        assertTrue(a.items.value.isEmpty(), "the messages and their images are gone")
        assertEquals(1, session.resets)
        assertEquals(before, filesDir.walk().map { it.relativeTo(filesDir).path }.toSet(), "nothing was written to the app's storage")
        assertFalse(File(filesDir, "chat-attachments").listFiles().orEmpty().any(), "no attachment file was saved")
    }

    // ---- out-of-scope chats ----

    @Test fun outOfScopeChatsHaveNoAttachAction() = runBlocking {
        assertTrue(attachOffered(bead))
        assertTrue(attachOffered(board))
        assertFalse(attachOffered(ChatContext.Multi), "the assistant across boards (and the all-boards chat bar) offers no attach")

        // Across boards takes no images on either backend.
        assertEquals(ImageReasons.ACROSS_BOARDS, imageSupport(onMac = false, repoKey = null, hosts = emptyList()).reason)
        assertEquals(ImageReasons.ACROSS_BOARDS, imageSupport(onMac = true, repoKey = null, hosts = emptyList()).reason)
        val session = Recorder()
        val b = Backend(session)
        val across = BoardAssistant(ClaudeBackends(b, b), { AppSettings() }, Everywhere, scope)
        assertFalse(across.send("look", listOf(image("x.png"))), "the assistant across boards refuses images")
        assertTrue(session.inputs.isEmpty())

        // The new-bead draft chat and the all-boards chat bar have no attach action at all.
        val src = File("src/main/java/com/ryderdavid/nacre")
        val outOfScope = listOf("ui/board/NewBeadSheet.kt", "data/claude/AllBoardsRouter.kt").map { File(src, it) }
        outOfScope.forEach { f ->
            assertTrue(f.isFile, "$f exists")
            val text = f.readText()
            listOf("AttachImageButton", "rememberImageAttacher", "PickMultipleVisualMedia", "PendingImageRow").forEach {
                assertFalse(it in text, "${f.name} must not offer $it")
            }
        }
        // The assistant pane offers attach only where attachOffered says so.
        val pane = File(src, "ui/assistant/AssistantPane.kt").readText()
        assertTrue("attachOffered(chatContext)" in pane)
        assertTrue("if (attachable) AttachImageButton" in pane)
    }

    // ---- fakes ----

    private val macSettings = AppSettings(claudeRunner = AppSettings.RUNNER_MAC, hosts = listOf(Host("h1", "hid", "Studio", "http://studio", "t")))

    private fun host(features: List<String>) =
        HostState(Host("h1", "hid", "Studio", "http://studio", "t"), Connection.Online(Health(features = features)))

    private class Recorder : AssistantSession {
        val inputs = mutableListOf<TurnInput>()
        var resets = 0
        override suspend fun turn(input: TurnInput, sink: TurnSink): TurnResult {
            inputs += input
            sink.newBlock(); sink.text("ok")
            return TurnResult(emptyList(), TurnStop.DONE)
        }
        override fun reset() { resets++ }
    }

    private class Backend(val s: AssistantSession) : ClaudeBackend {
        override val runner = ClaudeRunner.API
        override suspend fun refine(repoKey: String, ctx: BeadContext, instruction: String, history: List<DraftTurn>, images: ChatTurnImages) = RefineProposal()
        override suspend fun ask(repoKey: String, ctx: BeadContext, history: List<DraftTurn>, question: String, images: ChatTurnImages) = ""
        override suspend fun draft(repoKey: String, repoName: String, rough: String, parentTitle: String?, existingTitles: List<String>) = BeadDraft("x")
        override suspend fun reviseDraft(repoKey: String, repoName: String, current: BeadDraft, history: List<DraftTurn>, message: String, parentTitle: String?, existingTitles: List<String>) = current
        override fun session(board: AssistantBoard) = s
    }

    private class Link : MacLink {
        val streamed = mutableListOf<AssistRequest>()
        override suspend fun claudeJson(key: String, req: AssistRequest): AssistResult = error("not used")
        override fun claudeStream(key: String, req: AssistRequest): Flow<AssistMessage> {
            streamed += req
            val output = buildJsonObject { put("reply", "seen"); putJsonArray("proposals") {} }
            return flowOf(AssistMessage.Turn("t${streamed.size}"), AssistMessage.Done(AssistResult(sessionId = "s1", output = output)))
        }
        override fun claudeStreamAcross(hostId: String, req: AssistRequest): Flow<AssistMessage> = error("not used")
        override fun claudeResume(key: String, turn: String, after: Long): Flow<AssistMessage> = error("not resumed")
        override suspend fun claudeCancel(key: String, turn: String) = Unit
        override fun hostName(hostId: String) = "Studio"
        override fun features(hostId: String): List<String>? = listOf(Features.CLAUDE, Features.CHAT_IMAGES)
    }

    private object Tools : BoardTools {
        override val boardKey = Keys.of("h1", "r1")
        override val boardName = "Nacre"
        override suspend fun listBeads(lane: String?, query: String?, limit: Int) = ""
        override suspend fun getBead(id: String) = ""
        override suspend fun updateBead(id: String, fields: JsonObject) = ""
        override suspend fun createBead(fields: JsonObject) = ""
        override suspend fun closeBead(id: String, reason: String?) = ""
        override suspend fun addComment(id: String, text: String) = ""
        override suspend fun addDependency(issueId: String, dependsOnId: String) = ""
        override suspend fun runInClaudeCode(id: String, instructions: String?) = ""
    }

    private object Everywhere : BoardRouter {
        override val only: BoardTools? = null
        override suspend fun resolve(board: String?) = Tools
        override suspend fun listBoards(query: String?) = "Nacre"
        override suspend fun listBeads(board: String?, lane: String?, query: String?, limit: Int) = ""
    }

    private fun assistant(session: AssistantSession): BoardAssistant {
        val b = Backend(session)
        return BoardAssistant(ClaudeBackends(b, b), { AppSettings() }, SingleBoard(Tools), scope)
    }

    private suspend fun BoardAssistant.awaitReply() = awaitReplies(1)

    private suspend fun BoardAssistant.awaitReplies(n: Int) = withTimeout(5_000) {
        busy.first { !it }
        items.first { list -> list.count { it is ChatItem.Assistant && !it.streaming } >= n }
        busy.first { !it }
    }
}
