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
import kotlinx.serialization.json.intOrNull
import java.util.UUID

/** The app side of the assistant's tools: reads run freely, writes only after approval. */
interface BoardTools {
    /** The repo key this board lives under, used to reach its computer. */
    val boardKey: String
    val repoName: String
    suspend fun listBeads(lane: String?, query: String?, limit: Int): String
    suspend fun getBead(id: String): String
    suspend fun updateBead(id: String, fields: JsonObject): String
    suspend fun createBead(fields: JsonObject): String
    suspend fun closeBead(id: String, reason: String?): String
    suspend fun addComment(id: String, text: String): String
    suspend fun addDependency(issueId: String, dependsOnId: String): String
    suspend fun runInClaudeCode(id: String, instructions: String?): String
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
        override val key: String = id,
    ) : ChatItem {
        enum class State { RUNNING, NEEDS_APPROVAL, DONE, FAILED, REJECTED }
        val isWrite get() = name in WRITE_TOOLS
    }

    /**
     * Something went wrong. [offerApiKey] marks the failures the user can fix
     * by running Claude on this phone's API key instead.
     */
    data class Error(
        val text: String,
        val offerApiKey: Boolean = false,
        override val key: String = UUID.randomUUID().toString(),
    ) : ChatItem
}

/**
 * A conversation with Claude about one board. [backends] decides each new
 * conversation's home — the Claude API with this phone's key, or Claude Code
 * on the board's own computer — and the loop below is the same either way:
 * Claude asks for tools, write tools wait for the user's approval unless
 * [autoApprove] is on, and the app makes every change itself.
 */
class BoardAssistant(
    private val backends: ClaudeBackends,
    private val settings: suspend () -> AppSettings,
    private val tools: BoardTools,
    private val scope: CoroutineScope,
) {
    private val approvals = mutableMapOf<String, CompletableDeferred<Boolean>>()

    private var session: AssistantSession? = null
    private var runner: ClaudeRunner? = null

    private val _items = MutableStateFlow<List<ChatItem>>(emptyList())
    val items: StateFlow<List<ChatItem>> = _items.asStateFlow()

    private val _busy = MutableStateFlow(false)
    val busy: StateFlow<Boolean> = _busy.asStateFlow()

    val autoApprove = MutableStateFlow(false)
    private var job: Job? = null

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
        _items.update { it + ChatItem.User(said) }
        job = scope.launch { converse(TurnInput.Say(said)) }
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
        val fresh = backend.session(AssistantBoard(tools.boardKey, tools.repoName, system = ::systemPrompt))
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
            error(e.message ?: "Claude isn't available right now", offerApiKey = e.offerApiKey && settings().apiKeyConfigured)
        } catch (e: Exception) {
            finishStreaming()
            error(e.message ?: "Something went wrong")
        } finally {
            _busy.value = false
        }
    }

    private suspend fun execute(item: ChatItem.ToolCall, input: JsonObject?): Pair<String, Boolean> {
        if (input == null) return finish(item, "INVALID_JSON: the tool input was not valid JSON. Re-issue the call.", failed = true)
        val missing = REQUIRED[item.name].orEmpty().filter { input[it] == null || (input[it] as? JsonPrimitive)?.content.isNullOrBlank() }
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
            val out = when (item.name) {
                "list_beads" -> tools.listBeads(s("lane"), s("query"), (input["limit"] as? JsonPrimitive)?.intOrNull ?: 50)
                "get_bead" -> tools.getBead(s("id")!!)
                "update_bead" -> tools.updateBead(s("id")!!, input)
                "create_bead" -> tools.createBead(input)
                "close_bead" -> tools.closeBead(s("id")!!, s("reason"))
                "add_comment" -> tools.addComment(s("id")!!, s("text")!!)
                "add_dependency" -> tools.addDependency(s("issue_id")!!, s("depends_on_id")!!)
                "run_in_claude_code" -> tools.runInClaudeCode(s("id")!!, s("instructions"))
                else -> return finish(item, "Unknown tool ${item.name}", failed = true)
            }
            finish(item, out, failed = false)
        } catch (e: Exception) {
            finish(item, "Error: ${e.message}", failed = true)
        }
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

    private fun error(text: String, offerApiKey: Boolean = false) {
        _items.update { it + ChatItem.Error(text, offerApiKey) }
    }

    private fun systemPrompt() = """
        You are the board assistant inside Nacre, Ryder's Android app for the beads (bd) issue tracker. You are working on the "${tools.repoName}" board.

        Beads have an id, title, type (bug, feature, task, chore, epic, decision, spike), priority 0 (critical) to 4 (backlog), a status, labels, a parent (epics contain children) and "blocks" dependencies. The board shows lanes: ready (open, nothing open blocking it), doing (in progress), blocked (status blocked or waiting on an open blocker), later (deferred) and done (closed). A bead with the "human" label, or an open decision, needs Ryder.

        Use the tools to look things up before answering; never guess ids. Changes you make through write tools are shown to Ryder for approval first, so propose concrete edits rather than asking whether to make them. Prefer a few well-aimed calls over many. run_in_claude_code dispatches a bead to Claude Code on Ryder's Mac Studio, which does the actual coding work there.

        Answer in short, scannable markdown suited to a phone screen. Refer to beads by id and title.
    """.trimIndent()

    companion object {
        // Large requests ("start everything that's ready") take many tool steps.
        private const val MAX_TURNS = 40

        fun summarize(name: String, input: JsonObject?): String {
            fun s(k: String) = (input?.get(k) as? JsonPrimitive)?.takeUnless { it.content == "null" }?.content
            return when (name) {
                "list_beads" -> listOfNotNull(s("lane")?.let { "lane: $it" }, s("query")?.let { "\"$it\"" }).joinToString(" · ").ifEmpty { "all beads" }
                "get_bead" -> s("id").orEmpty()
                "update_bead" -> "${s("id")}: " + (input?.keys?.filter { it != "id" && s(it) != null || (it == "labels" && input[it] !is JsonPrimitive) }?.joinToString() ?: "")
                "create_bead" -> s("title").orEmpty()
                "close_bead" -> listOfNotNull(s("id"), s("reason")).joinToString(" · ")
                "add_comment" -> "${s("id")}: ${s("text")?.take(80)}"
                "add_dependency" -> "${s("issue_id")} waits on ${s("depends_on_id")}"
                "run_in_claude_code" -> s("id").orEmpty()
                else -> ""
            }
        }
    }
}
