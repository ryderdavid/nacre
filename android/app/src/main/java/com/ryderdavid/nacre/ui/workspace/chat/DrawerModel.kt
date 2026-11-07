package com.ryderdavid.nacre.ui.workspace.chat

import com.ryderdavid.nacre.data.BoardSnapshot
import com.ryderdavid.nacre.data.ChatFirstData
import com.ryderdavid.nacre.ui.workspace.ChatReference
import com.ryderdavid.nacre.data.model.ChangeSummary
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.WorkSummary

/** What a chat card says the chat is working on. */
data class ChatJob(val label: String, val subject: String, val status: String, val done: Int, val total: Int)

/** A chat as a card in the drawer: what it is, where it stands, and the job it works toward, if any. */
data class ChatCard(
    val key: String,
    val title: String,
    val agent: String,
    val lastLine: String,
    val waiting: Int,
    val running: Boolean,
    val updatedAt: String,
    val job: ChatJob?,
) {
    /** Needs Ryder or is working: listed first. */
    val active get() = waiting > 0 || running
}

object ChatCards {
    /** Every chat of the project as a card: those waiting on Ryder or working first, then the most recent. */
    fun build(
        chats: List<ChatRecord>,
        repoKey: String,
        siblings: Set<String>,
        work: List<WorkSummary>,
        board: BoardSnapshot?,
        openDecisions: Map<String, Int> = emptyMap(),
    ): List<ChatCard> {
        val list = Jobs.build(chats, repoKey, siblings, work, board, openDecisions)
        val byKey = chats.associateBy { it.key }
        val cards = list.jobs.mapNotNull { j ->
            val key = j.chatKey ?: return@mapNotNull null
            ChatCard(
                key, j.title, j.agent, j.lastLine, j.waiting, j.running, j.updatedAt,
                ChatJob(j.label, j.subject, j.status, j.done, j.total),
            )
        } + list.conversations.map { t ->
            val c = byKey[t.key]
            ChatCard(t.key, t.title, t.agent, t.lastLine, t.waiting, c?.active == true, c?.updatedAt?.ifBlank { c.createdAt }.orEmpty(), null)
        }
        return cards.sortedWith(compareByDescending<ChatCard> { it.active }.thenByDescending { it.updatedAt })
    }
}

enum class StepState { DONE, ACTIVE, TODO }

data class JobStep(val state: StepState, val text: String)

/** A piece of work as a card in Jobs: a bead, an epic, a spec change or a round, with the chats on it. */
data class WorkJob(
    val key: String,
    /** epic, bug, bead, spec or round. */
    val kind: String,
    /** The bead id, the change's name or the round's id. */
    val label: String,
    val title: String,
    val status: String,
    val done: Int,
    /** 0 when there is nothing to count. */
    val total: Int,
    val running: Boolean,
    val waiting: Int,
    /** The chats working on it: key and title. */
    val chats: List<Pair<String, String>>,
    val steps: List<JobStep>,
    val finished: Boolean,
    /** What a new chat about it attaches. */
    val reference: ChatReference?,
    /** The open round or run, for the work inspector. */
    val work: WorkSummary?,
)

data class WorkJobs(val now: List<WorkJob>, val done: List<WorkJob>) {
    val runningCount get() = now.count { it.running }
}

object WorkJobsModel {
    private const val DONE_SHOWN = 5

    /**
     * The project's work, one card per item: what its chats link to (beads and changes) and the rounds they
     * started, work started from the board, and beads and changes in progress that no chat is on yet.
     */
    fun build(
        chats: List<ChatRecord>,
        repoKey: String,
        siblings: Set<String>,
        work: List<WorkSummary>,
        board: BoardSnapshot?,
        openDecisions: Map<String, Int> = emptyMap(),
    ): WorkJobs {
        val host = Keys.host(repoKey)
        val repoId = Keys.id(repoKey)
        val index = board?.index
        val changes = board?.changes?.forSpec.orEmpty()
        val threads = threadsFor(chats, repoKey, siblings, openDecisions).associateBy { it.key }
        val byKey = chats.associateBy { it.key }

        class Acc(val key: String) {
            val chats = linkedMapOf<String, String>()
            var waiting = 0
            var round: WorkSummary? = null
        }
        val acc = linkedMapOf<String, Acc>()
        fun at(key: String) = acc.getOrPut(key) { Acc(key) }
        fun changeKey(name: String) = changes.firstOrNull { it.name == name || it.id == name }?.let { "change:" + it.id }

        for ((key, t) in threads) {
            val c = byKey[key] ?: continue
            val keys = mutableSetOf<String>()
            if (c.repo == repoId && c.hostId == host) {
                c.work.filter { it.repo == c.repo }.forEach { l ->
                    when (l.kind) {
                        "bead" -> keys += "bead:" + l.id
                        "change" -> keys += "change:" + l.id
                    }
                }
                work.filter { it.chatId == c.id }.forEach { w ->
                    val k = w.change.takeIf { it.isNotBlank() }?.let(::changeKey) ?: "round:${w.kind}:${w.id}"
                    keys += k
                    if (w.open || at(k).round == null) at(k).round = w
                }
            }
            keys.forEach { k -> at(k).also { it.chats[key] = t.title; it.waiting += t.waiting } }
        }
        // Work started from the board, outside any chat, while it is open.
        work.filter { it.chatId.isNullOrBlank() && it.open }.forEach { w ->
            val k = w.change.takeIf { it.isNotBlank() }?.let(::changeKey) ?: "round:${w.kind}:${w.id}"
            at(k).round = w
        }
        // Beads and changes in progress that nothing above covers: a chat can be started about them.
        index?.issues.orEmpty().filter { it.status == "in_progress" }.forEach { at("bead:" + it.id) }
        changes.filter { it.tasksTotal > 0 && it.tasksDone in 1 until it.tasksTotal && !it.missing }.forEach { at("change:" + it.id) }

        val jobs = acc.values.mapNotNull { a ->
            val r = a.round
            val chatsOn = a.chats.toList()
            when {
                a.key.startsWith("bead:") -> {
                    val issue = index?.byId?.get(a.key.removePrefix("bead:")) ?: return@mapNotNull null
                    beadJob(a.key, issue, index, r, chatsOn, a.waiting)
                }
                a.key.startsWith("change:") -> {
                    val change = changes.firstOrNull { it.id == a.key.removePrefix("change:") } ?: return@mapNotNull null
                    changeJob(a.key, change, index, r, chatsOn, a.waiting)
                }
                r != null -> WorkJob(
                    a.key, "round", r.id, r.title.ifBlank { "Round ${r.id}" }, ChatFirstData.statusText(r), r.landed, r.total,
                    r.running, a.waiting, chatsOn, emptyList(), finished = !r.open, reference = null, work = r.takeIf { it.open },
                )
                else -> null
            }
        }
        val now = jobs.filterNot { it.finished }
            .sortedWith(compareByDescending<WorkJob> { it.waiting > 0 || it.running }.thenByDescending { it.chats.isNotEmpty() }.thenBy { it.title })
        val done = jobs.filter { it.finished && it.chats.isNotEmpty() }.take(DONE_SHOWN)
        return WorkJobs(now, done)
    }

    private fun stepOf(i: Issue) = JobStep(
        when { i.isClosed -> StepState.DONE; i.status == "in_progress" -> StepState.ACTIVE; else -> StepState.TODO },
        i.title.ifBlank { i.id },
    )

    private fun beadJob(key: String, issue: Issue, index: com.ryderdavid.nacre.data.board.BoardIndex, r: WorkSummary?, chats: List<Pair<String, String>>, waiting: Int): WorkJob {
        val kids = index.childrenOf(issue.id)
        val all = if (issue.isEpic) index.descendants(issue.id) else kids
        val done = all.count { it.isClosed }
        val status = when {
            r != null && r.open -> ChatFirstData.statusText(r)
            issue.isClosed -> "Closed"
            all.isNotEmpty() -> "$done of ${all.size} closed"
            else -> issue.status.replace('_', ' ').replaceFirstChar { it.uppercase() }
        }
        return WorkJob(
            key, when { issue.isEpic -> "epic"; issue.type == "bug" -> "bug"; else -> "bead" }, issue.id, issue.title.ifBlank { issue.id },
            status, if (r != null && r.open) r.landed else done, if (r != null && r.open) r.total else all.size,
            r?.running == true, waiting, chats, kids.map(::stepOf), finished = issue.isClosed && (r == null || !r.open),
            reference = com.ryderdavid.nacre.ui.workspace.beads.beadReference(issue.id), work = r?.takeIf { it.open },
        )
    }

    private fun changeJob(key: String, change: ChangeSummary, index: com.ryderdavid.nacre.data.board.BoardIndex?, r: WorkSummary?, chats: List<Pair<String, String>>, waiting: Int): WorkJob {
        val steps = change.sections.map { s ->
            val bead = index?.byId?.get(s.beadId)
            JobStep(
                when { bead?.isClosed == true -> StepState.DONE; bead?.status == "in_progress" -> StepState.ACTIVE; else -> StepState.TODO },
                s.title.ifBlank { s.id },
            )
        }
        val complete = change.tasksTotal > 0 && change.tasksDone >= change.tasksTotal
        val roundOpen = r != null && r.open
        return WorkJob(
            key, "spec", change.name.ifBlank { change.id }, change.name.ifBlank { change.id },
            when {
                roundOpen -> ChatFirstData.statusText(r!!)
                change.tasksTotal > 0 -> "${change.tasksDone} of ${change.tasksTotal} tasks"
                else -> change.phase.replace('_', ' ')
            },
            if (roundOpen) r!!.landed else change.tasksDone, if (roundOpen) r!!.total else change.tasksTotal,
            r?.running == true, waiting, chats, steps, finished = complete && !roundOpen,
            reference = ChatReference("change", change.name.ifBlank { change.id }, change.name.ifBlank { change.id }), work = r?.takeIf { it.open },
        )
    }
}
