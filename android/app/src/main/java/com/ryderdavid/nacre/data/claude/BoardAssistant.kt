package com.ryderdavid.nacre.data.claude

import com.ryderdavid.nacre.data.settings.AppSettings
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.doubleOrNull
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.intOrNull
import java.util.UUID

/** The app side of the assistant's tools: reads run freely, writes only after approval. */
interface BoardTools {
    /** The repo key this board lives under, used to reach its computer. */
    val boardKey: String
    val boardName: String
    suspend fun listBeads(lane: String?, query: String?, limit: Int): String
    suspend fun getBead(id: String): String
    suspend fun updateBead(id: String, fields: JsonObject): String
    suspend fun createBead(fields: JsonObject): String
    suspend fun closeBead(id: String, reason: String?): String
    suspend fun addComment(id: String, text: String): String
    suspend fun addDependency(issueId: String, dependsOnId: String): String
    suspend fun runInClaudeCode(id: String, instructions: String?): String

    /** Hands several beads to the board's computer as one round. */
    suspend fun startRound(ids: List<String>, budgetUsd: Double?): String = "Rounds aren't available on this board."
}

/**
 * Picks the board a tool call acts on. A conversation either covers one board
 * ([SingleBoard]) or every board on the boards screen ([AllBoardsRouter]); in the
 * second case the tools gain a `board` argument and a way to list the boards.
 */
interface BoardRouter {
    /** The board, when the conversation is about exactly one; null when it spans all of them. */
    val only: BoardTools?

    /** The board named [board]; throws with the choices when the name is missing or unclear. */
    suspend fun resolve(board: String?): BoardTools

    /** The boards the assistant can reach, as compact lines. */
    suspend fun listBoards(query: String?): String

    /** Beads from [board], or from every board when it is null. */
    suspend fun listBeads(board: String?, lane: String?, query: String?, limit: Int): String
}

/** A conversation about one board, where no tool call names a board. */
class SingleBoard(private val tools: BoardTools) : BoardRouter {
    override val only get() = tools
    override suspend fun resolve(board: String?) = tools
    override suspend fun listBoards(query: String?) = tools.boardName
    override suspend fun listBeads(board: String?, lane: String?, query: String?, limit: Int) = tools.listBeads(lane, query, limit)
}

sealed interface ChatItem {
    val key: String

    data class User(val text: String, override val key: String = UUID.randomUUID().toString()) : ChatItem
    data class Assistant(val text: String, val streaming: Boolean, override val key: String = UUID.randomUUID().toString()) : ChatItem
    data class Thinking(val text: String, val streaming: Boolean, override val key: String = UUID.randomUUID().toString()) : ChatItem
    data class ToolCall(
        val id: String,
        val name: String,
        val summary: String,
        val input: JsonObject,
        val state: State,
        val result: String = "",
        /** The board the call acts on; empty until a cross-board call resolves its board. */
        val boardKey: String = "",
        override val key: String = id,
    ) : ChatItem {
        enum class State { RUNNING, NEEDS_APPROVAL, DONE, FAILED, REJECTED }
        val isWrite get() = name in WRITE_TOOLS
    }

    /**
     * Something went wrong. [offerApiKey] marks the failures the user can fix
     * by running Claude on this phone's API key instead, and [canRetry] the
     * ones where asking again is likely to work.
     */
    data class Error(
        val text: String,
        val offerApiKey: Boolean = false,
        val canRetry: Boolean = false,
        override val key: String = UUID.randomUUID().toString(),
    ) : ChatItem
}

/**
 * A conversation with Claude about one board, or about every board at once — [router]
 * decides which. [backends] decides each new
 * conversation's home — the Claude API with this phone's key, or Claude Code
 * on the board's own computer — and the loop below is the same either way:
 * Claude asks for tools, write tools wait for the user's approval unless
 * [autoApprove] is on, and the app makes every change itself.
 */
class BoardAssistant(
    private val backends: ClaudeBackends,
    private val settings: suspend () -> AppSettings,
    private val router: BoardRouter,
    private val scope: CoroutineScope,
    /** Idle time after which the next message starts a new conversation; null keeps it forever. */
    private val idleResetMs: Long? = null,
    private val now: () -> Long = System::currentTimeMillis,
) {
    private val everyBoard = router.only == null
    private val toolSpecs = assistantTools(everyBoard)

    private val approvals = mutableMapOf<String, CompletableDeferred<Boolean>>()

    private var session: AssistantSession? = null
    private var runner: ClaudeRunner? = null

    private val _items = MutableStateFlow<List<ChatItem>>(emptyList())
    val items: StateFlow<List<ChatItem>> = _items.asStateFlow()

    private val _busy = MutableStateFlow(false)
    val busy: StateFlow<Boolean> = _busy.asStateFlow()

    val autoApprove = MutableStateFlow(false)
    private var job: Job? = null

    /** When the user or Claude last said anything, for [idleResetMs]. */
    private var lastActive = 0L

    /** The input of the turn in flight, which [retry] sends again. */
    private var retryInput: TurnInput? = null

    // Which entry each kind of streamed output is currently appending to.
    private var textKey: String? = null
    private var thinkKey: String? = null

    private val sink = object : TurnSink {
        override fun newBlock() {
            textKey = null
            thinkKey = null
        }

        override fun text(delta: String) {
            textKey = appendText(textKey, delta)
        }

        override fun thinking(delta: String) {
            thinkKey = appendThinking(thinkKey, delta)
        }

        override fun aside(id: String, name: String, summary: String) {
            _items.update { it + ChatItem.ToolCall(id, name, summary, JsonObject(emptyMap()), ChatItem.ToolCall.State.RUNNING) }
        }

        override fun asideDone(id: String, result: String, isError: Boolean) {
            setState(id, if (isError) ChatItem.ToolCall.State.FAILED else ChatItem.ToolCall.State.DONE, result)
        }
    }

    fun send(text: String) {
        if (text.isBlank() || _busy.value) return
        val said = text.trim()
        val idle = idleResetMs
        if (idle != null && _items.value.isNotEmpty() && now() - lastActive >= idle) {
            clear()
            error("Started a new chat: the last one had been idle for over ${idle / HOUR_MS} hours.")
        }
        lastActive = now()
        _items.update { it + ChatItem.User(said) }
        job = scope.launch { converse(TurnInput.Say(said)) }
    }

    /** Asks again after a failure that [ChatItem.Error.canRetry] says is worth retrying. */
    fun retry() {
        val input = retryInput ?: return
        if (_busy.value) return
        _items.update { list -> list.filterNot { it is ChatItem.Error && it.canRetry } }
        job = scope.launch { converse(input) }
    }

    fun stop() {
        job?.cancel()
        approvals.values.forEach { it.complete(false) }
        _busy.value = false
        finishStreaming()
    }

    fun decide(toolId: String, approve: Boolean) {
        approvals.remove(toolId)?.complete(approve)
    }

    fun approveAllPending() {
        approvals.keys.toList().forEach { decide(it, true) }
    }

    fun clear() {
        stop()
        retryInput = null
        session?.reset()
        _items.value = emptyList()
    }

    /** The session for the backend the user has chosen; switching starts a new thread. */
    private suspend fun sessionFor(): AssistantSession {
        val backend = backends.of(settings())
        val current = session
        if (current != null && runner == backend.runner) return current
        if (current != null) {
            current.reset()
            _items.update { it + ChatItem.Error("Now asking Claude on ${backend.runner.label.lowercase()}. This starts a new conversation.") }
        }
        val only = router.only
        // A Mac session runs Claude Code on one board's computer, so with no single board
        // (runKey null) it asks for the API key instead; the API session handles every board.
        val fresh = backend.session(AssistantBoard(only?.boardKey, only?.boardName ?: "All boards", toolSpecs, system = ::systemPrompt))
        session = fresh
        runner = backend.runner
        return fresh
    }

    private suspend fun converse(first: TurnInput) {
        _busy.value = true
        try {
            val session = sessionFor()
            var input = first
            repeat(MAX_TURNS) {
                retryInput = input
                val result = session.turn(input, sink)
                finishStreaming()
                when (result.stop) {
                    TurnStop.DONE -> return
                    TurnStop.REFUSAL -> return error("Claude declined this request.")
                    TurnStop.TOO_LONG -> return error("Claude ran out of room before finishing.")
                    TurnStop.CALLS -> Unit
                }
                val answers = mutableListOf<ToolAnswer>()
                for (call in result.calls) {
                    val item = ChatItem.ToolCall(
                        call.id, call.name, summarize(call.name, call.input),
                        call.input ?: JsonObject(emptyMap()), ChatItem.ToolCall.State.RUNNING,
                        boardKey = router.only?.boardKey.orEmpty(),
                    )
                    _items.update { it + item }
                    val (content, isError) = execute(item, call.input)
                    answers += ToolAnswer(call.id, call.name, content, isError)
                }
                input = TurnInput.Answers(answers)
            }
            error("Stopped after $MAX_TURNS steps. Ask me to continue if needed.")
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e
        } catch (e: ClaudeUnavailableException) {
            finishStreaming()
            error(e.message ?: "Claude isn't available right now", offerApiKey = e.offerApiKey && settings().apiKeyConfigured, canRetry = e.canRetry)
        } catch (e: Exception) {
            finishStreaming()
            error(e.message ?: "Something went wrong")
        } finally {
            lastActive = now()
            _busy.value = false
        }
    }

    private suspend fun execute(item: ChatItem.ToolCall, input: JsonObject?): Pair<String, Boolean> {
        if (input == null) return finish(item, "INVALID_JSON: the tool input was not valid JSON. Re-issue the call.", failed = true)
        val required = toolSpecs.firstOrNull { it.name == item.name }?.required.orEmpty()
        val missing = required.filter {
            when (val v = input[it]) {
                null -> true
                is JsonArray -> v.isEmpty()
                else -> (v as? JsonPrimitive)?.content.isNullOrBlank()
            }
        }
        if (missing.isNotEmpty()) return finish(item, "Missing required field(s): ${missing.joinToString()}", failed = true)

        // Nothing that changes a bead happens without the user saying yes here,
        // whichever backend asked for it.
        if (item.isWrite && !autoApprove.value) {
            val gate = CompletableDeferred<Boolean>()
            approvals[item.id] = gate
            setState(item.id, ChatItem.ToolCall.State.NEEDS_APPROVAL)
            if (!gate.await()) {
                setState(item.id, ChatItem.ToolCall.State.REJECTED, "Declined")
                return "The user declined this action. Do not retry it unless they ask." to true
            }
            setState(item.id, ChatItem.ToolCall.State.RUNNING)
        }
        return try {
            fun s(k: String) = (input[k] as? JsonPrimitive)?.takeUnless { it.content == "null" }?.content
            val board = s("board")
            val out = when (item.name) {
                "list_boards" -> router.listBoards(s("query"))
                "list_beads" -> router.listBeads(board, s("lane"), s("query"), (input["limit"] as? JsonPrimitive)?.intOrNull ?: 50)
                else -> {
                    // Everything else works on exactly one board; resolving it can fail with advice.
                    val tools = router.resolve(board)
                    noteBoard(item.id, tools.boardKey)
                    when (item.name) {
                        "get_bead" -> tools.getBead(s("id")!!)
                        "update_bead" -> tools.updateBead(s("id")!!, input)
                        "create_bead" -> tools.createBead(input)
                        "close_bead" -> tools.closeBead(s("id")!!, s("reason"))
                        "add_comment" -> tools.addComment(s("id")!!, s("text")!!)
                        "add_dependency" -> tools.addDependency(s("issue_id")!!, s("depends_on_id")!!)
                        "run_in_claude_code" -> tools.runInClaudeCode(s("id")!!, s("instructions"))
                        "start_round" -> tools.startRound(
                            (input["ids"] as? JsonArray)?.mapNotNull { (it as? JsonPrimitive)?.content }.orEmpty(),
                            (input["budget_usd"] as? JsonPrimitive)?.doubleOrNull,
                        )
                        else -> return finish(item, "Unknown tool ${item.name}", failed = true)
                    }
                }
            }
            finish(item, out, failed = false)
        } catch (e: Exception) {
            finish(item, "Error: ${e.message}", failed = true)
        }
    }

    private fun noteBoard(id: String, key: String) {
        if (key.isEmpty()) return
        _items.update { list -> list.map { if (it is ChatItem.ToolCall && it.id == id) it.copy(boardKey = key) else it } }
    }

    private fun finish(item: ChatItem.ToolCall, result: String, failed: Boolean): Pair<String, Boolean> {
        setState(item.id, if (failed) ChatItem.ToolCall.State.FAILED else ChatItem.ToolCall.State.DONE, result)
        return result to failed
    }

    private fun setState(id: String, state: ChatItem.ToolCall.State, result: String? = null) {
        _items.update { list -> list.map { if (it is ChatItem.ToolCall && it.id == id) it.copy(state = state, result = result ?: it.result) else it } }
    }

    private fun appendText(key: String?, delta: String): String {
        if (key == null) {
            val item = ChatItem.Assistant(delta, streaming = true)
            _items.update { it + item }
            return item.key
        }
        _items.update { list -> list.map { if (it is ChatItem.Assistant && it.key == key) it.copy(text = it.text + delta) else it } }
        return key
    }

    private fun appendThinking(key: String?, delta: String): String {
        if (key == null) {
            val item = ChatItem.Thinking(delta, streaming = true)
            _items.update { it + item }
            return item.key
        }
        _items.update { list -> list.map { if (it is ChatItem.Thinking && it.key == key) it.copy(text = it.text + delta) else it } }
        return key
    }

    private fun finishStreaming() {
        textKey = null
        thinkKey = null
        _items.update { list ->
            list.mapNotNull {
                when {
                    it is ChatItem.Assistant && it.streaming -> it.copy(streaming = false)
                    it is ChatItem.Thinking && it.streaming -> if (it.text.isBlank()) null else it.copy(streaming = false)
                    else -> it
                }
            }
        }
    }

    private fun error(text: String, offerApiKey: Boolean = false, canRetry: Boolean = false) {
        _items.update { it + ChatItem.Error(text, offerApiKey, canRetry) }
    }

    private fun systemPrompt(): String {
        val scope = router.only?.let { "You are working on the \"${it.boardName}\" board." }
            ?: "You can see every beads board on Ryder's computers — one board per project — and you work across all of them."
        val howTo = if (everyBoard)
            "Call list_boards first: it costs nothing and gives every board's exact name and bead counts (open, in progress, blocked, done) without reading any board. Read those counts, decide which few boards actually matter to the question, and only then call list_beads with that board's exact name — each board you read is a fetch over Ryder's network, so read the boards you need, not all of them. Pass the exact name as `board` on every other call; never guess a board name or an id. list_beads with a null board is a last resort: it reads at most ${AllBoardsRouter.MAX_BOARDS_SWEPT} boards, skips hidden ones, and ends with a note naming what it didn't read — when that note says the sweep was cut short, tell Ryder so rather than presenting a partial answer as complete. A board marked hidden in list_boards is one Ryder has tucked away: leave it out unless Ryder names it, and then use it like any other. Bead ids are only unique within a board, so always keep a bead's board alongside its id. To orchestrate, find the beads that matter, propose the edits that unblock them, and dispatch the ones ready to be coded with run_in_claude_code on their own board, or, when Ryder wants several beads on one board finished together, propose start_round with them."
        else
            "Use the tools to look things up before answering; never guess ids."
        val refer = if (everyBoard) "Refer to beads by id and title, and say which board each one is on." else "Refer to beads by id and title."
        return """
            You are the board assistant inside Nacre, Ryder's Android app for the beads (bd) issue tracker. $scope

            Beads have an id, title, type (bug, feature, task, chore, epic, decision, spike), priority 0 (critical) to 4 (backlog), a status, labels, a parent (epics contain children) and "blocks" dependencies. A board shows lanes: ready (open, nothing open blocking it), doing (in progress), blocked (status blocked or waiting on an open blocker), later (deferred) and done (closed). A bead with the "human" label, or an open decision, needs Ryder.

            $howTo Changes you make through write tools are shown to Ryder for approval first, so propose concrete edits rather than asking whether to make them. Prefer a few well-aimed calls over many. run_in_claude_code dispatches one bead to Claude Code on Ryder's Mac Studio, which does the coding there. When Ryder asks for several beads to be worked until they're done, propose start_round with them instead: the computer reviews them together, asks Ryder only where they disagree, works each on its own branch, checks and merges them into one commit that passes the gates and builds.

            Answer in short, scannable markdown suited to a phone screen. $refer
        """.trimIndent()
    }

    companion object {
        // Large requests ("start everything that's ready") take many tool steps.
        private const val MAX_TURNS = 40

        const val HOUR_MS = 60 * 60 * 1000L

        /** The boards-screen conversation starts fresh once it has sat this long untouched. */
        const val ORCHESTRATOR_IDLE_RESET_MS = 4 * HOUR_MS

        fun summarize(name: String, input: JsonObject?): String {
            fun s(k: String) = (input?.get(k) as? JsonPrimitive)?.takeUnless { it.content == "null" }?.content
            val what = when (name) {
                "list_boards" -> s("query")?.let { "\"$it\"" } ?: "all boards"
                "list_beads" -> listOfNotNull(s("lane")?.let { "lane: $it" }, s("query")?.let { "\"$it\"" }).joinToString(" · ").ifEmpty { "all beads" }
                "get_bead" -> s("id").orEmpty()
                "update_bead" -> "${s("id")}: " + (input?.keys?.filter { it != "id" && it != "board" && (s(it) != null || (it == "labels" && input[it] !is JsonPrimitive)) }?.joinToString() ?: "")
                "create_bead" -> s("title").orEmpty()
                "close_bead" -> listOfNotNull(s("id"), s("reason")).joinToString(" · ")
                "add_comment" -> "${s("id")}: ${s("text")?.take(80)}"
                "add_dependency" -> "${s("issue_id")} waits on ${s("depends_on_id")}"
                "run_in_claude_code" -> s("id").orEmpty()
                "start_round" -> (input?.get("ids") as? JsonArray)?.joinToString(", ") { (it as? JsonPrimitive)?.content.orEmpty() }.orEmpty()
                else -> ""
            }
            // Cross-board calls name their board; say which one up front.
            val board = s("board")?.takeUnless { name == "list_boards" }
            return listOfNotNull(board, what.ifBlank { null }).joinToString(" · ")
        }
    }
}
