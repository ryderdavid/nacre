package com.ryderdavid.nacre.data.claude

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow

/** Identity deliberately excludes the display name: renaming a bead must not create a chat. */
sealed interface ChatContext {
    data object Multi : ChatContext
    data class Board(val repoKey: String) : ChatContext
    data class Bead(val repoKey: String, val beadId: String) : ChatContext
}

data class ActiveChat(val context: ChatContext, val label: String, val lastActivity: Long)
data class ChatSlot(val context: ChatContext, val label: String, val isNew: Boolean)

/** Process-owned conversations. Viewing, opening and reordering never start or refresh a chat. */
class ChatStack(
    private val now: () -> Long = System::currentTimeMillis,
    private val onRemoved: (ChatContext) -> Unit = {},
) {
    private val state = MutableStateFlow<List<ActiveChat>>(emptyList())
    val active = state.asStateFlow()

    /** Called only after the backend accepts the first user message. */
    @Synchronized fun started(context: ChatContext, label: String) {
        val chat = ActiveChat(context, label, now())
        state.value = listOf(chat) + state.value.filterNot { it.context == context }
    }

    /** Replies, tool progress and user decisions keep an existing conversation alive. */
    @Synchronized fun touch(context: ChatContext) {
        val chat = state.value.firstOrNull { it.context == context } ?: return
        state.value = listOf(chat.copy(lastActivity = now())) + state.value.filterNot { it.context == context }
    }

    fun dismiss(context: ChatContext) {
        synchronized(this) { state.value = state.value.filterNot { it.context == context } }
        onRemoved(context)
    }

    fun expire() {
        val expired = synchronized(this) {
            state.value.filter { now() - it.lastActivity >= IDLE_MS }.also {
                state.value = state.value - it.toSet()
            }
        }
        expired.forEach { onRemoved(it.context) }
    }

    /** The current screen always owns the first slot; other chats follow by recent activity. */
    fun slots(context: ChatContext, label: String): List<ChatSlot> {
        val chats = state.value
        val current = chats.firstOrNull { it.context == context }
        return listOf(ChatSlot(context, current?.label ?: label, current == null)) +
            chats.filterNot { it.context == context }.map { ChatSlot(it.context, it.label, false) }
    }

    fun shouldExpand(context: ChatContext) = state.value.any { it.context != context }
    fun contains(context: ChatContext) = state.value.any { it.context == context }

    companion object { const val IDLE_MS = 30 * 60 * 1000L }
}
