package com.ryderdavid.nacre.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.Transient
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.JsonPrimitive

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
    /** Gate names from the repository's nacre.yaml on that computer; empty when it has none. */
    val gates: List<String> = emptyList(),
    /** Why nacre.yaml can't be used, with the line it is about; "" when fine or absent. */
    @SerialName("gates_error") val gatesError: String = "",
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
    /** Set by the bridge when bd shows the bead claimed but no run on that computer is working it. */
    @SerialName("active_elsewhere") val activeElsewhere: ActiveElsewhere? = null,
    val metadata: JsonObject? = null,
) {
    val isClosed get() = status == "closed"

    /** Built and merged, with only Ryder's own check left before it closes. */
    val awaitsSignoff get() = !isClosed && SIGNOFF in labels

    /** What to check before signing off; blank when the round didn't say. */
    val signoffCheck get() = (metadata?.get(SIGNOFF) as? JsonPrimitive)?.contentOrNull.orEmpty()
    val isEpic get() = type == "epic"

    /** Ids this bead waits on (outgoing `blocks` edges). */
    val blockerIds get() = dependencies.filter { it.type == "blocks" && it.dependsOnId != id }.map { it.dependsOnId }

    /** Open owner decisions and anything flagged with bd's `human` label. */
    val needsYou get() = !isClosed && ("human" in labels || type == "decision" || SIGNOFF in labels)
}

/** The label and metadata key nacred puts on a bead left for Ryder's sign-off. */
const val SIGNOFF = "signoff"

/**
 * A bead someone holds in bd while no run on the bridge's computer works on it: another agent
 * (Codex…), another computer, a session that ended. A marker, not a run: there is nothing to open.
 */
@Serializable
data class ActiveElsewhere(
    val assignee: String = "",
    /** The bead's started_at (else updated_at). */
    val since: String = "",
    /** The assignee is the person the bridge acts for, i.e. the user. */
    val self: Boolean = false,
) {
    /**
     * The marker's heading. The user's own claim is the usual case (one person, many sessions), so
     * it is about the session; anyone else's names them.
     */
    val title get() = if (self || assignee.isBlank()) "Active in another session" else "Active elsewhere · $assignee"

    /** The detail's explanation; [ago] is [since] already formatted ("3 min. ago"), or "". */
    fun detail(ago: String): String {
        val whenText = ago.ifBlank { null }?.let { " $it" } ?: ""
        return if (self || assignee.isBlank()) {
            "Claimed$whenText from a session Nacre isn't running or watching: another terminal or computer, " +
                "or an agent Nacre can't follow, like Codex. There is no transcript to show here."
        } else {
            "Claimed by $assignee$whenText. No run in Nacre is working on it, so there is no transcript to show here."
        }
    }
}

/**
 * The [Issue.activeElsewhere] marker to show, given whether a live run on any paired computer is
 * linked to the bead: that run's own marker wins, so the two never show together.
 */
fun Issue.elsewhere(liveRun: Boolean): ActiveElsewhere? = activeElsewhere?.takeIf { !liveRun && !isClosed }

/**
 * Beads with a run working on them now, among [runs] of the repository keys [repoKeys]. Stale agent
 * sessions are left out, as in the Runs list: the bead then shows the bridge's
 * [Issue.activeElsewhere] marker instead.
 */
fun liveRunIssueIds(runs: List<Run>, repoKeys: Set<String>): Set<String> =
    runs.filter { it.isActive && it.inRunsList && it.repoKey in repoKeys }.map { it.issueId }.toSet()

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
    val effort: String = "",
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
    /** Tool that owns the session: [AGENT_CODEX] for a run sent to Codex; absent means Claude Code. */
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
    /**
     * A run whose claude has printed nothing for the bridge's stall window (stall_minutes):
     * probably stuck, e.g. on a dead connection, rather than working. The Mac leaves it
     * running; the person decides whether to wait, or cancel and follow up.
     */
    val stalled: Boolean = false,
    /** When the stalled run last printed anything. */
    @SerialName("stalled_since") val stalledSince: String = "",
    /** Set on runs a round started: its round, and "bead" or "integration". */
    @SerialName("round_id") val roundId: String = "",
    val kind: String = "",
    /** The branch and worktree nacred made for a round run, and where the branch started. */
    val branch: String = "",
    val workspace: String = "",
    val base: String = "",
    /** nacred's own check of a round run's work, once its turn ended. */
    val verification: Verification? = null,
    /** Null means "allowed": the bridge only sends these to say an action is refused outright. */
    @SerialName("can_cancel") val canCancel: Boolean? = null,
    @SerialName("can_followup") val canFollowup: Boolean? = null,
    /**
     * An adopted Claude Code session the Mac can hand a message to as a peer message over its
     * messaging socket ("Send to live session"). Only set when the session passes the bridge's
     * guard; the ordinary follow-up stays refused either way.
     */
    @SerialName("can_send_live") val canSendLive: Boolean = false,
    /** Why an adopted Claude Code session can't be sent a live message; "" when it can. */
    @SerialName("live_send_unavailable") val liveSendUnavailable: String = "",
    /** The newest live messages sent to this session, oldest first, with what became of them. */
    @SerialName("peer_messages") val peerMessages: List<PeerMessage> = emptyList(),
    /** Set by the app: which paired computer ran this. */
    @Transient val hostId: String = "",
    @Transient val hostName: String = "",
) {
    val isActive get() = status == "queued" || status == "running"
    val key get() = Keys.of(hostId, id)
    val repoKey get() = Keys.of(hostId, repoId)

    /** A session an agent started on its own, which the Mac only watches (see docs/API.md). */
    val isAdopted get() = origin == ORIGIN_ADOPTED

    /** A run the Mac does with the OpenAI Codex CLI rather than Claude Code. */
    val isCodex get() = agent == AGENT_CODEX

    /** The agent doing the run, as the app names it in sentences ("Codex is working…"). */
    val agentName get() = if (isCodex) "Codex" else "Claude"

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

    /** "Send to live session" is offered on a live adopted run the bridge says passes its guard. */
    val showsSendLive get() = isAdopted && isActive && canSendLive

    /** The `cd … && claude --resume …` line to pick this session up by hand, when there is one. */
    val takeoverCommand get() = resumeCommand.takeIf { it.isNotBlank() && sessionId.isNotBlank() }
}

/** A message the Mac handed a live adopted session over its messaging socket. */
@Serializable
data class PeerMessage(
    @SerialName("msg_id") val msgId: String,
    val text: String = "",
    @SerialName("sent_at") val sentAt: String = "",
    /** sent, held, denied, expired, delivered, refused, dropped, or unconfirmed. */
    val status: String = "",
    val reason: String = "",
    /** The session's transcript shows the message: it reached Claude. */
    val confirmed: Boolean = false,
    @SerialName("confirmed_at") val confirmedAt: String = "",
    @SerialName("idle_at") val idleAt: String = "",
) {
    /** Bad news the person should see: the message did not, or may not, get through. */
    val failed get() = !confirmed && status in setOf("denied", "expired", "refused", "dropped", "unconfirmed")

    /** One short phrase for where the message stands. */
    val label: String get() = when {
        confirmed && idleAt.isNotBlank() -> "Delivered · session idle again"
        confirmed -> "Delivered"
        else -> when (status) {
            "sent" -> "Sent · waiting for the session"
            "held" -> "Held · waiting for approval on the Mac"
            "delivered" -> "Accepted · waiting for the transcript"
            "denied" -> "Declined on the Mac"
            "expired" -> "Expired before it was approved"
            "refused" -> "Refused: the session doesn't take peer messages"
            "dropped" -> "Dropped by the session"
            "unconfirmed" -> "Not confirmed · check the Mac"
            else -> status.replaceFirstChar { it.uppercase() }
        }
    }
}

/** `origin` of a run the Mac only watches; see "Adopted runs" in docs/API.md. */
const val ORIGIN_ADOPTED = "adopted"

/** `agent` of a run done with the OpenAI Codex CLI; [RunRequest.agent] to start one. */
const val AGENT_CODEX = "codex"

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
    val commit: String = "",
    val hostname: String = "",
    @SerialName("host_id") val hostId: String = "",
    @SerialName("host_name") val hostName: String = "",
    @SerialName("bd_version") val bdVersion: String = "",
    @SerialName("claude_version") val claudeVersion: String = "",
    @SerialName("codex_version") val codexVersion: String = "",
    /** Why beads can't be run with Codex on this computer, in a sentence; "" when they can. */
    @SerialName("codex_unavailable") val codexUnavailable: String = "",
    /** Why Chat can't run on this computer, in a sentence; "" when it can or nacred doesn't say. */
    @SerialName("agent_chat_unavailable") val agentChatUnavailable: String = "",
    /** Why Codex chats can't run on this computer; "" when they can or nacred doesn't say. */
    @SerialName("agent_chat_codex_unavailable") val agentChatCodexUnavailable: String = "",
    val repos: Int = 0,
    @SerialName("active_runs") val activeRuns: Int = 0,
    /** Optional capabilities this computer reports; see [Features]. */
    val features: List<String> = emptyList(),
) {
    /** True when this computer can run Refine, Draft and the assistant itself. */
    val runsClaude get() = Features.CLAUDE in features

    /** True when beads can be run with the Codex CLI on this computer. */
    val runsCodex get() = Features.CODEX in features

    /** Why "Implement via Codex" can't be used here, or null when it can. */
    val codexBlocker: String? get() = when {
        runsCodex -> null
        codexUnavailable.isNotBlank() -> codexUnavailable
        // An older nacred reports neither the feature nor a reason.
        else -> "This computer's nacred is too old to run Codex; update it"
    }
}

object Features {
    /** nacred can run the app's Claude features headless on its own subscription. */
    const val CLAUDE = "claude"

    /** Beads can be run with the OpenAI Codex CLI, installed and signed in on the computer. */
    const val CODEX = "codex"

    /** Chat (`/v1/chats`) can run here, with host permission prompts answered from the phone. */
    const val AGENT_CHAT = "agent_chat"

    /** Chats can run on Codex through `codex app-server`. */
    const val AGENT_CHAT_CODEX = "agent_chat_codex"
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
    /** Related dependencies from the new bead (a follow-on and the epic it came from). */
    val related: List<String>? = null,
    /** A found bead this effort replaces as a follow-on change; nacred closes it with a link. */
    @SerialName("follow_on_of") val followOnOf: String? = null,
    /** The `Found:` comment this effort answers; nacred marks it answered. */
    @SerialName("found_comment") val foundComment: FoundComment? = null,
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
    @Transient val goal: GoalOptions? = null,
    val mode: String? = null,
    val model: String? = null,
    /** An effort level of [agent]'s (see [RunAgent]); null leaves it to the agent's default. */
    val effort: String? = null,
    val instructions: String? = null,
    val worktree: Boolean? = null,
    /** [AGENT_CODEX] to run the bead with Codex; null runs it with Claude Code. */
    val agent: String? = null,
)

/** One image of a chat message as nacred takes it: its media type, file name and base64 bytes. */
@Serializable
data class AssistImage(
    @SerialName("media_type") val mediaType: String,
    val name: String,
    val data: String,
)

/** nacred's check that a round run's work is done: committed, on its branch, passing the gates. */
@Serializable
data class Verification(
    val ok: Boolean = false,
    val reason: String = "",
    val uncommitted: List<String> = emptyList(),
    val commits: Int = 0,
    val gates: List<GateResult> = emptyList(),
    val at: String = "",
)

/** One nacre.yaml gate or build command's outcome. */
@Serializable
data class GateResult(
    val name: String = "",
    val ok: Boolean = false,
    @SerialName("exit_code") val exitCode: Int = 0,
    @SerialName("timed_out") val timedOut: Boolean = false,
    @SerialName("duration_ms") val durationMs: Long = 0,
    val tail: String = "",
    val error: String = "",
)

/**
 * Several beads in one repository that nacred carries from a pre-flight review to one commit
 * that passes the repository's gates and builds (docs/API.md, Rounds).
 */
@Serializable
data class Round(
    val id: String,
    @SerialName("repo_id") val repoId: String = "",
    @SerialName("repo_name") val repoName: String = "",
    @SerialName("repo_path") val repoPath: String = "",
    val epic: String = "",
    val state: String = ROUND_PLANNING,
    @SerialName("paused_from") val pausedFrom: String = "",
    @SerialName("pause_reason") val pauseReason: String = "",
    /** Why a paused round stopped, and the ways forward; null on an older nacred. */
    val stop: RoundStop? = null,
    val target: String = "",
    val base: String = "",
    val branch: String = "",
    val workspace: String = "",
    val beads: List<RoundBead> = emptyList(),
    val plan: Plan? = null,
    val questions: List<Question> = emptyList(),
    val integration: IntegrationStep? = null,
    @SerialName("budget_usd") val budgetUsd: Double = 0.0,
    @SerialName("spent_usd") val spentUsd: Double = 0.0,
    @SerialName("overhead_usd") val overheadUsd: Double = 0.0,
    @SerialName("time_limit_minutes") val timeLimitMinutes: Int = 0,
    @SerialName("run_ids") val runIds: List<String> = emptyList(),
    @SerialName("created_at") val createdAt: String = "",
    @SerialName("started_at") val startedAt: String = "",
    @SerialName("updated_at") val updatedAt: String = "",
    @SerialName("ended_at") val endedAt: String = "",
    @SerialName("final_commit") val finalCommit: String = "",
    val version: String = "",
    val artifacts: List<String> = emptyList(),
    @SerialName("landed_commit") val landedCommit: String = "",
    val pushed: Boolean = false,
    /** Whether landing pushes [target], resolved from the round's nacre.yaml; null when nacred didn't say. */
    @SerialName("land_push") val landPush: Boolean? = null,
    @SerialName("auto_land") val autoLand: Boolean = false,
    @SerialName("land_when_ready") val landWhenReady: Boolean = false,
    /** Finish the epic into main after its children land. */
    val finish: Boolean = false,
    /** The epic whose branch this round merges into [target]; such a round has no beads. */
    @SerialName("finish_epic") val finishEpic: String = "",
    val error: String = "",
    /** Set by the app: which paired computer runs this round. */
    @Transient val hostId: String = "",
    @Transient val hostName: String = "",
) {
    val key get() = Keys.of(hostId, id)
    val repoKey get() = Keys.of(hostId, repoId)
    val isActive get() = state != ROUND_LANDED && state != ROUND_CANCELLED
    val openQuestions get() = questions.filter { it.answer.isBlank() }
    val mergesAutomatically get() = autoLand || landWhenReady
    val needsYou get() = state == ROUND_NEEDS_DECISION || (state == ROUND_READY && !mergesAutomatically) || state == ROUND_PAUSED
    /** Still going on its own: not landed or cancelled, and not stopped or waiting on the user. */
    val isWorking get() = isActive && !needsYou
    fun runKey(runId: String) = Keys.of(hostId, runId)
}

/**
 * The Activity tab's badge: runs working now, as the Runs list shows them, plus rounds working on
 * their own. A round waiting on a decision, a landing or a stop is left out: Inbox lists and counts it.
 */
fun activityBadgeCount(runs: List<Run>, rounds: List<Round>): Int =
    runs.count { it.isActive && it.inRunsList } + rounds.count { it.isWorking }

@Serializable
data class RoundBead(
    val id: String,
    /** Set on a spec section bead: the change its pinned contract belongs to. */
    @SerialName("spec_change_id") val specChangeId: String = "",
    val title: String = "",
    val type: String = "",
    val priority: Int = 2,
    val state: String = BEAD_WAITING,
    val after: List<String> = emptyList(),
    val needs: List<String> = emptyList(),
    val branch: String = "",
    val workspace: String = "",
    val base: String = "",
    @SerialName("run_id") val runId: String = "",
    @SerialName("fix_attempts") val fixAttempts: Int = 0,
    val stalls: Int = 0,
    val verification: Verification? = null,
    @SerialName("merge_commit") val mergeCommit: String = "",
    val reason: String = "",
    @SerialName("human_check") val humanCheck: String = "",
    val closed: Boolean = false,
    /** The kept branch of an earlier round this bead's work continues from. */
    @SerialName("carried_from") val carriedFrom: String = "",
    /** The later round that continued this bead's kept work. */
    @SerialName("picked_up_by") val pickedUpBy: String = "",
)

/** Why a round stopped after trying to recover by itself, and the ways forward. */
@Serializable
data class RoundStop(
    val kind: String = "",
    val reason: String = "",
    val phase: String = "",
    val bead: String = "",
    val branch: String = "",
    val diagnosis: StopDiagnosis? = null,
    val choices: List<StopChoice> = emptyList(),
    val attempted: List<String> = emptyList(),
    val spend: List<BeadSpend> = emptyList(),
    @SerialName("estimate_usd") val estimateUsd: Double = 0.0,
)

@Serializable
data class StopDiagnosis(val summary: String = "", val cause: String = "", @SerialName("cost_usd") val costUsd: Double = 0.0)

@Serializable
data class StopChoice(val id: String, val label: String = "", val explanation: String = "")

@Serializable
data class BeadSpend(val bead: String, @SerialName("spent_usd") val spentUsd: Double = 0.0, val done: Boolean = false)

const val CHOICE_RETRY_WITH_BUDGET = "retry-with-budget"
const val CHOICE_SKIP_BRANCH = "skip-branch"
const val CHOICE_SKIP_BEAD = "skip-bead"
const val CHOICE_CONTINUE = "continue"
const val CHOICE_RAISE_LIMITS = "raise-limits"
const val CHOICE_CANCEL = "cancel"

@Serializable
data class Plan(
    val summary: String = "",
    val beads: List<PlanBead> = emptyList(),
    val findings: List<Finding> = emptyList(),
    @SerialName("cost_usd") val costUsd: Double = 0.0,
)

@Serializable
data class PlanBead(
    val id: String,
    val areas: List<String> = emptyList(),
    val files: List<String> = emptyList(),
    @SerialName("depends_on") val dependsOn: List<String> = emptyList(),
    val needs: List<String> = emptyList(),
    @SerialName("human_check") val humanCheck: String = "",
)

@Serializable
data class Finding(
    val id: String = "",
    val kind: String = "",
    val beads: List<String> = emptyList(),
    val branch: String = "",
    val files: List<String> = emptyList(),
    val text: String = "",
    val accepted: Boolean = false,
    val unlinked: Boolean = false,
)

@Serializable
data class Question(
    val id: String,
    @SerialName("finding_id") val findingId: String = "",
    val kind: String = "",
    val beads: List<String> = emptyList(),
    val branch: String = "",
    val text: String = "",
    val options: List<String> = emptyList(),
    val answer: String = "",
    @SerialName("answered_at") val answeredAt: String = "",
    /** Why an earlier answer couldn't be carried out, when the question has come back. */
    val problem: String = "",
)

@Serializable
data class IntegrationStep(
    val bead: String = "",
    val phase: String = "",
    @SerialName("pre_merge") val preMerge: String = "",
    @SerialName("run_id") val runId: String = "",
    val attempts: Int = 0,
    val problem: String = "",
)

/** One line of a round's log. */
@Serializable
data class RoundEvent(
    val seq: Long = 0,
    val ts: String = "",
    val kind: String = "",
    val bead: String = "",
    val text: String = "",
    val commit: String = "",
    val gates: List<GateResult> = emptyList(),
)

/** POST /v1/repos/{repo}/rounds. */
@Serializable
data class StartRound(
    @Transient val goal: GoalOptions? = null,
    val beads: List<String> = emptyList(),
    val epic: String? = null,
    @SerialName("budget_usd") val budgetUsd: Double? = null,
    @SerialName("time_limit_minutes") val timeLimitMinutes: Int? = null,
    @SerialName("spec_change_id") val specChangeId: String? = null,
    @SerialName("spec_revision") val specRevision: String? = null,
    /** Land as soon as the round is verified, without waiting for a tap. */
    @SerialName("land_when_ready") val landWhenReady: Boolean? = null,
    val finish: Boolean = false,
)

/** What finishing an epic did: a round that merges its branch, or nothing to merge and the epic closed. */
@Serializable
data class EpicFinish(val round: Round? = null, val closed: Boolean = false)

/** The branch an epic's work lands on until the epic is finished. */
fun epicBranch(epic: String) = "epic/" + epic.lowercase()

const val ROUND_PLANNING = "planning"
const val ROUND_NEEDS_DECISION = "needs_decision"
const val ROUND_IMPLEMENTING = "implementing"
const val ROUND_INTEGRATING = "integrating"
const val ROUND_READY = "ready_to_land"
const val ROUND_LANDED = "landed"
const val ROUND_CANCELLED = "cancelled"
const val ROUND_PAUSED = "paused"

const val BEAD_WAITING = "waiting"
const val BEAD_RUNNING = "running"
const val BEAD_VERIFYING = "verifying"
const val BEAD_INTEGRATING = "integrating"
const val BEAD_INTEGRATED = "integrated"
const val BEAD_PARKED = "parked"
const val BEAD_SKIPPED = "skipped"

const val FINDING_OVERLAP = "overlap"
const val FINDING_ORDERING = "ordering"
const val FINDING_UNMERGED = "unmerged-work"

/** What a round's result adds to the commit it started from, for review before landing. */
@Serializable
data class RoundChanges(
    val base: String = "",
    val head: String = "",
    val commits: List<RoundCommit> = emptyList(),
    val files: List<RoundFile> = emptyList(),
    @SerialName("more_commits") val moreCommits: Int = 0,
) {
    val added get() = files.sumOf { it.added }
    val deleted get() = files.sumOf { it.deleted }
}

@Serializable
data class RoundCommit(val sha: String = "", val subject: String = "")

@Serializable
data class RoundFile(val path: String = "", val added: Int = 0, val deleted: Int = 0, val binary: Boolean = false)
