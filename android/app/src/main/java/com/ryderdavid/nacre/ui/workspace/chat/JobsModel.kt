package com.ryderdavid.nacre.ui.workspace.chat

import com.ryderdavid.nacre.data.ChatFirstData
import com.ryderdavid.nacre.data.BoardSnapshot
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.WorkSummary

/**
 * One job: a chat that started or created work, or a round or run started from the board. Its progress
 * is read from that work, never kept separately: the open round's sections landed, else the change's
 * tasks, else the epic's or the beads' closed count.
 */
data class JobRow(
    /** The chat's key, or null for work started outside a chat. */
    val chatKey: String?,
    val title: String,
    /** claude or codex; empty for work started from the board. */
    val agent: String,
    /** What it is working on: the change, the epic or bead, or the round. */
    val subject: String,
    /** Where the work stands, in words. */
    val status: String,
    val done: Int,
    /** 0 when there is nothing to count. */
    val total: Int,
    val running: Boolean,
    val stopped: Boolean,
    val waiting: Int,
    /** The open round or run, for the work inspector. */
    val work: WorkSummary?,
    val updatedAt: String,
)

/** The Jobs list: chats with work, then the other conversations. */
data class JobsList(val jobs: List<JobRow>, val conversations: List<ThreadRow>) {
    val runningCount get() = jobs.count { it.running }
}

object Jobs {
    fun build(
        chats: List<ChatRecord>,
        repoKey: String,
        siblings: Set<String>,
        work: List<WorkSummary>,
        board: BoardSnapshot?,
        openDecisions: Map<String, Int> = emptyMap(),
    ): JobsList {
        val host = Keys.host(repoKey)
        val repoId = Keys.id(repoKey)
        val threads = threadsFor(chats, repoKey, siblings, openDecisions)
        val byKey = chats.associateBy { it.key }
        val jobs = mutableListOf<JobRow>()
        val conversations = mutableListOf<ThreadRow>()
        for (t in threads) {
            val c = byKey[t.key] ?: continue
            // The project's work list is this checkout's; a chat on another computer joins only its links.
            val items = if (c.hostId == host) work.filter { it.chatId == c.id } else emptyList()
            val beads = c.work.filter { it.kind == "bead" && it.repo == c.repo }.map { it.id }
            val changes = c.work.filter { it.kind == "change" && it.repo == c.repo }.map { it.id }
            if (items.isEmpty() && beads.isEmpty() && changes.isEmpty()) { conversations += t; continue }
            jobs += job(t, items, beads, changes, if (c.repo == repoId && c.hostId == host) board else null, c.updatedAt.ifBlank { c.createdAt })
        }
        // Work started from the board, outside any chat, while it is still open.
        work.filter { it.chatId.isNullOrBlank() && it.open }.forEach { w ->
            jobs += JobRow(
                chatKey = null, title = w.title.ifBlank { "Round ${w.id}" }, agent = "", subject = w.change.ifBlank { "Started from the board" },
                status = ChatFirstData.statusText(w), done = w.landed, total = w.total, running = w.running, stopped = w.stopped,
                waiting = 0, work = w, updatedAt = w.lastOutputAt.ifBlank { w.startedAt },
            )
        }
        val sorted = jobs.sortedWith(
            compareByDescending<JobRow> { it.waiting > 0 || it.running || it.stopped }.thenByDescending { it.updatedAt },
        )
        return JobsList(sorted, conversations)
    }

    private fun job(t: ThreadRow, items: List<WorkSummary>, beads: List<String>, changes: List<String>, board: BoardSnapshot?, updatedAt: String): JobRow {
        val w = items.firstOrNull { it.open } ?: items.maxByOrNull { it.startedAt }
        val index = board?.index
        val change = changes.firstNotNullOfOrNull { id -> board?.changes?.forSpec?.firstOrNull { it.id == id } }
        val epic = beads.firstNotNullOfOrNull { id -> index?.byId?.get(id)?.takeIf { it.isEpic } }
        val linked = beads.mapNotNull { index?.byId?.get(it) }
        val subject = when {
            w != null && w.change.isNotBlank() -> w.change
            change != null -> change.name.ifBlank { change.id }
            epic != null -> epic.title
            linked.isNotEmpty() -> linked.first().title + if (linked.size > 1) " and ${linked.size - 1} more" else ""
            w != null -> w.title
            else -> beads.firstOrNull() ?: changes.firstOrNull().orEmpty()
        }
        var done = 0
        var total = 0
        var status = ""
        when {
            w != null && (w.open || w.total > 0) -> { done = w.landed; total = w.total; status = ChatFirstData.statusText(w) }
            change != null && change.tasksTotal > 0 -> { done = change.tasksDone; total = change.tasksTotal; status = "$done of $total tasks done" }
            epic != null -> {
                val kids = index!!.descendants(epic.id)
                done = kids.count { it.isClosed }; total = kids.size
                status = if (total == 0) "Epic, no beads yet" else "$done of $total beads closed"
            }
            linked.isNotEmpty() -> {
                done = linked.count { it.isClosed }; total = linked.size
                status = if (total == 1) (if (done == 1) "Closed" else linked.first().status.replace('_', ' ')) else "$done of $total beads closed"
            }
            change != null -> status = change.phase.replace('_', ' ')
            w != null -> status = ChatFirstData.statusText(w)
        }
        return JobRow(
            chatKey = t.key, title = t.title, agent = t.agent, subject = subject, status = status, done = done, total = total,
            running = w?.running == true, stopped = w?.stopped == true, waiting = t.waiting, work = w?.takeIf { it.open }, updatedAt = updatedAt,
        )
    }
}
