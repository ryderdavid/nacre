package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.data.bridge.BridgeClient
import com.ryderdavid.nacre.data.bridge.BridgeException
import com.ryderdavid.nacre.data.bridge.ChatStreamMessage
import com.ryderdavid.nacre.data.claude.ChatImage
import com.ryderdavid.nacre.data.claude.ChatStack
import com.ryderdavid.nacre.data.claude.ComposerReasons
import com.ryderdavid.nacre.data.claude.ImageBudget
import com.ryderdavid.nacre.data.claude.ImagePayload
import com.ryderdavid.nacre.data.claude.ImagePrep
import com.ryderdavid.nacre.data.claude.SkipReasons
import com.ryderdavid.nacre.data.claude.plusPicked
import com.ryderdavid.nacre.data.model.ChatEvent
import com.ryderdavid.nacre.data.model.ChatImageRef
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.data.model.ChatWire
import com.ryderdavid.nacre.data.model.ScreenContext
import com.ryderdavid.nacre.data.model.SendChatMessage
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.encodeToString
import java.util.Base64
import kotlin.test.Test
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertIs
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** 9.6: images in Chat — attaching, pending images, sending, the per-chat limit and thumbnails. */
class ChatImagesTest {
    private val key = "a~c1"
    private val idle = ChatRecord("c1", state = ChatWire.IDLE)
    private val bead = ScreenContext(ScreenContext.BEAD, repo = "r1", beadId = "NAC-7")

    private fun image(name: String, size: Int = 1_000, mime: String = "image/png") =
        ImagePayload(ChatImage.create(mime, name), ByteArray(size) { (it % 251).toByte() })

    /** A conversation on [link] whose draft and pending images live in [stack]. */
    private fun conversation(link: FakeChatLink, scope: CoroutineScope, stack: ChatStack, chat: ChatRecord = idle): ChatConversation {
        link.detail = { BridgeClient.ChatDetail(chat) }
        val ids = generateSequence(1) { it + 1 }.map { "t_$it" }.iterator()
        return ChatConversation(key, link, scope, stack = stack, newTurnId = { ids.next() }, backoff = ::quickBackoff)
    }

    /** nacred's stream after a send: the turn ends and the chat settles as [after]. */
    private fun FakeChatLink.settlesAs(after: ChatRecord) {
        streams += { _, _ -> stream(ChatStreamMessage.State(after)) }
    }

    // ---- 9.2: preparing picked images ----

    @Test fun `picked images are downscaled to a long edge of 1568 px and kept within 3 MB`() {
        assertEquals(1568 to 1176, ImagePrep.fit(4000, 3000))
        assertEquals(882 to 1568, ImagePrep.fit(2250, 4000))
        assertEquals(1200 to 800, ImagePrep.fit(1200, 800))
        // A screenshot already within both limits is sent as it is.
        assertTrue(ImagePrep.keepsOriginal("image/png", 1080, 1568, 2_000_000))
        assertFalse(ImagePrep.keepsOriginal("image/png", 1080, 2400, 2_000_000))
        assertFalse(ImagePrep.keepsOriginal("image/jpeg", 1000, 1000, 3_000_001))
        // Too large as PNG, so it is re-encoded as JPEG at falling quality until it fits 3 MB.
        val tried = mutableListOf<Pair<String, Int>>()
        val (type, data) = ImagePrep.shrink("image/png") { mime, q ->
            tried += mime to q
            ByteArray(if (mime == "image/jpeg" && q <= 70) 2_900_000 else 3_500_000)
        }!!
        assertEquals("image/jpeg", type)
        assertTrue(data.size <= ImageBudget.IMAGE_BYTES)
        assertEquals(listOf("image/png" to 90, "image/jpeg" to 90, "image/jpeg" to 80, "image/jpeg" to 70), tried)
        assertEquals("shot.jpg", ImagePrep.rename("shot.png", type))
        // Nothing fits: the file is skipped as oversized.
        assertNull(ImagePrep.shrink("image/png") { _, _ -> ByteArray(4_000_000) })
    }

    @Test fun `an unsupported file is skipped and the snackbar names it`() {
        val pdf = "%PDF-1.7".toByteArray()
        assertNull(ImagePrep.sniff(pdf))
        assertEquals("image/png", ImagePrep.sniff(byteArrayOf(0x89.toByte(), 'P'.code.toByte(), 'N'.code.toByte(), 'G'.code.toByte(), 13, 10, 26, 10)))
        assertTrue("notes.pdf" in SkipReasons.unsupported("notes.pdf"))
        assertTrue("big.png" in SkipReasons.tooLarge("big.png"))
        assertTrue("broken.jpg" in SkipReasons.unreadable("broken.jpg"))
    }

    // ---- 9.3: pending images in the chat's view state ----

    @Test fun `pending images survive collapse and moves between the pane, the sheet and the tab`() {
        val stack = ChatStack()
        val two = listOf(image("a.png"), image("b.png"))
        stack.open(key)
        stack.updateView(key) { it.copy(draft = "what's wrong here?", images = it.images.plusPicked(two)) }
        stack.collapse() // the docked pane leaves its edge tab
        stack.expand()
        stack.followedLink(wide = true) // the pane stays docked beside the next screen
        stack.followedLink(wide = false) // a link out of the phone sheet closes it
        stack.select("b~other") // another chat in the Chat tab
        stack.select(key) // back to this one in the tab
        assertTrue(stack.openCurrent()) // ✨ reopens it in the pane or sheet
        val view = stack.view(key)
        assertEquals("what's wrong here?", view.draft)
        assertEquals(two, view.images)
        assertTrue(stack.view("b~other").images.isEmpty())
    }

    @Test fun `the 5-image limit disables Attach image until one is removed`() {
        val four = (1..4).map { image("s$it.png") }
        val open = chatComposerGate("", four, idle, canSendNow = true)
        assertTrue(open.canAttach)
        assertEquals(1, open.remaining)
        val full = chatComposerGate("", four + image("s5.png"), idle, canSendNow = true)
        assertFalse(full.canAttach)
        assertEquals(0, full.remaining)
        assertEquals(ComposerReasons.FULL, full.attachReason)
        assertTrue(full.canSend)
        // Picking more than fit keeps only the first five.
        assertEquals(5, four.plusPicked(listOf(image("x.png"), image("y.png"))).size)
        val stack = ChatStack()
        stack.updateView(key) { it.copy(images = four + image("s5.png")) }
        stack.removePending(key, four[0].image.id)
        assertTrue(chatComposerGate("", stack.view(key).images, idle, canSendNow = true).canAttach)
    }

    @Test fun `a removed pending image isn't sent`() = runBlocking {
        val link = FakeChatLink()
        val stack = ChatStack()
        val three = listOf(image("one.png"), image("two.png"), image("three.png"))
        stack.updateView(key) { it.copy(draft = "compare these", images = three) }
        stack.removePending(key, three[1].image.id)
        link.settlesAs(idle.copy(imageCount = 2))
        val conv = conversation(link, this, stack)
        conv.load()
        assertEquals("t_1", conv.sendDraftNow(bead))
        val body = link.sent.single()
        assertEquals("compare these", body.text)
        assertEquals(listOf("one.png", "three.png"), body.images.map { it.name })
        assertContentEquals(three[2].bytes, Base64.getDecoder().decode(body.images[1].data))
        // Accepted: the draft and the sent images are cleared.
        assertEquals("", stack.view(key).draft)
        assertTrue(stack.view(key).images.isEmpty())
    }

    @Test fun `an images-only message is sent with no text`() = runBlocking {
        val shot = image("screenshot.png", mime = "image/png")
        assertFalse(chatComposerGate("", emptyList(), idle, canSendNow = true).canSend)
        assertTrue(chatComposerGate("   ", listOf(shot), idle, canSendNow = true).canSend)
        assertFalse(chatComposerGate("", listOf(shot), idle, canSendNow = false).canSend)

        val link = FakeChatLink()
        val stack = ChatStack()
        stack.updateView(key) { it.copy(images = listOf(shot)) }
        link.settlesAs(idle.copy(imageCount = 1, imageBytes = 1_000))
        val conv = conversation(link, this, stack)
        conv.load()
        assertEquals("t_1", conv.sendDraftNow(bead))
        val body = link.sent.single()
        assertEquals("", body.text)
        assertEquals("image/png", body.images.single().mediaType)
        assertEquals("screenshot.png", body.images.single().name)
        assertContentEquals(shot.bytes, Base64.getDecoder().decode(body.images.single().data))
        assertEquals(bead, body.context)
        assertTrue(stack.view(key).images.isEmpty())
    }

    @Test fun `images go on the wire as an ordered media_type, name and data array, left out when there are none`() {
        val json = BridgeClient.json
        val withImages = json.encodeToString(SendChatMessage("t_1", "", null, listOf(com.ryderdavid.nacre.data.model.AssistImage("image/png", "a.png", "AAA="))))
        assertTrue(""""images":[{"media_type":"image/png","name":"a.png","data":"AAA="}]""" in withImages, withImages)
        assertFalse("images" in json.encodeToString(SendChatMessage("t_1", "hi", null)))
    }

    // ---- 9.4: the chat's image budget ----

    @Test fun `the per-chat limit keeps images pending and blocks Send with the reason`() = runBlocking {
        val sent18 = idle.copy(imageCount = 18, imageBytes = 9_000_000)
        val three = listOf(image("a.png"), image("b.png"), image("c.png"))
        val gate = chatComposerGate("and these?", three, sent18, canSendNow = true)
        assertFalse(gate.canSend)
        assertEquals(ChatImageLimits.REASON, gate.sendReason)
        assertEquals("This chat has reached its image limit. Start a new chat to send more images.", gate.sendReason)
        // Two more fit, a third doesn't; the bytes count too.
        assertNull(chatImageLimit(sent18, 2, 2_000))
        assertEquals(ChatImageLimits.REASON, chatImageLimit(idle.copy(imageCount = 3, imageBytes = 14_000_000), 1, 1_000_001))
        assertNull(chatImageLimit(idle.copy(imageCount = 3, imageBytes = 14_000_000), 1, 1_000_000))

        val link = FakeChatLink()
        val stack = ChatStack()
        stack.updateView(key) { it.copy(draft = "and these?", images = three) }
        val conv = conversation(link, this, stack, chat = sent18)
        conv.load()
        assertNull(conv.sendDraftNow(bead))
        assertTrue(link.sent.isEmpty())
        assertEquals(three, stack.view(key).images)
        assertEquals("and these?", stack.view(key).draft)

        // Removing the images lets the text be sent.
        three.forEach { stack.removePending(key, it.image.id) }
        assertTrue(chatComposerGate(stack.view(key).draft, stack.view(key).images, sent18, canSendNow = true).canSend)
        link.settlesAs(sent18)
        assertEquals("t_1", conv.sendDraftNow(bead))
        assertEquals("and these?", link.sent.single().text)
        assertTrue(link.sent.single().images.isEmpty())
    }

    @Test fun `a sent message counts toward the chat's images`() = runBlocking {
        val link = FakeChatLink()
        val stack = ChatStack()
        val conv = conversation(link, this, stack)
        conv.load()
        stack.updateView(key) { it.copy(images = (1..5).map { n -> image("p$n.png", size = 2_000) }) }
        // nacred's record after the turn carries the chat's own count.
        link.settlesAs(idle.copy(imageCount = 5, imageBytes = 10_000))
        conv.sendDraftNow(bead)
        assertEquals(5, conv.state.value.chat?.imageCount)
        assertEquals(10_000, conv.state.value.chat?.imageBytes)
    }

    @Test fun `nacred's 400, 409 and 413 reasons are shown and the draft and images are kept`() = runBlocking {
        val refusals = listOf(
            BridgeException("image 2 (shot.png): bytes are not a png image", code = 400),
            BridgeException("too many images: 6, at most 5", code = 400),
            BridgeException(ChatImageLimits.REASON, code = 409),
            BridgeException("request body too large", code = 413),
        )
        for (refusal in refusals) {
            val link = FakeChatLink()
            val stack = ChatStack()
            val two = listOf(image("one.png"), image("shot.png"))
            stack.updateView(key) { it.copy(draft = "look", images = two) }
            val conv = conversation(link, this, stack)
            conv.load()
            link.refuse = refusal
            assertNull(conv.sendDraftNow(bead))
            assertEquals(1, link.sent.size, "a refusal is not resent")
            val s = conv.state.value
            assertEquals(refusal.message, s.error)
            assertFalse(s.sending)
            assertEquals("look", stack.view(key).draft)
            assertEquals(two, stack.view(key).images)
        }
    }

    // ---- 9.5: thumbnails of sent messages ----

    @Test fun `a sent message shows its images, and one that can't be fetched shows Image unavailable`() = runBlocking {
        val t = ChatTranscript()
        val refs = listOf(ChatImageRef("im_aaaaaaaaaaaaaaaaaaaaaaaa.png", "a.png", "image/png", 3), ChatImageRef("im_bbbbbbbbbbbbbbbbbbbbbbbb.jpg", "b.jpg", "image/jpeg", 3))
        t.add(ChatEvent(seq = 1, kind = ChatWire.USER, text = "", context = bead, images = refs))
        val user = assertIs<ChatItem.User>(t.shown.single())
        assertEquals(refs, user.images)

        val link = FakeChatLink()
        link.images[refs[0].id] = byteArrayOf(1, 2, 3)
        val thumbs = ChatThumbs(link)
        assertIs<ThumbState.Shown>(loadThumb { thumbs.bytes(key, refs[0]) })
        assertEquals(ThumbState.Unavailable, loadThumb { thumbs.bytes(key, refs[1]) })
        assertEquals(ThumbState.Unavailable, loadThumb { error("connection reset") })
        assertEquals(ThumbState.Unavailable, loadThumb { ByteArray(0) })
        assertEquals("Image unavailable", IMAGE_UNAVAILABLE)
        // A fetched image is kept; a missing one is tried again next time.
        thumbs.bytes(key, refs[0])
        thumbs.bytes(key, refs[1])
        assertEquals(3, link.imageFetches)
    }

    @Test fun `user events read their images from nacred's log`() {
        val detail = BridgeClient.json.decodeFromString<BridgeClient.ChatDetail>(
            """{"chat":{"id":"c1","image_count":1,"image_bytes":2048},"events":[{"seq":1,"kind":"user","text":"","images":[{"id":"im_0123456789abcdef01234567.png","name":"shot.png","media_type":"image/png","bytes":2048}]}]}""",
        )
        assertEquals(1, detail.chat.imageCount)
        assertEquals(2048, detail.chat.imageBytes)
        assertEquals(ChatImageRef("im_0123456789abcdef01234567.png", "shot.png", "image/png", 2048), detail.events.single().images.single())
    }
}
