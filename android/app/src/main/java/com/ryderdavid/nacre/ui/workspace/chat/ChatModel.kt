package com.ryderdavid.nacre.ui.workspace.chat

import com.ryderdavid.nacre.data.chat.ConversationState
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.ScreenContext
import com.ryderdavid.nacre.data.model.WorkSummary
import com.ryderdavid.nacre.ui.workspace.ChatReference

/** What a message sends: the Viewing item as the screen context, and references in the text. */
object ChatSend {
    /** The screen context for a message from a project checkout ([repoId] on the chat's computer). */
    fun context(repoId: String, viewing: ChatReference?, dropped: Boolean): ScreenContext {
        val v = viewing.takeUnless { dropped }
        return when (v?.kind) {
            null -> ScreenContext(ScreenContext.BOARD, repo = repoId)
            "bead" -> ScreenContext(ScreenContext.BEAD, repo = repoId, beadId = v.id, summary = com.ryderdavid.nacre.data.chat.trimUtf8(v.text).ifBlank { null })
            "round" -> ScreenContext(ScreenContext.ROUND, repo = repoId, roundId = v.id)
            "run" -> ScreenContext(ScreenContext.RUN, repo = repoId, runId = v.id)
            else -> ScreenContext(
                ScreenContext.CHANGE, repo = repoId, changeId = changeIdOf(v.id),
                pane = v.kind,
                // A document or requirement names its file, so the agent can open exactly what Ryder sees.
                selection = if ('/' in v.id) "${v.id} · ${v.label}" else v.label,
                summary = com.ryderdavid.nacre.data.chat.trimUtf8(v.text).ifBlank { null },
            )
        }
    }

    /**
     * The change a Spec reference belongs to: a change name as is, the change folder of a path under
     * openspec/changes/ (archived ones included), or the first segment of a change-relative path.
     * A current spec under openspec/specs/ belongs to none.
     */
    fun changeIdOf(id: String): String? {
        if ('/' !in id) return id.ifBlank { null }
        val parts = id.split('/')
        if (parts[0] != "openspec") return parts[0].ifBlank { null }
        if (parts.size >= 3 && parts[1] == "changes") {
            return if (parts[2] == "archive") parts.getOrNull(3) else parts[2]
        }
        return null
    }

    /** The message text with the attached references named first, and quoted text where there is some. */
    fun text(draft: String, refs: List<ChatReference>): String {
        if (refs.isEmpty()) return draft.trim()
        val head = "About: " + refs.joinToString("; ") { r -> if (r.id.isBlank() || r.id == r.label) "${r.label} (${r.kind})" else "${r.label} (${r.kind} ${r.id})" }
        val quotes = refs.filter { it.text.isNotBlank() }.joinToString("\n") { r -> r.text.trim().lines().joinToString("\n") { "> $it" } }
        return listOf(head, quotes, draft.trim()).filter { it.isNotBlank() }.joinToString("\n\n")
    }
}

/** What the agent is doing, from the turn's execution state; silence is stated, never called stalled. */
enum class AgentPhase { IDLE, WORKING, QUIET, WAITING, RECONNECTING, FAILED, STALLED }

data class AgentState(val phase: AgentPhase, val text: String)

object AgentStates {
    const val QUIET_AFTER_MS = 2 * 60_000L

    /**
     * [sinceOutputMs] is how long since the turn last produced anything. "May be stalled" comes only from
     * nacred's stall signal ([stalled]); a quiet running turn says "No updates for N min".
     */
    fun of(s: ConversationState, sinceOutputMs: Long, stalled: Boolean, pendingDecisions: Int = 0): AgentState = when {
        s.reconnecting -> AgentState(AgentPhase.RECONNECTING, "Reconnecting")
        s.error != null && !s.turnRunning -> AgentState(AgentPhase.FAILED, "Failed: ${s.error}")
        s.pendingCards.isNotEmpty() || pendingDecisions > 0 -> AgentState(AgentPhase.WAITING, "Waiting for you")
        s.turnRunning && stalled -> AgentState(AgentPhase.STALLED, "May be stalled")
        s.turnRunning && sinceOutputMs >= QUIET_AFTER_MS -> AgentState(AgentPhase.QUIET, "Working · no updates for ${sinceOutputMs / 60_000} min")
        s.turnRunning -> AgentState(AgentPhase.WORKING, "Working")
        else -> AgentState(AgentPhase.IDLE, "")
    }
}

/** Suggestion chips for an empty composer; they fill the composer and never send. */
fun suggestions(needsYou: Int, work: WorkSummary?, hasViewing: ChatReference?): List<String> = buildList {
    if (needsYou > 0) add("What needs me?")
    if (work?.stopped == true) add("Why did the round stop?")
    if (work?.running == true) add("How's the round going?")
    hasViewing?.let { add("Explain ${it.label}") }
    add("What's next?")
    add("What's blocked?")
    add("Draft a proposal for ")
}.distinct().take(4)

/** A thread as the switcher lists it. */
data class ThreadRow(val key: String, val title: String, val agent: String, val waiting: Int, val lastLine: String, val hostId: String, val here: Boolean)

/**
 * The threads of a project: this checkout's first, then the project's checkouts on other computers
 * ([siblings] are the repository keys of the same project), each newest first.
 */
fun threadsFor(chats: List<ChatRecord>, repoKey: String, siblings: Set<String>): List<ThreadRow> =
    chats.filter { c -> c.repo != null && Keys.of(c.hostId, c.repo) in (siblings + repoKey) }
        .sortedWith(compareByDescending<ChatRecord> { Keys.of(it.hostId, it.repo.orEmpty()) == repoKey }.thenByDescending { it.updatedAt.ifBlank { it.createdAt } })
        .map { c ->
            ThreadRow(
                key = c.key, title = c.title.ifBlank { "Untitled chat" }, agent = c.agent,
                waiting = maxOf(c.waitingCount, if (c.waiting) 1 else 0), lastLine = c.lastLine,
                hostId = c.hostId, here = Keys.of(c.hostId, c.repo.orEmpty()) == repoKey,
            )
        }
