package com.ryderdavid.nacre.data.chat

import android.content.Context
import com.ryderdavid.nacre.data.claude.ChatStack
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.FlowPreview
import kotlinx.coroutines.flow.debounce
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch
import kotlinx.serialization.json.Json

/**
 * Keeps each chat's unsent text across the process being killed: written a moment after it changes, put
 * back into the composer at the next launch unless something is already typed there. Images aren't kept.
 */
object ChatDrafts {
    private const val FILE = "chat_drafts"
    private const val KEY = "drafts"

    @OptIn(FlowPreview::class)
    fun keep(context: Context, stack: ChatStack, scope: CoroutineScope) {
        val prefs = context.getSharedPreferences(FILE, Context.MODE_PRIVATE)
        decode(prefs.getString(KEY, null)).forEach { (key, text) ->
            stack.updateView(key) { v -> if (v.draft.isEmpty()) v.copy(draft = text) else v }
        }
        scope.launch {
            stack.views.map { m -> m.mapValues { it.value.draft }.filterValues { it.isNotBlank() } }
                .distinctUntilChanged().debounce(400)
                .collect { d -> prefs.edit().putString(KEY, encode(d)).apply() }
        }
    }

    fun encode(drafts: Map<String, String>): String = Json.encodeToString(drafts)
    fun decode(s: String?): Map<String, String> = s?.let { runCatching { Json.decodeFromString<Map<String, String>>(it) }.getOrNull() }.orEmpty()
}
