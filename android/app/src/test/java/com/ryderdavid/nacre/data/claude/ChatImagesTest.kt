package com.ryderdavid.nacre.data.claude

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.runBlocking
import java.io.File
import java.io.IOException
import java.nio.file.Files
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class ChatImagesTest {
    private val root: File = Files.createTempDirectory("bead-chats").toFile().also { it.deleteOnExit() }
    private val file = File(root, "bead-chats.json")
    private val dir = File(root, "chat-attachments")
    private val key = BeadChats.key("mac~nacre", "NAC-1")

    private fun chats(attachments: ChatAttachments = ChatAttachments(dir)) =
        BeadChats(file, CoroutineScope(Dispatchers.Unconfined), Dispatchers.Unconfined, attachments = attachments)

    private fun payload(name: String, size: Int = 64, mime: String = "image/png") =
        ImagePayload(ChatImage.create(mime, name), ByteArray(size) { (it % 251).toByte() })

    private fun files() = dir.listFiles().orEmpty().map { it.name }.toSet()

    private fun sendWith(c: BeadChats, k: String, vararg images: ImagePayload): BeadChatItem.Mine {
        val msg = BeadChatItem.Mine("look", ChatMode.ASK, images = images.map { it.image })
        c.send(k, msg, retrying = false, images = images.toList()) { BeadChatItem.Answer("seen") }
        return msg
    }

    // --- Storage ---

    @Test fun referencesRoundTripWithNoBytesInTheJson() = runBlocking {
        val a = payload("bug.png", 4096)
        val b = payload("sketch.jpg", 2048, "image/jpeg")
        val msg = sendWith(chats(), key, a, b)
        assertEquals(setOf("${a.image.id}.png", "${b.image.id}.jpg"), files())
        assertTrue(a.bytes.contentEquals(File(dir, "${a.image.id}.png").readBytes()))

        val json = file.readText()
        assertTrue("\"mime_type\":\"image/png\"" in json && "\"file_name\":\"bug.png\"" in json, json)
        val base64 = java.util.Base64.getEncoder().encodeToString(a.bytes).take(40)
        assertFalse(base64 in json, "no base64 in bead-chats.json")
        assertTrue(json.length < 1000, "only references are stored: ${json.length} chars")

        val again = chats()
        assertEquals(msg, again.current(key)[0])
        assertTrue(a.bytes.contentEquals(again.image(a.image)!!))
        assertEquals(2, files().size, "the load sweep keeps referenced files")
    }

    @Test fun olderJsonLoadsAsTextOnly() {
        file.writeText(
            """{"mode":"ASK","threads":[{"key":"$key","items":[{"type":"mine","text":"Why?","mode":"ASK"},{"type":"answer","text":"Because."}],"updated":1}]}""",
        )
        val c = chats()
        assertEquals(listOf(BeadChatItem.Mine("Why?", ChatMode.ASK), BeadChatItem.Answer("Because.")), c.current(key))
        assertEquals(emptyList(), (c.current(key)[0] as BeadChatItem.Mine).images)
        assertEquals(listOf(DraftTurn(true, "(asking) Why?"), DraftTurn(false, "Because.")), c.current(key).turns())
    }

    @Test fun textOnlyThreadsAreWrittenAsBefore() {
        chats().send(key, BeadChatItem.Mine("Why?", ChatMode.ASK), retrying = false) { BeadChatItem.Answer("Because.") }
        assertFalse("images" in file.readText(), "no images field on a text-only message")
    }

    @Test fun clearDeletesTheThreadsFiles() {
        val c = chats()
        val other = BeadChats.key("mac~nacre", "NAC-2")
        sendWith(c, key, payload("a.png"), payload("b.png"))
        val kept = payload("c.png")
        sendWith(c, other, kept)
        assertEquals(3, files().size)
        c.clear(key)
        assertFalse(key in file.readText(), "the JSON no longer has the thread")
        assertEquals(setOf("${kept.image.id}.png"), files())
    }

    @Test fun the61stThreadEvictsTheOldestThreadAndItsFiles() {
        val c = chats()
        val oldest = payload("oldest.png")
        sendWith(c, BeadChats.key("r", "B-0"), oldest)
        val second = payload("second.png")
        sendWith(c, BeadChats.key("r", "B-1"), second)
        for (i in 2 until BeadChats.MAX_THREADS) c.send(BeadChats.key("r", "B-$i"), BeadChatItem.Mine("hi", ChatMode.ASK), false) { BeadChatItem.Answer("ok") }
        assertEquals(2, files().size, "60 threads fit")
        c.send(BeadChats.key("r", "B-60"), BeadChatItem.Mine("hi", ChatMode.ASK), false) { BeadChatItem.Answer("ok") }
        assertEquals(emptyList(), c.current(BeadChats.key("r", "B-0")))
        assertFalse("r/B-0\"" in file.readText())
        assertEquals(setOf("${second.image.id}.png"), files(), "only the evicted thread's file is gone")
    }

    @Test fun loadSweepsUnreferencedFiles() {
        val kept = payload("kept.png")
        sendWith(chats(), key, kept)
        File(dir, "0e0e0e0e-orphan.png").writeBytes(byteArrayOf(1, 2, 3))
        File(dir, "${kept.image.id}.png.tmp").writeBytes(byteArrayOf(1))
        chats()
        assertEquals(setOf("${kept.image.id}.png"), files())
    }

    @Test fun aFailedWriteLeavesNoFilesAndNoTurn() {
        var writes = 0
        val failing = ChatAttachments(dir) { f, b ->
            if (++writes == 2) { f.writeBytes(b.copyOf(3)); throw IOException("disk full") }
            f.writeBytes(b)
        }
        val c = chats(failing)
        val images = listOf(payload("a.png"), payload("b.png"), payload("c.png"))
        var asked = false
        val e = assertFailsWith<ChatAttachmentException> {
            c.send(key, BeadChatItem.Mine("look", ChatMode.ASK, images = images.map { it.image }), false, images) { asked = true; BeadChatItem.Answer("x") }
        }
        assertTrue("disk full" in e.message!!)
        assertEquals(emptySet(), files(), "the first image and the partial second are removed")
        assertEquals(emptyList(), c.current(key))
        assertFalse(asked, "nothing is sent")
        assertFalse(file.exists() && key in file.readText())
    }

    // --- Selection ---

    private fun turn(vararg images: ChatImage) = DraftTurn(true, "(asking) see", images.toList())

    @Test fun twentyImageCap() {
        val earlier = (1..25).map { payload("e$it.png", 1000) }
        val history = earlier.chunked(5).map { turn(*it.map { p -> p.image }.toTypedArray()) }
        val bytes = earlier.associate { it.image.id to it.bytes }
        val r = resolveImages(history, emptyList()) { bytes[it.id] }
        assertEquals(20, r.images.size)
        assertEquals(earlier.drop(5).map { it.image }, r.images.map { it.image }, "the newest 20, oldest first")
        earlier.take(5).forEach { assertEquals(ImageMark.Omitted, r.marks[it.image.id]) }
        assertEquals(ImageMark.Sent(1), r.marks[earlier[5].image.id])
    }

    @Test fun thirtyMegabytesEarlierPlusFiveNewThreeMegabyteImages() {
        val mb3 = ImageBudget.IMAGE_BYTES.toInt()
        val earlier = (1..10).map { payload("old$it.jpg", mb3, "image/jpeg") } // 30 MB
        val history = earlier.chunked(2).flatMap { listOf(turn(*it.map { p -> p.image }.toTypedArray()), DraftTurn(false, "ok")) }
        val current = (1..5).map { payload("new$it.jpg", mb3, "image/jpeg") }
        val bytes = (earlier + current).associate { it.image.id to it.bytes }
        val r = resolveImages(history, current.map { it.image }) { bytes[it.id] }
        assertEquals(current.map { it.image }, r.images.map { it.image }, "the five new images fill the 15 MB budget")
        assertTrue(r.images.sumOf { it.bytes.size.toLong() } <= ImageBudget.REQUEST_BYTES)
        assertEquals((1..5).map { ImageMark.Sent(it) }, current.map { r.marks[it.image.id] })
        earlier.forEach { assertEquals(ImageMark.Omitted, r.marks[it.image.id]) }
    }

    @Test fun byteBudgetKeepsTheNewestEarlierImagesThatFit() {
        val earlier = (1..8).map { payload("old$it.png", 2_000_000) } // 16 MB
        val history = earlier.map { turn(it.image) }
        val current = listOf(payload("new.png", 3_000_000))
        val bytes = (earlier + current).associate { it.image.id to it.bytes }
        val r = resolveImages(history, current.map { it.image }) { bytes[it.id] }
        // 3 MB new + six 2 MB earlier = 15 MB; the two oldest don't fit.
        assertEquals(earlier.drop(2).map { it.image } + current.map { it.image }, r.images.map { it.image })
        assertEquals(ImageBudget.REQUEST_BYTES, r.images.sumOf { it.bytes.size.toLong() })
        assertEquals(ImageMark.Omitted, r.marks[earlier[0].image.id])
        assertEquals(ImageMark.Omitted, r.marks[earlier[1].image.id])
    }

    @Test fun aThreadWithoutImagesResolvesToNothing() {
        val r = resolveImages(listOf(DraftTurn(true, "q"), DraftTurn(false, "a")), emptyList()) { error("nothing to read") }
        assertEquals(RequestImages.NONE, r)
    }

    // --- Markers ---

    private val ctx = BeadContext(
        "Nacre", "NAC-1", "Title", "task", 2, "open", listOf("ui"), "Desc", "Design", "", "", "NAC-0 Parent", listOf("NAC-2 Sib"), emptyList(),
        comments = listOf("ryder: looks close"),
    )

    @Test fun numberingAndMarkersAcrossTwoTurnsWithImages() {
        val shot = payload("shot.png")
        val photo1 = payload("photo1.jpg", mime = "image/jpeg")
        val photo2 = payload("photo2.jpg", mime = "image/jpeg")
        val history = listOf(turn(shot.image), DraftTurn(false, "A crash."))
        val bytes = listOf(shot, photo1, photo2).associate { it.image.id to it.bytes }
        val current = listOf(photo1.image, photo2.image)
        val r = resolveImages(history, current) { bytes[it.id] }
        assertEquals(listOf(shot, photo1, photo2), r.images)

        val ask = Prompts.ask(ctx, history, "And these?", readsRepo = true, current, r.marks)
        assertTrue("<ryder>(asking) see\n[image 1: shot.png]</ryder>" in ask, ask)
        assertTrue("<question>And these?\n[image 2: photo1.jpg] [image 3: photo2.jpg]</question>" in ask, ask)
        val refine = Prompts.refine(ctx, "Match them", history, current, r.marks)
        assertTrue("<ryder>(asking) see\n[image 1: shot.png]</ryder>" in refine)
        assertTrue("<request>Match them\n[image 2: photo1.jpg] [image 3: photo2.jpg]</request>" in refine, refine)
    }

    @Test fun omittedAndMissingImagesGetNotesAtTheirMarkers() {
        val big = (1..5).map { payload("big$it.png", 3_000_000) }
        val gone = payload("gone.png")
        val small = payload("small.png")
        val history = listOf(turn(small.image, gone.image), DraftTurn(false, "ok"))
        val current = big.map { it.image }
        val bytes = (big + small).associate { it.image.id to it.bytes } // gone.png has no file
        val r = resolveImages(history, current) { bytes[it.id] }
        assertEquals(big, r.images)
        val ask = Prompts.ask(ctx, history, "", readsRepo = false, current, r.marks)
        assertTrue("<ryder>(asking) see\n[image omitted: small.png] [image unavailable: gone.png]</ryder>" in ask, ask)
        assertTrue("<question>${(1..5).joinToString(" ") { "[image $it: big$it.png]" }}</question>" in ask, "an images-only message is just its markers")
    }

    @Test fun aMissingFileIsUnavailableThroughTheStore() {
        val c = chats()
        val p = payload("shot.png")
        sendWith(c, key, p)
        File(dir, "${p.image.id}.png").delete()
        val r = c.requestImages(c.current(key).turns(), emptyList())
        assertEquals(emptyList(), r.images)
        assertEquals(ImageMark.Unavailable, r.marks[p.image.id])
    }

    @Test fun aTextOnlyThreadsPromptEqualsTodays() {
        val history = listOf(
            BeadChatItem.Mine("Why --dir?", ChatMode.ASK),
            BeadChatItem.Answer("So reads name their board."),
            BeadChatItem.Mine("Tighten it", ChatMode.REFINE, about = "On the allowlist"),
            BeadChatItem.Proposal(RefineProposal(summary = "Shorter"), BeadChatItem.Outcome.APPLIED, applied = 1),
        ).turns()
        val marks = resolveImages(history, emptyList()) { null }.marks
        for (readsRepo in listOf(true, false)) {
            assertEquals(Today.ask(ctx, history, "And the allowlist?", readsRepo), Prompts.ask(ctx, history, "And the allowlist?", readsRepo, emptyList(), marks))
        }
        assertEquals(Today.refine(ctx, "Make it agent-ready", history), Prompts.refine(ctx, "Make it agent-ready", history, emptyList(), marks))
        assertEquals(Today.refine(ctx, "", emptyList()), Prompts.refine(ctx, ""))
    }

    /** Prompts.ask and Prompts.refine exactly as they were before images, frozen. */
    private object Today {
        fun refine(ctx: BeadContext, instruction: String, history: List<DraftTurn>): String = buildString {
            appendLine("Improve this bead from the \"${ctx.repoName}\" beads (bd) issue tracker according to the request below.")
            appendLine()
            conversation(history, "Earlier in this conversation about the bead (questions and answers may bear on the request):")
            appendLine("<request>${instruction.ifBlank { "Make this bead clear and ready for an agent to pick up: a crisp title, a description that says why and what, and testable acceptance criteria." }}</request>")
            appendLine()
            appendLine(ctx.render())
            appendLine()
            appendLine("Return only fields you are changing; use null for everything you leave as is. Keep the author's facts, ids, paths and decisions; do not invent requirements. Write in plain, direct markdown. Propose children only when the request asks to split or break down the work. In `summary`, say in one or two sentences what you changed and why.")
        }

        fun ask(ctx: BeadContext, history: List<DraftTurn>, question: String, readsRepo: Boolean): String = buildString {
            appendLine("Ryder is asking about this bead from the \"${ctx.repoName}\" beads (bd) issue tracker.")
            appendLine()
            appendLine(ctx.render())
            appendLine()
            conversation(history, "Earlier in this conversation:")
            appendLine("<question>$question</question>")
            appendLine()
            if (readsRepo) appendLine("You are in the repository the bead belongs to and may read its code, git history and other beads (read-only bd commands) when the question needs them. Look before you answer a question about the code; don't guess.")
            appendLine("Answer the question in `answer`: short, scannable markdown for a phone screen, with ids, paths and names as they appear. Don't propose edits to the bead unless asked what you would change; Ryder switches to Refine mode to make changes.")
        }

        private fun StringBuilder.conversation(history: List<DraftTurn>, intro: String) {
            if (history.isEmpty()) return
            appendLine(intro)
            appendLine("<conversation>")
            history.takeLast(24).forEach { appendLine(if (it.fromUser) "<ryder>${it.text}</ryder>" else "<you>${it.text}</you>") }
            appendLine("</conversation>")
            appendLine()
        }
    }
}
