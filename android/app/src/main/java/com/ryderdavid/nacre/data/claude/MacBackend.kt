package com.ryderdavid.nacre.data.claude

import com.ryderdavid.nacre.data.NacreRepository
import com.ryderdavid.nacre.data.bridge.AssistMessage
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.model.AssistRequest
import com.ryderdavid.nacre.data.model.AssistResult
import com.ryderdavid.nacre.data.model.RunEvent
import com.ryderdavid.nacre.data.settings.AppSettings
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.decodeFromJsonElement
import java.util.UUID

/**
 * Runs the Claude features as `claude -p` on a paired computer, which spends
 * Ryder's Claude subscription instead of API credit. nacred does the invoking;
 * the prompts, schemas and result types are the same ones the API backend
 * uses, so Refine's diff review and the assistant's approval flow are unchanged.
 */
class MacBackend(
    private val repo: NacreRepository,
    private val settings: suspend () -> AppSettings,
) : ClaudeBackend {
    override val runner = ClaudeRunner.MAC

    override suspend fun refine(repoKey: String, ctx: BeadContext, instruction: String): RefineProposal =
        json(repoKey, Prompts.refine(ctx, instruction), ClaudeService.refineSchema)

    override suspend fun draft(repoKey: String, repoName: String, rough: String, parentTitle: String?, existingTitles: List<String>): BeadDraft =
        json(repoKey, Prompts.draft(repoName, rough, parentTitle, existingTitles), ClaudeService.draftSchema)

    override fun session(board: AssistantBoard): AssistantSession = MacSession(repo, settings, board)

    /** One structured call, decoded with the same serializers as the API path. */
    private suspend inline fun <reified T> json(repoKey: String, prompt: String, schema: Map<String, Any>): T {
        val s = settings()
        val out = translate {
            repo.claudeJson(
                repoKey,
                AssistRequest(
                    prompt = prompt, system = Prompts.EDITOR, schema = schema.toSchemaJson(),
                    model = s.claudeModel, effort = s.claudeEffort,
                ),
            )
        }
        val answer = out.output ?: throw ClaudeException("The Mac answered without a proposal. Try again, or switch to your API key in Settings.")
        return ClaudeService.json.decodeFromJsonElement(answer)
    }
}

/**
 * A conversation held by Claude Code on the Mac. Each turn is one `claude -p`
 * call that reads the board with read-only bd commands and answers with prose
 * plus the writes it would like to make; `--resume` keeps the thread.
 *
 * The writes come back as proposals rather than tool calls, so they land in the
 * app's normal approval flow and are applied by the app through nacred's REST
 * API — Claude Code on the Mac is never allowed to change a bead itself.
 */
class MacSession(
    private val repo: NacreRepository,
    private val settings: suspend () -> AppSettings,
    private val board: AssistantBoard,
) : AssistantSession {
    private var sessionId = ""

    override fun reset() {
        sessionId = ""
    }

    override suspend fun turn(input: TurnInput, sink: TurnSink): TurnResult {
        val runKey = board.runKey ?: throw ClaudeUnavailableException(
            "Asking across every board needs the Claude API; choose \"Claude API key\" in Settings for that.",
            offerApiKey = true,
        )
        val s = settings()
        val req = AssistRequest(
            prompt = when (input) {
                is TurnInput.Say -> input.text
                is TurnInput.Answers -> Prompts.outcomes(input.answers)
            },
            system = Prompts.macAssistant(board.system(), board.tools),
            schema = Prompts.proposalsSchema(board.tools).toSchemaJson(),
            model = s.claudeModel,
            effort = s.claudeEffort,
            sessionId = sessionId,
            bdReads = true,
            keep = true,
        )

        var done: AssistResult? = null
        var failed: String? = null
        translate {
            repo.claudeStream(runKey, req).collect { msg ->
                when (msg) {
                    is AssistMessage.Event -> relay(msg.event, sink)
                    is AssistMessage.Done -> done = msg.result
                    is AssistMessage.Failed -> failed = msg.error
                }
            }
        }
        failed?.let { throw ClaudeException(it) }
        val result = done ?: throw ClaudeException("The Mac stopped sending before Claude finished.")
        if (result.sessionId.isNotBlank()) sessionId = result.sessionId

        val (reply, calls) = result.output?.let { runCatching { macAnswer(it) }.getOrNull() }
            ?: throw ClaudeException("Claude's answer didn't arrive in the shape the app expects.")

        if (reply.isNotBlank()) {
            sink.newBlock()
            sink.text(reply)
        }
        return TurnResult(calls, if (calls.isEmpty()) TurnStop.DONE else TurnStop.CALLS)
    }

    /**
     * Shows the Mac's own steps as they happen. Assistant text is deliberately
     * dropped: with a JSON schema in play those blocks are drafts of the
     * structured answer, and the finished `reply` is what Ryder should read.
     */
    private fun relay(event: RunEvent, sink: TurnSink) {
        when (event.kind) {
            "thinking" -> {
                sink.newBlock()
                sink.thinking(event.text)
            }
            "tool_use" -> sink.aside(event.toolId.ifBlank { event.seq.toString() }, BD_TOOL, event.text)
            "tool_result" -> sink.asideDone(event.toolId, event.text, event.isError == true)
        }
    }

    companion object {
        /** The name shown for a step the Mac ran itself. */
        const val BD_TOOL = "bd"
    }
}

@Serializable
internal data class MacAnswer(val reply: String = "", val proposals: List<MacProposal> = emptyList())

@Serializable
internal data class MacProposal(val tool: String = "", val input: JsonObject = JsonObject(emptyMap()))

/**
 * Reads a Mac turn's structured answer: the prose for Ryder, and the write
 * calls it proposes. Anything that isn't a write tool is dropped — a read
 * belongs to the Mac, which does its own with bd, so a model asking for one
 * here is confused and the app will not run it.
 */
internal fun macAnswer(output: JsonObject, turn: String = UUID.randomUUID().toString().take(8)): Pair<String, List<ToolRequest>> {
    val answer = ClaudeService.json.decodeFromJsonElement<MacAnswer>(output)
    val calls = answer.proposals
        .filter { it.tool in WRITE_TOOLS }
        .mapIndexed { i, p -> ToolRequest("mac-$turn-$i", p.tool, p.input) }
    return answer.reply to calls
}

/**
 * Turns a bridge failure into words about this feature. A Mac that cannot be
 * reached is the common case and the app offers the API key instead.
 */
internal suspend fun <T> translate(block: suspend () -> T): T = try {
    block()
} catch (e: ClaudeException) {
    throw e
} catch (e: ClaudeUnavailableException) {
    throw e
} catch (e: kotlinx.coroutines.CancellationException) {
    throw e
} catch (e: Exception) {
    val friendly = e.friendly()
    val tooOld = friendly.contains("no such route")
    throw ClaudeUnavailableException(
        when {
            tooOld -> "That computer's nacred is too old to run Claude for the app. Update nacred, or use your API key."
            else -> friendly
        },
        offerApiKey = true,
    )
}
