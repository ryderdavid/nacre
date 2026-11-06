package com.ryderdavid.nacre.data.chat

import android.content.Context
import androidx.compose.runtime.MutableState
import androidx.compose.runtime.mutableStateOf
import com.ryderdavid.nacre.data.model.ChatWire

/** Which agent the new-chat button starts with one tap; the other one is in its menu. Claude unless changed. */
object DefaultAgent {
    private const val FILE = "chat_prefs"
    private const val KEY = "default_agent"
    private var state: MutableState<String>? = null

    fun state(context: Context): MutableState<String> = state ?: mutableStateOf(
        context.getSharedPreferences(FILE, Context.MODE_PRIVATE).getString(KEY, null).let(::normalize),
    ).also { state = it }

    fun set(context: Context, agent: String) {
        val a = normalize(agent)
        state(context).value = a
        context.getSharedPreferences(FILE, Context.MODE_PRIVATE).edit().putString(KEY, a).apply()
    }

    fun other(agent: String) = if (agent == ChatWire.AGENT_CODEX) ChatWire.AGENT_CLAUDE else ChatWire.AGENT_CODEX
    fun label(agent: String) = if (agent == ChatWire.AGENT_CODEX) "Codex" else "Claude"
    fun normalize(agent: String?) = if (agent == ChatWire.AGENT_CODEX) ChatWire.AGENT_CODEX else ChatWire.AGENT_CLAUDE
}
