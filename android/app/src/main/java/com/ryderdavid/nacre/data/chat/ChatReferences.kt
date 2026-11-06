package com.ryderdavid.nacre.data.chat

import com.ryderdavid.nacre.ui.workspace.ChatReference
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/**
 * What the next message points at. References Ryder attached with Ask about this belong to a thread (by chat
 * key); the item a pane shows (the Viewing chip) belongs to the project checkout and survives leaving the
 * pane, and Ryder can leave it out of one message.
 */
class ChatReferences {
    data class State(
        val attached: List<ChatReference> = emptyList(),
        val viewing: ChatReference? = null,
        /** Viewing is left out of the next message. */
        val viewingDropped: Boolean = false,
    )

    private val attachedByThread = MutableStateFlow<Map<String, List<ChatReference>>>(emptyMap())
    private val viewingByRepo = MutableStateFlow<Map<String, Pair<ChatReference?, Boolean>>>(emptyMap())

    val attachedAll: StateFlow<Map<String, List<ChatReference>>> = attachedByThread.asStateFlow()
    val viewingAll: StateFlow<Map<String, Pair<ChatReference?, Boolean>>> = viewingByRepo.asStateFlow()

    /** The state the composer of [thread] in [repoKey] shows. */
    fun state(repoKey: String, thread: String): State {
        val v = viewingByRepo.value[repoKey]
        return State(attachedByThread.value[thread].orEmpty(), v?.first, v?.second ?: false)
    }

    fun add(thread: String, ref: ChatReference) = attachedByThread.update { m ->
        val list = m[thread].orEmpty()
        m + (thread to (list.filterNot { it.kind == ref.kind && it.id == ref.id && it.text == ref.text } + ref).takeLast(MAX))
    }

    fun remove(thread: String, ref: ChatReference) = attachedByThread.update { m -> m + (thread to (m[thread].orEmpty() - ref)) }

    /** What a pane shows. Clearing it (null) only happens when the pane shows nothing, not when it's left. */
    fun view(repoKey: String, ref: ChatReference?) = viewingByRepo.update { m ->
        if (m[repoKey]?.first == ref) m else m + (repoKey to (ref to false))
    }

    fun dropViewing(repoKey: String, dropped: Boolean) = viewingByRepo.update { m ->
        m + (repoKey to ((m[repoKey]?.first) to dropped))
    }

    /** After the message was accepted: the thread's references are spent and Viewing comes back. */
    fun sent(repoKey: String, thread: String) {
        attachedByThread.update { it - thread }
        viewingByRepo.update { m -> m[repoKey]?.let { m + (repoKey to (it.first to false)) } ?: m }
    }

    /** What the Spec pane opens: a change, or a cited file ([ref]) and the requirement it names, if any. */
    data class SpecTarget(val ref: String, val requirement: String = "")

    private val specTargets = MutableStateFlow<Map<String, SpecTarget>>(emptyMap())

    /** A change or cited file the Spec pane should open next, by project checkout (set by Open change and evidence links). */
    val specTarget: StateFlow<Map<String, SpecTarget>> = specTargets.asStateFlow()

    fun openSpec(repoKey: String, changeId: String, requirement: String = "") = specTargets.update { it + (repoKey to SpecTarget(changeId, requirement)) }

    /** The Spec pane took the target. */
    fun specOpened(repoKey: String) = specTargets.update { it - repoKey }

    private val beadTargets = MutableStateFlow<Map<String, String>>(emptyMap())

    /** A bead the Beads pane should peek next, by project checkout (set by evidence links). */
    val beadTarget: StateFlow<Map<String, String>> = beadTargets.asStateFlow()

    fun openBead(repoKey: String, beadId: String) = beadTargets.update { it + (repoKey to beadId) }

    fun beadOpened(repoKey: String) = beadTargets.update { it - repoKey }

    companion object {
        const val MAX = 6
    }
}
