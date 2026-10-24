package com.ryderdavid.nacre.data.claude

import com.anthropic.core.jsonMapper
import com.anthropic.models.messages.JsonOutputFormat
import com.anthropic.models.messages.OutputConfig
import com.anthropic.models.messages.ToolUnion
import com.ryderdavid.nacre.data.settings.AppSettings
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/** Checks the request bodies we send match what strict tools and structured outputs require. */
class RequestShapeTest {
    private val settings = AppSettings(claudeModel = "claude-opus-5-5")

    private fun body(b: com.anthropic.models.messages.MessageCreateParams): JsonObject =
        Json.parseToJsonElement(jsonMapper().writeValueAsString(b._body())).jsonObject

    @Test fun strictToolsListEveryPropertyAsRequired() {
        val params = ClaudeService.baseParams(settings, "system", 64000)
            .tools(BoardAssistant.TOOL_DEFS.map { ToolUnion.ofTool(it) })
            .addUserMessage("hi")
            .build()
        val tools = body(params)["tools"]!!.jsonArray
        assertEquals(8, tools.size)
        for (t in tools) {
            val o = t.jsonObject
            assertTrue(o["strict"]!!.jsonPrimitive.boolean, "${o["name"]} strict")
            assertTrue(o["eager_input_streaming"]!!.jsonPrimitive.boolean)
            val schema = o["input_schema"]!!.jsonObject
            assertEquals("false", schema["additionalProperties"].toString())
            val props = schema["properties"]!!.jsonObject.keys
            val required = schema["required"]!!.jsonArray.map { it.jsonPrimitive.content }.toSet()
            assertEquals(props, required, "${o["name"]} requires all properties")
        }
        val raw = body(params)
        assertEquals("\"default\"", raw["fallbacks"].toString())
        assertTrue(params._headers().values("anthropic-beta").contains("server-side-fallback-2026-07-01"))
    }

    @Test fun refineSchemaIsStrictObject() {
        val params = ClaudeService.baseParams(settings, "system", 16000)
            .outputConfig(OutputConfig.builder().format(JsonOutputFormat.builder().schema(ClaudeService.schemaOf(ClaudeService.refineSchema)).build()).build())
            .addUserMessage("hi")
            .build()
        val schema = body(params)["output_config"]!!.jsonObject["format"]!!.jsonObject["schema"]!!.jsonObject
        assertEquals((schema["properties"] as JsonObject).keys, (schema["required"] as JsonArray).map { it.jsonPrimitive.content }.toSet())
        println(Json { prettyPrint = true }.encodeToString(JsonObject.serializer(), body(params)).take(1500))
    }

    @Test fun draftSchemaIsStrictObject() {
        val params = ClaudeService.baseParams(settings, "system", 8000)
            .outputConfig(OutputConfig.builder().format(JsonOutputFormat.builder().schema(ClaudeService.schemaOf(ClaudeService.draftSchema)).build()).build())
            .addUserMessage("hi")
            .build()
        val schema = body(params)["output_config"]!!.jsonObject["format"]!!.jsonObject["schema"]!!.jsonObject
        val props = (schema["properties"] as JsonObject).keys
        assertEquals(props, (schema["required"] as JsonArray).map { it.jsonPrimitive.content }.toSet())
        assertTrue("open_questions" in props && "reply" in props)
    }

    @Test fun revisePromptCarriesDraftQuestionsHistoryAndMessage() {
        val current = BeadDraft(
            title = "Add a chat box", description = "Steer the draft", issue_type = "feature", priority = 3,
            labels = listOf("ux"), open_questions = listOf("Should it sit at the bottom?"),
        )
        val prompt = ClaudeService.revisePrompt(
            "Nacre", current, listOf(DraftTurn(true, "Make it P1"), DraftTurn(false, "Raised priority to P1.")),
            "Yes, at the bottom. Also change the title to Steer drafts by chat.", parentTitle = null, existingTitles = emptyList(),
        )
        assertTrue("<title>Add a chat box</title>" in prompt)
        assertTrue("priority=\"P3\"" in prompt)
        assertTrue("- Should it sit at the bottom?" in prompt)
        assertTrue("<owner>Make it P1</owner>" in prompt && "<you>Raised priority to P1.</you>" in prompt)
        assertTrue("<message>Yes, at the bottom. Also change the title to Steer drafts by chat.</message>" in prompt)
    }

    @Test fun draftParsesQuestionsAndReply() {
        val d = ClaudeService.json.decodeFromString<BeadDraft>(
            """{"title":"t","description":"d","design":"","acceptance_criteria":"","issue_type":"task","priority":1,"labels":[],"open_questions":["q?"],"reply":"Changed the title."}"""
        )
        assertEquals(listOf("q?"), d.open_questions)
        assertEquals("Changed the title.", d.reply)
        assertEquals(emptyList(), ClaudeService.json.decodeFromString<BeadDraft>("""{"title":"t"}""").open_questions)
    }

    @Test fun proposalParsesNullsAsUnchanged() {
        val p = ClaudeService.json.decodeFromString<RefineProposal>(
            """{"summary":"s","title":null,"description":"d","design":null,"acceptance_criteria":null,"notes":null,"priority":1,"issue_type":null,"labels":null,"children":[{"title":"c","description":"","issue_type":"task","priority":2,"acceptance_criteria":""}]}"""
        )
        assertEquals(null, p.title)
        assertEquals("d", p.description)
        assertEquals(1, p.children.size)
    }
}
