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
            .tools(BOARD_TOOLS.map { ToolUnion.ofTool(it.toTool()) })
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

    @Test fun proposalParsesNullsAsUnchanged() {
        val p = ClaudeService.json.decodeFromString<RefineProposal>(
            """{"summary":"s","title":null,"description":"d","design":null,"acceptance_criteria":null,"notes":null,"priority":1,"issue_type":null,"labels":null,"children":[{"title":"c","description":"","issue_type":"task","priority":2,"acceptance_criteria":""}]}"""
        )
        assertEquals(null, p.title)
        assertEquals("d", p.description)
        assertEquals(1, p.children.size)
    }
}
