package com.ryderdavid.nacre.data.claude

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * How the current chat is shown beside the screens, the same on every screen: [shown] is false
 * when its pane or sheet is closed, and [collapsed] leaves only the edge tab on wide layouts.
 */
data class ChatPresentation(val shown: Boolean = false, val collapsed: Boolean = false)

/**
 * What a chat's composer and list hold while the chat is away: the draft, the images pending
 * with it (in memory only), the scroll position and the panel shown beside it.
 */
data class ChatViewState(
    val draft: String = "",
    val firstVisibleIndex: Int = 0,
    val firstVisibleOffset: Int = 0,
    val images: List<ImagePayload> = emptyList(),
    /** The panel (Chat, Spec or Beads destination name) last shown with this chat, restored on return. */
    val panel: String? = null,
)

/**
 * The one chat in the app: the current chat's key (persisted through [persist]), whether its
 * pane or sheet is shown or collapsed, and a [ChatViewState] for each chat key. Opening, closing
 * and collapsing never start, end or delete a chat.
 */
class ChatStack(private val persist: (String?) -> Unit = {}) {
    private val currentState = MutableStateFlow<String?>(null)
    /** The app-wide current chat, or null when there is none (✨ then offers New). */
    val current: StateFlow<String?> = currentState.asStateFlow()

    private val presentationState = MutableStateFlow(ChatPresentation())
    val presentation: StateFlow<ChatPresentation> = presentationState.asStateFlow()

    private val viewState = MutableStateFlow<Map<String, ChatViewState>>(emptyMap())
    val views: StateFlow<Map<String, ChatViewState>> = viewState.asStateFlow()

    /** Keys of deleted chats: a late update can't bring their draft back. */
    private val gone = mutableSetOf<String>()

    private val deletedState = MutableStateFlow<Set<String>>(emptySet())
    /** Chats deleted in this run of the app, so a workspace showing one can move to another. */
    val deleted: StateFlow<Set<String>> = deletedState.asStateFlow()

    /** What the pane or sheet shows now, if anything. */
    fun shownChat(): String? = currentState.value?.takeIf { presentationState.value.shown }

    /** Restores the current chat remembered in settings, unless one was chosen since. */
    @Synchronized fun restore(key: String?) {
        if (key == null || currentState.value != null || key in gone) return
        currentState.value = key
    }

    /** Makes [key] the current chat without changing how it is shown (the Chat tab, a notification). */
    @Synchronized fun select(key: String) {
        gone -= key
        if (currentState.value != key) {
            currentState.value = key
            persist(key)
        }
    }

    /** Makes [key] current and shows it expanded in the pane or the sheet. */
    @Synchronized fun open(key: String) {
        select(key)
        presentationState.value = ChatPresentation(shown = true, collapsed = false)
    }

    /** ✨ and the bead's chat entry: shows the current chat, or returns false when there is none. */
    @Synchronized fun openCurrent(): Boolean {
        val key = currentState.value ?: return false
        open(key)
        return true
    }

    @Synchronized fun collapse() {
        if (!presentationState.value.shown) return
        presentationState.value = presentationState.value.copy(collapsed = true)
    }

    @Synchronized fun expand() {
        if (currentState.value == null) return
        presentationState.value = ChatPresentation(shown = true, collapsed = false)
    }

    /** Closes the pane or sheet; the chat stays current with its draft. */
    @Synchronized fun close() {
        presentationState.value = ChatPresentation()
    }

    /**
     * A link was followed out of the chat. On phones the sheet closes; on wide layouts the pane
     * stays docked, or collapsed, beside the screen the link opens.
     */
    fun followedLink(wide: Boolean) {
        if (!wide) close()
    }

    fun view(key: String) = viewState.value[key] ?: ChatViewState()

    @Synchronized fun updateView(key: String, change: (ChatViewState) -> ChatViewState) {
        if (key in gone) return
        viewState.value = viewState.value + (key to change(view(key)))
    }

    /** Removes the pending image [id] from [key]'s next message. */
    fun removePending(key: String, id: String) = updateView(key) { it.copy(images = it.images.without(id)) }

    /** Drops [key]'s draft, pending images and scroll position. */
    @Synchronized fun clearView(key: String) {
        viewState.value = viewState.value - key
    }

    /**
     * A chat was deleted: its pane or sheet closes, its draft and pending images are dropped,
     * and nothing more is kept for it. When it was current there is no current chat.
     */
    @Synchronized fun deleted(key: String) {
        gone += key
        deletedState.value = deletedState.value + key
        viewState.value = viewState.value - key
        if (currentState.value == key) {
            currentState.value = null
            presentationState.value = ChatPresentation()
            persist(null)
        }
    }
}
