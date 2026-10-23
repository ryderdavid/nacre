package com.ryderdavid.nacre.data.claude

import com.anthropic.client.AnthropicClient
import com.anthropic.client.okhttp.AnthropicOkHttpClient
import com.anthropic.core.JsonValue
import com.anthropic.core.jsonMapper
import com.anthropic.errors.AnthropicIoException
import com.anthropic.errors.AnthropicServiceException
import com.anthropic.errors.PermissionDeniedException
import com.anthropic.errors.RateLimitException
import com.anthropic.errors.UnauthorizedException
import com.anthropic.models.messages.CacheControlEphemeral
import com.anthropic.models.messages.JsonOutputFormat
import com.anthropic.models.messages.Message
import com.anthropic.models.messages.MessageCreateParams
import com.anthropic.models.messages.OutputConfig
import com.anthropic.models.messages.StopReason
import com.anthropic.models.messages.TextBlockParam
import com.ryderdavid.nacre.data.settings.AppSettings
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import java.time.Duration

class ClaudeException(message: String) : Exception(message)

/** Models offered in Settings. Opus 5.5 is the default. */
val CLAUDE_MODELS = listOf(
    "claude-opus-5-5" to "Claude Opus 5.5",
    "claude-sonnet-5-5" to "Claude Sonnet 5.5",
    "claude-haiku-5-5" to "Claude Haiku 5.5",
    "claude-fable-5-1" to "Claude Fable 5.1",
)

/** Everything Claude needs to know about one bead and its neighbourhood. */
data class BeadContext(
    val repoName: String,
    val id: String,
    val title: String,
    val type: String,
    val priority: Int,
    val status: String,
    val labels: List<String>,
    val description: String,
    val design: String,
    val acceptance: String,
    val notes: String,
    val parentTitle: String?,
    val siblingTitles: List<String>,
    val childTitles: List<String>,
)

@Serializable
data class ChildDraft(
    val title: String,
    val description: String = "",
    val issue_type: String = "task",
    val priority: Int = 2,
    val acceptance_criteria: String = "",
)

/** A proposed edit. Null fields mean "leave unchanged". */
@Serializable
data class RefineProposal(
    val summary: String = "",
    val title: String? = null,
    val description: String? = null,
    val design: String? = null,
    val acceptance_criteria: String? = null,
    val notes: String? = null,
    val priority: Int? = null,
    val issue_type: String? = null,
    val labels: List<String>? = null,
    val children: List<ChildDraft> = emptyList(),
)

@Serializable
data class BeadDraft(
    val title: String,
    val description: String = "",
    val design: String = "",
    val acceptance_criteria: String = "",
    val issue_type: String = "task",
    val priority: Int = 2,
    val labels: List<String> = emptyList(),
)

/** Claude API access with the user's own key (Keystore-encrypted on device). */
class ClaudeService {
    private var cached: Pair<String, AnthropicClient>? = null

    fun client(key: String): AnthropicClient {
        if (key.isBlank()) throw ClaudeException("Add your Claude API key in Settings first")
        cached?.let { (k, c) -> if (k == key) return c }
        val c = AnthropicOkHttpClient.builder()
            .apiKey(key)
            .maxRetries(2)
            .timeout(Duration.ofMinutes(10))
            .build()
        cached = key to c
        return c
    }

    /** Ask Claude to improve a bead; returns a field-level proposal the user reviews. */
    suspend fun refine(s: AppSettings, ctx: BeadContext, instruction: String): RefineProposal = withContext(Dispatchers.IO) {
        val text = structured(s, Prompts.EDITOR, Prompts.refine(ctx, instruction), refineSchema, maxTokens = 16000)
        json.decodeFromString<RefineProposal>(text)
    }

    /** Expand a rough note into a complete new bead. */
    suspend fun draft(s: AppSettings, repoName: String, rough: String, parentTitle: String?, existingTitles: List<String>): BeadDraft = withContext(Dispatchers.IO) {
        val text = structured(s, Prompts.EDITOR, Prompts.draft(repoName, rough, parentTitle, existingTitles), draftSchema, maxTokens = 8000)
        json.decodeFromString<BeadDraft>(text)
    }

    private fun structured(s: AppSettings, system: String, prompt: String, schema: Map<String, Any>, maxTokens: Long): String {
        val params = baseParams(s, system, maxTokens)
            .outputConfig(
                OutputConfig.builder()
                    .effort(effortOf(s.claudeEffort))
                    .format(JsonOutputFormat.builder().schema(schemaOf(schema)).build())
                    .build()
            )
            .addUserMessage(prompt)
            .build()
        val msg = call { client(s.anthropicKey).messages().create(params) }
        checkStop(msg)
        return msg.content().mapNotNull { b -> b.text().map { it.text() }.orElse(null) }.joinToString("")
            .ifBlank { throw ClaudeException("Claude returned no proposal") }
    }

    companion object {
        internal val json = Json { ignoreUnknownKeys = true; coerceInputValues = true; isLenient = true }

        /** Refusal fallbacks for models that support the server-side "default" mode. */
        private val FALLBACK_MODELS = setOf("claude-opus-5-5", "claude-sonnet-5-5", "claude-fable-5-1")

        internal fun baseParams(s: AppSettings, system: String, maxTokens: Long): MessageCreateParams.Builder =
            MessageCreateParams.builder()
                .model(s.claudeModel)
                .maxTokens(maxTokens)
                .systemOfTextBlockParams(listOf(TextBlockParam.builder().text(system).build()))
                .cacheControl(CacheControlEphemeral.builder().build())
                .apply {
                    if (s.claudeModel in FALLBACK_MODELS) {
                        putAdditionalHeader("anthropic-beta", "server-side-fallback-2026-07-01")
                        putAdditionalBodyProperty("fallbacks", JsonValue.from("default"))
                    }
                }

        internal fun effortOf(e: String): OutputConfig.Effort = when (e) {
            "low" -> OutputConfig.Effort.LOW
            "high" -> OutputConfig.Effort.HIGH
            "xhigh" -> OutputConfig.Effort.XHIGH
            else -> OutputConfig.Effort.MEDIUM
        }

        internal fun schemaOf(schema: Map<String, Any>): JsonOutputFormat.Schema =
            JsonOutputFormat.Schema.builder().apply { schema.forEach { (k, v) -> putAdditionalProperty(k, JsonValue.from(v)) } }.build()

        internal fun checkStop(msg: Message) {
            when (msg.stopReason().orElse(null)) {
                StopReason.REFUSAL -> throw ClaudeException("Claude declined this request")
                StopReason.MAX_TOKENS -> throw ClaudeException("Claude ran out of room before finishing; try a narrower request")
                else -> Unit
            }
        }

        internal fun JsonValue.toJsonString(): String = jsonMapper().writeValueAsString(this)

        /** Run an SDK call, translating SDK errors into messages a person can act on. */
        internal inline fun <T> call(block: () -> T): T = try {
            block()
        } catch (e: UnauthorizedException) {
            throw ClaudeException("Your Claude API key was rejected. Check it in Settings.")
        } catch (e: PermissionDeniedException) {
            throw ClaudeException("This API key can't use that model.")
        } catch (e: RateLimitException) {
            throw ClaudeException("Rate limited by the Claude API. Try again in a moment.")
        } catch (e: AnthropicServiceException) {
            throw ClaudeException("Claude API error ${e.statusCode()}: ${e.message?.take(200)}")
        } catch (e: AnthropicIoException) {
            throw ClaudeException("Couldn't reach the Claude API. Check your connection.")
        }

        private fun nullable(type: String, description: String) =
            mapOf("anyOf" to listOf(mapOf("type" to type), mapOf("type" to "null")), "description" to description)

        private val childSchema = mapOf(
            "type" to "object",
            "additionalProperties" to false,
            "required" to listOf("title", "description", "issue_type", "priority", "acceptance_criteria"),
            "properties" to mapOf(
                "title" to mapOf("type" to "string"),
                "description" to mapOf("type" to "string"),
                "issue_type" to mapOf("type" to "string"),
                "priority" to mapOf("type" to "integer"),
                "acceptance_criteria" to mapOf("type" to "string"),
            ),
        )

        val refineSchema: Map<String, Any> = mapOf(
            "type" to "object",
            "additionalProperties" to false,
            "required" to listOf("summary", "title", "description", "design", "acceptance_criteria", "notes", "priority", "issue_type", "labels", "children"),
            "properties" to mapOf(
                "summary" to mapOf("type" to "string", "description" to "One or two sentences on what changed and why"),
                "title" to nullable("string", "New title, or null"),
                "description" to nullable("string", "New description (markdown), or null"),
                "design" to nullable("string", "New design notes (markdown), or null"),
                "acceptance_criteria" to nullable("string", "New acceptance criteria (markdown list), or null"),
                "notes" to nullable("string", "New notes, or null"),
                "priority" to nullable("integer", "0-4, or null"),
                "issue_type" to nullable("string", "bug|feature|task|chore|epic|decision|spike, or null"),
                "labels" to mapOf("anyOf" to listOf(mapOf("type" to "array", "items" to mapOf("type" to "string")), mapOf("type" to "null"))),
                "children" to mapOf("type" to "array", "items" to childSchema, "description" to "New child beads to create, usually empty"),
            ),
        )

        val draftSchema: Map<String, Any> = mapOf(
            "type" to "object",
            "additionalProperties" to false,
            "required" to listOf("title", "description", "design", "acceptance_criteria", "issue_type", "priority", "labels"),
            "properties" to mapOf(
                "title" to mapOf("type" to "string"),
                "description" to mapOf("type" to "string"),
                "design" to mapOf("type" to "string"),
                "acceptance_criteria" to mapOf("type" to "string"),
                "issue_type" to mapOf("type" to "string"),
                "priority" to mapOf("type" to "integer"),
                "labels" to mapOf("type" to "array", "items" to mapOf("type" to "string")),
            ),
        )
    }
}

internal fun BeadContext.render(): String = buildString {
    appendLine("<bead id=\"$id\" type=\"$type\" priority=\"P$priority\" status=\"$status\">")
    appendLine("<title>$title</title>")
    if (labels.isNotEmpty()) appendLine("<labels>${labels.joinToString(", ")}</labels>")
    appendLine("<description>\n$description\n</description>")
    if (design.isNotBlank()) appendLine("<design>\n$design\n</design>")
    if (acceptance.isNotBlank()) appendLine("<acceptance_criteria>\n$acceptance\n</acceptance_criteria>")
    if (notes.isNotBlank()) appendLine("<notes>\n$notes\n</notes>")
    appendLine("</bead>")
    parentTitle?.let { appendLine("<parent>$it</parent>") }
    if (siblingTitles.isNotEmpty()) appendLine("<siblings>\n${siblingTitles.take(40).joinToString("\n") { "- $it" }}\n</siblings>")
    if (childTitles.isNotEmpty()) appendLine("<children>\n${childTitles.take(40).joinToString("\n") { "- $it" }}\n</children>")
}
