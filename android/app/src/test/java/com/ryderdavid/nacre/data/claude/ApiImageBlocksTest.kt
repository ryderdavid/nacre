package com.ryderdavid.nacre.data.claude

import com.anthropic.core.JsonValue
import com.anthropic.core.http.Headers
import com.anthropic.core.jsonMapper
import com.anthropic.errors.UnexpectedStatusCodeException
import com.anthropic.models.messages.JsonOutputFormat
import com.anthropic.models.messages.MessageCreateParams
import com.anthropic.models.messages.MessageParam
import com.anthropic.models.messages.OutputConfig
import com.anthropic.models.messages.ThinkingConfigAdaptive
import com.anthropic.models.messages.ToolUnion
import com.ryderdavid.nacre.data.settings.AppSettings
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.util.Base64
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

/** How the API backend puts images into the requests it sends. */
class ApiImageBlocksTest {
    private val settings = AppSettings(claudeModel = "claude-opus-5-5")

    private val ctx = BeadContext(
        "Nacre", "NAC-1", "Title", "task", 2, "open", listOf("ui"), "Desc", "Design", "", "", "NAC-0 Parent", listOf("NAC-2 Sib"), emptyList(),
        comments = listOf("ryder: looks close"),
    )

    private fun payload(name: String, size: Int = 64, mime: String = "image/png") =
        ImagePayload(ChatImage.create(mime, name), ByteArray(size) { (it % 251).toByte() })

    private fun body(p: MessageCreateParams): JsonObject =
        Json.parseToJsonElement(jsonMapper().writeValueAsString(p._body())).jsonObject

    private fun JsonObject.messages() = this["messages"]!!.jsonArray.map { it.jsonObject }

    private fun JsonElement.type() = jsonObject["type"]!!.jsonPrimitive.content
    private fun JsonElement.text() = jsonObject["text"]!!.jsonPrimitive.content

    /** Asserts [block] is [p]'s image as a base64 block with its media type. */
    private fun assertImage(p: ImagePayload, block: JsonElement) {
        assertEquals("image", block.type())
        val source = block.jsonObject["source"]!!.jsonObject
        assertEquals("base64", source["type"]!!.jsonPrimitive.content)
        assertEquals(p.image.mimeType, source["media_type"]!!.jsonPrimitive.content)
        assertEquals(Base64.getEncoder().encodeToString(p.bytes), source["data"]!!.jsonPrimitive.content)
    }

    // --- Bead chat ---

    @Test fun anImageTurnSendsLabelledImageBlocksBeforeThePrompt() {
        val shot = payload("shot.png")
        val photo = payload("photo.jpg", mime = "image/jpeg")
        val current = listOf(shot.image, photo.image)
        val bytes = listOf(shot, photo).associate { it.image.id to it.bytes }
        val images = ChatTurnImages(current, resolveImages(emptyList(), current) { bytes[it.id] })

        val params = ClaudeService.askParams(settings, ctx, emptyList(), "what's wrong here?", images)
        val messages = body(params).messages()
        assertEquals(1, messages.size)
        assertEquals("user", messages[0]["role"]!!.jsonPrimitive.content)
        val blocks = messages[0]["content"]!!.jsonArray
        assertEquals(listOf("text", "image", "text", "image", "text"), blocks.map { it.type() })
        assertEquals("Image 1: shot.png", blocks[0].text())
        assertImage(shot, blocks[1])
        assertEquals("Image 2: photo.jpg", blocks[2].text())
        assertImage(photo, blocks[3])
        val prompt = Prompts.ask(ctx, emptyList(), "what's wrong here?", readsRepo = false, current, images.request.marks)
        assertEquals(prompt, blocks[4].text())
        assertTrue("<question>what's wrong here?\n[image 1: shot.png] [image 2: photo.jpg]</question>" in prompt, prompt)

        // Refine is shaped the same way, around its own prompt.
        val refine = body(ClaudeService.refineParams(settings, ctx, "Match it", emptyList(), images)).messages().single()["content"]!!.jsonArray
        assertEquals(listOf("text", "image", "text", "image", "text"), refine.map { it.type() })
        assertEquals(Prompts.refine(ctx, "Match it", emptyList(), current, images.request.marks), refine[4].text())
    }

    @Test fun earlierImagesAreLabelledInMarkerOrderAndOmittedOnesNoted() {
        val old = payload("old.png", 2_000_000)
        val earlier = payload("earlier.png", 1_000_000)
        val gone = payload("gone.png")
        val big = (1..5).map { payload("big$it.jpg", 2_800_000, "image/jpeg") }
        val history = listOf(
            DraftTurn(true, "(asking) first", listOf(old.image)),
            DraftTurn(false, "Seen."),
            DraftTurn(true, "(asking) then", listOf(earlier.image, gone.image)),
            DraftTurn(false, "Also seen."),
        )
        val current = big.map { it.image }
        val bytes = (listOf(old, earlier) + big).associate { it.image.id to it.bytes } // gone.png has no file
        val r = resolveImages(history, current) { bytes[it.id] }
        // 5 × 2.8 MB + 1 MB fits 15 MB; old.png's 2 MB doesn't.
        assertEquals(listOf(earlier) + big, r.images)

        val params = ClaudeService.askParams(settings, ctx, history, "And now?", ChatTurnImages(current, r))
        val blocks = body(params).messages().single()["content"]!!.jsonArray
        val labels = blocks.filter { it.type() == "text" }.dropLast(1).map { it.text() }
        assertEquals(listOf("Image 1: earlier.png") + (1..5).map { "Image ${it + 1}: big$it.jpg" }, labels)
        (listOf(earlier) + big).forEachIndexed { i, p -> assertImage(p, blocks[2 * i + 1]) }

        val prompt = blocks.last().text()
        // Each label matches the marker at its turn, in the same order.
        val markers = Regex("""\[image (\d+): ([^\]]+)]""").findAll(prompt).map { "Image ${it.groupValues[1]}: ${it.groupValues[2]}" }.toList()
        assertEquals(labels, markers)
        assertTrue("<ryder>(asking) first\n[image omitted: old.png]</ryder>" in prompt, prompt)
        assertTrue("<ryder>(asking) then\n[image 1: earlier.png] [image unavailable: gone.png]</ryder>" in prompt, prompt)
        assertTrue("<question>And now?\n${(1..5).joinToString(" ") { "[image ${it + 1}: big$it.jpg]" }}</question>" in prompt, prompt)
    }

    @Test fun notesWithoutAnySentImageKeepATextMessage() {
        val gone = payload("gone.png")
        val history = listOf(DraftTurn(true, "(asking) see", listOf(gone.image)), DraftTurn(false, "ok"))
        val r = resolveImages(history, emptyList()) { null }
        val user = body(ClaudeService.askParams(settings, ctx, history, "Still there?", ChatTurnImages(emptyList(), r))).messages().single()
        val prompt = user["content"]!!.jsonPrimitive.content
        assertTrue("[image unavailable: gone.png]" in prompt, prompt)
    }

    @Test fun aTextOnlyTurnBuildsTodaysParams() {
        val history = listOf(
            BeadChatItem.Mine("Why --dir?", ChatMode.ASK),
            BeadChatItem.Answer("So reads name their board."),
        ).turns()
        val r = resolveImages(history, emptyList()) { error("nothing to read") }
        for (images in listOf(ChatTurnImages.NONE, ChatTurnImages(emptyList(), r))) {
            assertEquals(
                body(today(Prompts.ADVISOR, Prompts.ask(ctx, history, "And the allowlist?", readsRepo = false), ClaudeService.askSchema, 8000)),
                body(ClaudeService.askParams(settings, ctx, history, "And the allowlist?", images)),
            )
            assertEquals(
                body(today(Prompts.EDITOR, Prompts.refine(ctx, "Tighten it", history), ClaudeService.refineSchema, 16000)),
                body(ClaudeService.refineParams(settings, ctx, "Tighten it", history, images)),
            )
        }
        val user = body(ClaudeService.askParams(settings, ctx, history, "q", ChatTurnImages.NONE)).messages().single()
        assertTrue(user["content"]!!.jsonPrimitive.isString, "a text-only message is a plain string")
    }

    /** ClaudeService.structured's request exactly as it was built before images, frozen. */
    private fun today(system: String, prompt: String, schema: Map<String, Any>, maxTokens: Long): MessageCreateParams =
        ClaudeService.baseParams(settings, system, maxTokens)
            .outputConfig(
                OutputConfig.builder()
                    .effort(ClaudeService.effortOf(settings.claudeEffort))
                    .format(JsonOutputFormat.builder().schema(ClaudeService.schemaOf(schema)).build())
                    .build()
            )
            .addUserMessage(prompt)
            .build()

    // --- Board assistant ---

    private val board = AssistantBoard("mac~nacre", "Nacre") { "system" }

    private fun session() = ApiSession(ClaudeService(), { settings }, board)

    private fun assistant(text: String) = MessageParam.builder().role(MessageParam.Role.ASSISTANT).content(text).build()

    @Test fun assistantImagesStayInTheirOwnMessagesBeforeTheirText() {
        val shot = payload("shot.png")
        val photo1 = payload("photo1.jpg", mime = "image/jpeg")
        val photo2 = payload("photo2.jpg", mime = "image/jpeg")
        val s = session()
        s.record(TurnInput.Say("What's this?", listOf(shot)))
        s.record(TurnInput.Answers(emptyList()))
        s.record(TurnInput.Say("No images here"))
        s.record(TurnInput.Say("And these two?", listOf(photo1, photo2)))
        s.record(TurnInput.Say("", listOf(photo1.copyAs("again.jpg"))))

        val messages = body(s.params(settings, "system")).messages()
        val first = messages[0]["content"]!!.jsonArray
        assertEquals(listOf("image", "text"), first.map { it.type() })
        assertImage(shot, first[0])
        assertEquals("What's this?", first[1].text())
        assertEquals("No images here", messages[2]["content"]!!.jsonPrimitive.content, "a text-only message is as today")
        val third = messages[3]["content"]!!.jsonArray
        assertEquals(listOf("image", "image", "text"), third.map { it.type() })
        assertImage(photo1, third[0])
        assertImage(photo2, third[1])
        assertEquals("And these two?", third[2].text())
        val imagesOnly = messages[4]["content"]!!.jsonArray
        assertEquals(listOf("image"), imagesOnly.map { it.type() }, "an images-only message has no empty text block")
    }

    @Test fun assistantHistoryKeepsTheBudgetNewestFirstWithNotesForTheRest() {
        val s = session()
        val early = (1..4).map { payload("early$it.png", 2_500_000) }
        val late = (1..5).map { payload("late$it.jpg", 2_500_000, "image/jpeg") }
        s.record(TurnInput.Say("first", early))
        s.record(TurnInput.Answers(emptyList()))
        s.record(TurnInput.Say("second", late))

        val messages = body(s.params(settings, "system")).messages()
        // 15 MB holds six 2.5 MB images: all five new ones and the newest that fits of the earlier.
        val first = messages[0]["content"]!!.jsonArray
        assertEquals(listOf("image", "text"), first.map { it.type() })
        assertImage(early[0], first[0])
        assertEquals("first\n[image omitted: early2.png] [image omitted: early3.png] [image omitted: early4.png]", first[1].text())
        val second = messages[2]["content"]!!.jsonArray
        assertEquals(List(5) { "image" } + "text", second.map { it.type() })
        late.forEachIndexed { i, p -> assertImage(p, second[i]) }
        assertEquals("second", second[5].text())
    }

    @Test fun aTextOnlyAssistantConversationBuildsTodaysParams() {
        val s = session()
        s.record(TurnInput.Say("hello"))
        val tools = board.tools.map { ToolUnion.ofTool(it.toTool()) }
        val today = ClaudeService.baseParams(settings, "system", 64000)
            .thinking(ThinkingConfigAdaptive.builder().display(ThinkingConfigAdaptive.Display.SUMMARIZED).build())
            .outputConfig(OutputConfig.builder().effort(ClaudeService.effortOf(settings.claudeEffort)).build())
            .tools(tools)
            .messages(listOf(MessageParam.builder().role(MessageParam.Role.USER).content("hello").build()))
            .build()
        assertEquals(body(today), body(s.params(settings, "system")))
    }

    // --- Errors ---

    @Test fun aRequestTooLargeErrorSurfacesItsReason() {
        val e = UnexpectedStatusCodeException.builder()
            .statusCode(413)
            .headers(Headers.builder().build())
            .body(JsonValue.from(mapOf("type" to "error", "error" to mapOf("type" to "request_too_large", "message" to "Request exceeds the maximum size"))))
            .build()
        val thrown = assertFailsWith<ClaudeException> { ClaudeService.call<Unit> { throw e } }
        assertEquals("The request was too large for the Claude API: Request exceeds the maximum size", thrown.message)
    }

    private fun ImagePayload.copyAs(name: String) = ImagePayload(ChatImage.create(image.mimeType, name), bytes)
}
