package com.ryderdavid.nacre.data.claude

import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.io.File

/** What a message in a bead's chat asks for: an answer, or edits to review. */
@Serializable
enum class ChatMode { ASK, REFINE }

/** One entry in a bead's chat with Claude. */
@Serializable
sealed class BeadChatItem {
    @Serializable @SerialName("mine")
    data class Mine(val text: String, val mode: ChatMode) : BeadChatItem()

    @Serializable @SerialName("answer")
    data class Answer(val text: String) : BeadChatItem()

    /** Edits Claude proposed; nothing changes until Ryder applies them. */
    @Serializable @SerialName("proposal")
    data class Proposal(val proposal: RefineProposal, val outcome: Outcome = Outcome.PENDING, val applied: Int = 0) : BeadChatItem()

    enum class Outcome { PENDING, APPLIED, DISCARDED }
}

/** Why the last message in a thread got no reply, and what Ryder can do about it. */
data class ChatFailure(val message: String, val offerApiKey: Boolean, val retry: BeadChatItem.Mine)

@Serializable
private data class StoredThread(val key: String, val items: List<BeadChatItem>, val updated: Long)

@Serializable
private data class StoredChats(val mode: ChatMode = ChatMode.REFINE, val threads: List<StoredThread> = emptyList())

/**
 * Every bead's chat with Claude, kept on disk until Ryder starts a new
 * conversation, and the mode he used last. Replies are awaited on the app's
 * scope, so one still lands in its thread if the sheet or the bead is closed
 * while Claude is thinking.
 */
class BeadChats(
    private val file: File,
    private val scope: CoroutineScope,
    private val io: CoroutineDispatcher = Dispatchers.IO,
) {
    private val state = MutableStateFlow(read())
    private val busy = MutableStateFlow<Set<String>>(emptySet())
    private val failures = MutableStateFlow<Map<String, ChatFailure>>(emptyMap())
    private val writing = Mutex()

    val mode: Flow<ChatMode> = state.map { it.mode }.distinctUntilChanged()

    fun items(key: String): Flow<List<BeadChatItem>> =
        state.map { s -> s.threads.firstOrNull { it.key == key }?.items.orEmpty() }.distinctUntilChanged()

    fun thinking(key: String): Flow<Boolean> = busy.map { key in it }.distinctUntilChanged()
    fun failure(key: String): Flow<ChatFailure?> = failures.map { it[key] }.distinctUntilChanged()

    fun current(key: String): List<BeadChatItem> = state.value.threads.firstOrNull { it.key == key }?.items.orEmpty()

    fun setMode(mode: ChatMode) = change { it.copy(mode = mode) }

    /** Start the bead's conversation over. */
    fun clear(key: String) {
        failures.update { it - key }
        change { s -> s.copy(threads = s.threads.filter { it.key != key }) }
    }

    /**
     * Add Ryder's [message] and the reply [answer] produces once it arrives. A
     * failure leaves the message in the thread and offers to send it again.
     */
    fun send(key: String, message: BeadChatItem.Mine, retrying: Boolean, answer: suspend (history: List<BeadChatItem>) -> BeadChatItem) {
        if (key in busy.value) return
        val history = current(key)
        failures.update { it - key }
        if (!retrying) edit(key) { it + message }
        busy.update { it + key }
        scope.launch {
            try {
                val reply = answer(history)
                edit(key) { it + reply }
            } catch (e: ClaudeUnavailableException) {
                failures.update { it + (key to ChatFailure(e.message ?: "Claude isn't available right now", e.offerApiKey, message)) }
            } catch (e: Exception) {
                failures.update { it + (key to ChatFailure(e.message ?: "Claude couldn't answer", false, message)) }
            } finally {
                busy.update { it - key }
            }
        }
    }

    /** Replace the item at [index], e.g. to record what became of a proposal. */
    fun set(key: String, index: Int, item: BeadChatItem) = edit(key) { list ->
        if (index in list.indices) list.toMutableList().also { it[index] = item } else list
    }

    private fun edit(key: String, change: (List<BeadChatItem>) -> List<BeadChatItem>) = change { s ->
        val old = s.threads.firstOrNull { it.key == key }
        val thread = StoredThread(key, change(old?.items.orEmpty()), System.currentTimeMillis())
        // Most recently used first; old threads drop off rather than growing the file forever.
        s.copy(threads = (listOf(thread) + s.threads.filter { it.key != key }).take(MAX_THREADS))
    }

    private fun change(block: (StoredChats) -> StoredChats) {
        state.update(block)
        scope.launch(io) {
            writing.withLock {
                runCatching {
                    val tmp = File(file.path + ".tmp")
                    tmp.writeText(ClaudeService.json.encodeToString(StoredChats.serializer(), state.value))
                    tmp.renameTo(file)
                }
            }
        }
    }

    private fun read(): StoredChats =
        runCatching { ClaudeService.json.decodeFromString(StoredChats.serializer(), file.readText()) }.getOrDefault(StoredChats())

    companion object {
        const val MAX_THREADS = 60

        fun key(repoKey: String, beadId: String) = "$repoKey/$beadId"
    }
}

/** The thread as the turns a prompt carries: Ryder's messages, Claude's answers, and what became of each proposal. */
fun List<BeadChatItem>.turns(): List<DraftTurn> = mapNotNull { item ->
    when (item) {
        is BeadChatItem.Mine -> DraftTurn(true, if (item.mode == ChatMode.ASK) "(asking) ${item.text}" else "(asking for changes) ${item.text}")
        is BeadChatItem.Answer -> DraftTurn(false, item.text)
        is BeadChatItem.Proposal -> DraftTurn(
            false,
            "Proposed changes: ${item.proposal.summary.ifBlank { "edits to the bead" }} " + when (item.outcome) {
                BeadChatItem.Outcome.APPLIED -> "(Ryder applied ${item.applied}; the bead above already includes them)"
                BeadChatItem.Outcome.DISCARDED -> "(Ryder discarded them)"
                BeadChatItem.Outcome.PENDING -> "(not yet reviewed)"
            },
        )
    }
}
