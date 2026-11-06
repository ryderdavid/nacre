package com.ryderdavid.nacre.data

import com.ryderdavid.nacre.data.model.AttentionItem
import com.ryderdavid.nacre.data.model.BEAD_INTEGRATED
import com.ryderdavid.nacre.data.model.Decision
import com.ryderdavid.nacre.data.model.DecisionOption
import com.ryderdavid.nacre.data.model.GateState
import com.ryderdavid.nacre.data.model.ProjectSummary
import com.ryderdavid.nacre.data.model.ROUND_CANCELLED
import com.ryderdavid.nacre.data.model.ROUND_LANDED
import com.ryderdavid.nacre.data.model.ROUND_NEEDS_DECISION
import com.ryderdavid.nacre.data.model.ROUND_PAUSED
import com.ryderdavid.nacre.data.model.ROUND_READY
import com.ryderdavid.nacre.data.model.Repo
import com.ryderdavid.nacre.data.model.Round
import com.ryderdavid.nacre.data.model.WorkSummary

/**
 * Pure mappings for the chat-first screens, used when a computer's nacred predates the chat-first API
 * and to combine what each computer reports.
 */
object ChatFirstData {
    /** A round as a work summary, for a nacred without `/work`. */
    fun work(round: Round): WorkSummary = WorkSummary(
        kind = "round",
        id = round.id,
        title = "Round ${round.id}",
        branch = round.branch,
        state = when (round.state) {
            ROUND_READY -> "ready_to_land"
            ROUND_LANDED -> "done"
            ROUND_CANCELLED -> "cancelled"
            ROUND_PAUSED, ROUND_NEEDS_DECISION -> "stopped"
            else -> "running"
        },
        landed = round.beads.count { it.state == BEAD_INTEGRATED },
        total = round.beads.size,
        gates = emptyList<GateState>(),
        spendUsd = round.spentUsd,
        budgetUsd = round.budgetUsd,
        startedAt = round.startedAt,
    )

    /** The open work of one repository on one computer, newest first, from its rounds. */
    fun openWork(rounds: List<Round>, hostId: String, repoId: String): List<WorkSummary> =
        rounds.filter { it.hostId == hostId && it.repoId == repoId }.map(::work).filter { it.open }

    /** One line about a repository's open work: "Round running · 3 of 7 landed". */
    fun statusText(work: WorkSummary?): String = when {
        work == null -> ""
        work.state == "ready_to_land" -> "Ready to land · ${work.landed} of ${work.total}"
        work.stopped -> "Round stopped · ${work.landed} of ${work.total} landed"
        work.state == "held" -> "Held by you · ${work.landed} of ${work.total} landed"
        else -> "${if (work.kind == "run") "Run" else "Round"} running · ${work.landed} of ${work.total} landed"
    }

    /** A project row for a nacred without `/projects`, from what the app already knows. */
    fun fallbackProject(repo: Repo, needsYou: Int, work: WorkSummary?): ProjectSummary = ProjectSummary(
        repo = repo.id,
        name = repo.name,
        remote = repo.gitRemote,
        status = when {
            work?.stopped == true || needsYou > 0 -> "waiting"
            work?.running == true || repo.activeRuns > 0 -> "working"
            else -> "idle"
        },
        statusText = statusText(work).ifBlank { if (repo.activeRuns > 0) "${repo.activeRuns} running" else "" },
        needsYou = needsYou,
        work = work,
    )

    /**
     * An attention item as a decision, for a nacred without decisions. Its options keep their labels; an
     * answer performs the item's own action (method, path and body) as the Inbox did.
     */
    fun decision(item: AttentionItem): Decision = Decision(
        id = "att_" + item.id,
        repo = item.repo,
        source = "attention",
        kind = when (item.kind) {
            AttentionItem.ROUND_QUESTION -> "question"
            AttentionItem.BEAD_SIGNOFF -> "signoff"
            AttentionItem.PROPOSAL_READY, AttentionItem.APPROVAL_STALE -> "approve"
            AttentionItem.BEAD_PARKED, AttentionItem.WORKFLOW_INTERRUPTED -> "recovery"
            AttentionItem.FOUND_WORK -> "found"
            else -> "question"
        },
        title = item.title,
        subtitle = item.repoName,
        body = item.why,
        options = item.options.filter { it.type == "action" && it.disabled.isBlank() }.mapIndexed { i, o ->
            DecisionOption(id = i.toString(), label = o.label, recommended = o.recommended)
        },
        allowDefer = false,
        allowReject = false,
        hostId = item.hostId,
    )
}
