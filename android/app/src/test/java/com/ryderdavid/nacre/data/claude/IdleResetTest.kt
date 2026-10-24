package com.ryderdavid.nacre.data.claude

import com.ryderdavid.nacre.data.settings.AppSettings
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.JsonObject
import kotlin.test.AfterTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/** The boards-screen conversation starts fresh after sitting idle; others never do. */
class IdleResetTest {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    @AfterTest fun tearDown() = scope.cancel()

    private class EchoSession : AssistantSession {
        var resets = 0

        override suspend fun turn(input: TurnInput, sink: TurnSink): TurnResult {
            sink.newBlock()
            sink.text(REPLY)
            return TurnResult(emptyList(), TurnStop.DONE)
        }

        override fun reset() { resets++ }
    }

    private class Backend(val s: AssistantSession) : ClaudeBackend {
        override val runner = ClaudeRunner.API
        override suspend fun refine(repoKey: String, ctx: BeadContext, instruction: String, history: List<DraftTurn>) = RefineProposal()
        override suspend fun ask(repoKey: String, ctx: BeadContext, history: List<DraftTurn>, question: String) = ""
        override suspend fun draft(repoKey: String, repoName: String, rough: String, parentTitle: String?, existingTitles: List<String>) = BeadDraft("x")
        override suspend fun reviseDraft(repoKey: String, repoName: String, current: BeadDraft, history: List<DraftTurn>, message: String, parentTitle: String?, existingTitles: List<String>) = current
        override fun session(board: AssistantBoard) = s
    }

    private object NoBoard : BoardTools {
        override val boardKey = "c1~nacre"
        override val boardName = "Nacre"
        override suspend fun listBeads(lane: String?, query: String?, limit: Int) = ""
        override suspend fun getBead(id: String) = ""
        override suspend fun updateBead(id: String, fields: JsonObject) = ""
        override suspend fun createBead(fields: JsonObject) = ""
        override suspend fun closeBead(id: String, reason: String?) = ""
        override suspend fun addComment(id: String, text: String) = ""
        override suspend fun addDependency(issueId: String, dependsOnId: String) = ""
        override suspend fun runInClaudeCode(id: String, instructions: String?) = ""
    }

    private var clock = 0L

    private fun assistant(session: AssistantSession, idleResetMs: Long?): BoardAssistant {
        val b = Backend(session)
        return BoardAssistant(ClaudeBackends(b, b), { AppSettings() }, SingleBoard(NoBoard), scope, idleResetMs, now = { clock })
    }

    private suspend fun BoardAssistant.awaitReplies(n: Int) = withTimeout(5_000) {
        items.first { list -> list.count { it is ChatItem.Assistant && it.text == REPLY && !it.streaming } >= n }
    }

    private fun BoardAssistant.questions() = items.value.filterIsInstance<ChatItem.User>().map { it.text }

    @Test fun aQuickFollowUpContinuesTheConversation() = runBlocking {
        val session = EchoSession()
        val a = assistant(session, BoardAssistant.ORCHESTRATOR_IDLE_RESET_MS)
        a.send("one"); a.awaitReplies(1)
        clock += BoardAssistant.ORCHESTRATOR_IDLE_RESET_MS - 1
        a.send("two"); a.awaitReplies(2)
        assertEquals(listOf("one", "two"), a.questions())
        assertEquals(0, session.resets)
    }

    @Test fun afterTheIdleThresholdTheNextMessageStartsANewChatAndSaysSo() = runBlocking {
        val session = EchoSession()
        val a = assistant(session, BoardAssistant.ORCHESTRATOR_IDLE_RESET_MS)
        a.send("one"); a.awaitReplies(1)
        clock += BoardAssistant.ORCHESTRATOR_IDLE_RESET_MS
        a.send("two"); a.awaitReplies(1)
        assertEquals(listOf("two"), a.questions(), "the old conversation is gone")
        assertEquals(1, session.resets, "Claude's side of the thread is reset too")
        val note = a.items.value.filterIsInstance<ChatItem.Error>().single()
        assertTrue("4 hours" in note.text, note.text)
    }

    @Test fun withoutAThresholdAConversationNeverExpires() = runBlocking {
        val a = assistant(EchoSession(), idleResetMs = null)
        a.send("one"); a.awaitReplies(1)
        clock += 30 * BoardAssistant.ORCHESTRATOR_IDLE_RESET_MS
        a.send("two"); a.awaitReplies(2)
        assertEquals(listOf("one", "two"), a.questions())
    }

    private companion object {
        const val REPLY = "ok"
    }
}
