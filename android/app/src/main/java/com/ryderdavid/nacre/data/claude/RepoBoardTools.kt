package com.ryderdavid.nacre.data.claude

import com.ryderdavid.nacre.data.NacreRepository
import com.ryderdavid.nacre.data.board.BoardFilter
import com.ryderdavid.nacre.data.board.BoardIndex
import com.ryderdavid.nacre.data.model.CreateIssue
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.RunRequest
import com.ryderdavid.nacre.data.model.UpdateIssue
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.intOrNull

/** [BoardTools] backed by the bridge, scoped to one repo. */
class RepoBoardTools(
    private val repo: NacreRepository,
    private val repoKey: String,
    override val boardName: String,
) : BoardTools {
    override val boardKey get() = repoKey

    private suspend fun index(): BoardIndex = repo.awaitBoard(repoKey).index

    private fun line(ix: BoardIndex, i: Issue): String {
        val blockers = ix.openBlockers(i)
        return buildString {
            append(i.id).append(" [").append(ix.laneOf(i).key).append("] P").append(i.priority).append(' ').append(i.type)
            append(" — ").append(i.title)
            if (i.needsYou) append(" (needs Ryder)")
            if (blockers.isNotEmpty()) append(" (waits on ").append(blockers.joinToString()).append(')')
            if (i.parent.isNotEmpty()) append(" ⊂ ").append(i.parent)
        }
    }

    override suspend fun listBeads(lane: String?, query: String?, limit: Int): String {
        val ix = index()
        val filter = BoardFilter(query = query.orEmpty())
        val rows = ix.issues.filter { ix.matches(it, filter) }.filter { i ->
            when (lane) {
                null, "", "all" -> true
                "open" -> !i.isClosed
                else -> ix.laneOf(i).key == lane
            }
        }.sortedWith(compareBy<Issue> { ix.laneOf(it).ordinal }.thenBy { it.priority }.then(BoardIndex.idOrder))
        if (rows.isEmpty()) return "No beads match."
        val shown = rows.take(limit.coerceIn(1, 200))
        return shown.joinToString("\n") { line(ix, it) } + if (rows.size > shown.size) "\n… ${rows.size - shown.size} more" else ""
    }

    override suspend fun getBead(id: String): String {
        val d = repo.detail(repoKey, id)
        val ix = index()
        val i = d.issue
        return buildString {
            appendLine(line(ix, i))
            appendLine("status: ${i.status}; assignee: ${i.assignee.ifBlank { "none" }}; labels: ${i.labels.joinToString().ifBlank { "none" }}")
            if (i.description.isNotBlank()) appendLine("\nDescription:\n${i.description}")
            if (i.design.isNotBlank()) appendLine("\nDesign:\n${i.design}")
            if (i.acceptance.isNotBlank()) appendLine("\nAcceptance criteria:\n${i.acceptance}")
            if (i.notes.isNotBlank()) appendLine("\nNotes:\n${i.notes}")
            val kids = ix.childrenOf(i.id)
            if (kids.isNotEmpty()) appendLine("\nChildren:\n" + kids.joinToString("\n") { "- " + line(ix, it) })
            val deps = ix.dependentsOf(i.id)
            if (deps.isNotEmpty()) appendLine("\nBlocks:\n" + deps.joinToString("\n") { "- " + line(ix, it) })
            if (d.comments.isNotEmpty()) appendLine("\nComments:\n" + d.comments.joinToString("\n") { "- ${it.author} (${it.createdAt.take(10)}): ${it.text}" })
        }
    }

    private fun JsonObject.s(k: String) = (this[k] as? JsonPrimitive)?.takeUnless { it.content == "null" }?.content

    override suspend fun updateBead(id: String, fields: JsonObject): String {
        val status = fields.s("status")
        if (status == "closed") return closeBead(id, null)
        val saved = repo.update(
            repoKey, id,
            UpdateIssue(
                title = fields.s("title"), description = fields.s("description"), design = fields.s("design"),
                acceptance = fields.s("acceptance_criteria"), notes = fields.s("notes"), status = status,
                priority = (fields["priority"] as? JsonPrimitive)?.intOrNull, type = fields.s("issue_type"),
                assignee = fields.s("assignee"), labels = fields.stringList("labels"),
            ),
        )
        return "Updated: " + line(index(), saved)
    }

    override suspend fun createBead(fields: JsonObject): String {
        val created = repo.create(
            repoKey,
            CreateIssue(
                title = fields.s("title")!!, description = fields.s("description"), type = fields.s("issue_type"),
                priority = (fields["priority"] as? JsonPrimitive)?.intOrNull, parent = fields.s("parent")?.ifBlank { null },
                acceptance = fields.s("acceptance_criteria")?.ifBlank { null }, labels = fields.stringList("labels")?.takeIf { it.isNotEmpty() },
            ),
        )
        return "Created: " + line(index(), created)
    }

    override suspend fun closeBead(id: String, reason: String?): String {
        val saved = repo.close(repoKey, id, reason)
        return "Closed: " + line(index(), saved)
    }

    override suspend fun addComment(id: String, text: String): String {
        val comments = repo.comment(repoKey, id, text)
        return "Comment added to $id (${comments.size} total)."
    }

    override suspend fun addDependency(issueId: String, dependsOnId: String): String {
        repo.addDep(repoKey, issueId, dependsOnId)
        return "$issueId now waits on $dependsOnId."
    }

    override suspend fun startRound(ids: List<String>, budgetUsd: Double?): String {
        if (ids.isEmpty()) return "Name at least one bead."
        val round = repo.startRound(repoKey, ids, budgetUsd)
        return "Started round ${round.id} with ${ids.joinToString(", ")} on ${round.hostName.ifBlank { "the computer" }}, budget $" +
            String.format(java.util.Locale.US, "%.2f", round.budgetUsd) + ". It reviews the beads together first and asks Ryder if they disagree. Ryder can follow it in the Runs tab."
    }

    override suspend fun runInClaudeCode(id: String, instructions: String?): String {
        val run = repo.startRun(repoKey, id, RunRequest(instructions = instructions?.ifBlank { null }))
        return "Started Claude Code run ${run.id} for $id on ${run.hostName.ifBlank { "the computer" }} (${run.status}). Ryder can follow it in the Runs tab."
    }
}
