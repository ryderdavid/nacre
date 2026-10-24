package com.ryderdavid.nacre.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.Transient
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject

/**
 * Repo and run ids are only unique on one computer, so the app addresses them as
 * "<host>~<id>", where host is the local id of the paired computer.
 */
object Keys {
    private const val SEP = '~'
    fun of(hostId: String, id: String) = "$hostId$SEP$id"
    fun host(key: String) = key.substringBefore(SEP)
    fun id(key: String) = key.substringAfter(SEP)
}

/** A beads workspace on one computer, as reported by nacred (`/v1/repos`). */
@Serializable
data class Repo(
    val id: String,
    val name: String,
    val path: String,
    val prefix: String = "",
    val counts: Counts = Counts(),
    @SerialName("last_updated") val lastUpdated: String = "",
    @SerialName("active_runs") val activeRuns: Int = 0,
    /** Live agent-owned sessions nacred only watches here; not counted by [activeRuns]. */
    @SerialName("adopted_runs") val adoptedRuns: Int = 0,
    val error: String = "",
    /** Normalized origin remote, e.g. "github.com/owner/repo"; "" if none. */
    @SerialName("git_remote") val gitRemote: String = "",
    @SerialName("git_branch") val gitBranch: String = "",
    /** Set by the app: which paired computer this repo is on. */
    @Transient val hostId: String = "",
    @Transient val hostName: String = "",
) {
    val key get() = Keys.of(hostId, id)

    /** Repos on different computers with the same remote are one repository. */
    val groupId get() = if (gitRemote.isNotEmpty()) "git:$gitRemote" else key

    @Serializable
    data class Counts(
        val open: Int = 0,
        @SerialName("in_progress") val inProgress: Int = 0,
        val blocked: Int = 0,
        val deferred: Int = 0,
        val closed: Int = 0,
        val pinned: Int = 0,
        val hooked: Int = 0,
        val total: Int = 0,
    ) {
        val active get() = open + inProgress + blocked + hooked
    }
}

@Serializable
data class Dependency(
    @SerialName("issue_id") val issueId: String,
    @SerialName("depends_on_id") val dependsOnId: String,
    val type: String = "blocks",
)

/** One bead. Field names follow `bd list --json`; unknown fields are ignored. */
@Serializable
data class Issue(
    val id: String,
    val title: String = "",
    val status: String = "open",
    val priority: Int = 2,
    @SerialName("issue_type") val type: String = "task",
    val description: String = "",
    val design: String = "",
    @SerialName("acceptance_criteria") val acceptance: String = "",
    val notes: String = "",
    val assignee: String = "",
    val owner: String = "",
    val labels: List<String> = emptyList(),
    @SerialName("created_at") val createdAt: String = "",
    @SerialName("updated_at") val updatedAt: String = "",
    @SerialName("closed_at") val closedAt: String = "",
    @SerialName("close_reason") val closeReason: String = "",
    val parent: String = "",
    val dependencies: List<Dependency> = emptyList(),
    @SerialName("comment_count") val commentCount: Int = 0,
    @SerialName("spec_id") val specId: String = "",
    @SerialName("external_ref") val externalRef: String = "",
) {
    val isClosed get() = status == "closed"
    val isEpic get() = type == "epic"

    /** Ids this bead waits on (outgoing `blocks` edges). */
    val blockerIds get() = dependencies.filter { it.type == "blocks" && it.dependsOnId != id }.map { it.dependsOnId }

    /** Open owner decisions and anything flagged with bd's `human` label. */
    val needsYou get() = !isClosed && ("human" in labels || type == "decision")
}

@Serializable
data class Comment(
    val id: String = "",
    @SerialName("issue_id") val issueId: String = "",
    val author: String = "",
    val text: String = "",
    @SerialName("created_at") val createdAt: String = "",
)

@Serializable
data class Run(
    val id: String,
    @SerialName("repo_id") val repoId: String = "",
    @SerialName("repo_name") val repoName: String = "",
    @SerialName("repo_path") val repoPath: String = "",
    @SerialName("issue_id") val issueId: String = "",
    @SerialName("issue_title") val issueTitle: String = "",
    val status: String = "queued",
    val mode: String = "",
    val model: String = "",
    val worktree: Boolean = false,
    @SerialName("session_id") val sessionId: String = "",
    /** Where Claude actually ran (differs from repo_path for worktree runs). */
    @SerialName("session_cwd") val sessionCwd: String = "",
    @SerialName("created_at") val createdAt: String = "",
    @SerialName("started_at") val startedAt: String = "",
    @SerialName("ended_at") val endedAt: String = "",
    @SerialName("cost_usd") val costUsd: Double = 0.0,
    @SerialName("duration_ms") val durationMs: Long = 0,
    @SerialName("num_turns") val numTurns: Int = 0,
    val turns: Int = 1,
    @SerialName("tool_calls") val toolCalls: Int = 0,
    @SerialName("last_text") val lastText: String = "",
    @SerialName("result_text") val resultText: String = "",
    val error: String = "",
    @SerialName("resume_command") val resumeCommand: String = "",
    /** [ORIGIN_ADOPTED] for a session an agent started itself; absent means Nacre dispatched the run. */
    val origin: String = "",
    /** Tool that owns an adopted session ("claude-code"); absent means Claude Code. */
    val agent: String = "",
    /** The name the agent gave its session; the only human label an adopted run has. */
    @SerialName("session_name") val sessionName: String = "",
    /** The agent's own process on the Mac. Shown, never signalled: it is not nacred's child. */
    @SerialName("owner_pid") val ownerPid: Int = 0,
    /** [ACTIVITY_WORKING], [ACTIVITY_IDLE] or [ACTIVITY_ENDED] on an adopted run, "" otherwise. */
    val activity: String = "",
    /**
     * An adopted session that is alive but has not written its transcript for the bridge's idle
     * window (adopt_idle_minutes): left open rather than working. It still shows on its bead,
     * as idle, but not in the Runs list or the active count; see [inRunsList].
     */
    val stale: Boolean = false,
    /** When the agent's transcript last gained an entry, as Claude Code wrote it; "" until then. */
    @SerialName("last_activity_at") val lastActivityAt: String = "",
    /** Null means "allowed": the bridge only sends these to say an action is refused outright. */
    @SerialName("can_cancel") val canCancel: Boolean? = null,
    @SerialName("can_followup") val canFollowup: Boolean? = null,
    /** Set by the app: which paired computer ran this. */
    @Transient val hostId: String = "",
    @Transient val hostName: String = "",
) {
    val isActive get() = status == "queued" || status == "running"
    val key get() = Keys.of(hostId, id)
    val repoKey get() = Keys.of(hostId, repoId)

    /** A session an agent started on its own, which the Mac only watches (see docs/API.md). */
    val isAdopted get() = origin == ORIGIN_ADOPTED

    /**
     * Work Nacre asked for and is waiting on. Adopted runs are excluded everywhere this
     * decides whether something is "in progress for us": nobody asked for them from here,
     * and they may be a terminal a person leaves open for hours.
     */
    val isNacreWork get() = isActive && !isAdopted

    /** Whether the Runs list and its badge show this run: everything but a [stale] agent session. */
    val inRunsList get() = !stale

    /** Cancel is offered while the run is active unless the bridge says it is refused. */
    val showsCancel get() = isActive && canCancel != false

    /** A follow-up needs a finished run with a session to resume, and the bridge's permission. */
    val showsFollowUp get() = !isActive && sessionId.isNotBlank() && canFollowup != false

    /** The `cd … && claude --resume …` line to pick this session up by hand, when there is one. */
    val takeoverCommand get() = resumeCommand.takeIf { it.isNotBlank() && sessionId.isNotBlank() }
}

/** `origin` of a run the Mac only watches; see "Adopted runs" in docs/API.md. */
const val ORIGIN_ADOPTED = "adopted"

/** `activity` of an adopted run: finer-grained than `status`, which stays one of the five. */
const val ACTIVITY_WORKING = "working"
const val ACTIVITY_IDLE = "idle"
const val ACTIVITY_ENDED = "ended"

@Serializable
data class RunEvent(
    val seq: Long,
    val ts: String = "",
    val kind: String,
    val text: String = "",
    val tool: String = "",
    @SerialName("tool_id") val toolId: String = "",
    val input: JsonElement? = null,
    @SerialName("is_error") val isError: Boolean = false,
    val status: String = "",
)

@Serializable
data class Health(
    val name: String = "",
    val version: String = "",
    val hostname: String = "",
    @SerialName("host_id") val hostId: String = "",
    @SerialName("host_name") val hostName: String = "",
    @SerialName("bd_version") val bdVersion: String = "",
    @SerialName("claude_version") val claudeVersion: String = "",
    val repos: Int = 0,
    @SerialName("active_runs") val activeRuns: Int = 0,
    /** Optional capabilities this computer reports; see [Features]. */
    val features: List<String> = emptyList(),
) {
    /** True when this computer can run Refine, Draft and the assistant itself. */
    val runsClaude get() = Features.CLAUDE in features
}

object Features {
    /** nacred can run the app's Claude features headless on its own subscription. */
    const val CLAUDE = "claude"
}

// ---- request bodies ----

@Serializable
data class CreateIssue(
    val title: String,
    val description: String? = null,
    @SerialName("issue_type") val type: String? = null,
    val priority: Int? = null,
    val parent: String? = null,
    val labels: List<String>? = null,
    val design: String? = null,
    @SerialName("acceptance_criteria") val acceptance: String? = null,
    val notes: String? = null,
    val assignee: String? = null,
)

/** Partial update; only non-null fields are sent. */
@Serializable
data class UpdateIssue(
    val title: String? = null,
    val description: String? = null,
    val design: String? = null,
    @SerialName("acceptance_criteria") val acceptance: String? = null,
    val notes: String? = null,
    val status: String? = null,
    val priority: Int? = null,
    @SerialName("issue_type") val type: String? = null,
    val assignee: String? = null,
    val labels: List<String>? = null,
    val parent: String? = null,
)

@Serializable
data class RunRequest(
    val mode: String? = null,
    val model: String? = null,
    val instructions: String? = null,
    val worktree: Boolean? = null,
)

/**
 * Asks a computer to answer one prompt with `claude -p` on its own Claude
 * subscription. The app sends the prompt, the system prompt and the JSON
 * schema, so both ways of reaching Claude return the same shapes.
 */
@Serializable
data class AssistRequest(
    val prompt: String,
    val system: String = "",
    val schema: JsonObject? = null,
    val model: String = "",
    val effort: String = "",
    /** Continues an earlier assistant conversation. */
    @SerialName("session_id") val sessionId: String = "",
    /** Lets the call run read-only bd commands in the repository. */
    @SerialName("bd_reads") val bdReads: Boolean = false,
    /** Keeps the session so the next turn can resume it. */
    val keep: Boolean = false,
)

/** What one [AssistRequest] produced. */
@Serializable
data class AssistResult(
    @SerialName("session_id") val sessionId: String = "",
    val output: JsonObject? = null,
    val text: String = "",
    @SerialName("cost_usd") val costUsd: Double = 0.0,
)
