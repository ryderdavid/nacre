package com.ryderdavid.nacre.data.claude

import com.ryderdavid.nacre.data.Connection
import com.ryderdavid.nacre.data.NacreRepository
import com.ryderdavid.nacre.data.bridge.AssistMessage
import com.ryderdavid.nacre.data.bridge.BridgeException
import com.ryderdavid.nacre.data.friendly
import com.ryderdavid.nacre.data.model.AssistBoardRef
import com.ryderdavid.nacre.data.model.AssistRequest
import com.ryderdavid.nacre.data.model.AssistResult
import com.ryderdavid.nacre.data.model.Keys
import com.ryderdavid.nacre.data.model.RunEvent
import com.ryderdavid.nacre.data.settings.AppSettings
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.decodeFromJsonElement
import java.io.IOException
import java.util.UUID

/**
 * Runs the Claude features as `claude -p` on a paired computer, which spends
 * Ryder's Claude subscription instead of API credit. nacred does the invoking;
 * the prompts, schemas and result types are the same ones the API backend
 * uses, so Refine's diff review and the assistant's approval flow are unchanged.
 */
class MacBackend(
    private val repo: MacLink,
    private val settings: suspend () -> AppSettings,
) : ClaudeBackend {
    constructor(repo: NacreRepository, settings: suspend () -> AppSettings) : this(repo.macLink(), settings)

    override val runner = ClaudeRunner.MAC

    /** [images] go in the request's `images`, in marker order, with their markers in the prompt. */
    override suspend fun refine(
        repoKey: String, ctx: BeadContext, instruction: String, history: List<DraftTurn>, images: ChatTurnImages,
    ): RefineProposal =
        json(repoKey, Prompts.refine(ctx, instruction, history, images.current, images.request.marks), ClaudeService.refineSchema, images = images.request.images)

    /** On the Mac an answer can come from the code too: the call may read the repository, never write. */
    override suspend fun ask(
        repoKey: String, ctx: BeadContext, history: List<DraftTurn>, question: String, images: ChatTurnImages,
    ): String =
        json<AskReply>(
            repoKey, Prompts.ask(ctx, history, question, readsRepo = true, images.current, images.request.marks),
            ClaudeService.askSchema, Prompts.ADVISOR, images = images.request.images,
        ).answer.ifBlank { throw ClaudeException("The Mac answered without an answer. Try again.") }

    override suspend fun draft(repoKey: String, repoName: String, rough: String, parentTitle: String?, existingTitles: List<String>): BeadDraft =
        json(repoKey, Prompts.draft(repoName, rough, parentTitle, existingTitles), ClaudeService.draftSchema)

    override suspend fun reviseDraft(
        repoKey: String, repoName: String, current: BeadDraft, history: List<DraftTurn>, message: String,
        parentTitle: String?, existingTitles: List<String>,
    ): BeadDraft = json(repoKey, Prompts.reviseDraft(repoName, current, history, message, parentTitle, existingTitles), ClaudeService.draftSchema)

    override fun session(board: AssistantBoard): AssistantSession = MacSession(repo, settings, board)

    /**
     * One structured call, decoded with the same serializers as the API path; it may read the repository, never write.
     * [images] are sent numbered 1..N in their order; the call is never kept as a session, so they leave none behind.
     */
    private suspend inline fun <reified T> json(
        repoKey: String, prompt: String, schema: Map<String, Any>,
        system: String = Prompts.EDITOR, readRepo: Boolean = true, images: List<ImagePayload> = emptyList(),
    ): T {
        val s = settings()
        if (images.isNotEmpty()) checkImages(repo, repoKey)
        val out = translate(images = images.isNotEmpty()) {
            repo.claudeJson(
                repoKey,
                AssistRequest(
                    prompt = prompt, system = system, schema = schema.toSchemaJson(),
                    model = s.claudeModel, effort = s.claudeEffort, bdReads = readRepo, readRepo = readRepo,
                    images = images.map { it.toAssist() },
                ),
            )
        }
        val answer = out.output ?: throw ClaudeException("The Mac answered without a proposal. Try again, or switch to your API key in Settings.")
        return ClaudeService.json.decodeFromJsonElement(answer)
    }
}

/**
 * A conversation held by Claude Code on the Mac. Each turn is one `claude -p`
 * call that reads the board with read-only bd commands and answers with prose
 * plus the writes it would like to make; `--resume` keeps the thread.
 *
 * The writes come back as proposals rather than tool calls, so they land in the
 * app's normal approval flow and are applied by the app through nacred's REST
 * API — Claude Code on the Mac is never allowed to change a bead itself.
 */
class MacSession(
    private val repo: MacLink,
    private val settings: suspend () -> AppSettings,
    private val board: AssistantBoard,
) : AssistantSession {
    private var sessionId = ""

    /**
     * The images and decoded bytes this conversation has sent. The resumed
     * session holds them all, so the conversation as a whole keeps to one
     * request's budget. A failed turn doesn't give them back: the session may
     * hold them already.
     */
    private var sentImages = 0
    private var sentBytes = 0L

    /** A turn that lost its connection, kept so Retry can pick it back up. */
    private var pending: TurnProgress? = null

    /** Across boards, the computer this conversation runs on; its session lives there. */
    private var acrossHost: String? = null

    /** Boards already reported as unreadable in this conversation, so each is said once. */
    private val reported = mutableSetOf<String>()

    override fun reset() {
        sessionId = ""
        pending = null
        acrossHost = null
        reported.clear()
        sentImages = 0
        sentBytes = 0L
    }

    override fun imageLimit(count: Int, bytes: Long): String? = when {
        count == 0 -> null
        board.runKey == null -> ImageReasons.ACROSS_BOARDS
        sentImages + count > ImageBudget.REQUEST_IMAGES || sentBytes + bytes > ImageBudget.REQUEST_BYTES -> ImageReasons.CONVERSATION_LIMIT
        else -> null
    }

    override suspend fun turn(input: TurnInput, sink: TurnSink): TurnResult {
        val s = settings()
        // Only this turn's images: the resumed session already holds the earlier ones.
        val images = (input as? TurnInput.Say)?.images.orEmpty()
        val prompt = when (input) {
            // Markers are numbered within the turn, matching its own images.
            is TurnInput.Say -> withMarkers(
                input.text, images.map { it.image },
                images.withIndex().associate { (i, p) -> p.image.id to ImageMark.Sent(i + 1) },
            )
            is TurnInput.Answers -> Prompts.outcomes(input.answers)
        }
        // A Retry that picks a lost turn back up was counted when it was sent.
        val reattaching = pending?.let { it.prompt == prompt && it.turn != null } == true
        if (!reattaching) imageLimit(images.size, images.byteCount())?.let { throw ClaudeException(it) }
        val base = AssistRequest(
            prompt = prompt,
            schema = Prompts.proposalsSchema(board.tools).toSchemaJson(),
            model = s.claudeModel,
            effort = s.claudeEffort,
            sessionId = sessionId,
            bdReads = true,
            keep = true,
            // This backend can't call the needs_you read tool, so nacred adds the list.
            needsYou = true,
        )
        val runKey: String
        val req: AssistRequest
        val start: (AssistRequest) -> Flow<AssistMessage>
        val single = board.runKey
        if (single != null) {
            runKey = single
            // One board's chat runs in its repository and can read the code there too.
            if (images.isNotEmpty()) checkImages(repo, runKey)
            req = base.copy(system = Prompts.macAssistant(board.system(), board.tools), readRepo = true, images = images.map { it.toAssist() })
            start = { repo.claudeStream(runKey, it) }
        } else {
            // Across boards: one call on one computer reads every board it holds.
            val span = board.span?.invoke() ?: BoardSpan(emptyList(), "")
            val plan = planAcross(span.boards, s.hosts.map { it.id }, acrossHost)
                ?: throw ClaudeUnavailableException("No boards are reachable. Pair a computer in Settings, or check that nacred is running.")
            acrossHost = plan.hostId
            runKey = plan.read.first().second
            val here = repo.hostName(plan.hostId).ifBlank { "this computer" }
            val elsewhere = plan.elsewhere.map { b ->
                val on = b.keys.map { repo.hostName(Keys.host(it)).ifBlank { "another computer" } }.distinct()
                "${b.name}: only on ${on.joinToString(" and ").ifBlank { "another computer" }}, not on $here"
            }
            report(sink, elsewhere)
            req = base.copy(
                system = Prompts.macAcrossBoards(board.system(), board.tools, span.overview, here, elsewhere),
                boards = plan.read.map { (name, key) -> AssistBoardRef(Keys.id(key), name) },
            )
            start = { repo.claudeStreamAcross(plan.hostId, it) }
        }

        val host = repo.hostName(Keys.host(runKey)).ifBlank { "the computer" }
        // A Retry of a turn that lost its connection picks the same turn back up: it
        // carried on on the computer, and its answer is probably waiting there.
        var progress = pending?.takeIf { it.prompt == req.prompt } ?: TurnProgress(req.prompt)
        pending = null
        val reconnecting = Reconnecting(sink, host)
        // Images count once they are sent; picking a lost turn back up sends nothing new.
        if (progress.turn == null) {
            sentImages += images.size
            sentBytes += images.byteCount()
        }

        val outcome = try {
            translate(images = images.isNotEmpty()) {
                val resumable = progress.turn
                try {
                    val first = if (resumable != null) repo.claudeResume(runKey, resumable, progress.lastSeq) else start(req)
                    followTurn(first, progress, resume = { t, after -> repo.claudeResume(runKey, t, after) }, relay = { relay(it, sink) }, dropped = reconnecting, noted = { report(sink, it) })
                } catch (e: BridgeException) {
                    if (resumable == null || e.code != 404) throw e
                    // The computer has forgotten that turn; ask again.
                    progress = TurnProgress(req.prompt)
                    followTurn(start(req), progress, resume = { t, after -> repo.claudeResume(runKey, t, after) }, relay = { relay(it, sink) }, dropped = reconnecting, noted = { report(sink, it) })
                }
            }
        } catch (e: kotlinx.coroutines.CancellationException) {
            // Stopped: Claude shouldn't keep spending the subscription on an answer nobody wants.
            progress.turn?.let { t -> withContext(NonCancellable) { withTimeoutOrNull(3_000) { runCatching { repo.claudeCancel(runKey, t) } } } }
            throw e
        } catch (e: TurnLostException) {
            reconnecting.gaveUp()
            if (progress.turn != null) pending = progress
            throw ClaudeUnavailableException(
                if (progress.turn != null) "Lost the connection to $host. Claude kept working there; Retry picks the answer back up."
                else "Lost the connection to $host before Claude finished.",
                canRetry = true,
            )
        }
        reconnecting.back()

        val result = when (outcome) {
            is TurnOutcome.Failed -> throw ClaudeException(outcome.error)
            is TurnOutcome.Done -> outcome.result
        }
        if (result.sessionId.isNotBlank()) sessionId = result.sessionId

        val (reply, calls) = result.output?.let { runCatching { macAnswer(it) }.getOrNull() }
            ?: throw ClaudeException("Claude's answer didn't arrive in the shape the app expects.")

        if (reply.isNotBlank()) {
            sink.newBlock()
            sink.text(reply)
        }
        return TurnResult(calls, if (calls.isEmpty()) TurnStop.DONE else TurnStop.CALLS)
    }

    /**
     * Says which boards this conversation can't read, once each, as a failed
     * step: a board left out is reported, never skipped in silence. Claude is
     * told the same in its prompt, so its answer says so too.
     */
    private fun report(sink: TurnSink, boards: List<String>) {
        val fresh = boards.filter { reported.add(it) }
        if (fresh.isEmpty()) return
        val id = "boards-" + UUID.randomUUID().toString().take(8)
        val n = fresh.size
        sink.aside(id, BOARDS_STEP, if (n == 1) "1 board can't be read on the Mac" else "$n boards can't be read on the Mac")
        sink.asideDone(id, fresh.joinToString("\n"), true)
    }

    /**
     * Shows the Mac's own steps as they happen. Assistant text is deliberately
     * dropped: with a JSON schema in play those blocks are drafts of the
     * structured answer, and the finished `reply` is what Ryder should read.
     */
    private fun relay(event: RunEvent, sink: TurnSink) {
        when (event.kind) {
            "thinking" -> {
                sink.newBlock()
                sink.thinking(event.text)
            }
            // StructuredOutput is how claude -p hands back the schema answer, shown below as the reply.
            "tool_use" -> if (event.tool != STRUCTURED_OUTPUT) sink.aside(event.toolId.ifBlank { event.seq.toString() }, step(event.tool, event.text), event.text)
            "tool_result" -> sink.asideDone(event.toolId, event.text, event.isError == true)
        }
    }

    companion object {
        /** The names shown for a step the Mac ran itself: reading beads, code or history. */
        const val BD_TOOL = "bd"
        const val CODE_TOOL = "code"
        const val GIT_TOOL = "git"

        fun step(tool: String, text: String): String = when {
            tool == "Read" || tool == "Grep" || tool == "Glob" -> CODE_TOOL
            text.trimStart().startsWith("git") -> GIT_TOOL
            else -> BD_TOOL
        }

        private const val STRUCTURED_OUTPUT = "StructuredOutput"
    }
}

@Serializable
internal data class MacAnswer(val reply: String = "", val proposals: List<MacProposal> = emptyList())

@Serializable
internal data class MacProposal(val tool: String = "", val input: JsonObject = JsonObject(emptyMap()))

/**
 * Reads a Mac turn's structured answer: the prose for Ryder, and the write
 * calls it proposes. Anything that isn't a write tool is dropped — a read
 * belongs to the Mac, which does its own with bd, so a model asking for one
 * here is confused and the app will not run it.
 */
internal fun macAnswer(output: JsonObject, turn: String = UUID.randomUUID().toString().take(8)): Pair<String, List<ToolRequest>> {
    val answer = ClaudeService.json.decodeFromJsonElement<MacAnswer>(output)
    val calls = answer.proposals
        .filter { it.tool in WRITE_TOOLS }
        .mapIndexed { i, p -> ToolRequest("mac-$turn-$i", p.tool, p.input) }
    return answer.reply to calls
}

/**
 * Where a turn across boards runs: [hostId], and the boards it reads there as
 * (name, repo key). [elsewhere] are the boards with no checkout on that computer.
 */
internal data class AcrossPlan(val hostId: String, val read: List<Pair<String, String>>, val elsewhere: List<SpanBoard>)

/**
 * Picks the computer a turn across boards runs on: [pinned] if it still holds
 * a board (a conversation's session lives on one computer), otherwise the one
 * holding the most boards, earlier in [hostOrder] on a tie. Null when no
 * computer holds any.
 */
internal fun planAcross(boards: List<SpanBoard>, hostOrder: List<String>, pinned: String? = null): AcrossPlan? {
    fun on(host: String) = boards.mapNotNull { b -> b.keys.firstOrNull { Keys.host(it) == host }?.let { b.name to it } }
    val hosts = boards.flatMap { b -> b.keys.map(Keys::host) }.distinct()
    if (hosts.isEmpty()) return null
    val rank = hostOrder.withIndex().associate { (i, h) -> h to i }
    val host = pinned?.takeIf { it in hosts }
        ?: hosts.sortedWith(compareByDescending<String> { h -> on(h).size }.thenBy { rank[it] ?: Int.MAX_VALUE }).first()
    return AcrossPlan(host, on(host), boards.filter { b -> b.keys.none { Keys.host(it) == host } })
}

/** How far a Mac turn has got, so it can be reattached after a dropped connection. */
internal class TurnProgress(val prompt: String) {
    var turn: String? = null
    var lastSeq = 0L
    var events = 0
}

internal sealed interface TurnOutcome {
    data class Done(val result: AssistResult) : TurnOutcome
    data class Failed(val error: String) : TurnOutcome
}

/** The connection dropped and couldn't be brought back in time. */
internal class TurnLostException(cause: Throwable) : IOException("connection lost", cause)

/** Hears about a dropped connection: [lost] when it goes, [back] when a reattach works. */
internal interface DropListener {
    fun lost(attempt: Int)
    fun back()
}

/** How long to keep trying to reattach before telling the user. */
internal const val RECONNECT_BUDGET_MS = 3 * 60_000L

/** 1 s, 2 s, 4 s, 8 s, then every 10 s. */
internal fun reconnectDelay(attempt: Int): Long = minOf(10_000L, 1_000L shl (attempt - 1).coerceIn(0, 4))

/**
 * Follows one Mac turn to its answer, reattaching when the connection drops.
 *
 * nacred keeps the turn running when the phone goes away, so a drop costs
 * nothing as long as the app comes back: [resume] replays the events after
 * the last one seen, which are de-duplicated by seq. It gives up with
 * [TurnLostException] after [budgetMs] of failed attempts, always making one
 * attempt after each pause — a phone waking from sleep finds the answer
 * waiting. A failure that isn't the network, such as a 404 or a bad token,
 * ends it at once.
 */
internal suspend fun followTurn(
    first: Flow<AssistMessage>,
    progress: TurnProgress,
    resume: (turn: String, after: Long) -> Flow<AssistMessage>,
    relay: (RunEvent) -> Unit,
    dropped: DropListener,
    noted: (unreadable: List<String>) -> Unit = {},
    budgetMs: Long = RECONNECT_BUDGET_MS,
    pause: suspend (attempt: Int) -> Unit = { delay(reconnectDelay(it)) },
    now: () -> Long = System::currentTimeMillis,
): TurnOutcome {
    var stream = first
    var attempt = 0
    var lostAt = 0L
    while (true) {
        var outcome: TurnOutcome? = null
        try {
            stream.collect { msg ->
                if (attempt > 0) {
                    attempt = 0
                    dropped.back()
                }
                when (msg) {
                    is AssistMessage.Turn -> {
                        progress.turn = msg.id
                        if (msg.unreadable.isNotEmpty()) noted(msg.unreadable)
                    }
                    is AssistMessage.Event -> {
                        val seq = msg.event.seq
                        if (seq == 0L || seq > progress.lastSeq) {
                            if (seq > 0) progress.lastSeq = seq
                            progress.events++
                            relay(msg.event)
                        }
                    }
                    is AssistMessage.Done -> outcome = TurnOutcome.Done(msg.result)
                    is AssistMessage.Failed -> outcome = TurnOutcome.Failed(msg.error)
                }
            }
            outcome?.let { return it }
            throw IOException("The computer closed the stream before Claude finished")
        } catch (e: IOException) {
            if (e is BridgeException && e.code != 0) throw e
            val turn = progress.turn
            // Nothing to reattach to: either nothing started (let the caller say the computer
            // can't be reached) or an older nacred that can't resume.
            if (turn == null) throw if (progress.events == 0) e else TurnLostException(e)
            if (attempt == 0) lostAt = now()
            else if (now() - lostAt > budgetMs) throw TurnLostException(e)
            attempt++
            dropped.lost(attempt)
            pause(attempt)
            stream = resume(turn, progress.lastSeq)
        }
    }
}

/** Shows a dropped connection in the conversation as a step of its own. */
private class Reconnecting(private val sink: TurnSink, private val host: String) : DropListener {
    private var id: String? = null

    override fun lost(attempt: Int) {
        if (id != null) return
        id = "reconnect-" + UUID.randomUUID().toString().take(8)
        sink.aside(id!!, CONNECTION_STEP, "The connection to $host dropped")
    }

    override fun back() {
        id?.let { sink.asideDone(it, "Reconnected; Claude kept working on $host.", false) }
        id = null
    }

    fun gaveUp() {
        id?.let { sink.asideDone(it, "Couldn't reconnect.", true) }
        id = null
    }
}

/** The step name for boards a turn across boards couldn't read. */
const val BOARDS_STEP = "boards"

/** The step name for a dropped-and-recovered connection. */
const val CONNECTION_STEP = "connection"

/**
 * Turns a bridge failure into words about this feature. A computer that can't
 * be reached at all is worth trying again, and the app offers the API key
 * instead; one that refused the request is not.
 */
internal suspend fun <T> translate(images: Boolean = false, block: suspend () -> T): T = try {
    block()
} catch (e: BridgeException) {
    // nacred refused the images (400) or the body (413): its reason names the image or the limit.
    if (images && (e.code == 400 || e.code == 413)) throw ClaudeException("The computer didn't accept the images: ${e.message.orEmpty()}")
    throw translated(e)
} catch (e: ClaudeException) {
    throw e
} catch (e: ClaudeUnavailableException) {
    throw e
} catch (e: TurnLostException) {
    throw e
} catch (e: kotlinx.coroutines.CancellationException) {
    throw e
} catch (e: Exception) {
    throw translated(e)
}

private fun translated(e: Exception): ClaudeUnavailableException {
    val friendly = e.friendly()
    val tooOld = friendly.contains("no such route")
    val network = e is IOException && (e !is BridgeException || e.code == 0)
    return ClaudeUnavailableException(
        when {
            tooOld -> "That computer's nacred is too old to run Claude for the app. Update nacred, or use your API key."
            else -> friendly
        },
        offerApiKey = true,
        canRetry = network,
    )
}

/** Refuses an image send at the moment it is made when [repoKey]'s computer doesn't list `chat_images`. */
private fun checkImages(repo: MacLink, repoKey: String) {
    val host = Keys.host(repoKey)
    macImageBlocker(repo.hostName(host).ifBlank { "this computer" }, repo.features(host))?.let { throw ClaudeException(it) }
}

/** What the Mac backend needs from the paired computers; [NacreRepository] in the app. */
interface MacLink {
    suspend fun claudeJson(key: String, req: AssistRequest): AssistResult
    fun claudeStream(key: String, req: AssistRequest): Flow<AssistMessage>
    fun claudeStreamAcross(hostId: String, req: AssistRequest): Flow<AssistMessage>
    fun claudeResume(key: String, turn: String, after: Long): Flow<AssistMessage>
    suspend fun claudeCancel(key: String, turn: String)
    fun hostName(hostId: String): String

    /** The features computer [hostId] reported; null while it isn't connected. */
    fun features(hostId: String): List<String>?
}

internal fun NacreRepository.macLink(): MacLink = object : MacLink {
    override suspend fun claudeJson(key: String, req: AssistRequest) = this@macLink.claudeJson(key, req)
    override fun claudeStream(key: String, req: AssistRequest) = this@macLink.claudeStream(key, req)
    override fun claudeStreamAcross(hostId: String, req: AssistRequest) = this@macLink.claudeStreamAcross(hostId, req)
    override fun claudeResume(key: String, turn: String, after: Long) = this@macLink.claudeResume(key, turn, after)
    override suspend fun claudeCancel(key: String, turn: String) { this@macLink.claudeCancel(key, turn) }
    override fun hostName(hostId: String) = this@macLink.hostName(hostId)
    override fun features(hostId: String) =
        (hosts.value.firstOrNull { it.host.id == hostId }?.connection as? Connection.Online)?.health?.features
}
