package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.ui.workspace.ChatReference
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/**
 * What the next message in a project checkout points at: references Ryder attached with Ask about this,
 * and the item a pane shows (the Viewing chip), which Ryder can drop for one message.
 */
class ChatReferences {
    data class State(
        val attached: List<ChatReference> = emptyList(),
        val viewing: ChatReference? = null,
        /** Viewing is left out of the next message. */
        val viewingDropped: Boolean = false,
    )

    private val states = MutableStateFlow<Map<String, State>>(emptyMap())
    val all: StateFlow<Map<String, State>> = states.asStateFlow()

    fun state(repoKey: String): State = states.value[repoKey] ?: State()

    fun add(repoKey: String, ref: ChatReference) = states.update { m ->
        val s = m[repoKey] ?: State()
        m + (repoKey to s.copy(attached = (s.attached.filterNot { it.kind == ref.kind && it.id == ref.id && it.text == ref.text } + ref).takeLast(MAX)))
    }

    fun remove(repoKey: String, ref: ChatReference) = states.update { m ->
        val s = m[repoKey] ?: return@update m
        m + (repoKey to s.copy(attached = s.attached - ref))
    }

    fun view(repoKey: String, ref: ChatReference?) = states.update { m ->
        val s = m[repoKey] ?: State()
        if (s.viewing == ref) m else m + (repoKey to s.copy(viewing = ref, viewingDropped = false))
    }

    fun dropViewing(repoKey: String, dropped: Boolean) = states.update { m ->
        val s = m[repoKey] ?: State()
        m + (repoKey to s.copy(viewingDropped = dropped))
    }

    /** After a send: attached references are spent and Viewing comes back for the next message. */
    fun sent(repoKey: String) = states.update { m ->
        val s = m[repoKey] ?: return@update m
        m + (repoKey to s.copy(attached = emptyList(), viewingDropped = false))
    }

    private val specTargets = MutableStateFlow<Map<String, String>>(emptyMap())

    /** A change the Spec pane should open next, by project checkout (set by Open change and evidence links). */
    val specTarget: StateFlow<Map<String, String>> = specTargets.asStateFlow()

    fun openSpec(repoKey: String, changeId: String) = specTargets.update { it + (repoKey to changeId) }

    /** The Spec pane took the target. */
    fun specOpened(repoKey: String) = specTargets.update { it - repoKey }

    companion object {
        const val MAX = 6
    }
}
