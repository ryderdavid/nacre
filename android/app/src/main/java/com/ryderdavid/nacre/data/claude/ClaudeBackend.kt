package com.ryderdavid.nacre.data.claude

import com.ryderdavid.nacre.data.settings.AppSettings
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/**
 * Where Claude ran for the retired bead chat, Draft and board assistant; the
 * setting is kept (agent-chat-tab leaves it in place).
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

/** Claude's half of Refine, Ask and Draft on this phone's API key. */
interface ClaudeBackend {
    val runner: ClaudeRunner

    /** Improve one bead; the result is reviewed field by field before anything is saved. */
    suspend fun refine(repoKey: String, ctx: BeadContext, instruction: String, history: List<DraftTurn>): RefineProposal

    /** Answer a question about one bead. Nothing is changed and nothing is proposed. */
    suspend fun ask(repoKey: String, ctx: BeadContext, history: List<DraftTurn>, question: String): String

    /** Expand a rough note into a complete new bead. */
    suspend fun draft(repoKey: String, repoName: String, rough: String, parentTitle: String?, existingTitles: List<String>): BeadDraft

    /** Revise a draft from the chat under it: an answer to an open question, or a field to fix. */
    suspend fun reviseDraft(
        repoKey: String, repoName: String, current: BeadDraft, history: List<DraftTurn>, message: String,
        parentTitle: String?, existingTitles: List<String>,
    ): BeadDraft
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
