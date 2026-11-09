package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.data.model.ChatEvent
import com.ryderdavid.nacre.data.model.ChatWire
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/** How a permission request stands, as its card shows it. */
enum class CardState { PENDING, ALLOWED, DENIED, TIMED_OUT, CLOSED }

/** One permission request in a conversation. */
data class PermissionCard(
    val requestId: String,
    val tool: String,
    /** The step: the command, the file path, or the MCP tool and server. */
    val step: String,
    /** The classifier's reason, when it gave one. */
    val reason: String,
    val state: CardState = CardState.PENDING,
    /** The deny note or the reason it was closed. */
    val outcome: String = "",
) {
    val pending get() = state == CardState.PENDING

    /** The read-only line a decided card shows. */
    val outcomeLine: String
        get() = when (state) {
            CardState.PENDING -> ""
            CardState.ALLOWED -> "Allowed"
            CardState.DENIED -> if (outcome.isBlank()) "Denied" else "Denied: $outcome"
            CardState.TIMED_OUT -> outcome.ifBlank { "No answer from the phone within 15 minutes" }.let { "Denied: $it" }
            CardState.CLOSED -> if (outcome.isBlank()) "Closed" else "Denied: $outcome"
        }
}

/** One thing the conversation shows. */
sealed interface ChatItem {
    val key: String

    /** A message Ryder sent, with the images it carried ([images], in order) for its thumbnails. */
    data class User(
        override val key: String,
        val text: String,
        val about: String?,
        val images: List<com.ryderdavid.nacre.data.model.ChatImageRef> = emptyList(),
    ) : ChatItem
    data class Assistant(override val key: String, val text: String, val route: String) : ChatItem
    data class ToolStep(
        override val key: String,
        val tool: String,
        val summary: String,
        val result: String? = null,
        val failed: Boolean = false,
    ) : ChatItem
    /** "Switching to the full agent: <reason>". */
    data class Handoff(override val key: String, val text: String) : ChatItem
    data class Permission(override val key: String, val card: PermissionCard) : ChatItem
    data class Note(override val key: String, val text: String, val error: Boolean) : ChatItem
    /**
     * A New effort draft, recorded as event [seq]; only the newest ([latest]) is shown, as a Draft card
     * whose "Where does this belong?" raises intake from that event.
     */
    data class Draft(override val key: String, val draft: com.ryderdavid.nacre.data.model.ChatDraft, val latest: Boolean = true, val seq: Long = 0) : ChatItem
    /** Where the agent asked about decision [decisionId]: the conversation shows its receipt here. */
    data class Ask(override val key: String, val decisionId: String) : ChatItem
    /** A new chat's heads-up from nacred: [text] says what waits, [total] how many. */
    data class HeadsUp(override val key: String, val text: String, val total: Int) : ChatItem
}

/** How Ryder's /needs-you message reads in the conversation. */
const val NEEDS_YOU_SHOWN = "Go through what needs me"

/** The handoff line, as the conversation shows it. */
fun handoffLine(event: ChatEvent): String {
    val t = event.text.trim()
    return when {
        t.startsWith(HANDED_OFF) || t.startsWith(OLD_HANDED_OFF) -> t
        t.isNotEmpty() -> "$HANDED_OFF: $t"
        event.reason.isNotBlank() -> "$HANDED_OFF: ${event.reason.trim()}"
        else -> HANDED_OFF
    }
}

const val HANDED_OFF = "Switching to the full agent"

/** What older bridges wrote for a handoff; their lines are shown as they came. */
private const val OLD_HANDED_OFF = "Handed to the full harness"

/** A one-line summary of a tool call's input: the command, the file, or the pattern. */
fun toolSummary(tool: String, input: kotlinx.serialization.json.JsonElement?): String {
    val o = input as? JsonObject ?: return tool
    fun s(k: String) = (o[k] as? JsonPrimitive)?.content?.takeIf { it.isNotBlank() }
    val detail = s("command") ?: s("file_path") ?: s("path") ?: s("pattern") ?: s("url") ?: s("query") ?: s("description")
    return if (detail == null) tool else "$tool · ${detail.lineSequence().first().take(200)}"
}

/**
 * A chat's log as the conversation shows it. Events are taken once each, by seq, so replaying
 * the stream after a reattach neither repeats nor loses anything.
 */
class ChatTranscript {
    private val items = mutableListOf<ChatItem>()
    private val tools = mutableMapOf<String, Int>()
    private val cards = mutableMapOf<String, Int>()
    /** Where the newest draft sits in [items]. */
    private var draftAt: Int? = null

    /** The newest seq taken; what a reattach asks to resume after. */
    var lastSeq: Long = 0
        private set

    /** Every event taken, in order; for tests and reattach checks. */
    val seqs = mutableListOf<Long>()

    val shown: List<ChatItem> get() = items.toList()

    /** When each shown item's event was logged (its key to the event's ts), to place cards beside it. */
    private val stamps = mutableMapOf<String, String>()
    val times: Map<String, String> get() = stamps.toMap()

    /** The newest draft the agent wrote, for the Draft card. */
    val newestDraft: com.ryderdavid.nacre.data.model.ChatDraft? get() = draftAt?.let { (items[it] as ChatItem.Draft).draft }

    val cardsById: Map<String, PermissionCard>
        get() = cards.mapValues { (_, i) -> (items[i] as ChatItem.Permission).card }

    /** Requests still waiting for an answer. */
    val pending: List<PermissionCard> get() = cardsById.values.filter { it.pending }

    /** Takes [events] in order; returns how many were new. */
    fun addAll(events: List<ChatEvent>): Int = events.count(::add)

    /** Takes one event; false when it was already taken. */
    fun add(ev: ChatEvent): Boolean {
        if (ev.seq != 0L && ev.seq <= lastSeq) return false
        if (ev.seq != 0L) {
            lastSeq = ev.seq
            seqs += ev.seq
        }
        val key = "e${ev.seq}"
        if (ev.ts.isNotBlank()) stamps.putIfAbsent(key, ev.ts)
        when (ev.kind) {
            ChatWire.USER -> items += ChatItem.User(key, if (ev.text.trim().startsWith(ChatWire.NEEDS_YOU_COMMAND)) NEEDS_YOU_SHOWN else ev.text, ev.context?.let { ctxLabel(it) }, ev.images)
            ChatWire.ASK -> if (ev.text.isNotBlank()) items += ChatItem.Ask(key, ev.text)
            ChatWire.NEEDS_YOU -> if (ev.text.isNotBlank()) items += ChatItem.HeadsUp(key, ev.text, ev.total)
            ChatWire.TEXT -> {
                val last = items.lastOrNull()
                if (last is ChatItem.Assistant && last.route == ev.route) {
                    items[items.lastIndex] = last.copy(text = last.text + ev.text)
                } else if (ev.text.isNotBlank()) {
                    items += ChatItem.Assistant(key, ev.text, ev.route)
                }
            }
            ChatWire.TOOL_USE -> {
                tools[ev.toolId.ifBlank { key }] = items.size
                items += ChatItem.ToolStep(key, ev.tool, toolSummary(ev.tool, ev.input))
            }
            ChatWire.TOOL_RESULT -> tools[ev.toolId]?.let { i ->
                val step = items[i] as ChatItem.ToolStep
                items[i] = step.copy(result = ev.text, failed = ev.isError == true)
            }
            ChatWire.HANDOFF -> items += ChatItem.Handoff(key, handoffLine(ev))
            ChatWire.PERMISSION -> {
                if (ev.requestId in cards) return true
                cards[ev.requestId] = items.size
                items += ChatItem.Permission(key, PermissionCard(ev.requestId, ev.tool, ev.text.ifBlank { ev.tool }, ev.reason))
            }
            ChatWire.PERMISSION_ANSWER -> settle(ev)
            ChatWire.ERROR -> items += ChatItem.Note(key, ev.text.ifBlank { "Something went wrong on the computer." }, error = true)
            ChatWire.TRUNCATED -> items += ChatItem.Note(key, ev.text.ifBlank { "Earlier messages were trimmed." }, error = false)
            ChatWire.SYSTEM -> if (ev.text.isNotBlank()) items += ChatItem.Note(key, ev.text, error = false)
            ChatWire.DRAFT -> ev.draft?.takeIf { it.title.isNotBlank() }?.let { d ->
                draftAt?.let { i -> items[i] = (items[i] as ChatItem.Draft).copy(latest = false) }
                draftAt = items.size
                items += ChatItem.Draft(key, d, seq = ev.seq)
            }
            else -> Unit // thinking, result and turn_end aren't shown here
        }
        return true
    }

    /** Records how a request was decided; a card already decided keeps its outcome. */
    fun settle(answer: ChatEvent) {
        val i = cards[answer.requestId] ?: return
        val item = items[i] as ChatItem.Permission
        if (!item.card.pending) return
        items[i] = item.copy(card = item.card.copy(state = answerState(answer), outcome = answer.reason.ifBlank { noteOf(answer.text) }))
    }

    private fun noteOf(text: String) = text.removePrefix("Allowed").removePrefix("Denied:").removePrefix("Denied").trim()

    private fun ctxLabel(c: com.ryderdavid.nacre.data.model.ScreenContext): String? = when {
        c.screen in com.ryderdavid.nacre.data.model.ScreenContext.ALL_BOARDS_SCREENS -> com.ryderdavid.nacre.ui.ScreenOrigin.ALL_BOARDS
        c.beadId != null -> c.beadId
        c.roundId != null -> "round ${c.roundId}"
        c.runId != null -> "run ${c.runId}"
        else -> c.repo
    }
}

/** A permission answer's card state. */
fun answerState(answer: ChatEvent): CardState = when {
    answer.allowed == true -> CardState.ALLOWED
    answer.by == "timeout" -> CardState.TIMED_OUT
    answer.by == "stop" || answer.by == "restart" || answer.by == "turn_end" -> CardState.CLOSED
    else -> CardState.DENIED
}
