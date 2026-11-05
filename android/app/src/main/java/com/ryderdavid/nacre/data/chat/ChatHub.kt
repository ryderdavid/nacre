package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.data.claude.ChatStack
import com.ryderdavid.nacre.data.claude.TurnLostException
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.data.model.ChatWire
import com.ryderdavid.nacre.data.model.ScreenContext
import com.ryderdavid.nacre.data.model.SendChatMessage
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.util.UUID

/** The reason a chat at its runaway ceiling gives when nacred didn't send one. */
const val LIMIT_REASON = "This chat reached its runaway limit. Start a new chat to keep going."

/** One conversation as the Chat screen shows it. */
data class ConversationState(
    val chat: ChatRecord? = null,
    val items: List<ChatItem> = emptyList(),
    val loading: Boolean = true,
    /** A message is on its way to nacred. */
    val sending: Boolean = false,
    val error: String? = null,
    /** The stream dropped and the app is reattaching. */
    val reconnecting: Boolean = false,
    /** The newest seq shown. */
    val lastSeq: Long = 0,
) {
    /** A turn is running: Send is disabled and Stop is offered. */
    val turnRunning get() = sending || chat?.active == true
    val limitReached get() = chat?.state == ChatWire.LIMIT_REACHED
    val limitReason: String? get() = if (limitReached) chat?.reason?.ifBlank { null } ?: LIMIT_REASON else null
    val canSend get() = chat != null && !turnRunning && !limitReached
    val pendingCards get() = items.filterIsInstance<ChatItem.Permission>().filter { it.card.pending }.map { it.card }
}

/** Posts and clears "Chat needs your OK" notifications. */
interface ChatNotices {
    fun post(chat: ChatRecord, card: PermissionCard)
    fun clear(chatKey: String, requestId: String)
}

/**
 * One chat's conversation: loads the record and its log, follows the stream while a turn runs,
 * sends messages and answers permission requests. Events are shown once each by seq.
 */
class ChatConversation(
    val key: String,
    private val link: ChatLink,
    private val scope: CoroutineScope,
    private val notices: ChatNotices? = null,
    private val newTurnId: () -> String = { UUID.randomUUID().toString() },
    private val backoff: () -> Backoff = { Backoff() },
) {
    private val transcript = ChatTranscript()
    private val stateFlow = MutableStateFlow(ConversationState())
    val state: StateFlow<ConversationState> = stateFlow.asStateFlow()
    private var follow: Job? = null
    private var loaded = false

    /** Every seq taken from nacred, in order. */
    val seqs: List<Long> get() = transcript.seqs.toList()

    private fun show(chat: ChatRecord? = stateFlow.value.chat, change: (ConversationState) -> ConversationState = { it }) {
        synchronized(transcript) {
            stateFlow.update { change(it.copy(chat = chat, items = transcript.shown, lastSeq = transcript.lastSeq)) }
        }
    }

    private fun take(ev: com.ryderdavid.nacre.data.model.ChatEvent) {
        synchronized(transcript) { transcript.add(ev) }
        if (ev.kind == ChatWire.PERMISSION_ANSWER) notices?.clear(key, ev.requestId)
        show()
    }

    /** Loads the chat (once) and follows it if a turn is running. */
    fun open() {
        if (loaded && follow?.isActive == true) return
        scope.launch { runCatching { load() }.onFailure { e -> if (e is CancellationException) throw e; show { it.copy(loading = false, error = e.friendly()) } } }
    }

    /** Reads the record and its newest events, then follows a running turn. */
    suspend fun load() {
        val d = link.chatDetail(key)
        synchronized(transcript) { transcript.addAll(d.events) }
        loaded = true
        show(d.chat.withHost()) { it.copy(loading = false, error = null) }
        if (d.chat.active) startFollow()
    }

    private fun ChatRecord.withHost() = stateFlow.value.chat?.let { copy(hostId = it.hostId, hostName = it.hostName) }
        ?: copy(hostId = com.ryderdavid.nacre.data.model.Keys.host(key))

    /** Follows the stream from the last event shown until the chat settles. */
    fun startFollow(): Job {
        follow?.takeIf { it.isActive }?.let { return it }
        return scope.launch { followNow() }.also { follow = it }
    }

    /** Follows the stream from the last event shown until the chat settles; for tests and [startFollow]. */
    suspend fun followNow() {
        try {
            val end = followChat(
                open = { after -> link.chatStream(key, after) },
                after = transcript.lastSeq,
                onEvent = ::take,
                backoff = backoff(),
                onDrop = { show { it.copy(reconnecting = true) } },
                onBack = { show { it.copy(reconnecting = false) } },
            )
            when (end) {
                is ChatFollowEnd.Settled -> show(end.chat.withHost()) { it.copy(reconnecting = false) }
                is ChatFollowEnd.Gone -> show { it.copy(reconnecting = false, error = end.reason) }
            }
        } catch (e: CancellationException) {
            throw e
        } catch (e: TurnLostException) {
            show { it.copy(reconnecting = false, error = "Couldn't reconnect to the computer. Open the chat again to pick it back up.") }
        } catch (e: Exception) {
            show { it.copy(reconnecting = false, error = e.friendly()) }
        }
    }

    /** Sends [text] with [context] on the background scope. */
    fun send(text: String, context: ScreenContext?) {
        scope.launch { sendNow(text, context) }
    }

    /**
     * Sends one message and follows its turn. A lost response is resent with the same `turn_id`,
     * which nacred recognises, so no second turn starts. Returns the turn id, or null on failure.
     */
    suspend fun sendNow(text: String, context: ScreenContext?): String? {
        if (!stateFlow.value.canSend && stateFlow.value.chat != null) return null
        val turnId = newTurnId()
        show { it.copy(sending = true, error = null) }
        return try {
            val res = sendWithResend(backoff()) { link.sendChat(key, SendChatMessage(turnId, text, context)) }
            show(res.chat?.withHost() ?: stateFlow.value.chat?.copy(state = ChatWire.RUNNING)) { it.copy(sending = false) }
            startFollow()
            turnId
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            show { it.copy(sending = false, error = e.friendly()) }
            null
        }
    }

    /** Stops the running turn; nacred denies any pending request. */
    fun stop() {
        scope.launch { runCatching { link.cancelChat(key) }.onFailure { e -> if (e is CancellationException) throw e; show { it.copy(error = e.friendly()) } } }
    }

    fun answer(requestId: String, allow: Boolean, note: String?) {
        scope.launch { answerNow(requestId, allow, note) }
    }

    /**
     * Answers a permission request. Once decided, timed out or answered elsewhere (409), the card
     * shows the recorded outcome and its notification is cleared.
     */
    suspend fun answerNow(requestId: String, allow: Boolean, note: String?) {
        try {
            val out = link.answerPermission(key, requestId, allow, note)
            out.answer?.let { synchronized(transcript) { transcript.settle(it) } }
            if (out.answer == null && out.conflict) {
                // nacred knew the answer but didn't send it: read the log again for it.
                val d = link.chatDetail(key)
                synchronized(transcript) { transcript.addAll(d.events) }
                show(d.chat.withHost())
            } else {
                show(out.chat?.withHost() ?: stateFlow.value.chat)
            }
            notices?.clear(key, requestId)
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            show { it.copy(error = e.friendly()) }
        }
    }

    fun dismissError() = show { it.copy(error = null) }

    fun close() {
        follow?.cancel()
        follow = null
    }
}

/**
 * Background watching for Chat: while any chat on a paired computer is running or waiting for
 * permission, follows its stream with the `followTurn` backoff and posts one "Chat needs your OK"
 * per permission request when the app is in the background. Request ids are remembered, so a
 * reattach never posts the same request twice. A chat is dropped once it is idle, interrupted or
 * limit_reached.
 */
class ChatWatch(
    private val link: ChatLink,
    private val scope: CoroutineScope,
    private val notices: ChatNotices,
    private val inBackground: () -> Boolean,
    private val backoff: () -> Backoff = { Backoff() },
) {
    private val watchingState = MutableStateFlow<Set<String>>(emptySet())

    /** Keys of the chats being watched; the run watcher keeps running while this isn't empty. */
    val watching: StateFlow<Set<String>> = watchingState.asStateFlow()

    private val jobs = mutableMapOf<String, Job>()
    private val posted = mutableSetOf<String>()

    /** Learns the active chats from the chat list again: when the app goes to the background and on refresh. */
    suspend fun refresh() {
        val listing = runCatching { link.refreshChats() }.getOrElse { e -> if (e is CancellationException) throw e; return }
        learn(listing)
    }

    /** Starts watching every running or waiting chat in [listing] not watched yet. */
    fun learn(listing: ChatListing) {
        listing.chats.filter { it.active }.forEach(::watch)
    }

    @Synchronized private fun watch(chat: ChatRecord) {
        if (jobs[chat.key]?.isActive == true) return
        watchingState.update { it + chat.key }
        jobs[chat.key] = scope.launch {
            try {
                follow(chat)
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                // Unreachable or refused: dropped until the next refresh finds it again.
            } finally {
                synchronized(this@ChatWatch) { jobs.remove(chat.key) }
                watchingState.update { it - chat.key }
            }
        }
    }

    private suspend fun follow(chat: ChatRecord) {
        val t = ChatTranscript()
        val d = link.chatDetail(chat.key)
        t.addAll(d.events)
        announce(chat, t)
        if (!d.chat.active) return
        followChat(
            open = { after -> link.chatStream(chat.key, after) },
            after = t.lastSeq,
            onEvent = { ev ->
                t.add(ev)
                if (ev.kind == ChatWire.PERMISSION_ANSWER) notices.clear(chat.key, ev.requestId)
                announce(chat, t)
            },
            backoff = backoff(),
        )
    }

    private fun announce(chat: ChatRecord, t: ChatTranscript) {
        if (!inBackground()) return
        t.pending.forEach { card ->
            val id = chat.key + "/" + card.requestId
            val fresh = synchronized(posted) { posted.add(id) }
            if (fresh) notices.post(chat, card)
        }
    }

    /** Stops watching everything. */
    fun stopAll() {
        synchronized(this) { jobs.values.forEach { it.cancel() } }
    }
}

/** Whether the run watcher should keep running: runs, proposals being worked on, or chats to watch. */
fun watcherShouldRun(activeRuns: Int, workingChanges: Int, watchedChats: Int): Boolean =
    activeRuns > 0 || workingChanges > 0 || watchedChats > 0

/**
 * Chat for the whole app: conversations by chat key, the one current chat with its view state
 * ([stack]), the screen context holder and the background watch.
 */
class ChatHub(
    val link: ChatLink,
    private val scope: CoroutineScope,
    val notices: ChatNotices,
    inBackground: () -> Boolean,
    /** The app-wide current chat, how it is shown, and each chat's draft and pending images. */
    val stack: ChatStack = ChatStack(),
) {
    val screens = ScreenContextHolder()
    val watch = ChatWatch(link, scope, notices, inBackground)
    private val conversations = mutableMapOf<String, ChatConversation>()

    /** The current chat: the Chat tab shows it full screen, ✨ and the bead entry open it. */
    val current: StateFlow<String?> get() = stack.current

    /** A permission request a notification opened the chat at. */
    val focus = MutableStateFlow<String?>(null)

    @Synchronized fun conversation(key: String): ChatConversation =
        conversations.getOrPut(key) { ChatConversation(key, link, scope, notices) }

    /** Makes [key] the current chat (the Chat tab shows it), at the card for [requestId] when given. */
    fun openChat(key: String, requestId: String? = null) {
        stack.select(key)
        focus.value = requestId
    }

    /** Sets the first message's text of a new chat, such as the note New carries from a chat at its limit. */
    fun prefill(key: String, text: String) = stack.updateView(key) { it.copy(draft = text) }

    /**
     * Deletes the chat on its computer. Its pane or sheet closes, its draft and pending images
     * are dropped, and it leaves the header menu and the Chat list.
     */
    suspend fun delete(key: String) {
        link.deleteChat(key)
        synchronized(this) { conversations.remove(key)?.close() }
        stack.deleted(key)
    }
}
