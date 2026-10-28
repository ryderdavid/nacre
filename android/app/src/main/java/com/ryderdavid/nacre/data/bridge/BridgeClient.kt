package com.ryderdavid.nacre.data.bridge

import com.ryderdavid.nacre.data.model.AssistRequest
import com.ryderdavid.nacre.data.model.AssistResult
import com.ryderdavid.nacre.data.model.Comment
import com.ryderdavid.nacre.data.model.DaemonUpdate
import com.ryderdavid.nacre.data.model.ChangeAction
import com.ryderdavid.nacre.data.model.ChangeEnvelope
import com.ryderdavid.nacre.data.model.CreateIssue
import com.ryderdavid.nacre.data.model.Health
import com.ryderdavid.nacre.data.model.Issue
import com.ryderdavid.nacre.data.model.Repo
import com.ryderdavid.nacre.data.model.Run
import com.ryderdavid.nacre.data.model.StartRound
import com.ryderdavid.nacre.data.model.RoundEvent
import com.ryderdavid.nacre.data.model.Round
import com.ryderdavid.nacre.data.model.RunEvent
import com.ryderdavid.nacre.data.model.RunRequest
import com.ryderdavid.nacre.data.model.UpdateIssue
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.Call
import okhttp3.Callback
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import okhttp3.sse.EventSource
import okhttp3.sse.EventSourceListener
import okhttp3.sse.EventSources
import java.io.IOException
import java.util.concurrent.TimeUnit
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

class BridgeException(message: String, val detail: String = "", val code: Int = 0) : IOException(message)

/** Events from `/v1/runs/{id}/stream` and `/v1/events`. */
sealed interface StreamMessage {
    data class Event(val event: RunEvent) : StreamMessage
    data class RunChanged(val run: Run) : StreamMessage
    data class RepoChanged(val repoId: String) : StreamMessage
    data class RoundChanged(val round: Round) : StreamMessage
    data object Open : StreamMessage
}

/** Messages from `/v1/rounds/{id}/stream`. */
sealed interface RoundMessage {
    data class Event(val event: RoundEvent) : RoundMessage
    data class Changed(val round: Round) : RoundMessage
}

/** Messages from a streamed `claude -p` turn on the computer. */
sealed interface AssistMessage {
    /** The id to reattach with if the connection drops; always first from a current nacred. */
    data class Turn(val id: String) : AssistMessage

    /** One line of the Mac's own transcript: a thought, a bd command, its output. */
    data class Event(val event: RunEvent) : AssistMessage

    /** The finished answer. */
    data class Done(val result: AssistResult) : AssistMessage

    /** Claude or nacred gave up; [error] is already phrased for a person. */
    data class Failed(val error: String) : AssistMessage
}

/** Thin typed client for nacred. One instance per (url, token). */
class BridgeClient(val baseUrl: String, private val token: String, base: OkHttpClient) {
    private val http = base.newBuilder().addInterceptor { chain ->
        chain.proceed(chain.request().newBuilder().header("Authorization", "Bearer $token").build())
    }.build()

    private val streamHttp = http.newBuilder().readTimeout(0, TimeUnit.MILLISECONDS).build()

    /**
     * Assistant turns notice a dead connection: nacred pings every 15 s, so a
     * silent minute means the link is gone and the turn should be reattached.
     */
    private val assistHttp = http.newBuilder().readTimeout(60, TimeUnit.SECONDS).build()

    @Serializable private data class Repos(val repos: List<Repo>)
    @Serializable data class Board(val repo: Repo, val issues: List<Issue>)
    @Serializable data class Detail(val issue: Issue, val comments: List<Comment> = emptyList())
    @Serializable private data class One(val issue: Issue)
    @Serializable private data class Comments(val comments: List<Comment>)
    @Serializable private data class Runs(val runs: List<Run>)
    @Serializable data class RunDetail(val run: Run, val events: List<RunEvent> = emptyList())
    @Serializable private data class Reason(val reason: String? = null)
    @Serializable private data class Text(val text: String)
    @Serializable private data class Dep(val issue_id: String, val depends_on_id: String, val type: String)
    @Serializable private data class FollowUp(val message: String, val mode: String? = null)
    @Serializable private data class ErrorBody(val error: String = "", val detail: String = "")

    suspend fun health(): Health = get("/v1/health")
    suspend fun daemonUpdate(): DaemonUpdate = updateCall(false)
    suspend fun startDaemonUpdate(): DaemonUpdate = updateCall(true)

    private suspend fun updateCall(start: Boolean): DaemonUpdate {
        val req = Request.Builder().url(url("/v1/daemon/update")).apply {
            if (start) post("{}".toRequestBody(JSON))
        }.build()
        val updateHttp = http.newBuilder().callTimeout(20, TimeUnit.SECONDS).build()
        val body = withContext(Dispatchers.IO) { updateHttp.newCall(req).await().use { it.checked() } }
        return json.decodeFromString(body)
    }
    suspend fun repos(refresh: Boolean = false): List<Repo> = get<Repos>("/v1/repos" + if (refresh) "?refresh=1" else "").repos
    suspend fun board(repo: String): Board = get("/v1/repos/$repo/issues")
    suspend fun detail(repo: String, id: String): Detail = get("/v1/repos/$repo/issues/${enc(id)}")
    suspend fun change(repo: String, id: String): ChangeEnvelope = try {
        get("/v1/repos/$repo/issues/${enc(id)}/change")
    } catch (e: BridgeException) {
        if (e.code == 404 || e.code == 405 || e.code == 501) ChangeEnvelope(reason = "Update nacred on this computer to develop OpenSpec changes here.") else throw e
    }
    suspend fun changeAction(repo: String, id: String, action: ChangeAction): ChangeEnvelope =
        send("POST", "/v1/repos/$repo/issues/${enc(id)}/change", action)
    suspend fun create(repo: String, body: CreateIssue): Issue = send<CreateIssue, One>("POST", "/v1/repos/$repo/issues", body).issue
    suspend fun update(repo: String, id: String, body: UpdateIssue): Issue = send<UpdateIssue, One>("PATCH", "/v1/repos/$repo/issues/${enc(id)}", body).issue
    suspend fun close(repo: String, id: String, reason: String? = null): Issue = send<Reason, One>("POST", "/v1/repos/$repo/issues/${enc(id)}/close", Reason(reason)).issue
    suspend fun reopen(repo: String, id: String): Issue = send<Reason, One>("POST", "/v1/repos/$repo/issues/${enc(id)}/reopen", Reason()).issue
    suspend fun comment(repo: String, id: String, text: String): List<Comment> = send<Text, Comments>("POST", "/v1/repos/$repo/issues/${enc(id)}/comments", Text(text)).comments
    suspend fun addDep(repo: String, issue: String, dependsOn: String, type: String = "blocks") {
        send<Dep, JsonObject>("POST", "/v1/repos/$repo/deps", Dep(issue, dependsOn, type))
    }
    suspend fun removeDep(repo: String, issue: String, dependsOn: String) {
        call<JsonObject>(Request.Builder().url(url("/v1/repos/$repo/deps").newBuilder()
            .addQueryParameter("issue_id", issue).addQueryParameter("depends_on_id", dependsOn).build()).delete().build())
    }

    suspend fun startRun(repo: String, id: String, req: RunRequest): Run = send("POST", "/v1/repos/$repo/issues/${enc(id)}/runs", req)
    /**
     * Adopted runs are listed only on request, and stale ones (sessions left open and quiet) on
     * a second request: they are fetched so a bead can still show its idle agent, and kept out of
     * the Runs list by [Run.inRunsList]. An older bridge ignores what it does not know.
     */
    suspend fun runs(repo: String? = null, issue: String? = null, active: Boolean = false, adopted: Boolean = true): List<Run> {
        val u = url("/v1/runs").newBuilder().apply {
            repo?.let { addQueryParameter("repo", it) }
            issue?.let { addQueryParameter("issue", it) }
            if (active) addQueryParameter("active", "1")
            if (adopted) addQueryParameter("include", "adopted,stale")
        }.build()
        return call<Runs>(Request.Builder().url(u).build()).runs
    }
    suspend fun run(id: String): RunDetail = get("/v1/runs/$id")
    suspend fun cancel(id: String): Run = send<Reason, Run>("POST", "/v1/runs/$id/cancel", Reason())
    suspend fun followUp(id: String, message: String, mode: String?): Run = send("POST", "/v1/runs/$id/followup", FollowUp(message, mode))

    // ---- rounds ----

    @Serializable private data class Rounds(val rounds: List<Round>)
    @Serializable data class RoundDetail(val round: Round, val events: List<RoundEvent> = emptyList())
    @Serializable private data class Answer(@SerialName("question_id") val questionId: String, val answer: String)
    @Serializable private data class PlanEdit(
        val unlink: List<List<String>> = emptyList(),
        @SerialName("accept_dependencies") val acceptDependencies: List<List<String>> = emptyList(),
    )
    @Serializable private data class Limits(@SerialName("budget_usd") val budgetUsd: Double = 0.0, @SerialName("time_limit_minutes") val timeLimitMinutes: Int = 0)

    suspend fun rounds(): List<Round> = get<Rounds>("/v1/rounds").rounds
    suspend fun round(id: String): RoundDetail = get("/v1/rounds/${enc(id)}")
    suspend fun startRound(repo: String, body: StartRound): Round = send("POST", "/v1/repos/$repo/rounds", body)
    suspend fun answer(id: String, questionId: String, answer: String): Round = send("POST", "/v1/rounds/${enc(id)}/answers", Answer(questionId, answer))
    suspend fun editPlan(id: String, unlink: List<Pair<String, String>> = emptyList(), accept: List<Pair<String, String>> = emptyList()): Round =
        send("POST", "/v1/rounds/${enc(id)}/plan", PlanEdit(unlink.map { listOf(it.first, it.second) }, accept.map { listOf(it.first, it.second) }))
    suspend fun setLimits(id: String, budgetUsd: Double, minutes: Int): Round = send("POST", "/v1/rounds/${enc(id)}/limits", Limits(budgetUsd, minutes))
    suspend fun land(id: String): Round = send<Reason, Round>("POST", "/v1/rounds/${enc(id)}/land", Reason())
    suspend fun cancelRound(id: String): Round = send<Reason, Round>("POST", "/v1/rounds/${enc(id)}/cancel", Reason())
    suspend fun resumeRound(id: String): Round = send<Reason, Round>("POST", "/v1/rounds/${enc(id)}/resume", Reason())
    suspend fun retryBead(id: String, bead: String): Round = send<Reason, Round>("POST", "/v1/rounds/${enc(id)}/beads/${enc(bead)}/retry", Reason())

    @Serializable private data class Choice(
        val choice: String,
        @SerialName("budget_usd") val budgetUsd: Double = 0.0,
        @SerialName("time_limit_minutes") val timeLimitMinutes: Int = 0,
    )

    /** Carries out one of a stopped round's choices. */
    suspend fun chooseRound(id: String, choice: String, budgetUsd: Double = 0.0, minutes: Int = 0): Round =
        send("POST", "/v1/rounds/${enc(id)}/choose", Choice(choice, budgetUsd, minutes))

    /** A round's log after [after], then its changes live. */
    fun roundStream(id: String, after: Long = 0): Flow<RoundMessage> = callbackFlow {
        val req = Request.Builder().url(url("/v1/rounds/${enc(id)}/stream?after=$after")).header("Accept", "text/event-stream").build()
        val source = EventSources.createFactory(streamHttp).newEventSource(req, object : EventSourceListener() {
            override fun onEvent(eventSource: EventSource, id: String?, type: String?, data: String) {
                val msg = runCatching {
                    when (type) {
                        "round" -> RoundMessage.Changed(json.decodeFromString<Round>(data))
                        else -> RoundMessage.Event(json.decodeFromString<RoundEvent>(data))
                    }
                }.getOrNull() ?: return
                trySend(msg)
            }

            override fun onClosed(eventSource: EventSource) {
                close()
            }

            override fun onFailure(eventSource: EventSource, t: Throwable?, response: Response?) {
                close(t ?: streamError(response))
            }
        })
        awaitClose { source.cancel() }
    }

    /** One Refine or Draft call on this computer's Claude subscription. */
    suspend fun claudeJson(repo: String, req: AssistRequest): AssistResult = send("POST", "/v1/repos/$repo/claude/json", req)

    /** One board-assistant turn on this computer, streamed as it runs. */
    fun claudeStream(repo: String, req: AssistRequest): Flow<AssistMessage> {
        val body: RequestBody = json.encodeToString(req).toRequestBody(JSON)
        return assistSse(Request.Builder().url(url("/v1/repos/$repo/claude/stream")).post(body))
    }

    /**
     * Picks a turn back up after the connection dropped: the events after [after], then
     * the rest live. The turn kept running on the computer in the meantime.
     */
    fun claudeResume(turn: String, after: Long): Flow<AssistMessage> =
        assistSse(Request.Builder().url(url("/v1/assist/${enc(turn)}/stream?after=$after")))

    /** Stops a turn nobody is waiting for any more. */
    suspend fun claudeCancel(turn: String) {
        withContext(Dispatchers.IO) {
            http.newCall(Request.Builder().url(url("/v1/assist/${enc(turn)}/cancel")).post(ByteArray(0).toRequestBody()).build())
                .await().close()
        }
    }

    private fun assistSse(builder: Request.Builder): Flow<AssistMessage> = callbackFlow {
        val request = builder.header("Accept", "text/event-stream").build()
        val source = EventSources.createFactory(assistHttp).newEventSource(request, object : EventSourceListener() {
            override fun onEvent(eventSource: EventSource, id: String?, type: String?, data: String) {
                val msg = runCatching {
                    when (type) {
                        "turn" -> AssistMessage.Turn(json.decodeFromString<JsonObject>(data)["turn_id"]!!.jsonPrimitive.content)
                        "done" -> AssistMessage.Done(json.decodeFromString<AssistResult>(data))
                        "error" -> AssistMessage.Failed(json.decodeFromString<ErrorBody>(data).error.ifBlank { "Claude failed on the computer" })
                        else -> AssistMessage.Event(json.decodeFromString<RunEvent>(data))
                    }
                }.getOrNull() ?: return
                trySend(msg)
            }

            override fun onClosed(eventSource: EventSource) {
                close()
            }

            override fun onFailure(eventSource: EventSource, t: Throwable?, response: Response?) {
                close(t ?: streamError(response))
            }
        })
        awaitClose { source.cancel() }
    }

    /** A streaming POST that answered with an error instead of events. */
    private fun streamError(response: Response?): Throwable {
        val code = response?.code ?: 0
        val body = response?.let { runCatching { it.body?.string() }.getOrNull() }.orEmpty()
        val err = runCatching { json.decodeFromString<ErrorBody>(body) }.getOrNull()
        return BridgeException(err?.error?.takeIf { it.isNotBlank() } ?: "The computer stopped answering (${code.takeIf { it != 0 } ?: "no response"})", code = code)
    }

    /** Live transcript of one run; replays events after [after] first. */
    fun runStream(id: String, after: Long = 0): Flow<StreamMessage> = sse("/v1/runs/$id/stream?after=$after")

    /** Global run + repo notifications. */
    fun events(): Flow<StreamMessage> = sse("/v1/events")

    // ---- plumbing ----

    private fun url(path: String) = (baseUrl.trimEnd('/') + path).toHttpUrl()

    private suspend inline fun <reified T> get(path: String): T = call(Request.Builder().url(url(path)).build())

    private suspend inline fun <reified B, reified T> send(method: String, path: String, body: B): T {
        val rb: RequestBody = json.encodeToString(body).toRequestBody(JSON)
        return call(Request.Builder().url(url(path)).method(method, rb).build())
    }

    private suspend inline fun <reified T> call(req: Request): T {
        val body = withContext(Dispatchers.IO) { http.newCall(req).await().use { resp -> resp.checked() } }
        return json.decodeFromString(body)
    }

    private fun Response.checked(): String {
        val text = body?.string().orEmpty()
        if (!isSuccessful) {
            val err = runCatching { json.decodeFromString<ErrorBody>(text) }.getOrNull()
            val msg = err?.error?.takeIf { it.isNotBlank() } ?: when (code) {
                401 -> "The Mac rejected the pairing token"
                404 -> "Not found on the Mac"
                else -> "Bridge error $code"
            }
            throw BridgeException(msg, err?.detail.orEmpty(), code)
        }
        return text
    }

    private fun sse(path: String): Flow<StreamMessage> = callbackFlow {
        val req = Request.Builder().url(url(path)).header("Accept", "text/event-stream").build()
        val source = EventSources.createFactory(streamHttp).newEventSource(req, object : EventSourceListener() {
            override fun onOpen(eventSource: EventSource, response: Response) {
                trySend(StreamMessage.Open)
            }

            override fun onEvent(eventSource: EventSource, id: String?, type: String?, data: String) {
                val msg = runCatching {
                    when (type) {
                        "run" -> StreamMessage.RunChanged(json.decodeFromString<Run>(data))
                        "repo" -> StreamMessage.RepoChanged(json.decodeFromString<JsonObject>(data)["repo_id"]!!.jsonPrimitive.content)
                        "round" -> StreamMessage.RoundChanged(json.decodeFromString<Round>(data))
                        else -> StreamMessage.Event(json.decodeFromString<RunEvent>(data))
                    }
                }.getOrNull() ?: return
                trySend(msg)
            }

            override fun onClosed(eventSource: EventSource) {
                close()
            }

            override fun onFailure(eventSource: EventSource, t: Throwable?, response: Response?) {
                val err = t ?: BridgeException("Stream failed (${response?.code ?: "no response"})", code = response?.code ?: 0)
                close(err)
            }
        })
        awaitClose { source.cancel() }
    }

    companion object {
        val JSON = "application/json; charset=utf-8".toMediaType()
        val json = Json { ignoreUnknownKeys = true; explicitNulls = false; encodeDefaults = false; coerceInputValues = true }
        private fun enc(id: String) = java.net.URLEncoder.encode(id, "UTF-8").replace("+", "%20")

        /** Unauthenticated probe used while pairing; null when nothing answers. */
        suspend fun ping(base: OkHttpClient, baseUrl: String): Health? = withContext(Dispatchers.IO) {
            runCatching {
                base.newCall(Request.Builder().url(baseUrl.trimEnd('/') + "/v1/ping").build()).await().use { resp ->
                    if (resp.isSuccessful) json.decodeFromString<Health>(resp.body?.string().orEmpty()) else null
                }
            }.getOrNull()
        }
    }
}

suspend fun Call.await(): Response = suspendCancellableCoroutine { cont ->
    enqueue(object : Callback {
        override fun onResponse(call: Call, response: Response) = cont.resume(response)
        override fun onFailure(call: Call, e: IOException) {
            if (!cont.isCancelled) cont.resumeWithException(e)
        }
    })
    cont.invokeOnCancellation { runCatching { cancel() } }
}
