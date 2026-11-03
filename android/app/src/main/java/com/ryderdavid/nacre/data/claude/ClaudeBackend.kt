package com.ryderdavid.nacre.data.claude

import com.ryderdavid.nacre.data.settings.AppSettings
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/**
 * Where Refine, Draft and the board assistant run.
 *
 * [MAC] spends Ryder's Claude subscription: the work happens in `claude -p` on
 * a paired computer, through nacred. [API] spends money per token against the
 * key stored on this phone. The two produce the same types, so the review
 * screens and the approval flow do not know which one answered.
 */
enum class ClaudeRunner(val key: String, val label: String) {
    MAC(AppSettings.RUNNER_MAC, "My Mac (subscription)"),
    API(AppSettings.RUNNER_API, "Claude API key"),
    ;

    companion object {
        fun of(key: String) = entries.firstOrNull { it.key == key } ?: MAC
    }
}

/** Raised when the chosen backend can't do the job, with wording for the user. */
class ClaudeUnavailableException(
    message: String,
    val offerApiKey: Boolean = false,
    /** Trying the same thing again may well work: the network, not the request, failed. */
    val canRetry: Boolean = false,
) : Exception(message)

/** Claude's half of the app's three Claude features. */
interface ClaudeBackend {
    val runner: ClaudeRunner

    /**
     * Improve one bead; the result is reviewed field by field before anything
     * is saved. [images] are the message's images and those its request carries.
     */
    suspend fun refine(
        repoKey: String, ctx: BeadContext, instruction: String, history: List<DraftTurn>,
        images: ChatTurnImages = ChatTurnImages.NONE,
    ): RefineProposal

    /** Answer a question about one bead. Nothing is changed and nothing is proposed. [images] as for [refine]. */
    suspend fun ask(
        repoKey: String, ctx: BeadContext, history: List<DraftTurn>, question: String,
        images: ChatTurnImages = ChatTurnImages.NONE,
    ): String

    /** Expand a rough note into a complete new bead. */
    suspend fun draft(repoKey: String, repoName: String, rough: String, parentTitle: String?, existingTitles: List<String>): BeadDraft

    /** Revise a draft from the chat under it: an answer to an open question, or a field to fix. */
    suspend fun reviseDraft(
        repoKey: String, repoName: String, current: BeadDraft, history: List<DraftTurn>, message: String,
        parentTitle: String?, existingTitles: List<String>,
    ): BeadDraft

    /** A fresh conversation about [board]. */
    fun session(board: AssistantBoard): AssistantSession
}

/**
 * The board a conversation is about. [runKey] is the repo key a Mac-hosted
 * session runs `claude -p` in; it is null when the conversation spans several
 * boards, and then [span] says which ones, so the Mac can read them all in one
 * call. [tools] is the catalogue this conversation offers, so a conversation
 * that covers several boards can widen it without either backend knowing how.
 */
class AssistantBoard(
    val runKey: String?,
    val name: String,
    val tools: List<ToolSpec> = BOARD_TOOLS,
    val span: (suspend () -> BoardSpan)? = null,
    val system: suspend () -> String,
)

/**
 * The boards a conversation across boards covers. [overview] is what
 * list_boards would answer, for a backend that can't call it.
 */
class BoardSpan(val boards: List<SpanBoard>, val overview: String)

/** One board, by the name proposals use, with its checkout on each computer as repo keys. */
data class SpanBoard(val name: String, val keys: List<String>)

/** A conversation with Claude. The session owns the history; the caller owns the UI. */
interface AssistantSession {
    /** Streams one turn and reports what Claude wants to do next. */
    suspend fun turn(input: TurnInput, sink: TurnSink): TurnResult

    /** Forget the conversation so the next turn starts fresh. */
    fun reset()
}

sealed interface TurnInput {
    /** Something the user typed, with the images attached to it. */
    data class Say(val text: String, val images: List<ImagePayload> = emptyList()) : TurnInput

    /** What came of the calls the previous turn asked for. */
    data class Answers(val answers: List<ToolAnswer>) : TurnInput
}

/** A call Claude wants to make. Writes are approved in the app before they run. */
data class ToolRequest(val id: String, val name: String, val input: JsonObject?)

/** The outcome of one [ToolRequest], as Claude will read it. */
data class ToolAnswer(val id: String, val name: String, val content: String, val isError: Boolean)

enum class TurnStop {
    /** Claude is finished answering. */
    DONE,

    /** Claude asked for the calls in [TurnResult.calls]. */
    CALLS,
    REFUSAL,
    TOO_LONG,
}

data class TurnResult(val calls: List<ToolRequest>, val stop: TurnStop)

/** Where a turn's output goes while it is still arriving. */
interface TurnSink {
    /** A new block started: the next text or thinking begins a new entry. */
    fun newBlock()

    fun text(delta: String)
    fun thinking(delta: String)

    /** A step the backend took by itself — on the Mac, a read-only bd command. */
    fun aside(id: String, name: String, summary: String)

    fun asideDone(id: String, result: String, isError: Boolean)
}

/** Resolves the backend from the user's choice in Settings. */
class ClaudeBackends(private val api: ClaudeBackend, private val mac: ClaudeBackend) {
    fun of(s: AppSettings): ClaudeBackend = if (s.onMac) mac else api
}

/**
 * The schemas in this package are plain maps so both backends can send them —
 * the SDK wants JSON values, nacred wants a JSON body.
 */
internal fun Any?.toJsonElement(): kotlinx.serialization.json.JsonElement = when (this) {
    null -> JsonNull
    is String -> JsonPrimitive(this)
    is Boolean -> JsonPrimitive(this)
    is Number -> JsonPrimitive(this)
    is Map<*, *> -> JsonObject(entries.associate { (k, v) -> k.toString() to v.toJsonElement() })
    is Iterable<*> -> JsonArray(map { it.toJsonElement() })
    else -> JsonPrimitive(toString())
}

internal fun Map<String, Any>.toSchemaJson(): JsonObject = (this as Any).toJsonElement() as JsonObject
