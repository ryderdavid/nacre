package com.ryderdavid.nacre.ui.workspace.chat

import com.ryderdavid.nacre.data.chat.ChatItem
import com.ryderdavid.nacre.data.chat.ChatTranscript
import com.ryderdavid.nacre.data.chat.NEEDS_YOU_SHOWN
import com.ryderdavid.nacre.data.chat.PermissionCard
import com.ryderdavid.nacre.data.model.ChatEvent
import com.ryderdavid.nacre.data.model.ChatRecord
import com.ryderdavid.nacre.data.model.Decision
import com.ryderdavid.nacre.data.model.DecisionOption
import com.ryderdavid.nacre.notify.ChatNotifier
import com.ryderdavid.nacre.ui.workspace.needsYouThread
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertIs
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** Needs you as a conversation: what the slot above the composer asks, receipts, and the walk-through's chat. */
class NeedsYouInChatTest {
    private val approve = Decision(id = "a", kind = "approve", title = "Approve export-chats", state = "pending",
        options = listOf(DecisionOption("run_hold", "Approve, run, I'll land", detail = "Stops before main", recommended = true), DecisionOption("approve_only", "Approve only")),
        consequences = listOf("Up to $25", "Pushes main"))
    private val question = Decision(id = "q", kind = "question", title = "Where do titles live?", state = "pending")
    private val skipped = Decision(id = "s", kind = "question", state = "deferred")
    private val done = Decision(id = "d", kind = "land", title = "Land r-41", state = "done")
    private val queue = listOf(question, approve, skipped, done)

    @Test fun `a permission request comes before any decision`() {
        val card = PermissionCard("r1", "Bash", "./gradlew assembleRelease", "")
        assertIs<AskTarget.Permission>(AskSlotModel.target(listOf(card), "a", queue, walkThrough = true, turnRunning = true))
    }

    @Test fun `the asked decision is shown while it waits, even mid-turn`() {
        val t = AskSlotModel.target(emptyList(), "a", queue, walkThrough = false, turnRunning = true)
        assertEquals(AskTarget.Ask(approve, next = false), t)
    }

    @Test fun `an answered or skipped ask gives way to the next waiting one, but only in a walk-through between turns`() {
        assertEquals(AskTarget.Ask(question, next = true), AskSlotModel.target(emptyList(), "d", queue, walkThrough = true, turnRunning = false))
        assertEquals(AskTarget.Ask(question, next = true), AskSlotModel.target(emptyList(), "s", queue, walkThrough = true, turnRunning = false))
        assertNull(AskSlotModel.target(emptyList(), "d", queue, walkThrough = true, turnRunning = true))
        assertNull(AskSlotModel.target(emptyList(), "d", queue, walkThrough = false, turnRunning = false))
        assertNull(AskSlotModel.target(emptyList(), null, listOf(done, skipped), walkThrough = true, turnRunning = false))
    }

    @Test fun `a failed outcome not yet seen counts, as nacred counts it`() {
        val failed = Decision(id = "f", kind = "approve", state = "failed")
        assertEquals(3, AskSlotModel.left(queue + failed))
        assertEquals(2, AskSlotModel.left(queue + failed.copy(acknowledged = true)))
        assertEquals(AskTarget.Ask(failed), AskSlotModel.target(emptyList(), "f", queue + failed, walkThrough = false, turnRunning = false))
    }

    @Test fun `the slot says the kind, how many wait, and the focused option's consequence`() {
        assertEquals(2, AskSlotModel.left(queue))
        assertEquals("Approve change · 2 waiting", AskSlotModel.heading(approve, 2, next = false))
        assertEquals("Next · Question", AskSlotModel.heading(question, 1, next = true))
        assertEquals("Stops before main", AskSlotModel.consequence(approve, approve.options[0]))
        assertEquals("Up to $25 · Pushes main", AskSlotModel.consequence(approve, approve.options[1]))
        assertEquals("Answered: Approve only", AskSlotModel.answeredText("Approve only"))
    }

    @Test fun `receipts say where a decision stands`() {
        assertEquals("Waiting for you: Approve export-chats", Receipts.line(approve))
        assertEquals("Land r-41 · Done", Receipts.line(done))
        assertTrue(Receipts.line(skipped).startsWith("Skipped"))
        assertEquals("Allowed: Bash · ls", Receipts.permissionLine(PermissionCard("r", "Bash", "ls", "", state = com.ryderdavid.nacre.data.chat.CardState.ALLOWED)))
    }

    @Test fun `the transcript reads the walk-through's events`() {
        val t = ChatTranscript()
        t.addAll(listOf(
            ChatEvent(seq = 1, kind = "needs_you", text = "2 things need you here: 1 question and 1 approval. Go through them one by one?", total = 2),
            ChatEvent(seq = 2, kind = "user", text = "/needs-you"),
            ChatEvent(seq = 3, kind = "text", text = "2 wait."),
            ChatEvent(seq = 4, kind = "ask", text = "q"),
            ChatEvent(seq = 5, kind = "user", text = "Answered: Per chat", answered = "q"),
            ChatEvent(seq = 6, kind = "ask", text = "a"),
            ChatEvent(seq = 7, kind = "ask", text = "a"),
        ))
        val items = t.shown
        assertEquals(2, (items[0] as ChatItem.HeadsUp).total)
        assertEquals(NEEDS_YOU_SHOWN, (items[1] as ChatItem.User).text)
        assertTrue(AskSlotModel.walkThrough(items))
        // One receipt per decision asked, none for one the chat raised itself.
        val rows = ChatTimeline.receipts(items, raised = setOf("q"))
        assertEquals(listOf("a"), rows.filterIsInstance<ChatItem.Ask>().map { it.decisionId })
    }

    @Test fun `the badge reuses the project's Needs you chat`() {
        val chats = listOf(
            ChatRecord(id = "c1", repo = "nacre", title = "Needs you", updatedAt = "2026-11-09T10:00:00Z", hostId = "h"),
            ChatRecord(id = "c2", repo = "nacre", title = "Needs you", updatedAt = "2026-11-09T11:00:00Z", hostId = "h"),
            ChatRecord(id = "c3", repo = "nacre", title = "Release notes", updatedAt = "2026-11-09T12:00:00Z", hostId = "h"),
            ChatRecord(id = "c4", repo = "other", title = "Needs you", hostId = "h"),
        )
        assertEquals("h~c2", needsYouThread(chats, "h~nacre"))
        assertNull(needsYouThread(chats.filter { it.title != "Needs you" }, "h~nacre"))
    }

    @Test fun `one summary counts what waits`() {
        assertEquals("1 thing needs you", ChatNotifier.summaryTitle(1))
        assertEquals("9 things need you", ChatNotifier.summaryTitle(9))
    }
}
