package com.ryderdavid.nacre.data.claude

import com.ryderdavid.nacre.data.settings.AppSettings
import com.ryderdavid.nacre.data.settings.Host
import kotlinx.serialization.json.JsonObject
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/** The choice in Settings, and what the two backends have to agree on. */
class BackendTest {
    private val host = Host("c1", "h1", "Mac Studio", "http://mac:7420", "tok")

    @Test fun macIsTheDefaultOnceAComputerIsPaired() {
        val fresh = AppSettings()
        assertEquals(AppSettings.RUNNER_MAC, fresh.claudeRunner)
        // Nothing paired and no key: the features say so rather than failing later.
        assertFalse(fresh.onMac)
        assertFalse(fresh.claudeConfigured)

        val paired = AppSettings(hosts = listOf(host))
        assertTrue(paired.onMac, "a paired computer runs Claude without an API key")
        assertTrue(paired.claudeConfigured)
        assertFalse(paired.apiKeyConfigured)
    }

    @Test fun choosingTheApiKeyUsesIt() {
        val s = AppSettings(hosts = listOf(host), claudeRunner = AppSettings.RUNNER_API, anthropicKey = "sk-ant-x")
        assertFalse(s.onMac)
        assertTrue(s.claudeConfigured)
    }

    @Test fun theMacChoiceFallsBackToTheKeyWithNoComputer() {
        // Picking the Mac on a phone that has never paired one must not strand
        // Refine and Draft; the key still answers.
        val s = AppSettings(claudeRunner = AppSettings.RUNNER_MAC, anthropicKey = "sk-ant-x")
        assertFalse(s.onMac)
        assertTrue(s.claudeConfigured)
    }

    @Test fun bothBackendsSendTheSameSchemas() {
        // The Mac path posts these as JSON; the API path puts them in output_config.
        // They have to be the same objects or the two answers would not parse
        // against RefineProposal and BeadDraft alike.
        val refine = ClaudeService.refineSchema.toSchemaJson()
        assertEquals("object", refine["type"].asText())
        val required = refine["required"].toString()
        for (field in listOf("summary", "title", "acceptance_criteria", "children")) {
            assertTrue(field in required, "$field is required in the refine schema")
        }
        val draft = ClaudeService.draftSchema.toSchemaJson()
        assertTrue("issue_type" in draft["required"].toString())
    }

    @Test fun schemaMapsConvertToJsonFaithfully() {
        val json = mapOf(
            "type" to "object",
            "strict" to true,
            "max" to 4,
            "required" to listOf("a", "b"),
            "properties" to mapOf("a" to mapOf("type" to "string")),
        ).toSchemaJson()
        assertEquals("object", json["type"].asText())
        assertEquals("true", json["strict"].toString())
        assertEquals("4", json["max"].toString())
        assertEquals("""["a","b"]""", json["required"].toString())
        assertEquals("string", json["properties"].asObject()["a"].asObject()["type"].asText())
    }
}

internal fun kotlinx.serialization.json.JsonElement?.asText(): String = toString().trim('"')
internal fun kotlinx.serialization.json.JsonElement?.asObject(): JsonObject = this as JsonObject

/** The draft chat sends the same prompt and schema whichever backend runs it. */
class DraftChatOnTheMacTest {
    @Test fun revisingADraftOnTheMacUsesTheDraftSchemaAndTheRevisePrompt() {
        val current = BeadDraft(title = "Add chat", open_questions = listOf("Bottom of the sheet?"))
        val prompt = Prompts.reviseDraft("Nacre", current, listOf(DraftTurn(true, "make it P1")), "yes, at the bottom", null, emptyList())
        assertTrue("<message>yes, at the bottom</message>" in prompt)
        assertTrue("Bottom of the sheet?" in prompt)
        val schema = ClaudeService.draftSchema.toSchemaJson().toString()
        assertTrue("open_questions" in schema && "reply" in schema)
    }
}
