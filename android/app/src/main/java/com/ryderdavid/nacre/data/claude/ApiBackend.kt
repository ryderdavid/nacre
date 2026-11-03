package com.ryderdavid.nacre.data.claude

import com.anthropic.helpers.MessageAccumulator
import com.anthropic.models.messages.Base64ImageSource
import com.anthropic.models.messages.ContentBlockParam
import com.anthropic.models.messages.ImageBlockParam
import com.anthropic.models.messages.MessageCreateParams
import com.anthropic.models.messages.MessageParam
import com.anthropic.models.messages.OutputConfig
import com.anthropic.models.messages.StopReason
import com.anthropic.models.messages.TextBlockParam
import com.anthropic.models.messages.ThinkingConfigAdaptive
import com.anthropic.models.messages.Tool
import com.anthropic.models.messages.ToolResultBlockParam
import com.anthropic.models.messages.ToolUnion
import com.ryderdavid.nacre.data.claude.ClaudeService.Companion.toJsonString
import com.ryderdavid.nacre.data.settings.AppSettings
import com.anthropic.core.JsonValue
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.util.Base64

/** Runs the Claude features against the Claude API with the key on this phone. */
class ApiBackend(
    private val service: ClaudeService,
    private val settings: suspend () -> AppSettings,
) : ClaudeBackend {
    override val runner = ClaudeRunner.API

    override suspend fun refine(
        repoKey: String, ctx: BeadContext, instruction: String, history: List<DraftTurn>, images: ChatTurnImages,
    ): RefineProposal = service.refine(settings(), ctx, instruction, history, images)

    override suspend fun ask(
        repoKey: String, ctx: BeadContext, history: List<DraftTurn>, question: String, images: ChatTurnImages,
    ): String = service.ask(settings(), ctx, history, question, images)

    override suspend fun draft(repoKey: String, repoName: String, rough: String, parentTitle: String?, existingTitles: List<String>): BeadDraft =
        service.draft(settings(), repoName, rough, parentTitle, existingTitles)

    override suspend fun reviseDraft(
        repoKey: String, repoName: String, current: BeadDraft, history: List<DraftTurn>, message: String,
        parentTitle: String?, existingTitles: List<String>,
    ): BeadDraft = service.reviseDraft(settings(), repoName, current, history, message, parentTitle, existingTitles)

    override fun session(board: AssistantBoard): AssistantSession = ApiSession(service, settings, board)
}

/**
 * A conversation held in the app: the history lives here as Messages API
 * parameters, and each turn is one streaming request whose tool calls the
 * caller executes.
 */
class ApiSession(
    private val service: ClaudeService,
    private val settings: suspend () -> AppSettings,
    private val board: AssistantBoard,
) : AssistantSession {
    /**
     * Ryder's messages are kept as what he said and the images he attached, so
     * each request can choose again which images fit its budget; everything
     * else is kept as the parameters it was sent as.
     */
    private sealed interface Entry {
        class Said(val text: String, val images: List<ImagePayload>) : Entry
        class Sent(val param: MessageParam) : Entry
    }

    private val history = mutableListOf<Entry>()
    private val tools: List<ToolUnion> by lazy { board.tools.map { ToolUnion.ofTool(it.toTool()) } }

    override fun reset() {
        history.clear()
    }

    /** Adds [input] to the conversation, closing any tool calls a cancelled turn left unanswered. */
    internal fun record(input: TurnInput) {
        when (input) {
            // A turn cancelled while its tools were running leaves calls nobody
            // answered, and the API rejects the conversation from then on unless
            // every tool_use has a tool_result. Close them before saying more.
            is TurnInput.Say -> {
                answerDanglingToolUse(messages())?.let { history += Entry.Sent(it) }
                history += Entry.Said(input.text, input.images)
            }
            is TurnInput.Answers -> history += Entry.Sent(
                toolResults(
                    input.answers.map { it.id },
                    input.answers.associate { it.id to (it.content to it.isError) },
                )
            )
        }
    }

    /**
     * The conversation as Messages API parameters. Each of Ryder's messages
     * holds its own images before its text, as many as fit the request budget
     * (newest first); the rest are noted in the text instead. A message
     * without images is the plain text message it always was.
     */
    internal fun messages(): List<MessageParam> {
        val said = history.filterIsInstance<Entry.Said>()
        val bytes = said.flatMap { it.images }.associate { it.image.id to it.bytes }
        val chosen = selectImages(said.map { e -> e.images.map { it.image } }) { bytes[it.id] }
        return history.map { e ->
            when (e) {
                is Entry.Sent -> e.param
                is Entry.Said -> userMessage(e, chosen)
            }
        }
    }

    private fun userMessage(e: Entry.Said, chosen: RequestImages): MessageParam {
        val b = MessageParam.builder().role(MessageParam.Role.USER)
        if (e.images.isEmpty()) return b.content(e.text).build()
        val sent = chosen.images.associateBy { it.image.id }
        val text = withNotes(e.text, e.images.map { it.image }, chosen.marks)
        val blocks = e.images.mapNotNull { sent[it.image.id]?.let(::imageBlock) } +
            listOfNotNull(text.takeIf { it.isNotBlank() }?.let(::textBlock))
        return b.contentOfBlockParams(blocks).build()
    }

    internal fun params(s: AppSettings, system: String): MessageCreateParams =
        ClaudeService.baseParams(s, system, 64000)
            .thinking(ThinkingConfigAdaptive.builder().display(ThinkingConfigAdaptive.Display.SUMMARIZED).build())
            .outputConfig(OutputConfig.builder().effort(ClaudeService.effortOf(s.claudeEffort)).build())
            .tools(tools)
            .messages(messages())
            .build()

    override suspend fun turn(input: TurnInput, sink: TurnSink): TurnResult {
        record(input)

        val s = settings()
        val client = service.client(s.anthropicKey)
        val params = params(s, board.system())

        val acc = MessageAccumulator.create()
        withContext(Dispatchers.IO) {
            ClaudeService.call {
                client.messages().createStreaming(params).use { stream ->
                    stream.stream().forEach { event ->
                        ensureActive()
                        acc.accumulate(event)
                        event.contentBlockStart().ifPresent { sink.newBlock() }
                        event.contentBlockDelta().ifPresent { d ->
                            d.delta().text().ifPresent { sink.text(it.text()) }
                            d.delta().thinking().ifPresent { sink.thinking(it.thinking()) }
                        }
                    }
                }
            }
        }
        val msg = acc.message()
        history += Entry.Sent(msg.toParam())

        val stop = when (msg.stopReason().orElse(null)) {
            StopReason.TOOL_USE -> TurnStop.CALLS
            StopReason.REFUSAL -> TurnStop.REFUSAL
            StopReason.MAX_TOKENS -> TurnStop.TOO_LONG
            else -> TurnStop.DONE
        }
        if (stop != TurnStop.CALLS) return TurnResult(emptyList(), stop)
        val calls = msg.content().mapNotNull { it.toolUse().orElse(null) }.map { call ->
            ToolRequest(
                call.id(), call.name(),
                runCatching { ClaudeService.json.parseToJsonElement(call._input().toJsonString()).jsonObject }.getOrNull(),
            )
        }
        return TurnResult(calls, stop)
    }
}

internal fun textBlock(text: String): ContentBlockParam = ContentBlockParam.ofText(TextBlockParam.builder().text(text).build())

/** An image as a base64 image block with its media type. */
internal fun imageBlock(p: ImagePayload): ContentBlockParam = ContentBlockParam.ofImage(
    ImageBlockParam.builder()
        .source(
            Base64ImageSource.builder()
                .mediaType(Base64ImageSource.MediaType.of(p.image.mimeType))
                .data(Base64.getEncoder().encodeToString(p.bytes))
                .build()
        )
        .build()
)

/** Each of [images] as its `Image N: <name>` label and its image block, numbered 1..N in order. */
internal fun labelledImageBlocks(images: List<ImagePayload>): List<ContentBlockParam> =
    images.flatMapIndexed { i, p -> listOf(textBlock("Image ${i + 1}: ${p.image.fileName}"), imageBlock(p)) }

/** The Messages API definition of a tool from the shared catalogue. */
internal fun ToolSpec.toTool(): Tool = Tool.builder()
    .name(name)
    .description(description)
    .strict(true)
    .eagerInputStreaming(true)
    .inputSchema(
        Tool.InputSchema.builder()
            .properties(
                Tool.InputSchema.Properties.builder()
                    .apply { props.forEach { (k, v) -> putAdditionalProperty(k, JsonValue.from(v)) } }
                    .build()
            )
            .required(props.keys.filter { it !in optional })
            .putAdditionalProperty("additionalProperties", JsonValue.from(false))
            .build()
    )
    .build()

/** One user message answering [ids] in order; any without a result are marked not run. */
internal fun toolResults(ids: List<String>, results: Map<String, Pair<String, Boolean>>): MessageParam {
    val blocks = ids.map { id ->
        val (content, isError) = results[id] ?: ("Not run: the step was interrupted before this tool call ran." to true)
        ContentBlockParam.ofToolResult(ToolResultBlockParam.builder().toolUseId(id).content(content).isError(isError).build())
    }
    // All results go back in one user message so parallel tool use keeps working.
    return MessageParam.builder().role(MessageParam.Role.USER).contentOfBlockParams(blocks).build()
}

/** The message that repairs [history] when its last assistant turn asked for tools nobody answered. */
internal fun answerDanglingToolUse(history: List<MessageParam>): MessageParam? {
    val last = history.lastOrNull() ?: return null
    if (last.role() != MessageParam.Role.ASSISTANT) return null
    val ids = last.content().blockParams().orElse(emptyList()).mapNotNull { b -> b.toolUse().orElse(null)?.id() }
    return if (ids.isEmpty()) null else toolResults(ids, emptyMap())
}

/** Labels may arrive as a JSON array in update/create inputs. */
internal fun JsonObject.stringList(key: String): List<String>? =
    (this[key] as? kotlinx.serialization.json.JsonArray)?.map { it.jsonPrimitive.content }
