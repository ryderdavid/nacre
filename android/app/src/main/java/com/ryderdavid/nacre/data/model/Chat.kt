package com.ryderdavid.nacre.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.Transient
import kotlinx.serialization.json.JsonElement

/** Chat's wire types: nacred's `/v1/chats` records, events and request bodies (docs/API.md). */
object ChatWire {
    const val AGENT_CLAUDE = "claude"
    const val AGENT_CODEX = "codex"

    const val IDLE = "idle"
    const val RUNNING = "running"
    const val WAITING_PERMISSION = "waiting_permission"
    const val INTERRUPTED = "interrupted"
    const val LIMIT_REACHED = "limit_reached"

    // Event kinds.
    const val USER = "user"
    const val TEXT = "text"
    const val THINKING = "thinking"
    const val TOOL_USE = "tool_use"
    const val TOOL_RESULT = "tool_result"
    const val SYSTEM = "system"
    const val RESULT = "result"
    const val PERMISSION = "permission"
    const val PERMISSION_ANSWER = "permission_answer"
    const val HANDOFF = "handoff"
    const val DRAFT = "draft"
    const val ERROR = "error"
    const val TURN_END = "turn_end"
    const val TRUNCATED = "truncated"

    const val ROUTE_LIGHT = "light"
    const val ROUTE_FULL = "full"

    /** States in which a chat has a turn nacred is still working on. */
    fun active(state: String) = state == RUNNING || state == WAITING_PERMISSION
}

/** What was on the phone's screen when a message was sent; the `context` object. */
@Serializable
data class ScreenContext(
    /** boards, inbox, activity, board, bead, change, round or run. */
    val screen: String,
    /** Repository id on the chat's computer, or on [elsewhere] when that is set. */
    val repo: String? = null,
    /** The computer holding [repo] when it has no checkout on the chat's computer. */
    val elsewhere: String? = null,
    @SerialName("bead_id") val beadId: String? = null,
    @SerialName("change_id") val changeId: String? = null,
    @SerialName("round_id") val roundId: String? = null,
    @SerialName("run_id") val runId: String? = null,
    val selection: String? = null,
    val pane: String? = null,
    /** Up to [SUMMARY_MAX_BYTES] of what the screen shows. */
    val summary: String? = null,
) {
    companion object {
        const val SUMMARY_MAX_BYTES = 4096
        const val BOARDS = "boards"
        const val INBOX = "inbox"
        const val ACTIVITY = "activity"
        const val BOARD = "board"
        const val BEAD = "bead"
        const val CHANGE = "change"
        const val ROUND = "round"
        const val RUN = "run"
        val ALL_BOARDS_SCREENS = setOf(BOARDS, INBOX, ACTIVITY)
    }
}

/** One chat as nacred keeps it. */
@Serializable
data class ChatRecord(
    val id: String,
    val agent: String = ChatWire.AGENT_CLAUDE,
    /** The repository id the chat started in on its computer; null for all boards. */
    val repo: String? = null,
    @SerialName("host_name") val reportedHostName: String = "",
    val title: String = "",
    @SerialName("new_effort") val newEffort: Boolean = false,
    @SerialName("created_at") val createdAt: String = "",
    @SerialName("updated_at") val updatedAt: String = "",
    val state: String = ChatWire.IDLE,
    /** Why the chat is interrupted or limit_reached. */
    val reason: String = "",
    @SerialName("turn_id") val turnId: String = "",
    @SerialName("last_seq") val lastSeq: Long = 0,
    @SerialName("image_count") val imageCount: Int = 0,
    @SerialName("image_bytes") val imageBytes: Long = 0,
    /** Set by the app: which paired computer this chat is on. */
    @Transient val hostId: String = "",
    @Transient val hostName: String = "",
) {
    /** "<paired host>~<chat id>", unique across computers. */
    val key get() = Keys.of(hostId, id)
    val active get() = ChatWire.active(state)
    val waiting get() = state == ChatWire.WAITING_PERMISSION
}

/** One entry of a chat's event log. */
@Serializable
data class ChatEvent(
    val seq: Long = 0,
    val ts: String = "",
    val kind: String = "",
    @SerialName("turn_id") val turnId: String = "",
    val text: String = "",
    val route: String = "",
    val tool: String = "",
    @SerialName("tool_id") val toolId: String = "",
    val input: JsonElement? = null,
    @SerialName("is_error") val isError: Boolean? = null,
    val context: ScreenContext? = null,
    @SerialName("request_id") val requestId: String = "",
    val allowed: Boolean? = null,
    val reason: String = "",
    /** Who decided a permission_answer: phone, timeout, stop, restart or turn_end. */
    val by: String = "",
)

@Serializable
data class CreateChat(
    val agent: String,
    /** Repository id on the chat's computer; null for all boards. */
    val repo: String?,
    val context: ScreenContext?,
    @SerialName("new_effort") val newEffort: Boolean = false,
)

@Serializable
data class SendChatMessage(
    @SerialName("turn_id") val turnId: String,
    val text: String,
    val context: ScreenContext?,
)

@Serializable
data class ChatSendResult(
    @SerialName("turn_id") val turnId: String = "",
    val started: Boolean = false,
    val chat: ChatRecord? = null,
)

@Serializable
data class PermissionAnswer(val allow: Boolean, val note: String? = null)

/** What answering a permission request came back with; [conflict] when it was already decided (409). */
data class PermissionOutcome(val answer: ChatEvent?, val conflict: Boolean, val chat: ChatRecord? = null)
