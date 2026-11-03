package com.ryderdavid.nacre.data.claude

import com.anthropic.helpers.MessageAccumulator
import com.anthropic.models.messages.ContentBlockParam
import com.anthropic.models.messages.MessageParam
import com.anthropic.models.messages.OutputConfig
import com.anthropic.models.messages.StopReason
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

/** Runs the Claude features against the Claude API with the key on this phone. */
class ApiBackend(
    private val service: ClaudeService,
    private val settings: suspend () -> AppSettings,
) : ClaudeBackend {
    override val runner = ClaudeRunner.API

    override suspend fun refine(
        repoKey: String, ctx: BeadContext, instruction: String, history: List<DraftTurn>, images: ChatTurnImages,
    ): RefineProposal = service.refine(settings(), ctx, instruction, history)

    override suspend fun ask(
        repoKey: String, ctx: BeadContext, history: List<DraftTurn>, question: String, images: ChatTurnImages,
    ): String = service.ask(settings(), ctx, history, question)

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
    private val history = mutableListOf<MessageParam>()
    private val tools: List<ToolUnion> by lazy { board.tools.map { ToolUnion.ofTool(it.toTool()) } }

    override fun reset() {
        history.clear()
    }

    override suspend fun turn(input: TurnInput, sink: TurnSink): TurnResult {
        when (input) {
            // A turn cancelled while its tools were running leaves calls nobody
            // answered, and the API rejects the conversation from then on unless
            // every tool_use has a tool_result. Close them before saying more.
            is TurnInput.Say -> {
                answerDanglingToolUse(history)?.let { history += it }
                history += MessageParam.builder().role(MessageParam.Role.USER).content(input.text).build()
            }
            is TurnInput.Answers -> history += toolResults(
                input.answers.map { it.id },
                input.answers.associate { it.id to (it.content to it.isError) },
            )
        }

        val s = settings()
        val client = service.client(s.anthropicKey)
        val params = ClaudeService.baseParams(s, board.system(), 64000)
            .thinking(ThinkingConfigAdaptive.builder().display(ThinkingConfigAdaptive.Display.SUMMARIZED).build())
            .outputConfig(OutputConfig.builder().effort(ClaudeService.effortOf(s.claudeEffort)).build())
            .tools(tools)
            .messages(history.toList())
            .build()

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
        history += msg.toParam()

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
