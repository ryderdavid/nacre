package com.ryderdavid.nacre.data.claude

import com.ryderdavid.nacre.data.settings.AppSettings
import com.ryderdavid.nacre.data.settings.Host
import kotlinx.serialization.json.JsonObject
import kotlin.test.Test
import com.ryderdavid.nacre.data.bridge.BridgeException
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
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

    @Test fun backendsFollowTheSetting() {
        val api = StubBackend(ClaudeRunner.API)
        val mac = StubBackend(ClaudeRunner.MAC)
        val backends = ClaudeBackends(api, mac)
        assertEquals(ClaudeRunner.MAC, backends.of(AppSettings(hosts = listOf(host))).runner)
        assertEquals(ClaudeRunner.API, backends.of(AppSettings(hosts = listOf(host), claudeRunner = AppSettings.RUNNER_API)).runner)
        assertEquals(ClaudeRunner.API, backends.of(AppSettings(anthropicKey = "k")).runner)
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

    @Test fun macAnswerSchemaOnlyOffersWriteTools() {
        val schema = Prompts.proposalsSchema().toSchemaJson()
        val tool = schema["properties"]!!.asObject()["proposals"]!!.asObject()["items"]!!
            .asObject()["properties"]!!.asObject()["tool"]!!.asObject()
        val allowed = tool["enum"].toString()
        assertTrue("update_bead" in allowed && "run_in_claude_code" in allowed)
        // Reads happen on the Mac with bd, so proposing one would be nonsense.
        assertFalse("list_beads" in allowed, "reads are not proposals")
        assertFalse("get_bead" in allowed)
    }

    @Test fun theCatalogueDescribesEveryWriteTool() {
        val catalogue = writeToolCatalogue()
        for (spec in BOARD_TOOLS) {
            if (spec.write) {
                assertTrue("### ${spec.name}" in catalogue, "${spec.name} is missing from the catalogue")
                spec.required.forEach { assertTrue("`$it`" in catalogue, "${spec.name}.$it is missing") }
            } else {
                assertFalse("### ${spec.name}" in catalogue, "${spec.name} is a read and should not be proposable")
            }
        }
        assertTrue("(required)" in catalogue)
    }

    @Test fun macSystemPromptKeepsTheBoardPromptAndForbidsWriting() {
        val prompt = Prompts.macAssistant("You are the board assistant. Board: Nacre.")
        assertTrue("Board: Nacre." in prompt, "the board's own prompt is kept")
        assertTrue("cannot change beads yourself" in prompt)
        assertTrue("bd show <id>" in prompt)
    }

    @Test fun macProposalsBecomeToolCallsTheAppExecutes() {
        val output = ClaudeService.json.decodeFromString<JsonObject>(
            """
            {"reply":"Two edits for you.","proposals":[
              {"tool":"update_bead","input":{"id":"NAC-1","priority":1}},
              {"tool":"list_beads","input":{"lane":"ready"}},
              {"tool":"close_bead","input":{"id":"NAC-2","reason":"done"}}
            ]}
            """.trimIndent()
        )
        val (reply, calls) = macAnswer(output, turn = "t")
        assertEquals("Two edits for you.", reply)
        // The read is dropped; the two writes keep their order and get distinct ids.
        assertEquals(listOf("update_bead", "close_bead"), calls.map { it.name })
        assertEquals(listOf("mac-t-0", "mac-t-1"), calls.map { it.id })
        assertEquals("NAC-1", calls[0].input?.get("id").asText())
        assertTrue(calls.all { it.name in WRITE_TOOLS })
    }

    @Test fun anAnswerWithNoProposalsIsJustAReply() {
        val output = ClaudeService.json.decodeFromString<JsonObject>("""{"reply":"Nothing is blocked.","proposals":[]}""")
        val (reply, calls) = macAnswer(output)
        assertEquals("Nothing is blocked.", reply)
        assertTrue(calls.isEmpty())
    }

    @Test fun outcomesTellTheNextTurnWhatWasDeclined() {
        val note = Prompts.outcomes(
            listOf(
                ToolAnswer("a", "update_bead", "Updated: NAC-1", isError = false),
                ToolAnswer("b", "close_bead", "The user declined this action.", isError = true),
            )
        )
        assertTrue("applied — Updated: NAC-1" in note)
        assertTrue("NOT APPLIED" in note)
        assertTrue("Do not repeat anything that was declined" in note)
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

    @Test fun anUnreachableMacOffersTheApiKeyInstead() = kotlinx.coroutines.runBlocking {
        val e = assertFailsWith<ClaudeUnavailableException> {
            translate<Unit> { throw java.net.ConnectException("Connection refused") }
        }
        assertTrue("Tailscale" in e.message.orEmpty(), "says what to check: ${e.message}")
        assertTrue(e.offerApiKey, "and the app can offer the key instead")

        // An older nacred has no such route; say so rather than showing a 404.
        val old = assertFailsWith<ClaudeUnavailableException> {
            translate<Unit> { throw BridgeException("no such route: POST /v1/repos/x/claude/json", code = 404) }
        }
        assertTrue("too old" in old.message.orEmpty(), old.message.orEmpty())

        // Claude's own complaints are already phrased for a person; keep them.
        val claude = assertFailsWith<ClaudeException> {
            translate<Unit> { throw ClaudeException("usage limit reached") }
        }
        assertEquals("usage limit reached", claude.message)
    }

    private class StubBackend(override val runner: ClaudeRunner) : ClaudeBackend {
        override suspend fun refine(repoKey: String, ctx: BeadContext, instruction: String, history: List<DraftTurn>) = RefineProposal()
        override suspend fun ask(repoKey: String, ctx: BeadContext, history: List<DraftTurn>, question: String) = ""
        override suspend fun draft(repoKey: String, repoName: String, rough: String, parentTitle: String?, existingTitles: List<String>) = BeadDraft("x")
        override suspend fun reviseDraft(repoKey: String, repoName: String, current: BeadDraft, history: List<DraftTurn>, message: String, parentTitle: String?, existingTitles: List<String>) = current
        override fun session(board: AssistantBoard) = throw UnsupportedOperationException()
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
