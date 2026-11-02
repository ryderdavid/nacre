package com.ryderdavid.nacre.data.claude

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow

/** Identity deliberately excludes the display name: renaming a bead must not create a chat. */
sealed interface ChatContext {
    data object Multi : ChatContext
    data class Board(val repoKey: String) : ChatContext
    data class Bead(val repoKey: String, val beadId: String) : ChatContext
}

data class ChatPresentation(val selected: ChatContext? = null, val collapsed: Boolean = false)
data class ChatViewState(val draft: String = "", val firstVisibleIndex: Int = 0, val firstVisibleOffset: Int = 0)

data class ActiveChat(val context: ChatContext, val label: String, val lastActivity: Long)
data class ChatSlot(val context: ChatContext, val label: String, val isNew: Boolean)

/** Process-owned conversations. Viewing, opening and reordering never start or refresh a chat. */
class ChatStack(
    private val now: () -> Long = System::currentTimeMillis,
    private val onRemoved: (ChatContext) -> Unit = {},
) {
    private val state = MutableStateFlow<List<ActiveChat>>(emptyList())
    val active = state.asStateFlow()

    private val presentationState = MutableStateFlow<Map<ChatContext, ChatPresentation>>(emptyMap())
    val presentations = presentationState.asStateFlow()
    private val viewState = MutableStateFlow<Map<ChatContext, ChatViewState>>(emptyMap())
    val views = viewState.asStateFlow()

    fun presentation(screen: ChatContext) = presentationState.value[screen] ?: ChatPresentation()
    fun view(context: ChatContext) = viewState.value[context] ?: ChatViewState()
    @Synchronized fun select(screen: ChatContext, selected: ChatContext) {
        presentationState.value = presentationState.value + (screen to ChatPresentation(selected))
    }
    @Synchronized fun collapse(screen: ChatContext) {
        if (presentation(screen).selected == null) return
        presentationState.value = presentationState.value + (screen to presentation(screen).copy(collapsed = true))
    }
    @Synchronized fun expand(screen: ChatContext) {
        presentationState.value = presentationState.value + (screen to presentation(screen).copy(collapsed = false))
    }
    @Synchronized fun close(screen: ChatContext) {
        presentationState.value = presentationState.value - screen
    }
    @Synchronized fun updateView(context: ChatContext, change: (ChatViewState) -> ChatViewState) {
        if (!contains(context) && presentationState.value.values.none { it.selected == context }) return
        viewState.value = viewState.value + (context to change(view(context)))
    }
    private fun clearView(context: ChatContext) {
        viewState.value = viewState.value - context
        presentationState.value = presentationState.value.filterValues { it.selected != context }
    }

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
        synchronized(this) {
            state.value = state.value.filterNot { it.context == context }
            clearView(context)
        }
        onRemoved(context)
    }

    fun expire() {
        val expired = synchronized(this) {
            state.value.filter { now() - it.lastActivity >= IDLE_MS }.also {
                state.value = state.value - it.toSet()
                it.forEach { chat -> clearView(chat.context) }
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
