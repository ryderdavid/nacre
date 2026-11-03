package com.ryderdavid.nacre.data.claude

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.CoroutineStart
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
    data class Mine(
        val text: String,
        val mode: ChatMode,
        val about: String = "",
        /** References only; the bytes are files in the chat attachment directory. */
        val images: List<ChatImage> = emptyList(),
    ) : BeadChatItem() {
        /** What Claude is sent: the question being discussed, then the message. */
        val prompt get() = if (about.isBlank()) text else "$about\n\n$text"
    }

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

private val StoredThread.imageIds get() = items.flatMap { (it as? BeadChatItem.Mine)?.images.orEmpty() }.map { it.id }

/**
 * Every bead's chat with Claude, kept on disk until Ryder starts a new
 * conversation, and the mode he used last. Replies are awaited on the app's
 * scope, so one still lands in its thread if the sheet or the bead is closed
 * while Claude is thinking. Images attached to messages are saved as files in
 * [attachments]; the threads hold only references, and the files go when
 * their thread does.
 */
class BeadChats(
    private val file: File,
    private val scope: CoroutineScope,
    private val io: CoroutineDispatcher = Dispatchers.IO,
    private val onStarted: (String) -> Unit = {},
    private val onActivity: (String) -> Unit = {},
    private val attachments: ChatAttachments = ChatAttachments(File(file.parentFile, "chat-attachments")),
) {
    private val state = MutableStateFlow(read())
    private val busy = MutableStateFlow<Set<String>>(emptySet())
    private val failures = MutableStateFlow<Map<String, ChatFailure>>(emptyMap())
    private val writing = Mutex()
    private val requests = mutableMapOf<String, Any>()
    private val jobs = mutableMapOf<String, Job>()

    init {
        // Finishes any deletion a previous run didn't get to, and drops files a failed save left behind.
        scope.launch(io) {
            synchronized(this@BeadChats) {
                runCatching { attachments.sweep(state.value.threads.flatMap { it.imageIds }.toSet()) }
            }
        }
    }

    val mode: Flow<ChatMode> = state.map { it.mode }.distinctUntilChanged()

    fun items(key: String): Flow<List<BeadChatItem>> =
        state.map { s -> s.threads.firstOrNull { it.key == key }?.items.orEmpty() }.distinctUntilChanged()

    fun thinking(key: String): Flow<Boolean> = busy.map { key in it }.distinctUntilChanged()
    fun failure(key: String): Flow<ChatFailure?> = failures.map { it[key] }.distinctUntilChanged()

    fun current(key: String): List<BeadChatItem> = state.value.threads.firstOrNull { it.key == key }?.items.orEmpty()

    fun setMode(mode: ChatMode) = change { it.copy(mode = mode) }

    /** Start the bead's conversation over. */
    @Synchronized fun clear(key: String) {
        requests.remove(key)
        jobs.remove(key)?.cancel()
        busy.update { it - key }
        failures.update { it - key }
        val removed = state.value.threads.filter { it.key == key }.flatMap { it.imageIds }
        change({ removed }) { s -> s.copy(threads = s.threads.filter { it.key != key }) }
    }

    /** The bytes of an attached image, or null when its file is missing or unreadable. */
    fun image(image: ChatImage): ByteArray? = attachments.read(image)

    /** The images a request in [key]'s thread carries, given the history it is sent with and [current]'s images. */
    fun requestImages(history: List<DraftTurn>, current: List<ChatImage>): RequestImages =
        resolveImages(history, current, ::image)

    /**
     * Add Ryder's [message] and the reply [answer] produces once it arrives. A
     * failure leaves the message in the thread and offers to send it again.
     *
     * [images] are the bytes of [message]'s images, saved before the message
     * is recorded. If saving fails, the files already written are removed,
     * nothing is recorded or sent, and [ChatAttachmentException] says why.
     */
    @Synchronized fun send(
        key: String, message: BeadChatItem.Mine, retrying: Boolean, images: List<ImagePayload> = emptyList(),
        answer: suspend (history: List<BeadChatItem>) -> BeadChatItem,
    ) {
        if (key in busy.value) return
        attachments.save(images)
        val request = Any()
        requests[key] = request
        val history = current(key)
        onStarted(key)
        failures.update { it - key }
        if (!retrying) edit(key) { it + message }
        busy.update { it + key }
        val job = scope.launch(start = CoroutineStart.LAZY) {
            try {
                val reply = answer(history)
                synchronized(this@BeadChats) {
                    if (requests[key] === request) {
                        edit(key) { it + reply }
                        onActivity(key)
                    }
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: ClaudeUnavailableException) {
                synchronized(this@BeadChats) {
                    if (requests[key] === request) failures.update { it + (key to ChatFailure(e.message ?: "Claude isn't available right now", e.offerApiKey, message)) }
                }
            } catch (e: Exception) {
                synchronized(this@BeadChats) {
                    if (requests[key] === request) failures.update { it + (key to ChatFailure(e.message ?: "Claude couldn't answer", false, message)) }
                }
            } finally {
                synchronized(this@BeadChats) {
                    if (requests[key] === request) {
                        busy.update { it - key }
                        jobs.remove(key)
                        onActivity(key)
                    }
                }
            }
        }
        jobs[key] = job
        job.start()
    }

    /** Replace the item at [index], e.g. to record what became of a proposal. */
    fun set(key: String, index: Int, item: BeadChatItem) {
        onActivity(key)
        edit(key) { list ->
            if (index in list.indices) list.toMutableList().also { it[index] = item } else list
        }
    }

    private fun edit(key: String, change: (List<BeadChatItem>) -> List<BeadChatItem>) {
        var dropped: List<String> = emptyList()
        this.change({ dropped }) { s ->
            val old = s.threads.firstOrNull { it.key == key }
            val thread = StoredThread(key, change(old?.items.orEmpty()), System.currentTimeMillis())
            // Most recently used first; old threads drop off rather than growing the file forever.
            val all = listOf(thread) + s.threads.filter { it.key != key }
            dropped = all.drop(MAX_THREADS).flatMap { it.imageIds }
            s.copy(threads = all.take(MAX_THREADS))
        }
    }

    private fun change(block: (StoredChats) -> StoredChats) = change({ emptyList() }, block)

    /**
     * Applies [block] and saves. The attachment files [removed] names belong to
     * threads [block] dropped; they are deleted only once the file no longer
     * refers to them, and a load sweeps up any that a failed save left.
     */
    private fun change(removed: () -> List<String>, block: (StoredChats) -> StoredChats) {
        state.update(block)
        val ids = removed()
        scope.launch(io) {
            val saved = writing.withLock {
                runCatching {
                    val tmp = File(file.path + ".tmp")
                    tmp.writeText(ClaudeService.json.encodeToString(StoredChats.serializer(), state.value))
                    if (!tmp.renameTo(file)) error("couldn't replace ${file.name}")
                }.isSuccess
            }
            if (saved) runCatching { attachments.delete(ids) }
        }
    }

    private fun read(): StoredChats =
        runCatching { ClaudeService.json.decodeFromString(StoredChats.serializer(), file.readText()) }.getOrDefault(StoredChats())

    companion object {
        const val MAX_THREADS = 60

        fun key(repoKey: String, beadId: String) = "$repoKey/$beadId"
    }
}

/**
 * Ryder's message for a bead's thread: [text] in [mode] with references to
 * [images]. Null when there is nothing to send: an Ask needs text or an image,
 * while an empty Refine means "make it agent-ready".
 */
fun beadChatMessage(text: String, mode: ChatMode, images: List<ImagePayload> = emptyList()): BeadChatItem.Mine? {
    val message = text.trim()
    if (message.isEmpty() && images.isEmpty() && mode == ChatMode.ASK) return null
    return BeadChatItem.Mine(message, mode, images = images.map { it.image })
}

/** The thread as the turns a prompt carries: Ryder's messages, Claude's answers, and what became of each proposal. */
fun List<BeadChatItem>.turns(): List<DraftTurn> = mapNotNull { item ->
    when (item) {
        is BeadChatItem.Mine -> DraftTurn(true, if (item.mode == ChatMode.ASK) "(asking) ${item.prompt}" else "(asking for changes) ${item.prompt}", item.images)
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
