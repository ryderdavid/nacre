package com.ryderdavid.nacre.ui.workspace.decisions

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.update

/**
 * Unsent card input, kept per decision for as long as the app runs (switching thread or pane keeps it), and
 * which decisions are being sent, so the same card can't be sent twice from the queue and the thread.
 */
object DecisionDrafts {
    val all = MutableStateFlow<Map<String, DecisionDraft>>(emptyMap())
    val sending = MutableStateFlow<Set<String>>(emptySet())
    fun set(key: String, draft: DecisionDraft) = all.update { it + (key to draft) }
    fun clear(key: String) = all.update { it - key }
    fun begin(key: String): Boolean {
        var started = false
        sending.update { if (key in it) it else { started = true; it + key } }
        return started
    }
    fun end(key: String) = sending.update { it - key }
}

