package com.ryderdavid.nacre.data.claude

import com.anthropic.core.JsonValue
import com.anthropic.helpers.MessageAccumulator
import com.anthropic.models.messages.ContentBlockParam
import com.anthropic.models.messages.MessageParam
import com.anthropic.models.messages.OutputConfig
import com.anthropic.models.messages.StopReason
import com.anthropic.models.messages.ThinkingConfigAdaptive
import com.anthropic.models.messages.Tool
import com.anthropic.models.messages.ToolResultBlockParam
import com.ryderdavid.nacre.data.claude.ClaudeService.Companion.toJsonString
import com.ryderdavid.nacre.data.settings.AppSettings
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.util.UUID

/** The app side of the assistant's tools on one board: reads run freely, writes only after approval. */
interface BoardTools {
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
    data class Error(val text: String, override val key: String = UUID.randomUUID().toString()) : ChatItem
}

/** One user message answering [ids] in order; any without a result are marked not run. */
internal fun toolResults(ids: List<String>, results: Map<String, Pair<String, Boolean>>): MessageParam {
    val blocks = ids.map { id ->
        val (content, isError) = results[id] ?: ("Not run: the step was interrupted before this tool call ran." to true)
        ContentBlockParam.ofToolResult(ToolResultBlockParam.builder().toolUseId(id).content(content).isError(isError).build())
    }
    // All results go back in one user message so parallel tool use keeps working.
    return MessageParam.builder().role(MessageParam.Role.USER).contentOfBlockParams(blocks).build()
}

/** The message that repairs [history] when its last assistant turn asked for tools nobody answered. */
internal fun answerDanglingToolUse(history: List<MessageParam>): MessageParam? {
    val last = history.lastOrNull() ?: return null
    if (last.role() != MessageParam.Role.ASSISTANT) return null
    val ids = last.content().blockParams().orElse(emptyList()).mapNotNull { b -> b.toolUse().orElse(null)?.id() }
    return if (ids.isEmpty()) null else toolResults(ids, emptyMap())
}

internal val WRITE_TOOLS = setOf("update_bead", "create_bead", "close_bead", "add_comment", "add_dependency", "run_in_claude_code")

/**
 * A conversation with Claude about one board, or about every board at once — [router]
 * decides which. Runs a manual tool loop over streaming requests; write tools pause
 * for the user's approval unless [autoApprove] is on.
 */
class BoardAssistant(
    private val service: ClaudeService,
    private val settings: suspend () -> AppSettings,
    private val router: BoardRouter,
    private val scope: CoroutineScope,
) {
    private val everyBoard = router.only == null
    private val toolUnions by lazy { toolDefs(everyBoard).map { com.anthropic.models.messages.ToolUnion.ofTool(it) } }

    private val history = mutableListOf<MessageParam>()
    private val approvals = mutableMapOf<String, CompletableDeferred<Boolean>>()

    private val _items = MutableStateFlow<List<ChatItem>>(emptyList())
    val items: StateFlow<List<ChatItem>> = _items.asStateFlow()

    private val _busy = MutableStateFlow(false)
    val busy: StateFlow<Boolean> = _busy.asStateFlow()

    val autoApprove = MutableStateFlow(false)
    private var job: Job? = null

    fun send(text: String) {
        if (text.isBlank() || _busy.value) return
        closeDanglingToolUse()
        _items.update { it + ChatItem.User(text.trim()) }
        history += MessageParam.builder().role(MessageParam.Role.USER).content(text.trim()).build()
        job = scope.launch { loop() }
    }

    fun stop() {
        job?.cancel()
        approvals.values.forEach { it.complete(false) }
        _busy.value = false
        _items.update { list -> list.map { if (it is ChatItem.Assistant && it.streaming) it.copy(streaming = false) else if (it is ChatItem.Thinking && it.streaming) it.copy(streaming = false) else it } }
    }

    fun decide(toolId: String, approve: Boolean) {
        approvals.remove(toolId)?.complete(approve)
    }

    fun approveAllPending() {
        approvals.keys.toList().forEach { decide(it, true) }
    }

    fun clear() {
        stop()
        history.clear()
        _items.value = emptyList()
    }

    private suspend fun loop() {
        _busy.value = true
        try {
            val s = settings()
            val client = service.client(s.anthropicKey)
            repeat(MAX_TURNS) {
                val params = ClaudeService.baseParams(s, systemPrompt(), 64000)
                    .thinking(ThinkingConfigAdaptive.builder().display(ThinkingConfigAdaptive.Display.SUMMARIZED).build())
                    .outputConfig(OutputConfig.builder().effort(ClaudeService.effortOf(s.claudeEffort)).build())
                    .tools(toolUnions)
                    .messages(history.toList())
                    .build()

                val acc = MessageAccumulator.create()
                var textKey: String? = null
                var thinkKey: String? = null
                withContext(Dispatchers.IO) {
                    ClaudeService.call {
                        client.messages().createStreaming(params).use { stream ->
                            stream.stream().forEach { event ->
                                ensureActive()
                                acc.accumulate(event)
                                event.contentBlockStart().ifPresent { textKey = null; thinkKey = null }
                                event.contentBlockDelta().ifPresent { d ->
                                    d.delta().text().ifPresent { t -> textKey = appendText(textKey, t.text()) }
                                    d.delta().thinking().ifPresent { t -> thinkKey = appendThinking(thinkKey, t.thinking()) }
                                }
                            }
                        }
                    }
                }
                finishStreaming()
                val msg = acc.message()
                history += msg.toParam()

                val stop = msg.stopReason().orElse(null)
                val calls = msg.content().mapNotNull { it.toolUse().orElse(null) }
                // Every tool_use must be answered by a tool_result in the next message, or the
                // API rejects the whole conversation from then on. Calls are only executed after
                // a normal tool_use stop; on any other stop (or if this step is interrupted) the
                // unanswered ones are recorded as not run.
                val results = LinkedHashMap<String, Pair<String, Boolean>>()
                try {
                    if (stop == StopReason.TOOL_USE) for (call in calls) {
                        val input = runCatching {
                            ClaudeService.json.parseToJsonElement(call._input().toJsonString()).jsonObject
                        }.getOrNull()
                        val item = ChatItem.ToolCall(
                            call.id(), call.name(), summarize(call.name(), input), input ?: JsonObject(emptyMap()),
                            ChatItem.ToolCall.State.RUNNING, boardKey = router.only?.boardKey.orEmpty(),
                        )
                        _items.update { it + item }
                        results[call.id()] = execute(item, input)
                    }
                } finally {
                    if (calls.isNotEmpty()) history += toolResults(calls.map { it.id() }, results)
                }

                when (stop) {
                    StopReason.TOOL_USE -> Unit
                    StopReason.REFUSAL -> { error("Claude declined this request."); return }
                    StopReason.MAX_TOKENS -> { error("Claude ran out of room before finishing."); return }
                    else -> return
                }
            }
            error("Stopped after $MAX_TURNS steps. Ask me to continue if needed.")
        } catch (e: kotlinx.coroutines.CancellationException) {
            throw e
        } catch (e: Exception) {
            finishStreaming()
            error(e.message ?: "Something went wrong")
        } finally {
            _busy.value = false
        }
    }

    private fun closeDanglingToolUse() {
        answerDanglingToolUse(history)?.let { history += it }
    }

    private suspend fun execute(item: ChatItem.ToolCall, input: JsonObject?): Pair<String, Boolean> {
        if (input == null) return finish(item, "INVALID_JSON: the tool input was not valid JSON. Re-issue the call.", failed = true)
        val missing = REQUIRED[item.name].orEmpty().filter { input[it] == null || (input[it] as? JsonPrimitive)?.content.isNullOrBlank() }
        if (missing.isNotEmpty()) return finish(item, "Missing required field(s): ${missing.joinToString()}", failed = true)

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

    private fun error(text: String) {
        _items.update { it + ChatItem.Error(text) }
    }

    private fun systemPrompt(): String {
        val scope = router.only?.let { "You are working on the \"${it.boardName}\" board." }
            ?: "You can see every beads board on Ryder's computers — one board per project — and you work across all of them."
        val howTo = if (everyBoard)
            "Call list_boards first to learn the board names, then pass the exact name as `board` on every other call; never guess a board name or an id. list_beads with a null board sweeps every board at once, which is the way to answer \"what's next everywhere\" questions. Bead ids are only unique within a board, so always keep a bead's board alongside its id. To orchestrate, find the beads that matter, propose the edits that unblock them, and dispatch the ones ready to be coded with run_in_claude_code on their own board."
        else
            "Use the tools to look things up before answering; never guess ids."
        val refer = if (everyBoard) "Refer to beads by id and title, and say which board each one is on." else "Refer to beads by id and title."
        return """
            You are the board assistant inside Nacre, Ryder's Android app for the beads (bd) issue tracker. $scope

            Beads have an id, title, type (bug, feature, task, chore, epic, decision, spike), priority 0 (critical) to 4 (backlog), a status, labels, a parent (epics contain children) and "blocks" dependencies. A board shows lanes: ready (open, nothing open blocking it), doing (in progress), blocked (status blocked or waiting on an open blocker), later (deferred) and done (closed). A bead with the "human" label, or an open decision, needs Ryder.

            $howTo Changes you make through write tools are shown to Ryder for approval first, so propose concrete edits rather than asking whether to make them. Prefer a few well-aimed calls over many. run_in_claude_code dispatches a bead to Claude Code on Ryder's Mac Studio, which does the actual coding work there.

            Answer in short, scannable markdown suited to a phone screen. $refer
        """.trimIndent()
    }

    companion object {
        // Large requests ("start everything that's ready") take many tool steps.
        private const val MAX_TURNS = 40

        private val REQUIRED = mapOf(
            "get_bead" to listOf("id"),
            "update_bead" to listOf("id"),
            "create_bead" to listOf("title"),
            "close_bead" to listOf("id"),
            "add_comment" to listOf("id", "text"),
            "add_dependency" to listOf("issue_id", "depends_on_id"),
            "run_in_claude_code" to listOf("id"),
        )

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
                else -> ""
            }
            // Cross-board calls name their board; say which one up front.
            val board = s("board")?.takeUnless { name == "list_boards" }
            return listOfNotNull(board, what.ifBlank { null }).joinToString(" · ")
        }

        private fun str(desc: String) = mapOf("type" to "string", "description" to desc)
        private fun nullable(type: String, desc: String) = mapOf("anyOf" to listOf(mapOf("type" to type), mapOf("type" to "null")), "description" to desc)

        /** Strict tools must list every property as required, so optional values use a nullable type. */
        private fun tool(name: String, description: String, props: Map<String, Any>): Tool =
            Tool.builder()
                .name(name)
                .description(description)
                .strict(true)
                .eagerInputStreaming(true)
                .inputSchema(
                    Tool.InputSchema.builder()
                        .properties(Tool.InputSchema.Properties.builder().apply { props.forEach { (k, v) -> putAdditionalProperty(k, JsonValue.from(v)) } }.build())
                        .required(props.keys.toList())
                        .putAdditionalProperty("additionalProperties", JsonValue.from(false))
                        .build()
                )
                .build()

        private class Spec(val name: String, val description: String, val props: Map<String, Any>)

        /** The tools that act on one board. Cross-board conversations prepend a `board` argument. */
        private val BOARD_SPECS: List<Spec> = listOf(
            Spec(
                "list_beads", "List beads as compact lines (id, lane, priority, type, title, blockers). Use lane to narrow down and query for text search.",
                mapOf(
                    "lane" to nullable("string", "ready | doing | blocked | later | done | open (everything not done), or null for all"),
                    "query" to nullable("string", "Text to match in id, title, labels and description, or null"),
                    "limit" to nullable("integer", "Maximum rows, default 50"),
                ),
            ),
            Spec("get_bead", "Get one bead in full: fields, dependencies, children and comments.", mapOf("id" to str("Bead id"))),
            Spec(
                "update_bead", "Change fields of a bead. Pass null for fields to leave unchanged. labels replaces the whole set. Requires the user's approval.",
                mapOf(
                    "id" to str("Bead id"),
                    "title" to nullable("string", "New title"),
                    "description" to nullable("string", "New description (markdown)"),
                    "design" to nullable("string", "New design notes"),
                    "acceptance_criteria" to nullable("string", "New acceptance criteria"),
                    "notes" to nullable("string", "New notes"),
                    "status" to nullable("string", "open | in_progress | blocked | deferred"),
                    "priority" to nullable("integer", "0-4"),
                    "issue_type" to nullable("string", "bug | feature | task | chore | epic | decision | spike"),
                    "assignee" to nullable("string", "Assignee name"),
                    "labels" to mapOf("anyOf" to listOf(mapOf("type" to "array", "items" to mapOf("type" to "string")), mapOf("type" to "null"))),
                ),
            ),
            Spec(
                "create_bead", "Create a new bead. Requires the user's approval.",
                mapOf(
                    "title" to str("Imperative title under 80 characters"),
                    "description" to str("Why and what (markdown)"),
                    "issue_type" to str("bug | feature | task | chore | epic | decision | spike"),
                    "priority" to mapOf("type" to "integer", "description" to "0-4"),
                    "parent" to nullable("string", "Parent bead id, e.g. an epic"),
                    "acceptance_criteria" to str("Testable acceptance criteria, or empty"),
                    "labels" to mapOf("type" to "array", "items" to mapOf("type" to "string")),
                ),
            ),
            Spec("close_bead", "Close a bead as done. Requires the user's approval.", mapOf("id" to str("Bead id"), "reason" to nullable("string", "One-line close reason"))),
            Spec("add_comment", "Add a comment to a bead. Requires the user's approval.", mapOf("id" to str("Bead id"), "text" to str("Comment text (markdown)"))),
            Spec(
                "add_dependency", "Record that one bead waits on (is blocked by) another. Both beads must be on the same board. Requires the user's approval.",
                mapOf("issue_id" to str("The bead that waits"), "depends_on_id" to str("The bead it waits on")),
            ),
            Spec(
                "run_in_claude_code", "Dispatch a bead to Claude Code on Ryder's Mac Studio to do the work. Requires the user's approval.",
                mapOf("id" to str("Bead id"), "instructions" to nullable("string", "Extra instructions for the coding agent")),
            ),
        )

        private val LIST_BOARDS = Spec(
            "list_boards",
            "List every board (project) you can reach, with its bead counts, computers and path. Call this before anything else so you know the board names.",
            mapOf("query" to nullable("string", "Text to match in the board name, path or git remote, or null for all boards")),
        )

        private fun boardParam(tool: String) =
            if (tool == "list_beads") nullable("string", "Board name from list_boards, or null to sweep every board at once (a few rows from each)")
            else str("Board name, exactly as list_boards reports it")

        /**
         * The tool definitions. With [everyBoard] the assistant can also list the boards,
         * and every board-scoped tool takes the board it acts on.
         */
        fun toolDefs(everyBoard: Boolean): List<Tool> {
            if (!everyBoard) return BOARD_SPECS.map { tool(it.name, it.description, it.props) }
            return listOf(tool(LIST_BOARDS.name, LIST_BOARDS.description, LIST_BOARDS.props)) +
                BOARD_SPECS.map { s -> tool(s.name, s.description, mapOf("board" to boardParam(s.name)) + s.props) }
        }
    }
}

/** Labels may arrive as a JSON array in update/create inputs. */
internal fun JsonObject.stringList(key: String): List<String>? =
    (this[key] as? kotlinx.serialization.json.JsonArray)?.jsonArray?.map { it.jsonPrimitive.content }
