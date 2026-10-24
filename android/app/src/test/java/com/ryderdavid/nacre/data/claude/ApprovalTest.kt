package com.ryderdavid.nacre.data.claude

import com.ryderdavid.nacre.data.settings.AppSettings
import com.ryderdavid.nacre.data.settings.Host
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlin.test.AfterTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * The assistant may only change a board through the app, and only once the
 * user has said yes — whichever backend asked. These tests drive
 * [BoardAssistant] with a scripted session, so they hold for the Claude API
 * path and the Mac path alike.
 */
class ApprovalTest {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    @AfterTest fun tearDown() = scope.cancel()

    /** Asks for [calls] on the first turn, then finishes. */
    private class ScriptedSession(private val calls: List<ToolRequest>) : AssistantSession {
        val inputs = mutableListOf<TurnInput>()
        var resets = 0

        override suspend fun turn(input: TurnInput, sink: TurnSink): TurnResult {
            inputs += input
            if (inputs.size == 1 && calls.isNotEmpty()) return TurnResult(calls, TurnStop.CALLS)
            sink.newBlock()
            sink.text(DONE)
            return TurnResult(emptyList(), TurnStop.DONE)
        }

        override fun reset() {
            resets++
            inputs.clear()
        }
    }

    private class FakeBackend(override val runner: ClaudeRunner, val session: AssistantSession) : ClaudeBackend {
        override suspend fun refine(repoKey: String, ctx: BeadContext, instruction: String) = RefineProposal()
        override suspend fun draft(repoKey: String, repoName: String, rough: String, parentTitle: String?, existingTitles: List<String>) = BeadDraft("x")
        override suspend fun reviseDraft(repoKey: String, repoName: String, current: BeadDraft, history: List<DraftTurn>, message: String, parentTitle: String?, existingTitles: List<String>) = current
        override fun session(board: AssistantBoard) = session
    }

    /** Records what the app was actually asked to change. */
    private class RecordingTools : BoardTools {
        override val boardKey = "c1~nacre"
        override val boardName = "Nacre"
        val writes = mutableListOf<String>()

        override suspend fun listBeads(lane: String?, query: String?, limit: Int) = "NAC-1 [ready] P2 task — Something"
        override suspend fun getBead(id: String) = "NAC-1"
        override suspend fun updateBead(id: String, fields: JsonObject) = "Updated: $id".also { writes += "update $id" }
        override suspend fun createBead(fields: JsonObject) = "Created".also { writes += "create" }
        override suspend fun closeBead(id: String, reason: String?) = "Closed: $id".also { writes += "close $id" }
        override suspend fun addComment(id: String, text: String) = "Commented".also { writes += "comment $id" }
        override suspend fun addDependency(issueId: String, dependsOnId: String) = "Linked".also { writes += "dep" }
        override suspend fun runInClaudeCode(id: String, instructions: String?) = "Started".also { writes += "run $id" }
        override suspend fun startRound(ids: List<String>, budgetUsd: Double?) = "Round started".also { writes += "round ${ids.joinToString(",")} $budgetUsd" }
    }

    private fun assistant(session: AssistantSession, tools: BoardTools, settings: AppSettings = AppSettings(hosts = listOf(HOST))): BoardAssistant {
        val backend = FakeBackend(ClaudeRunner.MAC, session)
        return BoardAssistant(ClaudeBackends(backend, backend), { settings }, SingleBoard(tools), scope)
    }

    private suspend fun BoardAssistant.awaitPending(): ChatItem.ToolCall = withTimeout(TIMEOUT) {
        items.first { list -> list.any { it is ChatItem.ToolCall && it.state == ChatItem.ToolCall.State.NEEDS_APPROVAL } }
            .first { it is ChatItem.ToolCall && it.state == ChatItem.ToolCall.State.NEEDS_APPROVAL } as ChatItem.ToolCall
    }

    /**
     * Waits for [n] finished conversations. The scripted session signs off with
     * [DONE] on its last turn, which is a definite signal — unlike `busy`, which
     * a conversation with nothing to approve can come and go through before a
     * collector ever sees it.
     */
    private suspend fun BoardAssistant.awaitFinished(n: Int = 1) = withTimeout(TIMEOUT) {
        items.first { list -> list.count { it is ChatItem.Assistant && it.text == DONE } >= n }
    }

    @Test fun aWriteWaitsForApprovalAndThenRuns() = runBlocking {
        val tools = RecordingTools()
        val session = ScriptedSession(listOf(ToolRequest("p0", "close_bead", buildJsonObject { put("id", "NAC-1"); put("reason", "done") })))
        val a = assistant(session, tools)

        a.send("close NAC-1")
        val pending = a.awaitPending()
        assertEquals("close_bead", pending.name)
        assertTrue(pending.isWrite)
        assertEquals(emptyList(), tools.writes, "nothing is applied while the user is still deciding")

        a.decide(pending.id, true)
        a.awaitFinished()
        assertEquals(listOf("close NAC-1"), tools.writes)
        // Claude is told what came of it, so the conversation can carry on.
        val answers = (session.inputs.last() as TurnInput.Answers).answers
        assertEquals("Closed: NAC-1", answers.single().content)
        assertTrue(!answers.single().isError)
    }

    @Test fun aRoundIsProposedAndStartsOnlyOnceApproved() = runBlocking {
        val tools = RecordingTools()
        val ids = kotlinx.serialization.json.JsonArray(listOf(kotlinx.serialization.json.JsonPrimitive("NAC-1"), kotlinx.serialization.json.JsonPrimitive("NAC-2")))
        val session = ScriptedSession(listOf(ToolRequest("p0", "start_round", buildJsonObject { put("ids", ids); put("budget_usd", 12.0) })))
        val a = assistant(session, tools)

        a.send("finish NAC-1 and NAC-2")
        val pending = a.awaitPending()
        assertEquals("start_round", pending.name)
        assertTrue(pending.isWrite, "starting a round spends the subscription, so it waits for approval")
        assertEquals("NAC-1, NAC-2", pending.summary)
        assertEquals(emptyList(), tools.writes)

        a.decide(pending.id, true)
        a.awaitFinished()
        assertEquals(listOf("round NAC-1,NAC-2 12.0"), tools.writes)
    }

    @Test fun aDeclinedWriteNeverTouchesTheBoard() = runBlocking {
        val tools = RecordingTools()
        val session = ScriptedSession(listOf(ToolRequest("p0", "update_bead", buildJsonObject { put("id", "NAC-1"); put("priority", 0) })))
        val a = assistant(session, tools)

        a.send("make it critical")
        val pending = a.awaitPending()
        a.decide(pending.id, false)
        a.awaitFinished()

        assertEquals(emptyList(), tools.writes)
        val answers = (session.inputs.last() as TurnInput.Answers).answers
        assertTrue(answers.single().isError)
        assertTrue("declined" in answers.single().content)
        assertTrue(a.items.value.any { it is ChatItem.ToolCall && it.state == ChatItem.ToolCall.State.REJECTED })
    }

    @Test fun readsDoNotWaitForAnybody() = runBlocking {
        val tools = RecordingTools()
        val session = ScriptedSession(listOf(ToolRequest("p0", "list_beads", buildJsonObject { put("lane", "ready") })))
        val a = assistant(session, tools)

        a.send("what's ready?")
        a.awaitFinished()
        assertEquals(emptyList(), tools.writes)
        val call = a.items.value.filterIsInstance<ChatItem.ToolCall>().single()
        assertEquals(ChatItem.ToolCall.State.DONE, call.state, "a read runs without approval")
    }

    @Test fun autoApproveSkipsTheGateDeliberately() = runBlocking {
        val tools = RecordingTools()
        val session = ScriptedSession(listOf(ToolRequest("p0", "close_bead", buildJsonObject { put("id", "NAC-9") })))
        val a = assistant(session, tools)
        a.autoApprove.value = true

        a.send("close NAC-9")
        a.awaitFinished()
        assertEquals(listOf("close NAC-9"), tools.writes)
    }

    @Test fun aWriteMissingItsIdIsRefusedBeforeAnyApproval() = runBlocking {
        val tools = RecordingTools()
        val session = ScriptedSession(listOf(ToolRequest("p0", "close_bead", buildJsonObject { put("reason", "done") })))
        val a = assistant(session, tools)

        a.send("close it")
        a.awaitFinished()
        assertEquals(emptyList(), tools.writes)
        val call = a.items.value.filterIsInstance<ChatItem.ToolCall>().single()
        assertEquals(ChatItem.ToolCall.State.FAILED, call.state)
        assertTrue("Missing required field" in call.result)
    }

    @Test fun stoppingReleasesEveryPendingApproval() = runBlocking {
        val tools = RecordingTools()
        val session = ScriptedSession(listOf(ToolRequest("p0", "create_bead", buildJsonObject { put("title", "New thing") })))
        val a = assistant(session, tools)

        a.send("add a bead")
        a.awaitPending()
        a.stop()
        // The gate was answered "no" on the way out, so the create never runs.
        assertEquals(emptyList(), tools.writes, "a stopped turn applies nothing")
        assertFalse(a.busy.value)
    }

    @Test fun switchingBackendMidConversationStartsFresh() = runBlocking {
        val tools = RecordingTools()
        val session = ScriptedSession(emptyList())
        val api = FakeBackend(ClaudeRunner.API, session)
        val mac = FakeBackend(ClaudeRunner.MAC, session)
        val backends = ClaudeBackends(api, mac)
        var settings = AppSettings(hosts = listOf(HOST))
        val a = BoardAssistant(backends, { settings }, SingleBoard(tools), scope)

        a.send("hello")
        a.awaitFinished()
        settings = settings.copy(claudeRunner = AppSettings.RUNNER_API, anthropicKey = "sk-ant-x")
        a.send("hello again")
        a.awaitFinished(n = 2)

        assertEquals(1, session.resets, "the new backend gets a clean conversation")
        assertTrue(a.items.value.any { it is ChatItem.Error && "new conversation" in it.text }, "and the user is told")
    }

    companion object {
        private const val DONE = "All done."
        private const val TIMEOUT = 5_000L
        private val HOST = Host("c1", "h1", "Mac Studio", "http://mac:7420", "tok")
    }
}
